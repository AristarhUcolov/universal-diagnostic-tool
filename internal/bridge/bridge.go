// Package bridge publishes the current vehicle session as an ELM327 on
// a TCP port (like a Wi-Fi ELM327 adapter). Phone and PC apps that only
// know ELM327 (Torque, Car Scanner, OBD Auto Doctor...) can then use any
// adapter this program supports, and even control units that do not
// speak OBD-II: standard PIDs are synthesised from the control unit's own
// parameters (for example a Микас-7.1 on the old K-Line protocol appears
// as an OBD-II car with engine speed, temperature and voltage).
package bridge

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/elm327"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/vehicle"
)

// Server is the TCP bridge.
type Server struct {
	ln      net.Listener
	session func() *vehicle.Session
	log     journal.Logger
	mu      sync.Mutex
	clients int
	conns   map[net.Conn]bool
}

// Start listens on addr (e.g. "0.0.0.0:35000").
func Start(addr string, session func() *vehicle.Session, log journal.Logger) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("bridge: cannot listen on %s: %w", addr, err)
	}
	s := &Server{ln: ln, session: session, log: journal.Or(log), conns: map[net.Conn]bool{}}
	go s.accept()
	journal.Infof(s.log, "bridge", "ELM327 bridge listening on %s", ln.Addr())
	return s, nil
}

// Addr returns the listening address.
func (s *Server) Addr() string { return s.ln.Addr().String() }

// Clients returns the number of connected apps.
func (s *Server) Clients() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clients
}

// Close stops the bridge.
func (s *Server) Close() error {
	err := s.ln.Close()
	s.mu.Lock()
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	return err
}

func (s *Server) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.clients++
		s.conns[c] = true
		s.mu.Unlock()
		journal.Infof(s.log, "bridge", "app connected from %s", c.RemoteAddr())
		go func() {
			em := elm327.NewEmulator(&backend{s: s})
			em.Desc = "Universal Diagnostic Tool bridge"
			_ = em.Serve(context.Background(), c)
			c.Close()
			s.mu.Lock()
			s.clients--
			delete(s.conns, c)
			s.mu.Unlock()
			journal.Infof(s.log, "bridge", "app %s disconnected", c.RemoteAddr())
		}()
	}
}

// backend implements elm327.Backend on top of the session.
type backend struct {
	s     *Server
	mu    sync.Mutex
	dtcAt time.Time
	dtcs  []dtc.DTC
}

type rawAller interface {
	RawAll(ctx context.Context, req []byte) ([]diag.Message, error)
}

func (b *backend) handler() (ecu.Handler, *vehicle.Session, error) {
	sess := b.s.session()
	if sess == nil {
		return nil, nil, errors.New("not connected to a vehicle")
	}
	e, err := sess.ECU("")
	if err != nil {
		return nil, nil, err
	}
	return e.Handler(), sess, nil
}

// Protocol implements elm327.Backend.
func (b *backend) Protocol() byte {
	h, sess, err := b.handler()
	if err != nil {
		return '6'
	}
	if _, ok := h.(rawAller); !ok {
		return '6' // synthesised OBD answers are given in CAN format
	}
	p := h.Describe().Protocol
	_ = sess
	switch {
	case strings.Contains(p, "29-bit") && strings.Contains(p, "250"):
		return '9'
	case strings.Contains(p, "29-bit"):
		return '7'
	case strings.Contains(p, "CAN") && strings.Contains(p, "250"):
		return '8'
	case strings.Contains(p, "CAN"):
		return '6'
	case strings.Contains(p, "9141"):
		return '3'
	case strings.Contains(p, "5-baud"):
		return '4'
	case strings.Contains(p, "14230"), strings.Contains(p, "KWP"):
		return '5'
	case strings.Contains(p, "VPW"):
		return '2'
	case strings.Contains(p, "PWM"):
		return '1'
	}
	return '6'
}

// Voltage implements elm327.Backend.
func (b *backend) Voltage() float64 {
	sess := b.s.session()
	if sess == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	v, _ := sess.Voltage(ctx)
	return v
}

// Transact implements elm327.Backend.
func (b *backend) Transact(ctx context.Context, proto byte, header []byte, data []byte) ([]elm327.Answer, error) {
	h, sess, err := b.handler()
	if err != nil {
		return nil, err
	}
	sess.Touch()
	b.mu.Lock()
	defer b.mu.Unlock()
	if ra, ok := h.(rawAller); ok && len(header) == 0 {
		msgs, err := ra.RawAll(ctx, data)
		if err != nil {
			return nil, err
		}
		var out []elm327.Answer
		for _, m := range msgs {
			if len(m.Data) > 0 && m.Data[0] != 0x7F {
				out = append(out, elm327.Answer{Src: m.Src, Data: m.Data})
			}
		}
		return out, nil
	}
	if len(header) > 0 || data[0] >= 0x10 {
		// Physical request or manufacturer service: pass it through.
		resp, err := h.Raw(ctx, data)
		if err != nil && len(resp) == 0 {
			return nil, err
		}
		return []elm327.Answer{{Src: 0x7E8, Data: resp}}, nil
	}
	ans, err := b.synthesize(ctx, h, data)
	if err != nil || ans == nil {
		return nil, err
	}
	return []elm327.Answer{{Src: 0x7E8, Data: ans}}, nil
}

// synthesize builds OBD-II answers from a manufacturer control unit.
func (b *backend) synthesize(ctx context.Context, h ecu.Handler, req []byte) ([]byte, error) {
	mapped := map[byte]ecu.Param{}
	for _, p := range h.Params() {
		if p.OBD == "" {
			continue
		}
		var pid byte
		if _, err := fmt.Sscanf(p.OBD, "%02X", &pid); err == nil {
			if _, dup := mapped[pid]; !dup {
				mapped[pid] = p
			}
		}
	}
	switch req[0] {
	case 0x01:
		if len(req) < 2 {
			return nil, nil
		}
		out := []byte{0x41}
		var want []string
		for _, pid := range req[1:] {
			if p, ok := mapped[pid]; ok {
				want = append(want, p.ID)
			}
		}
		values := map[string]ecu.Value{}
		if len(want) > 0 {
			vals, err := h.Read(ctx, want)
			if err != nil {
				return nil, err
			}
			for _, v := range vals {
				values[v.ID] = v
			}
		}
		for _, pid := range req[1:] {
			switch {
			case pid%0x20 == 0:
				out = append(out, pid)
				out = append(out, supportMask(pid, mapped)...)
			case pid == 0x01:
				n, mil := b.dtcSummary(ctx, h)
				a := byte(n) & 0x7F
				if mil {
					a |= 0x80
				}
				out = append(out, 0x01, a, 0x00, 0x00, 0x00)
			default:
				p, ok := mapped[pid]
				if !ok {
					continue
				}
				v := values[p.ID]
				if !v.OK {
					continue
				}
				if enc := encodePID(pid, v.V, p.Unit); enc != nil {
					out = append(out, pid)
					out = append(out, enc...)
				}
			}
		}
		if len(out) == 1 {
			return nil, nil
		}
		return out, nil
	case 0x03, 0x07, 0x0A:
		codes, err := h.ReadDTC(ctx)
		if err != nil {
			return nil, err
		}
		var raw []byte
		for _, d := range codes {
			if req[0] == 0x07 && !d.Pending {
				continue
			}
			if hi, lo, err := dtc.ParseSAE(d.Code); err == nil {
				raw = append(raw, hi, lo)
			} else if sae, ok := d.Extra["sae"]; ok {
				if hi, lo, err := dtc.ParseSAE(sae); err == nil {
					raw = append(raw, hi, lo)
				}
			}
		}
		return append([]byte{req[0] + 0x40, byte(len(raw) / 2)}, raw...), nil
	case 0x04:
		if err := h.ClearDTC(ctx); err != nil {
			return nil, err
		}
		b.dtcAt = time.Time{}
		return []byte{0x44}, nil
	case 0x09:
		if len(req) < 2 {
			return nil, nil
		}
		switch req[1] {
		case 0x00:
			return []byte{0x49, 0x00, 0x40, 0x00, 0x00, 0x00}, nil
		case 0x02:
			items, err := h.Identify(ctx)
			if err != nil {
				return nil, err
			}
			for _, it := range items {
				if it.Key == "vin" && len(strings.TrimSpace(it.Value)) >= 11 {
					v := []byte(strings.TrimSpace(it.Value))
					if len(v) > 17 {
						v = v[:17]
					}
					return append([]byte{0x49, 0x02, 0x01}, v...), nil
				}
			}
		}
	}
	return nil, nil
}

func (b *backend) dtcSummary(ctx context.Context, h ecu.Handler) (int, bool) {
	if time.Since(b.dtcAt) > 10*time.Second {
		if codes, err := h.ReadDTC(ctx); err == nil {
			b.dtcs, b.dtcAt = codes, time.Now()
		}
	}
	mil := false
	for _, d := range b.dtcs {
		mil = mil || d.MIL || d.Active
	}
	return len(b.dtcs), mil
}

// supportMask reports which PIDs base+1..base+0x20 can be synthesised.
func supportMask(base byte, mapped map[byte]ecu.Param) []byte {
	mask := make([]byte, 4)
	pids := []int{0x01}
	for p := range mapped {
		pids = append(pids, int(p))
	}
	sort.Ints(pids)
	for _, p := range pids {
		if p > int(base) && p <= int(base)+0x20 {
			bit := p - int(base) - 1
			mask[bit/8] |= 0x80 >> (bit % 8)
		}
		if p > int(base)+0x20 {
			mask[3] |= 0x01
		}
	}
	return mask
}

func clampByte(v float64) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v + 0.5)
}

func word(v float64) []byte {
	if v < 0 {
		v = 0
	}
	if v > 65535 {
		v = 65535
	}
	x := uint16(v + 0.5)
	return []byte{byte(x >> 8), byte(x)}
}

// encodePID converts a value into OBD-II PID data (SAE J1979 scaling).
func encodePID(pid byte, v float64, unit string) []byte {
	switch pid {
	case 0x04, 0x11, 0x2F, 0x45:
		return []byte{clampByte(v * 255 / 100)}
	case 0x05, 0x0F, 0x46, 0x5C:
		return []byte{clampByte(v + 40)}
	case 0x0B, 0x0D, 0x33:
		return []byte{clampByte(v)}
	case 0x0C:
		return word(v * 4)
	case 0x0E:
		return []byte{clampByte((v + 64) * 2)}
	case 0x10:
		if unit == "kg/h" {
			v /= 3.6
		}
		return word(v * 100)
	case 0x14, 0x15:
		return []byte{clampByte(v * 200), 0xFF}
	case 0x42:
		return word(v * 1000)
	case 0x5E:
		return word(v * 20)
	}
	return nil
}

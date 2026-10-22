// Package doip implements diagnostics over Internet Protocol (ISO 13400-2)
// used by vehicles with an Ethernet diagnostic port (VAG MQB evo, Volvo
// SPA, JLR, Mercedes, many EVs) through an "OBD to RJ45" (ENET) cable,
// and BMW's HSFZ variant (F/G series with the ENET cable).
package doip

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
)

// Payload types.
const (
	TypeNACK               = 0x0000
	TypeVehicleIDRequest   = 0x0001
	TypeVehicleAnnounce    = 0x0004
	TypeRoutingRequest     = 0x0005
	TypeRoutingResponse    = 0x0006
	TypeAliveRequest       = 0x0007
	TypeAliveResponse      = 0x0008
	TypeEntityStatusReq    = 0x4001
	TypeEntityStatusResp   = 0x4002
	TypeDiagnosticMessage  = 0x8001
	TypeDiagnosticPosAck   = 0x8002
	TypeDiagnosticNegAck   = 0x8003
	Port                   = 13400
	DefaultTester          = 0x0E00
	protocolVersion        = 0x02
	routingActivationOK    = 0x10
	maxPayload             = 1 << 20
	defaultResponseTimeout = 2 * time.Second
)

// Header builds a DoIP generic header.
func Header(version byte, ptype uint16, length int) []byte {
	h := make([]byte, 8)
	h[0], h[1] = version, ^version
	binary.BigEndian.PutUint16(h[2:], ptype)
	binary.BigEndian.PutUint32(h[4:], uint32(length))
	return h
}

// Packet is one DoIP message.
type Packet struct {
	Version byte
	Type    uint16
	Payload []byte
}

// ReadPacket reads one message from a stream.
func ReadPacket(r io.Reader) (Packet, error) {
	var h [8]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return Packet{}, err
	}
	if h[0] != ^h[1] {
		return Packet{}, fmt.Errorf("doip: bad header % X", h)
	}
	n := binary.BigEndian.Uint32(h[4:])
	if n > maxPayload {
		return Packet{}, fmt.Errorf("doip: payload too large (%d)", n)
	}
	p := Packet{Version: h[0], Type: binary.BigEndian.Uint16(h[2:]), Payload: make([]byte, n)}
	_, err := io.ReadFull(r, p.Payload)
	return p, err
}

// Encode serialises a packet.
func (p Packet) Encode() []byte {
	v := p.Version
	if v == 0 {
		v = protocolVersion
	}
	return append(Header(v, p.Type, len(p.Payload)), p.Payload...)
}

// Entity is a DoIP node found by discovery.
type Entity struct {
	IP      string `json:"ip"`
	VIN     string `json:"vin"`
	Logical uint16 `json:"logical"`
	EID     string `json:"eid"`
	GID     string `json:"gid"`
}

// Discover broadcasts a vehicle identification request and collects the
// announcements.
func Discover(ctx context.Context, timeout time.Duration) ([]Entity, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	req := Packet{Version: 0xFF, Type: TypeVehicleIDRequest}.Encode()
	if _, err := conn.WriteToUDP(req, &net.UDPAddr{IP: net.IPv4bcast, Port: Port}); err != nil {
		return nil, fmt.Errorf("doip: broadcast failed (is the Ethernet adapter up?): %w", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var out []Entity
	seen := map[string]bool{}
	buf := make([]byte, 2048)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		p, err := ReadPacket(bytesReader(buf[:n]))
		if err != nil || p.Type != TypeVehicleAnnounce || len(p.Payload) < 32 {
			continue
		}
		e := Entity{IP: addr.IP.String(), VIN: hexutil.ASCII(p.Payload[:17]),
			Logical: binary.BigEndian.Uint16(p.Payload[17:19]),
			EID:     hexutil.Format(p.Payload[19:25]), GID: hexutil.Format(p.Payload[25:31])}
		key := fmt.Sprintf("%s/%04X", e.IP, e.Logical)
		if !seen[key] {
			seen[key] = true
			out = append(out, e)
		}
	}
	return out, nil
}

type sliceReader struct {
	b []byte
}

func (s *sliceReader) Read(p []byte) (int, error) {
	if len(s.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s.b)
	s.b = s.b[n:]
	return n, nil
}

func bytesReader(b []byte) io.Reader { return &sliceReader{b: b} }

// Conn is a DoIP diagnostic connection to one target (implements
// diag.Channel).
type Conn struct {
	c      net.Conn
	tester uint16
	target uint16
	log    journal.Logger
	rx     chan Packet
	done   chan struct{}
	err    error
	wmu    sync.Mutex
	addr   string
	closed bool
}

// Dial connects to a DoIP entity and activates routing.
func Dial(ctx context.Context, addr string, tester, target uint16, log journal.Logger) (*Conn, error) {
	if tester == 0 {
		tester = DefaultTester
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, fmt.Sprint(Port))
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("doip: cannot connect to %s: %w", addr, err)
	}
	c := &Conn{c: nc, tester: tester, target: target, log: journal.Or(log), rx: make(chan Packet, 256), done: make(chan struct{}), addr: addr}
	go c.reader()
	req := make([]byte, 7)
	binary.BigEndian.PutUint16(req, tester)
	if err := c.write(Packet{Type: TypeRoutingRequest, Payload: req}); err != nil {
		nc.Close()
		return nil, err
	}
	deadline := time.After(3 * time.Second)
	for {
		select {
		case p := <-c.rx:
			if p.Type != TypeRoutingResponse {
				continue
			}
			if len(p.Payload) < 5 {
				nc.Close()
				return nil, fmt.Errorf("doip: short routing activation answer")
			}
			if code := p.Payload[4]; code != routingActivationOK {
				nc.Close()
				return nil, fmt.Errorf("doip: routing activation refused (code 0x%02X)", code)
			}
			journal.Infof(c.log, "doip", "routing active: tester 0x%04X, entity 0x%04X", tester, binary.BigEndian.Uint16(p.Payload[2:]))
			return c, nil
		case <-deadline:
			nc.Close()
			return nil, fmt.Errorf("doip: no routing activation answer from %s", addr)
		case <-c.done:
			return nil, fmt.Errorf("doip: connection closed: %v", c.err)
		}
	}
}

func (c *Conn) write(p Packet) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.c.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err := c.c.Write(p.Encode())
	return err
}

func (c *Conn) reader() {
	defer close(c.done)
	for {
		p, err := ReadPacket(c.c)
		if err != nil {
			c.err = err
			return
		}
		if p.Type == TypeAliveRequest {
			resp := make([]byte, 2)
			binary.BigEndian.PutUint16(resp, c.tester)
			_ = c.write(Packet{Type: TypeAliveResponse, Payload: resp})
			continue
		}
		select {
		case c.rx <- p:
		default:
		}
	}
}

// Send implements diag.Channel.
func (c *Conn) Send(ctx context.Context, data []byte) error {
	if c.closed {
		return diag.ErrClosed
	}
	payload := make([]byte, 4, 4+len(data))
	binary.BigEndian.PutUint16(payload, c.tester)
	binary.BigEndian.PutUint16(payload[2:], c.target)
	payload = append(payload, data...)
	journal.Tx(c.log, "doip", fmt.Sprintf("%04X", c.target), data, diag.ServiceName(data[0]))
	return c.write(Packet{Type: TypeDiagnosticMessage, Payload: payload})
}

// Recv implements diag.Channel.
func (c *Conn) Recv(ctx context.Context, timeout time.Duration) (diag.Message, error) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case p := <-c.rx:
			switch p.Type {
			case TypeDiagnosticMessage:
				if len(p.Payload) < 5 {
					continue
				}
				src := binary.BigEndian.Uint16(p.Payload)
				m := diag.Message{Src: uint32(src), Dst: uint32(binary.BigEndian.Uint16(p.Payload[2:])),
					Data: append([]byte(nil), p.Payload[4:]...), Time: time.Now()}
				journal.Rx(c.log, "doip", fmt.Sprintf("%04X", src), m.Data, diag.ServiceName(m.Data[0]))
				return m, nil
			case TypeDiagnosticNegAck:
				code := byte(0)
				if len(p.Payload) > 4 {
					code = p.Payload[4]
				}
				return diag.Message{}, fmt.Errorf("doip: message rejected by the gateway (NACK 0x%02X: %s)", code, nackName(code))
			}
		case <-t.C:
			return diag.Message{}, diag.ErrTimeout
		case <-ctx.Done():
			return diag.Message{}, ctx.Err()
		case <-c.done:
			return diag.Message{}, errors.New("doip: connection closed")
		}
	}
}

func nackName(code byte) string {
	switch code {
	case 0x02:
		return "invalid source address"
	case 0x03:
		return "unknown target address"
	case 0x04:
		return "message too large"
	case 0x05:
		return "out of memory"
	case 0x06:
		return "target unreachable"
	case 0x07:
		return "unknown network"
	case 0x08:
		return "transport protocol error"
	}
	return "reserved"
}

// Info implements diag.Channel.
func (c *Conn) Info() diag.ChannelInfo {
	return diag.ChannelInfo{Protocol: "ISO 13400 DoIP (UDS over Ethernet)", ProtoID: "doip", Adapter: c.addr,
		Target: uint32(c.target), Tester: uint32(c.tester), CAN: true}
}

// Close implements diag.Channel.
func (c *Conn) Close() error {
	c.closed = true
	return c.c.Close()
}

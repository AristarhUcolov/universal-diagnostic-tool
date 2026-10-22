package j1939

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// Lamps is the lamp status of a DM1/DM2 message.
type Lamps struct {
	MIL, RedStop, AmberWarning, Protect bool
}

// DTC is one J1939 trouble code.
type DTC struct {
	SPN uint32
	FMI byte
	OC  byte // occurrence count
}

// ParseDM decodes a DM1 or DM2 message.
func ParseDM(data []byte) (Lamps, []DTC) {
	var l Lamps
	if len(data) < 2 {
		return l, nil
	}
	l.MIL = (data[0]>>6)&3 == 1
	l.RedStop = (data[0]>>4)&3 == 1
	l.AmberWarning = (data[0]>>2)&3 == 1
	l.Protect = data[0]&3 == 1
	var out []DTC
	for i := 2; i+3 < len(data); i += 4 {
		d := data[i : i+4]
		spn := uint32(d[0]) | uint32(d[1])<<8 | uint32(d[2]>>5)<<16
		fmi := d[2] & 0x1F
		if spn == 0 && fmi == 0 || spn == 0x7FFFF {
			continue
		}
		out = append(out, DTC{SPN: spn, FMI: fmi, OC: d[3] & 0x7F})
	}
	return l, out
}

// EncodeDM builds DM1/DM2 data (used by the simulator).
func EncodeDM(l Lamps, dtcs []DTC) []byte {
	lamp := func(on bool, shift uint) byte {
		if on {
			return 1 << shift
		}
		return 0
	}
	out := []byte{lamp(l.MIL, 6) | lamp(l.RedStop, 4) | lamp(l.AmberWarning, 2) | lamp(l.Protect, 0), 0xFF}
	if len(dtcs) == 0 {
		return append(out, 0, 0, 0, 0)
	}
	for _, d := range dtcs {
		out = append(out, byte(d.SPN), byte(d.SPN>>8), byte(d.SPN>>16)<<5|d.FMI&0x1F, d.OC&0x7F)
	}
	return out
}

// Handler presents a J1939 network as one ecu.Handler: values come from
// broadcast parameter groups, trouble codes from DM1/DM2 of every
// controller.
type Handler struct {
	link   Link
	mu     sync.Mutex
	latest map[uint32]Message // PGN -> newest message (any source)
	seen   map[byte]time.Time
	reqMu  sync.Mutex
	waits  []*waiter
	cancel context.CancelFunc
	done   chan struct{}
	params []ecu.Param
}

type waiter struct {
	pgn  uint32
	ch   chan Message
	from byte
}

// NewHandler starts listening on the link.
func NewHandler(link Link) *Handler {
	ctx, cancel := context.WithCancel(context.Background())
	h := &Handler{link: link, latest: map[uint32]Message{}, seen: map[byte]time.Time{}, cancel: cancel, done: make(chan struct{})}
	for i := range SPNs {
		s := &SPNs[i]
		kind := "num"
		if s.Bits < 8 {
			kind = "bool"
		}
		h.params = append(h.params, ecu.Param{ID: s.ParamID(), Name: s.Name, Unit: s.Unit, Group: s.Group,
			Min: s.Min, Max: s.Max, Kind: kind, Digits: digitsFor(s.Scale), Verified: true})
	}
	go h.listen(ctx)
	return h
}

func digitsFor(scale float64) int {
	switch {
	case scale >= 1:
		return 0
	case scale >= 0.1:
		return 1
	default:
		return 2
	}
}

func (h *Handler) listen(ctx context.Context) {
	defer close(h.done)
	for ctx.Err() == nil {
		m, err := h.link.Recv(ctx, 500*time.Millisecond)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		h.mu.Lock()
		h.latest[m.PGN] = m
		h.seen[m.Src] = m.Time
		h.mu.Unlock()
		h.reqMu.Lock()
		for _, w := range h.waits {
			if w.pgn == m.PGN && (w.from == AddrGlobal || w.from == m.Src) {
				select {
				case w.ch <- m:
				default:
				}
			}
		}
		h.reqMu.Unlock()
	}
}

// collect requests a PGN and gathers answers for wait.
func (h *Handler) collect(ctx context.Context, pgn uint32, dst byte, wait time.Duration) []Message {
	w := &waiter{pgn: pgn, ch: make(chan Message, 64), from: dst}
	h.reqMu.Lock()
	h.waits = append(h.waits, w)
	h.reqMu.Unlock()
	defer func() {
		h.reqMu.Lock()
		for i, x := range h.waits {
			if x == w {
				h.waits = append(h.waits[:i], h.waits[i+1:]...)
				break
			}
		}
		h.reqMu.Unlock()
	}()
	if cl, ok := h.link.(interface {
		Request(context.Context, uint32, byte) error
	}); ok {
		_ = cl.Request(ctx, pgn, dst)
	} else {
		_ = h.link.Send(ctx, Message{PGN: PGNRequest, Priority: 6, Dst: dst, Data: []byte{byte(pgn), byte(pgn >> 8), byte(pgn >> 16)}})
	}
	var out []Message
	deadline := time.After(wait)
	for {
		select {
		case m := <-w.ch:
			out = append(out, m)
		case <-deadline:
			return out
		case <-ctx.Done():
			return out
		}
	}
}

func (h *Handler) Describe() ecu.Description {
	h.mu.Lock()
	var srcs []string
	for sa := range h.seen {
		srcs = append(srcs, fmt.Sprintf("0x%02X", sa))
	}
	h.mu.Unlock()
	sort.Strings(srcs)
	return ecu.Description{ID: "j1939", Name: "SAE J1939 network", Protocol: h.link.Name(),
		Address: strings.Join(srcs, ", "), Status: "generic",
		Capabilities: []string{ecu.CapDTC, ecu.CapClear, ecu.CapLive, ecu.CapRaw}}
}

func (h *Handler) Identify(ctx context.Context) ([]ecu.InfoItem, error) {
	var items []ecu.InfoItem
	add := func(key string, name i18n.Text, v string) {
		if v = strings.TrimSpace(v); v != "" {
			items = append(items, ecu.InfoItem{Key: key, Name: name, Value: v})
		}
	}
	add("protocol", i18n.T("Protocol", "Протокол"), h.link.Name())
	h.mu.Lock()
	var srcs []byte
	for sa := range h.seen {
		srcs = append(srcs, sa)
	}
	h.mu.Unlock()
	sort.Slice(srcs, func(i, j int) bool { return srcs[i] < srcs[j] })
	var names []string
	for _, sa := range srcs {
		names = append(names, fmt.Sprintf("0x%02X %s", sa, SourceName(sa)))
	}
	add("controllers", i18n.T("Controllers seen on the bus", "Блоки на шине"), strings.Join(names, ", "))
	for _, m := range h.collect(ctx, PGNVIN, AddrGlobal, 1500*time.Millisecond) {
		add(fmt.Sprintf("vin.%02X", m.Src), i18n.T("VIN ("+SourceName(m.Src)+")", "VIN ("+SourceName(m.Src)+")"), strings.TrimRight(hexutil.ASCII(m.Data), "*"))
	}
	for _, m := range h.collect(ctx, PGNComponentID, AddrGlobal, 1500*time.Millisecond) {
		add(fmt.Sprintf("component.%02X", m.Src), i18n.T("Component ("+SourceName(m.Src)+")", "Компонент ("+SourceName(m.Src)+")"), strings.ReplaceAll(strings.TrimRight(hexutil.ASCII(m.Data), "*"), "*", " / "))
	}
	for _, m := range h.collect(ctx, PGNSoftwareID, AddrGlobal, 1500*time.Millisecond) {
		if len(m.Data) > 1 {
			add(fmt.Sprintf("software.%02X", m.Src), i18n.T("Software ("+SourceName(m.Src)+")", "ПО ("+SourceName(m.Src)+")"), strings.ReplaceAll(strings.TrimRight(hexutil.ASCII(m.Data[1:]), "*"), "*", " / "))
		}
	}
	return items, nil
}

func (h *Handler) ReadDTC(ctx context.Context) ([]dtc.DTC, error) {
	var out []dtc.DTC
	add := func(msgs []Message, active bool) {
		// A controller may answer the request and broadcast its periodic
		// DM1 in the same window: keep only its newest message.
		newest := map[byte]Message{}
		var order []byte
		for _, m := range msgs {
			if _, ok := newest[m.Src]; !ok {
				order = append(order, m.Src)
			}
			newest[m.Src] = m
		}
		for _, src := range order {
			m := newest[src]
			lamps, codes := ParseDM(m.Data)
			for _, c := range codes {
				d := dtc.DTC{Code: fmt.Sprintf("SPN %d FMI %d", c.SPN, c.FMI), Raw: hexutil.Format(m.Data),
					Active: active, Confirmed: true, MIL: lamps.MIL || lamps.RedStop || lamps.AmberWarning,
					Source: fmt.Sprintf("0x%02X %s", m.Src, SourceName(m.Src)), Protocol: "J1939",
					Desc: SPNName(c.SPN), Detail: FMIName(c.FMI), Known: true,
					Extra: map[string]string{"spn": fmt.Sprint(c.SPN), "fmi": fmt.Sprint(c.FMI), "oc": fmt.Sprint(c.OC)}}
				if active {
					d.Flags = []string{dtc.FlagActive}
				} else {
					d.Flags = []string{dtc.FlagHistory}
				}
				if lamps.MIL {
					d.Flags = append(d.Flags, dtc.FlagMIL)
				}
				out = append(out, d)
			}
		}
	}
	// DM1 is broadcast once per second; requesting it gets an answer
	// from controllers without active faults too.
	add(h.collect(ctx, PGNDM1, AddrGlobal, 1500*time.Millisecond), true)
	add(h.collect(ctx, PGNDM2, AddrGlobal, 1500*time.Millisecond), false)
	return out, nil
}

func (h *Handler) ClearDTC(ctx context.Context) error {
	h.collect(ctx, PGNDM11, AddrGlobal, 500*time.Millisecond)
	h.collect(ctx, PGNDM3, AddrGlobal, 500*time.Millisecond)
	return nil
}

func (h *Handler) Params() []ecu.Param { return h.params }

func (h *Handler) Read(ctx context.Context, ids []string) ([]ecu.Value, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]ecu.Value, 0, len(ids))
	for _, id := range ids {
		v := ecu.Value{ID: id}
		var n uint32
		if _, err := fmt.Sscanf(id, "j1939.spn%d", &n); err == nil {
			if s := SPNByNumber(n); s != nil {
				if m, ok := h.latest[s.PGN]; ok && time.Since(m.Time) < 5*time.Second {
					v.V, v.OK = s.Decode(m.Data)
				}
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func (h *Handler) Actuators() []ecu.Actuator { return nil }

func (h *Handler) Actuate(ctx context.Context, id, action string, value float64) (string, error) {
	return "", fmt.Errorf("J1939 output tests are not supported")
}

// Raw requests a PGN: the request is three bytes (PGN, most significant
// first), optionally followed by a destination address.
func (h *Handler) Raw(ctx context.Context, req []byte) ([]byte, error) {
	if len(req) < 3 {
		return nil, fmt.Errorf("enter a PGN as three bytes, e.g. 00 FE CA for DM1")
	}
	pgn := uint32(req[0])<<16 | uint32(req[1])<<8 | uint32(req[2])
	dst := byte(AddrGlobal)
	if len(req) > 3 {
		dst = req[3]
	}
	msgs := h.collect(ctx, pgn, dst, 1500*time.Millisecond)
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no answer for PGN %d", pgn)
	}
	return msgs[0].Data, nil
}

func (h *Handler) KeepAlive(ctx context.Context) error { return nil }

// Latest returns the newest broadcast messages for the bus monitor.
func (h *Handler) Latest() []Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Message, 0, len(h.latest))
	for _, m := range h.latest {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PGN < out[j].PGN })
	return out
}

func (h *Handler) Close() error {
	h.cancel()
	<-h.done
	return h.link.Close()
}

// Package can defines CAN frames and the Bus interface implemented by
// every CAN adapter (SocketCAN, slcan/Lawicel, PCAN, Kvaser, J2534 raw
// CAN, the simulator). A Mux fans incoming frames out to several users,
// so ISO-TP sessions, J1939 and the bus monitor can share one adapter.
package can

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
)

// Frame is one CAN or CAN FD frame.
type Frame struct {
	ID   uint32    `json:"id"`
	Ext  bool      `json:"ext"` // 29-bit identifier
	RTR  bool      `json:"rtr,omitempty"`
	FD   bool      `json:"fd,omitempty"`
	BRS  bool      `json:"brs,omitempty"`
	Len  uint8     `json:"len"`
	Data [64]byte  `json:"-"`
	Time time.Time `json:"time"`
	// Echo marks a frame we transmitted ourselves (adapters that report
	// transmitted frames).
	Echo bool `json:"echo,omitempty"`
}

// NewFrame builds a frame with the given payload.
func NewFrame(id uint32, ext bool, data []byte) Frame {
	f := Frame{ID: id, Ext: ext || id > 0x7FF}
	n := copy(f.Data[:], data)
	f.Len = uint8(n)
	if n > 8 {
		f.FD = true
	}
	return f
}

// Payload returns the data bytes.
func (f *Frame) Payload() []byte { return f.Data[:f.Len] }

// String renders the frame like candump: "7E8#0641000000".
func (f Frame) String() string {
	id := fmt.Sprintf("%03X", f.ID)
	if f.Ext {
		id = fmt.Sprintf("%08X", f.ID)
	}
	if f.RTR {
		return id + "#R"
	}
	sep := "#"
	if f.FD {
		sep = "##"
	}
	return id + sep + hexutil.Compact(f.Payload())
}

// IDText formats the identifier.
func (f Frame) IDText() string {
	if f.Ext {
		return fmt.Sprintf("%08X", f.ID)
	}
	return fmt.Sprintf("%03X", f.ID)
}

// DLCLen converts a CAN FD DLC code to a byte count.
func DLCLen(dlc uint8) int {
	table := [...]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 20, 24, 32, 48, 64}
	if int(dlc) < len(table) {
		return table[dlc]
	}
	return 64
}

// LenDLC converts a byte count to the smallest CAN FD DLC code.
func LenDLC(n int) uint8 {
	switch {
	case n <= 8:
		return uint8(n)
	case n <= 12:
		return 9
	case n <= 16:
		return 10
	case n <= 20:
		return 11
	case n <= 24:
		return 12
	case n <= 32:
		return 13
	case n <= 48:
		return 14
	}
	return 15
}

// Bus is a CAN adapter channel.
type Bus interface {
	// Send transmits a frame.
	Send(ctx context.Context, f Frame) error
	// Recv waits up to timeout for the next frame. Only one goroutine
	// may call Recv; use a Mux to share the bus.
	Recv(ctx context.Context, timeout time.Duration) (Frame, error)
	Info() BusInfo
	Close() error
}

// BusInfo describes an open bus.
type BusInfo struct {
	Name    string `json:"name"`
	Bitrate int    `json:"bitrate"`
	FD      bool   `json:"fd"`
}

// Errors.
var (
	ErrTimeout = errors.New("can: no frame (timeout)")
	ErrClosed  = errors.New("can: bus closed")
)

// ParseFrame parses candump-style text "123#DEADBEEF" or "18DAF110#0201".
func ParseFrame(s string) (Frame, error) {
	s = strings.TrimSpace(s)
	i := strings.Index(s, "#")
	if i <= 0 {
		return Frame{}, fmt.Errorf("can: frame %q must look like 7E0#0201", s)
	}
	var id uint32
	if _, err := fmt.Sscanf(s[:i], "%X", &id); err != nil {
		return Frame{}, fmt.Errorf("can: bad id in %q", s)
	}
	ext := len(s[:i]) > 3
	rest := s[i+1:]
	fd := strings.HasPrefix(rest, "#")
	rest = strings.TrimPrefix(rest, "#")
	if rest == "R" {
		f := Frame{ID: id, Ext: ext, RTR: true}
		return f, nil
	}
	data, err := hexutil.Parse(rest)
	if err != nil {
		return Frame{}, err
	}
	f := NewFrame(id, ext, data)
	f.FD = fd || len(data) > 8
	return f, nil
}

// Mux reads frames from a Bus in the background and delivers them to
// subscribers whose filter accepts them.
type Mux struct {
	bus  Bus
	log  journal.Logger
	mu   sync.Mutex
	subs map[int]*Sub
	next int
	done chan struct{}
	err  error
	smu  sync.Mutex // serialises Send
	// LogFrames controls whether every frame goes to the journal
	// (useful for small amounts of diagnostic traffic, too much for a
	// busy powertrain bus).
	LogFrames bool
}

// Sub is one subscription.
type Sub struct {
	m      *Mux
	id     int
	filter func(*Frame) bool
	ch     chan Frame
}

// NewMux starts reading from bus.
func NewMux(bus Bus, log journal.Logger) *Mux {
	m := &Mux{bus: bus, log: journal.Or(log), subs: map[int]*Sub{}, done: make(chan struct{})}
	go m.loop()
	return m
}

func (m *Mux) loop() {
	defer close(m.done)
	ctx := context.Background()
	for {
		f, err := m.bus.Recv(ctx, 200*time.Millisecond)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				continue
			}
			m.mu.Lock()
			m.err = err
			for _, s := range m.subs {
				close(s.ch)
			}
			m.subs = map[int]*Sub{}
			m.mu.Unlock()
			return
		}
		if m.LogFrames && !f.Echo {
			journal.Rx(m.log, "can", f.IDText(), f.Payload(), "")
		}
		m.mu.Lock()
		for _, s := range m.subs {
			if s.filter == nil || s.filter(&f) {
				select {
				case s.ch <- f:
				default: // subscriber too slow: drop
				}
			}
		}
		m.mu.Unlock()
	}
}

// Subscribe registers a filter (nil = all frames).
func (m *Mux) Subscribe(filter func(*Frame) bool, buffer int) *Sub {
	if buffer <= 0 {
		buffer = 256
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	s := &Sub{m: m, id: m.next, filter: filter, ch: make(chan Frame, buffer)}
	if m.err != nil {
		close(s.ch)
		return s
	}
	m.subs[s.id] = s
	return s
}

// Recv waits for the next matching frame.
func (s *Sub) Recv(ctx context.Context, timeout time.Duration) (Frame, error) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case f, ok := <-s.ch:
		if !ok {
			return Frame{}, ErrClosed
		}
		return f, nil
	case <-t.C:
		return Frame{}, ErrTimeout
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	}
}

// Drain discards queued frames.
func (s *Sub) Drain() {
	for {
		select {
		case _, ok := <-s.ch:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// Close ends the subscription.
func (s *Sub) Close() {
	s.m.mu.Lock()
	if _, ok := s.m.subs[s.id]; ok {
		delete(s.m.subs, s.id)
		close(s.ch)
	}
	s.m.mu.Unlock()
}

// Send transmits a frame (safe for concurrent use).
func (m *Mux) Send(ctx context.Context, f Frame) error {
	m.smu.Lock()
	defer m.smu.Unlock()
	if m.LogFrames {
		journal.Tx(m.log, "can", f.IDText(), f.Payload(), "")
	}
	return m.bus.Send(ctx, f)
}

// Bus returns the underlying bus.
func (m *Mux) Bus() Bus { return m.bus }

// Close closes the bus and waits for the reader to stop.
func (m *Mux) Close() error {
	err := m.bus.Close()
	<-m.done
	return err
}

package sim

import (
	"context"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/isotp"
)

// CANBus is a virtual CAN bus; every endpoint receives the frames sent
// by all other endpoints.
type CANBus struct {
	mu        sync.Mutex
	endpoints map[int]*CANEndpoint
	next      int
	Bitrate   int
	// FD makes the control units talk ISO-TP over CAN FD frames.
	FD     bool
	done   chan struct{}
	closed bool
}

// NewCANBus creates a bus.
func NewCANBus(bitrate int) *CANBus {
	return &CANBus{endpoints: map[int]*CANEndpoint{}, Bitrate: bitrate, done: make(chan struct{})}
}

// Endpoint attaches a new node.
func (b *CANBus) Endpoint(name string) *CANEndpoint {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	e := &CANEndpoint{bus: b, id: b.next, name: name, rx: make(chan can.Frame, 4096), done: make(chan struct{})}
	b.endpoints[e.id] = e
	return e
}

// Close shuts the bus down.
func (b *CANBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.done)
	}
}

// CANEndpoint is one node's connection; it implements can.Bus.
type CANEndpoint struct {
	bus  *CANBus
	id   int
	name string
	rx   chan can.Frame
	done chan struct{}
	once sync.Once
}

var _ can.Bus = (*CANEndpoint)(nil)

func (e *CANEndpoint) Send(ctx context.Context, f can.Frame) error {
	e.bus.mu.Lock()
	defer e.bus.mu.Unlock()
	if e.bus.closed {
		return can.ErrClosed
	}
	f.Time = time.Now()
	for id, other := range e.bus.endpoints {
		if id == e.id {
			continue
		}
		select {
		case other.rx <- f:
		default:
		}
	}
	return nil
}

func (e *CANEndpoint) Recv(ctx context.Context, timeout time.Duration) (can.Frame, error) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case f := <-e.rx:
		return f, nil
	case <-t.C:
		return can.Frame{}, can.ErrTimeout
	case <-ctx.Done():
		return can.Frame{}, ctx.Err()
	case <-e.bus.done:
		return can.Frame{}, can.ErrClosed
	case <-e.done:
		return can.Frame{}, can.ErrClosed
	}
}

func (e *CANEndpoint) Info() can.BusInfo {
	return can.BusInfo{Name: "simulator CAN (" + e.name + ")", Bitrate: e.bus.Bitrate, FD: e.bus.FD}
}

func (e *CANEndpoint) Close() error {
	e.once.Do(func() {
		e.bus.mu.Lock()
		delete(e.bus.endpoints, e.id)
		e.bus.mu.Unlock()
		close(e.done)
	})
	return nil
}

// AttachISOTP starts a control unit that serves app over ISO-TP. It
// listens on rx and the functional id (0 = none) and answers on tx.
func (b *CANBus) AttachISOTP(name string, tx, rx, functional uint32, ext bool, app App) {
	ep := b.Endpoint(name)
	mux := can.NewMux(ep, nil)
	cfg := isotp.DefaultConfig(tx, rx)
	cfg.Ext, cfg.FD = ext, b.FD
	cfg.RxIDs = []uint32{rx}
	if functional != 0 {
		cfg.RxIDs = append(cfg.RxIDs, functional)
	}
	cfg.FlowTx = func(uint32) uint32 { return tx }
	conn := isotp.New(mux, cfg, nil)
	go func() {
		ctx := context.Background()
		for {
			select {
			case <-b.done:
				return
			default:
			}
			m, err := conn.Recv(ctx, 500*time.Millisecond)
			if err != nil {
				continue
			}
			isFunctional := functional != 0 && m.Src == functional
			for i, ans := range app.Handle(m.Data, TransportCAN, isFunctional) {
				if i == 0 {
					time.Sleep(2 * time.Millisecond)
				} else {
					time.Sleep(30 * time.Millisecond)
				}
				_ = conn.Send(ctx, ans)
			}
		}
	}()
}

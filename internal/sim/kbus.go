package sim

import (
	"errors"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kline"
)

type kEventKind int

const (
	evByte  kEventKind = iota // a byte sent by the tester
	evEnd                     // end of one Send call (burst)
	evWake                    // fast-init wake-up pattern
	ev5Baud                   // 5-baud address
)

type kEvent struct {
	kind kEventKind
	b    byte
	baud int
}

// kNode is a simulated control unit on the K-Line.
type kNode interface {
	input() chan kEvent
	run(bus *KBus)
}

// KBus is a virtual K-Line. Its tester side implements kline.Line, so
// the real protocol code runs on it unchanged.
type KBus struct {
	mu     sync.Mutex
	nodes  []kNode
	rx     chan byte
	baud   int
	closed bool
	done   chan struct{}
	// TimeScale shortens the protocol pauses of simulated units (1 =
	// real time). Tests use small values.
	TimeScale float64
}

var _ kline.Line = (*KBus)(nil)

// NewKBus creates an empty bus.
func NewKBus() *KBus {
	return &KBus{rx: make(chan byte, 8192), baud: 10400, done: make(chan struct{}), TimeScale: 1}
}

// Attach adds a control unit and starts it.
func (b *KBus) Attach(n kNode) {
	b.mu.Lock()
	b.nodes = append(b.nodes, n)
	b.mu.Unlock()
	go n.run(b)
}

func (b *KBus) sleep(d time.Duration) {
	time.Sleep(time.Duration(float64(d) * b.TimeScale))
}

// emit puts bytes from a control unit on the bus. Bytes sent at a speed
// different from the tester's arrive as garbage, like on a real line.
func (b *KBus) emit(p []byte, baud int) {
	b.mu.Lock()
	tb := b.baud
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return
	}
	for _, v := range p {
		if baud != tb {
			v = 0xFF ^ v ^ 0x5A
		}
		select {
		case b.rx <- v:
		default:
		}
	}
}

func (b *KBus) broadcast(ev kEvent) {
	b.mu.Lock()
	nodes := append([]kNode(nil), b.nodes...)
	b.mu.Unlock()
	for _, n := range nodes {
		select {
		case n.input() <- ev:
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (b *KBus) Name() string { return "simulator K-Line" }

func (b *KBus) Baud() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.baud
}

func (b *KBus) SetBaud(baud int) error {
	b.mu.Lock()
	b.baud = baud
	b.mu.Unlock()
	return nil
}

func (b *KBus) Send(p []byte, gap time.Duration) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errors.New("simulator: K-Line closed")
	}
	baud := b.baud
	b.mu.Unlock()
	for _, v := range p {
		b.broadcast(kEvent{kind: evByte, b: v, baud: baud})
	}
	b.broadcast(kEvent{kind: evEnd, baud: baud})
	return nil
}

func (b *KBus) RecvByte(timeout time.Duration) (byte, error) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case v := <-b.rx:
		return v, nil
	case <-t.C:
		return 0, kline.ErrTimeout
	case <-b.done:
		return 0, errors.New("simulator: K-Line closed")
	}
}

func (b *KBus) Drain() {
	for {
		select {
		case <-b.rx:
		default:
			return
		}
	}
}

func (b *KBus) WakeUp(low, high time.Duration) error {
	b.Drain()
	b.broadcast(kEvent{kind: evWake})
	b.sleep(low + high)
	return nil
}

func (b *KBus) FiveBaud(addr byte) error {
	b.Drain()
	b.broadcast(kEvent{kind: ev5Baud, b: addr})
	// The address takes 2 s at 5 baud; units answer after it.
	b.sleep(2 * time.Second)
	return nil
}

func (b *KBus) SetLLine(bool) error { return nil }

// Close stops all simulated units.
func (b *KBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.done)
	}
	return nil
}

// Done is closed when the bus is closed.
func (b *KBus) Done() <-chan struct{} { return b.done }

// next waits for the next event for a node, giving up after timeout
// (timeout 0 waits forever). ok is false on timeout or bus close.
func next(b *KBus, in chan kEvent, timeout time.Duration) (kEvent, bool) {
	if timeout <= 0 {
		select {
		case ev := <-in:
			return ev, true
		case <-b.done:
			return kEvent{}, false
		}
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case ev := <-in:
		return ev, true
	case <-t.C:
		return kEvent{}, false
	case <-b.done:
		return kEvent{}, false
	}
}

// Package kline drives the ISO 9141 K-Line: a single half-duplex wire on
// which the tester and every control unit share one bus. Adapters of the
// "KKL" family (VAG KKL 409.1 on FTDI or CH340, K+DCAN in K mode, RS232
// dumb cables) are plain serial ports whose TxD and RxD are joined to the
// K-Line through a transceiver, so every byte we send is echoed back and
// the initialisation patterns have to be produced by bit-banging the
// break condition.
package kline

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/serial"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/timing"
)

// Line is a K-Line bus as seen from the tester.
type Line interface {
	Name() string
	Baud() int
	SetBaud(baud int) error
	// Send transmits bytes with gap between them (0 = back to back) and
	// consumes the echo the bus produces.
	Send(p []byte, gap time.Duration) error
	// ReadByte waits up to timeout for the next byte from the bus.
	RecvByte(timeout time.Duration) (byte, error)
	// Drain discards everything received so far.
	Drain()
	// WakeUp produces the ISO 14230 fast-init pattern: the line is held
	// low for low and then high for high.
	WakeUp(low, high time.Duration) error
	// FiveBaud transmits one byte at 5 baud (ISO 9141 slow init, KW1281).
	FiveBaud(b byte) error
	// SetLLine drives the L-Line where the adapter has one (RTS on most
	// K+L cables). Most control units do not need it.
	SetLLine(high bool) error
	Close() error
}

// Errors.
var (
	ErrTimeout = errors.New("k-line: no data (timeout)")
	ErrEcho    = errors.New("k-line: echo mismatch (bus collision or wrong adapter mode)")
	// ErrNoEcho: an echoing adapter heard nothing of its own bytes. The
	// cable's line driver has lost its +12 V (OBD pin 16, often switched
	// with the ignition) or the cable is unplugged.
	ErrNoEcho = errors.New("k-line: the adapter does not hear its own bytes (no +12 V on the cable: ignition off, or the cable unplugged)")
)

// EchoMode tells whether the adapter echoes transmitted bytes.
type EchoMode int

const (
	EchoAuto EchoMode = iota // detect on the first transmission
	EchoOn                   // classic KKL cables
	EchoOff                  // adapters that suppress the echo
)

// InitMethod selects how the fast-init low pulse is produced.
type InitMethod int

const (
	InitBreak   InitMethod = iota // UART break condition (works on FTDI, CH340, CP210x, PL2303)
	InitBaud360                   // send 0x00 at 360 baud: 9 bit times = 25 ms low
)

// Options configure a serial K-Line adapter.
type Options struct {
	Baud int      // initial speed, default 10400
	Echo EchoMode // default auto
	// DTR and RTS levels. Dumb RS232 KKL cables take their supply from
	// DTR; on many USB K+L cables RTS drives the L-Line. The defaults
	// (DTR on, RTS off) suit VAG KKL 409.1 cables.
	DTR, RTS bool
	Init     InitMethod
	// Latency is extra slack added to every byte timeout to absorb USB
	// buffering (FTDI's default latency timer is 16 ms). Default 40 ms.
	Latency time.Duration
}

// DefaultOptions returns settings that suit KKL cables.
func DefaultOptions() Options {
	return Options{Baud: 10400, Echo: EchoAuto, DTR: true, RTS: false, Init: InitBreak, Latency: 40 * time.Millisecond}
}

// SerialLine is a K-Line adapter on a serial port.
type SerialLine struct {
	port    serial.Port
	opt     Options
	baud    int
	echo    EchoMode
	buf     []byte
	tmp     [256]byte
	log     journal.Logger
	release func()
}

// OpenSerial opens a KKL-style adapter on the named serial port.
func OpenSerial(name string, opt Options, log journal.Logger) (*SerialLine, error) {
	if opt.Baud == 0 {
		opt.Baud = 10400
	}
	if opt.Latency == 0 {
		opt.Latency = 40 * time.Millisecond
	}
	port, err := serial.Open(name, serial.Config{Baud: opt.Baud, DTR: opt.DTR, RTS: opt.RTS})
	if err != nil {
		return nil, err
	}
	l := &SerialLine{port: port, opt: opt, baud: opt.Baud, echo: opt.Echo, log: journal.Or(log)}
	l.release = timing.HighResolution()
	journal.Infof(l.log, "kline", "opened %s at %d baud (DTR=%v RTS=%v)", name, opt.Baud, opt.DTR, opt.RTS)
	return l, nil
}

// FromPort wraps an already open port (used by tests and by adapters
// that need custom port settings).
func FromPort(port serial.Port, opt Options, log journal.Logger) *SerialLine {
	if opt.Baud == 0 {
		opt.Baud = 10400
	}
	if opt.Latency == 0 {
		opt.Latency = 40 * time.Millisecond
	}
	return &SerialLine{port: port, opt: opt, baud: opt.Baud, echo: opt.Echo, log: journal.Or(log), release: func() {}}
}

func (l *SerialLine) Name() string { return l.port.Name() }
func (l *SerialLine) Baud() int    { return l.baud }

// EchoDetected reports the echo mode after auto-detection.
func (l *SerialLine) EchoDetected() EchoMode { return l.echo }

func (l *SerialLine) SetBaud(baud int) error {
	if baud == l.baud {
		return nil
	}
	if err := l.port.SetBaud(baud); err != nil {
		return err
	}
	l.baud = baud
	return nil
}

// byteTime is how long one 10-bit character takes on the wire.
func (l *SerialLine) byteTime() time.Duration {
	return time.Duration(float64(10*time.Second) / float64(l.baud))
}

func (l *SerialLine) Send(p []byte, gap time.Duration) error {
	if len(p) == 0 {
		return nil
	}
	if gap <= 0 {
		if _, err := l.port.Write(p); err != nil {
			return err
		}
	} else {
		for i, b := range p {
			if i > 0 {
				timing.Sleep(gap)
			}
			if _, err := l.port.Write([]byte{b}); err != nil {
				return err
			}
		}
	}
	return l.consumeEcho(p, gap)
}

func (l *SerialLine) consumeEcho(p []byte, gap time.Duration) error {
	if l.echo == EchoOff {
		return nil
	}
	window := (l.byteTime()+gap)*time.Duration(len(p)) + l.opt.Latency + 20*time.Millisecond
	deadline := time.Now().Add(window)
	got := make([]byte, 0, len(p))
	for len(got) < len(p) {
		b, err := l.readRaw(time.Until(deadline))
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				break
			}
			return err
		}
		// A zero byte may precede the echo: it is the break condition of
		// the init pulse arriving late through the USB buffer.
		if len(got) == 0 && b == 0x00 && p[0] != 0x00 {
			continue
		}
		got = append(got, b)
	}
	if bytes.Equal(got, p) {
		if l.echo == EchoAuto {
			l.echo = EchoOn
			journal.Infof(l.log, "kline", "adapter echoes transmitted bytes (normal for KKL cables)")
		}
		return nil
	}
	if l.echo == EchoAuto {
		// What came back is not our echo: the adapter suppresses it and
		// these are already answer bytes.
		l.echo = EchoOff
		l.buf = append(got, l.buf...)
		if len(got) == 0 {
			journal.Infof(l.log, "kline", "adapter does not echo transmitted bytes (a KKL cable always does: if this is one, it has no +12 V from the car)")
		} else {
			journal.Infof(l.log, "kline", "adapter does not echo transmitted bytes")
		}
		return nil
	}
	if len(got) == 0 {
		journal.Errorf(l.log, "kline", "no echo of % X: the cable has no +12 V from the car (ignition off?) or is unplugged", p)
		return fmt.Errorf("%w: sent % X", ErrNoEcho, p)
	}
	journal.Errorf(l.log, "kline", "echo mismatch: sent % X, read back % X", p, got)
	return fmt.Errorf("%w: sent % X, read back % X", ErrEcho, p, got)
}

func (l *SerialLine) readRaw(timeout time.Duration) (byte, error) {
	if len(l.buf) > 0 {
		b := l.buf[0]
		l.buf = l.buf[1:]
		return b, nil
	}
	if timeout < time.Millisecond {
		timeout = time.Millisecond
	}
	// Round to whole milliseconds so the driver timeout is not
	// reprogrammed for every call.
	timeout = timeout.Round(time.Millisecond)
	if err := l.port.SetReadTimeout(timeout); err != nil {
		return 0, err
	}
	n, err := l.port.Read(l.tmp[:])
	if err != nil {
		if errors.Is(err, serial.ErrTimeout) {
			return 0, ErrTimeout
		}
		return 0, err
	}
	l.buf = append(l.buf, l.tmp[:n]...)
	b := l.buf[0]
	l.buf = l.buf[1:]
	return b, nil
}

func (l *SerialLine) RecvByte(timeout time.Duration) (byte, error) {
	return l.readRaw(timeout + l.opt.Latency)
}

func (l *SerialLine) Drain() {
	l.buf = l.buf[:0]
	_ = l.port.Flush()
}

func (l *SerialLine) WakeUp(low, high time.Duration) error {
	l.Drain()
	switch l.opt.Init {
	case InitBaud360:
		old := l.baud
		if err := l.port.SetBaud(360); err != nil {
			return fmt.Errorf("k-line: adapter cannot do 360 baud, use the break method: %w", err)
		}
		start := time.Now()
		if _, err := l.port.Write([]byte{0x00}); err != nil {
			return err
		}
		// Start bit plus eight zero bits keep the line low for 25 ms; the
		// stop bit releases it.
		timing.Until(start.Add(low + 3*time.Millisecond))
		if err := l.port.SetBaud(old); err != nil {
			return err
		}
		timing.Until(start.Add(low + high))
	default:
		if err := l.port.SetBreak(true); err != nil {
			return fmt.Errorf("k-line: adapter does not support the break signal: %w", err)
		}
		timing.Sleep(low)
		if err := l.port.SetBreak(false); err != nil {
			return err
		}
		timing.Sleep(high)
	}
	l.Drain()
	journal.Infof(l.log, "kline", "fast init pulse: %v low, %v high", low, high)
	return nil
}

func (l *SerialLine) FiveBaud(b byte) error {
	l.Drain()
	const bit = 200 * time.Millisecond
	levels := make([]bool, 0, 9)
	levels = append(levels, false) // start bit
	for i := 0; i < 8; i++ {
		levels = append(levels, b&(1<<i) != 0)
	}
	start := time.Now()
	for i, high := range levels {
		if err := l.port.SetBreak(!high); err != nil {
			return fmt.Errorf("k-line: adapter does not support the break signal: %w", err)
		}
		timing.Until(start.Add(time.Duration(i+1) * bit))
	}
	if err := l.port.SetBreak(false); err != nil { // stop bit
		return err
	}
	// The bit-banged bits produce garbage characters on our own receiver.
	// Drop them in the middle of the stop bit, before the control unit
	// starts answering (W1 >= 20 ms after the stop bit).
	timing.Until(start.Add(9*bit + bit/2))
	l.Drain()
	timing.Until(start.Add(10 * bit))
	journal.Infof(l.log, "kline", "5-baud address 0x%02X sent", b)
	return nil
}

func (l *SerialLine) SetLLine(high bool) error {
	// On K+L cables RTS asserted pulls the L-Line low through a
	// transistor, so "high" means RTS off.
	return l.port.SetRTS(!high)
}

func (l *SerialLine) Close() error {
	l.release()
	return l.port.Close()
}

// OddParity sets bit 7 of a 7-bit address so that the byte has an odd
// number of ones, as KW1281 control units expect at 5 baud.
func OddParity(addr byte) byte {
	addr &= 0x7F
	ones := 0
	for v := addr; v != 0; v >>= 1 {
		ones += int(v & 1)
	}
	if ones%2 == 0 {
		addr |= 0x80
	}
	return addr
}

package kline

import (
	"errors"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/serial"
)

// cablePort is a KKL cable: every byte written comes back as the echo
// while the cable has its +12 V from the car.
type cablePort struct {
	powered bool
	in      []byte
}

func (p *cablePort) Read(b []byte) (int, error) {
	if len(p.in) == 0 {
		return 0, serial.ErrTimeout
	}
	n := copy(b, p.in)
	p.in = p.in[n:]
	return n, nil
}

func (p *cablePort) Write(b []byte) (int, error) {
	if p.powered {
		p.in = append(p.in, b...)
	}
	return len(b), nil
}

func (p *cablePort) Close() error                       { return nil }
func (p *cablePort) SetReadTimeout(time.Duration) error { return nil }
func (p *cablePort) SetBaud(int) error                  { return nil }
func (p *cablePort) SetBreak(bool) error                { return nil }
func (p *cablePort) SetDTR(bool) error                  { return nil }
func (p *cablePort) SetRTS(bool) error                  { return nil }
func (p *cablePort) Flush() error                       { p.in = nil; return nil }
func (p *cablePort) Name() string                       { return "test" }

func TestEcho(t *testing.T) {
	port := &cablePort{powered: true}
	l := &SerialLine{port: port, baud: 9600, echo: EchoAuto, log: journal.Discard}
	if err := l.Send([]byte{0x02, 0xFE, 0x0D}, 0); err != nil || l.echo != EchoOn {
		t.Fatalf("powered cable: %v, echo mode %v", err, l.echo)
	}

	// The supply goes away in the middle of a session (seen on a real
	// GAZ-3110 while clearing codes): nothing comes back at all.
	port.powered = false
	if err := l.Send([]byte{0x62, 0x0E, 0x08, 0x88, 0x0D}, 0); !errors.Is(err, ErrNoEcho) {
		t.Fatalf("cable without power: %v", err)
	}

	// Another byte on the bus is a collision, not a lost supply.
	port.powered = true
	port.in = []byte{0x55}
	if err := l.Send([]byte{0x01, 0xFF, 0x0D}, 0); !errors.Is(err, ErrEcho) {
		t.Fatalf("collision: %v", err)
	}
}

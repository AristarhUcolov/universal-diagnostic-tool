package slcan

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/serial"
)

// fakePort answers commands the way a given firmware does.
type fakePort struct {
	mu     sync.Mutex
	cmds   []string
	out    []byte
	answer func(cmd string) string // "" for no answer at all
}

func (p *fakePort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range strings.Split(string(b), "\r") {
		if c == "" {
			continue
		}
		p.cmds = append(p.cmds, c)
		p.out = append(p.out, p.answer(c)...)
	}
	return len(b), nil
}

func (p *fakePort) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.out) == 0 {
		time.Sleep(time.Millisecond)
		return 0, serial.ErrTimeout
	}
	n := copy(b, p.out)
	p.out = p.out[n:]
	return n, nil
}

func (p *fakePort) SetReadTimeout(time.Duration) error { return nil }
func (p *fakePort) Close() error                       { return nil }
func (p *fakePort) Name() string                       { return "test" }

func (p *fakePort) commands() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.cmds, " ")
}

func TestOpenSilentFirmware(t *testing.T) {
	// CANable 2.0 answers the version request but acknowledges nothing.
	p := &fakePort{answer: func(c string) string {
		if c == "V" {
			return "FW2.0\r"
		}
		return ""
	}}
	b, err := OpenFD(p, 500000, 2000000, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if got := p.commands(); !strings.HasSuffix(got, "V S6 Y2 O") {
		t.Errorf("commands %q, want the data rate set before opening", got)
	}
	if !b.Info().FD || b.Version != "FW2.0" {
		t.Errorf("info %+v, version %q", b.Info(), b.Version)
	}
}

func TestOpenRejected(t *testing.T) {
	// A Lawicel-style adapter rejects an unknown setting with BEL.
	p := &fakePort{answer: func(c string) string {
		switch {
		case c == "V":
			return "V1013\r"
		case strings.HasPrefix(c, "Y"):
			return "\a"
		}
		return "\r"
	}}
	if _, err := OpenFD(p, 500000, 5000000, false, nil); err == nil || !strings.Contains(err.Error(), "data bit rate") {
		t.Errorf("rejected data rate accepted: %v", err)
	}
	if _, err := OpenFD(p, 500000, 3000000, false, nil); err == nil {
		t.Error("3 Mbit/s accepted")
	}
}

func TestOpenNothingThere(t *testing.T) {
	// Nothing answers at all: the wrong port, not an adapter.
	p := &fakePort{answer: func(string) string { return "" }}
	if _, err := Open(p, 500000, false, nil); err == nil {
		t.Error("a silent port was accepted")
	}
}

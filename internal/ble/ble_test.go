package ble

import (
	"errors"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/serial"
)

func TestPort(t *testing.T) {
	var sent []int
	p := newPort("test", func(b []byte) error { sent = append(sent, len(b)); return nil }, nil)
	_ = p.SetReadTimeout(50 * time.Millisecond)

	// Commands go out in pieces that fit the smallest BLE packet.
	if n, err := p.Write(make([]byte, 45)); n != 45 || err != nil {
		t.Fatalf("write: %d %v", n, err)
	}
	if len(sent) != 3 || sent[0] != 20 || sent[1] != 20 || sent[2] != 5 {
		t.Errorf("pieces %v", sent)
	}

	buf := make([]byte, 64)
	if _, err := p.Read(buf); !errors.Is(err, serial.ErrTimeout) {
		t.Errorf("read without data: %v", err)
	}
	// An answer notified while the driver waits.
	go func() {
		time.Sleep(10 * time.Millisecond)
		p.push([]byte("ELM327 v1.5\r\r>"))
	}()
	_ = p.SetReadTimeout(time.Second)
	n, err := p.Read(buf)
	if err != nil || string(buf[:n]) != "ELM327 v1.5\r\r>" {
		t.Errorf("read %q %v", buf[:n], err)
	}
	// Closing wakes a waiting reader.
	go func() {
		time.Sleep(10 * time.Millisecond)
		p.Close()
	}()
	if _, err := p.Read(buf); !errors.Is(err, ErrClosed) {
		t.Errorf("read after close: %v", err)
	}
}

func TestLikely(t *testing.T) {
	for name, want := range map[string]bool{
		"OBDII": true, "IOS-Vlink": true, "Vgate iCar Pro": true, "VEEPEAK": true, "OBDLink CX": true,
		"Mi Band 6": false, "": false, "JBL Flip 5": false,
	} {
		if likely(name) != want {
			t.Errorf("likely(%q) = %v", name, !want)
		}
	}
}

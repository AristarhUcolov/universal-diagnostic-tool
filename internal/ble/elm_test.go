package ble

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/elm327"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/obd"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/sim"
)

// TestELM327OverBLE runs the ELM327 driver over the BLE byte stream: the
// commands go to the program's ELM327 emulator in front of a simulated
// car, the answers come back as notifications of at most 20 bytes, as a
// BLE 4.0 adapter sends them.
func TestELM327OverBLE(t *testing.T) {
	w, err := sim.NewWorld("can")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	adapter := sim.ELMPort(w.CAN, w.Engine)
	p := newPort("ble://test", func(b []byte) error {
		_, err := adapter.Write(b)
		return err
	}, adapter.Close)
	go func() {
		buf := make([]byte, 256)
		for {
			_ = adapter.SetReadTimeout(100 * time.Millisecond)
			n, err := adapter.Read(buf)
			for off := 0; off < n; off += 20 {
				end := min(off+20, n)
				p.push(append([]byte(nil), buf[off:end]...))
			}
			if err != nil && !errors.Is(err, elm327.ErrTimeout) {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dev, err := elm327.Open(ctx, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()
	if dev.Version != "ELM327 v1.5" {
		t.Errorf("version %q", dev.Version)
	}
	ch, err := dev.Open(ctx, elm327.Spec{Proto: '0'})
	if err != nil {
		t.Fatal(err)
	}
	h, err := obd.NewHandler(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	vals, err := h.Read(ctx, []string{"obd.0C", "obd.05"})
	if err != nil || !vals[0].OK || vals[0].V < 500 || !vals[1].OK {
		t.Fatalf("values %+v %v", vals, err)
	}
}

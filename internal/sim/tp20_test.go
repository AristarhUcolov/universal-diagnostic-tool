package sim

import (
	"context"
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/tp20"
)

func TestTP20(t *testing.T) {
	bus := NewCANBus(500000)
	defer bus.Close()
	e := NewEngine()
	bus.AttachTP20(0x01, &VAGApp{Engine: e})
	mux := can.NewMux(bus.Endpoint("tester"), nil)
	ctx := context.Background()
	c, err := tp20.Connect(ctx, mux, 0x01, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := diag.Request(ctx, c, []byte{0x10, 0x89}, diag.DefaultTiming()); err != nil {
		t.Fatal(err)
	}
	id, err := diag.Request(ctx, c, []byte{0x1A, 0x9B}, diag.DefaultTiming())
	if err != nil || len(id) < 30 {
		t.Fatalf("ident %q %v", id, err)
	}
	dtcs, err := diag.Request(ctx, c, []byte{0x18, 0x02, 0xFF, 0x00}, diag.DefaultTiming())
	if err != nil || dtcs[1] != 2 {
		t.Fatalf("dtc % X %v", dtcs, err)
	}
	if err := c.KeepAlive(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

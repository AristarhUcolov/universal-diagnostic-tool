package sim

import (
	"context"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/j1939"
)

func TestJ1939(t *testing.T) {
	bus := NewCANBus(250000)
	defer bus.Close()
	e := NewEngine()
	bus.AttachJ1939Engine(e)
	mux := can.NewMux(bus.Endpoint("tester"), nil)
	link := j1939.NewCANLink(mux, j1939.AddrTool1, nil)
	ctx := context.Background()
	if err := link.ClaimAddress(ctx); err != nil {
		t.Fatal(err)
	}
	h := j1939.NewHandler(link)
	defer h.Close()
	time.Sleep(1200 * time.Millisecond)
	vals, err := h.Read(ctx, []string{"j1939.spn190", "j1939.spn110", "j1939.spn168", "j1939.spn70"})
	if err != nil {
		t.Fatal(err)
	}
	if !vals[0].OK || vals[0].V < 300 || vals[0].V > 2000 {
		t.Fatalf("engine speed %+v", vals[0])
	}
	if !vals[1].OK || vals[1].V < e.State().CoolantC-2 {
		t.Fatalf("coolant %+v", vals[1])
	}
	if !vals[3].OK || vals[3].V != 1 {
		t.Fatalf("parking brake %+v", vals[3])
	}
	codes, err := h.ReadDTC(ctx)
	if err != nil || len(codes) != 3 {
		t.Fatalf("dtc %+v %v", codes, err)
	}
	info, _ := h.Identify(ctx)
	vin := false
	for _, it := range info {
		if it.Value == e.VIN {
			vin = true
		}
	}
	if !vin {
		t.Fatalf("VIN not received (BAM): %+v", info)
	}
	if err := h.ClearDTC(ctx); err != nil {
		t.Fatal(err)
	}
	codes, _ = h.ReadDTC(ctx)
	if len(codes) != 0 {
		t.Fatalf("after clearing: %+v", codes)
	}
}

func TestJ1939IDs(t *testing.T) {
	m := j1939.Message{PGN: 61444, Priority: 3, Src: 0x00, Dst: 0xFF}
	if id := m.ID(); id != 0x0CF00400 {
		t.Fatalf("EEC1 id %08X", id)
	}
	pgn, prio, src, dst := j1939.Decode(0x18EA00F9)
	if pgn != j1939.PGNRequest || prio != 6 || src != 0xF9 || dst != 0x00 {
		t.Fatalf("decode %d %d %X %X", pgn, prio, src, dst)
	}
}

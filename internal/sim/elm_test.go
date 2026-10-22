package sim

import (
	"context"
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/elm327"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/obd"
)

func TestELM327EndToEnd(t *testing.T) {
	bus, e := newCANCar(false)
	defer bus.Close()
	ctx := context.Background()
	dev, err := elm327.Open(ctx, ELMPort(bus, e), nil)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Version != "ELM327 v1.5" {
		t.Fatalf("version %q", dev.Version)
	}
	if v, err := dev.Voltage(ctx); err != nil || v < 11 || v > 15 {
		t.Fatalf("voltage %v %v", v, err)
	}
	ch, err := dev.Open(ctx, elm327.Spec{Proto: '0'})
	if err != nil {
		t.Fatal(err)
	}
	if ch.Protocol() != '6' {
		t.Fatalf("protocol %c", ch.Protocol())
	}
	h, err := obd.NewHandler(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	vals, err := h.Read(ctx, []string{"obd.0C", "obd.05", "obd.42"})
	if err != nil || !vals[0].OK || vals[0].V < 500 {
		t.Fatalf("values %+v %v", vals, err)
	}
	codes, err := h.ReadDTC(ctx)
	if err != nil || len(codes) != 2 {
		t.Fatalf("dtc %v %v", codes, err)
	}
	info, _ := h.Identify(ctx)
	found := false
	for _, it := range info {
		if it.Key == "vin" && it.Value == e.VIN {
			found = true
		}
	}
	if !found {
		t.Fatalf("VIN not read through the ELM327: %+v", info)
	}
	// Physical addressing of the engine control unit (UDS through ELM).
	phys, err := dev.Open(ctx, elm327.Spec{Proto: '6', Header: "7E0", RxFilter: "7E8", FlowHeader: "7E0"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := diag.Request(ctx, phys, []byte{0x22, 0xF1, 0x90}, diag.DefaultTiming())
	if err != nil || string(resp[3:]) != e.VIN {
		t.Fatalf("UDS VIN %q %v", resp, err)
	}
}

func TestParseLinesKLine(t *testing.T) {
	msgs, err := elm327.ParseLines([]string{"48 6B 10 41 0C 0B B8 13", "80 F1 10 03 61 01 47 3C"}, '3')
	if err != nil || len(msgs) != 2 {
		t.Fatalf("%v %v", msgs, err)
	}
	if msgs[0].Src != 0x10 || len(msgs[0].Data) != 4 {
		t.Fatalf("first %+v", msgs[0])
	}
	msgs, err = elm327.ParseLines([]string{"80 F1 10 03 61 01 47 3C"}, '5')
	if err != nil || len(msgs) != 1 || string(msgs[0].Data) != string([]byte{0x61, 0x01, 0x47}) {
		t.Fatalf("KWP length byte: %v %v", msgs, err)
	}
}

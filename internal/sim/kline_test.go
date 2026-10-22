package sim

import (
	"context"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/iso9141"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kw1281"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kwp2000"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/legacy"
)

func fastBus() *KBus {
	b := NewKBus()
	b.TimeScale = 0.05
	return b
}

func TestKWP2000VAZ(t *testing.T) {
	for _, v := range []struct {
		variant VAZVariant
		n       int // length of the 21 01 answer including 61 01
	}{{VariantEuro2, 36}, {VariantJ72, 38}, {VariantR83, 34}} {
		bus := fastBus()
		e := NewEngine()
		node := NewKWPNode(0x10, NewVAZApp(e, v.variant))
		node.LenByteFrom = 20
		bus.Attach(node)
		ctx := context.Background()
		link, err := kwp2000.Open(ctx, bus, kwp2000.Config{Target: 0x10, Tester: 0xF1, Init: kwp2000.InitFast, StartComm: true}, nil)
		if err != nil {
			t.Fatalf("variant %d: open: %v", v.variant, err)
		}
		if kb := link.KeyBytes(); kb[0] != 0x6B || kb[1] != 0x8F {
			t.Fatalf("key bytes % X", kb)
		}
		resp, err := diag.Request(ctx, link, []byte{0x21, 0x01}, diag.DefaultTiming())
		if err != nil {
			t.Fatalf("variant %d: 21 01: %v", v.variant, err)
		}
		if len(resp) != v.n {
			t.Fatalf("variant %d: 21 01 answer has %d bytes, want %d", v.variant, len(resp), v.n)
		}
		// Coolant temperature is byte #11 of the answer (index 10): N = E - 40.
		temp := float64(resp[10]) - 40
		if want := e.State().CoolantC; temp < want-3 || temp > want+3 {
			t.Fatalf("coolant %v, engine %v", temp, want)
		}
		dtcs, err := diag.Request(ctx, link, []byte{0x18, 0x00, 0x00, 0x00}, diag.DefaultTiming())
		if err != nil || dtcs[0] != 0x58 || dtcs[1] != 2 {
			t.Fatalf("read DTC: % X %v", dtcs, err)
		}
		id, err := diag.Request(ctx, link, []byte{0x1A, 0x80}, diag.DefaultTiming())
		if err != nil || len(id) != 97 {
			t.Fatalf("ident: %d bytes %v", len(id), err)
		}
		if _, err := diag.Request(ctx, link, []byte{0x21, 0x55}, diag.DefaultTiming()); !diag.IsNRC(err, 0x31) {
			t.Fatalf("expected request-out-of-range, got %v", err)
		}
		link.Close()
		bus.Close()
	}
}

func TestKWP2000OBDFunctional(t *testing.T) {
	bus := fastBus()
	e := NewEngine()
	app := NewVAZApp(e, VariantJ72)
	app.OBD = NewOBDApp(e, "ECM")
	node := NewKWPNode(0x10, app)
	node.Functional = true
	bus.Attach(node)
	ctx := context.Background()
	link, err := kwp2000.Open(ctx, bus, kwp2000.Config{Target: 0x33, Tester: 0xF1, Functional: true, Init: kwp2000.InitFast, StartComm: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := diag.RequestAll(ctx, link, []byte{0x01, 0x0C}, time.Second, 100*time.Millisecond)
	if err != nil || len(msgs) != 1 || msgs[0].Src != 0x10 {
		t.Fatalf("functional 01 0C: %v %v", msgs, err)
	}
	rpm := float64(int(msgs[0].Data[2])<<8|int(msgs[0].Data[3])) / 4
	if rpm < 500 || rpm > 2000 {
		t.Fatalf("rpm %v", rpm)
	}
	bus.Close()
}

func TestLegacyMikas(t *testing.T) {
	bus := fastBus()
	e := NewEngine()
	bus.Attach(NewLegacyNode(e))
	ctx := context.Background()
	l, err := legacy.Open(bus, 9600, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err := l.Version(ctx)
	if err != nil || v != 0x0A {
		t.Fatalf("version %02X %v", v, err)
	}
	// Coolant (0x1A, 1 byte), injection time (0x3F, 2 bytes) and fuel
	// rate (0x40, 2 bytes: the code itself needs escaping).
	raw, err := l.ReadParams(ctx, []byte{0x1A, 0x3F, 0x40}, []int{1, 2, 2})
	if err != nil {
		t.Fatal(err)
	}
	if temp := float64(raw[0]) - 40; temp < e.State().CoolantC-3 || temp > e.State().CoolantC+3 {
		t.Fatalf("coolant %v", temp)
	}
	inj := float64(int(raw[2])<<8|int(raw[1])) / 125
	if inj < 1 || inj > 6 {
		t.Fatalf("injection %v ms", inj)
	}
	codes, err := l.ReadErrors(ctx)
	if err != nil || len(codes) != 2 || codes[0] != 35 {
		t.Fatalf("errors %v %v", codes, err)
	}
	if err := l.WriteParam(ctx, 0x0E, 0x08); err != nil {
		t.Fatal(err)
	}
	if err := l.WriteParam(ctx, 0x0E, 0x00); err != nil {
		t.Fatal(err)
	}
	codes, _ = l.ReadErrors(ctx)
	if len(codes) != 1 { // the active fault comes back
		t.Fatalf("after clearing: %v", codes)
	}
	prog, _, err := l.Passport(ctx)
	if err != nil || prog == "" {
		t.Fatalf("passport %q %v", prog, err)
	}
	bus.Close()
}

func TestISO9141(t *testing.T) {
	bus := fastBus()
	e := NewEngine()
	bus.Attach(NewISO9141Node(0x10, NewOBDApp(e, "ECM")))
	ctx := context.Background()
	link, err := iso9141.Open(ctx, bus, nil)
	if err != nil {
		t.Fatal(err)
	}
	link.P4 = 0
	msgs, err := diag.RequestAll(ctx, link, []byte{0x09, 0x02}, time.Second, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 5 {
		t.Fatalf("VIN messages: %d", len(msgs))
	}
	var vin []byte
	for _, m := range msgs {
		vin = append(vin, m.Data[3:]...)
	}
	if string(vin[3:]) != e.VIN {
		t.Fatalf("VIN %q", vin)
	}
	bus.Close()
}

func TestKW1281(t *testing.T) {
	bus := fastBus()
	e := NewEngine()
	bus.Attach(NewKW1281Node(e))
	ctx := context.Background()
	c, err := kw1281.Connect(ctx, bus, 0x01, 9600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.ID) < 2 {
		t.Fatalf("id %v", c.ID)
	}
	faults, err := c.ReadFaults(ctx)
	if err != nil || len(faults) != 2 {
		t.Fatalf("faults %v %v", faults, err)
	}
	cells, err := c.ReadGroup(ctx, 1)
	if err != nil || len(cells) != 4 || !cells[0].Numeric || cells[0].Unit != "/min" {
		t.Fatalf("group 1: %+v %v", cells, err)
	}
	if _, err := c.ReadGroup(ctx, 99); err != kw1281.ErrNAK {
		t.Fatalf("group 99: %v", err)
	}
	if err := c.KeepAlive(ctx); err != nil {
		t.Fatal(err)
	}
	c.Close()
	bus.Close()
}

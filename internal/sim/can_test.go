package sim

import (
	"context"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/isotp"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/obd"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/uds"
)

func newCANCar(ext bool) (*CANBus, *Engine) {
	bus := NewCANBus(500000)
	e := NewEngine()
	eng := NewOBDApp(e, "ECM-EngineControl")
	eng.UDS = true
	tcm := NewOBDApp(e, "TCM-TransmissionCtl")
	tcm.Minor = true
	if ext {
		bus.AttachISOTP("ecm", 0x18DAF110, 0x18DA10F1, 0x18DB33F1, true, eng)
		bus.AttachISOTP("tcm", 0x18DAF118, 0x18DA18F1, 0x18DB33F1, true, tcm)
	} else {
		bus.AttachISOTP("ecm", 0x7E8, 0x7E0, 0x7DF, false, eng)
		bus.AttachISOTP("tcm", 0x7E9, 0x7E1, 0x7DF, false, tcm)
	}
	return bus, e
}

func TestOBDOverCAN(t *testing.T) {
	for _, ext := range []bool{false, true} {
		bus, e := newCANCar(ext)
		mux := can.NewMux(bus.Endpoint("tester"), nil)
		conn := isotp.New(mux, isotp.OBDFunctional(ext), nil)
		ctx := context.Background()
		h, err := obd.NewHandler(ctx, conn)
		if err != nil {
			t.Fatalf("ext=%v: %v", ext, err)
		}
		if n := len(h.ControlUnits()); n != 2 {
			t.Fatalf("ext=%v: %d control units", ext, n)
		}
		vals, err := h.Read(ctx, []string{"obd.0C", "obd.05", "obd.14.v", "obd.42", "obd.0D", "obd.11", "obd.04"})
		if err != nil {
			t.Fatal(err)
		}
		if len(vals) != 7 || !vals[0].OK || vals[0].V < 500 {
			t.Fatalf("values %+v", vals)
		}
		if c := vals[1].V; c < e.State().CoolantC-2 || c > e.State().CoolantC+2 {
			t.Fatalf("coolant %v", c)
		}
		codes, err := h.ReadDTC(ctx)
		if err != nil || len(codes) != 2 {
			t.Fatalf("dtc %+v %v", codes, err)
		}
		if !codes[0].Known {
			t.Fatalf("description missing for %s", codes[0].Code)
		}
		info, err := h.Identify(ctx)
		if err != nil {
			t.Fatal(err)
		}
		vin := ""
		for _, it := range info {
			if it.Key == "vin" {
				vin = it.Value
			}
		}
		if vin != e.VIN {
			t.Fatalf("VIN %q", vin)
		}
		mil, n, mons, err := h.Readiness(ctx)
		if err != nil || !mil || n != 2 || len(mons) == 0 {
			t.Fatalf("readiness %v %d %v %v", mil, n, mons, err)
		}
		if err := h.ClearDTC(ctx); err != nil {
			t.Fatal(err)
		}
		bus.Close()
	}
}

func TestUDSOverCAN(t *testing.T) {
	bus, e := newCANCar(false)
	defer bus.Close()
	mux := can.NewMux(bus.Endpoint("tester"), nil)
	conn := isotp.New(mux, isotp.DefaultConfig(0x7E0, 0x7E8), nil)
	c := uds.NewClient(conn)
	ctx := context.Background()
	vin, err := c.ReadDID(ctx, 0xF190)
	if err != nil || string(vin) != e.VIN {
		t.Fatalf("VIN %q %v", vin, err)
	}
	codes, _, err := c.ReadDTCs(ctx, 0xFF)
	if err != nil || len(codes) != 2 {
		t.Fatalf("dtc %v %v", codes, err)
	}
	if err := c.WriteDID(ctx, 0x0100, []byte{1, 2, 3, 4}); !diag.IsNRC(err, 0x7F) {
		t.Fatalf("write in default session: %v", err)
	}
	if _, _, err := c.Session(ctx, uds.SessionExtended); err != nil {
		t.Fatal(err)
	}
	key := func(level byte, seed []byte) ([]byte, error) {
		v := uint16(seed[0])<<8 | uint16(seed[1])
		v ^= 0x55AA
		return []byte{byte(v >> 8), byte(v)}, nil
	}
	if err := c.SecurityAccess(ctx, 0x01, key); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteDID(ctx, 0x0100, []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	// Routine that answers "response pending" first.
	start := time.Now()
	if _, err := c.Routine(ctx, 0x01, 0x0203, nil); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("pending handling too slow")
	}
	// A long DID read exercises multi-frame transfer in both directions.
	long, err := diag.Request(ctx, conn, []byte{0x22, 0xF1, 0x90, 0xF1, 0x87, 0xF1, 0x8A, 0xF1, 0x97}, diag.DefaultTiming())
	if err != nil || len(long) < 40 {
		t.Fatalf("multi DID %d %v", len(long), err)
	}
}

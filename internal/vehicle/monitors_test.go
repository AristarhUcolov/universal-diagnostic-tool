package vehicle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/obd"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/sim"
)

// TestMonitorTests reads the on-board monitoring test results (OBD-II
// service $06) of the simulated CAN car: the catalyst and the oxygen
// sensors pass, and an injected misfire shows up on the cylinder
// counters before anything else.
func TestMonitorTests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s, err := Connect(ctx, Config{Adapter: AdapterSim, Profile: ProfileOBD, SimVariant: "can"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e, _ := s.ECU("")
	tr, ok := e.Handler().(ecu.TestReader)
	if !ok {
		t.Fatal("the OBD handler reads no test results")
	}
	read := func() map[[2]byte]ecu.TestResult {
		res, err := tr.TestResults(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := map[[2]byte]ecu.TestResult{}
		for _, r := range res {
			out[[2]byte{r.MID, r.TID}] = r
		}
		return out
	}
	res := read()
	cat, ok := res[[2]byte{0x21, 0x81}]
	if !ok || !cat.Limits || !cat.Pass || cat.Source != "7E8" {
		t.Errorf("catalyst: %+v", cat)
	}
	if o2 := res[[2]byte{0x01, 0x08}]; o2.Unit != "mV" || !o2.Pass {
		t.Errorf("oxygen sensor: %+v", o2)
	}
	if n := res[[2]byte{0xA4, 0x0C}].Value; n != 0 {
		t.Errorf("cylinder 3 misfires %v without a fault", n)
	}

	s.World().Engine.AddFault(sim.Fault{Code: "P0300", Active: true})
	res = read()
	if n := res[[2]byte{0xA4, 0x0C}].Value; n != 118 {
		t.Errorf("cylinder 3 misfires %v, want 118", n)
	}
	if n := res[[2]byte{0xA1, 0x0C}].Value; n != 168 {
		t.Errorf("all cylinders %v, want 168", n)
	}

	// Older protocols carry manufacturer-specific values only.
	k, err := Connect(ctx, Config{Adapter: AdapterSim, Profile: ProfileOBD, SimVariant: "obd-kline"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	ke, _ := k.ECU("")
	if _, err := ke.Handler().(ecu.TestReader).TestResults(ctx); !errors.Is(err, obd.ErrTestsNeedCAN) {
		t.Errorf("K-Line: %v", err)
	}
	if code := ErrorCode(obd.ErrTestsNeedCAN); code != "tests_can" {
		t.Errorf("error code %q", code)
	}
}

package vehicle

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kline"
)

func TestErrorCode(t *testing.T) {
	for err, want := range map[error]string{
		fmt.Errorf("legacy: %w: sent 62 0E 08 88 0D", kline.ErrNoEcho):     "no_echo",
		fmt.Errorf("actuate: %w", &ecu.ConditionError{Need: "engine_off"}): "engine_off",
		kline.ErrEcho: "",
	} {
		if got := ErrorCode(err); got != want {
			t.Errorf("ErrorCode(%v) = %q, want %q", err, got, want)
		}
	}
}

// TestServiceFunctions drives the service page's functions against the
// simulated VAZ-2110 (Bosch M1.5.4N on KWP2000): output tests with their
// engine conditions, the idle speed set point and an ECU restart that
// must bring communication back by itself.
func TestServiceFunctions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s, err := Connect(ctx, Config{Adapter: AdapterSim, Profile: ProfileRuKWP, SimVariant: "vaz-m154"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e, _ := s.ECU("")
	h := e.Handler()

	active := func(id string) bool {
		for _, a := range h.Actuators() {
			if a.ID == id {
				return a.Active
			}
		}
		t.Fatalf("no output test %q", id)
		return false
	}
	rpm := func() float64 {
		vals, err := h.Read(ctx, []string{"rpm"})
		if err != nil || len(vals) != 1 || !vals[0].OK {
			t.Fatalf("engine speed: %v %+v", err, vals)
		}
		return vals[0].V
	}

	// The simulated engine runs, so an injector pulse must be refused.
	_, err = h.Actuate(ctx, "inj1", "pulse", 0)
	var ce *ecu.ConditionError
	if !errors.As(err, &ce) || ce.Need != "engine_off" {
		t.Fatalf("injector test with the engine running: %v", err)
	}

	if _, err := h.Actuate(ctx, "fan", "on", 0); err != nil {
		t.Fatal(err)
	}
	if !active("fan") {
		t.Fatal("fan not reported as held by the tester")
	}

	if _, err := h.Actuate(ctx, "idle_rpm", "set", 1300); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if got := rpm(); math.Abs(got-1300) > 120 {
		t.Fatalf("idle speed %v rpm after asking for 1300", got)
	}
	if _, err := h.Actuate(ctx, "idle_rpm", "off", 0); err != nil {
		t.Fatal(err)
	}
	if active("idle_rpm") {
		t.Fatal("idle speed still held after handing it back")
	}

	// A restart drops the session; the handler must wake the unit up
	// again and forget the outputs the unit no longer holds.
	r, ok := h.(ecu.Resetter)
	if !ok {
		t.Fatal("the handler cannot restart the control unit")
	}
	if err := r.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if active("fan") {
		t.Fatal("fan still reported as held after the restart")
	}
	if got := rpm(); got < 500 {
		t.Fatalf("engine speed %v after the restart", got)
	}
	if _, err := h.ReadDTC(ctx); err != nil {
		t.Fatalf("trouble codes after the restart: %v", err)
	}
}

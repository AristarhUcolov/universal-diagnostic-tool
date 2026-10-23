package vehicle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestCANFD talks UDS to the simulated car from 2021 over CAN FD: the
// identification and trouble codes must arrive, and the bus must carry
// FD frames longer than eight bytes for them.
func TestCANFD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	s, err := Connect(ctx, Config{Adapter: AdapterSim, SimVariant: "can-fd"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Config.CANFD || s.Config.Profile != ProfileUDS {
		t.Fatalf("config %+v", s.Config)
	}
	if e, _ := s.ECU(""); !strings.Contains(e.Handler().Describe().Protocol, "CAN FD") {
		t.Errorf("protocol %q does not say CAN FD", e.Handler().Describe().Protocol)
	}
	sniff := s.World().CAN.Endpoint("sniffer")
	e, _ := s.ECU("")
	info, err := e.Handler().Identify(ctx)
	if err != nil || len(info) == 0 {
		t.Fatalf("identification: %v %v", info, err)
	}
	if _, err := e.Handler().ReadDTC(ctx); err != nil {
		t.Fatal(err)
	}
	long := 0
	for {
		f, err := sniff.Recv(ctx, 100*time.Millisecond)
		if err != nil {
			break
		}
		if f.FD && f.Len > 8 {
			long++
		}
	}
	if long == 0 {
		t.Error("no CAN FD frame longer than 8 bytes on the bus")
	}

	// Adapters without CAN FD refuse instead of failing later.
	if _, err := Connect(ctx, Config{Adapter: AdapterELM, Port: "COM99", CANFD: true}, nil, nil); !errors.Is(err, ErrNoCANFD) {
		t.Errorf("ELM327 with CAN FD: %v", err)
	}
}

package vehicle

import (
	"context"
	"testing"
	"time"
)

func TestConnectSimulatedVehicles(t *testing.T) {
	cases := []struct {
		variant, profile string
		dtcs             int
		param            string // a parameter that must read OK
		definition       string
	}{
		{"gaz-mikas", ProfileRuAuto, 2, "twat", "gaz-mikas71"},
		{"vaz-j72", ProfileRuKWP, 2, "rpm", "vaz-j72"},
		{"vaz-m154", ProfileRuKWP, 2, "uacc", "vaz-m154n-euro2"},
		{"vaz-r83", ProfileRuKWP, 2, "co_corr", "vaz-m154-r83"},
		{"obd-kline", ProfileOBD, 2, "obd.0C", ""},
		{"vag-kline", ProfileVAG, 2, "kw.g001.1", ""},
		{"can", ProfileAuto, 2, "obd.05", ""},
		{"can", ProfileUDS, 2, "", ""},
		{"vag-can", ProfileTP20, 2, "kw.g001.2", ""},
		{"truck", ProfileJ1939, 3, "j1939.spn190", ""},
		{"doip", ProfileUDS, 2, "", ""},
	}
	for _, c := range cases {
		t.Run(c.variant+"/"+c.profile, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			s, err := Connect(ctx, Config{Adapter: AdapterSim, Profile: c.profile, SimVariant: c.variant}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			e, err := s.ECU("")
			if err != nil {
				t.Fatal(err)
			}
			if c.definition != "" && e.Desc.Definition != c.definition {
				t.Fatalf("definition %q, want %q", e.Desc.Definition, c.definition)
			}
			if c.variant == "truck" {
				time.Sleep(1200 * time.Millisecond) // let broadcasts arrive
			}
			codes, err := e.Handler().ReadDTC(ctx)
			if err != nil {
				t.Fatalf("DTC: %v", err)
			}
			if len(codes) != c.dtcs {
				t.Fatalf("got %d codes: %+v", len(codes), codes)
			}
			for _, d := range codes {
				if d.Desc.Empty() {
					t.Errorf("code %s has no description", d.Code)
				}
			}
			if c.param != "" {
				vals, err := e.Handler().Read(ctx, []string{c.param})
				if err != nil || len(vals) != 1 || !vals[0].OK {
					t.Fatalf("read %s: %+v %v", c.param, vals, err)
				}
			}
			if _, err := e.Handler().Identify(ctx); err != nil {
				t.Fatalf("identify: %v", err)
			}
			if v, err := s.Voltage(ctx); err != nil || v < 10 {
				t.Fatalf("voltage %v %v", v, err)
			}
		})
	}
}

func TestLiveAndMonitor(t *testing.T) {
	ctx := context.Background()
	s, err := Connect(ctx, Config{Adapter: AdapterSim, Profile: ProfileOBD, SimVariant: "can"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id, ch := s.Subscribe()
	defer s.Unsubscribe(id)
	if err := s.StartLive(LiveConfig{Params: []string{"obd.0C", "obd.05"}, Interval: 100}); err != nil {
		t.Fatal(err)
	}
	if err := s.StartMonitor(); err != nil {
		t.Fatal(err)
	}
	gotLive, gotCAN := false, false
	timeout := time.After(5 * time.Second)
	for !(gotLive && gotCAN) {
		select {
		case ev := <-ch:
			switch ev.Type {
			case "live":
				if len(ev.Values) == 2 && ev.Values[0].OK {
					gotLive = true
				}
			case "can":
				gotCAN = true
			}
		case <-timeout:
			t.Fatalf("live %v, can %v", gotLive, gotCAN)
		}
	}
	s.StopLive()
}

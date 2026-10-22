package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/elm327"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/obd"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/vehicle"
)

// A Микас-7.1 on the legacy K-Line protocol, seen through the bridge by
// the ELM327 driver as if it were an OBD-II car.
func TestBridgeSynthesisedOBD(t *testing.T) {
	ctx := context.Background()
	sess, err := vehicle.Connect(ctx, vehicle.Config{Adapter: vehicle.AdapterSim, Profile: vehicle.ProfileRuLegacy, SimVariant: "gaz-mikas"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	srv, err := Start("127.0.0.1:0", func() *vehicle.Session { return sess }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	port, err := elm327.DialTCP(srv.Addr(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := elm327.Open(ctx, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()
	ch, err := dev.Open(ctx, elm327.Spec{Proto: '0'})
	if err != nil {
		t.Fatal(err)
	}
	h, err := obd.NewHandler(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	vals, err := h.Read(ctx, []string{"obd.0C", "obd.05", "obd.42"})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vals {
		if !v.OK {
			t.Fatalf("%s not available through the bridge: %+v", v.ID, vals)
		}
	}
	if vals[0].V < 500 || vals[2].V < 10 {
		t.Fatalf("implausible values %+v", vals)
	}
}

// An OBD-II car is passed through unchanged.
func TestBridgePassThrough(t *testing.T) {
	ctx := context.Background()
	sess, err := vehicle.Connect(ctx, vehicle.Config{Adapter: vehicle.AdapterSim, Profile: vehicle.ProfileOBD, SimVariant: "can"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	srv, err := Start("127.0.0.1:0", func() *vehicle.Session { return sess }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	port, err := elm327.DialTCP(srv.Addr(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := elm327.Open(ctx, port, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()
	ch, err := dev.Open(ctx, elm327.Spec{Proto: '0'})
	if err != nil {
		t.Fatal(err)
	}
	h, err := obd.NewHandler(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := h.ReadDTC(ctx)
	if err != nil || len(codes) != 2 {
		t.Fatalf("%v %v", codes, err)
	}
}

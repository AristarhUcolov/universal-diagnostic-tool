package sim

import (
	"context"
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/isotp"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/obd"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/slcan"
)

func TestSLCAN(t *testing.T) {
	bus, _ := newCANCar(false)
	defer bus.Close()
	b, err := slcan.Open(SLCANPort(bus), 500000, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != "V1013" {
		t.Fatalf("version %q", b.Version)
	}
	mux := can.NewMux(b, nil)
	defer mux.Close()
	ctx := context.Background()
	h, err := obd.NewHandler(ctx, isotp.New(mux, isotp.OBDFunctional(false), nil))
	if err != nil {
		t.Fatal(err)
	}
	vals, err := h.Read(ctx, []string{"obd.0C", "obd.0D"})
	if err != nil || !vals[0].OK {
		t.Fatalf("%+v %v", vals, err)
	}
}

func TestSLCANFormat(t *testing.T) {
	f := can.NewFrame(0x7E0, false, []byte{0x02, 0x01, 0x0C})
	if s := slcan.FormatFrame(f); s != "t7E03"+"02010C" {
		t.Fatalf("%s", s)
	}
	g, ok := slcan.ParseFrame("T18DAF1108064100BE3FA81300")
	if !ok || g.ID != 0x18DAF110 || !g.Ext || g.Len != 8 || g.Data[1] != 0x41 {
		t.Fatalf("%+v %v", g, ok)
	}
}

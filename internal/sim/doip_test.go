package sim

import (
	"context"
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/doip"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/uds"
)

func TestDoIP(t *testing.T) {
	e := NewEngine()
	app := NewOBDApp(e, "ECM")
	app.UDS = true
	srv, err := StartDoIP("127.0.0.1:0", app)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ctx := context.Background()
	c, err := doip.Dial(ctx, srv.Addr(), 0, 0x1000, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	u := uds.NewClient(c)
	vin, err := u.ReadDID(ctx, 0xF190)
	if err != nil || string(vin) != e.VIN {
		t.Fatalf("VIN %q %v", vin, err)
	}
	rpm, err := u.ReadDID(ctx, 0xF40C) // WWH-OBD engine speed
	if err != nil || len(rpm) != 2 {
		t.Fatalf("F40C % X %v", rpm, err)
	}
	codes, _, err := u.ReadDTCs(ctx, 0xFF)
	if err != nil || len(codes) != 2 {
		t.Fatalf("dtc %v %v", codes, err)
	}
}

package j1939

import (
	"math"
	"reflect"
	"testing"
)

func TestIdentifier(t *testing.T) {
	cases := []struct {
		m  Message
		id uint32
	}{
		// EEC1 (engine speed) broadcast by the engine at priority 3.
		{Message{PGN: 61444, Priority: 3, Src: 0x00, Dst: AddrGlobal}, 0x0CF00400},
		// Request (PDU1, destination specific) from the tool to the engine.
		{Message{PGN: 59904, Priority: 6, Src: AddrTool1, Dst: 0x00}, 0x18EA00F9},
		// DM1 broadcast at priority 6.
		{Message{PGN: 65226, Priority: 6, Src: 0x00, Dst: AddrGlobal}, 0x18FECA00},
	}
	for _, c := range cases {
		if id := c.m.ID(); id != c.id {
			t.Errorf("PGN %d: ID %08X, want %08X", c.m.PGN, id, c.id)
		}
		pgn, prio, src, dst := Decode(c.id)
		if pgn != c.m.PGN || prio != c.m.Priority || src != c.m.Src || dst != c.m.Dst {
			t.Errorf("Decode(%08X) = %d %d %02X %02X", c.id, pgn, prio, src, dst)
		}
	}
}

func TestSPNDecode(t *testing.T) {
	rpm := SPNByNumber(190)
	if rpm == nil {
		t.Fatal("SPN 190 missing")
	}
	// EEC1 bytes 4-5, 0.125 rpm/bit, little endian: 0x1F40 = 8000 -> 1000 rpm.
	v, ok := rpm.Decode([]byte{0xFF, 0xFF, 0xFF, 0x40, 0x1F, 0xFF, 0xFF, 0xFF})
	if !ok || v != 1000 {
		t.Fatalf("engine speed = %v %v", v, ok)
	}
	// 0xFFFF means "not available".
	if _, ok := rpm.Decode([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}); ok {
		t.Fatal("not-available value accepted")
	}
	// 0xFB00 is in the error range above 0xFAFF.
	if _, ok := rpm.Decode([]byte{0xFF, 0xFF, 0xFF, 0x00, 0xFB, 0xFF, 0xFF, 0xFF}); ok {
		t.Fatal("error-range value accepted")
	}
	// Short data must not panic.
	if _, ok := rpm.Decode([]byte{0xFF, 0xFF}); ok {
		t.Fatal("short data accepted")
	}
	cool := SPNByNumber(110)
	if v, ok := cool.Decode([]byte{0x7D}); !ok || v != 85 {
		t.Fatalf("coolant = %v %v", v, ok)
	}
	speed := SPNByNumber(84)
	if v, ok := speed.Decode([]byte{0xFF, 0x00, 0x50}); !ok || math.Abs(v-80) > 1e-9 {
		t.Fatalf("speed = %v %v", v, ok)
	}
}

func TestDMRoundTrip(t *testing.T) {
	lamps := Lamps{MIL: true, AmberWarning: true}
	dtcs := []DTC{{SPN: 110, FMI: 0, OC: 3}, {SPN: 520192, FMI: 31, OC: 127}, {SPN: 3216, FMI: 4, OC: 1}}
	l, got := ParseDM(EncodeDM(lamps, dtcs))
	if l != lamps {
		t.Errorf("lamps = %+v", l)
	}
	if !reflect.DeepEqual(got, dtcs) {
		t.Errorf("codes = %+v", got)
	}
	// A DM1 without faults carries one all-zero record.
	l, got = ParseDM(EncodeDM(Lamps{}, nil))
	if l != (Lamps{}) || len(got) != 0 {
		t.Errorf("empty DM1: %+v %+v", l, got)
	}
	// A known DM1 frame: MIL on, SPN 100 (oil pressure) FMI 1, seen twice.
	_, got = ParseDM([]byte{0x40, 0xFF, 0x64, 0x00, 0x01, 0x02, 0xFF, 0xFF})
	if len(got) != 1 || got[0] != (DTC{SPN: 100, FMI: 1, OC: 2}) {
		t.Errorf("oil pressure frame: %+v", got)
	}
}

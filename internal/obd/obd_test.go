package obd

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
)

func TestParseMode01(t *testing.T) {
	// Several PIDs in one answer: engine speed 0x0C (2 bytes), coolant
	// 0x05 (1 byte) and speed 0x0D (1 byte).
	got := map[byte][]byte{}
	ParseMode01(hexutil.MustParse("41 0C 1A F8 05 7B 0D 32"), got)
	want := map[byte][]byte{0x0C: {0x1A, 0xF8}, 0x05: {0x7B}, 0x0D: {0x32}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// Freeze frame answers carry a frame number after each PID.
	got = map[byte][]byte{}
	ParseMode01(hexutil.MustParse("42 0C 00 0F A0"), got)
	if !bytes.Equal(got[0x0C], []byte{0x0F, 0xA0}) {
		t.Fatalf("mode 02: %v", got)
	}
	// Truncated data must not panic or produce a short value.
	got = map[byte][]byte{}
	ParseMode01(hexutil.MustParse("41 0C 1A"), got)
	if len(got) != 0 {
		t.Fatalf("truncated: %v", got)
	}
}

func TestPIDValues(t *testing.T) {
	cases := []struct {
		pid  byte
		data string
		want float64
	}{
		{0x0C, "1A F8", 1726},  // (0x1A*256 + 0xF8) / 4 rpm
		{0x05, "7B", 83},       // 0x7B - 40 °C
		{0x0D, "32", 50},       // km/h
		{0x11, "FF", 100},      // throttle %
		{0x42, "36 B0", 14.0},  // module voltage, mV / 1000
		{0x10, "01 F4", 5.0},   // MAF g/s, /100
		{0x0F, "28", 0},        // intake air temperature
		{0x04, "80", 50.19608}, // calculated load
	}
	for _, c := range cases {
		def := PIDs[c.pid]
		if def == nil || len(def.Values) == 0 {
			t.Errorf("PID %02X missing from the table", c.pid)
			continue
		}
		d := hexutil.MustParse(c.data)
		if len(d) != def.Bytes {
			t.Errorf("PID %02X: table says %d bytes, test data has %d", c.pid, def.Bytes, len(d))
			continue
		}
		v, ok := def.Values[0].F(d)
		if !ok || math.Abs(v-c.want) > 0.001 {
			t.Errorf("PID %02X % X = %v (ok %v), want %v", c.pid, d, v, ok, c.want)
		}
	}
}

func TestParseDTCs(t *testing.T) {
	// K-Line/J1850: pairs of bytes padded with 00 00.
	got := ParseDTCs(hexutil.MustParse("43 01 30 C1 00 00 00"), false)
	if !reflect.DeepEqual(got, []string{"P0130", "U0100"}) {
		t.Fatalf("K-Line: %v", got)
	}
	// CAN: the second byte is the number of codes.
	got = ParseDTCs(hexutil.MustParse("43 02 01 71 43 00"), true)
	if !reflect.DeepEqual(got, []string{"P0171", "C0300"}) {
		t.Fatalf("CAN: %v", got)
	}
	if got := ParseDTCs(hexutil.MustParse("43 00"), true); len(got) != 0 {
		t.Fatalf("no codes: %v", got)
	}
	if got := ParseDTCs(nil, true); got != nil {
		t.Fatalf("nil: %v", got)
	}
}

func TestParamIDRoundTrip(t *testing.T) {
	for _, pid := range []byte{0x0C, 0x05, 0x14} {
		for _, key := range []string{"", "trim"} {
			id := ParamID(pid, key)
			p, k, ok := ParseParamID(id)
			if !ok || p != pid || k != key {
				t.Errorf("%q -> %02X %q %v", id, p, k, ok)
			}
		}
	}
}

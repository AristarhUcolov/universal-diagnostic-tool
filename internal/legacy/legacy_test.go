package legacy

import (
	"bytes"
	"errors"
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
)

func TestEncodeKnownFrame(t *testing.T) {
	// A parameter request as sent to a Микас-7.1 (captured in the
	// journal of a session): command 0x61 with eight parameter codes.
	got := Encode(hexutil.MustParse("61 1A 29 1E 20 3F 21 26 5C"))
	want := hexutil.MustParse("61 1A 29 1E 20 3F 21 26 5C 3C 0D")
	if !bytes.Equal(got, want) {
		t.Fatalf("Encode = % X, want % X", got, want)
	}
}

func TestEscaping(t *testing.T) {
	// 0x0D inside a message would end it early and 0x40 starts an escape,
	// so both must be sent as pairs that sum to the original value.
	body := []byte{0x21, 0x0D, 0x40, 0x02}
	raw := Encode(body)
	if bytes.Count(raw, []byte{0x0D}) != 1 || raw[len(raw)-1] != 0x0D {
		t.Fatalf("0x0D not escaped: % X", raw)
	}
	if !bytes.Contains(raw, []byte{0x40, 0xCD}) || !bytes.Contains(raw, []byte{0x40, 0x00}) {
		t.Fatalf("escape pairs missing: % X", raw)
	}
	back, err := Decode(raw[:len(raw)-1])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, body) {
		t.Fatalf("round trip = % X, want % X", back, body)
	}
}

func TestRoundTripAllBytes(t *testing.T) {
	for v := 0; v < 256; v++ {
		body := []byte{0x61, byte(v), byte(255 - v)}
		raw := Encode(body)
		back, err := Decode(raw[:len(raw)-1])
		if err != nil || !bytes.Equal(back, body) {
			t.Fatalf("byte %02X: % X -> % X, %v", v, body, back, err)
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := Decode(hexutil.MustParse("61 1A 29 1E 20 3F 21 26 5C 3D")); !errors.Is(err, ErrChecksum) {
		t.Errorf("bad checksum: %v", err)
	}
	if _, err := Decode(hexutil.MustParse("01 40 11 AE")); !errors.Is(err, ErrEscape) {
		t.Errorf("bad escape: %v", err)
	}
	if _, err := Decode(hexutil.MustParse("01 40")); !errors.Is(err, ErrEscape) {
		t.Errorf("escape at the end: %v", err)
	}
	if _, err := Decode(nil); !errors.Is(err, ErrChecksum) {
		t.Errorf("empty message: %v", err)
	}
}

func TestParseErrors(t *testing.T) {
	// The fault list of a real Микас-5.47 (GAZ-3110, ZMZ-406) as it
	// arrived on the wire: faults 062, 182 and 168, each followed by 0x40.
	body, err := Decode(hexutil.MustParse("03 3E 40 00 B6 40 00 A8 40 00 A1"))
	if err != nil {
		t.Fatal(err)
	}
	codes, err := ParseErrors(body)
	if err != nil || !bytes.Equal(codes, []byte{62, 182, 168}) {
		t.Fatalf("real Микас-5.47: % X, %v", codes, err)
	}
	for _, c := range []struct{ name, resp, want string }{
		{"separator from the protocol description", "02 23 E0 AF E0", "23 AF"},
		{"last separator missing", "02 23 E0 AF", "23 AF"},
		{"codes equal to separators", "02 40 40 E0 E0", "40 E0"},
		{"no separators", "02 23 AF", "23 AF"},
		{"no faults", "00", ""},
	} {
		codes, err := ParseErrors(hexutil.MustParse(c.resp))
		if err != nil || hexutil.Format(codes) != c.want {
			t.Errorf("%s: % X, %v; want %s", c.name, codes, err, c.want)
		}
	}
	if _, err := ParseErrors(hexutil.MustParse("03 3E 40")); err == nil {
		t.Error("short list accepted")
	}
	if _, err := ParseErrors(nil); err == nil {
		t.Fatal("empty answer accepted")
	}
}

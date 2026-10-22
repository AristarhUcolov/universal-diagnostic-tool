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
	// Count, then the codes; 0xE0 entries are fillers and skipped.
	codes, err := ParseErrors([]byte{0x02, 0x23, 0xE0, 0xAF})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(codes, []byte{0x23, 0xAF}) {
		t.Fatalf("codes = % X", codes)
	}
	codes, err = ParseErrors([]byte{0x00})
	if err != nil || len(codes) != 0 {
		t.Fatalf("no faults: % X, %v", codes, err)
	}
	if _, err := ParseErrors(nil); err == nil {
		t.Fatal("empty answer accepted")
	}
}

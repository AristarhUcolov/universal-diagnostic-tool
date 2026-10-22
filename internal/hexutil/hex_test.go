package hexutil

import (
	"bytes"
	"testing"
)

func TestParse(t *testing.T) {
	want := []byte{0x81, 0x10, 0xF1}
	for _, s := range []string{"81 10 F1", "8110F1", "0x81,0x10,0xF1", "81-10-f1", "81:10:F1", " 81  10\tF1 ", "0X81;10;f1"} {
		got, err := Parse(s)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("Parse(%q) = % X, %v", s, got, err)
		}
	}
	if got, err := Parse("1 2 A"); err != nil || !bytes.Equal(got, []byte{1, 2, 0x0A}) {
		t.Errorf("single digits: % X, %v", got, err)
	}
	if got, err := Parse(""); err != nil || len(got) != 0 {
		t.Errorf("empty: % X, %v", got, err)
	}
	for _, bad := range []string{"123", "GG", "81 1X", "0x"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
}

func TestFormat(t *testing.T) {
	b := []byte{0x00, 0x7F, 0xAB}
	if s := Format(b); s != "00 7F AB" {
		t.Errorf("Format = %q", s)
	}
	if s := Compact(b); s != "007FAB" {
		t.Errorf("Compact = %q", s)
	}
	if s := Format(nil); s != "" {
		t.Errorf("Format(nil) = %q", s)
	}
}

func TestText(t *testing.T) {
	b := []byte{0x00, 'X', 'T', 'A', 0x0A, '1', ' ', 0xFF}
	if s := Printable(b); s != ".XTA.1 ." {
		t.Errorf("Printable = %q", s)
	}
	if s := ASCII(b); s != "XTA1" {
		t.Errorf("ASCII = %q", s)
	}
}

// Package hexutil converts between byte slices and the hex notation used
// throughout diagnostics ("81 10 F1 81 03").
package hexutil

import (
	"fmt"
	"strings"
)

// Parse accepts "81 10 F1", "8110F1", "0x81,0x10", "81-10-f1" and similar
// spellings. Groups are separated by spaces, commas, dashes, colons or
// semicolons; each group may carry a 0x prefix. A group with a single
// digit means one byte ("1 2" is 01 02); longer groups must have an even
// number of digits and are read as a run of bytes.
func Parse(s string) ([]byte, error) {
	fields := strings.FieldsFunc(s, isSep)
	var out []byte
	for _, f := range fields {
		if len(f) > 2 && (f[:2] == "0x" || f[:2] == "0X") {
			f = f[2:]
		}
		if len(f) == 1 {
			f = "0" + f
		}
		if len(f)%2 != 0 {
			return nil, fmt.Errorf("odd number of hex digits in %q", f)
		}
		for i := 0; i < len(f); i += 2 {
			hi, ok1 := nibble(f[i])
			lo, ok2 := nibble(f[i+1])
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("invalid hex %q", f)
			}
			out = append(out, hi<<4|lo)
		}
	}
	return out, nil
}

// MustParse is Parse for constants in code and tests.
func MustParse(s string) []byte {
	b, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return b
}

// Format renders bytes as upper-case pairs separated by spaces.
func Format(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	const digits = "0123456789ABCDEF"
	out := make([]byte, 0, len(b)*3-1)
	for i, v := range b {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, digits[v>>4], digits[v&0x0F])
	}
	return string(out)
}

// Compact renders bytes as upper-case hex without separators.
func Compact(b []byte) string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, digits[v>>4], digits[v&0x0F])
	}
	return string(out)
}

// Printable returns the bytes as text, replacing control and non-ASCII
// bytes with '.', which is handy for identification strings.
func Printable(b []byte) string {
	out := make([]byte, len(b))
	for i, v := range b {
		if v >= 0x20 && v < 0x7F {
			out[i] = v
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}

// ASCII returns the printable ASCII part of b with padding bytes (00, FF,
// spaces) trimmed from both ends. Non-printable bytes inside are dropped.
func ASCII(b []byte) string {
	var sb strings.Builder
	for _, v := range b {
		if v >= 0x20 && v < 0x7F {
			sb.WriteByte(v)
		}
	}
	return strings.TrimSpace(sb.String())
}

func isSep(r rune) bool {
	switch r {
	case ' ', ',', '-', ':', ';', '\t', '\n', '\r', '_':
		return true
	}
	return false
}

func nibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

package kw1281

import (
	"fmt"
	"math"
)

// Cell is one decoded value of a measuring value group. Each value is
// sent as three bytes: formula type, A and B.
type Cell struct {
	Formula byte    `json:"formula"`
	A       byte    `json:"a"`
	B       byte    `json:"b"`
	Value   float64 `json:"value"`
	Numeric bool    `json:"numeric"`
	Unit    string  `json:"unit"`
	Text    string  `json:"text"`
	Raw     []byte  `json:"raw,omitempty"`
}

type formula struct {
	unit string
	fn   func(a, b float64) float64
}

// formulas lists the common measuring value types. The table follows the
// widely published community documentation of KW1281 (it is not an
// official VAG document); unknown types are shown as raw A/B values.
var formulas = map[byte]formula{
	1:  {"/min", func(a, b float64) float64 { return a * b / 5 }},
	2:  {"%", func(a, b float64) float64 { return a * 0.002 * b }},
	3:  {"°", func(a, b float64) float64 { return 0.002 * a * b }},
	5:  {"°C", func(a, b float64) float64 { return a * (b - 100) * 0.1 }},
	6:  {"V", func(a, b float64) float64 { return 0.001 * a * b }},
	7:  {"km/h", func(a, b float64) float64 { return 0.01 * a * b }},
	8:  {"", func(a, b float64) float64 { return 0.1 * a * b }},
	9:  {"°", func(a, b float64) float64 { return (b - 127) * 0.02 * a }},
	11: {"", func(a, b float64) float64 { return 0.0001*a*(b-128) + 1 }},
	12: {"Ω", func(a, b float64) float64 { return 0.001 * a * b }},
	13: {"mm", func(a, b float64) float64 { return (b - 127) * 0.001 * a }},
	14: {"bar", func(a, b float64) float64 { return 0.005 * a * b }},
	15: {"ms", func(a, b float64) float64 { return 0.01 * a * b }},
	18: {"mbar", func(a, b float64) float64 { return 0.04 * a * b }},
	19: {"l", func(a, b float64) float64 { return a * b * 0.01 }},
	20: {"%", func(a, b float64) float64 { return a * (b - 128) / 128 }},
	21: {"V", func(a, b float64) float64 { return 0.001 * a * b }},
	22: {"ms", func(a, b float64) float64 { return 0.001 * a * b }},
	23: {"%", func(a, b float64) float64 { return b / 256 * a }},
	24: {"A", func(a, b float64) float64 { return 0.001 * a * b }},
	25: {"g/s", func(a, b float64) float64 { return b*1.421 + a/182 }},
	26: {"°C", func(a, b float64) float64 { return b - a }},
	28: {"", func(a, b float64) float64 { return b - a }},
	30: {"°KW", func(a, b float64) float64 { return b / 12 * a }},
	31: {"°C", func(a, b float64) float64 { return b / 2560 * a }},
	33: {"%", func(a, b float64) float64 {
		if a == 0 {
			return 100 * b
		}
		return 100 * b / a
	}},
	34: {"kW", func(a, b float64) float64 { return (b - 128) * 0.01 * a }},
	35: {"l/h", func(a, b float64) float64 { return 0.01 * a * b }},
	36: {"km", func(a, b float64) float64 { return a*2560 + b*10 }},
	38: {"°KW", func(a, b float64) float64 { return (b - 128) * 0.001 * a }},
	39: {"mg/h", func(a, b float64) float64 { return b / 256 * a }},
	40: {"A", func(a, b float64) float64 { return b*0.1 + 25.5*a - 400 }},
	41: {"Ah", func(a, b float64) float64 { return b + a*255 }},
	42: {"kW", func(a, b float64) float64 { return b*0.1 + 25.5*a - 400 }},
	43: {"V", func(a, b float64) float64 { return b*0.1 + 25.5*a }},
	47: {"ms", func(a, b float64) float64 { return (b - 128) * a }},
	48: {"", func(a, b float64) float64 { return b + a*255 }},
	49: {"mg/h", func(a, b float64) float64 { return (b / 4) * a * 0.1 }},
	50: {"mbar", func(a, b float64) float64 { return (b - 128) / (0.01 * a) }},
	51: {"mg/h", func(a, b float64) float64 { return ((b - 128) / 255) * a }},
	52: {"Nm", func(a, b float64) float64 { return b*0.02*a - a }},
	53: {"g/s", func(a, b float64) float64 { return (b-128)*1.4222 + 0.006*a }},
	54: {"", func(a, b float64) float64 { return a*256 + b }},
	55: {"s", func(a, b float64) float64 { return a * b / 200 }},
	59: {"%", func(a, b float64) float64 { return (a*256 + b) / 32768 }},
	60: {"s", func(a, b float64) float64 { return (a*256 + b) * 0.01 }},
	61: {"", func(a, b float64) float64 { return (b - 128) / a }},
	62: {"S", func(a, b float64) float64 { return 0.256 * a * b }},
	64: {"Ω", func(a, b float64) float64 { return a + b }},
	65: {"mm", func(a, b float64) float64 { return 0.01 * a * (b - 127) }},
	66: {"V", func(a, b float64) float64 { return (a * b) / 511.12 }},
}

// DecodeCell converts one measuring value.
func DecodeCell(f, a, b byte) Cell {
	c := Cell{Formula: f, A: a, B: b}
	fa, fb := float64(a), float64(b)
	switch f {
	case 4: // ignition angle, before/after TDC
		v := math.Abs(fb-127) * 0.01 * fa
		dir := "BTDC"
		if fb > 127 {
			dir = "ATDC"
		}
		c.Value, c.Numeric, c.Unit = v, true, "° "+dir
	case 10:
		if b == 0 {
			c.Text = "COLD"
		} else {
			c.Text = "WARM"
		}
	case 16: // bit field: B masked with A
		c.Text = fmt.Sprintf("%08b", a&b)
	case 17:
		c.Text = string([]byte{a, b})
	case 27: // ignition angle variant
		v := math.Abs(fb-128) * 0.01 * fa
		dir := "BTDC"
		if fb > 128 {
			dir = "ATDC"
		}
		c.Value, c.Numeric, c.Unit = v, true, "° "+dir
	case 29:
		if b < a {
			c.Text = "map 1"
		} else {
			c.Text = "map 2"
		}
	case 32:
		v := fb
		if b > 128 {
			v = fb - 256
		}
		c.Value, c.Numeric = v, true
	case 37:
		c.Text = fmt.Sprintf("%d", b)
	case 44:
		c.Text = fmt.Sprintf("%02d:%02d", a, b)
	case 63:
		c.Text = string([]byte{a, b})
	default:
		if fm, ok := formulas[f]; ok {
			v := fm.fn(fa, fb)
			if !math.IsInf(v, 0) && !math.IsNaN(v) {
				c.Value, c.Numeric, c.Unit = v, true, fm.unit
			}
		}
	}
	if c.Numeric {
		c.Text = trimFloat(c.Value) + addSpace(c.Unit)
	} else if c.Text == "" {
		c.Text = fmt.Sprintf("type %d: %d %d", f, a, b)
	}
	return c
}

func addSpace(unit string) string {
	if unit == "" {
		return ""
	}
	return " " + unit
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.3f", v)
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

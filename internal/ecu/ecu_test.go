package ecu

import (
	"math"
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
)

func TestExpr(t *testing.T) {
	cases := []struct {
		src  string
		x    float64
		want float64
	}{
		{"x-40", 71, 31},
		{"5.2+x*0.05", 0x8E, 12.3},
		{"14.7*(x+128)/256", 128, 14.7},
		{"-x/2", 24, -12},
		{"(x & 0x0F) << 2", 0x35, 20},
		{"abs(x-128)/256", 64, 0.25},
		{"max(x, 10, 3)", 4, 10},
		{"2 + 3 * 4", 0, 14},
		{"(2 + 3) * 4", 0, 20},
		{"x % 7", 23, 2},
	}
	for _, c := range cases {
		e, err := Compile(c.src)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		got, err := e.Eval(map[string]float64{"x": c.x})
		if err != nil || math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("%s with x=%v: got %v (%v), want %v", c.src, c.x, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "x +", "(x", "foo(", "1 2"} {
		if _, err := Compile(bad); err == nil {
			t.Fatalf("%q should not compile", bad)
		}
	}
}

func TestBuiltinDefinitions(t *testing.T) {
	lib := Builtin()
	if errs := lib.Errors(); len(errs) > 0 {
		t.Fatalf("definition errors: %v", errs)
	}
	for _, id := range []string{"vaz-m154n-euro2", "vaz-j72", "vaz-m154-r83", "ru-kwp-generic", "gaz-mikas71"} {
		if _, ok := lib.Get(id); !ok {
			t.Fatalf("definition %s missing", id)
		}
	}
	j72, _ := lib.Get("vaz-j72")
	if len(j72.Blocks) == 0 || j72.Match.BlockLen["21 01"] != 38 || j72.Status != "partial" {
		t.Fatalf("extends did not merge: %+v", j72.Match)
	}
}

// A real answer of a Январь-7.2 controller to 21 01 with the engine
// stopped (published on cxem.net): it must decode to plausible values
// with the AvtoVAZ layout.
func TestJanuary72RealFrame(t *testing.T) {
	resp := hexutil.MustParse("61 01 3B 90 41 04 00 00 00 00 47 80 00 00 00 52 52 80 18 00 8E 00 5C 00 00 00 00 00 00 00 00 00 FF FF DD A4 47 02")
	def, _ := Builtin().Get("vaz-j72")
	want := map[string]float64{
		"twat": 31, "afr": 14.7, "thr": 0, "rpm": 0, "iac_desired": 82, "iac": 82,
		"inj_corr": 1, "uoz": 12, "speed": 0, "uacc": 12.3, "o2v": 0.449, "engine_stopped": 1,
	}
	data := resp[2:]
	got := map[string]float64{}
	for _, p := range def.Blocks[0].Params {
		v := p.Decode(data)
		if v.OK {
			got[p.ID] = v.V
		}
	}
	for id, w := range want {
		if g, ok := got[id]; !ok || math.Abs(g-w) > 0.001 {
			t.Errorf("%s = %v, want %v", id, g, w)
		}
	}
}

func TestFill(t *testing.T) {
	b, err := Fill("30 42 07 {v/10}", 1000)
	if err != nil || hexutil.Format(b) != "30 42 07 64" {
		t.Fatalf("% X %v", b, err)
	}
	b, err = Fill("2E 01 00 {v:u16be}", 0x1234)
	if err != nil || hexutil.Format(b) != "2E 01 00 12 34" {
		t.Fatalf("% X %v", b, err)
	}
}

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

// A definition that extends another inherits the fields it does not name,
// but a list it brings replaces the base's list as a whole: an entry must
// not pick up the formula, OBD mapping or engine condition of whatever
// stood at the same place in the base.
func TestExtend(t *testing.T) {
	l := &Library{defs: map[string]*Definition{}, raw: map[string][]byte{}}
	errs := l.addAll(map[string][]byte{
		"base": []byte(`{"id": "base", "name": "Base", "protocol": "kwp2000", "profile": "p", "status": "partial",
			"session": "10 81", "match": {"version": ["09"]},
			"blocks": [{"id": "01", "request": "21 01", "params": [
				{"id": "a", "offset": 0, "type": "u16le", "expr": "x/128", "unit": "l/100km", "obd": "14"}]}],
			"actuators": [{"id": "inj", "kind": "pulse", "on": "30 01 06 01", "condition": "engine_off", "timeout_s": 30}]}`),
		"child": []byte(`{"id": "child", "extends": "base", "name": "Child",
			"blocks": [{"id": "01", "request": "21 01", "params": [{"id": "b", "offset": 0}]}],
			"actuators": [{"id": "fan", "kind": "toggle", "on": "30 0A 06 01"}]}`),
	})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	c, ok := l.Get("child")
	if !ok {
		t.Fatal("child not loaded")
	}
	if c.Protocol != "kwp2000" || c.Session != "10 81" || c.Name != "Child" {
		t.Errorf("fields not inherited or not overridden: %+v", c)
	}
	if len(c.Match.Version) != 0 {
		t.Errorf("identification rules inherited: %+v", c.Match)
	}
	if p := c.Blocks[0].Params[0]; p.ID != "b" || p.Type != "u8" || p.Expr != "" || p.Unit != "" || p.OBD != "" {
		t.Errorf("parameter mixed with the base's: %+v", p)
	}
	if a := c.Actuators[0]; a.ID != "fan" || a.Condition != "" || a.TimeoutS != 0 {
		t.Errorf("output test mixed with the base's: %+v", a)
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

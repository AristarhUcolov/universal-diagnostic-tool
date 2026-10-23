package pcan

import (
	"regexp"
	"strconv"
	"testing"
)

// The bit timing strings must give exactly the asked bit rates at an
// 80 MHz clock, with the sample point at 80 % (81 % for 5 Mbit/s).
func TestFDBitrate(t *testing.T) {
	field := regexp.MustCompile(`(\w+)=(\d+)`)
	for _, c := range []struct{ nominal, data int }{
		{500000, 2000000}, {500000, 4000000}, {500000, 5000000}, {500000, 8000000},
		{500000, 1000000}, {250000, 2000000}, {1000000, 4000000}, {125000, 1000000},
	} {
		s, err := FDBitrate(c.nominal, c.data)
		if err != nil {
			t.Fatalf("%d/%d: %v", c.nominal, c.data, err)
		}
		v := map[string]int{}
		for _, m := range field.FindAllStringSubmatch(s, -1) {
			v[m[1]], _ = strconv.Atoi(m[2])
		}
		nomTQ := 1 + v["nom_tseg1"] + v["nom_tseg2"]
		dataTQ := 1 + v["data_tseg1"] + v["data_tseg2"]
		if got := 80000000 / v["nom_brp"] / nomTQ; got != c.nominal {
			t.Errorf("%s: nominal %d", s, got)
		}
		if got := 80000000 / v["data_brp"] / dataTQ; got != c.data {
			t.Errorf("%s: data %d", s, got)
		}
		if sp := float64(1+v["data_tseg1"]) / float64(dataTQ); sp < 0.75 || sp > 0.85 {
			t.Errorf("%s: data sample point %.2f", s, sp)
		}
	}
	for _, c := range [][2]int{{300000, 2000000}, {500000, 3000000}, {500000, 0}} {
		if s, err := FDBitrate(c[0], c[1]); err == nil {
			t.Errorf("%v accepted: %s", c, s)
		}
	}
}

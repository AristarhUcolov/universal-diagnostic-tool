package repair

import (
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// TestHints checks the advice file: every entry complete in both
// languages, known severities, unique ids.
func TestHints(t *testing.T) {
	sev := map[string]bool{"stop": true, "soon": true, "info": true}
	seen := map[string]bool{}
	both := func(id, field string, x i18n.Text) {
		if x["en"] == "" || x["ru"] == "" {
			t.Errorf("%s %s: needs en and ru, got %v", id, field, x)
		}
	}
	hs := All()
	if len(hs) < 20 {
		t.Fatalf("only %d hints", len(hs))
	}
	for _, h := range hs {
		if seen[h.ID] {
			t.Errorf("%s: duplicate id", h.ID)
		}
		seen[h.ID] = true
		if !sev[h.Severity] {
			t.Errorf("%s: severity %q", h.ID, h.Severity)
		}
		both(h.ID, "title", h.Title)
		both(h.ID, "what", h.What)
		both(h.ID, "symptoms", h.Symptoms)
		if len(h.Causes) == 0 || len(h.Checks) == 0 {
			t.Errorf("%s: needs causes and checks", h.ID)
		}
		for _, c := range append(append([]i18n.Text{}, h.Causes...), h.Checks...) {
			both(h.ID, "item", c)
		}
	}
	for _, r := range saeRanges {
		if Get(r.id) == nil {
			t.Errorf("range %X-%X points to unknown hint %q", r.lo, r.hi, r.id)
		}
	}
	for _, k := range keywords {
		if Get(k.id) == nil {
			t.Errorf("keyword rule points to unknown hint %q", k.id)
		}
	}
}

func TestMatch(t *testing.T) {
	db := dtc.Default()
	cases := []struct{ code, family, want string }{
		{"P0130", "", "o2"},
		{"P0135", "", "o2heater"},
		{"P0171", "", "lean"},
		{"P0301", "", "misfire"},
		{"P0335", "", "ckp"},
		{"P0505", "", "iac"},
		{"P0562", "", "voltage"},
		{"U0100", "", "network"},
		{"P0118", "", "ect"},
		{"035", dtc.FamilyGazMikas, "o2"},            // Микас: low signal of the oxygen sensor
		{"175", dtc.FamilyGazMikas, "evap"},          // Микас: canister valve circuit
		{"071", dtc.FamilyGazMikas, "iac"},           // low idle speed, not the crankshaft sensor
		{"062", dtc.FamilyGazMikas, "ecu"},           // the three codes of a real Микас-5.47:
		{"182", dtc.FamilyGazMikas, "mil"},           // RAM, the lamp circuit
		{"168", dtc.FamilyGazMikas, "fuelpump"},      // and the fuel pump relay circuit
		{"177", dtc.FamilyGazMikas, "outputcircuit"}, // short to battery in the main relay circuit
		{"099", dtc.FamilyGazMikas, "coil"},          // ignition high-voltage stage
		{"025", dtc.FamilyGazMikas, "voltage"},
	}
	for _, c := range cases {
		var desc i18n.Text
		if c.family != "" {
			if txt, _, ok := db.Describe(c.code, c.family); ok {
				desc = txt
			}
		}
		if got := Match(c.code, desc); got != c.want {
			t.Errorf("%s: %s, want %s (description %v)", c.code, got, c.want, desc)
		}
	}
	if got := Match("B1234", nil); got != "generic" {
		t.Errorf("unknown body code: %s", got)
	}
}

// TestCoverage reports how many described manufacturer codes get
// specific advice, so a regression in the keyword rules shows up.
func TestCoverage(t *testing.T) {
	db := dtc.Default()
	for _, fam := range []string{dtc.FamilyGazMikas, dtc.FamilyRuKWP} {
		total, specific := 0, 0
		for _, code := range db.Codes(fam) {
			txt, _, _ := db.Describe(code, fam)
			total++
			if Match(code, txt) != "generic" {
				specific++
			}
		}
		t.Logf("%s: %d of %d codes have specific advice", fam, specific, total)
		if total > 0 && specific*10 < total*7 {
			t.Errorf("%s: only %d of %d codes have specific advice", fam, specific, total)
		}
	}
}

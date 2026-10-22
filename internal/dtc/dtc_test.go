package dtc

import "testing"

func TestDatabase(t *testing.T) {
	db := Default()
	for _, fam := range []string{FamilySAE, FamilyRuKWP, FamilyRuR83, FamilyGazMikas, FamilyVAG} {
		if db.Count(fam) == 0 {
			t.Fatalf("family %s is empty (families: %v)", fam, db.Families())
		}
	}
	cases := map[string]string{"053": FamilyGazMikas, "P1426": FamilyRuKWP, "00532": FamilyVAG, "P0130": FamilySAE}
	for code, fam := range cases {
		ms := db.Lookup(code)
		found := false
		for _, m := range ms {
			if m.Family == fam {
				found = true
			}
		}
		if !found {
			t.Errorf("%s not found in %s: %+v", code, fam, ms)
		}
	}
	if ms := db.Lookup("16684"); len(ms) == 0 || ms[0].Code != "16684 = P0300" {
		t.Errorf("VAG 16684: %+v", ms)
	}
	if ms := db.Lookup("17965"); len(ms) == 0 || ms[0].Code != "17965 = P1557" {
		t.Errorf("VAG 17965: %+v", ms)
	}
}

func TestSAE(t *testing.T) {
	for _, c := range []struct {
		hi, lo byte
		code   string
	}{{0x01, 0x30, "P0130"}, {0x14, 0x26, "P1426"}, {0xC1, 0x00, "U0100"}, {0x40, 0x35, "C0035"}, {0x9A, 0x01, "B1A01"}} {
		if got := SAE(c.hi, c.lo); got != c.code {
			t.Errorf("SAE(%02X %02X) = %s, want %s", c.hi, c.lo, got, c.code)
		}
		hi, lo, err := ParseSAE(c.code)
		if err != nil || hi != c.hi || lo != c.lo {
			t.Errorf("ParseSAE(%s) = %02X %02X %v", c.code, hi, lo, err)
		}
	}
}

func TestStatus(t *testing.T) {
	var d DTC
	ApplyKWPStatus(&d, 0xE0)
	if !d.MIL || !d.Active {
		t.Errorf("KWP E0: %+v", d)
	}
	ApplyUDSStatus(&d, 0x2C)
	if d.Active || !d.Confirmed || !d.Pending {
		t.Errorf("UDS 2C: %+v", d)
	}
}

package obd

import (
	"math"
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
)

func TestParseTests(t *testing.T) {
	// One answer with three records: an oxygen sensor threshold in
	// 0.122 mV steps, a misfire count without limits and a signed
	// temperature; the trailing byte of an incomplete record is ignored.
	got := ParseTests(hexutil.MustParse("46" +
		" 01 01 0A 0E 69 0E 69 0E 69" +
		" A4 0C 24 00 76 00 00 00 00" +
		" 21 85 96 FF 83 FF 38 01 F4" +
		" 05"))
	if len(got) != 3 {
		t.Fatalf("%d results: %+v", len(got), got)
	}
	o2, mis, temp := got[0], got[1], got[2]
	if math.Abs(o2.Value-450.058) > 0.001 || o2.Unit != "mV" || !o2.Limits || !o2.Pass || o2.Test.Empty() {
		t.Errorf("oxygen sensor threshold: %+v", o2)
	}
	if o2.Monitor["en"] != "Oxygen sensor, bank 1 sensor 1" {
		t.Errorf("monitor name %q", o2.Monitor["en"])
	}
	if mis.Value != 118 || mis.Limits || !mis.Pass || mis.Monitor["ru"] != "Пропуски зажигания, цилиндр 3" {
		t.Errorf("misfire count: %+v", mis)
	}
	if temp.Value != -12.5 || temp.Min != -20 || temp.Max != 50 || !temp.Pass || temp.Unit != "°C" || !temp.Test.Empty() {
		t.Errorf("signed temperature: %+v", temp)
	}

	// A value outside its limits fails.
	got = ParseTests(hexutil.MustParse("46 21 81 20 00 DD 00 00 00 C0"))
	if len(got) != 1 || got[0].Pass || math.Abs(got[0].Value-0.863) > 0.001 {
		t.Errorf("catalyst over its limit: %+v", got)
	}
	// Unknown scaling shows the raw number.
	got = ParseTests(hexutil.MustParse("46 81 90 50 00 07 00 00 00 10"))
	if len(got) != 1 || got[0].Value != 7 || got[0].Unit != "" {
		t.Errorf("unknown scaling: %+v", got)
	}
	if ParseTests(hexutil.MustParse("41 0C 1A F8")) != nil {
		t.Error("an answer of another service was decoded")
	}
}

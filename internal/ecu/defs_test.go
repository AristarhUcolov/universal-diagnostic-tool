package ecu

import (
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// TestDefinitionData checks what loading cannot: every description says
// where its data comes from, which vehicles it is for, and reads in both
// languages. A wrong formula can make someone replace a good part, so a
// description without a source is not accepted.
func TestDefinitionData(t *testing.T) {
	statuses := map[string]bool{"verified": true, "partial": true, "unverified": true}
	kinds := map[string]bool{"toggle": true, "pulse": true, "value": true}
	both := func(where string, txt i18n.Text) {
		if txt["en"] == "" || txt["ru"] == "" {
			t.Errorf("%s: needs \"en\" and \"ru\" text, got %v", where, txt)
		}
	}
	defs := Builtin().All()
	if len(defs) == 0 {
		t.Fatal("no built-in definitions")
	}
	for _, d := range defs {
		if !statuses[d.Status] {
			t.Errorf("%s: status %q, want verified, partial or unverified", d.ID, d.Status)
		}
		if len(d.Sources) == 0 {
			t.Errorf("%s: no sources", d.ID)
		}
		if len(d.Vehicles) == 0 {
			t.Errorf("%s: no vehicles", d.ID)
		}
		if len(d.Notes) > 0 {
			both(d.ID+" notes", d.Notes)
		}
		ids := map[string]bool{}
		for _, b := range d.Blocks {
			both(d.ID+" block "+b.ID, b.Name)
			for _, p := range b.Params {
				where := d.ID + " parameter " + p.ID
				if ids[p.ID] {
					t.Errorf("%s: id used twice", where)
				}
				ids[p.ID] = true
				both(where, p.Name)
				if len(p.Note) > 0 {
					both(where+" note", p.Note)
				}
				for k, v := range p.Enum {
					both(where+" value "+k, v)
				}
				numeric := p.Type != "ascii" && p.Type != "flags" && p.Type != "bit" && len(p.Enum) == 0
				if numeric && !(p.Max > p.Min) {
					t.Errorf("%s: range %v..%v is empty", where, p.Min, p.Max)
				}
			}
		}
		rpm := false // conditions are checked against the engine speed
		for _, b := range d.Blocks {
			for _, p := range b.Params {
				rpm = rpm || p.OBD == "0C"
			}
		}
		for _, a := range d.Actuators {
			where := d.ID + " actuator " + a.ID
			if a.Condition != "" && !rpm {
				t.Errorf("%s: condition %s, but no parameter maps to OBD 0C (engine speed)", where, a.Condition)
			}
			both(where, a.Name)
			if !kinds[a.Kind] {
				t.Errorf("%s: kind %q, want toggle, pulse or value", where, a.Kind)
			}
			if a.Condition != "" && a.Condition != "engine_off" && a.Condition != "engine_running" {
				t.Errorf("%s: condition %q, want engine_off or engine_running", where, a.Condition)
			}
			// A pulse fires an injector or a coil: never into a running engine.
			if a.Kind == "pulse" && a.Condition != "engine_off" {
				t.Errorf("%s: a pulse test needs condition engine_off", where)
			}
			if len(a.Warning) > 0 {
				both(where+" warning", a.Warning)
			}
		}
	}
}

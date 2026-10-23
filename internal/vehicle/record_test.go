package vehicle

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
)

// A page that adds values to the polled set during a recording must not
// shift the recording's columns.
func TestRecorderColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live.csv")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	r := &recorder{f: f, w: csv.NewWriter(f), path: path}
	r.write([]ecu.Value{{ID: "rpm", V: 800, OK: true}, {ID: "twat", V: 82, OK: true}})
	r.write([]ecu.Value{{ID: "speed", V: 50, OK: true}, {ID: "twat", V: 83, OK: true}, {ID: "rpm", V: 900, OK: true}})
	r.write([]ecu.Value{{ID: "rpm", V: 950, OK: true}})
	r.w.Flush()
	f.Close()

	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	rows, err := csv.NewReader(in).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"time,rpm,twat", "800,82", "900,83", "950,"}
	if len(rows) != len(want) {
		t.Fatalf("%d rows: %v", len(rows), rows)
	}
	for i, row := range rows {
		got := strings.Join(row, ",")
		if i > 0 {
			got = strings.Join(row[1:], ",")
		}
		if got != want[i] {
			t.Errorf("row %d = %q, want %q", i, got, want[i])
		}
	}
}

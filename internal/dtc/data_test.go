package dtc

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

// TestDataFiles checks every line of the embedded tables. Most
// contributions to this project are descriptions, often sent by people
// without Go installed, and the loader skips lines it cannot read, so a
// typo would silently lose a description. Here it fails the build.
func TestDataFiles(t *testing.T) {
	saeCode := regexp.MustCompile(`^[PCBU][0-3][0-9A-F]{3}$`)
	formats := map[string]*regexp.Regexp{
		FamilySAE:      saeCode,
		FamilyRuKWP:    saeCode,
		FamilyRuR83:    saeCode,
		FamilyGazMikas: regexp.MustCompile(`^[0-9]{3}$`),
		FamilyVAG:      regexp.MustCompile(`^[0-9]{5}$`),
	}
	cyrillic := regexp.MustCompile(`\p{Cyrillic}`)
	files, err := fs.Glob(dataFS, "data/*.dtc")
	if err != nil || len(files) == 0 {
		t.Fatalf("no tables: %v", err)
	}
	for _, f := range files {
		family := strings.TrimSuffix(path.Base(f), ".dtc")
		format := formats[family]
		if format == nil {
			t.Errorf("%s: unknown family; add its code format to this test", f)
			continue
		}
		data, _ := dataFS.ReadFile(f)
		seen := map[string]int{}
		for i, line := range strings.Split(string(data), "\n") {
			n := i + 1
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) != 3 {
				t.Errorf("%s:%d: want \"CODE | English | Russian\", got %q", f, n, line)
				continue
			}
			code := strings.TrimSpace(parts[0])
			en, ru := strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
			if !format.MatchString(code) {
				t.Errorf("%s:%d: code %q does not look like a %s code", f, n, code, family)
			}
			if en == "" || ru == "" {
				t.Errorf("%s:%d: %s needs both the English and the Russian text", f, n, code)
			}
			if ru != "" && !cyrillic.MatchString(ru) {
				t.Errorf("%s:%d: %s: the third column must be Russian", f, n, code)
			}
			if prev, dup := seen[code]; dup {
				t.Errorf("%s:%d: %s is already described on line %d", f, n, code, prev)
			}
			seen[code] = n
		}
	}
}

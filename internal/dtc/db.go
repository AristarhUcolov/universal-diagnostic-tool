package dtc

import (
	"bufio"
	"embed"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// Description families. A family is a set of descriptions that applies
// to a group of control units; it overrides the generic SAE table.
const (
	FamilySAE      = "sae"       // SAE J2012 generic codes
	FamilyRuKWP    = "ru-kwp"    // VAZ/GAZ/UAZ KWP2000 ECUs: Bosch M1.5.4/M7.9.7, Январь-5/7, Микас-10/11, Итэлма
	FamilyRuR83    = "ru-r83"    // Bosch M1.5.4 / Январь-5 for Russia-83 norms (no oxygen sensor)
	FamilyGazMikas = "gaz-mikas" // numeric codes of Микас-5.4/7.x and Автрон (012...253)
	FamilyVAG      = "vag"       // VAG KW1281 five-digit codes
)

//go:embed data/*.dtc
var dataFS embed.FS

// DB holds fault descriptions grouped by family.
type DB struct {
	families map[string]map[string]i18n.Text
}

var (
	defaultOnce sync.Once
	defaultDB   *DB
)

// Default returns the database built from the embedded tables.
func Default() *DB {
	defaultOnce.Do(func() {
		db := &DB{families: map[string]map[string]i18n.Text{}}
		entries, _ := dataFS.ReadDir("data")
		for _, e := range entries {
			f, err := dataFS.Open("data/" + e.Name())
			if err != nil {
				continue
			}
			family := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
			_ = db.Load(f, family)
			f.Close()
		}
		defaultDB = db
	})
	return defaultDB
}

// Load adds descriptions from a "|"-separated table: CODE | English |
// Russian. Lines starting with # are comments.
func (db *DB) Load(r io.Reader, family string) error {
	m := db.families[family]
	if m == nil {
		m = map[string]i18n.Text{}
		db.families[family] = m
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		code := strings.ToUpper(strings.TrimSpace(parts[0]))
		ru := ""
		if len(parts) > 2 {
			ru = strings.TrimSpace(parts[2])
		}
		m[code] = i18n.T(strings.TrimSpace(parts[1]), ru)
	}
	return sc.Err()
}

// Families lists the loaded families.
func (db *DB) Families() []string {
	out := make([]string, 0, len(db.families))
	for f := range db.families {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// Count returns the number of descriptions in a family.
func (db *DB) Count(family string) int { return len(db.families[family]) }

// Describe looks the code up in the given families in order and then in
// the generic SAE table. It returns the text and the family it came
// from.
func (db *DB) Describe(code string, families ...string) (i18n.Text, string, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, f := range families {
		if t, ok := db.families[f][code]; ok {
			return t, f, true
		}
	}
	if t, ok := db.families[FamilySAE][code]; ok {
		return t, FamilySAE, true
	}
	return nil, "", false
}

// Fill sets the description of d, falling back to the code's system when
// the code is not in the database.
func (db *DB) Fill(d *DTC, families ...string) {
	if t, fam, ok := db.Describe(d.Code, families...); ok {
		d.Desc, d.Family, d.Known = t, fam, true
		return
	}
	if sae, ok := d.Extra["sae"]; ok {
		if t, fam, ok := db.Describe(sae, families...); ok {
			d.Desc, d.Family, d.Known = t, fam, true
			return
		}
	}
	d.Known = false
	if IsSAE(d.Code) {
		d.Desc = SystemOf(d.Code)
	}
}

// Match is one search result.
type Match struct {
	Code   string    `json:"code"`
	Family string    `json:"family"`
	Desc   i18n.Text `json:"desc"`
}

// Search finds codes whose code or description contains the query (case
// insensitive), for the offline lookup screen.
func (db *DB) Search(query string, limit int) []Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	// Exact code matches (including VAG numbers) come first.
	out := db.Lookup(query)
	seen := map[string]bool{}
	for _, m := range out {
		seen[m.Family+"/"+m.Code] = true
	}
	for _, fam := range db.Families() {
		codes := make([]string, 0, len(db.families[fam]))
		for c := range db.families[fam] {
			codes = append(codes, c)
		}
		sort.Strings(codes)
		for _, c := range codes {
			t := db.families[fam][c]
			hit := strings.Contains(strings.ToLower(c), q)
			if !hit {
				for _, s := range t {
					if strings.Contains(strings.ToLower(s), q) {
						hit = true
						break
					}
				}
			}
			if hit && !seen[fam+"/"+c] {
				out = append(out, Match{Code: c, Family: fam, Desc: t})
				if limit > 0 && len(out) >= limit {
					return out
				}
			}
		}
	}
	return out
}

// Lookup finds a code in every family; five-digit VAG codes from 16384
// are also looked up as their SAE equivalent.
func (db *DB) Lookup(code string) []Match {
	code = strings.ToUpper(strings.TrimSpace(code))
	var out []Match
	for _, f := range db.Families() {
		if t, ok := db.families[f][code]; ok {
			out = append(out, Match{Code: code, Family: f, Desc: t})
		}
	}
	if n, err := strconv.Atoi(code); err == nil && len(code) == 5 && n >= 16384 && n <= 0xFFFF {
		if _, sae := VAGCode(uint16(n)); sae != "" {
			if t, fam, ok := db.Describe(sae); ok {
				out = append(out, Match{Code: code + " = " + sae, Family: fam, Desc: t})
			} else if len(out) == 0 {
				out = append(out, Match{Code: code + " = " + sae, Family: "vag", Desc: SystemOf(sae)})
			}
		}
	}
	return out
}

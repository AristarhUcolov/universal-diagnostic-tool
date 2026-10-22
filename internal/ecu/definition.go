package ecu

import (
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// Definition describes a manufacturer-specific control unit: how to
// connect, where its data lives and how to decode it. Definitions are
// JSON files so that anyone can add a control unit without touching Go
// code (see docs/ECU-DEFINITIONS.md).
type Definition struct {
	ID        string     `json:"id"`
	Extends   string     `json:"extends,omitempty"` // id of a definition this one is based on
	Name      string     `json:"name"`
	Vendor    string     `json:"vendor,omitempty"`
	Protocol  string     `json:"protocol"` // kwp2000, legacy, uds
	Profile   string     `json:"profile"`  // vehicle profile it belongs to
	Family    string     `json:"family,omitempty"`
	Status    string     `json:"status"` // verified, partial, unverified
	Vehicles  []string   `json:"vehicles,omitempty"`
	Sources   []string   `json:"sources,omitempty"`
	Notes     i18n.Text  `json:"notes,omitempty"`
	Link      LinkDef    `json:"link"`
	Match     MatchDef   `json:"match,omitempty"`
	Session   string     `json:"session,omitempty"`   // request sent after connecting
	KeepAlive string     `json:"keepalive,omitempty"` // request that keeps the session open
	Ident     []IdentDef `json:"ident,omitempty"`
	DTC       DTCDef     `json:"dtc"`
	Blocks    []BlockDef `json:"blocks,omitempty"`
	Actuators []ActDef   `json:"actuators,omitempty"`

	file string // where it was loaded from
}

// LinkDef holds connection parameters.
type LinkDef struct {
	Init    string `json:"init,omitempty"` // fast, slow, none
	Baud    int    `json:"baud,omitempty"`
	Target  string `json:"target,omitempty"`
	Tester  string `json:"tester,omitempty"`
	P3MinMs int    `json:"p3min_ms,omitempty"`
	TxID    string `json:"tx_id,omitempty"`
	RxID    string `json:"rx_id,omitempty"`
	Bitrate int    `json:"bitrate,omitempty"`
}

// MatchDef decides whether a connected control unit is this one.
type MatchDef struct {
	Ident    []string       `json:"ident,omitempty"`     // any substring in the identification
	Version  []string       `json:"version,omitempty"`   // legacy version byte (hex)
	BlockLen map[string]int `json:"block_len,omitempty"` // request -> answer length
	Fallback bool           `json:"fallback,omitempty"`  // use when nothing else matches
}

// IdentDef reads identification.
type IdentDef struct {
	Request string     `json:"request"`
	Skip    int        `json:"skip"`
	Fields  []FieldDef `json:"fields"`
}

// FieldDef is one identification field.
type FieldDef struct {
	Key  string    `json:"key"`
	Name i18n.Text `json:"name"`
	Len  int       `json:"len"`  // 0 = rest of the answer
	Type string    `json:"type"` // ascii (default), hex, cp866, u8
}

// DTCDef reads and clears trouble codes.
type DTCDef struct {
	Read   string   `json:"read"`
	Format string   `json:"format"` // kwp, uds, legacy
	Clear  []string `json:"clear,omitempty"`
}

// BlockDef is a data record (KWP2000 local identifier, UDS DID) or, for
// the legacy protocol, a list of parameter codes.
type BlockDef struct {
	ID      string     `json:"id"`
	Name    i18n.Text  `json:"name"`
	Request string     `json:"request,omitempty"`
	Skip    int        `json:"skip"`
	MinLen  int        `json:"min_len,omitempty"`
	Params  []ParamDef `json:"params"`
}

// ParamDef decodes one value.
type ParamDef struct {
	ID       string               `json:"id"`
	Name     i18n.Text            `json:"name"`
	Offset   int                  `json:"offset"`
	Type     string               `json:"type"` // u8 s8 u16le u16be s16le s16be u32le u32be bit flags ascii
	Bit      int                  `json:"bit,omitempty"`
	Code     string               `json:"code,omitempty"` // legacy parameter code
	Expr     string               `json:"expr,omitempty"`
	Unit     string               `json:"unit,omitempty"`
	Min      float64              `json:"min"`
	Max      float64              `json:"max"`
	Digits   int                  `json:"digits"`
	Group    string               `json:"group,omitempty"`
	OBD      string               `json:"obd,omitempty"`
	Enum     map[string]i18n.Text `json:"enum,omitempty"`
	Flags    []i18n.Text          `json:"flags,omitempty"` // names of bits 0..7 for type "flags"
	Verified *bool                `json:"verified,omitempty"`
	Note     i18n.Text            `json:"note,omitempty"`

	expr *Expr
	code byte
}

// ActDef is an output test.
type ActDef struct {
	ID      string    `json:"id"`
	Name    i18n.Text `json:"name"`
	Kind    string    `json:"kind"` // toggle, pulse, value
	On      string    `json:"on,omitempty"`
	Off     string    `json:"off,omitempty"`
	Set     string    `json:"set,omitempty"`
	Min     float64   `json:"min,omitempty"`
	Max     float64   `json:"max,omitempty"`
	Unit    string    `json:"unit,omitempty"`
	Warning i18n.Text `json:"warning,omitempty"`
	// Condition: "engine_off" or "engine_running"; checked against the
	// engine speed before the test starts.
	Condition string `json:"condition,omitempty"`
	// TimeoutS: an output left on is handed back to the control unit
	// after this many seconds (default 60).
	TimeoutS int `json:"timeout_s,omitempty"`
}

// Size returns how many bytes the parameter type occupies.
func (p *ParamDef) Size() int {
	switch p.Type {
	case "u16le", "u16be", "s16le", "s16be":
		return 2
	case "u24be":
		return 3
	case "u32le", "u32be":
		return 4
	}
	return 1
}

// IsVerified reports whether the value was checked against a real unit.
func (p *ParamDef) IsVerified(def *Definition) bool {
	if p.Verified != nil {
		return *p.Verified
	}
	return def.Status == "verified"
}

// prepare compiles formulas and parses codes.
func (d *Definition) prepare() error {
	if d.ID == "" || d.Protocol == "" {
		return fmt.Errorf("definition needs id and protocol")
	}
	seen := map[string]bool{}
	for bi := range d.Blocks {
		b := &d.Blocks[bi]
		for pi := range b.Params {
			p := &b.Params[pi]
			if p.ID == "" {
				return fmt.Errorf("%s: block %s has a parameter without id", d.ID, b.ID)
			}
			if seen[p.ID] {
				return fmt.Errorf("%s: duplicate parameter id %s", d.ID, p.ID)
			}
			seen[p.ID] = true
			if p.Type == "" {
				p.Type = "u8"
			}
			if p.Expr != "" {
				e, err := Compile(p.Expr)
				if err != nil {
					return fmt.Errorf("%s/%s: %w", d.ID, p.ID, err)
				}
				p.expr = e
			}
			if p.Code != "" {
				v, err := parseByte(p.Code)
				if err != nil {
					return fmt.Errorf("%s/%s: code: %w", d.ID, p.ID, err)
				}
				p.code = v
			}
		}
	}
	return nil
}

func parseByte(s string) (byte, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"))
	v, err := strconv.ParseUint(s, 16, 8)
	return byte(v), err
}

// ParseAddr parses a hex address such as "0x10" or "7E0".
func ParseAddr(s string) (uint32, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"))
	v, err := strconv.ParseUint(s, 16, 32)
	return uint32(v), err
}

// Decode extracts the value of p from block data (after the header).
func (p *ParamDef) Decode(data []byte) Value {
	v := Value{ID: p.ID}
	n := p.Size()
	if p.Type == "ascii" {
		n = int(p.Max)
		if n <= 0 || p.Offset+n > len(data) {
			n = len(data) - p.Offset
		}
	}
	if p.Offset < 0 || p.Offset+n > len(data) || n <= 0 {
		return v
	}
	raw := data[p.Offset : p.Offset+n]
	v.Raw = hexutil.Format(raw)
	var x float64
	switch p.Type {
	case "u8", "bit", "flags":
		x = float64(raw[0])
	case "s8":
		x = float64(int8(raw[0]))
	case "u16le":
		x = float64(binary.LittleEndian.Uint16(raw))
	case "u16be":
		x = float64(binary.BigEndian.Uint16(raw))
	case "s16le":
		x = float64(int16(binary.LittleEndian.Uint16(raw)))
	case "s16be":
		x = float64(int16(binary.BigEndian.Uint16(raw)))
	case "u24be":
		x = float64(uint32(raw[0])<<16 | uint32(raw[1])<<8 | uint32(raw[2]))
	case "u32le":
		x = float64(binary.LittleEndian.Uint32(raw))
	case "u32be":
		x = float64(binary.BigEndian.Uint32(raw))
	case "ascii":
		v.Text = hexutil.ASCII(raw)
		v.OK = true
		return v
	default:
		return v
	}
	switch p.Type {
	case "bit":
		if int(raw[0])&(1<<p.Bit) != 0 {
			v.V = 1
		}
		v.OK = true
		return v
	case "flags":
		v.V = x
		v.OK = true
		var en, ru []string
		for i, name := range p.Flags {
			if i < 8 && raw[0]&(1<<i) != 0 && !name.Empty() {
				en = append(en, name.Get("en"))
				ru = append(ru, name.Get("ru"))
			}
		}
		if len(en) == 0 {
			v.L = i18n.T("—", "—")
		} else {
			v.L = i18n.T(strings.Join(en, ", "), strings.Join(ru, ", "))
		}
		v.Text = v.L.Get("en")
		return v
	}
	val, err := p.expr.Eval(map[string]float64{"x": x})
	if err != nil || math.IsNaN(val) || math.IsInf(val, 0) {
		return v
	}
	v.V, v.OK = val, true
	if len(p.Enum) > 0 {
		if t, ok := p.Enum[strconv.Itoa(int(x))]; ok {
			v.L = t
			v.Text = t.Get("en")
		}
	}
	return v
}

// Param converts the definition into the user-facing description.
func (p *ParamDef) Param(def *Definition) Param {
	kind := "num"
	switch p.Type {
	case "bit":
		kind = "bool"
	case "flags", "ascii":
		kind = "text"
	}
	max := p.Max
	if kind == "bool" {
		max = 1
	}
	return Param{ID: p.ID, Name: p.Name, Unit: p.Unit, Group: p.Group, Min: p.Min, Max: max,
		Kind: kind, Digits: p.Digits, OBD: p.OBD, Verified: p.IsVerified(def), Note: p.Note}
}

//go:embed defs/*.json
var builtinFS embed.FS

// Library holds all known definitions.
type Library struct {
	mu   sync.RWMutex
	defs map[string]*Definition
	raw  map[string][]byte // source JSON, for "extends"
	errs []string
}

var (
	libOnce sync.Once
	lib     *Library
)

// Builtin returns the library with the embedded definitions.
func Builtin() *Library {
	libOnce.Do(func() {
		lib = &Library{defs: map[string]*Definition{}, raw: map[string][]byte{}}
		files := map[string][]byte{}
		_ = fs.WalkDir(builtinFS, "defs", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
				return nil
			}
			if data, err := builtinFS.ReadFile(path); err == nil {
				files["builtin:"+path] = data
			}
			return nil
		})
		for _, err := range lib.addAll(files) {
			lib.errs = append(lib.errs, err.Error())
		}
	})
	return lib
}

// addAll loads several files, resolving "extends" chains in any order.
func (l *Library) addAll(files map[string][]byte) []error {
	var errs []error
	pending := map[string][]byte{}
	for f, data := range files {
		pending[f] = data
	}
	for len(pending) > 0 {
		progress := false
		names := make([]string, 0, len(pending))
		for f := range pending {
			names = append(names, f)
		}
		sort.Strings(names)
		for _, f := range names {
			var head struct {
				Extends string `json:"extends"`
			}
			if err := json.Unmarshal(pending[f], &head); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", f, err))
				delete(pending, f)
				progress = true
				continue
			}
			l.mu.RLock()
			_, baseReady := l.raw[head.Extends]
			l.mu.RUnlock()
			if head.Extends != "" && !baseReady {
				continue
			}
			if err := l.add(pending[f], f); err != nil {
				errs = append(errs, err)
			}
			delete(pending, f)
			progress = true
		}
		if !progress {
			for f := range pending {
				errs = append(errs, fmt.Errorf("%s: base definition not found", f))
			}
			break
		}
	}
	return errs
}

func (l *Library) add(data []byte, file string) error {
	var head struct {
		Extends string `json:"extends"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if head.Extends != "" {
		l.mu.RLock()
		base, ok := l.raw[head.Extends]
		l.mu.RUnlock()
		if !ok {
			return fmt.Errorf("%s: base definition %q not found", file, head.Extends)
		}
		merged, err := extend(base, data)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		data = merged
	}
	var d Definition
	if err := json.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if err := d.prepare(); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	d.file = file
	l.mu.Lock()
	l.defs[d.ID] = &d
	l.raw[d.ID] = data // merged, so that definitions can extend this one
	l.mu.Unlock()
	return nil
}

// extend applies a definition to the one it extends. Every top-level
// field the child names replaces the base's field as a whole: its blocks
// or output tests are never mixed with the base's element by element,
// which would hand a parameter the formula of whatever stood at the same
// place in the base. The base's identification rules are not inherited.
func extend(base, child []byte) ([]byte, error) {
	var b, c map[string]json.RawMessage
	if err := json.Unmarshal(base, &b); err != nil {
		return nil, fmt.Errorf("base: %w", err)
	}
	if err := json.Unmarshal(child, &c); err != nil {
		return nil, err
	}
	delete(b, "match")
	for k, v := range c {
		b[k] = v
	}
	return json.Marshal(b)
}

// LoadDir adds user definitions from a directory (they override built-in
// ones with the same id).
func (l *Library) LoadDir(dir string) []error {
	files := map[string][]byte{}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var errs []error
	for _, f := range paths {
		data, err := os.ReadFile(f)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		files[f] = data
	}
	errs = append(errs, l.addAll(files)...)
	l.mu.Lock()
	for _, err := range errs {
		l.errs = append(l.errs, err.Error())
	}
	l.mu.Unlock()
	return errs
}

// Errors lists definitions that failed to load.
func (l *Library) Errors() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]string(nil), l.errs...)
}

// Get returns a definition by id.
func (l *Library) Get(id string) (*Definition, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	d, ok := l.defs[id]
	return d, ok
}

// ByProtocol returns definitions for a protocol and profile (empty
// profile = any), sorted by id.
func (l *Library) ByProtocol(protocol, profile string) []*Definition {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var out []*Definition
	for _, d := range l.defs {
		if d.Protocol == protocol && (profile == "" || d.Profile == profile) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// All returns every definition sorted by id.
func (l *Library) All() []*Definition {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*Definition, 0, len(l.defs))
	for _, d := range l.defs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Summary is the definition list entry shown in the user interface.
type Summary struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Vendor   string    `json:"vendor,omitempty"`
	Protocol string    `json:"protocol"`
	Profile  string    `json:"profile"`
	Status   string    `json:"status"`
	Vehicles []string  `json:"vehicles,omitempty"`
	Notes    i18n.Text `json:"notes,omitempty"`
	Params   int       `json:"params"`
	Sources  []string  `json:"sources,omitempty"`
}

// Summaries lists all definitions.
func (l *Library) Summaries() []Summary {
	var out []Summary
	for _, d := range l.All() {
		n := 0
		for _, b := range d.Blocks {
			n += len(b.Params)
		}
		out = append(out, Summary{ID: d.ID, Name: d.Name, Vendor: d.Vendor, Protocol: d.Protocol, Profile: d.Profile,
			Status: d.Status, Vehicles: d.Vehicles, Notes: d.Notes, Params: n, Sources: d.Sources})
	}
	return out
}

// Fill substitutes {expr} and {expr:u16be} placeholders in a request
// template with values computed from v, then parses the hex.
func Fill(template string, v float64) ([]byte, error) {
	var sb strings.Builder
	rest := template
	for {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			sb.WriteString(rest)
			break
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			return nil, fmt.Errorf("unclosed { in %q", template)
		}
		sb.WriteString(rest[:i])
		inner := rest[i+1 : i+j]
		rest = rest[i+j+1:]
		format := "u8"
		if k := strings.LastIndexByte(inner, ':'); k >= 0 {
			inner, format = inner[:k], inner[k+1:]
		}
		e, err := Compile(inner)
		if err != nil {
			return nil, err
		}
		x, err := e.Eval(map[string]float64{"v": v, "x": v})
		if err != nil {
			return nil, err
		}
		n := uint64(math.Round(math.Max(0, x)))
		switch format {
		case "u16be":
			fmt.Fprintf(&sb, " %02X %02X ", byte(n>>8), byte(n))
		case "u16le":
			fmt.Fprintf(&sb, " %02X %02X ", byte(n), byte(n>>8))
		default:
			if n > 255 {
				n = 255
			}
			fmt.Fprintf(&sb, " %02X ", byte(n))
		}
	}
	return hexutil.Parse(sb.String())
}

package ecu

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/legacy"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/uds"
)

type paramRef struct {
	block *BlockDef
	param *ParamDef
}

// DefHandler runs a KWP2000 or UDS control unit from its definition.
// With a nil definition it offers what the standard services provide
// (identification, trouble codes, raw requests).
type DefHandler struct {
	ch     diag.Channel
	def    *Definition
	proto  string // kwp2000, uds
	name   string
	mu     sync.Mutex
	t      diag.Timing
	params []Param
	index  map[string]paramRef
	active map[string]*ActDef
}

// NewDefHandler creates a handler for a KWP2000 ("kwp2000") or UDS
// ("uds") control unit.
func NewDefHandler(ch diag.Channel, def *Definition, proto, name string) *DefHandler {
	h := &DefHandler{ch: ch, def: def, proto: proto, name: name, t: diag.DefaultTiming(),
		index: map[string]paramRef{}, active: map[string]*ActDef{}}
	if def != nil {
		if h.name == "" {
			h.name = def.Name
		}
		for bi := range def.Blocks {
			b := &def.Blocks[bi]
			for pi := range b.Params {
				p := &b.Params[pi]
				h.params = append(h.params, p.Param(def))
				h.index[p.ID] = paramRef{block: b, param: p}
			}
		}
	}
	return h
}

// Definition returns the definition in use (may be nil).
func (h *DefHandler) Definition() *Definition { return h.def }

func (h *DefHandler) Describe() Description {
	info := h.ch.Info()
	d := Description{ID: "ecu", Name: h.name, Protocol: info.Protocol, Address: fmt.Sprintf("0x%02X", info.Target),
		Status: "generic", Capabilities: []string{CapDTC, CapClear, CapRaw, CapBlocks}}
	if info.CAN {
		d.Address = fmt.Sprintf("%X", info.Target)
	}
	if h.proto == "kwp2000" {
		d.Family = dtc.FamilyRuKWP
	}
	if h.def != nil {
		d.Definition, d.Status, d.Notes, d.Family = h.def.ID, h.def.Status, h.def.Notes, h.def.Family
		if len(h.params) > 0 {
			d.Capabilities = append(d.Capabilities, CapLive)
		}
		if len(h.def.Actuators) > 0 {
			d.Capabilities = append(d.Capabilities, CapActuators)
		}
	}
	return d
}

func (h *DefHandler) request(ctx context.Context, req []byte) ([]byte, error) {
	return diag.Request(ctx, h.ch, req, h.t)
}

func (h *DefHandler) Identify(ctx context.Context) ([]InfoItem, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	info := h.ch.Info()
	items := []InfoItem{{Key: "protocol", Name: i18n.T("Protocol", "Протокол"), Value: info.Protocol}}
	if len(info.KeyBytes) == 2 {
		items = append(items, InfoItem{Key: "keybytes", Name: i18n.T("Key bytes", "Ключевые байты"), Value: hexutil.Format(info.KeyBytes)})
	}
	if h.def != nil {
		items = append(items, InfoItem{Key: "definition", Name: i18n.T("Control unit description", "Описание блока"), Value: h.def.Name + " [" + h.def.Status + "]"})
	}
	var idents []IdentDef
	if h.def != nil {
		idents = h.def.Ident
	}
	if len(idents) == 0 && h.proto == "uds" {
		for _, did := range uds.IdentDIDs {
			data, err := h.request(ctx, []byte{0x22, byte(did >> 8), byte(did)})
			if err != nil {
				continue
			}
			items = append(items, InfoItem{Key: fmt.Sprintf("did.%04X", did), Name: didName(did), Value: textOrHex(data[3:])})
		}
		return items, nil
	}
	if len(idents) == 0 && h.proto == "kwp2000" {
		idents = []IdentDef{{Request: "1A 80", Skip: 2}}
	}
	for _, id := range idents {
		req, err := hexutil.Parse(id.Request)
		if err != nil {
			continue
		}
		resp, err := h.request(ctx, req)
		if err != nil {
			items = append(items, InfoItem{Key: "ident.error", Name: i18n.T("Identification", "Идентификация"), Value: err.Error()})
			continue
		}
		if id.Skip > len(resp) {
			continue
		}
		data := resp[id.Skip:]
		if len(id.Fields) == 0 {
			items = append(items, InfoItem{Key: "ident", Name: i18n.T("Identification", "Идентификация"), Value: textOrHex(data)})
			continue
		}
		off := 0
		for _, f := range id.Fields {
			if off >= len(data) {
				break
			}
			end := len(data)
			if f.Len > 0 && off+f.Len < end {
				end = off + f.Len
			}
			chunk := data[off:end]
			off = end
			var v string
			switch f.Type {
			case "hex":
				v = hexutil.Format(chunk)
			case "cp866":
				v = strings.TrimSpace(legacy.CP866(chunk))
			case "u8":
				v = fmt.Sprintf("%d", chunk[0])
			default:
				v = hexutil.ASCII(chunk)
			}
			if v != "" {
				items = append(items, InfoItem{Key: f.Key, Name: f.Name, Value: v})
			}
		}
	}
	return items, nil
}

func didName(did uint16) i18n.Text {
	if n, ok := uds.DIDNames[did]; ok {
		return n
	}
	s := fmt.Sprintf("DID %04X", did)
	return i18n.T(s, s)
}

// textOrHex shows mostly printable data as text and the rest as hex.
func textOrHex(b []byte) string {
	printable := 0
	for _, c := range b {
		if c >= 0x20 && c < 0x7F {
			printable++
		}
	}
	if len(b) > 0 && printable*10 >= len(b)*8 {
		return hexutil.ASCII(b)
	}
	return hexutil.Format(b)
}

func (h *DefHandler) ReadDTC(ctx context.Context) ([]dtc.DTC, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	read, format := "", ""
	if h.def != nil {
		read, format = h.def.DTC.Read, h.def.DTC.Format
	}
	if read == "" {
		switch h.proto {
		case "uds":
			read, format = "19 02 FF", "uds"
		default:
			read, format = "18 00 FF 00", "kwp"
		}
	}
	req, err := hexutil.Parse(read)
	if err != nil {
		return nil, err
	}
	resp, err := h.request(ctx, req)
	if err != nil {
		return nil, err
	}
	codes, err := ParseDTCAnswer(format, resp)
	if err != nil {
		return nil, err
	}
	fam := h.Describe().Family
	db := dtc.Default()
	for i := range codes {
		codes[i].Source = h.name
		db.Fill(&codes[i], fam)
	}
	return codes, nil
}

// ParseDTCAnswer decodes a trouble code answer in the given format.
func ParseDTCAnswer(format string, resp []byte) ([]dtc.DTC, error) {
	var out []dtc.DTC
	switch format {
	case "kwp":
		// 58 n [hi lo status]...
		if len(resp) < 2 || resp[0] != 0x58 {
			return nil, fmt.Errorf("unexpected DTC answer % X", resp)
		}
		n := int(resp[1])
		for i := 2; i+2 < len(resp) && len(out) < n; i += 3 {
			d := dtc.DTC{Code: dtc.SAE(resp[i], resp[i+1]), Raw: hexutil.Format(resp[i : i+3]), Protocol: "KWP2000"}
			dtc.ApplyKWPStatus(&d, resp[i+2])
			out = append(out, d)
		}
	case "uds":
		if len(resp) < 3 || resp[0] != 0x59 {
			return nil, fmt.Errorf("unexpected DTC answer % X", resp)
		}
		for i := 3; i+3 < len(resp); i += 4 {
			out = append(out, uds.DecodeDTC(resp[i:i+3], resp[i+3]))
		}
	default:
		return nil, fmt.Errorf("unknown DTC format %q", format)
	}
	return out, nil
}

func (h *DefHandler) ClearDTC(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	clears := []string{}
	if h.def != nil {
		clears = h.def.DTC.Clear
	}
	if len(clears) == 0 {
		if h.proto == "uds" {
			clears = []string{"14 FF FF FF"}
		} else {
			clears = []string{"14 FF 00"}
		}
	}
	for _, c := range clears {
		req, err := hexutil.Parse(c)
		if err != nil {
			return err
		}
		if _, err := h.request(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

func (h *DefHandler) Params() []Param { return h.params }

func (h *DefHandler) Read(ctx context.Context, ids []string) ([]Value, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	data := map[*BlockDef][]byte{}
	var firstErr error
	for _, id := range ids {
		ref, ok := h.index[id]
		if !ok {
			continue
		}
		if _, done := data[ref.block]; done {
			continue
		}
		req, err := hexutil.Parse(ref.block.Request)
		if err != nil {
			continue
		}
		resp, err := h.request(ctx, req)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			data[ref.block] = nil
			continue
		}
		if ref.block.Skip <= len(resp) {
			data[ref.block] = resp[ref.block.Skip:]
		}
	}
	out := make([]Value, 0, len(ids))
	got := false
	for _, id := range ids {
		ref, ok := h.index[id]
		if !ok {
			out = append(out, Value{ID: id})
			continue
		}
		v := ref.param.Decode(data[ref.block])
		got = got || v.OK
		out = append(out, v)
	}
	if !got && firstErr != nil {
		return out, firstErr
	}
	return out, nil
}

func (h *DefHandler) Actuators() []Actuator {
	if h.def == nil {
		return nil
	}
	out := make([]Actuator, 0, len(h.def.Actuators))
	for _, a := range h.def.Actuators {
		out = append(out, Actuator{ID: a.ID, Name: a.Name, Kind: a.Kind, Min: a.Min, Max: a.Max, Unit: a.Unit, Warning: a.Warning})
	}
	return out
}

// Actuate runs an output test: action "on"/"off" for toggles, "pulse"
// for pulses, "set" (with value) or "off" for adjustments.
func (h *DefHandler) Actuate(ctx context.Context, id, action string, value float64) (string, error) {
	if h.def == nil {
		return "", fmt.Errorf("no output tests known for this control unit")
	}
	var act *ActDef
	for i := range h.def.Actuators {
		if h.def.Actuators[i].ID == id {
			act = &h.def.Actuators[i]
		}
	}
	if act == nil {
		return "", fmt.Errorf("unknown output test %q", id)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	send := func(tmpl string) ([]byte, error) {
		req, err := Fill(tmpl, value)
		if err != nil {
			return nil, err
		}
		return h.request(ctx, req)
	}
	switch {
	case act.Kind == "pulse" || action == "pulse":
		if _, err := send(act.On); err != nil {
			return "", err
		}
		ms := act.Max
		if ms <= 0 || ms > 1000 {
			ms = 300
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		resp, err := send(act.Off)
		return hexutil.Format(resp), err
	case action == "on":
		resp, err := send(act.On)
		if err == nil {
			h.active[id] = act
		}
		return hexutil.Format(resp), err
	case action == "set":
		if value < act.Min || (act.Max > act.Min && value > act.Max) {
			return "", fmt.Errorf("value %v outside %v..%v", value, act.Min, act.Max)
		}
		resp, err := send(act.Set)
		if err == nil {
			h.active[id] = act
		}
		return hexutil.Format(resp), err
	case action == "off":
		resp, err := send(act.Off)
		delete(h.active, id)
		return hexutil.Format(resp), err
	}
	return "", fmt.Errorf("unknown action %q", action)
}

func (h *DefHandler) Raw(ctx context.Context, req []byte) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// A negative answer is returned together with the error so that the
	// terminal can show the raw bytes.
	m, err := diag.RequestMsg(ctx, h.ch, req, h.t)
	return m.Data, err
}

func (h *DefHandler) KeepAlive(ctx context.Context) error {
	ka := ""
	if h.def != nil {
		ka = h.def.KeepAlive
	}
	if ka == "" {
		if h.proto == "uds" {
			ka = "3E 00"
		} else {
			ka = "3E 01"
		}
	}
	req, err := hexutil.Parse(ka)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.request(ctx, req)
	return err
}

// Blocks lists the raw records offered by the record explorer.
func (h *DefHandler) Blocks() []string {
	seen := map[string]bool{}
	var out []string
	if h.def != nil {
		for _, b := range h.def.Blocks {
			if b.Request != "" && !seen[b.Request] {
				seen[b.Request] = true
				out = append(out, b.Request)
			}
		}
	}
	extra := []string{"21 01", "21 02", "21 03", "21 A0"}
	if h.proto == "uds" {
		extra = []string{"22 F1 90", "22 F1 87", "22 F1 8C"}
	}
	for _, e := range extra {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

// ReadBlock sends a record request and returns the full answer.
func (h *DefHandler) ReadBlock(ctx context.Context, id string) ([]byte, error) {
	req, err := hexutil.Parse(id)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.request(ctx, req)
}

func (h *DefHandler) Close() error {
	h.mu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	for id, act := range h.active {
		if req, err := Fill(act.Off, 0); err == nil {
			_, _ = h.request(ctx, req)
		}
		delete(h.active, id)
	}
	cancel()
	h.mu.Unlock()
	return h.ch.Close()
}

// LegacyHandler runs a controller on the legacy Russian protocol from its
// definition.
type LegacyHandler struct {
	link    *legacy.Link
	def     *Definition
	version byte
	mu      sync.Mutex
	params  []Param
	index   map[string]*ParamDef
	adapter string
}

// NewLegacyHandler creates the handler; version is the answer to the
// version request.
func NewLegacyHandler(link *legacy.Link, def *Definition, version byte, adapter string) *LegacyHandler {
	h := &LegacyHandler{link: link, def: def, version: version, index: map[string]*ParamDef{}, adapter: adapter}
	if def != nil {
		for bi := range def.Blocks {
			for pi := range def.Blocks[bi].Params {
				p := &def.Blocks[bi].Params[pi]
				h.params = append(h.params, p.Param(def))
				h.index[p.ID] = p
			}
		}
	}
	return h
}

func (h *LegacyHandler) Describe() Description {
	d := Description{ID: "ecu", Name: fmt.Sprintf("Legacy controller (version 0x%02X)", h.version),
		Protocol: "K-Line legacy protocol (Январь-4 / Микас), 9600 baud", Address: "-",
		Family: dtc.FamilyGazMikas, Status: "generic", Capabilities: []string{CapDTC, CapClear, CapRaw}}
	if h.def != nil {
		d.Name, d.Definition, d.Status, d.Notes, d.Family = h.def.Name, h.def.ID, h.def.Status, h.def.Notes, h.def.Family
		if len(h.params) > 0 {
			d.Capabilities = append(d.Capabilities, CapLive)
		}
	}
	return d
}

func (h *LegacyHandler) Identify(ctx context.Context) ([]InfoItem, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	items := []InfoItem{
		{Key: "protocol", Name: i18n.T("Protocol", "Протокол"), Value: "K-Line legacy (9600 baud, 0x0D framed)"},
		{Key: "version", Name: i18n.T("Controller version code", "Код версии ЭБУ"), Value: fmt.Sprintf("0x%02X%s", h.version, versionName(h.version))},
	}
	if h.def != nil {
		items = append(items, InfoItem{Key: "definition", Name: i18n.T("Control unit description", "Описание блока"), Value: h.def.Name + " [" + h.def.Status + "]"})
	}
	prog, data, err := h.link.Passport(ctx)
	if prog != "" {
		items = append(items, InfoItem{Key: "program", Name: i18n.T("Program passport", "Паспорт программы"), Value: prog})
	}
	if data != "" {
		items = append(items, InfoItem{Key: "data", Name: i18n.T("Calibration passport", "Паспорт калибровок"), Value: data})
	}
	if err != nil && prog == "" {
		items = append(items, InfoItem{Key: "passport.error", Name: i18n.T("Passport", "Паспорт"), Value: err.Error()})
	}
	return items, nil
}

func versionName(v byte) string {
	switch v {
	case 0x09:
		return " (Микас-5.4)"
	case 0x0A:
		return " (Микас-7.1)"
	}
	return ""
}

func (h *LegacyHandler) ReadDTC(ctx context.Context) ([]dtc.DTC, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	codes, err := h.link.ReadErrors(ctx)
	if err != nil {
		return nil, err
	}
	fam := dtc.FamilyGazMikas
	if h.def != nil && h.def.Family != "" {
		fam = h.def.Family
	}
	db := dtc.Default()
	out := make([]dtc.DTC, 0, len(codes))
	for _, c := range codes {
		d := dtc.DTC{Code: fmt.Sprintf("%03d", c), Raw: fmt.Sprintf("%02X", c), Confirmed: true,
			Protocol: "Микас / Январь-4", Source: h.Describe().Name}
		db.Fill(&d, fam)
		out = append(out, d)
	}
	return out, nil
}

func (h *LegacyHandler) ClearDTC(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	clears := []string{"62 0E 08", "62 0E 00"}
	if h.def != nil && len(h.def.DTC.Clear) > 0 {
		clears = h.def.DTC.Clear
	}
	for _, c := range clears {
		req, err := hexutil.Parse(c)
		if err != nil {
			return err
		}
		resp, err := h.link.Transact(ctx, req)
		if err != nil {
			return err
		}
		if len(resp) > 0 && resp[0] != 0x00 {
			return fmt.Errorf("controller refused %s (answer % X)", c, resp)
		}
	}
	return nil
}

func (h *LegacyHandler) Params() []Param { return h.params }

func (h *LegacyHandler) Read(ctx context.Context, ids []string) ([]Value, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var codes []byte
	var sizes []int
	have := map[byte]bool{}
	for _, id := range ids {
		p, ok := h.index[id]
		if !ok || have[p.code] {
			continue
		}
		have[p.code] = true
		codes = append(codes, p.code)
		sizes = append(sizes, p.Size())
	}
	raw := map[byte][]byte{}
	for i := 0; i < len(codes); i += 12 {
		end := i + 12
		if end > len(codes) {
			end = len(codes)
		}
		resp, err := h.link.ReadParams(ctx, codes[i:end], sizes[i:end])
		if err != nil {
			return nil, err
		}
		off := 0
		for j := i; j < end; j++ {
			if off+sizes[j] <= len(resp) {
				raw[codes[j]] = resp[off : off+sizes[j]]
			}
			off += sizes[j]
		}
	}
	out := make([]Value, 0, len(ids))
	for _, id := range ids {
		p, ok := h.index[id]
		if !ok {
			out = append(out, Value{ID: id})
			continue
		}
		out = append(out, p.Decode(raw[p.code]))
	}
	return out, nil
}

func (h *LegacyHandler) Actuators() []Actuator { return nil }

func (h *LegacyHandler) Actuate(ctx context.Context, id, action string, value float64) (string, error) {
	return "", fmt.Errorf("no output tests are documented for this protocol")
}

func (h *LegacyHandler) Raw(ctx context.Context, req []byte) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.link.Transact(ctx, req)
}

func (h *LegacyHandler) KeepAlive(ctx context.Context) error { return nil }

func (h *LegacyHandler) Close() error { return nil }

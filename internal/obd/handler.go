package obd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// Handler presents the OBD-II system of the vehicle (all emission
// related control units answering the functional address) as one
// ecu.Handler.
type Handler struct {
	c         *Client
	mu        sync.Mutex
	supported map[uint32][]byte
	params    []ecu.Param
	pidSrc    map[byte]uint32 // control unit that supplies each PID
	name      string
}

// NewHandler discovers the supported PIDs and returns the handler.
func NewHandler(ctx context.Context, ch diag.Channel) (*Handler, error) {
	h := &Handler{c: NewClient(ch), pidSrc: map[byte]uint32{}, name: "OBD-II / EOBD"}
	sup, err := h.c.Supported(ctx)
	if err != nil {
		return nil, err
	}
	h.supported = sup
	// The engine controller has the lowest response address (7E8, 0x10);
	// it supplies a PID whenever it supports it.
	for _, src := range sortedKeys(sup) {
		for _, pid := range sup[src] {
			if _, taken := h.pidSrc[pid]; !taken {
				h.pidSrc[pid] = src
			}
		}
	}
	h.params = h.buildParams()
	return h, nil
}

// ControlUnits returns the response addresses of the answering units.
func (h *Handler) ControlUnits() []uint32 { return sortedKeys(h.supported) }

func (h *Handler) buildParams() []ecu.Param {
	var pids []int
	for pid := range h.pidSrc {
		pids = append(pids, int(pid))
	}
	sort.Ints(pids)
	var out []ecu.Param
	for _, p := range pids {
		def, ok := PIDs[byte(p)]
		if !ok {
			continue
		}
		for _, v := range def.Values {
			kind := "num"
			if v.Text != nil {
				kind = "text"
			}
			if v.Min == 0 && v.Max == 1 && v.Unit == "" {
				kind = "bool"
			}
			out = append(out, ecu.Param{
				ID: ParamID(byte(p), v.Key), Name: v.Name, Unit: v.Unit, Group: v.Group,
				Min: v.Min, Max: v.Max, Kind: kind, Digits: v.Digits,
				OBD: fmt.Sprintf("%02X", p), Verified: true,
			})
		}
	}
	return out
}

func (h *Handler) Describe() ecu.Description {
	info := h.c.Ch.Info()
	var units []string
	for _, u := range h.ControlUnits() {
		units = append(units, fmt.Sprintf("%X", u))
	}
	return ecu.Description{
		ID: "obd", Name: h.name, Protocol: info.Protocol,
		Address: strings.Join(units, ", "), Family: dtc.FamilySAE, Status: "generic",
		Capabilities: []string{ecu.CapDTC, ecu.CapClear, ecu.CapLive, ecu.CapRaw, ecu.CapFreeze, ecu.CapReadiness},
	}
}

func (h *Handler) Identify(ctx context.Context) ([]ecu.InfoItem, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var items []ecu.InfoItem
	add := func(key string, name i18n.Text, v string) {
		if v != "" {
			items = append(items, ecu.InfoItem{Key: key, Name: name, Value: v})
		}
	}
	add("protocol", i18n.T("Protocol", "Протокол"), h.c.Ch.Info().Protocol)
	var units []string
	for _, u := range h.ControlUnits() {
		units = append(units, fmt.Sprintf("%X (%d PIDs)", u, len(h.supported[u])))
	}
	add("units", i18n.T("Control units answering", "Отвечающие блоки"), strings.Join(units, ", "))
	if vin, err := h.c.VIN(ctx); err == nil {
		add("vin", i18n.T("VIN", "VIN"), vin)
	}
	if cal, err := h.c.Strings(ctx, 0x04, 16); err == nil {
		for _, src := range sortedKeys(cal) {
			add("calid."+fmt.Sprintf("%X", src), i18n.T("Calibration ID "+fmt.Sprintf("%X", src), "Калибровка "+fmt.Sprintf("%X", src)), strings.Join(cal[src], ", "))
		}
	}
	if names, err := h.c.Strings(ctx, 0x0A, 20); err == nil {
		for _, src := range sortedKeys(names) {
			add("ecuname."+fmt.Sprintf("%X", src), i18n.T("Control unit name "+fmt.Sprintf("%X", src), "Имя блока "+fmt.Sprintf("%X", src)), strings.Join(names[src], ", "))
		}
	}
	if data, err := h.c.ReadPIDs(ctx, []byte{0x1C, 0x51}); err == nil {
		for _, src := range sortedKeys(data) {
			if d, ok := data[src][0x1C]; ok && len(d) >= 1 {
				add("standard", i18n.T("OBD standard", "Стандарт OBD"), obdStandard(d[0]))
			}
			if d, ok := data[src][0x51]; ok && len(d) >= 1 {
				t := fuelType(d[0])
				add("fuel", i18n.T("Fuel type", "Тип топлива"), t.Get("en")+" / "+t.Get("ru"))
			}
			break
		}
	}
	return items, nil
}

func (h *Handler) ReadDTC(ctx context.Context) ([]dtc.DTC, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var all []dtc.DTC
	stored, err := h.c.ReadDTCs(ctx, 0x03)
	if err != nil && !diag.IsTimeout(err) {
		return nil, err
	}
	all = append(all, stored...)
	if pending, err := h.c.ReadDTCs(ctx, 0x07); err == nil {
		all = mergeDTC(all, pending)
	}
	if perm, err := h.c.ReadDTCs(ctx, 0x0A); err == nil {
		all = mergeDTC(all, perm)
	}
	db := dtc.Default()
	for i := range all {
		db.Fill(&all[i])
	}
	return all, nil
}

// mergeDTC adds codes from b to a, combining the flags of codes that
// appear in both.
func mergeDTC(a, b []dtc.DTC) []dtc.DTC {
	for _, d := range b {
		found := false
		for i := range a {
			if a[i].Code == d.Code && a[i].Source == d.Source {
				a[i].Pending = a[i].Pending || d.Pending
				a[i].Permanent = a[i].Permanent || d.Permanent
				a[i].Flags = append(a[i].Flags, d.Flags...)
				found = true
				break
			}
		}
		if !found {
			a = append(a, d)
		}
	}
	return a
}

func (h *Handler) ClearDTC(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.c.Clear(ctx)
}

func (h *Handler) Params() []ecu.Param { return h.params }

func (h *Handler) Read(ctx context.Context, ids []string) ([]ecu.Value, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Group the requested values by PID.
	var pids []byte
	seen := map[byte]bool{}
	for _, id := range ids {
		pid, _, ok := ParseParamID(id)
		if !ok || seen[pid] {
			continue
		}
		if _, sup := h.pidSrc[pid]; !sup {
			continue
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	data, err := h.c.ReadPIDs(ctx, pids)
	if err != nil {
		return nil, err
	}
	var out []ecu.Value
	for _, id := range ids {
		pid, key, ok := ParseParamID(id)
		if !ok {
			continue
		}
		v := ecu.Value{ID: id}
		src := h.pidSrc[pid]
		d, have := data[src][pid]
		if !have {
			// Fall back to any unit that answered.
			for _, s := range sortedKeys(data) {
				if dd, ok := data[s][pid]; ok {
					d, have = dd, true
					break
				}
			}
		}
		if have {
			v = DecodeValue(id, pid, key, d)
		}
		out = append(out, v)
	}
	return out, nil
}

// DecodeValue converts the data of a PID into the value with the given
// key.
func DecodeValue(id string, pid byte, key string, d []byte) ecu.Value {
	v := ecu.Value{ID: id}
	def, ok := PIDs[pid]
	if !ok || len(d) < def.Bytes {
		return v
	}
	for _, vd := range def.Values {
		if vd.Key != key {
			continue
		}
		x, valid := vd.F(d)
		v.V, v.OK = x, valid
		if vd.Text != nil {
			v.L = vd.Text(d)
			v.Text = v.L.Get("en")
		}
		v.Raw = fmt.Sprintf("% X", d)
	}
	return v
}

func (h *Handler) FreezeFrame(ctx context.Context) ([]ecu.Value, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	msgs, err := h.c.Query(ctx, []byte{0x02, 0x02, 0x00})
	if err != nil {
		return nil, err
	}
	var out []ecu.Value
	for _, m := range msgs {
		if len(m.Data) >= 5 && (m.Data[3] != 0 || m.Data[4] != 0) {
			out = append(out, ecu.Value{ID: "freeze.dtc", Text: dtc.SAE(m.Data[3], m.Data[4]), OK: true})
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	pids := []byte{0x03, 0x04, 0x05, 0x06, 0x07, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11}
	for _, pid := range pids {
		if _, sup := h.pidSrc[pid]; !sup {
			continue
		}
		msgs, err := h.c.Query(ctx, []byte{0x02, pid, 0x00})
		if err != nil {
			continue
		}
		for _, m := range msgs {
			vals := map[byte][]byte{}
			ParseMode01(m.Data, vals)
			if d, ok := vals[pid]; ok {
				for _, vd := range PIDs[pid].Values {
					v := DecodeValue(ParamID(pid, vd.Key), pid, vd.Key, d)
					out = append(out, v)
				}
				break
			}
		}
	}
	return out, nil
}

// Monitor names (spark ignition / compression ignition).
var (
	monitorsCommon = []ecu.Monitor{
		{ID: "misfire", Name: i18n.T("Misfire", "Пропуски воспламенения")},
		{ID: "fuel", Name: i18n.T("Fuel system", "Топливная система")},
		{ID: "components", Name: i18n.T("Comprehensive components", "Компоненты")},
	}
	monitorsSpark = []i18n.Text{
		i18n.T("Catalyst", "Нейтрализатор"), i18n.T("Heated catalyst", "Подогрев нейтрализатора"),
		i18n.T("Evaporative system", "Система улавливания паров"), i18n.T("Secondary air system", "Подача вторичного воздуха"),
		i18n.T("A/C refrigerant", "Хладагент кондиционера"), i18n.T("Oxygen sensor", "Датчик кислорода"),
		i18n.T("Oxygen sensor heater", "Нагреватель датчика кислорода"), i18n.T("EGR / VVT system", "Система EGR / VVT"),
	}
	monitorsDiesel = []i18n.Text{
		i18n.T("NMHC catalyst", "Катализатор NMHC"), i18n.T("NOx / SCR aftertreatment", "Нейтрализация NOx / SCR"),
		{}, i18n.T("Boost pressure", "Давление наддува"), {}, i18n.T("Exhaust gas sensor", "Датчик ОГ"),
		i18n.T("Particulate filter", "Сажевый фильтр"), i18n.T("EGR / VVT system", "Система EGR / VVT"),
	}
)

// Readiness decodes PID 01 into the monitor list.
func (h *Handler) Readiness(ctx context.Context) (bool, int, []ecu.Monitor, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	data, err := h.c.ReadPIDs(ctx, []byte{0x01})
	if err != nil {
		return false, 0, nil, err
	}
	mil, count := false, 0
	var mons []ecu.Monitor
	for _, src := range sortedKeys(data) {
		d, ok := data[src][0x01]
		if !ok {
			continue
		}
		mil = mil || d[0]&0x80 != 0
		count += int(d[0] & 0x7F)
		if mons != nil {
			continue
		}
		for i, m := range monitorsCommon {
			if d[1]&(1<<i) != 0 {
				m.Complete = d[1]&(0x10<<i) == 0
				mons = append(mons, m)
			}
		}
		names := monitorsSpark
		if d[1]&0x08 != 0 {
			names = monitorsDiesel
		}
		for i, n := range names {
			if n.Empty() || d[2]&(1<<i) == 0 {
				continue
			}
			mons = append(mons, ecu.Monitor{ID: fmt.Sprintf("m%d", i), Name: n, Complete: d[3]&(1<<i) == 0})
		}
		if mons == nil {
			mons = []ecu.Monitor{}
		}
	}
	return mil, count, mons, nil
}

func (h *Handler) Actuators() []ecu.Actuator { return nil }

func (h *Handler) Actuate(ctx context.Context, id, action string, value float64) (string, error) {
	return "", fmt.Errorf("generic OBD-II has no output tests")
}

func (h *Handler) Raw(ctx context.Context, req []byte) ([]byte, error) {
	msgs, err := h.RawAll(ctx, req)
	if err != nil {
		return nil, err
	}
	return msgs[0].Data, nil
}

// RawAll sends a request and returns every answer.
func (h *Handler) RawAll(ctx context.Context, req []byte) ([]diag.Message, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.c.Ch.Info().Functional {
		return diag.RequestAll(ctx, h.c.Ch, req, h.c.First, h.c.Quiet)
	}
	m, err := diag.RequestMsg(ctx, h.c.Ch, req, diag.DefaultTiming())
	if err != nil {
		return nil, err
	}
	return []diag.Message{m}, nil
}

// KeepAlive keeps K-Line sessions open (ISO 9141-2 / 14230-4 drop the
// link after 5 s of silence). CAN needs nothing.
func (h *Handler) KeepAlive(ctx context.Context) error {
	if h.c.CAN {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.c.Query(ctx, []byte{0x01, 0x00})
	return err
}

// Close waits for a request in progress, then closes the channel.
func (h *Handler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.c.Ch.Close()
}

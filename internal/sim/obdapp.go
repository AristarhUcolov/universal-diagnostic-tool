package sim

import (
	"encoding/binary"
	"math"
	"sort"
	"sync"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
)

// OBDApp answers OBD-II (SAE J1979) requests and, on CAN, a useful
// subset of UDS (ISO 14229).
type OBDApp struct {
	Engine *Engine
	Name   string // control unit name for mode 09 PID 0A
	// Minor makes the unit behave like a secondary module (transmission):
	// few PIDs and no faults of its own.
	Minor bool
	UDS   bool // answer UDS services (CAN units)

	mu       sync.Mutex
	session  byte
	seed     uint16
	unlocked bool
	dids     map[uint16][]byte
}

// NewOBDApp creates an engine-type OBD application.
func NewOBDApp(e *Engine, name string) *OBDApp {
	return &OBDApp{Engine: e, Name: name, session: 0x01, dids: map[uint16][]byte{
		0xF187: []byte("8V0907115B  "),
		0xF18A: []byte("UDT-SIMULATOR"),
		0xF18C: []byte("SN0012345678"),
		0xF191: []byte("8V0907115   "),
		0xF194: []byte("SW-1.07.3"),
		0xF195: []byte("0107"),
		0xF197: []byte("SIM ENGINE 1.6"),
		0xF19E: []byte("EV_ECM16TFS"),
		0x0100: {0x00, 0x00, 0x00, 0x03}, // coding (writable demo DID)
	}}
}

func (a *OBDApp) pids() []byte {
	if a.Minor {
		return []byte{0x01, 0x05, 0x0C, 0x0D, 0x1C, 0x42}
	}
	return []byte{0x01, 0x03, 0x04, 0x05, 0x06, 0x07, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11,
		0x13, 0x14, 0x15, 0x1C, 0x1F, 0x21, 0x23, 0x2F, 0x30, 0x31, 0x33, 0x3C, 0x42, 0x43, 0x44, 0x45,
		0x46, 0x47, 0x49, 0x4C, 0x4D, 0x4E, 0x51, 0x5C}
}

func (a *OBDApp) supports(pid byte) bool {
	for _, p := range a.pids() {
		if p == pid {
			return true
		}
	}
	return false
}

// supportMask builds the 4-byte bitmap for PIDs base+1..base+0x20.
func (a *OBDApp) supportMask(base byte) []byte {
	mask := make([]byte, 4)
	for _, p := range a.pids() {
		if p > base && int(p) <= int(base)+0x20 {
			bit := int(p) - int(base) - 1
			mask[bit/8] |= 0x80 >> (bit % 8)
		}
	}
	// The next range is available when any PID above exists.
	for _, p := range a.pids() {
		if int(p) > int(base)+0x20 {
			mask[3] |= 0x01
			break
		}
	}
	return mask
}

// pidData encodes one mode 01 PID.
func (a *OBDApp) pidData(pid byte, s State) ([]byte, bool) {
	if pid%0x20 == 0 && pid <= 0xC0 {
		return a.supportMask(pid), true
	}
	if !a.supports(pid) {
		return nil, false
	}
	pct := func(v float64) byte { return u8(v * 255 / 100) }
	trim := func(v float64) byte { return u8(v*128/100 + 128) }
	switch pid {
	case 0x01:
		var n byte
		mil := false
		if !a.Minor {
			for _, f := range a.Engine.Faults() {
				if !f.Pending {
					n++
				}
				if f.Active {
					mil = true
				}
			}
		}
		b0 := n & 0x7F
		if mil {
			b0 |= 0x80
		}
		// Spark ignition: misfire, fuel, components supported; catalyst,
		// evap, O2 sensor, O2 heater supported, evap incomplete.
		return []byte{b0, 0x07, 0x65, 0x04}, true
	case 0x03:
		if s.ClosedLoop {
			return []byte{0x02, 0x00}, true
		}
		return []byte{0x01, 0x00}, true
	case 0x04:
		return []byte{pct(s.LoadPct)}, true
	case 0x05:
		return []byte{u8(s.CoolantC + 40)}, true
	case 0x06:
		return []byte{trim(s.STFT)}, true
	case 0x07:
		return []byte{trim(s.LTFT)}, true
	case 0x0B:
		return []byte{u8(s.MAPkPa)}, true
	case 0x0C:
		v := uint16(s.RPM * 4)
		return []byte{byte(v >> 8), byte(v)}, true
	case 0x0D:
		return []byte{u8(s.SpeedKmh)}, true
	case 0x0E:
		return []byte{u8((s.AdvanceDeg + 64) * 2)}, true
	case 0x0F:
		return []byte{u8(s.IntakeC + 40)}, true
	case 0x10:
		v := uint16(s.MAFkgh / 3.6 * 100)
		return []byte{byte(v >> 8), byte(v)}, true
	case 0x11:
		return []byte{pct(s.ThrottlePct)}, true
	case 0x13:
		return []byte{0x03}, true // bank 1 sensors 1 and 2
	case 0x14:
		return []byte{u8(s.O2V * 200), trim(s.STFT)}, true
	case 0x15:
		return []byte{u8(s.O2V2 * 200), 0xFF}, true
	case 0x1C:
		return []byte{0x06}, true // EOBD
	case 0x1F:
		v := uint16(math.Min(s.RunTime, 65535))
		return []byte{byte(v >> 8), byte(v)}, true
	case 0x21:
		return []byte{0x00, 0x00}, true
	case 0x23:
		v := uint16(s.FuelPressureKPa / 10)
		return []byte{byte(v >> 8), byte(v)}, true
	case 0x2F:
		return []byte{pct(s.FuelLevelPct)}, true
	case 0x30:
		return []byte{u8(s.WarmUps)}, true
	case 0x31:
		v := uint16(s.DistanceKm)
		return []byte{byte(v >> 8), byte(v)}, true
	case 0x33:
		return []byte{u8(s.BaroKPa)}, true
	case 0x3C:
		v := uint16((s.CatTempC + 40) * 10)
		return []byte{byte(v >> 8), byte(v)}, true
	case 0x42:
		v := uint16(s.BatteryV * 1000)
		return []byte{byte(v >> 8), byte(v)}, true
	case 0x43:
		v := uint16(s.LoadPct * 255 / 100)
		return []byte{byte(v >> 8), byte(v)}, true
	case 0x44:
		v := uint16(s.AFR / 14.7 * 32768)
		return []byte{byte(v >> 8), byte(v)}, true
	case 0x45:
		return []byte{pct(s.ThrottlePct)}, true
	case 0x46:
		return []byte{u8(s.AmbientC + 40)}, true
	case 0x47:
		return []byte{pct(s.ThrottlePct + 12)}, true
	case 0x49:
		return []byte{pct(s.PedalPct + 15)}, true
	case 0x4C:
		return []byte{pct(s.ThrottlePct)}, true
	case 0x4D:
		return []byte{0x00, 0x00}, true
	case 0x4E:
		v := uint16(math.Min(s.TimeSinceClear, 65535))
		return []byte{byte(v >> 8), byte(v)}, true
	case 0x51:
		return []byte{0x01}, true // gasoline
	case 0x5C:
		return []byte{u8(s.OilC + 40)}, true
	}
	return nil, false
}

// Handle implements App.
func (a *OBDApp) Handle(req []byte, t Transport, functional bool) [][]byte {
	if len(req) == 0 {
		return nil
	}
	s := a.Engine.State()
	switch req[0] {
	case 0x01:
		if len(req) < 2 {
			return nil
		}
		out := []byte{0x41}
		for _, pid := range req[1:] {
			if d, ok := a.pidData(pid, s); ok {
				out = append(out, pid)
				out = append(out, d...)
			}
		}
		if len(out) == 1 {
			if functional {
				return nil
			}
			return nrc(0x01, 0x12)
		}
		return [][]byte{out}
	case 0x02:
		if len(req) < 3 || a.Minor {
			return nil
		}
		ff, ok := a.Engine.FreezeFrame()
		if !ok {
			return nil
		}
		pid := req[1]
		if pid == 0x02 {
			faults := a.Engine.Faults()
			hi, lo, _ := dtc.ParseSAE(faults[0].Code)
			return [][]byte{{0x42, 0x02, req[2], hi, lo}}
		}
		if d, ok := a.pidData(pid, ff); ok {
			return [][]byte{append([]byte{0x42, pid, req[2]}, d...)}
		}
		return nil
	case 0x03, 0x07, 0x0A:
		if a.Minor {
			if functional {
				return nil
			}
		}
		var codes []string
		for _, f := range a.Engine.Faults() {
			switch req[0] {
			case 0x03:
				if !f.Pending {
					codes = append(codes, f.Code)
				}
			case 0x07:
				if f.Pending || f.Active {
					codes = append(codes, f.Code)
				}
			case 0x0A:
				if f.Active && !f.Pending {
					codes = append(codes, f.Code)
				}
			}
		}
		if a.Minor {
			codes = nil
		}
		return dtcAnswer(req[0]+0x40, codes, t)
	case 0x04:
		if !a.Minor {
			a.Engine.ClearFaults()
		}
		return [][]byte{{0x44}}
	case 0x09:
		if len(req) < 2 {
			return nil
		}
		return a.mode09(req[1], t)
	case 0x06:
		if len(req) == 2 && t == TransportCAN && !a.Minor {
			if out := a.mode06(req[1]); out != nil {
				return [][]byte{out}
			}
		}
		if functional {
			return nil
		}
		return nrc(0x06, 0x12)
	}
	if a.UDS && !functional && t == TransportCAN {
		return a.uds(req, s)
	}
	if functional {
		return nil
	}
	return nrc(req[0], 0x11)
}

func dtcAnswer(sid byte, codes []string, t Transport) [][]byte {
	var raw []byte
	for _, c := range codes {
		hi, lo, err := dtc.ParseSAE(c)
		if err == nil {
			raw = append(raw, hi, lo)
		}
	}
	if t == TransportCAN {
		return [][]byte{append([]byte{sid, byte(len(raw) / 2)}, raw...)}
	}
	// K-Line: three codes per message, padded with zeros.
	var out [][]byte
	for len(raw) > 0 || len(out) == 0 {
		chunk := make([]byte, 6)
		n := copy(chunk, raw)
		raw = raw[n:]
		out = append(out, append([]byte{sid}, chunk...))
	}
	return out
}

func (a *OBDApp) mode09(info byte, t Transport) [][]byte {
	if a.Minor && info != 0x00 && info != 0x0A {
		return nil
	}
	vin := []byte(a.Engine.VIN)
	switch info {
	case 0x00:
		mask := []byte{0x54, 0x40, 0x00, 0x00} // 02, 04, 06, 0A
		if t == TransportCAN {
			return [][]byte{append([]byte{0x49, 0x00}, mask...)}
		}
		return [][]byte{append([]byte{0x49, 0x00, 0x01}, mask...)}
	case 0x02:
		if t == TransportCAN {
			return [][]byte{append([]byte{0x49, 0x02, 0x01}, vin...)}
		}
		padded := append([]byte{0, 0, 0}, vin...)
		var out [][]byte
		for i := 0; i < 5; i++ {
			out = append(out, append([]byte{0x49, 0x02, byte(i + 1)}, padded[i*4:i*4+4]...))
		}
		return out
	case 0x04:
		cal := []byte("SIMCAL0107000000")
		if t == TransportCAN {
			return [][]byte{append([]byte{0x49, 0x04, 0x01}, cal...)}
		}
		var out [][]byte
		for i := 0; i < 4; i++ {
			out = append(out, append([]byte{0x49, 0x04, byte(i + 1)}, cal[i*4:i*4+4]...))
		}
		return out
	case 0x06:
		return [][]byte{{0x49, 0x06, 0x01, 0x1A, 0x2B, 0x3C, 0x4D}}
	case 0x0A:
		if t != TransportCAN {
			return nil
		}
		name := make([]byte, 20)
		copy(name, a.Name)
		return [][]byte{append([]byte{0x49, 0x0A, 0x01}, name...)}
	}
	return nil
}

// uds answers a subset of ISO 14229 services.
func (a *OBDApp) uds(req []byte, s State) [][]byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	sid := req[0]
	switch sid {
	case 0x10:
		if len(req) < 2 {
			return nrc(sid, 0x13)
		}
		a.session = req[1] & 0x7F
		a.unlocked = false
		if req[1]&0x80 != 0 {
			return nil
		}
		return [][]byte{{0x50, req[1], 0x00, 0x32, 0x01, 0xF4}}
	case 0x11:
		if len(req) < 2 {
			return nrc(sid, 0x13)
		}
		a.session, a.unlocked = 0x01, false
		return [][]byte{{0x51, req[1]}}
	case 0x3E:
		if len(req) >= 2 && req[1]&0x80 != 0 {
			return nil
		}
		return [][]byte{{0x7E, 0x00}}
	case 0x22:
		if len(req) < 3 || (len(req)-1)%2 != 0 {
			return nrc(sid, 0x13)
		}
		out := []byte{0x62}
		for i := 1; i+1 < len(req); i += 2 {
			did := binary.BigEndian.Uint16(req[i:])
			v, ok := a.readDID(did, s)
			if !ok {
				return nrc(sid, 0x31)
			}
			out = append(out, req[i], req[i+1])
			out = append(out, v...)
		}
		return [][]byte{out}
	case 0x2E:
		if len(req) < 4 {
			return nrc(sid, 0x13)
		}
		did := binary.BigEndian.Uint16(req[1:])
		if a.session == 0x01 {
			return nrc(sid, 0x7F)
		}
		if !a.unlocked {
			return nrc(sid, 0x33)
		}
		if _, ok := a.dids[did]; !ok || did >= 0xF100 {
			return nrc(sid, 0x31)
		}
		a.dids[did] = append([]byte(nil), req[3:]...)
		return [][]byte{{0x6E, req[1], req[2]}}
	case 0x27:
		if len(req) < 2 {
			return nrc(sid, 0x13)
		}
		if a.session == 0x01 {
			return nrc(sid, 0x7F)
		}
		switch req[1] {
		case 0x01:
			a.seed = 0x3A5C
			if a.unlocked {
				return [][]byte{{0x67, 0x01, 0x00, 0x00}}
			}
			return [][]byte{{0x67, 0x01, byte(a.seed >> 8), byte(a.seed)}}
		case 0x02:
			if len(req) < 4 {
				return nrc(sid, 0x13)
			}
			// Simulator-only demonstration algorithm: key = seed XOR 0x55AA.
			key := binary.BigEndian.Uint16(req[2:])
			if key != a.seed^0x55AA {
				return nrc(sid, 0x35)
			}
			a.unlocked = true
			return [][]byte{{0x67, 0x02}}
		}
		return nrc(sid, 0x12)
	case 0x19:
		return a.readDTCInfo(req)
	case 0x14:
		a.Engine.ClearFaults()
		return [][]byte{{0x54}}
	case 0x31:
		if len(req) < 4 {
			return nrc(sid, 0x13)
		}
		rid := binary.BigEndian.Uint16(req[2:])
		if rid == 0x0203 {
			// A slow routine: first "response pending", then the result.
			return [][]byte{{0x7F, 0x31, 0x78}, {0x71, req[1], req[2], req[3], 0x00}}
		}
		return [][]byte{{0x71, req[1], req[2], req[3], 0x00}}
	case 0x2F:
		if len(req) < 4 {
			return nrc(sid, 0x13)
		}
		return [][]byte{append([]byte{0x6F}, req[1:]...)}
	case 0x85:
		if len(req) < 2 {
			return nrc(sid, 0x13)
		}
		return [][]byte{{0xC5, req[1]}}
	case 0x28:
		if len(req) < 3 {
			return nrc(sid, 0x13)
		}
		return [][]byte{{0x68, req[1]}}
	}
	return nrc(sid, 0x11)
}

func (a *OBDApp) readDID(did uint16, s State) ([]byte, bool) {
	switch {
	case did == 0xF190:
		return []byte(a.Engine.VIN), true
	case did == 0xF186:
		return []byte{a.session}, true
	case did >= 0xF400 && did <= 0xF4FF:
		// ISO 27145 (WWH-OBD) maps OBD PIDs to DIDs F4xx.
		return a.pidData(byte(did), s)
	}
	v, ok := a.dids[did]
	return v, ok
}

func (a *OBDApp) readDTCInfo(req []byte) [][]byte {
	if len(req) < 2 {
		return nrc(0x19, 0x13)
	}
	type rec struct {
		code   []byte
		status byte
	}
	var recs []rec
	if !a.Minor {
		for _, f := range a.Engine.Faults() {
			hi, lo, err := dtc.ParseSAE(f.Code)
			if err != nil {
				continue
			}
			st := byte(0x28) // confirmed, failed since clear
			if f.Active {
				st = 0x8B
			}
			if f.Pending {
				st = 0x24
			}
			recs = append(recs, rec{[]byte{hi, lo, 0x00}, st})
		}
	}
	sort.Slice(recs, func(i, j int) bool { return string(recs[i].code) < string(recs[j].code) })
	switch req[1] {
	case 0x01:
		mask := byte(0xFF)
		if len(req) > 2 {
			mask = req[2]
		}
		n := 0
		for _, r := range recs {
			if r.status&mask != 0 {
				n++
			}
		}
		return [][]byte{{0x59, 0x01, 0xFF, 0x01, byte(n >> 8), byte(n)}}
	case 0x02:
		mask := byte(0xFF)
		if len(req) > 2 {
			mask = req[2]
		}
		out := []byte{0x59, 0x02, 0xFF}
		for _, r := range recs {
			if r.status&mask != 0 {
				out = append(out, r.code...)
				out = append(out, r.status)
			}
		}
		return [][]byte{out}
	case 0x0A:
		out := []byte{0x59, 0x0A, 0xFF}
		for _, r := range recs {
			out = append(out, r.code...)
			out = append(out, r.status)
		}
		return [][]byte{out}
	}
	return nrc(0x19, 0x12)
}

// mode06 answers on-board monitoring test results in the CAN format: the
// oxygen sensors and their heaters, the catalyst (failing with P0420)
// and misfire counts per cylinder that follow an injected misfire.
func (a *OBDApp) mode06(mid byte) []byte {
	masks := map[byte][4]byte{
		0x00: {0xC0, 0x00, 0x00, 0x01}, // 01, 02
		0x20: {0x80, 0x00, 0x00, 0x01}, // 21
		0x40: {0xC0, 0x00, 0x00, 0x01}, // 41, 42
		0x60: {0x00, 0x00, 0x00, 0x01},
		0x80: {0x00, 0x00, 0x00, 0x01},
		0xA0: {0xF8, 0x00, 0x00, 0x00}, // A1–A5
	}
	if mid%0x20 == 0 {
		m, ok := masks[mid]
		if !ok {
			return nil
		}
		return []byte{0x46, mid, m[0], m[1], m[2], m[3]}
	}
	test := func(tid, id byte, v, lo, hi uint16) []byte {
		return []byte{mid, tid, id, byte(v >> 8), byte(v), byte(lo >> 8), byte(lo), byte(hi >> 8), byte(hi)}
	}
	var recs [][]byte
	switch mid {
	case 0x01: // upstream sensor, switching between 0.1 and 0.8 V (0.122 mV per bit)
		recs = [][]byte{
			test(0x01, 0x0A, 3689, 3689, 3689), // rich to lean threshold 0.45 V
			test(0x05, 0x10, 72, 0, 160),       // switch times, ms
			test(0x06, 0x10, 81, 0, 160),
			test(0x07, 0x0A, 656, 0, 3279),     // lowest 0.08 V, at most 0.4 V
			test(0x08, 0x0A, 6721, 4918, 9836), // highest 0.82 V, 0.6–1.2 V
		}
	case 0x02: // downstream sensor, steady behind a working catalyst
		recs = [][]byte{test(0x07, 0x0A, 4590, 0, 5738), test(0x08, 0x0A, 5984, 4918, 9836)}
	case 0x21: // catalyst: rear to front switch ratio, at most 0.75
		ratio := uint16(38)
		if a.hasFault("P0420") {
			ratio = 221
		}
		recs = [][]byte{test(0x81, 0x20, ratio, 0, 192)}
	case 0x41, 0x42: // heater current, A
		cur := uint16(940)
		if mid == 0x42 {
			cur = 710
		}
		recs = [][]byte{test(0x81, 0x0E, cur, 500, 2500)}
	case 0xA1, 0xA2, 0xA3, 0xA4, 0xA5:
		avg, now := a.misfires()
		i := int(mid - 0xA1) // 0: all cylinders
		recs = [][]byte{test(0x0B, 0x24, avg[i], 0, 0), test(0x0C, 0x24, now[i], 0, 0)}
	default:
		return nil
	}
	out := []byte{0x46}
	for _, r := range recs {
		out = append(out, r...)
	}
	return out
}

// misfires returns the misfire counts (average of the last ten drives and
// the current drive) of all cylinders [0] and of cylinders 1–4: a random
// misfire (P0300) hits cylinders 3 and 2, P0301–P0304 one cylinder.
func (a *OBDApp) misfires() (avg, now [5]uint16) {
	now = [5]uint16{0, 0, 1, 0, 0}
	avg = [5]uint16{0, 0, 0, 0, 0}
	for _, f := range a.Engine.Faults() {
		switch {
		case f.Code == "P0300":
			now = [5]uint16{0, 3, 41, 118, 6}
			avg = [5]uint16{0, 1, 12, 36, 2}
		case len(f.Code) == 5 && f.Code[:4] == "P030" && f.Code[4] >= '1' && f.Code[4] <= '4':
			c := int(f.Code[4] - '0')
			now[c], avg[c] = 96, 29
		}
	}
	for c := 1; c <= 4; c++ {
		now[0] += now[c]
		avg[0] += avg[c]
	}
	return avg, now
}

func (a *OBDApp) hasFault(code string) bool {
	for _, f := range a.Engine.Faults() {
		if f.Code == code {
			return true
		}
	}
	return false
}

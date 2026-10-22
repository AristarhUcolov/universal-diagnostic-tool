package sim

import (
	"math"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
)

// VAZVariant selects the layout of local identifier 0x01.
type VAZVariant int

const (
	VariantEuro2 VAZVariant = iota // Bosch M1.5.4N / Январь-5.1 Euro-2: 34 data bytes
	VariantJ72                     // Январь-7.2: Euro-2 layout plus two bytes
	VariantR83                     // Bosch M1.5.4 Russia-83, no oxygen sensor: 32 data bytes
)

// VAZApp answers like an AvtoVAZ-specification KWP2000 engine controller.
type VAZApp struct {
	Engine  *Engine
	Variant VAZVariant
	OBD     *OBDApp // optional OBD-II services (Euro-3 controllers)
	io      map[byte]byte
}

// NewVAZApp creates the application.
func NewVAZApp(e *Engine, v VAZVariant) *VAZApp {
	return &VAZApp{Engine: e, Variant: v, io: map[byte]byte{}}
}

func nrc(sid, code byte) [][]byte { return [][]byte{{0x7F, sid, code}} }

// Handle implements App.
func (a *VAZApp) Handle(req []byte, t Transport, functional bool) [][]byte {
	if len(req) == 0 {
		return nil
	}
	if functional || (req[0] >= 0x01 && req[0] <= 0x0A) {
		if a.OBD != nil {
			return a.OBD.Handle(req, t, functional)
		}
		if functional {
			return nil
		}
		return nrc(req[0], 0x11)
	}
	s := a.Engine.State()
	switch req[0] {
	case 0x10:
		if len(req) < 2 {
			return nrc(0x10, 0x12)
		}
		return [][]byte{{0x50, req[1]}}
	case 0x11:
		return [][]byte{{0x51}}
	case 0x14:
		a.Engine.ClearFaults()
		g := []byte{0x00, 0x00}
		if len(req) >= 3 {
			g = req[1:3]
		}
		return [][]byte{append([]byte{0x54}, g...)}
	case 0x18:
		faults := a.Engine.Faults()
		out := []byte{0x58, byte(len(faults))}
		for _, f := range faults {
			hi, lo, err := dtc.ParseSAE(f.Code)
			if err != nil {
				continue
			}
			st := byte(0x20)
			if f.Active {
				st = 0xE0
			}
			out = append(out, hi, lo, st)
		}
		return [][]byte{out}
	case 0x1A:
		return a.ident(req)
	case 0x21:
		if len(req) < 2 {
			return nrc(0x21, 0x12)
		}
		switch req[1] {
		case 0x01:
			return [][]byte{append([]byte{0x61, 0x01}, a.block01(s)...)}
		case 0x02:
			return [][]byte{append([]byte{0x61, 0x02}, a.block02(s)...)}
		case 0x03:
			return [][]byte{append([]byte{0x61, 0x03}, a.block03(s)...)}
		case 0xA0:
			return [][]byte{{0x61, 0xA0, 0x00, 0x20, 0x12, 0x34}}
		case 0xA1:
			return [][]byte{append([]byte{0x61, 0xA1}, []byte("0712345")...)}
		case 0xA2:
			return [][]byte{append([]byte{0x61, 0xA2}, []byte("1234567")...)}
		case 0xA3:
			return [][]byte{append([]byte{0x61, 0xA3}, []byte("12-05-2003")...)}
		}
		return nrc(0x21, 0x31)
	case 0x23:
		if len(req) < 5 {
			return nrc(0x23, 0x12)
		}
		return [][]byte{append([]byte{0x63}, make([]byte, int(req[4]))...)}
	case 0x30:
		return a.ioControl(req, s)
	case 0x3B:
		if len(req) < 2 {
			return nrc(0x3B, 0x12)
		}
		return [][]byte{{0x7B, req[1]}}
	case 0x3E:
		if len(req) >= 2 && req[1] == 0x02 {
			return nil
		}
		return [][]byte{{0x7E}}
	}
	return nrc(req[0], 0x11)
}

func (a *VAZApp) ident(req []byte) [][]byte {
	if len(req) < 2 {
		return nrc(0x1A, 0x12)
	}
	hw, ss, sw, name := "2112 -1411020-60", "0261204726", "1411000-00", "SAMARA-1.5l, 8V"
	switch a.Variant {
	case VariantJ72:
		hw, ss, sw, name = "21124-1411020-3", "0000000000", "I203EL35  ", "VAZ-21124 1.6 16"
	case VariantR83:
		hw, sw = "2111 -1411020-20", "1411001-00"
	}
	fields := []struct {
		id   byte
		text string
		n    int
	}{
		{0x90, "XTA21093Y-0000010-20", 19},
		{0x91, hw, 16},
		{0x92, ss, 10},
		{0x94, sw, 10},
		{0x97, name, 15},
		{0x98, "2850358", 7},
		{0x99, "12-05-2003", 10},
		{0x9A, "M1V13F04", 8},
	}
	pad := func(s string, n int) []byte {
		b := []byte(s)
		for len(b) < n {
			b = append(b, ' ')
		}
		return b[:n]
	}
	if req[1] == 0x80 {
		out := []byte{0x5A, 0x80}
		for _, f := range fields {
			out = append(out, pad(f.text, f.n)...)
		}
		return [][]byte{out}
	}
	for _, f := range fields {
		if f.id == req[1] {
			return [][]byte{append([]byte{0x5A, f.id}, pad(f.text, f.n)...)}
		}
	}
	return nrc(0x1A, 0x31)
}

func le16(v float64) []byte {
	if v < 0 {
		v = 0
	}
	if v > 65535 {
		v = 65535
	}
	x := uint16(math.Round(v))
	return []byte{byte(x), byte(x >> 8)}
}

func (a *VAZApp) flags(s State) (cfg1, cfg2, mode1, mode2 byte, faults [4]byte) {
	cfg1 = 0x2B // O2 sensor, canister, knock sensor, phase sensor
	cfg2 = 0x5E
	if a.Variant == VariantR83 {
		cfg1 &^= 0x01
		cfg2 |= 0x01 // CO potentiometer
	}
	if !s.Running {
		mode1 |= 0x01
	}
	if s.Idle && s.Running {
		mode1 |= 0x02
	}
	if s.ThrottlePct > 70 {
		mode1 |= 0x04
	}
	if s.ClosedLoop {
		mode1 |= 0x10
	}
	if s.ClosedLoop && s.CoolantC > 70 {
		mode1 |= 0x40
	}
	if s.Idle {
		mode2 |= 0x02
	}
	if s.O2V > 0.45 {
		mode2 |= 0x80
	}
	for _, f := range a.Engine.Faults() {
		if !f.Active {
			continue
		}
		switch f.Code {
		case "P0335":
			faults[0] |= 0x01
		case "P0340":
			faults[0] |= 0x10
		case "P0135":
			faults[0] |= 0x08
		case "P0117":
			faults[1] |= 0x08
		case "P0118":
			faults[2] |= 0x08
		case "P0122":
			faults[1] |= 0x20
		case "P0123":
			faults[2] |= 0x20
		case "P0102":
			faults[1] |= 0x40
		case "P0103":
			faults[2] |= 0x40
		case "P0325":
			faults[3] |= 0x01
		case "P0134":
			faults[3] |= 0x08
		case "P0501":
			faults[3] |= 0x40
		case "P0505":
			faults[3] |= 0x80
		}
	}
	return
}

// block01 encodes the after-sales record (RLI_ASS) per the AvtoVAZ
// specification.
func (a *VAZApp) block01(s State) []byte {
	cfg1, cfg2, m1, m2, fl := a.flags(s)
	out := []byte{cfg1, cfg2, m1, m2, fl[0], fl[1], fl[2], fl[3]}
	out = append(out, u8(s.CoolantC+40))
	if a.Variant == VariantR83 {
		out = append(out, 0x00) // CO correction coefficient (signed E/256)
	} else {
		out = append(out, u8(s.AFR/14.7*256-128))
	}
	idle := s.RPM / 10
	out = append(out,
		u8(s.ThrottlePct),
		u8(s.RPM/40),
		u8(idle),
		u8(s.DesiredIACSteps),
		u8(s.IACSteps),
		u8((1+s.STFT/100)*256-128),
		s8(s.AdvanceDeg*2),
		u8(s.SpeedKmh),
		u8((s.BatteryV-5.2)/0.05),
		u8(s.DesiredIdleRPM/10),
	)
	if a.Variant != VariantR83 {
		var o2flags byte
		if s.ClosedLoop {
			o2flags = 0x03
		} else if s.Running {
			o2flags = 0x02
		}
		out = append(out, u8(s.O2V/1.25*256), o2flags)
	}
	trip := 0xFFFF * 1.0
	if s.SpeedKmh > 3 {
		trip = s.TripFuel100 * 128
	}
	out = append(out, le16(s.InjMs*125)...)
	out = append(out, le16(s.MAFkgh*10)...)
	out = append(out, le16(s.CyclicAirMg*6)...)
	out = append(out, le16(s.FuelLh*50)...)
	out = append(out, le16(trip)...)
	out = append(out, 0xDD, 0xA4) // ROM checksum
	if a.Variant == VariantJ72 {
		out = append(out, 0x00, 0x00)
	}
	return out
}

// block02 encodes the end-of-line record (RLI_EOL).
func (a *VAZApp) block02(s State) []byte {
	cfg1, cfg2, m1, m2, _ := a.flags(s)
	out := []byte{cfg1, cfg2, m1, m2, u8(s.CoolantC + 40), u8(s.AFR/14.7*256 - 128), u8(s.ThrottlePct),
		u8(s.RPM / 40), u8(s.RPM / 10), u8(s.IACSteps), u8((1+s.STFT/100)*256 - 128), s8(s.AdvanceDeg * 2),
		u8(s.SpeedKmh), u8((s.BatteryV - 5.2) / 0.05), 0x03}
	out = append(out, le16(s.InjMs*125)...)
	out = append(out, le16(s.MAFkgh*10)...)
	out = append(out, le16(s.CyclicAirMg*6)...)
	out = append(out, le16(s.FuelLh*50)...)
	out = append(out, le16(0xFFFF)...)
	return out
}

// block03 encodes the factory test record (raw ADC codes and ports).
func (a *VAZApp) block03(s State) []byte {
	ctsV := 4.5 - s.CoolantC/30
	mafV := 1.0 + s.MAFkgh/40
	tpsV := 0.5 + s.ThrottlePct*0.04
	var p1, p5 byte
	if s.CoolantC > 99 {
		p1 |= 0x80
	}
	if !s.Running {
		p5 |= 0x08 // check-engine lamp on
	}
	return []byte{u8(0.6 * 256 / 5), u8(ctsV * 256 / 5), u8(mafV * 256 / 5), u8(s.BatteryV / (0.287 * 5) * 256),
		u8(s.O2V * 256 / 1.25), u8(tpsV * 256 / 5), p1, p5, 0x07, 0x04}
}

func (a *VAZApp) ioControl(req []byte, s State) [][]byte {
	if len(req) < 3 {
		return nrc(0x30, 0x12)
	}
	lid, param := req[1], req[2]
	switch lid {
	case 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x09, 0x0A, 0x0B, 0x0C, 0x0D:
		switch param {
		case 0x00:
			delete(a.io, lid)
			return [][]byte{{0x70, lid, 0x00}}
		case 0x01:
			return [][]byte{{0x70, lid, 0x01, a.io[lid]}}
		case 0x06:
			if len(req) < 4 {
				return nrc(0x30, 0x12)
			}
			a.io[lid] = req[3]
			return [][]byte{{0x70, lid, 0x06, req[3]}}
		}
		return nrc(0x30, 0x12)
	case 0x41, 0x42:
		switch param {
		case 0x00:
			delete(a.io, lid)
			return [][]byte{{0x70, lid, 0x00}}
		case 0x07:
			if len(req) < 4 {
				return nrc(0x30, 0x12)
			}
			a.io[lid] = req[3]
			return [][]byte{{0x70, lid, 0x07, u8(s.DesiredIACSteps), u8(s.IACSteps), u8(s.RPM / 10), req[3]}}
		}
		return nrc(0x30, 0x12)
	}
	return nrc(0x30, 0x31)
}

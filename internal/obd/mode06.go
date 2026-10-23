package obd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// On-board monitoring test results (service $06, SAE J1979): the values
// an engine controller measured for its monitors — oxygen sensors,
// catalyst, EGR, evaporative system, misfire per cylinder — and the
// limits it judges them by. A value close to a limit shows a part on its
// way out before any trouble code is stored, and the misfire counters
// point to a cylinder that misfires too rarely to set a code.

// Errors.
var (
	ErrTestsNeedCAN     = errors.New("obd: test results with standard units and limits exist on CAN vehicles (ISO 15765-4) only; older protocols report manufacturer-specific values")
	ErrTestsUnsupported = errors.New("obd: the vehicle does not report on-board monitoring test results")
)

// uas is a unit and scaling identifier (SAE J1979, appendix E): how the
// two bytes of a test value and of its limits become a quantity.
type uas struct {
	scale, offset float64
	unit          string
	signed        bool
}

func (u uas) decode(b []byte) float64 {
	raw := float64(uint16(b[0])<<8 | uint16(b[1]))
	if u.signed {
		raw = float64(int16(uint16(b[0])<<8 | uint16(b[1])))
	}
	return raw*u.scale + u.offset
}

// digits is the number of decimals worth showing for a scale.
func (u uas) digits() int {
	if u.scale >= 1 {
		return 0
	}
	return int(math.Min(5, math.Ceil(-math.Log10(u.scale)-1e-9)))
}

var uasTable = map[byte]uas{
	0x01: {1, 0, "", false}, 0x02: {0.1, 0, "", false}, 0x03: {0.01, 0, "", false}, 0x04: {0.001, 0, "", false},
	0x05: {0.0000305, 0, "", false}, 0x06: {0.000305, 0, "", false},
	0x07: {0.25, 0, "rpm", false}, 0x08: {0.01, 0, "km/h", false}, 0x09: {1, 0, "km/h", false},
	0x0A: {0.122, 0, "mV", false}, 0x0B: {0.001, 0, "V", false}, 0x0C: {0.01, 0, "V", false},
	0x0D: {0.00390625, 0, "mA", false}, 0x0E: {0.001, 0, "A", false}, 0x0F: {0.01, 0, "A", false},
	0x10: {1, 0, "ms", false}, 0x11: {100, 0, "ms", false}, 0x12: {1, 0, "s", false},
	0x13: {1, 0, "mΩ", false}, 0x14: {1, 0, "Ω", false}, 0x15: {1, 0, "kΩ", false},
	0x16: {0.1, -40, "°C", false}, 0x17: {0.01, 0, "kPa", false}, 0x18: {0.0117, 0, "kPa", false},
	0x19: {0.079, 0, "kPa", false}, 0x1A: {1, 0, "kPa", false}, 0x1B: {10, 0, "kPa", false},
	0x1C: {0.01, 0, "°", false}, 0x1D: {0.5, 0, "°", false},
	0x1E: {0.0000305, 0, "λ", false}, 0x1F: {0.05, 0, "", false}, 0x20: {0.00390625, 0, "", false},
	0x21: {1, 0, "mHz", false}, 0x22: {1, 0, "Hz", false}, 0x23: {1, 0, "kHz", false},
	0x24: {1, 0, "", false}, 0x25: {1, 0, "km", false}, 0x26: {0.1, 0, "mV/ms", false},
	0x27: {0.01, 0, "g/s", false}, 0x28: {1, 0, "g/s", false}, 0x29: {0.25, 0, "Pa/s", false},
	0x2A: {0.001, 0, "kg/h", false}, 0x2B: {1, 0, "", false}, 0x2C: {0.01, 0, "g", false},
	0x2D: {0.01, 0, "mg", false}, 0x2E: {1, 0, "", false}, 0x2F: {0.01, 0, "%", false},
	0x30: {0.001526, 0, "%", false}, 0x31: {0.001, 0, "l", false}, 0x32: {0.0000305, 0, "in", false},
	0x33: {0.00024414, 0, "λ", false}, 0x34: {1, 0, "min", false}, 0x35: {10, 0, "ms", false},
	0x36: {0.01, 0, "g", false}, 0x37: {0.1, 0, "g", false}, 0x38: {1, 0, "g", false},
	0x39: {0.01, -327.68, "%", false}, 0x3A: {0.001, 0, "g", false}, 0x3B: {0.0001, 0, "g", false},
	0x3C: {0.1, 0, "µs", false}, 0x3D: {0.01, 0, "mA", false}, 0x3E: {0.00006103516, 0, "mm²", false},
	0x3F: {0.01, 0, "l", false}, 0x40: {1, 0, "ppm", false}, 0x41: {0.01, 0, "µA", false},
	0x81: {1, 0, "", true}, 0x82: {0.1, 0, "", true}, 0x83: {0.01, 0, "", true}, 0x84: {0.001, 0, "", true},
	0x85: {0.0000305, 0, "", true}, 0x86: {0.000305, 0, "", true}, 0x87: {1, 0, "ppm", true},
	0x8A: {0.122, 0, "mV", true}, 0x8B: {0.001, 0, "V", true}, 0x8C: {0.01, 0, "V", true},
	0x8D: {0.00390625, 0, "mA", true}, 0x8E: {0.001, 0, "A", true}, 0x90: {1, 0, "ms", true},
	0x96: {0.1, 0, "°C", true}, 0x99: {0.1, 0, "kPa", true}, 0x9C: {0.01, 0, "°", true}, 0x9D: {0.5, 0, "°", true},
	0xA8: {1, 0, "g/s", true}, 0xA9: {0.25, 0, "Pa/s", true}, 0xAD: {0.01, 0, "mg", true}, 0xAE: {0.1, 0, "mg", true},
	0xAF: {0.01, 0, "%", true}, 0xB0: {0.003052, 0, "%", true}, 0xB1: {2, 0, "mV/s", true},
	0xFC: {0.01, 0, "kPa", true}, 0xFD: {0.001, 0, "kPa", true}, 0xFE: {0.25, 0, "Pa", true},
}

// monitorName names an on-board monitor (OBDMID).
func monitorName(mid byte) i18n.Text {
	bankSensor := func(base byte) (int, int) { return int(mid-base)/4 + 1, int(mid-base)%4 + 1 }
	n := func(base byte) int { return int(mid-base) + 1 }
	switch {
	case mid >= 0x01 && mid <= 0x10:
		b, s := bankSensor(0x01)
		return i18n.T(fmt.Sprintf("Oxygen sensor, bank %d sensor %d", b, s), fmt.Sprintf("Датчик кислорода, ряд %d, датчик %d", b, s))
	case mid >= 0x21 && mid <= 0x24:
		return i18n.T(fmt.Sprintf("Catalyst, bank %d", n(0x21)), fmt.Sprintf("Катализатор, ряд %d", n(0x21)))
	case mid >= 0x31 && mid <= 0x34:
		return i18n.T(fmt.Sprintf("EGR, bank %d", n(0x31)), fmt.Sprintf("Рециркуляция газов (EGR), ряд %d", n(0x31)))
	case mid >= 0x35 && mid <= 0x38:
		return i18n.T(fmt.Sprintf("Variable valve timing, bank %d", n(0x35)), fmt.Sprintf("Фазы газораспределения (VVT), ряд %d", n(0x35)))
	case mid == 0x39:
		return i18n.T("Evaporative system: cap off / 0.150\" leak", "Система улавливания паров (EVAP): крышка бака / утечка 0,150″")
	case mid == 0x3A:
		return i18n.T("Evaporative system: 0.090\" leak", "Система улавливания паров (EVAP): утечка 0,090″")
	case mid == 0x3B:
		return i18n.T("Evaporative system: 0.040\" leak", "Система улавливания паров (EVAP): утечка 0,040″")
	case mid == 0x3C:
		return i18n.T("Evaporative system: 0.020\" leak", "Система улавливания паров (EVAP): утечка 0,020″")
	case mid == 0x3D:
		return i18n.T("Canister purge flow", "Продувка адсорбера")
	case mid >= 0x41 && mid <= 0x50:
		b, s := bankSensor(0x41)
		return i18n.T(fmt.Sprintf("Oxygen sensor heater, bank %d sensor %d", b, s), fmt.Sprintf("Подогрев датчика кислорода, ряд %d, датчик %d", b, s))
	case mid >= 0x61 && mid <= 0x64:
		return i18n.T(fmt.Sprintf("Heated catalyst, bank %d", n(0x61)), fmt.Sprintf("Подогреваемый катализатор, ряд %d", n(0x61)))
	case mid >= 0x71 && mid <= 0x74:
		return i18n.T(fmt.Sprintf("Secondary air %d", n(0x71)), fmt.Sprintf("Вторичный воздух %d", n(0x71)))
	case mid >= 0x81 && mid <= 0x84:
		return i18n.T(fmt.Sprintf("Fuel system, bank %d", n(0x81)), fmt.Sprintf("Топливная система, ряд %d", n(0x81)))
	case mid >= 0x85 && mid <= 0x86:
		return i18n.T(fmt.Sprintf("Boost pressure control, bank %d", n(0x85)), fmt.Sprintf("Регулирование наддува, ряд %d", n(0x85)))
	case mid >= 0x90 && mid <= 0x91:
		return i18n.T(fmt.Sprintf("NOx adsorber, bank %d", n(0x90)), fmt.Sprintf("Накопитель NOx, ряд %d", n(0x90)))
	case mid >= 0x98 && mid <= 0x99:
		return i18n.T(fmt.Sprintf("NOx catalyst, bank %d", n(0x98)), fmt.Sprintf("Катализатор NOx, ряд %d", n(0x98)))
	case mid == 0xA1:
		return i18n.T("Misfire, all cylinders", "Пропуски зажигания, все цилиндры")
	case mid >= 0xA2 && mid <= 0xAD:
		return i18n.T(fmt.Sprintf("Misfire, cylinder %d", n(0xA2)), fmt.Sprintf("Пропуски зажигания, цилиндр %d", n(0xA2)))
	case mid >= 0xB0 && mid <= 0xB1:
		return i18n.T(fmt.Sprintf("Particulate filter, bank %d", n(0xB0)), fmt.Sprintf("Сажевый фильтр, ряд %d", n(0xB0)))
	}
	return i18n.T(fmt.Sprintf("Monitor %02X", mid), fmt.Sprintf("Монитор %02X", mid))
}

// testName names the standard tests (TID 01–0C); tests 80–FF belong to
// the manufacturer and have no standard name.
func testName(tid byte) i18n.Text {
	switch tid {
	case 0x01:
		return i18n.T("Rich to lean switch threshold", "Порог переключения богато → бедно")
	case 0x02:
		return i18n.T("Lean to rich switch threshold", "Порог переключения бедно → богато")
	case 0x03:
		return i18n.T("Low voltage for the switch time", "Нижнее напряжение для расчёта времени переключения")
	case 0x04:
		return i18n.T("High voltage for the switch time", "Верхнее напряжение для расчёта времени переключения")
	case 0x05:
		return i18n.T("Rich to lean switch time", "Время переключения богато → бедно")
	case 0x06:
		return i18n.T("Lean to rich switch time", "Время переключения бедно → богато")
	case 0x07:
		return i18n.T("Lowest sensor voltage in the test", "Минимальное напряжение датчика за тест")
	case 0x08:
		return i18n.T("Highest sensor voltage in the test", "Максимальное напряжение датчика за тест")
	case 0x09:
		return i18n.T("Time between switches", "Время между переключениями")
	case 0x0A:
		return i18n.T("Sensor period", "Период переключения датчика")
	case 0x0B:
		return i18n.T("Misfires, average of the last 10 drives", "Пропуски: среднее за 10 последних поездок")
	case 0x0C:
		return i18n.T("Misfires, last or current drive", "Пропуски: последняя или текущая поездка")
	}
	return nil
}

// ParseTests decodes a CAN answer to service $06: "46" followed by
// records of nine bytes — monitor, test, unit and scaling, the value and
// its lower and upper limits.
func ParseTests(data []byte) []ecu.TestResult {
	if len(data) < 10 || data[0] != 0x46 {
		return nil
	}
	var out []ecu.TestResult
	for d := data[1:]; len(d) >= 9; d = d[9:] {
		mid, tid := d[0], d[1]
		u, ok := uasTable[d[2]]
		if !ok {
			u = uas{scale: 1} // unknown scaling: the raw number
		}
		r := ecu.TestResult{MID: mid, TID: tid, Monitor: monitorName(mid), Test: testName(tid),
			Value: u.decode(d[3:5]), Min: u.decode(d[5:7]), Max: u.decode(d[7:9]),
			Unit: u.unit, Digits: u.digits(), Raw: hexutil.Format(d[:9])}
		if d[2] == 0x2E { // a yes/no result
			r.Value = boolValue(d[3:5])
		}
		// Limits of zero mean "none", as for misfire counts.
		r.Limits = d[5]|d[6]|d[7]|d[8] != 0
		r.Pass = !r.Limits || (r.Value >= r.Min && r.Value <= r.Max)
		out = append(out, r)
	}
	return out
}

func boolValue(b []byte) float64 {
	if b[0]|b[1] != 0 {
		return 1
	}
	return 0
}

// TestResults reads the test results of every answering control unit.
func (h *Handler) TestResults(ctx context.Context) ([]ecu.TestResult, error) {
	if !h.c.CAN {
		return nil, ErrTestsNeedCAN
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	sup, err := h.c.supported(ctx, 0x06)
	if err != nil || len(sup) == 0 {
		return nil, ErrTestsUnsupported
	}
	set := map[byte]bool{}
	for _, mids := range sup {
		for _, m := range mids {
			set[m] = true
		}
	}
	mids := make([]byte, 0, len(set))
	for m := range set {
		mids = append(mids, m)
	}
	sort.Slice(mids, func(i, j int) bool { return mids[i] < mids[j] })
	out := []ecu.TestResult{}
	for _, mid := range mids {
		msgs, err := h.c.Query(ctx, []byte{0x06, mid})
		if err != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			continue
		}
		for _, m := range msgs {
			for _, r := range ParseTests(m.Data) {
				r.Source = fmt.Sprintf("%X", m.Src)
				out = append(out, r)
			}
		}
	}
	return out, nil
}

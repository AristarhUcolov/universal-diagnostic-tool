package j1939

import (
	"fmt"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// SPN describes how to find a parameter in a parameter group (SAE
// J1939-71). Start is the 1-based byte position, Bits the length.
type SPN struct {
	SPN    uint32
	PGN    uint32
	Start  int // byte, 1-based
	Bit    int // bit offset inside the start byte for sub-byte values (0-based)
	Bits   int
	Scale  float64
	Offset float64
	Unit   string
	Min    float64
	Max    float64
	Name   i18n.Text
	Group  string
}

// Decode extracts the value from a parameter group's data. ok is false
// for "not available" (all ones) and "error" values.
func (s *SPN) Decode(data []byte) (float64, bool) {
	i := s.Start - 1
	if s.Bits < 8 {
		if i >= len(data) {
			return 0, false
		}
		raw := (data[i] >> s.Bit) & (1<<s.Bits - 1)
		if raw == 1<<s.Bits-1 {
			return 0, false // not available
		}
		return float64(raw)*s.Scale + s.Offset, true
	}
	n := s.Bits / 8
	if i+n > len(data) {
		return 0, false
	}
	var raw uint64
	for k := n - 1; k >= 0; k-- {
		raw = raw<<8 | uint64(data[i+k])
	}
	// Valid values end at 0xFA, 0xFAFF, 0xFAFFFFFF; the rest mean
	// "error" or "not available".
	limit := uint64(0xFA)<<(8*(n-1)) | (uint64(1)<<(8*(n-1)) - 1)
	if raw > limit {
		return 0, false
	}
	return float64(raw)*s.Scale + s.Offset, true
}

// ParamID returns the program-wide parameter id.
func (s *SPN) ParamID() string { return fmt.Sprintf("j1939.spn%d", s.SPN) }

// SPNs is the table of common parameters.
var SPNs = []SPN{
	{SPN: 190, PGN: 61444, Start: 4, Bits: 16, Scale: 0.125, Unit: "rpm", Max: 3000, Name: i18n.T("Engine speed", "Обороты двигателя"), Group: "engine"},
	{SPN: 513, PGN: 61444, Start: 3, Bits: 8, Scale: 1, Offset: -125, Unit: "%", Min: -125, Max: 125, Name: i18n.T("Actual engine torque", "Фактический момент двигателя"), Group: "engine"},
	{SPN: 512, PGN: 61444, Start: 2, Bits: 8, Scale: 1, Offset: -125, Unit: "%", Min: -125, Max: 125, Name: i18n.T("Driver's demand torque", "Запрошенный момент"), Group: "engine"},
	{SPN: 91, PGN: 61443, Start: 2, Bits: 8, Scale: 0.4, Unit: "%", Max: 100, Name: i18n.T("Accelerator pedal position", "Положение педали акселератора"), Group: "engine"},
	{SPN: 92, PGN: 61443, Start: 3, Bits: 8, Scale: 1, Unit: "%", Max: 125, Name: i18n.T("Engine load at current speed", "Нагрузка двигателя"), Group: "engine"},
	{SPN: 110, PGN: 65262, Start: 1, Bits: 8, Scale: 1, Offset: -40, Unit: "°C", Min: -40, Max: 130, Name: i18n.T("Engine coolant temperature", "Температура охлаждающей жидкости"), Group: "temperature"},
	{SPN: 174, PGN: 65262, Start: 2, Bits: 8, Scale: 1, Offset: -40, Unit: "°C", Min: -40, Max: 120, Name: i18n.T("Fuel temperature", "Температура топлива"), Group: "temperature"},
	{SPN: 175, PGN: 65262, Start: 3, Bits: 16, Scale: 0.03125, Offset: -273, Unit: "°C", Min: -40, Max: 150, Name: i18n.T("Engine oil temperature", "Температура масла"), Group: "temperature"},
	{SPN: 52, PGN: 65262, Start: 7, Bits: 8, Scale: 1, Offset: -40, Unit: "°C", Min: -40, Max: 120, Name: i18n.T("Intercooler temperature", "Температура интеркулера"), Group: "temperature"},
	{SPN: 94, PGN: 65263, Start: 1, Bits: 8, Scale: 4, Unit: "kPa", Max: 1000, Name: i18n.T("Fuel delivery pressure", "Давление подачи топлива"), Group: "fuel"},
	{SPN: 98, PGN: 65263, Start: 3, Bits: 8, Scale: 0.4, Unit: "%", Max: 100, Name: i18n.T("Engine oil level", "Уровень масла"), Group: "engine"},
	{SPN: 100, PGN: 65263, Start: 4, Bits: 8, Scale: 4, Unit: "kPa", Max: 1000, Name: i18n.T("Engine oil pressure", "Давление масла"), Group: "engine"},
	{SPN: 109, PGN: 65263, Start: 7, Bits: 8, Scale: 2, Unit: "kPa", Max: 500, Name: i18n.T("Coolant pressure", "Давление охлаждающей жидкости"), Group: "engine"},
	{SPN: 111, PGN: 65263, Start: 8, Bits: 8, Scale: 0.4, Unit: "%", Max: 100, Name: i18n.T("Coolant level", "Уровень охлаждающей жидкости"), Group: "engine"},
	{SPN: 84, PGN: 65265, Start: 2, Bits: 16, Scale: 1.0 / 256, Unit: "km/h", Max: 150, Name: i18n.T("Wheel-based vehicle speed", "Скорость автомобиля"), Group: "vehicle"},
	{SPN: 70, PGN: 65265, Start: 1, Bit: 2, Bits: 2, Scale: 1, Max: 1, Name: i18n.T("Parking brake set", "Стояночный тормоз включён"), Group: "vehicle"},
	{SPN: 597, PGN: 65265, Start: 4, Bit: 4, Bits: 2, Scale: 1, Max: 1, Name: i18n.T("Brake switch", "Педаль тормоза"), Group: "vehicle"},
	{SPN: 598, PGN: 65265, Start: 4, Bit: 6, Bits: 2, Scale: 1, Max: 1, Name: i18n.T("Clutch switch", "Педаль сцепления"), Group: "vehicle"},
	{SPN: 86, PGN: 65265, Start: 6, Bits: 8, Scale: 1, Unit: "km/h", Max: 150, Name: i18n.T("Cruise control set speed", "Скорость круиз-контроля"), Group: "vehicle"},
	{SPN: 183, PGN: 65266, Start: 1, Bits: 16, Scale: 0.05, Unit: "l/h", Max: 150, Name: i18n.T("Fuel rate", "Расход топлива"), Group: "fuel"},
	{SPN: 184, PGN: 65266, Start: 3, Bits: 16, Scale: 1.0 / 512, Unit: "km/l", Max: 20, Name: i18n.T("Instantaneous fuel economy", "Мгновенная экономичность"), Group: "fuel"},
	{SPN: 51, PGN: 65266, Start: 7, Bits: 8, Scale: 0.4, Unit: "%", Max: 100, Name: i18n.T("Throttle position", "Положение дросселя"), Group: "engine"},
	{SPN: 168, PGN: 65271, Start: 5, Bits: 16, Scale: 0.05, Unit: "V", Max: 32, Name: i18n.T("Battery potential", "Напряжение батареи"), Group: "electrical"},
	{SPN: 167, PGN: 65271, Start: 3, Bits: 16, Scale: 0.05, Unit: "V", Max: 32, Name: i18n.T("Charging system potential", "Напряжение зарядки"), Group: "electrical"},
	{SPN: 115, PGN: 65271, Start: 2, Bits: 8, Scale: 1, Unit: "A", Max: 250, Name: i18n.T("Alternator current", "Ток генератора"), Group: "electrical"},
	{SPN: 108, PGN: 65269, Start: 1, Bits: 8, Scale: 0.5, Unit: "kPa", Max: 125, Name: i18n.T("Barometric pressure", "Атмосферное давление"), Group: "air"},
	{SPN: 171, PGN: 65269, Start: 4, Bits: 16, Scale: 0.03125, Offset: -273, Unit: "°C", Min: -40, Max: 60, Name: i18n.T("Ambient air temperature", "Температура наружного воздуха"), Group: "temperature"},
	{SPN: 172, PGN: 65269, Start: 6, Bits: 8, Scale: 1, Offset: -40, Unit: "°C", Min: -40, Max: 100, Name: i18n.T("Air inlet temperature", "Температура воздуха на впуске"), Group: "temperature"},
	{SPN: 102, PGN: 65270, Start: 2, Bits: 8, Scale: 2, Unit: "kPa", Max: 500, Name: i18n.T("Boost pressure", "Давление наддува"), Group: "air"},
	{SPN: 105, PGN: 65270, Start: 3, Bits: 8, Scale: 1, Offset: -40, Unit: "°C", Min: -40, Max: 150, Name: i18n.T("Intake manifold temperature", "Температура во впускном коллекторе"), Group: "temperature"},
	{SPN: 107, PGN: 65270, Start: 5, Bits: 8, Scale: 0.05, Unit: "kPa", Max: 12, Name: i18n.T("Air filter differential pressure", "Перепад давления на воздушном фильтре"), Group: "air"},
	{SPN: 173, PGN: 65270, Start: 6, Bits: 16, Scale: 0.03125, Offset: -273, Unit: "°C", Min: 0, Max: 800, Name: i18n.T("Exhaust gas temperature", "Температура отработавших газов"), Group: "temperature"},
	{SPN: 96, PGN: 65276, Start: 2, Bits: 8, Scale: 0.4, Unit: "%", Max: 100, Name: i18n.T("Fuel level", "Уровень топлива"), Group: "fuel"},
	{SPN: 247, PGN: 65253, Start: 1, Bits: 32, Scale: 0.05, Unit: "h", Max: 100000, Name: i18n.T("Total engine hours", "Моточасы"), Group: "status"},
	{SPN: 245, PGN: 65248, Start: 5, Bits: 32, Scale: 0.125, Unit: "km", Max: 5e6, Name: i18n.T("Total vehicle distance", "Общий пробег"), Group: "status"},
	{SPN: 250, PGN: 65257, Start: 5, Bits: 32, Scale: 0.5, Unit: "l", Max: 1e7, Name: i18n.T("Total fuel used", "Израсходовано топлива всего"), Group: "fuel"},
	{SPN: 523, PGN: 61445, Start: 4, Bits: 8, Scale: 1, Offset: -125, Unit: "", Min: -3, Max: 18, Name: i18n.T("Current gear", "Текущая передача"), Group: "transmission"},
	{SPN: 524, PGN: 61445, Start: 1, Bits: 8, Scale: 1, Offset: -125, Unit: "", Min: -3, Max: 18, Name: i18n.T("Selected gear", "Выбранная передача"), Group: "transmission"},
	{SPN: 191, PGN: 61442, Start: 2, Bits: 16, Scale: 0.125, Unit: "rpm", Max: 4000, Name: i18n.T("Transmission output shaft speed", "Обороты выходного вала КПП"), Group: "transmission"},
	{SPN: 1624, PGN: 65132, Start: 7, Bits: 16, Scale: 1.0 / 256, Unit: "km/h", Max: 150, Name: i18n.T("Tachograph vehicle speed", "Скорость по тахографу"), Group: "vehicle"},
}

// SPNByNumber finds a table entry.
func SPNByNumber(n uint32) *SPN {
	for i := range SPNs {
		if SPNs[i].SPN == n {
			return &SPNs[i]
		}
	}
	return nil
}

// SPNName returns the name of an SPN (generic text when unknown).
func SPNName(n uint32) i18n.Text {
	if s := SPNByNumber(n); s != nil {
		return s.Name
	}
	if t, ok := extraSPNNames[n]; ok {
		return t
	}
	return i18n.T(fmt.Sprintf("SPN %d", n), fmt.Sprintf("SPN %d", n))
}

// Names of SPNs that often appear in trouble codes.
var extraSPNNames = map[uint32]i18n.Text{
	27:   i18n.T("EGR valve position", "Положение клапана EGR"),
	29:   i18n.T("Accelerator pedal position 2", "Положение педали акселератора 2"),
	157:  i18n.T("Injector metering rail 1 pressure", "Давление в топливной рампе"),
	520:  i18n.T("Retarder torque", "Момент ретардера"),
	629:  i18n.T("Controller #1", "Блок управления #1"),
	636:  i18n.T("Engine position sensor (camshaft)", "Датчик положения распредвала"),
	637:  i18n.T("Engine timing sensor (crankshaft)", "Датчик положения коленвала"),
	639:  i18n.T("J1939 network #1", "Сеть J1939 #1"),
	641:  i18n.T("VGT actuator", "Привод турбины с изменяемой геометрией"),
	651:  i18n.T("Injector cylinder 1", "Форсунка цилиндра 1"),
	652:  i18n.T("Injector cylinder 2", "Форсунка цилиндра 2"),
	653:  i18n.T("Injector cylinder 3", "Форсунка цилиндра 3"),
	654:  i18n.T("Injector cylinder 4", "Форсунка цилиндра 4"),
	655:  i18n.T("Injector cylinder 5", "Форсунка цилиндра 5"),
	656:  i18n.T("Injector cylinder 6", "Форсунка цилиндра 6"),
	1569: i18n.T("Engine protection torque derate", "Ограничение момента для защиты двигателя"),
	3216: i18n.T("Aftertreatment intake NOx", "NOx на входе нейтрализатора"),
	3226: i18n.T("Aftertreatment outlet NOx", "NOx на выходе нейтрализатора"),
	3251: i18n.T("DPF differential pressure", "Перепад давления на сажевом фильтре"),
	4364: i18n.T("SCR conversion efficiency", "Эффективность SCR"),
	5246: i18n.T("SCR operator inducement severity", "Уровень ограничения из-за SCR"),
}

// FMIName describes a failure mode identifier (0-31).
func FMIName(fmi byte) i18n.Text {
	if fmi < byte(len(fmiNames)) && !fmiNames[fmi].Empty() {
		return fmiNames[fmi]
	}
	return i18n.T(fmt.Sprintf("failure mode %d", fmi), fmt.Sprintf("режим отказа %d", fmi))
}

var fmiNames = []i18n.Text{
	i18n.T("data valid but above normal range (most severe)", "значение выше нормы (критично)"),
	i18n.T("data valid but below normal range (most severe)", "значение ниже нормы (критично)"),
	i18n.T("data erratic, intermittent or incorrect", "данные нестабильны или неверны"),
	i18n.T("voltage above normal or shorted to high source", "напряжение выше нормы / замыкание на плюс"),
	i18n.T("voltage below normal or shorted to low source", "напряжение ниже нормы / замыкание на массу"),
	i18n.T("current below normal or open circuit", "ток ниже нормы / обрыв цепи"),
	i18n.T("current above normal or grounded circuit", "ток выше нормы / замыкание на массу"),
	i18n.T("mechanical system not responding or out of adjustment", "механическая система не отвечает / разрегулирована"),
	i18n.T("abnormal frequency, pulse width or period", "неверная частота / длительность импульса"),
	i18n.T("abnormal update rate", "неверная частота обновления"),
	i18n.T("abnormal rate of change", "неверная скорость изменения"),
	i18n.T("root cause not known", "причина не определена"),
	i18n.T("bad intelligent device or component", "неисправен компонент"),
	i18n.T("out of calibration", "нарушена калибровка"),
	i18n.T("special instructions", "особые указания"),
	i18n.T("data valid but above normal range (least severe)", "значение выше нормы (незначительно)"),
	i18n.T("data valid but above normal range (moderately severe)", "значение выше нормы (умеренно)"),
	i18n.T("data valid but below normal range (least severe)", "значение ниже нормы (незначительно)"),
	i18n.T("data valid but below normal range (moderately severe)", "значение ниже нормы (умеренно)"),
	i18n.T("received network data in error", "получены ошибочные данные из сети"),
	i18n.T("data drifted high", "дрейф значения вверх"),
	i18n.T("data drifted low", "дрейф значения вниз"),
	{}, {}, {}, {}, {}, {}, {}, {}, {},
	i18n.T("condition exists", "условие присутствует"),
}

// SourceName names common source addresses (SAE J1939 appendix B).
func SourceName(sa byte) string {
	switch sa {
	case 0:
		return "Engine #1"
	case 1:
		return "Engine #2"
	case 3:
		return "Transmission #1"
	case 4:
		return "Transmission #2"
	case 11:
		return "Brakes (ABS/EBS)"
	case 15:
		return "Retarder, engine"
	case 16:
		return "Retarder, driveline"
	case 17:
		return "Cruise control"
	case 23:
		return "Instrument cluster"
	case 33:
		return "Body controller"
	case 238:
		return "Tachograph"
	case 249:
		return "Off-board diagnostic tool #1"
	case 250:
		return "Off-board diagnostic tool #2"
	}
	return fmt.Sprintf("Controller 0x%02X", sa)
}

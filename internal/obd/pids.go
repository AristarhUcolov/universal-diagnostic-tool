// Package obd implements OBD-II / EOBD diagnostics as defined by SAE
// J1979 (ISO 15031-5): live data (mode 01), freeze frames (02), stored,
// pending and permanent trouble codes (03, 07, 0A), clearing (04) and
// vehicle information (09). It works over any diag.Channel, so the same
// code serves ISO 9141-2, ISO 14230-4, ISO 15765-4 CAN, J1850 through an
// ELM327 or J2534 adapter, and ISO 27145 over DoIP.
package obd

import (
	"fmt"
	"strings"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// ValueDef decodes one value out of a PID's data bytes.
type ValueDef struct {
	Key    string // suffix of the parameter id ("" for single-value PIDs)
	Name   i18n.Text
	Unit   string
	Min    float64
	Max    float64
	Digits int
	Group  string
	F      func(d []byte) (float64, bool)
	Text   func(d []byte) i18n.Text // for enumerations
}

// PIDDef describes one mode 01 PID.
type PIDDef struct {
	PID    byte
	Bytes  int
	Values []ValueDef
}

func a(d []byte) float64  { return float64(d[0]) }
func b(d []byte) float64  { return float64(d[1]) }
func ab(d []byte) float64 { return float64(d[0])*256 + float64(d[1]) }
func cd(d []byte) float64 { return float64(d[2])*256 + float64(d[3]) }

func val(v float64) (float64, bool) { return v, true }

func pct(d []byte) (float64, bool)  { return val(a(d) * 100 / 255) }
func temp(d []byte) (float64, bool) { return val(a(d) - 40) }
func trim(d []byte) (float64, bool) { return val((a(d) - 128) * 100 / 128) }

func one(pid byte, n int, name i18n.Text, unit string, min, max float64, digits int, group string, f func([]byte) (float64, bool)) *PIDDef {
	return &PIDDef{PID: pid, Bytes: n, Values: []ValueDef{{Name: name, Unit: unit, Min: min, Max: max, Digits: digits, Group: group, F: f}}}
}

// o2Name returns the bank/sensor name for O2 sensor number n (1-8) in the
// two-bank layout of PID 13.
func o2Name(n int) (string, string) {
	bank, sensor := (n-1)/4+1, (n-1)%4+1
	return fmt.Sprintf("bank %d sensor %d", bank, sensor), fmt.Sprintf("банк %d, датчик %d", bank, sensor)
}

// PIDs is the table of supported mode 01 PIDs.
var PIDs = map[byte]*PIDDef{}

func add(p *PIDDef) { PIDs[p.PID] = p }

func init() {
	add(&PIDDef{PID: 0x01, Bytes: 4, Values: []ValueDef{
		{Key: "mil", Name: i18n.T("Malfunction indicator lamp (MIL)", "Лампа неисправности (MIL)"), Min: 0, Max: 1, Group: "status",
			F: func(d []byte) (float64, bool) {
				if d[0]&0x80 != 0 {
					return 1, true
				}
				return 0, true
			}},
		{Key: "dtcs", Name: i18n.T("Number of confirmed DTCs", "Количество подтверждённых ошибок"), Min: 0, Max: 127, Group: "status",
			F: func(d []byte) (float64, bool) { return val(float64(d[0] & 0x7F)) }},
	}})
	add(&PIDDef{PID: 0x03, Bytes: 2, Values: []ValueDef{{Name: i18n.T("Fuel system status", "Состояние топливной системы"), Group: "fuel",
		F: func(d []byte) (float64, bool) { return val(a(d)) },
		Text: func(d []byte) i18n.Text {
			switch d[0] {
			case 1:
				return i18n.T("open loop: insufficient engine temperature", "разомкнутая петля: двигатель не прогрет")
			case 2:
				return i18n.T("closed loop (oxygen sensor)", "замкнутая петля (по датчику кислорода)")
			case 4:
				return i18n.T("open loop: load or deceleration fuel cut", "разомкнутая петля: нагрузка или торможение")
			case 8:
				return i18n.T("open loop: system failure", "разомкнутая петля: неисправность")
			case 16:
				return i18n.T("closed loop with a feedback fault", "замкнутая петля с неисправностью обратной связи")
			}
			return i18n.T("not available", "нет данных")
		}}}})
	add(one(0x04, 1, i18n.T("Calculated engine load", "Расчётная нагрузка двигателя"), "%", 0, 100, 1, "engine", pct))
	add(one(0x05, 1, i18n.T("Engine coolant temperature", "Температура охлаждающей жидкости"), "°C", -40, 130, 0, "temperature", temp))
	add(one(0x06, 1, i18n.T("Short term fuel trim, bank 1", "Краткосрочная коррекция топлива, банк 1"), "%", -25, 25, 1, "fuel", trim))
	add(one(0x07, 1, i18n.T("Long term fuel trim, bank 1", "Долгосрочная коррекция топлива, банк 1"), "%", -25, 25, 1, "fuel", trim))
	add(one(0x08, 1, i18n.T("Short term fuel trim, bank 2", "Краткосрочная коррекция топлива, банк 2"), "%", -25, 25, 1, "fuel", trim))
	add(one(0x09, 1, i18n.T("Long term fuel trim, bank 2", "Долгосрочная коррекция топлива, банк 2"), "%", -25, 25, 1, "fuel", trim))
	add(one(0x0A, 1, i18n.T("Fuel pressure (gauge)", "Давление топлива (избыточное)"), "kPa", 0, 765, 0, "fuel",
		func(d []byte) (float64, bool) { return val(3 * a(d)) }))
	add(one(0x0B, 1, i18n.T("Intake manifold absolute pressure", "Абсолютное давление во впускном коллекторе"), "kPa", 0, 255, 0, "air",
		func(d []byte) (float64, bool) { return val(a(d)) }))
	add(one(0x0C, 2, i18n.T("Engine speed", "Обороты двигателя"), "rpm", 0, 8000, 0, "engine",
		func(d []byte) (float64, bool) { return val(ab(d) / 4) }))
	add(one(0x0D, 1, i18n.T("Vehicle speed", "Скорость автомобиля"), "km/h", 0, 250, 0, "vehicle",
		func(d []byte) (float64, bool) { return val(a(d)) }))
	add(one(0x0E, 1, i18n.T("Ignition timing advance (cylinder 1)", "Угол опережения зажигания (цил. 1)"), "°", -64, 64, 1, "ignition",
		func(d []byte) (float64, bool) { return val(a(d)/2 - 64) }))
	add(one(0x0F, 1, i18n.T("Intake air temperature", "Температура впускного воздуха"), "°C", -40, 100, 0, "temperature", temp))
	add(one(0x10, 2, i18n.T("Mass air flow", "Массовый расход воздуха"), "g/s", 0, 300, 2, "air",
		func(d []byte) (float64, bool) { return val(ab(d) / 100) }))
	add(one(0x11, 1, i18n.T("Throttle position", "Положение дроссельной заслонки"), "%", 0, 100, 1, "air", pct))
	add(&PIDDef{PID: 0x12, Bytes: 1, Values: []ValueDef{{Name: i18n.T("Commanded secondary air status", "Состояние подачи вторичного воздуха"), Group: "emissions",
		F: func(d []byte) (float64, bool) { return val(a(d)) },
		Text: func(d []byte) i18n.Text {
			switch d[0] {
			case 1:
				return i18n.T("upstream", "до нейтрализатора")
			case 2:
				return i18n.T("downstream of catalytic converter", "после нейтрализатора")
			case 4:
				return i18n.T("from outside atmosphere or off", "в атмосферу / выключено")
			case 8:
				return i18n.T("pump commanded on for diagnostics", "насос включён для диагностики")
			}
			return i18n.T("not available", "нет данных")
		}}}})
	add(&PIDDef{PID: 0x13, Bytes: 1, Values: []ValueDef{{Name: i18n.T("Oxygen sensors present (2 banks)", "Установленные датчики кислорода (2 банка)"), Group: "o2",
		F:    func(d []byte) (float64, bool) { return val(a(d)) },
		Text: func(d []byte) i18n.Text { return sensorsPresent(d[0], 4) }}}})
	for i := 0; i < 8; i++ {
		pid := byte(0x14 + i)
		en, ru := o2Name(i + 1)
		add(&PIDDef{PID: pid, Bytes: 2, Values: []ValueDef{
			{Key: "v", Name: i18n.T("O2 sensor voltage, "+en, "Напряжение датчика кислорода, "+ru), Unit: "V", Min: 0, Max: 1.275, Digits: 3, Group: "o2",
				F: func(d []byte) (float64, bool) { return val(a(d) / 200) }},
			{Key: "trim", Name: i18n.T("O2 sensor short term trim, "+en, "Коррекция по датчику кислорода, "+ru), Unit: "%", Min: -25, Max: 25, Digits: 1, Group: "o2",
				F: func(d []byte) (float64, bool) {
					if d[1] == 0xFF {
						return 0, false
					}
					return val((b(d) - 128) * 100 / 128)
				}},
		}})
	}
	add(&PIDDef{PID: 0x1C, Bytes: 1, Values: []ValueDef{{Name: i18n.T("OBD standard", "Стандарт OBD"), Group: "status",
		F:    func(d []byte) (float64, bool) { return val(a(d)) },
		Text: func(d []byte) i18n.Text { return i18n.T(obdStandard(d[0]), obdStandard(d[0])) }}}})
	add(&PIDDef{PID: 0x1E, Bytes: 1, Values: []ValueDef{{Name: i18n.T("Power take-off (PTO) active", "Отбор мощности (PTO) включён"), Min: 0, Max: 1, Group: "status",
		F: func(d []byte) (float64, bool) { return val(float64(d[0] & 1)) }}}})
	add(one(0x1F, 2, i18n.T("Run time since engine start", "Время работы с момента пуска"), "s", 0, 65535, 0, "status",
		func(d []byte) (float64, bool) { return val(ab(d)) }))
	add(one(0x21, 2, i18n.T("Distance travelled with MIL on", "Пробег с горящей лампой MIL"), "km", 0, 65535, 0, "status",
		func(d []byte) (float64, bool) { return val(ab(d)) }))
	add(one(0x22, 2, i18n.T("Fuel rail pressure (relative to manifold)", "Давление в топливной рампе (относительно коллектора)"), "kPa", 0, 5177, 1, "fuel",
		func(d []byte) (float64, bool) { return val(0.079 * ab(d)) }))
	add(one(0x23, 2, i18n.T("Fuel rail gauge pressure", "Давление в топливной рампе"), "kPa", 0, 655350, 0, "fuel",
		func(d []byte) (float64, bool) { return val(10 * ab(d)) }))
	for i := 0; i < 8; i++ {
		en, ru := o2Name(i + 1)
		add(&PIDDef{PID: byte(0x24 + i), Bytes: 4, Values: []ValueDef{
			{Key: "lambda", Name: i18n.T("Wide-band O2 equivalence ratio (λ), "+en, "Широкополосный датчик кислорода, λ, "+ru), Min: 0, Max: 2, Digits: 3, Group: "o2",
				F: func(d []byte) (float64, bool) { return val(2 / 65536.0 * ab(d)) }},
			{Key: "v", Name: i18n.T("Wide-band O2 voltage, "+en, "Напряжение широкополосного датчика, "+ru), Unit: "V", Min: 0, Max: 8, Digits: 3, Group: "o2",
				F: func(d []byte) (float64, bool) { return val(8 / 65536.0 * cd(d)) }},
		}})
	}
	add(one(0x2C, 1, i18n.T("Commanded EGR", "Заданная степень рециркуляции (EGR)"), "%", 0, 100, 1, "emissions", pct))
	add(one(0x2D, 1, i18n.T("EGR error", "Ошибка рециркуляции (EGR)"), "%", -100, 100, 1, "emissions", trim))
	add(one(0x2E, 1, i18n.T("Commanded evaporative purge", "Заданная продувка адсорбера"), "%", 0, 100, 1, "emissions", pct))
	add(one(0x2F, 1, i18n.T("Fuel tank level", "Уровень топлива в баке"), "%", 0, 100, 1, "fuel", pct))
	add(one(0x30, 1, i18n.T("Warm-ups since codes cleared", "Прогревов после сброса ошибок"), "", 0, 255, 0, "status",
		func(d []byte) (float64, bool) { return val(a(d)) }))
	add(one(0x31, 2, i18n.T("Distance since codes cleared", "Пробег после сброса ошибок"), "km", 0, 65535, 0, "status",
		func(d []byte) (float64, bool) { return val(ab(d)) }))
	add(one(0x32, 2, i18n.T("Evap system vapour pressure", "Давление паров в системе улавливания"), "Pa", -8192, 8192, 1, "emissions",
		func(d []byte) (float64, bool) { return val(float64(int16(uint16(d[0])<<8|uint16(d[1]))) / 4) }))
	add(one(0x33, 1, i18n.T("Barometric pressure", "Атмосферное давление"), "kPa", 0, 255, 0, "air",
		func(d []byte) (float64, bool) { return val(a(d)) }))
	for i := 0; i < 8; i++ {
		en, ru := o2Name(i + 1)
		add(&PIDDef{PID: byte(0x34 + i), Bytes: 4, Values: []ValueDef{
			{Key: "lambda", Name: i18n.T("Wide-band O2 equivalence ratio (λ), "+en, "Широкополосный датчик кислорода, λ, "+ru), Min: 0, Max: 2, Digits: 3, Group: "o2",
				F: func(d []byte) (float64, bool) { return val(2 / 65536.0 * ab(d)) }},
			{Key: "ma", Name: i18n.T("Wide-band O2 current, "+en, "Ток широкополосного датчика, "+ru), Unit: "mA", Min: -128, Max: 128, Digits: 2, Group: "o2",
				F: func(d []byte) (float64, bool) { return val(cd(d)/256 - 128) }},
		}})
	}
	catNames := [][2]string{{"bank 1 sensor 1", "банк 1, датчик 1"}, {"bank 2 sensor 1", "банк 2, датчик 1"},
		{"bank 1 sensor 2", "банк 1, датчик 2"}, {"bank 2 sensor 2", "банк 2, датчик 2"}}
	for i, n := range catNames {
		add(one(byte(0x3C+i), 2, i18n.T("Catalyst temperature, "+n[0], "Температура нейтрализатора, "+n[1]), "°C", -40, 1000, 0, "emissions",
			func(d []byte) (float64, bool) { return val(ab(d)/10 - 40) }))
	}
	add(one(0x42, 2, i18n.T("Control module voltage", "Напряжение питания блока управления"), "V", 0, 18, 2, "electrical",
		func(d []byte) (float64, bool) { return val(ab(d) / 1000) }))
	add(one(0x43, 2, i18n.T("Absolute load value", "Абсолютная нагрузка"), "%", 0, 300, 1, "engine",
		func(d []byte) (float64, bool) { return val(ab(d) * 100 / 255) }))
	add(one(0x44, 2, i18n.T("Commanded air-fuel equivalence ratio (λ)", "Заданный коэффициент избытка воздуха (λ)"), "", 0, 2, 3, "fuel",
		func(d []byte) (float64, bool) { return val(2 / 65536.0 * ab(d)) }))
	add(one(0x45, 1, i18n.T("Relative throttle position", "Относительное положение дросселя"), "%", 0, 100, 1, "air", pct))
	add(one(0x46, 1, i18n.T("Ambient air temperature", "Температура наружного воздуха"), "°C", -40, 60, 0, "temperature", temp))
	add(one(0x47, 1, i18n.T("Absolute throttle position B", "Абсолютное положение дросселя B"), "%", 0, 100, 1, "air", pct))
	add(one(0x48, 1, i18n.T("Absolute throttle position C", "Абсолютное положение дросселя C"), "%", 0, 100, 1, "air", pct))
	add(one(0x49, 1, i18n.T("Accelerator pedal position D", "Положение педали акселератора D"), "%", 0, 100, 1, "air", pct))
	add(one(0x4A, 1, i18n.T("Accelerator pedal position E", "Положение педали акселератора E"), "%", 0, 100, 1, "air", pct))
	add(one(0x4B, 1, i18n.T("Accelerator pedal position F", "Положение педали акселератора F"), "%", 0, 100, 1, "air", pct))
	add(one(0x4C, 1, i18n.T("Commanded throttle actuator", "Заданное положение привода дросселя"), "%", 0, 100, 1, "air", pct))
	add(one(0x4D, 2, i18n.T("Time run with MIL on", "Время работы с горящей MIL"), "min", 0, 65535, 0, "status",
		func(d []byte) (float64, bool) { return val(ab(d)) }))
	add(one(0x4E, 2, i18n.T("Time since codes cleared", "Время после сброса ошибок"), "min", 0, 65535, 0, "status",
		func(d []byte) (float64, bool) { return val(ab(d)) }))
	add(one(0x50, 4, i18n.T("Maximum mass air flow", "Максимальный расход воздуха"), "g/s", 0, 2550, 0, "air",
		func(d []byte) (float64, bool) { return val(a(d) * 10) }))
	add(&PIDDef{PID: 0x51, Bytes: 1, Values: []ValueDef{{Name: i18n.T("Fuel type", "Тип топлива"), Group: "fuel",
		F:    func(d []byte) (float64, bool) { return val(a(d)) },
		Text: func(d []byte) i18n.Text { return fuelType(d[0]) }}}})
	add(one(0x52, 1, i18n.T("Ethanol fuel percentage", "Содержание этанола"), "%", 0, 100, 1, "fuel", pct))
	add(one(0x53, 2, i18n.T("Absolute evap system vapour pressure", "Абсолютное давление паров топлива"), "kPa", 0, 330, 2, "emissions",
		func(d []byte) (float64, bool) { return val(ab(d) / 200) }))
	add(one(0x54, 2, i18n.T("Evap system vapour pressure", "Давление паров топлива"), "Pa", -32768, 32767, 0, "emissions",
		func(d []byte) (float64, bool) { return val(float64(int16(uint16(d[0])<<8 | uint16(d[1])))) }))
	add(one(0x59, 2, i18n.T("Fuel rail absolute pressure", "Абсолютное давление в топливной рампе"), "kPa", 0, 655350, 0, "fuel",
		func(d []byte) (float64, bool) { return val(10 * ab(d)) }))
	add(one(0x5A, 1, i18n.T("Relative accelerator pedal position", "Относительное положение педали акселератора"), "%", 0, 100, 1, "air", pct))
	add(one(0x5B, 1, i18n.T("Hybrid battery pack remaining life", "Остаточный ресурс тяговой батареи"), "%", 0, 100, 1, "hybrid", pct))
	add(one(0x5C, 1, i18n.T("Engine oil temperature", "Температура моторного масла"), "°C", -40, 160, 0, "temperature", temp))
	add(one(0x5D, 2, i18n.T("Fuel injection timing", "Момент впрыска топлива"), "°", -210, 302, 2, "fuel",
		func(d []byte) (float64, bool) { return val(ab(d)/128 - 210) }))
	add(one(0x5E, 2, i18n.T("Engine fuel rate", "Расход топлива"), "l/h", 0, 100, 2, "fuel",
		func(d []byte) (float64, bool) { return val(ab(d) / 20) }))
	add(one(0x61, 1, i18n.T("Driver's demand engine torque", "Запрошенный водителем момент"), "%", -125, 130, 0, "engine",
		func(d []byte) (float64, bool) { return val(a(d) - 125) }))
	add(one(0x62, 1, i18n.T("Actual engine torque", "Фактический момент двигателя"), "%", -125, 130, 0, "engine",
		func(d []byte) (float64, bool) { return val(a(d) - 125) }))
	add(one(0x63, 2, i18n.T("Engine reference torque", "Номинальный момент двигателя"), "Nm", 0, 65535, 0, "engine",
		func(d []byte) (float64, bool) { return val(ab(d)) }))
	add(&PIDDef{PID: 0x67, Bytes: 3, Values: []ValueDef{
		{Key: "1", Name: i18n.T("Engine coolant temperature sensor 1", "Датчик температуры ОЖ 1"), Unit: "°C", Min: -40, Max: 130, Group: "temperature",
			F: func(d []byte) (float64, bool) { return float64(d[1]) - 40, d[0]&1 != 0 }},
		{Key: "2", Name: i18n.T("Engine coolant temperature sensor 2", "Датчик температуры ОЖ 2"), Unit: "°C", Min: -40, Max: 130, Group: "temperature",
			F: func(d []byte) (float64, bool) { return float64(d[2]) - 40, d[0]&2 != 0 }},
	}})
	add(&PIDDef{PID: 0x7F, Bytes: 13, Values: []ValueDef{{Name: i18n.T("Total engine run time", "Общее время работы двигателя"), Unit: "h", Min: 0, Max: 100000, Digits: 1, Group: "status",
		F: func(d []byte) (float64, bool) {
			s := uint32(d[1])<<24 | uint32(d[2])<<16 | uint32(d[3])<<8 | uint32(d[4])
			return float64(s) / 3600, d[0]&1 != 0
		}}}})
	add(one(0x8E, 1, i18n.T("Engine friction torque", "Момент трения двигателя"), "%", -125, 130, 0, "engine",
		func(d []byte) (float64, bool) { return val(a(d) - 125) }))
	add(one(0xA2, 2, i18n.T("Cylinder fuel rate", "Цикловая подача топлива"), "mg/stroke", 0, 2048, 1, "fuel",
		func(d []byte) (float64, bool) { return val(ab(d) / 32) }))
	add(one(0xA6, 4, i18n.T("Odometer", "Пробег (одометр)"), "km", 0, 1e7, 1, "vehicle",
		func(d []byte) (float64, bool) {
			v := uint32(d[0])<<24 | uint32(d[1])<<16 | uint32(d[2])<<8 | uint32(d[3])
			return float64(v) / 10, true
		}))
}

// PIDSize returns the data length of a mode 01 PID, or 0 if unknown.
func PIDSize(pid byte) int {
	if pid%0x20 == 0 {
		return 4
	}
	if p, found := PIDs[pid]; found {
		return p.Bytes
	}
	if n, found := extraSizes[pid]; found {
		return n
	}
	return 0
}

// Sizes of PIDs that are not decoded but may appear in multi-PID answers.
var extraSizes = map[byte]int{
	0x02: 2, 0x1D: 1, 0x41: 4, 0x4F: 4, 0x55: 2, 0x56: 2, 0x57: 2, 0x58: 2, 0x5F: 1,
	0x64: 5, 0x65: 2, 0x66: 5, 0x68: 7, 0x69: 7, 0x6A: 5, 0x6B: 5, 0x6C: 5, 0x6D: 6,
	0x6E: 5, 0x6F: 3, 0x70: 9, 0x71: 5, 0x72: 5, 0x73: 5, 0x74: 5, 0x75: 7, 0x76: 7,
	0x77: 5, 0x78: 9, 0x79: 9, 0x7A: 7, 0x7B: 7, 0x7C: 9, 0x7D: 1, 0x7E: 1,
}

// ParamID returns the program-wide id of a PID value.
func ParamID(pid byte, key string) string {
	if key == "" {
		return fmt.Sprintf("obd.%02X", pid)
	}
	return fmt.Sprintf("obd.%02X.%s", pid, key)
}

// ParseParamID splits "obd.14.v" into the PID and key.
func ParseParamID(id string) (byte, string, bool) {
	if !strings.HasPrefix(id, "obd.") {
		return 0, "", false
	}
	rest := id[4:]
	key := ""
	if i := strings.IndexByte(rest, '.'); i >= 0 {
		rest, key = rest[:i], rest[i+1:]
	}
	var pid byte
	if _, err := fmt.Sscanf(rest, "%02X", &pid); err != nil {
		return 0, "", false
	}
	return pid, key, true
}

func sensorsPresent(mask byte, perBank int) i18n.Text {
	var en, ru []string
	for i := 0; i < 8; i++ {
		if mask&(1<<i) != 0 {
			bank, sensor := i/perBank+1, i%perBank+1
			en = append(en, fmt.Sprintf("B%dS%d", bank, sensor))
			ru = append(ru, fmt.Sprintf("Б%dД%d", bank, sensor))
		}
	}
	if len(en) == 0 {
		return i18n.T("none", "нет")
	}
	return i18n.T(strings.Join(en, ", "), strings.Join(ru, ", "))
}

func obdStandard(v byte) string {
	names := map[byte]string{
		1: "OBD-II (CARB)", 2: "OBD (EPA)", 3: "OBD and OBD-II", 4: "OBD-I", 5: "not OBD compliant",
		6: "EOBD (Europe)", 7: "EOBD and OBD-II", 8: "EOBD and OBD", 9: "EOBD, OBD and OBD-II",
		10: "JOBD (Japan)", 11: "JOBD and OBD-II", 12: "JOBD and EOBD", 13: "JOBD, EOBD and OBD-II",
		17: "EMD", 18: "EMD+", 19: "HD OBD-C", 20: "HD OBD", 21: "WWH OBD", 23: "HD EOBD-I",
		24: "HD EOBD-I N", 25: "HD EOBD-II", 26: "HD EOBD-II N", 28: "OBDBr-1", 29: "OBDBr-2",
		30: "KOBD (Korea)", 31: "IOBD I (India)", 32: "IOBD II (India)", 33: "HD EOBD-IV",
	}
	if s, ok := names[v]; ok {
		return s
	}
	return fmt.Sprintf("code %d", v)
}

func fuelType(v byte) i18n.Text {
	switch v {
	case 1:
		return i18n.T("gasoline", "бензин")
	case 2:
		return i18n.T("methanol", "метанол")
	case 3:
		return i18n.T("ethanol", "этанол")
	case 4:
		return i18n.T("diesel", "дизель")
	case 5:
		return i18n.T("LPG", "пропан (LPG)")
	case 6:
		return i18n.T("CNG", "метан (CNG)")
	case 7:
		return i18n.T("propane", "пропан")
	case 8:
		return i18n.T("electric", "электричество")
	case 9:
		return i18n.T("bi-fuel gasoline", "двухтопливный, бензин")
	case 12:
		return i18n.T("bi-fuel LPG", "двухтопливный, LPG")
	case 13:
		return i18n.T("bi-fuel CNG", "двухтопливный, CNG")
	case 17:
		return i18n.T("hybrid gasoline", "гибрид, бензин")
	case 19:
		return i18n.T("hybrid diesel", "гибрид, дизель")
	case 20:
		return i18n.T("hybrid electric", "гибрид, электро")
	}
	return i18n.T(fmt.Sprintf("type %d", v), fmt.Sprintf("тип %d", v))
}

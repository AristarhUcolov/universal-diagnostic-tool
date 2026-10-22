// Package dtc models diagnostic trouble codes from every protocol the
// program speaks (OBD-II, KWP2000, UDS, KW1281, J1939 and the legacy
// Russian ECU protocols) and looks up their descriptions.
package dtc

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// DTC is one trouble code in a protocol-independent form.
type DTC struct {
	Code      string            `json:"code"`             // P0130, U0100, 17965, SPN 110 FMI 0, 053
	Raw       string            `json:"raw,omitempty"`    // raw bytes in hex
	Status    byte              `json:"status"`           // raw status byte
	Flags     []string          `json:"flags,omitempty"`  // decoded status flags (keys, see FlagNames)
	Active    bool              `json:"active"`           // present now
	Pending   bool              `json:"pending"`          // detected, not yet confirmed
	Confirmed bool              `json:"confirmed"`        // stored / confirmed
	Permanent bool              `json:"permanent"`        // OBD permanent code
	MIL       bool              `json:"mil"`              // requests the warning lamp
	Desc      i18n.Text         `json:"desc,omitempty"`   // description
	Detail    i18n.Text         `json:"detail,omitempty"` // failure type, FMI, fault symptom
	Known     bool              `json:"known"`            // description found in the database
	Source    string            `json:"source,omitempty"` // ECU name / address
	Protocol  string            `json:"protocol,omitempty"`
	Family    string            `json:"family,omitempty"` // description family used
	Extra     map[string]string `json:"extra,omitempty"`
}

// Status flag keys. The user interface translates them.
const (
	FlagTestFailed       = "testFailed"
	FlagFailedThisCycle  = "testFailedThisCycle"
	FlagPending          = "pending"
	FlagConfirmed        = "confirmed"
	FlagNotCompleteClear = "testNotCompletedSinceClear"
	FlagFailedSinceClear = "testFailedSinceClear"
	FlagNotCompleteCycle = "testNotCompletedThisCycle"
	FlagMIL              = "mil"
	FlagActive           = "active"
	FlagHistory          = "history"
	FlagIntermittent     = "intermittent"
	FlagPermanent        = "permanent"
	FlagNotReady         = "notReady"
)

// FlagNames gives readable names for the status flag keys.
var FlagNames = map[string]i18n.Text{
	FlagTestFailed:       i18n.T("test failed", "тест не пройден"),
	FlagFailedThisCycle:  i18n.T("failed this operation cycle", "ошибка в текущем цикле"),
	FlagPending:          i18n.T("pending", "ожидающий"),
	FlagConfirmed:        i18n.T("confirmed", "подтверждён"),
	FlagNotCompleteClear: i18n.T("test not completed since clear", "тест не завершён после сброса"),
	FlagFailedSinceClear: i18n.T("failed since clear", "ошибка после сброса"),
	FlagNotCompleteCycle: i18n.T("test not completed this cycle", "тест не завершён в цикле"),
	FlagMIL:              i18n.T("warning lamp on", "лампа неисправности"),
	FlagActive:           i18n.T("active (present now)", "активна (есть сейчас)"),
	FlagHistory:          i18n.T("stored (not present now)", "сохранена (сейчас нет)"),
	FlagIntermittent:     i18n.T("intermittent", "периодическая"),
	FlagPermanent:        i18n.T("permanent", "постоянная"),
	FlagNotReady:         i18n.T("test not complete", "тест не завершён"),
}

// SAE decodes the two-byte SAE J2012 / ISO 15031-6 representation used by
// OBD-II, KWP2000 and UDS: the top two bits select P/C/B/U, the next two
// the first digit, then three hex digits.
func SAE(hi, lo byte) string {
	letters := "PCBU"
	return fmt.Sprintf("%c%d%X%X%X", letters[hi>>6], (hi>>4)&0x03, hi&0x0F, lo>>4, lo&0x0F)
}

// ParseSAE is the inverse of SAE. It accepts codes such as "P0130" or
// "u0100".
func ParseSAE(code string) (byte, byte, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 5 {
		return 0, 0, fmt.Errorf("DTC %q must have 5 characters", code)
	}
	sys := strings.IndexByte("PCBU", code[0])
	if sys < 0 {
		return 0, 0, fmt.Errorf("DTC %q must start with P, C, B or U", code)
	}
	d1 := code[1] - '0'
	if d1 > 3 {
		return 0, 0, fmt.Errorf("DTC %q: second character must be 0-3", code)
	}
	rest, err := strconv.ParseUint(code[2:], 16, 16)
	if err != nil {
		return 0, 0, fmt.Errorf("DTC %q: %v", code, err)
	}
	hi := byte(sys)<<6 | d1<<4 | byte(rest>>8)
	return hi, byte(rest), nil
}

// IsSAE reports whether the code looks like P/C/B/U + 4 hex digits.
func IsSAE(code string) bool {
	_, _, err := ParseSAE(code)
	return err == nil
}

// ApplyKWPStatus fills the flags from an ISO 14230-3 statusOfDTC byte:
// bit 7 warning lamp, bits 6-5 storage state (01 stored but not present,
// 10 maturing/intermittent, 11 present), bit 4 test not complete,
// bits 3-0 fault symptom.
func ApplyKWPStatus(d *DTC, status byte) {
	d.Status = status
	d.Flags = d.Flags[:0]
	if status&0x80 != 0 {
		d.MIL = true
		d.Flags = append(d.Flags, FlagMIL)
	}
	switch (status >> 5) & 0x03 {
	case 0x01:
		d.Confirmed = true
		d.Flags = append(d.Flags, FlagHistory)
	case 0x02:
		d.Pending = true
		d.Flags = append(d.Flags, FlagIntermittent)
	case 0x03:
		d.Active, d.Confirmed = true, true
		d.Flags = append(d.Flags, FlagActive)
	}
	if status&0x10 != 0 {
		d.Flags = append(d.Flags, FlagNotReady)
	}
}

// ApplyUDSStatus fills the flags from an ISO 14229 DTC status byte.
func ApplyUDSStatus(d *DTC, status byte) {
	d.Status = status
	d.Flags = d.Flags[:0]
	names := []string{FlagTestFailed, FlagFailedThisCycle, FlagPending, FlagConfirmed,
		FlagNotCompleteClear, FlagFailedSinceClear, FlagNotCompleteCycle, FlagMIL}
	for bit, name := range names {
		if status&(1<<bit) != 0 {
			d.Flags = append(d.Flags, name)
		}
	}
	d.Active = status&0x01 != 0
	d.Pending = status&0x04 != 0
	d.Confirmed = status&0x08 != 0
	d.MIL = status&0x80 != 0
	if !d.Active && d.Confirmed {
		d.Flags = append(d.Flags, FlagHistory)
	}
}

// VAGCode converts a KW1281 five-digit fault code to text. Codes from
// 16384 encode SAE P-codes: 16384 + 1024*first digit + remaining three
// digits (16684 = P0300, 17965 = P1557).
func VAGCode(code uint16) (text string, sae string) {
	text = fmt.Sprintf("%05d", code)
	if code >= 16384 && code < 16384+4096 {
		v := int(code) - 16384
		d1, rest := v/1024, v%1024
		if rest <= 999 {
			sae = fmt.Sprintf("P%d%03d", d1, rest)
		}
	}
	return text, sae
}

// SystemOf returns a short description of the part of the vehicle a SAE
// code belongs to, used when the database has no entry for the code.
func SystemOf(code string) i18n.Text {
	if len(code) != 5 {
		return nil
	}
	// SAE J2012: x0xxx is generic; P2xxx is generic for powertrain only;
	// x3xxx is generic except P3000-P33FF.
	generic := code[1] == '0' ||
		(code[0] == 'P' && code[1] == '2') ||
		(code[1] == '3' && (code[0] != 'P' || code[2] >= '4'))
	kind := i18n.T("manufacturer-specific code", "код производителя")
	if generic {
		kind = i18n.T("generic code", "общий код")
	}
	var area i18n.Text
	switch code[0] {
	case 'P':
		switch code[2] {
		case '0', '1', '2':
			area = i18n.T("fuel and air metering", "топливоподача и учёт воздуха")
		case '3':
			area = i18n.T("ignition system or misfire", "зажигание или пропуски воспламенения")
		case '4':
			area = i18n.T("auxiliary emission controls", "системы снижения токсичности")
		case '5':
			area = i18n.T("vehicle speed, idle control, auxiliary inputs", "скорость, холостой ход, доп. входы")
		case '6':
			area = i18n.T("control module and outputs", "блок управления и выходные цепи")
		case '7', '8', '9':
			area = i18n.T("transmission", "трансмиссия")
		case 'A', 'B', 'C':
			area = i18n.T("hybrid / electric propulsion", "гибридный / электрический привод")
		default:
			area = i18n.T("powertrain", "силовой агрегат")
		}
	case 'C':
		area = i18n.T("chassis", "шасси")
	case 'B':
		area = i18n.T("body", "кузов")
	case 'U':
		area = i18n.T("network communication", "обмен данными по сети")
	}
	if area == nil {
		return kind
	}
	return i18n.T(area.Get("en")+", "+kind.Get("en"), area.Get("ru")+", "+kind.Get("ru"))
}

// FailureType names the UDS / SAE J2012-DA failure type byte (the third
// byte of a three-byte DTC).
func FailureType(ftb byte) i18n.Text {
	if t, ok := ftbNames[ftb]; ok {
		return t
	}
	return i18n.T(fmt.Sprintf("failure type 0x%02X", ftb), fmt.Sprintf("тип неисправности 0x%02X", ftb))
}

var ftbNames = map[byte]i18n.Text{
	0x00: i18n.T("no sub-type information", "без уточнения"),
	0x01: i18n.T("general electrical failure", "общая электрическая неисправность"),
	0x02: i18n.T("general signal failure", "общая неисправность сигнала"),
	0x03: i18n.T("FM/PWM failure", "неисправность ЧМ/ШИМ сигнала"),
	0x04: i18n.T("system internal failure", "внутренняя неисправность системы"),
	0x05: i18n.T("system programming failure", "ошибка программирования системы"),
	0x06: i18n.T("algorithm based failure", "неисправность по алгоритму"),
	0x07: i18n.T("mechanical failure", "механическая неисправность"),
	0x08: i18n.T("bus signal / message failure", "неисправность сигнала/сообщения шины"),
	0x09: i18n.T("component failure", "неисправность компонента"),
	0x11: i18n.T("circuit short to ground", "замыкание цепи на массу"),
	0x12: i18n.T("circuit short to battery", "замыкание цепи на питание"),
	0x13: i18n.T("circuit open", "обрыв цепи"),
	0x14: i18n.T("circuit short to ground or open", "замыкание на массу или обрыв"),
	0x15: i18n.T("circuit short to battery or open", "замыкание на питание или обрыв"),
	0x16: i18n.T("circuit voltage below threshold", "напряжение цепи ниже порога"),
	0x17: i18n.T("circuit voltage above threshold", "напряжение цепи выше порога"),
	0x18: i18n.T("circuit current below threshold", "ток цепи ниже порога"),
	0x19: i18n.T("circuit current above threshold", "ток цепи выше порога"),
	0x1A: i18n.T("circuit resistance below threshold", "сопротивление цепи ниже порога"),
	0x1B: i18n.T("circuit resistance above threshold", "сопротивление цепи выше порога"),
	0x1C: i18n.T("circuit voltage out of range", "напряжение цепи вне диапазона"),
	0x1D: i18n.T("circuit current out of range", "ток цепи вне диапазона"),
	0x1E: i18n.T("circuit resistance out of range", "сопротивление цепи вне диапазона"),
	0x1F: i18n.T("circuit intermittent", "периодическое нарушение цепи"),
	0x21: i18n.T("signal amplitude below minimum", "амплитуда сигнала ниже минимума"),
	0x22: i18n.T("signal amplitude above maximum", "амплитуда сигнала выше максимума"),
	0x23: i18n.T("signal stuck low", "сигнал застрял на низком уровне"),
	0x24: i18n.T("signal stuck high", "сигнал застрял на высоком уровне"),
	0x25: i18n.T("signal shape / waveform failure", "неверная форма сигнала"),
	0x26: i18n.T("signal rate of change below threshold", "скорость изменения сигнала ниже порога"),
	0x27: i18n.T("signal rate of change above threshold", "скорость изменения сигнала выше порога"),
	0x28: i18n.T("signal bias level out of range / zero adjustment failure", "смещение сигнала вне диапазона"),
	0x29: i18n.T("signal invalid", "недостоверный сигнал"),
	0x2F: i18n.T("signal erratic", "нестабильный сигнал"),
	0x31: i18n.T("no signal", "нет сигнала"),
	0x32: i18n.T("signal low time below minimum", "время низкого уровня меньше минимума"),
	0x33: i18n.T("signal low time above maximum", "время низкого уровня больше максимума"),
	0x34: i18n.T("signal high time below minimum", "время высокого уровня меньше минимума"),
	0x35: i18n.T("signal high time above maximum", "время высокого уровня больше максимума"),
	0x36: i18n.T("signal frequency too low", "частота сигнала слишком низкая"),
	0x37: i18n.T("signal frequency too high", "частота сигнала слишком высокая"),
	0x38: i18n.T("signal frequency incorrect", "неверная частота сигнала"),
	0x39: i18n.T("too few pulses", "слишком мало импульсов"),
	0x3A: i18n.T("too many pulses", "слишком много импульсов"),
	0x41: i18n.T("general checksum failure", "ошибка контрольной суммы"),
	0x42: i18n.T("general memory failure", "неисправность памяти"),
	0x43: i18n.T("special memory failure", "неисправность специальной памяти"),
	0x44: i18n.T("data memory failure", "неисправность памяти данных"),
	0x45: i18n.T("program memory failure", "неисправность памяти программы"),
	0x46: i18n.T("calibration / parameter memory failure", "неисправность памяти калибровок"),
	0x47: i18n.T("watchdog / safety microcontroller failure", "сбой сторожевого таймера / контроллера безопасности"),
	0x48: i18n.T("supervision software failure", "сбой контрольного ПО"),
	0x49: i18n.T("internal electronic failure", "внутренняя неисправность электроники"),
	0x4A: i18n.T("incorrect component installed", "установлен неверный компонент"),
	0x4B: i18n.T("over temperature", "перегрев"),
	0x51: i18n.T("not programmed", "не запрограммирован"),
	0x52: i18n.T("not activated", "не активирован"),
	0x53: i18n.T("deactivated", "деактивирован"),
	0x54: i18n.T("missing calibration", "нет калибровки"),
	0x55: i18n.T("not configured", "не сконфигурирован"),
	0x61: i18n.T("signal calculation failure", "ошибка вычисления сигнала"),
	0x62: i18n.T("signal compare failure", "ошибка сравнения сигналов"),
	0x63: i18n.T("circuit / component protection time-out", "срабатывание защиты цепи по времени"),
	0x64: i18n.T("signal plausibility failure", "неправдоподобный сигнал"),
	0x65: i18n.T("signal has too few transitions / events", "слишком мало переходов сигнала"),
	0x66: i18n.T("signal has too many transitions / events", "слишком много переходов сигнала"),
	0x67: i18n.T("signal incorrect after event", "неверный сигнал после события"),
	0x68: i18n.T("event information", "информация о событии"),
	0x71: i18n.T("actuator stuck", "исполнительный механизм заклинил"),
	0x72: i18n.T("actuator stuck open", "механизм заклинил в открытом положении"),
	0x73: i18n.T("actuator stuck closed", "механизм заклинил в закрытом положении"),
	0x74: i18n.T("actuator slipping", "проскальзывание механизма"),
	0x75: i18n.T("emergency position not reachable", "аварийное положение недостижимо"),
	0x76: i18n.T("wrong mounting position", "неверное положение установки"),
	0x77: i18n.T("commanded position not reachable", "заданное положение недостижимо"),
	0x78: i18n.T("alignment or adjustment incorrect", "неверная регулировка"),
	0x79: i18n.T("mechanical linkage failure", "неисправность механической связи"),
	0x7A: i18n.T("fluid leak or seal failure", "утечка жидкости / неисправность уплотнения"),
	0x7B: i18n.T("low fluid level", "низкий уровень жидкости"),
	0x81: i18n.T("invalid serial data received", "получены неверные данные"),
	0x82: i18n.T("alive / sequence counter incorrect", "неверный счётчик сообщений"),
	0x83: i18n.T("signal protection calculation incorrect", "неверная контрольная сумма сигнала"),
	0x84: i18n.T("signal below allowable range", "сигнал ниже допустимого диапазона"),
	0x85: i18n.T("signal above allowable range", "сигнал выше допустимого диапазона"),
	0x86: i18n.T("signal invalid", "недопустимый сигнал"),
	0x87: i18n.T("missing message", "сообщение отсутствует"),
	0x88: i18n.T("bus off", "шина отключена (bus off)"),
	0x8F: i18n.T("erratic", "нестабильно"),
	0x91: i18n.T("parametric", "параметрическая неисправность"),
	0x92: i18n.T("performance or incorrect operation", "неверная работа / характеристика"),
	0x93: i18n.T("no operation", "не работает"),
	0x94: i18n.T("unexpected operation", "неожиданная работа"),
	0x95: i18n.T("incorrect assembly", "неправильная сборка"),
	0x96: i18n.T("component internal failure", "внутренняя неисправность компонента"),
	0x97: i18n.T("component or system operation obstructed or blocked", "работа компонента заблокирована"),
	0x98: i18n.T("component or system over temperature", "перегрев компонента или системы"),
}

// VAGFaultType names the KW1281 fault elaboration (the third byte of a
// KW1281 fault record, lower seven bits). Bit 7 marks an intermittent
// ("sporadic") fault.
func VAGFaultType(b byte) i18n.Text {
	if t, ok := vagTypes[b&0x7F]; ok {
		return t
	}
	return i18n.T(fmt.Sprintf("fault type %02d", b&0x7F), fmt.Sprintf("тип неисправности %02d", b&0x7F))
}

var vagTypes = map[byte]i18n.Text{
	0:  i18n.T("-", "-"),
	1:  i18n.T("signal shorted to plus", "замыкание сигнала на плюс"),
	2:  i18n.T("signal shorted to ground", "замыкание сигнала на массу"),
	3:  i18n.T("no signal", "нет сигнала"),
	4:  i18n.T("mechanical malfunction", "механическая неисправность"),
	5:  i18n.T("input open", "обрыв входа"),
	6:  i18n.T("signal too high", "сигнал слишком высокий"),
	7:  i18n.T("signal too low", "сигнал слишком низкий"),
	8:  i18n.T("control limit surpassed", "превышен предел регулирования"),
	9:  i18n.T("adaptation limit surpassed", "превышен предел адаптации"),
	10: i18n.T("adaptation limit not reached", "не достигнут предел адаптации"),
	11: i18n.T("control limit not reached", "не достигнут предел регулирования"),
	12: i18n.T("adaptation limit (mul) exceeded", "превышен предел адаптации (мульт.)"),
	13: i18n.T("adaptation limit (mul) not reached", "не достигнут предел адаптации (мульт.)"),
	14: i18n.T("adaptation limit (add) exceeded", "превышен предел адаптации (адд.)"),
	15: i18n.T("adaptation limit (add) not reached", "не достигнут предел адаптации (адд.)"),
	16: i18n.T("signal outside specifications", "сигнал вне допуска"),
	17: i18n.T("control difference", "отклонение регулирования"),
	18: i18n.T("upper limit", "верхний предел"),
	19: i18n.T("lower limit", "нижний предел"),
	20: i18n.T("malfunction in basic setting", "ошибка базовой настройки"),
	21: i18n.T("front pressure build-up time too long", "долгое нарастание давления спереди"),
	22: i18n.T("front pressure reducing time too long", "долгий сброс давления спереди"),
	23: i18n.T("rear pressure build-up time too long", "долгое нарастание давления сзади"),
	24: i18n.T("rear pressure reducing time too long", "долгий сброс давления сзади"),
	25: i18n.T("unknown switch condition", "неизвестное состояние выключателя"),
	26: i18n.T("output open", "обрыв выхода"),
	27: i18n.T("implausible signal", "неправдоподобный сигнал"),
	28: i18n.T("short to plus", "замыкание на плюс"),
	29: i18n.T("short to ground", "замыкание на массу"),
	30: i18n.T("open or short to plus", "обрыв или замыкание на плюс"),
	31: i18n.T("open or short to ground", "обрыв или замыкание на массу"),
	32: i18n.T("short to other wire", "замыкание на другой провод"),
	33: i18n.T("blocked or no voltage", "заблокирован или нет напряжения"),
	34: i18n.T("speed deviation too high", "большое отклонение скорости"),
	35: i18n.T("closed", "замкнут"),
	36: i18n.T("short circuit", "короткое замыкание"),
	37: i18n.T("connector", "разъём"),
	38: i18n.T("leaking", "утечка"),
}

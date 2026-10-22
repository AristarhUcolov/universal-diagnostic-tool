// Package repair holds the "what to check" advice shown next to trouble
// codes: what the part does, the usual symptoms and causes, and the
// order in which to check them, in English and Russian. The advice is
// general on purpose; the repair manual of the car has the final word.
//
// A code is matched to advice by its SAE number range (P0130-P0167 are
// oxygen sensors and so on) or, for manufacturer codes such as the
// three-digit Микас codes, by keywords in its description.
package repair

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// Hint is the advice for one component or fault.
type Hint struct {
	ID string `json:"id"`
	// Severity: "stop" (do not keep driving), "soon" (repair soon),
	// "info" (can wait).
	Severity string      `json:"severity"`
	Title    i18n.Text   `json:"title"`
	What     i18n.Text   `json:"what"`
	Symptoms i18n.Text   `json:"symptoms"`
	Causes   []i18n.Text `json:"causes"`
	Checks   []i18n.Text `json:"checks"`
}

//go:embed hints.json
var hintsJSON []byte

var (
	once  sync.Once
	hints []*Hint
	byID  map[string]*Hint
)

func load() {
	once.Do(func() {
		if err := json.Unmarshal(hintsJSON, &hints); err != nil {
			panic("repair: hints.json: " + err.Error())
		}
		byID = map[string]*Hint{}
		for _, h := range hints {
			byID[h.ID] = h
		}
	})
}

// All returns every hint in file order.
func All() []*Hint {
	load()
	return hints
}

// Get returns a hint by id (nil when unknown).
func Get(id string) *Hint {
	load()
	return byID[id]
}

// saeRanges maps powertrain code numbers to advice.
var saeRanges = []struct {
	lo, hi int
	id     string
}{
	{0x100, 0x104, "maf"},
	{0x105, 0x109, "map"},
	{0x110, 0x114, "iat"},
	{0x115, 0x119, "ect"},
	{0x217, 0x217, "overheat"},
	{0x120, 0x124, "tps"},
	{0x125, 0x125, "thermostat"},
	{0x128, 0x128, "thermostat"},
	{0x135, 0x135, "o2heater"},
	{0x141, 0x141, "o2heater"},
	{0x155, 0x155, "o2heater"},
	{0x161, 0x161, "o2heater"},
	{0x130, 0x167, "o2"},
	{0x171, 0x171, "lean"},
	{0x174, 0x174, "lean"},
	{0x172, 0x172, "rich"},
	{0x175, 0x175, "rich"},
	{0x200, 0x212, "injector"},
	{0x220, 0x229, "tps"},
	{0x230, 0x232, "fuelpump"},
	{0x261, 0x272, "injector"},
	{0x300, 0x312, "misfire"},
	{0x325, 0x334, "knock"},
	{0x335, 0x339, "ckp"},
	{0x340, 0x349, "cmp"},
	{0x350, 0x362, "coil"},
	{0x400, 0x409, "egr"},
	{0x420, 0x434, "catalyst"},
	{0x440, 0x459, "evap"},
	{0x480, 0x483, "fan"},
	{0x500, 0x503, "vss"},
	{0x505, 0x509, "iac"},
	{0x560, 0x563, "voltage"},
	{0x600, 0x606, "ecu"},
	{0x650, 0x650, "mil"},
}

// keywords match descriptions of manufacturer codes; the first match
// wins, so the more specific ones come first.
var keywords = []struct {
	re *regexp.Regexp
	id string
}{
	{regexp.MustCompile(`(?i)самодиагностик|self-diagnos`), "selftest"},
	{regexp.MustCompile(`(?i)перегрев|overheat|over temperature`), "overheat"},
	{regexp.MustCompile(`(?i)(нагрев|подогрев|heater).*(кислород|лямбд|oxygen|o2)|(кислород|лямбд|oxygen|o2).*(нагрев|подогрев|heater)`), "o2heater"},
	{regexp.MustCompile(`(?i)кислород|лямбд|oxygen|\bo2\b`), "o2"},
	{regexp.MustCompile(`(?i)корректор\p{L}* со|со-потенциометр|co potentiometer|co trim`), "copot"},
	{regexp.MustCompile(`(?i)пропуск\p{L}* воспламен|misfire`), "misfire"},
	{regexp.MustCompile(`(?i)детонац|knock`), "knock"},
	// Idle speed codes mention the crankshaft, so they come before it.
	{regexp.MustCompile(`(?i)холост\p{L}* ход|рдв|регулятор\p{L}* хх|idle (air|speed)`), "iac"},
	{regexp.MustCompile(`(?i)превысил\p{L}* максимум|overspeed`), "generic"},
	{regexp.MustCompile(`(?i)коленчат|дпкв|crankshaft`), "ckp"},
	{regexp.MustCompile(`(?i)распредел|датчик\p{L}* фаз|camshaft`), "cmp"},
	{regexp.MustCompile(`(?i)расход\p{L}* воздуха|дмрв|mass air|air flow`), "maf"},
	{regexp.MustCompile(`(?i)абсолютного давления|manifold (absolute )?pressure`), "map"},
	{regexp.MustCompile(`(?i)температур\p{L}* (охлажд|ож)|coolant temp`), "ect"},
	{regexp.MustCompile(`(?i)температур\p{L}* (воздуха|на впуске)|intake air temp|air temperature`), "iat"},
	{regexp.MustCompile(`(?i)рециркуляц|\begr\b`), "egr"},
	{regexp.MustCompile(`(?i)дроссел|throttle position`), "tps"},
	{regexp.MustCompile(`(?i)форсунк|injector`), "injector"},
	{regexp.MustCompile(`(?i)бензонасос|топливн\p{L}* насос|fuel pump`), "fuelpump"},
	{regexp.MustCompile(`(?i)катушк|зажигани|формировател\p{L}* высокого напряжения|ignition`), "coil"},
	{regexp.MustCompile(`(?i)адсорбер|продувки|canister|purge|evap`), "evap"},
	{regexp.MustCompile(`(?i)нейтрализатор|катализатор|catalyst`), "catalyst"},
	{regexp.MustCompile(`(?i)вентилятор|\bfan\b`), "fan"},
	{regexp.MustCompile(`(?i)скорост\p{L}* (автомобиля|движения)|vehicle speed`), "vss"},
	{regexp.MustCompile(`(?i)иммоб|immobili|электронный ключ|transponder`), "immo"},
	{regexp.MustCompile(`(?i)бедн|too lean|\blean\b`), "lean"},
	{regexp.MustCompile(`(?i)богат|too rich|\brich\b`), "rich"},
	{regexp.MustCompile(`(?i)лампы|лампа|lamp|\bmil\b`), "mil"},
	// "Short to battery voltage in the relay circuit" is about the
	// circuit, not the supply, so outputs come before the voltage.
	{regexp.MustCompile(`(?i)цепи (главного )?реле|реле кондиционера|эпхх|тахометр|расходомер|relay|tachometer`), "outputcircuit"},
	{regexp.MustCompile(`(?i)напряжени|бортов\p{L}* сет|voltage|battery`), "voltage"},
	{regexp.MustCompile(`(?i)\bcan\b|связ|communication|lost`), "network"},
	{regexp.MustCompile(`(?i)озу|пзу|\bram\b|\brom\b|eeprom|контрольн\p{L}* сумм|checksum|блок\p{L}* управления|контроллер|control (unit|module)`), "ecu"},
}

// Match returns the id of the advice for a code ("generic" when nothing
// specific is known). desc is the code's description, used for
// manufacturer codes.
func Match(code string, desc i18n.Text) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) == 5 && code[0] == 'U' {
		return "network"
	}
	if len(code) == 5 && code[0] == 'P' && code[1] == '0' {
		if n, err := strconv.ParseUint(code[2:], 16, 16); err == nil {
			for _, r := range saeRanges {
				if int(n) >= r.lo && int(n) <= r.hi {
					return r.id
				}
			}
		}
	}
	text := desc.Get("ru") + " " + desc.Get("en")
	for _, k := range keywords {
		if k.re.MatchString(text) {
			return k.id
		}
	}
	return "generic"
}

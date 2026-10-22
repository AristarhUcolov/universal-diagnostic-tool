package ecu

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kline"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/legacy"
)

// mikasLine is a K-Line with a Микас behind it. The values are what a
// real Микас-5.47 sent (GAZ-3110 with ZMZ-406, journal of 22.10.2026).
type mikasLine struct {
	values map[byte][]byte // parameter code -> bytes the unit sends
	faults []byte          // body of the answer to command 0x02
	out    []byte          // answer bytes not read yet
	sent   []string        // frames received, as hex
	answer []string        // frames sent back, as hex
}

func (l *mikasLine) Name() string                              { return "test" }
func (l *mikasLine) Baud() int                                 { return 9600 }
func (l *mikasLine) SetBaud(int) error                         { return nil }
func (l *mikasLine) Drain()                                    { l.out = nil }
func (l *mikasLine) WakeUp(time.Duration, time.Duration) error { return nil }
func (l *mikasLine) FiveBaud(byte) error                       { return nil }
func (l *mikasLine) SetLLine(bool) error                       { return nil }
func (l *mikasLine) Close() error                              { return nil }

func (l *mikasLine) Send(p []byte, _ time.Duration) error {
	l.sent = append(l.sent, hexutil.Format(p))
	body, err := legacy.Decode(p[:len(p)-1])
	if err != nil {
		return err
	}
	var ans []byte
	switch body[0] {
	case legacy.CmdReadErrors:
		ans = l.faults
	case legacy.CmdReadParams:
		for _, c := range body[1:] {
			ans = append(ans, l.values[c]...)
		}
	default:
		return nil
	}
	l.out = legacy.Encode(ans)
	l.answer = append(l.answer, hexutil.Format(l.out))
	return nil
}

func (l *mikasLine) RecvByte(time.Duration) (byte, error) {
	if len(l.out) == 0 {
		return 0, kline.ErrTimeout
	}
	b := l.out[0]
	l.out = l.out[1:]
	return b, nil
}

// warmIdle: the engine warm and idling (12:21:34).
func warmIdle() *mikasLine {
	return &mikasLine{
		values: map[byte][]byte{
			0x29: {0x15}, 0x1A: {0x7A}, 0x1E: {0x83}, 0x20: {0x01}, 0x26: {0x1C},
			0x21: {0xAD, 0x00}, 0x40: {0x0D, 0x00}, 0x1C: {0x49},
		},
		faults: hexutil.MustParse("03 3E 40 B6 40 A8 40"),
	}
}

// engineOff: ignition on, engine stopped (12:00:13). The air flow set
// point (0x59) is one byte, although the protocol description says two.
func engineOff() *mikasLine {
	return &mikasLine{values: map[byte][]byte{
		0x1A: {0x33}, 0x1C: {0x37}, 0x19: {0x33}, 0x29: {0}, 0x2C: {0}, 0x20: {0},
		0x21: {0, 0}, 0x59: {0}, 0x26: {0x0A}, 0x28: {0}, 0x3F: {0, 0}, 0x40: {0, 0},
	}}
}

var engineOffIDs = []string{"twat", "tair", "twati", "freq", "freqx", "thr", "jair", "ugb", "uoz", "uozoc", "inj", "jqt"}

func checkValues(t *testing.T, vals []Value, want map[string]float64) {
	t.Helper()
	for _, v := range vals {
		w, ok := want[v.ID]
		if !ok {
			continue
		}
		if !v.OK || math.Abs(v.V-w) > 0.001 {
			t.Errorf("%s = %v (ok %v, raw %q), want %v", v.ID, v.V, v.OK, v.Raw, w)
		}
	}
}

func TestMikasRealCar(t *testing.T) {
	def, _ := Builtin().Get("gaz-mikas71")
	ctx := context.Background()

	line := warmIdle()
	link, err := legacy.Open(line, 9600, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewLegacyHandler(link, def, 0x09, "test")
	vals, err := h.Read(ctx, []string{"freq", "twat", "uacc", "thr", "uoz", "jair", "jqt", "tair"})
	if err != nil {
		t.Fatal(err)
	}
	// The same bytes on the wire as in the journal of the real car.
	if len(line.sent) != 1 || line.sent[0] != "61 29 1A 1E 20 26 21 40 00 1C 7B 0D" ||
		line.answer[0] != "15 7A 83 01 1C AD 00 40 CD 00 49 CE 0D" {
		t.Fatalf("exchange %q -> %q", line.sent, line.answer)
	}
	// The values the instrument cluster and common sense agree with.
	checkValues(t, vals, map[string]float64{
		"freq": 840, "twat": 82, "uacc": 13.1, "thr": 1, "uoz": 14, "jair": 17.3, "jqt": 1.3, "tair": 33,
	})

	codes, err := h.ReadDTC(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if line.answer[1] != "03 3E 40 00 B6 40 00 A8 40 00 A1 0D" {
		t.Fatalf("fault list frame %s", line.answer[1])
	}
	var got []string
	for _, c := range codes {
		got = append(got, c.Code)
		if !c.Known {
			t.Errorf("code %s has no description", c.Code)
		}
	}
	if strings.Join(got, " ") != "062 182 168" {
		t.Fatalf("codes = %v, want 062 182 168", got)
	}

	// Twelve values with the engine stopped: 15 bytes.
	line = engineOff()
	link, _ = legacy.Open(line, 9600, nil)
	h = NewLegacyHandler(link, def, 0x09, "test")
	vals, err = h.Read(ctx, engineOffIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(line.sent) != 1 || line.sent[0] != "61 1A 1C 19 29 2C 20 21 59 26 28 3F 40 00 94 0D" ||
		line.answer[0] != "33 37 33 00 00 00 00 00 00 0A 00 00 00 00 00 59 0D" {
		t.Fatalf("exchange %q -> %q", line.sent, line.answer)
	}
	checkValues(t, vals, map[string]float64{
		"twat": 11, "tair": 15, "twati": 11, "freq": 0, "freqx": 0, "thr": 0,
		"jair": 0, "ugb": 0, "uoz": 5, "uozoc": 0, "inj": 0, "jqt": 0,
	})
}

type entries []journal.Entry

func (e *entries) Log(x journal.Entry) { *e = append(*e, x) }

// A description that gets a parameter's size wrong must not shift the
// values after it: the handler learns the real size from the unit and
// leaves only that parameter out.
func TestLegacyLearnsSizes(t *testing.T) {
	data, err := builtinFS.ReadFile("defs/gaz-mikas71.json")
	if err != nil {
		t.Fatal(err)
	}
	var def Definition
	if err := json.Unmarshal(data, &def); err != nil {
		t.Fatal(err)
	}
	for i := range def.Blocks[0].Params {
		if p := &def.Blocks[0].Params[i]; p.ID == "ugb" {
			p.Type = "u16le" // what the protocol description says
		}
	}
	if err := def.prepare(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	line := engineOff()
	link, _ := legacy.Open(line, 9600, nil)
	h := NewLegacyHandler(link, &def, 0x09, "test")
	var log entries
	h.Log = &log

	vals, err := h.Read(ctx, engineOffIDs)
	if err != nil {
		t.Fatal(err)
	}
	// The batch, each parameter alone, the batch again.
	if len(line.sent) != 2+len(engineOffIDs) {
		t.Fatalf("%d requests: %q", len(line.sent), line.sent)
	}
	checkValues(t, vals, map[string]float64{
		"twat": 11, "tair": 15, "twati": 11, "freq": 0, "freqx": 0, "thr": 0,
		"jair": 0, "uoz": 5, "uozoc": 0, "inj": 0, "jqt": 0,
	})
	for _, v := range vals {
		if v.ID == "ugb" && (v.OK || v.Raw != "00") {
			t.Errorf("ugb = %+v, want not OK with raw 00", v)
		}
	}
	var warned bool
	for _, e := range log {
		if e.Kind == journal.Warn && strings.Contains(e.Text, "ugb") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no warning about ugb in %+v", log)
	}

	// The size is remembered: the next read is one request again.
	line.sent = nil
	if _, err := h.Read(ctx, engineOffIDs); err != nil {
		t.Fatal(err)
	}
	if len(line.sent) != 1 {
		t.Fatalf("%d requests after learning: %q", len(line.sent), line.sent)
	}
}

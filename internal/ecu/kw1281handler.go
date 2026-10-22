package ecu

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kw1281"
)

// KW1281Groups is how many measuring value groups are offered.
const KW1281Groups = 16

// KW1281Handler presents a VAG KW1281 control unit.
type KW1281Handler struct {
	conn   *kw1281.Conn
	mu     sync.Mutex
	params []Param
	name   string
}

// NewKW1281Handler wraps an open KW1281 session.
func NewKW1281Handler(conn *kw1281.Conn) *KW1281Handler {
	h := &KW1281Handler{conn: conn, name: fmt.Sprintf("VAG control unit 0x%02X", conn.Address())}
	if len(conn.ID) > 0 {
		h.name = fmt.Sprintf("VAG 0x%02X: %s", conn.Address(), conn.ID[0])
	}
	for g := 1; g <= KW1281Groups; g++ {
		for f := 1; f <= 4; f++ {
			h.params = append(h.params, Param{
				ID:    fmt.Sprintf("kw.g%03d.%d", g, f),
				Name:  i18n.T(fmt.Sprintf("Group %03d, field %d", g, f), fmt.Sprintf("Группа %03d, поле %d", g, f)),
				Group: fmt.Sprintf("group %03d", g), Kind: "num", Max: 100, Digits: 2, Verified: true,
			})
		}
	}
	return h
}

func (h *KW1281Handler) Describe() Description {
	return Description{ID: "ecu", Name: h.name, Protocol: fmt.Sprintf("VAG KW1281, %d baud", h.conn.Baud()),
		Address: fmt.Sprintf("0x%02X", h.conn.Address()), Family: dtc.FamilyVAG, Status: "generic",
		Capabilities: []string{CapDTC, CapClear, CapLive, CapRaw}}
}

func (h *KW1281Handler) Identify(ctx context.Context) ([]InfoItem, error) {
	items := []InfoItem{{Key: "protocol", Name: i18n.T("Protocol", "Протокол"), Value: h.Describe().Protocol}}
	for i, s := range h.conn.ID {
		items = append(items, InfoItem{Key: fmt.Sprintf("id%d", i), Name: i18n.T(fmt.Sprintf("Identification %d", i+1), fmt.Sprintf("Идентификация %d", i+1)), Value: s})
	}
	return items, nil
}

func (h *KW1281Handler) ReadDTC(ctx context.Context) ([]dtc.DTC, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	faults, err := h.conn.ReadFaults(ctx)
	if err != nil {
		return nil, err
	}
	out := kw1281.ToDTC(faults, h.name)
	db := dtc.Default()
	for i := range out {
		db.Fill(&out[i], dtc.FamilyVAG)
	}
	return out, nil
}

func (h *KW1281Handler) ClearDTC(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.conn.ClearFaults(ctx)
	return err
}

func (h *KW1281Handler) Params() []Param { return h.params }

func (h *KW1281Handler) Read(ctx context.Context, ids []string) ([]Value, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	groups := map[int][]kw1281.Cell{}
	failed := map[int]bool{}
	out := make([]Value, 0, len(ids))
	for _, id := range ids {
		g, f, ok := parseKWID(id)
		if !ok {
			out = append(out, Value{ID: id})
			continue
		}
		if _, done := groups[g]; !done && !failed[g] {
			cells, err := h.conn.ReadGroup(ctx, byte(g))
			if err != nil {
				failed[g] = true
				if err != kw1281.ErrNAK {
					return out, err
				}
			} else {
				groups[g] = cells
			}
		}
		v := Value{ID: id}
		if cells := groups[g]; f-1 < len(cells) {
			c := cells[f-1]
			v.OK = true
			v.V = c.Value
			v.Text = c.Text
			v.Raw = fmt.Sprintf("%02X %02X %02X", c.Formula, c.A, c.B)
		}
		out = append(out, v)
	}
	return out, nil
}

func parseKWID(id string) (group, field int, ok bool) {
	if !strings.HasPrefix(id, "kw.g") {
		return 0, 0, false
	}
	parts := strings.Split(id[4:], ".")
	if len(parts) != 2 {
		return 0, 0, false
	}
	g, err1 := strconv.Atoi(parts[0])
	f, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || g < 1 || g > 255 || f < 1 || f > 4 {
		return 0, 0, false
	}
	return g, f, true
}

func (h *KW1281Handler) Actuators() []Actuator { return nil }

func (h *KW1281Handler) Actuate(ctx context.Context, id, action string, value float64) (string, error) {
	return "", fmt.Errorf("output tests are not implemented for KW1281 yet")
}

// Raw sends a block: the first byte is the block title, the rest its
// data. The answer is returned the same way.
func (h *KW1281Handler) Raw(ctx context.Context, req []byte) ([]byte, error) {
	if len(req) == 0 {
		return nil, fmt.Errorf("empty request")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	b, err := h.conn.Exchange(ctx, req[0], req[1:])
	if err != nil {
		return nil, err
	}
	return append([]byte{b.Title}, b.Data...), nil
}

func (h *KW1281Handler) KeepAlive(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conn.KeepAlive(ctx)
}

// Close waits for a request in progress, then ends the session.
func (h *KW1281Handler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conn.Close()
}

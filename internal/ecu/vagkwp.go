package ecu

import (
	"context"
	"fmt"
	"sync"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kw1281"
)

// VAGKWPHandler talks to a VAG control unit using KWP2000 the VAG way
// (on TP 2.0 or K-Line): five-digit fault codes and measuring value
// groups read with 21 nn, encoded like KW1281.
type VAGKWPHandler struct {
	ch     diag.Channel
	mu     sync.Mutex
	t      diag.Timing
	params []Param
	name   string
}

// NewVAGKWPHandler opens the VAG diagnostic session (10 89).
func NewVAGKWPHandler(ctx context.Context, ch diag.Channel) (*VAGKWPHandler, error) {
	h := &VAGKWPHandler{ch: ch, t: diag.DefaultTiming(), name: "VAG KWP2000 control unit"}
	if _, err := diag.Request(ctx, ch, []byte{0x10, 0x89}, h.t); err != nil {
		return nil, err
	}
	for g := 1; g <= KW1281Groups; g++ {
		for f := 1; f <= 4; f++ {
			h.params = append(h.params, Param{ID: fmt.Sprintf("kw.g%03d.%d", g, f),
				Name:  i18n.T(fmt.Sprintf("Group %03d, field %d", g, f), fmt.Sprintf("Группа %03d, поле %d", g, f)),
				Group: fmt.Sprintf("group %03d", g), Kind: "num", Max: 100, Digits: 2, Verified: true})
		}
	}
	if resp, err := diag.Request(ctx, ch, []byte{0x1A, 0x9B}, h.t); err == nil && len(resp) > 2 {
		if id := hexutil.ASCII(resp[2:]); id != "" {
			h.name = "VAG: " + id
		}
	}
	return h, nil
}

func (h *VAGKWPHandler) Describe() Description {
	info := h.ch.Info()
	return Description{ID: "ecu", Name: h.name, Protocol: info.Protocol, Address: fmt.Sprintf("0x%02X", info.Target),
		Family: dtc.FamilyVAG, Status: "generic", Capabilities: []string{CapDTC, CapClear, CapLive, CapRaw}}
}

func (h *VAGKWPHandler) Identify(ctx context.Context) ([]InfoItem, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	items := []InfoItem{{Key: "protocol", Name: i18n.T("Protocol", "Протокол"), Value: h.ch.Info().Protocol}}
	if resp, err := diag.Request(ctx, h.ch, []byte{0x1A, 0x9B}, h.t); err == nil && len(resp) > 2 {
		items = append(items, InfoItem{Key: "ident", Name: i18n.T("Identification", "Идентификация"), Value: hexutil.ASCII(resp[2:])})
	}
	return items, nil
}

func (h *VAGKWPHandler) ReadDTC(ctx context.Context) ([]dtc.DTC, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	resp, err := diag.Request(ctx, h.ch, []byte{0x18, 0x02, 0xFF, 0x00}, h.t)
	if err != nil {
		return nil, err
	}
	if len(resp) < 2 {
		return nil, fmt.Errorf("short DTC answer % X", resp)
	}
	var faults []kw1281.Fault
	for i := 2; i+2 < len(resp); i += 3 {
		faults = append(faults, kw1281.Fault{Code: uint16(resp[i])<<8 | uint16(resp[i+1]), Status: resp[i+2]})
	}
	out := kw1281.ToDTC(faults, h.name)
	db := dtc.Default()
	for i := range out {
		out[i].Protocol = "KWP2000 (VAG)"
		dtc.ApplyKWPStatus(&out[i], faults[i].Status)
		db.Fill(&out[i], dtc.FamilyVAG)
	}
	return out, nil
}

func (h *VAGKWPHandler) ClearDTC(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := diag.Request(ctx, h.ch, []byte{0x14, 0xFF, 0x00}, h.t)
	return err
}

func (h *VAGKWPHandler) Params() []Param { return h.params }

func (h *VAGKWPHandler) Read(ctx context.Context, ids []string) ([]Value, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	groups := map[int][]kw1281.Cell{}
	out := make([]Value, 0, len(ids))
	for _, id := range ids {
		g, f, ok := parseKWID(id)
		v := Value{ID: id}
		if !ok {
			out = append(out, v)
			continue
		}
		cells, done := groups[g]
		if !done {
			resp, err := diag.Request(ctx, h.ch, []byte{0x21, byte(g)}, h.t)
			if err == nil && len(resp) > 2 {
				for i := 2; i+2 < len(resp); i += 3 {
					cells = append(cells, kw1281.DecodeCell(resp[i], resp[i+1], resp[i+2]))
				}
			} else if err != nil && !diag.IsNRC(err, 0x31) {
				return out, err
			}
			groups[g] = cells
		}
		if f-1 < len(cells) {
			c := cells[f-1]
			v.OK, v.V, v.Text = true, c.Value, c.Text
		}
		out = append(out, v)
	}
	return out, nil
}

func (h *VAGKWPHandler) Actuators() []Actuator { return nil }

func (h *VAGKWPHandler) Actuate(ctx context.Context, id, action string, value float64) (string, error) {
	return "", fmt.Errorf("output tests are not implemented for VAG KWP2000 yet")
}

func (h *VAGKWPHandler) Raw(ctx context.Context, req []byte) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	m, err := diag.RequestMsg(ctx, h.ch, req, h.t)
	return m.Data, err
}

func (h *VAGKWPHandler) KeepAlive(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ka, ok := h.ch.(interface{ KeepAlive(context.Context) error }); ok {
		return ka.KeepAlive(ctx)
	}
	_, err := diag.Request(ctx, h.ch, []byte{0x3E}, h.t)
	return err
}

// Close waits for a request in progress, then closes the channel.
func (h *VAGKWPHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ch.Close()
}

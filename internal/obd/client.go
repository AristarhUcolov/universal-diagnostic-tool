package obd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
)

// Client sends OBD-II requests over a channel.
type Client struct {
	Ch diag.Channel
	// CAN selects the ISO 15765-4 message formats (DTC count byte, one
	// message per answer, up to six PIDs per request).
	CAN bool
	// First is how long to wait for the first answer, Quiet how long to
	// keep listening for more control units.
	First, Quiet time.Duration
}

// NewClient creates a client for the channel.
func NewClient(ch diag.Channel) *Client {
	info := ch.Info()
	c := &Client{Ch: ch, CAN: info.CAN, First: 2 * time.Second, Quiet: 150 * time.Millisecond}
	if !info.CAN {
		c.Quiet = 250 * time.Millisecond
	}
	return c
}

// Query sends a request and returns all answers.
func (c *Client) Query(ctx context.Context, req []byte) ([]diag.Message, error) {
	if !c.Ch.Info().Functional {
		m, err := diag.RequestMsg(ctx, c.Ch, req, diag.Timing{P2: c.First, P2Ext: 6 * time.Second, BusyRetries: 2, BusyDelay: 200 * time.Millisecond})
		if err != nil {
			return nil, err
		}
		return []diag.Message{m}, nil
	}
	msgs, err := diag.RequestAll(ctx, c.Ch, req, c.First, c.Quiet)
	if err != nil {
		return nil, err
	}
	var out []diag.Message
	for _, m := range msgs {
		if len(m.Data) > 0 && m.Data[0] != 0x7F {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil, diag.ErrTimeout
	}
	return out, nil
}

// Supported returns the supported mode 01 PIDs of every answering
// control unit.
func (c *Client) Supported(ctx context.Context) (map[uint32][]byte, error) {
	out := map[uint32][]byte{}
	base := byte(0x00)
	active := map[uint32]bool{}
	for {
		msgs, err := c.Query(ctx, []byte{0x01, base})
		if err != nil {
			if base == 0 {
				return nil, err
			}
			break
		}
		more := false
		for _, m := range msgs {
			if base != 0 && !active[m.Src] {
				continue
			}
			if len(m.Data) < 6 || m.Data[1] != base {
				continue
			}
			active[m.Src] = false
			mask := m.Data[2:6]
			for i := 0; i < 32; i++ {
				if mask[i/8]&(0x80>>(i%8)) != 0 {
					pid := base + byte(i) + 1
					if pid%0x20 != 0 {
						out[m.Src] = append(out[m.Src], pid)
					}
				}
			}
			if mask[3]&0x01 != 0 {
				active[m.Src] = true
				more = true
			}
		}
		if !more || base >= 0xE0 {
			break
		}
		base += 0x20
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("obd: no control unit reported supported PIDs")
	}
	return out, nil
}

// ReadPIDs reads the given PIDs and returns their data bytes per control
// unit. On CAN up to six PIDs go into one request.
func (c *Client) ReadPIDs(ctx context.Context, pids []byte) (map[uint32]map[byte][]byte, error) {
	out := map[uint32]map[byte][]byte{}
	batch := 1
	if c.CAN {
		batch = 6
	}
	var lastErr error
	for i := 0; i < len(pids); i += batch {
		end := i + batch
		if end > len(pids) {
			end = len(pids)
		}
		req := append([]byte{0x01}, pids[i:end]...)
		msgs, err := c.Query(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}
		for _, m := range msgs {
			if out[m.Src] == nil {
				out[m.Src] = map[byte][]byte{}
			}
			ParseMode01(m.Data, out[m.Src])
		}
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

// ParseMode01 splits a "41 pid data pid data..." answer into its PIDs.
func ParseMode01(data []byte, into map[byte][]byte) {
	if len(data) < 2 || (data[0] != 0x41 && data[0] != 0x42) {
		return
	}
	i := 1
	frame := data[0] == 0x42
	for i < len(data) {
		pid := data[i]
		i++
		if frame {
			i++ // frame number
		}
		n := PIDSize(pid)
		if n == 0 || i+n > len(data) {
			return
		}
		into[pid] = append([]byte(nil), data[i:i+n]...)
		i += n
	}
}

// ReadDTCs reads trouble codes with mode 03 (stored), 07 (pending) or 0A
// (permanent).
func (c *Client) ReadDTCs(ctx context.Context, mode byte) ([]dtc.DTC, error) {
	msgs, err := c.Query(ctx, []byte{mode})
	if err != nil {
		return nil, err
	}
	var out []dtc.DTC
	for _, m := range msgs {
		for _, code := range ParseDTCs(m.Data, c.CAN) {
			d := dtc.DTC{Code: code, Source: fmt.Sprintf("%X", m.Src), Protocol: "OBD-II", Confirmed: mode == 0x03,
				Pending: mode == 0x07, Permanent: mode == 0x0A}
			switch mode {
			case 0x03:
				d.Flags = []string{dtc.FlagConfirmed}
			case 0x07:
				d.Flags = []string{dtc.FlagPending}
			case 0x0A:
				d.Flags = []string{dtc.FlagPermanent}
			}
			out = append(out, d)
		}
	}
	return out, nil
}

// ParseDTCs extracts the codes of a mode 03/07/0A answer.
func ParseDTCs(data []byte, can bool) []string {
	if len(data) < 1 {
		return nil
	}
	body := data[1:]
	if can && len(body) > 0 {
		n := int(body[0])
		body = body[1:]
		if 2*n < len(body) {
			body = body[:2*n]
		}
	}
	var out []string
	for i := 0; i+1 < len(body); i += 2 {
		if body[i] == 0 && body[i+1] == 0 {
			continue
		}
		out = append(out, dtc.SAE(body[i], body[i+1]))
	}
	return out
}

// Clear erases trouble codes and freeze frames (mode 04).
func (c *Client) Clear(ctx context.Context) error {
	_, err := c.Query(ctx, []byte{0x04})
	return err
}

// InfoRecords reads a mode 09 info type and returns the data of each
// control unit with the headers removed (multi-message K-Line answers are
// joined in message order).
func (c *Client) InfoRecords(ctx context.Context, info byte) (map[uint32][]byte, error) {
	msgs, err := c.Query(ctx, []byte{0x09, info})
	if err != nil {
		return nil, err
	}
	type part struct {
		seq  byte
		data []byte
	}
	parts := map[uint32][]part{}
	for _, m := range msgs {
		if len(m.Data) < 3 || m.Data[0] != 0x49 || m.Data[1] != info {
			continue
		}
		if c.CAN {
			// 49 info count data...
			parts[m.Src] = append(parts[m.Src], part{0, m.Data[3:]})
		} else {
			parts[m.Src] = append(parts[m.Src], part{m.Data[2], m.Data[3:]})
		}
	}
	out := map[uint32][]byte{}
	for src, ps := range parts {
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].seq < ps[j].seq })
		for _, p := range ps {
			out[src] = append(out[src], p.data...)
		}
	}
	return out, nil
}

// VIN reads the vehicle identification number.
func (c *Client) VIN(ctx context.Context) (string, error) {
	recs, err := c.InfoRecords(ctx, 0x02)
	if err != nil {
		return "", err
	}
	for _, src := range sortedKeys(recs) {
		v := strings.TrimLeft(hexutil.ASCII(recs[src]), "\x00 ")
		if len(v) >= 17 {
			return v[len(v)-17:], nil
		}
		if v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("obd: VIN not reported")
}

// Strings reads a text info type (04 calibration id, 0A ECU name) and
// splits it into strings of the given record length.
func (c *Client) Strings(ctx context.Context, info byte, recLen int) (map[uint32][]string, error) {
	recs, err := c.InfoRecords(ctx, info)
	if err != nil {
		return nil, err
	}
	out := map[uint32][]string{}
	for src, data := range recs {
		for i := 0; i < len(data); i += recLen {
			end := i + recLen
			if end > len(data) {
				end = len(data)
			}
			if s := hexutil.ASCII(data[i:end]); s != "" {
				out[src] = append(out[src], s)
			}
		}
	}
	return out, nil
}

func sortedKeys[V any](m map[uint32]V) []uint32 {
	keys := make([]uint32, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

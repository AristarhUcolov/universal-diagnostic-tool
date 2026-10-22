package elm327

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
)

// Spec selects the protocol and addressing of a channel.
type Spec struct {
	Proto      byte   // '0' auto, '1'..'C'
	Header     string // ATSH value, e.g. "81 10 F1" or "7E0"; empty = OBD default
	RxFilter   string // ATCRA value, e.g. "7E8"
	FlowHeader string // ATFCSH for CAN physical addressing
	InitAddr   string // ATIIA for 5-baud init (hex)
	WakeMsg    string // ATWM keep-alive message
	FastInit   bool   // run ATFI after configuring (KWP fast init)
	Functional bool
	Name       string
}

// Channel is a diagnostic channel through the adapter.
type Channel struct {
	d      *Device
	spec   Spec
	proto  byte
	queue  []diag.Message
	closed bool
	// Timeout waits for the adapter's answer to a data request.
	Timeout time.Duration
}

// Open configures the adapter for spec. With protocol '0' the adapter
// searches for the vehicle's OBD protocol, which takes a few seconds.
func (d *Device) Open(ctx context.Context, spec Spec) (*Channel, error) {
	if spec.Proto == 0 {
		spec.Proto = '0'
	}
	c := &Channel{d: d, spec: spec, Timeout: 6 * time.Second}
	cmds := []string{"ATD", "ATE0", "ATL0", "ATS1", "ATH1", "ATAT1", "ATCAF1", "ATSP" + string(spec.Proto)}
	if spec.Header != "" {
		cmds = append(cmds, "ATSH"+strings.ReplaceAll(spec.Header, " ", ""))
	}
	if spec.RxFilter != "" {
		cmds = append(cmds, "ATCRA"+strings.ReplaceAll(spec.RxFilter, " ", ""))
	}
	if spec.FlowHeader != "" {
		cmds = append(cmds, "ATFCSH"+strings.ReplaceAll(spec.FlowHeader, " ", ""), "ATFCSD300000", "ATFCSM1")
	}
	if spec.InitAddr != "" {
		cmds = append(cmds, "ATIIA"+spec.InitAddr)
	}
	if spec.WakeMsg != "" {
		cmds = append(cmds, "ATWM"+strings.ReplaceAll(spec.WakeMsg, " ", ""))
	}
	for _, cmd := range cmds {
		if _, err := d.Cmd(ctx, cmd, 2*time.Second); err != nil {
			if errors.Is(err, ErrUnknownCommand) && (strings.HasPrefix(cmd, "ATFCS") || strings.HasPrefix(cmd, "ATWM") || strings.HasPrefix(cmd, "ATCRA")) {
				journal.Warnf(d.log, "elm", "adapter does not support %s (old firmware or clone)", cmd)
				continue
			}
			return nil, fmt.Errorf("elm327: %s: %w", cmd, err)
		}
	}
	if spec.FastInit {
		if _, err := d.Cmd(ctx, "ATFI", 6*time.Second); err != nil && !errors.Is(err, ErrUnknownCommand) {
			return nil, fmt.Errorf("elm327: fast init: %w", err)
		}
	}
	c.proto = spec.Proto
	if spec.Proto == '0' {
		// Let the adapter find the protocol with a harmless request.
		if _, err := d.Cmd(ctx, "0100", 15*time.Second); err != nil {
			return nil, err
		}
		lines, err := d.Cmd(ctx, "ATDPN", 2*time.Second)
		if err != nil || len(lines) == 0 {
			return nil, fmt.Errorf("elm327: cannot read the detected protocol: %v", err)
		}
		p := strings.TrimPrefix(strings.ToUpper(lines[0]), "A")
		if len(p) == 0 {
			return nil, fmt.Errorf("elm327: unexpected protocol answer %q", lines[0])
		}
		c.proto = p[0]
		journal.Infof(d.log, "elm", "vehicle protocol: %s", ProtocolName(c.proto))
	}
	d.proto = c.proto
	return c, nil
}

// Reinit makes the adapter initialise the vehicle bus again, for
// example after the control unit restarted.
func (c *Channel) Reinit(ctx context.Context) error {
	if _, err := c.d.Cmd(ctx, "ATPC", 2*time.Second); err != nil && !errors.Is(err, ErrUnknownCommand) {
		return fmt.Errorf("elm327: protocol close: %w", err)
	}
	if c.spec.FastInit {
		if _, err := c.d.Cmd(ctx, "ATFI", 6*time.Second); err != nil && !errors.Is(err, ErrUnknownCommand) {
			return fmt.Errorf("elm327: fast init: %w", err)
		}
	}
	return nil
}

// Protocol returns the protocol number in use.
func (c *Channel) Protocol() byte { return c.proto }

// Send implements diag.Channel. The adapter waits for the answers
// itself, so Send already collects them for Recv.
func (c *Channel) Send(ctx context.Context, data []byte) error {
	if c.closed {
		return diag.ErrClosed
	}
	c.queue = c.queue[:0]
	if IsCAN(c.proto) && len(data) > 7 && c.d.STN == "" {
		return fmt.Errorf("elm327: requests longer than 7 bytes need an STN-based adapter (OBDLink)")
	}
	lines, err := c.d.Cmd(ctx, hexutil.Compact(data), c.Timeout)
	if err != nil {
		if errors.Is(err, ErrNoData) {
			return nil // Recv reports the timeout
		}
		return err
	}
	msgs, perr := ParseLines(lines, c.proto)
	if perr != nil {
		journal.Warnf(c.d.log, "elm", "%v", perr)
	}
	c.queue = msgs
	return nil
}

// Recv implements diag.Channel.
func (c *Channel) Recv(ctx context.Context, timeout time.Duration) (diag.Message, error) {
	if c.closed {
		return diag.Message{}, diag.ErrClosed
	}
	if len(c.queue) == 0 {
		return diag.Message{}, diag.ErrTimeout
	}
	m := c.queue[0]
	c.queue = c.queue[1:]
	return m, nil
}

// Info implements diag.Channel.
func (c *Channel) Info() diag.ChannelInfo {
	name := c.spec.Name
	if name == "" {
		name = ProtocolName(c.proto)
	}
	return diag.ChannelInfo{Protocol: name + " via ELM327", ProtoID: "elm-" + string(c.proto),
		Adapter: c.d.Name(), Functional: c.spec.Functional || c.spec.Header == "", CAN: IsCAN(c.proto)}
}

// Close implements diag.Channel (the adapter stays open).
func (c *Channel) Close() error {
	c.closed = true
	return nil
}

// ParseLines converts the adapter's answer lines (headers and spaces on)
// into messages, reassembling multi-frame CAN answers.
func ParseLines(lines []string, proto byte) ([]diag.Message, error) {
	var out []diag.Message
	type asm struct {
		total int
		data  []byte
	}
	pending := map[uint32]*asm{}
	var order []uint32
	var lastErr error
	now := time.Now()
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) == 0 {
			continue
		}
		if IsCAN(proto) {
			var id uint32
			var rest []string
			if len(fields[0]) == 3 {
				v, err := strconv.ParseUint(fields[0], 16, 32)
				if err != nil {
					lastErr = fmt.Errorf("bad CAN line %q", l)
					continue
				}
				id, rest = uint32(v), fields[1:]
			} else if len(fields) > 4 && len(fields[0]) == 2 {
				b, err := hexutil.Parse(strings.Join(fields[:4], ""))
				if err != nil {
					lastErr = fmt.Errorf("bad CAN line %q", l)
					continue
				}
				id, rest = uint32(b[0])<<24|uint32(b[1])<<16|uint32(b[2])<<8|uint32(b[3]), fields[4:]
			} else {
				lastErr = fmt.Errorf("unexpected CAN line %q", l)
				continue
			}
			data, err := hexutil.Parse(strings.Join(rest, " "))
			if err != nil || len(data) == 0 {
				lastErr = fmt.Errorf("bad data in %q", l)
				continue
			}
			switch data[0] >> 4 {
			case 0:
				n := int(data[0] & 0x0F)
				if 1+n > len(data) {
					n = len(data) - 1
				}
				out = append(out, diag.Message{Src: id, Data: append([]byte(nil), data[1:1+n]...), Time: now})
			case 1:
				if len(data) < 2 {
					continue
				}
				total := int(data[0]&0x0F)<<8 | int(data[1])
				pending[id] = &asm{total: total, data: append([]byte(nil), data[2:]...)}
				order = append(order, id)
			case 2:
				if a := pending[id]; a != nil {
					a.data = append(a.data, data[1:]...)
				}
			}
			continue
		}
		data, err := hexutil.Parse(l)
		if err != nil || len(data) < 2 {
			lastErr = fmt.Errorf("bad line %q", l)
			continue
		}
		head := 3
		if proto == '4' || proto == '5' {
			// KWP2000 header: format byte, optional target/source,
			// optional length byte.
			head = 1
			if data[0]&0xC0 != 0 {
				head = 3
			}
			if data[0]&0x3F == 0 {
				head++
			}
		}
		if len(data) < head+2 {
			lastErr = fmt.Errorf("short line %q", l)
			continue
		}
		src := uint32(0)
		if head >= 3 {
			src = uint32(data[2])
		}
		out = append(out, diag.Message{Src: src, Data: append([]byte(nil), data[head:len(data)-1]...), Time: now})
	}
	for _, id := range order {
		a := pending[id]
		if len(a.data) >= a.total {
			out = append(out, diag.Message{Src: id, Data: a.data[:a.total], Time: now})
		} else {
			lastErr = fmt.Errorf("incomplete multi-frame answer from %X (%d of %d bytes)", id, len(a.data), a.total)
		}
	}
	return out, lastErr
}

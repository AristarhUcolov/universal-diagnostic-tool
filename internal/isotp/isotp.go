// Package isotp implements ISO 15765-2 (ISO-TP), the transport layer that
// carries OBD-II, UDS and KWP2000 messages longer than one CAN frame:
// single frames, first frame + consecutive frames, flow control with
// block size and STmin, optional extended addressing and CAN FD.
package isotp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/timing"
)

// Config describes an ISO-TP connection.
type Config struct {
	TxID uint32 // identifier we send on
	RxID uint32 // identifier the control unit answers on
	// RxIDs, when set, accepts answers from several identifiers (OBD
	// functional requests); RxID is then ignored.
	RxIDs []uint32
	Ext   bool // 29-bit identifiers
	FD    bool // CAN FD frames (up to 64 bytes)
	// Padding is the fill byte for short frames; -1 sends frames
	// without padding. OBD (ISO 15765-4) requires 8-byte frames.
	Padding int
	// ExtAddr, when >= 0, is the target address byte of extended
	// addressing (BMW, some trucks); RxExtAddr is the expected byte in
	// answers.
	ExtAddr   int
	RxExtAddr int
	BlockSize byte // our flow control block size (0 = send everything)
	STmin     byte // our flow control separation time
	// Functional marks TxID as a functional (broadcast) address, which
	// only allows single frames.
	Functional bool
	// FlowTx returns the identifier for flow control frames sent in
	// answer to a first frame from rx. Defaults to TxID; for functional
	// OBD it maps 7E8->7E0 and 18DAF1xx->18DAxxF1.
	FlowTx  func(rx uint32) uint32
	NBs     time.Duration // wait for flow control (default 1 s)
	NCr     time.Duration // wait for the next consecutive frame (default 1 s)
	Name    string        // protocol name for the user interface
	Bitrate int
}

// DefaultConfig returns a physical 11-bit configuration with 8-byte padding.
func DefaultConfig(tx, rx uint32) Config {
	return Config{TxID: tx, RxID: rx, Padding: 0x00, ExtAddr: -1, RxExtAddr: -1}
}

// OBDFunctional returns the ISO 15765-4 configuration for functional
// OBD-II requests: 7DF to all, answers 7E8-7EF (or the 29-bit
// equivalents 18DB33F1 / 18DAF1xx).
func OBDFunctional(ext bool) Config {
	c := Config{Padding: 0x00, ExtAddr: -1, RxExtAddr: -1, Functional: true, Ext: ext}
	if ext {
		c.TxID = 0x18DB33F1
		for a := uint32(0); a < 256; a++ {
			c.RxIDs = append(c.RxIDs, 0x18DAF100|a)
		}
		c.FlowTx = func(rx uint32) uint32 { return 0x18DA00F1 | (rx&0xFF)<<8 }
	} else {
		c.TxID = 0x7DF
		for id := uint32(0x7E8); id <= 0x7EF; id++ {
			c.RxIDs = append(c.RxIDs, id)
		}
		c.FlowTx = func(rx uint32) uint32 { return rx - 8 }
	}
	return c
}

// OBDPhysical returns the configuration for one OBD control unit found
// at the given response identifier.
func OBDPhysical(rx uint32, ext bool) Config {
	c := Config{RxID: rx, Padding: 0x00, ExtAddr: -1, RxExtAddr: -1, Ext: ext}
	if ext {
		c.TxID = 0x18DA00F1 | (rx&0xFF)<<8
	} else {
		c.TxID = rx - 8
	}
	return c
}

// Errors.
var (
	ErrOverflow = errors.New("isotp: receiver reported buffer overflow")
	ErrSequence = errors.New("isotp: consecutive frame out of sequence")
)

type assembly struct {
	total int
	data  []byte
	seq   byte
	last  time.Time
}

// Conn is an ISO-TP connection; it implements diag.Channel.
type Conn struct {
	mux     *can.Mux
	sub     *can.Sub
	cfg     Config
	log     journal.Logger
	ready   []diag.Message
	pending map[uint32]*assembly
	rxSet   map[uint32]bool
	closed  bool
}

// New opens an ISO-TP connection on the mux.
func New(mux *can.Mux, cfg Config, log journal.Logger) *Conn {
	if cfg.NBs == 0 {
		cfg.NBs = time.Second
	}
	if cfg.NCr == 0 {
		cfg.NCr = time.Second
	}
	if cfg.FlowTx == nil {
		tx := cfg.TxID
		cfg.FlowTx = func(uint32) uint32 { return tx }
	}
	c := &Conn{mux: mux, cfg: cfg, log: journal.Or(log), pending: map[uint32]*assembly{}, rxSet: map[uint32]bool{}}
	if len(cfg.RxIDs) > 0 {
		for _, id := range cfg.RxIDs {
			c.rxSet[id] = true
		}
	} else {
		c.rxSet[cfg.RxID] = true
	}
	ext := cfg.Ext
	c.sub = mux.Subscribe(func(f *can.Frame) bool {
		if f.Echo || f.Ext != ext || !c.rxSet[f.ID] {
			return false
		}
		if cfg.RxExtAddr >= 0 && (f.Len == 0 || f.Data[0] != byte(cfg.RxExtAddr)) {
			return false
		}
		return true
	}, 512)
	return c
}

func (c *Conn) maxFrame() int {
	if c.cfg.FD {
		return 64
	}
	return 8
}

func (c *Conn) prefix() []byte {
	if c.cfg.ExtAddr >= 0 {
		return []byte{byte(c.cfg.ExtAddr)}
	}
	return nil
}

func (c *Conn) frame(id uint32, payload []byte) can.Frame {
	if c.cfg.Padding >= 0 {
		size := 8
		if len(payload) > 8 {
			size = can.DLCLen(can.LenDLC(len(payload)))
		}
		for len(payload) < size {
			payload = append(payload, byte(c.cfg.Padding))
		}
	} else if c.cfg.FD && len(payload) > 8 {
		// FD frames must still match a DLC size.
		size := can.DLCLen(can.LenDLC(len(payload)))
		for len(payload) < size {
			payload = append(payload, 0xCC)
		}
	}
	f := can.NewFrame(id, c.cfg.Ext, payload)
	f.FD = c.cfg.FD && len(payload) > 8
	return f
}

// Send implements diag.Channel.
func (c *Conn) Send(ctx context.Context, data []byte) error {
	if c.closed {
		return diag.ErrClosed
	}
	if len(data) == 0 {
		return fmt.Errorf("isotp: empty message")
	}
	c.sub.Drain()
	c.ready = c.ready[:0]
	c.pending = map[uint32]*assembly{}
	pre := c.prefix()
	room := c.maxFrame() - len(pre)
	journal.Tx(c.log, "isotp", fmt.Sprintf("%X", c.cfg.TxID), data, diag.ServiceName(data[0]))

	// Single frame: classic PCI byte for up to 7 bytes, CAN FD escape
	// (00 + length) for longer single frames.
	switch {
	case len(data) <= room-1 && len(data) <= 7:
		p := append(append([]byte(nil), pre...), byte(len(data)))
		p = append(p, data...)
		return c.mux.Send(ctx, c.frame(c.cfg.TxID, p))
	case c.cfg.FD && len(data) <= room-2:
		p := append(append([]byte(nil), pre...), 0x00, byte(len(data)))
		p = append(p, data...)
		return c.mux.Send(ctx, c.frame(c.cfg.TxID, p))
	}
	if c.cfg.Functional {
		return fmt.Errorf("isotp: functional requests must fit in a single frame")
	}

	// First frame.
	var p []byte
	p = append(p, pre...)
	if len(data) <= 4095 {
		p = append(p, 0x10|byte(len(data)>>8), byte(len(data)))
	} else {
		n := uint32(len(data))
		p = append(p, 0x10, 0x00, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	chunk := room - (len(p) - len(pre))
	p = append(p, data[:chunk]...)
	if err := c.mux.Send(ctx, c.frame(c.cfg.TxID, p)); err != nil {
		return err
	}
	sent := chunk
	seq := byte(1)
	for sent < len(data) {
		bs, stmin, err := c.awaitFlow(ctx)
		if err != nil {
			return err
		}
		for n := 0; sent < len(data) && (bs == 0 || n < int(bs)); n++ {
			if n > 0 {
				timing.Sleep(stmin)
			}
			p := append([]byte(nil), pre...)
			p = append(p, 0x20|(seq&0x0F))
			end := sent + room - 1
			if end > len(data) {
				end = len(data)
			}
			p = append(p, data[sent:end]...)
			if err := c.mux.Send(ctx, c.frame(c.cfg.TxID, p)); err != nil {
				return err
			}
			sent = end
			seq++
		}
	}
	return nil
}

// awaitFlow waits for a flow control frame and returns block size and
// separation time. Other frames that arrive meanwhile are processed.
func (c *Conn) awaitFlow(ctx context.Context) (byte, time.Duration, error) {
	deadline := time.Now().Add(c.cfg.NBs)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, 0, fmt.Errorf("isotp: no flow control from %X (timeout N_Bs)", c.cfg.RxID)
		}
		f, err := c.sub.Recv(ctx, remaining)
		if err != nil {
			if errors.Is(err, can.ErrTimeout) {
				continue
			}
			return 0, 0, err
		}
		p := f.Payload()
		if c.cfg.RxExtAddr >= 0 && len(p) > 0 {
			p = p[1:]
		}
		if len(p) < 3 || p[0]>>4 != 3 {
			c.process(ctx, f)
			continue
		}
		switch p[0] & 0x0F {
		case 0: // continue to send
			return p[1], stmin(p[2]), nil
		case 1: // wait
			deadline = time.Now().Add(c.cfg.NBs)
		case 2:
			return 0, 0, ErrOverflow
		}
	}
}

func stmin(b byte) time.Duration {
	switch {
	case b <= 0x7F:
		return time.Duration(b) * time.Millisecond
	case b >= 0xF1 && b <= 0xF9:
		return time.Duration(b-0xF0) * 100 * time.Microsecond
	}
	return 127 * time.Millisecond
}

// process feeds a received frame into the reassembly state machine.
func (c *Conn) process(ctx context.Context, f can.Frame) {
	p := f.Payload()
	if c.cfg.RxExtAddr >= 0 {
		if len(p) == 0 {
			return
		}
		p = p[1:]
	}
	if len(p) == 0 {
		return
	}
	switch p[0] >> 4 {
	case 0: // single frame
		n := int(p[0] & 0x0F)
		off := 1
		if n == 0 && len(p) > 2 {
			n, off = int(p[1]), 2
		}
		if n == 0 || off+n > len(p) {
			return
		}
		c.deliver(f, p[off:off+n])
	case 1: // first frame
		if len(p) < 2 {
			return
		}
		total := int(p[0]&0x0F)<<8 | int(p[1])
		off := 2
		if total == 0 && len(p) >= 6 {
			total = int(p[2])<<24 | int(p[3])<<16 | int(p[4])<<8 | int(p[5])
			off = 6
		}
		a := &assembly{total: total, seq: 1, last: time.Now()}
		a.data = append(a.data, p[off:]...)
		c.pending[f.ID] = a
		fc := append([]byte(nil), c.prefix()...)
		fc = append(fc, 0x30, c.cfg.BlockSize, c.cfg.STmin)
		_ = c.mux.Send(ctx, c.frame(c.cfg.FlowTx(f.ID), fc))
	case 2: // consecutive frame
		a := c.pending[f.ID]
		if a == nil {
			return
		}
		if p[0]&0x0F != a.seq&0x0F {
			journal.Warnf(c.log, "isotp", "%v from %X: expected %d, got %d", ErrSequence, f.ID, a.seq&0x0F, p[0]&0x0F)
			delete(c.pending, f.ID)
			return
		}
		a.seq++
		a.last = time.Now()
		a.data = append(a.data, p[1:]...)
		if len(a.data) >= a.total {
			delete(c.pending, f.ID)
			c.deliver(f, a.data[:a.total])
			return
		}
		if c.cfg.BlockSize > 0 && (a.seq-1)%c.cfg.BlockSize == 0 {
			fc := append([]byte(nil), c.prefix()...)
			fc = append(fc, 0x30, c.cfg.BlockSize, c.cfg.STmin)
			_ = c.mux.Send(ctx, c.frame(c.cfg.FlowTx(f.ID), fc))
		}
	}
}

func (c *Conn) deliver(f can.Frame, data []byte) {
	msg := diag.Message{Src: f.ID, Dst: c.cfg.TxID, Data: append([]byte(nil), data...), Time: f.Time}
	if msg.Time.IsZero() {
		msg.Time = time.Now()
	}
	journal.Rx(c.log, "isotp", fmt.Sprintf("%X", f.ID), msg.Data, diag.ServiceName(msg.Data[0]))
	c.ready = append(c.ready, msg)
}

// Recv implements diag.Channel.
func (c *Conn) Recv(ctx context.Context, timeout time.Duration) (diag.Message, error) {
	if c.closed {
		return diag.Message{}, diag.ErrClosed
	}
	deadline := time.Now().Add(timeout)
	for {
		if len(c.ready) > 0 {
			m := c.ready[0]
			c.ready = c.ready[1:]
			return m, nil
		}
		wait := time.Until(deadline)
		// A message in progress may extend the wait up to N_Cr.
		for _, a := range c.pending {
			if t := time.Until(a.last.Add(c.cfg.NCr)); t > wait {
				wait = t
			}
		}
		if wait <= 0 {
			return diag.Message{}, diag.ErrTimeout
		}
		f, err := c.sub.Recv(ctx, wait)
		if err != nil {
			if errors.Is(err, can.ErrTimeout) {
				for id, a := range c.pending {
					if time.Since(a.last) > c.cfg.NCr {
						journal.Warnf(c.log, "isotp", "message from %X incomplete (%d of %d bytes, timeout N_Cr)", id, len(a.data), a.total)
						delete(c.pending, id)
					}
				}
				if time.Now().After(deadline) && len(c.pending) == 0 {
					return diag.Message{}, diag.ErrTimeout
				}
				continue
			}
			return diag.Message{}, err
		}
		c.process(ctx, f)
	}
}

// Info implements diag.Channel.
func (c *Conn) Info() diag.ChannelInfo {
	name := c.cfg.Name
	if name == "" {
		bits := "11-bit"
		if c.cfg.Ext {
			bits = "29-bit"
		}
		name = fmt.Sprintf("ISO 15765 CAN %s", bits)
		if c.cfg.FD {
			name = fmt.Sprintf("ISO 15765 CAN FD %s", bits)
		}
		if c.cfg.Bitrate > 0 {
			name += fmt.Sprintf(", %d kbit/s", c.cfg.Bitrate/1000)
		}
	}
	return diag.ChannelInfo{Protocol: name, ProtoID: "iso15765", Adapter: c.mux.Bus().Info().Name,
		Target: c.cfg.TxID, Tester: c.cfg.RxID, Functional: c.cfg.Functional, Baud: c.cfg.Bitrate, CAN: true}
}

// Close releases the subscription (the bus stays open).
func (c *Conn) Close() error {
	if !c.closed {
		c.closed = true
		c.sub.Close()
	}
	return nil
}

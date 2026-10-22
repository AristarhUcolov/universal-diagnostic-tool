// Package tp20 implements Volkswagen Transport Protocol 2.0, which
// carries KWP2000 over CAN on VW, Audi, Seat and Skoda models from about
// 2004 to 2012 (PQ35/PQ46 platforms). A channel is negotiated on CAN id
// 0x200, then data packets with sequence numbers and acknowledgements
// flow on the negotiated identifiers.
//
// The packet format follows the publicly documented reverse engineering
// of TP 2.0; treat this implementation as experimental.
package tp20

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

// Errors.
var (
	ErrNoChannel = errors.New("tp20: control unit did not open a channel (wrong address or not a TP 2.0 car)")
	ErrNoAck     = errors.New("tp20: no acknowledgement")
)

// Conn is an open TP 2.0 channel; it implements diag.Channel.
type Conn struct {
	mux     *can.Mux
	sub     *can.Sub
	module  byte
	txID    uint32 // we send here
	rxID    uint32 // the control unit sends here
	txSeq   byte
	rxSeq   byte
	bs      byte
	t3      time.Duration
	log     journal.Logger
	ready   []diag.Message
	partial []byte
	closed  bool
	lastIO  time.Time
}

// Connect opens a channel to a module (0x01 engine, 0x02 gearbox,
// 0x03 ABS, 0x17 instruments, 0x19 gateway...).
func Connect(ctx context.Context, mux *can.Mux, module byte, log journal.Logger) (*Conn, error) {
	c := &Conn{mux: mux, module: module, log: journal.Or(log), bs: 0x0F, t3: 5 * time.Millisecond}
	setup := mux.Subscribe(func(f *can.Frame) bool { return !f.Ext && f.ID == 0x200+uint32(module) }, 16)
	defer setup.Close()
	req := can.NewFrame(0x200, false, []byte{module, 0xC0, 0x00, 0x10, 0x00, 0x03, 0x01})
	journal.Tx(c.log, "tp20", "200", req.Payload(), "channel setup")
	if err := mux.Send(ctx, req); err != nil {
		return nil, err
	}
	f, err := setup.Recv(ctx, time.Second)
	if err != nil {
		return nil, ErrNoChannel
	}
	p := f.Payload()
	journal.Rx(c.log, "tp20", f.IDText(), p, "channel setup answer")
	if len(p) < 7 || p[1] != 0xD0 {
		return nil, fmt.Errorf("%w: answer % X", ErrNoChannel, p)
	}
	c.txID = uint32(p[2]) | uint32(p[3]&0x0F)<<8
	c.rxID = uint32(p[4]) | uint32(p[5]&0x0F)<<8
	rx := c.rxID
	c.sub = mux.Subscribe(func(f *can.Frame) bool { return !f.Ext && f.ID == rx }, 256)
	// Channel parameters: block size 15, ACK timeout 100 ms, 5 ms
	// between packets.
	if err := c.raw(ctx, []byte{0xA0, 0x0F, 0x8A, 0xFF, 0x32, 0xFF}); err != nil {
		return nil, err
	}
	f, err = c.sub.Recv(ctx, time.Second)
	if err != nil || f.Len < 1 || f.Data[0] != 0xA1 {
		c.sub.Close()
		return nil, fmt.Errorf("tp20: no channel parameters from the control unit")
	}
	if f.Len >= 2 && f.Data[1] > 0 {
		c.bs = f.Data[1]
	}
	if f.Len >= 5 {
		c.t3 = decodeTime(f.Data[4])
	}
	journal.Infof(c.log, "tp20", "channel to module 0x%02X open: tx %03X rx %03X, block size %d", module, c.txID, c.rxID, c.bs)
	c.lastIO = time.Now()
	return c, nil
}

// decodeTime converts a TP 2.0 timing byte (2-bit unit, 6-bit value).
func decodeTime(b byte) time.Duration {
	units := []time.Duration{100 * time.Microsecond, time.Millisecond, 10 * time.Millisecond, 100 * time.Millisecond}
	return units[b>>6] * time.Duration(b&0x3F)
}

func (c *Conn) raw(ctx context.Context, p []byte) error {
	journal.Tx(c.log, "tp20", fmt.Sprintf("%03X", c.txID), p, "")
	return c.mux.Send(ctx, can.NewFrame(c.txID, false, p))
}

// Send implements diag.Channel.
func (c *Conn) Send(ctx context.Context, data []byte) error {
	if c.closed {
		return diag.ErrClosed
	}
	c.ready = c.ready[:0]
	payload := append([]byte{byte(len(data) >> 8), byte(len(data))}, data...)
	var chunks [][]byte
	for i := 0; i < len(payload); i += 7 {
		end := i + 7
		if end > len(payload) {
			end = len(payload)
		}
		chunks = append(chunks, payload[i:end])
	}
	journal.Tx(c.log, "tp20", fmt.Sprintf("%03X", c.txID), data, diag.ServiceName(data[0]))
	for i, ch := range chunks {
		last := i == len(chunks)-1
		var op byte
		waitAck := last || (i+1)%int(c.bs) == 0
		switch {
		case last:
			op = 0x10
		case waitAck:
			op = 0x00
		default:
			op = 0x20
		}
		if i > 0 {
			timing.Sleep(c.t3)
		}
		pkt := append([]byte{op | c.txSeq&0x0F}, ch...)
		if err := c.mux.Send(ctx, can.NewFrame(c.txID, false, pkt)); err != nil {
			return err
		}
		c.txSeq++
		if waitAck {
			if err := c.awaitAck(ctx); err != nil {
				return err
			}
		}
	}
	c.lastIO = time.Now()
	return nil
}

func (c *Conn) awaitAck(ctx context.Context) error {
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		f, err := c.sub.Recv(ctx, time.Until(deadline))
		if err != nil {
			break
		}
		p := f.Payload()
		if len(p) == 0 {
			continue
		}
		switch p[0] >> 4 {
		case 0xB:
			return nil
		case 0x9: // not ready: keep waiting
			deadline = time.Now().Add(500 * time.Millisecond)
		default:
			c.receive(ctx, p)
		}
	}
	return ErrNoAck
}

// receive handles one incoming packet; complete messages go to ready.
func (c *Conn) receive(ctx context.Context, p []byte) (done bool) {
	op := p[0] >> 4
	switch op {
	case 0x0, 0x1, 0x2, 0x3:
		c.rxSeq = p[0]&0x0F + 1
		c.partial = append(c.partial, p[1:]...)
		if op == 0x0 || op == 0x1 {
			_ = c.raw(ctx, []byte{0xB0 | c.rxSeq&0x0F})
		}
		if op == 0x1 || op == 0x3 {
			if len(c.partial) >= 2 {
				n := int(c.partial[0]&0x7F)<<8 | int(c.partial[1])
				body := c.partial[2:]
				if n <= len(body) {
					body = body[:n]
				}
				m := diag.Message{Src: c.rxID, Dst: c.txID, Data: append([]byte(nil), body...), Time: time.Now()}
				journal.Rx(c.log, "tp20", fmt.Sprintf("%03X", c.rxID), m.Data, diag.ServiceName(m.Data[0]))
				c.ready = append(c.ready, m)
			}
			c.partial = c.partial[:0]
			return true
		}
	case 0xA:
		if p[0] == 0xA8 {
			c.closed = true
		}
	}
	return false
}

// Recv implements diag.Channel.
func (c *Conn) Recv(ctx context.Context, timeout time.Duration) (diag.Message, error) {
	deadline := time.Now().Add(timeout)
	for {
		if len(c.ready) > 0 {
			m := c.ready[0]
			c.ready = c.ready[1:]
			c.lastIO = time.Now()
			return m, nil
		}
		if c.closed {
			return diag.Message{}, diag.ErrClosed
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return diag.Message{}, diag.ErrTimeout
		}
		f, err := c.sub.Recv(ctx, remaining)
		if err != nil {
			if errors.Is(err, can.ErrTimeout) {
				return diag.Message{}, diag.ErrTimeout
			}
			return diag.Message{}, err
		}
		if p := f.Payload(); len(p) > 0 {
			c.receive(ctx, p)
		}
	}
}

// KeepAlive sends the channel test; the control unit closes an idle
// channel after about a second.
func (c *Conn) KeepAlive(ctx context.Context) error {
	if err := c.raw(ctx, []byte{0xA3}); err != nil {
		return err
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		f, err := c.sub.Recv(ctx, time.Until(deadline))
		if err != nil {
			break
		}
		if p := f.Payload(); len(p) > 0 && p[0] == 0xA1 {
			return nil
		} else if len(p) > 0 {
			c.receive(ctx, p)
		}
	}
	return ErrNoAck
}

// Info implements diag.Channel.
func (c *Conn) Info() diag.ChannelInfo {
	return diag.ChannelInfo{Protocol: fmt.Sprintf("VW TP 2.0 (KWP2000 on CAN), module 0x%02X", c.module), ProtoID: "tp20",
		Adapter: c.mux.Bus().Info().Name, Target: uint32(c.module), Tester: c.rxID, CAN: true}
}

// Close disconnects the channel.
func (c *Conn) Close() error {
	if c.sub == nil {
		return nil
	}
	if !c.closed {
		_ = c.raw(context.Background(), []byte{0xA8})
		c.closed = true
	}
	c.sub.Close()
	return nil
}

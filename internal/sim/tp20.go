package sim

import (
	"context"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
)

// VAGApp answers VAG KWP2000 requests (used over TP 2.0).
type VAGApp struct {
	Engine *Engine
}

// Handle implements App.
func (a *VAGApp) Handle(req []byte, t Transport, functional bool) [][]byte {
	if len(req) == 0 {
		return nil
	}
	switch req[0] {
	case 0x10:
		if len(req) < 2 {
			return nrc(0x10, 0x12)
		}
		return [][]byte{{0x50, req[1]}}
	case 0x1A:
		return [][]byte{append([]byte{0x5A, 0x9B}, []byte("06J906026C  1.8l R4/4V TFSI 0030")...)}
	case 0x18:
		out := []byte{0x58, 0}
		for _, f := range a.Engine.Faults() {
			st := byte(0x24)
			if f.Active {
				st = 0x64
			}
			out = append(out, byte(f.VAG>>8), byte(f.VAG), st)
			out[1]++
		}
		return [][]byte{out}
	case 0x14:
		a.Engine.ClearFaults()
		return [][]byte{{0x54, 0xFF, 0x00}}
	case 0x21:
		if len(req) < 2 {
			return nrc(0x21, 0x12)
		}
		s := a.Engine.State()
		if req[1] != 1 {
			return nrc(0x21, 0x31)
		}
		return [][]byte{{0x61, 0x01,
			0x01, 0xC8, u8(s.RPM / 40),
			0x05, 0x0A, u8(s.CoolantC + 100),
			0x15, 0x64, u8(s.O2V * 10),
			0x10, 0xFF, 0x0B}}
	case 0x3E:
		return [][]byte{{0x7E}}
	}
	return nrc(req[0], 0x11)
}

// AttachTP20 starts a VAG control unit that speaks TP 2.0 on the bus.
func (b *CANBus) AttachTP20(module byte, app App) {
	ep := b.Endpoint("tp20")
	mux := can.NewMux(ep, nil)
	sub := mux.Subscribe(func(f *can.Frame) bool { return !f.Ext && (f.ID == 0x200 || f.ID == 0x300) }, 1024)
	const ecuTx = 0x740
	go func() {
		ctx := context.Background()
		var seqOut byte
		var partial []byte
		send := func(p []byte) { _ = mux.Send(ctx, can.NewFrame(ecuTx, false, p)) }
		waitAck := func() bool {
			deadline := time.Now().Add(500 * time.Millisecond)
			for time.Now().Before(deadline) {
				f, err := sub.Recv(ctx, time.Until(deadline))
				if err != nil {
					return false
				}
				if f.ID == 0x300 && f.Len > 0 && f.Data[0]>>4 == 0xB {
					return true
				}
			}
			return false
		}
		for {
			select {
			case <-b.done:
				return
			default:
			}
			f, err := sub.Recv(ctx, 300*time.Millisecond)
			if err != nil {
				continue
			}
			p := f.Payload()
			if len(p) == 0 {
				continue
			}
			if f.ID == 0x200 {
				if len(p) >= 7 && p[0] == module && p[1] == 0xC0 {
					_ = mux.Send(ctx, can.NewFrame(0x200+uint32(module), false, []byte{0x00, 0xD0, 0x00, 0x03, byte(ecuTx & 0xFF), byte(ecuTx >> 8), 0x01}))
					seqOut, partial = 0, nil
				}
				continue
			}
			switch {
			case p[0] == 0xA0 || p[0] == 0xA3:
				send([]byte{0xA1, 0x0F, 0x8A, 0xFF, 0x4A, 0xFF})
			case p[0] == 0xA8:
				send([]byte{0xA8})
			case p[0]>>4 <= 0x3:
				op := p[0] >> 4
				partial = append(partial, p[1:]...)
				if op == 0x0 || op == 0x1 {
					send([]byte{0xB0 | (p[0]+1)&0x0F})
				}
				if op != 0x1 && op != 0x3 {
					continue
				}
				if len(partial) < 2 {
					partial = nil
					continue
				}
				n := int(partial[0])<<8 | int(partial[1])
				req := partial[2:]
				if n < len(req) {
					req = req[:n]
				}
				partial = nil
				for _, ans := range app.Handle(req, TransportCAN, false) {
					time.Sleep(3 * time.Millisecond)
					payload := append([]byte{byte(len(ans) >> 8), byte(len(ans))}, ans...)
					for i := 0; i < len(payload); i += 7 {
						end := i + 7
						if end > len(payload) {
							end = len(payload)
						}
						op := byte(0x20)
						if end == len(payload) {
							op = 0x10
						}
						send(append([]byte{op | seqOut&0x0F}, payload[i:end]...))
						seqOut++
					}
					waitAck()
				}
			}
		}
	}()
}

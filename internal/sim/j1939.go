package sim

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/j1939"
)

// TruckEngine is a simulated J1939 engine controller (source address 0).
type TruckEngine struct {
	Engine   *Engine
	mu       sync.Mutex
	active   []j1939.DTC
	previous []j1939.DTC
	link     *j1939.CANLink
}

// AttachJ1939Engine starts a truck engine on the bus.
func (b *CANBus) AttachJ1939Engine(e *Engine) *TruckEngine {
	ep := b.Endpoint("j1939-engine")
	mux := can.NewMux(ep, nil)
	t := &TruckEngine{Engine: e, link: j1939.NewCANLink(mux, 0x00, nil),
		active:   []j1939.DTC{{SPN: 3251, FMI: 0, OC: 1}, {SPN: 110, FMI: 16, OC: 2}},
		previous: []j1939.DTC{{SPN: 100, FMI: 1, OC: 3}}}
	go t.broadcast(b.done)
	go t.answer(b.done)
	return t
}

func put(data []byte, spn uint32, v float64) {
	s := j1939.SPNByNumber(spn)
	if s == nil {
		return
	}
	raw := math.Round((v - s.Offset) / s.Scale)
	if raw < 0 {
		raw = 0
	}
	i := s.Start - 1
	if s.Bits < 8 {
		data[i] &^= byte((1<<s.Bits - 1) << s.Bit)
		data[i] |= byte(uint64(raw)&(1<<s.Bits-1)) << s.Bit
		return
	}
	u := uint64(raw)
	for k := 0; k < s.Bits/8; k++ {
		data[i+k] = byte(u >> (8 * k))
	}
}

func filled(n int) []byte {
	d := make([]byte, n)
	for i := range d {
		d[i] = 0xFF
	}
	return d
}

func (t *TruckEngine) pgn(pgn uint32) []byte {
	s := t.Engine.State()
	d := filled(8)
	rpm := s.RPM * 0.75 // diesels idle lower
	switch pgn {
	case 61444: // EEC1
		put(d, 190, rpm)
		put(d, 513, 25+s.ThrottlePct*0.6)
		put(d, 512, 20+s.ThrottlePct*0.6)
	case 61443: // EEC2
		put(d, 91, s.PedalPct)
		put(d, 92, s.LoadPct)
	case 65262: // ET1
		put(d, 110, s.CoolantC)
		put(d, 174, 35)
		put(d, 175, s.OilC)
		put(d, 52, 38)
	case 65263: // EFL/P1
		put(d, 94, 420)
		put(d, 98, 92)
		put(d, 100, 180+rpm*0.12)
		put(d, 111, 88)
	case 65265: // CCVS
		put(d, 84, s.SpeedKmh)
		put(d, 70, 1)
		put(d, 597, 0)
		put(d, 598, 0)
	case 65266: // LFE
		put(d, 183, s.FuelLh*3.2)
		put(d, 51, s.ThrottlePct)
	case 65271: // VEP1
		put(d, 168, s.BatteryV*2)
		put(d, 167, s.BatteryV*2)
		put(d, 115, 28)
	case 65269: // AMB
		put(d, 108, s.BaroKPa)
		put(d, 171, s.AmbientC)
		put(d, 172, s.IntakeC)
	case 65270: // IC1
		put(d, 102, 20+s.ThrottlePct*1.5)
		put(d, 105, s.IntakeC+8)
		put(d, 107, 1.2)
		put(d, 173, s.CatTempC*0.8)
	case 65276: // DD
		put(d, 96, s.FuelLevelPct)
	case 65253: // HOURS
		put(d, 247, 12345.6)
	case 65248: // VD
		put(d, 245, 486213.5)
	case 65257: // LFC
		put(d, 250, 152340)
	case 61445: // ETC2
		put(d, 523, 0)
		put(d, 524, 0)
	default:
		return nil
	}
	return d
}

func (t *TruckEngine) send(pgn uint32, data []byte, dst byte) {
	_ = t.link.Send(context.Background(), j1939.Message{PGN: pgn, Priority: 6, Src: 0x00, Dst: dst, Data: data})
}

func (t *TruckEngine) dm(active bool) []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	if active {
		return j1939.EncodeDM(j1939.Lamps{AmberWarning: len(t.active) > 0}, t.active)
	}
	return j1939.EncodeDM(j1939.Lamps{}, t.previous)
}

func (t *TruckEngine) broadcast(done <-chan struct{}) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	n := 0
	for {
		select {
		case <-done:
			return
		case <-tick.C:
		}
		n++
		for _, p := range []uint32{61444, 65265, 65266} {
			t.send(p, t.pgn(p), j1939.AddrGlobal)
		}
		if n%5 == 0 {
			for _, p := range []uint32{61443, 65263, 65270, 61445} {
				t.send(p, t.pgn(p), j1939.AddrGlobal)
			}
		}
		if n%10 == 0 {
			for _, p := range []uint32{65262, 65271, 65269, 65276} {
				t.send(p, t.pgn(p), j1939.AddrGlobal)
			}
			go t.send(j1939.PGNDM1, t.dm(true), j1939.AddrGlobal)
		}
	}
}

func (t *TruckEngine) answer(done <-chan struct{}) {
	ctx := context.Background()
	for {
		select {
		case <-done:
			return
		default:
		}
		m, err := t.link.Recv(ctx, 300*time.Millisecond)
		if err != nil || m.PGN != j1939.PGNRequest || len(m.Data) < 3 {
			continue
		}
		if m.Dst != 0x00 && m.Dst != j1939.AddrGlobal {
			continue
		}
		pgn := uint32(m.Data[0]) | uint32(m.Data[1])<<8 | uint32(m.Data[2])<<16
		reply := m.Src
		if m.Dst == j1939.AddrGlobal {
			reply = j1939.AddrGlobal
		}
		switch pgn {
		case j1939.PGNVIN:
			go t.send(pgn, []byte(t.Engine.VIN+"*"), reply)
		case j1939.PGNComponentID:
			go t.send(pgn, []byte("SIM*DIESEL 12L*SN123456*ENG01*"), reply)
		case j1939.PGNSoftwareID:
			go t.send(pgn, append([]byte{1}, []byte("UDT-SIM-ECM 4.2*")...), reply)
		case j1939.PGNDM1:
			go t.send(pgn, t.dm(true), reply)
		case j1939.PGNDM2:
			go t.send(pgn, t.dm(false), reply)
		case j1939.PGNDM11:
			t.mu.Lock()
			t.active = nil
			t.mu.Unlock()
		case j1939.PGNDM3:
			t.mu.Lock()
			t.previous = nil
			t.mu.Unlock()
		default:
			if d := t.pgn(pgn); d != nil {
				go t.send(pgn, d, reply)
			}
		}
	}
}

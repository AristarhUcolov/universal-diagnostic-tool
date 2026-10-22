package sim

import (
	"time"
)

// KW1281Node simulates a VAG control unit speaking KW1281.
type KW1281Node struct {
	Addr   byte // logical address, 0x01 engine
	Baud   int  // 9600 or 10400
	Engine *Engine
	ID     []string
	in     chan kEvent
	cnt    byte
}

// NewKW1281Node creates a VAG engine unit at address 0x01, 9600 baud.
func NewKW1281Node(e *Engine) *KW1281Node {
	return &KW1281Node{Addr: 0x01, Baud: 9600, Engine: e, in: make(chan kEvent, 1024),
		ID: []string{"06A906032HS ", "1.8L R4/5VT G  ", "0002", "Coding 00031"}}
}

func (n *KW1281Node) input() chan kEvent { return n.in }

func (n *KW1281Node) run(bus *KBus) {
	for {
		ev, ok := next(bus, n.in, 0)
		if !ok {
			return
		}
		if ev.kind != ev5Baud || ev.b&0x7F != n.Addr {
			continue
		}
		bus.sleep(2*time.Second + 30*time.Millisecond) // end of the 5-baud address + W1
		bus.emit([]byte{0x55}, n.Baud)
		bus.sleep(5 * time.Millisecond)
		bus.emit([]byte{0x01}, n.Baud)
		bus.sleep(5 * time.Millisecond)
		bus.emit([]byte{0x8A}, n.Baud)
		if !n.waitByte(bus, 0x75, time.Second) {
			continue
		}
		n.session(bus)
	}
}

// waitByte waits for a specific byte from the tester.
func (n *KW1281Node) waitByte(bus *KBus, want byte, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		ev, ok := next(bus, n.in, time.Until(deadline))
		if !ok {
			return false
		}
		if ev.kind == evByte {
			return ev.b == want
		}
	}
}

func (n *KW1281Node) readByte(bus *KBus, timeout time.Duration) (byte, bool) {
	deadline := time.Now().Add(timeout)
	for {
		ev, ok := next(bus, n.in, time.Until(deadline))
		if !ok {
			return 0, false
		}
		if ev.kind == evByte {
			if ev.baud != n.Baud {
				return 0, false
			}
			return ev.b, true
		}
	}
}

func (n *KW1281Node) send(bus *KBus, title byte, data []byte) bool {
	n.cnt++
	raw := append([]byte{byte(len(data) + 3), n.cnt, title}, data...)
	raw = append(raw, 0x03)
	bus.sleep(10 * time.Millisecond)
	for i, b := range raw {
		bus.emit([]byte{b}, n.Baud)
		if i == len(raw)-1 {
			break
		}
		ack, ok := n.readByte(bus, 300*time.Millisecond)
		if !ok || ack != ^b {
			return false
		}
		bus.sleep(time.Millisecond)
	}
	return true
}

func (n *KW1281Node) recv(bus *KBus) (byte, []byte, bool) {
	ln, ok := n.readByte(bus, 2*time.Second)
	if !ok || ln < 3 {
		return 0, nil, false
	}
	bus.emit([]byte{^ln}, n.Baud)
	body := make([]byte, 0, ln)
	for i := 0; i < int(ln)-1; i++ {
		b, ok := n.readByte(bus, 300*time.Millisecond)
		if !ok {
			return 0, nil, false
		}
		bus.emit([]byte{^b}, n.Baud)
		body = append(body, b)
	}
	end, ok := n.readByte(bus, 300*time.Millisecond)
	if !ok || end != 0x03 {
		return 0, nil, false
	}
	n.cnt = body[0]
	return body[1], body[2:], true
}

func (n *KW1281Node) session(bus *KBus) {
	n.cnt = 0
	for _, id := range n.ID {
		if !n.send(bus, 0xF6, []byte(id)) {
			return
		}
		if t, _, ok := n.recv(bus); !ok || t != 0x09 {
			return
		}
	}
	if !n.send(bus, 0x09, nil) {
		return
	}
	for {
		title, data, ok := n.recv(bus)
		if !ok {
			return
		}
		switch title {
		case 0x09:
			if !n.send(bus, 0x09, nil) {
				return
			}
		case 0x06:
			return
		case 0x07, 0x05:
			if title == 0x05 {
				n.Engine.ClearFaults()
			}
			var recs []byte
			for _, f := range n.Engine.Faults() {
				st := byte(0x23) // implausible-ish signal
				if !f.Active {
					st |= 0x80
				}
				recs = append(recs, byte(f.VAG>>8), byte(f.VAG), st)
			}
			if len(recs) == 0 {
				recs = []byte{0xFF, 0xFF, 0x88}
			}
			for len(recs) > 0 {
				chunk := recs
				if len(chunk) > 12 {
					chunk = chunk[:12]
				}
				recs = recs[len(chunk):]
				if !n.send(bus, 0xFC, chunk) {
					return
				}
				if t, _, ok := n.recv(bus); !ok || t != 0x09 {
					return
				}
			}
			if !n.send(bus, 0x09, nil) {
				return
			}
		case 0x29:
			if len(data) < 1 {
				return
			}
			s := n.Engine.State()
			var cells []byte
			switch data[0] {
			case 1:
				cells = []byte{
					0x01, 0xC8, u8(s.RPM / 40), // rpm: a*b/5 with a=200 -> b = rpm/40
					0x05, 0x0A, u8(s.CoolantC + 100), // a*(b-100)*0.1 with a=10
					0x15, 0x64, u8(s.O2V * 1000 / 100), // V: 0.001*a*b with a=100
					0x10, 0xFF, 0x0B,
				}
			case 2:
				cells = []byte{
					0x01, 0xC8, u8(s.RPM / 40),
					0x21, 0x64, u8(s.LoadPct), // %: 100*b/a with a=100
					0x0F, 0x0A, u8(s.InjMs * 10), // ms: 0.01*a*b with a=10
					0x08, 0x01, u8(s.MAFkgh / 3.6 * 10), // g/s: 0.1*a*b with a=1
				}
			case 3:
				cells = []byte{
					0x01, 0xC8, u8(s.RPM / 40),
					0x06, 0x64, u8(s.BatteryV * 10), // V: 0.001*a*b with a=100
					0x05, 0x0A, u8(s.CoolantC + 100),
					0x05, 0x0A, u8(s.IntakeC + 100),
				}
			default:
				if !n.send(bus, 0x0A, nil) {
					return
				}
				continue
			}
			if !n.send(bus, 0xE7, cells) {
				return
			}
		default:
			if !n.send(bus, 0x0A, nil) {
				return
			}
		}
	}
}

package sim

import (
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/legacy"
)

// Transport tells an application which bus a request came from, because
// some OBD answers are formatted differently on K-Line and CAN.
type Transport int

const (
	TransportKLine Transport = iota
	TransportCAN
)

// App is the application layer of a simulated control unit: it turns a
// request into zero or more answers.
type App interface {
	Handle(req []byte, t Transport, functional bool) [][]byte
}

func checksum(b []byte) byte {
	var s byte
	for _, v := range b {
		s += v
	}
	return s
}

// KWPNode is a control unit speaking ISO 14230 KWP2000 on the K-Line.
type KWPNode struct {
	Addr       byte // physical address, 0x10 for engines
	Baud       int
	KB1, KB2   byte
	Functional bool // also answers the OBD functional address 0x33
	SlowInit   bool // accepts 5-baud initialisation
	App        App
	// LenByteFrom: answers with at least this many data bytes use the
	// separate length byte (0 = only when longer than 63 bytes).
	LenByteFrom int
	P2          time.Duration

	in    chan kEvent
	buf   []byte
	awake bool
}

// NewKWPNode returns a node with the AvtoVAZ key bytes 6B 8F.
func NewKWPNode(addr byte, app App) *KWPNode {
	return &KWPNode{Addr: addr, Baud: 10400, KB1: 0x6B, KB2: 0x8F, App: app, P2: 12 * time.Millisecond, in: make(chan kEvent, 1024)}
}

func (n *KWPNode) input() chan kEvent { return n.in }

func (n *KWPNode) run(bus *KBus) {
	for {
		ev, ok := next(bus, n.in, 0)
		if !ok {
			return
		}
		switch ev.kind {
		case evWake:
			n.buf, n.awake = nil, false
		case ev5Baud:
			if !n.SlowInit || (ev.b != n.Addr && !(n.Functional && ev.b == 0x33)) {
				continue
			}
			bus.sleep(2*time.Second + 30*time.Millisecond) // end of the 5-baud address + W1
			bus.emit([]byte{0x55}, n.Baud)
			bus.sleep(8 * time.Millisecond)
			bus.emit([]byte{n.KB1}, n.Baud)
			bus.sleep(8 * time.Millisecond)
			bus.emit([]byte{n.KB2}, n.Baud)
			// Wait for the inverted key byte 2, then confirm.
			for {
				ev2, ok := next(bus, n.in, 2*time.Second)
				if !ok {
					break
				}
				if ev2.kind == evByte && ev2.b == ^n.KB2 {
					bus.sleep(30 * time.Millisecond)
					bus.emit([]byte{^ev.b}, n.Baud)
					n.awake, n.buf = true, nil
					break
				}
			}
		case evByte:
			if ev.baud != n.Baud {
				n.buf = nil
				continue
			}
			n.buf = append(n.buf, ev.b)
			n.tryFrame(bus)
		case evEnd:
			n.buf = nil
		}
	}
}

// tryFrame checks whether the buffer holds a complete request frame.
func (n *KWPNode) tryFrame(bus *KBus) {
	b := n.buf
	if len(b) < 1 {
		return
	}
	mode := b[0] >> 6
	length := int(b[0] & 0x3F)
	pos := 1
	var tgt, src byte
	if mode != 0 {
		if len(b) < 3 {
			return
		}
		tgt, src = b[1], b[2]
		pos = 3
	}
	if length == 0 {
		if len(b) < pos+1 {
			return
		}
		length = int(b[pos])
		pos++
	}
	total := pos + length + 1
	if len(b) < total {
		return
	}
	frame := b[:total]
	n.buf = append([]byte(nil), b[total:]...)
	if checksum(frame[:total-1]) != frame[total-1] {
		return
	}
	functional := mode == 3
	if mode != 0 && !(tgt == n.Addr || (n.Functional && functional && tgt == 0x33)) {
		return
	}
	if src == 0 {
		src = 0xF1
	}
	data := frame[pos : pos+length]
	var answers [][]byte
	switch {
	case len(data) == 1 && data[0] == 0x81:
		n.awake = true
		answers = [][]byte{{0xC1, n.KB1, n.KB2}}
	case !n.awake:
		return
	case len(data) == 1 && data[0] == 0x82:
		n.awake = false
		answers = [][]byte{{0xC2}}
	default:
		answers = n.App.Handle(append([]byte(nil), data...), TransportKLine, functional)
	}
	for i, a := range answers {
		if i == 0 {
			bus.sleep(n.P2)
		} else {
			bus.sleep(n.P2 + 5*time.Millisecond)
		}
		bus.emit(n.frame(src, a), n.Baud)
	}
}

func (n *KWPNode) frame(tester byte, data []byte) []byte {
	var f []byte
	if len(data) <= 63 && (n.LenByteFrom == 0 || len(data) < n.LenByteFrom) {
		f = append(f, 0x80|byte(len(data)), tester, n.Addr)
	} else {
		f = append(f, 0x80, tester, n.Addr, byte(len(data)))
	}
	f = append(f, data...)
	return append(f, checksum(f))
}

// ISO9141Node is an OBD-II control unit on ISO 9141-2.
type ISO9141Node struct {
	Addr  byte // source address in answers (0x10 engine)
	App   App
	in    chan kEvent
	buf   []byte
	awake bool
}

// NewISO9141Node creates the node.
func NewISO9141Node(addr byte, app App) *ISO9141Node {
	return &ISO9141Node{Addr: addr, App: app, in: make(chan kEvent, 1024)}
}

func (n *ISO9141Node) input() chan kEvent { return n.in }

func (n *ISO9141Node) run(bus *KBus) {
	for {
		ev, ok := next(bus, n.in, 0)
		if !ok {
			return
		}
		switch ev.kind {
		case ev5Baud:
			if ev.b != 0x33 {
				continue
			}
			bus.sleep(2*time.Second + 30*time.Millisecond) // end of the 5-baud address + W1
			bus.emit([]byte{0x55}, 10400)
			bus.sleep(8 * time.Millisecond)
			bus.emit([]byte{0x08}, 10400)
			bus.sleep(8 * time.Millisecond)
			bus.emit([]byte{0x08}, 10400)
			for {
				ev2, ok := next(bus, n.in, 2*time.Second)
				if !ok {
					break
				}
				if ev2.kind == evByte && ev2.b == 0xF7 {
					bus.sleep(30 * time.Millisecond)
					bus.emit([]byte{0xCC}, 10400)
					n.awake, n.buf = true, nil
					break
				}
			}
		case evByte:
			if ev.baud == 10400 {
				n.buf = append(n.buf, ev.b)
			}
		case evEnd:
			frame := n.buf
			n.buf = nil
			if !n.awake || len(frame) < 5 || frame[0] != 0x68 || frame[1] != 0x6A {
				continue
			}
			if checksum(frame[:len(frame)-1]) != frame[len(frame)-1] {
				continue
			}
			for i, a := range n.App.Handle(frame[3:len(frame)-1], TransportKLine, true) {
				if i == 0 {
					bus.sleep(30 * time.Millisecond)
				} else {
					bus.sleep(35 * time.Millisecond)
				}
				f := append([]byte{0x48, 0x6B, n.Addr}, a...)
				bus.emit(append(f, checksum(f)), 10400)
			}
		}
	}
}

// LegacyNode is a Микас-7.1 style controller (9600 baud, 0x0D framed).
type LegacyNode struct {
	Engine  *Engine
	Version byte // 0x0A = Микас-7.1, 0x09 = Микас-5.4
	in      chan kEvent
	buf     []byte
	clear1  bool
}

// NewLegacyNode creates a Микас-7.1 simulation.
func NewLegacyNode(e *Engine) *LegacyNode {
	return &LegacyNode{Engine: e, Version: 0x0A, in: make(chan kEvent, 1024)}
}

func (n *LegacyNode) input() chan kEvent { return n.in }

func (n *LegacyNode) run(bus *KBus) {
	for {
		ev, ok := next(bus, n.in, 0)
		if !ok {
			return
		}
		if ev.kind != evByte {
			continue
		}
		if ev.baud != 9600 {
			n.buf = nil
			continue
		}
		if ev.b != 0x0D {
			n.buf = append(n.buf, ev.b)
			continue
		}
		body, err := legacy.Decode(n.buf)
		n.buf = nil
		if err != nil || len(body) == 0 {
			continue
		}
		resp := n.handle(body)
		if resp == nil {
			continue
		}
		bus.sleep(15 * time.Millisecond)
		bus.emit(legacy.Encode(resp), 9600)
	}
}

func (n *LegacyNode) handle(req []byte) []byte {
	switch c := req[0]; {
	case c == legacy.CmdVersion:
		return []byte{n.Version}
	case c == legacy.CmdReadErrors:
		faults := n.Engine.Faults()
		out := []byte{byte(len(faults))}
		for _, f := range faults {
			out = append(out, f.Mikas, 0xE0)
		}
		return out
	case c == legacy.CmdWriteParam && len(req) >= 3:
		if req[1] == 0x0E && req[2] == 0x08 {
			n.clear1 = true
		} else if req[1] == 0x0E && req[2] == 0x00 && n.clear1 {
			n.clear1 = false
			n.Engine.ClearFaults()
		}
		return []byte{0x00}
	case c >= 0x51 && c <= 0x53:
		texts := []string{"MIKAS 7.1 ZMZ406", "SOFT 4062.3763  ", "000 SIMULATOR   "}
		return padText(texts[c-0x51])
	case c >= 0x54 && c <= 0x58:
		texts := []string{"GAZ-3110 CALIBR.", "ZMZ-4062.10 EVRO", "0-2003 DATA SIM ", "                ", "                "}
		return padText(texts[c-0x54])
	case c == legacy.CmdNumParams:
		return []byte{0x80}
	case c == legacy.CmdReadParams:
		s := n.Engine.State()
		var out []byte
		for _, code := range req[1:] {
			out = append(out, mikasParam(code, s, n.Engine)...)
		}
		return out
	}
	return nil
}

func padText(s string) []byte {
	b := []byte(s)
	for len(b) < 16 {
		b = append(b, 0)
	}
	return b[:16]
}

func u16le(v float64) []byte {
	if v < 0 {
		v = 0
	}
	if v > 65535 {
		v = 65535
	}
	x := uint16(v + 0.5)
	return []byte{byte(x), byte(x >> 8)}
}

func u8(v float64) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v + 0.5)
}

func s8(v float64) byte {
	if v < -128 {
		v = -128
	}
	if v > 127 {
		v = 127
	}
	if v < 0 {
		return byte(int8(v - 0.5))
	}
	return byte(int8(v + 0.5))
}

// mikasParam encodes one parameter of the Микас-5.4/7.1 table.
func mikasParam(code byte, s State, e *Engine) []byte {
	switch code {
	case 0x1A:
		return []byte{u8(s.CoolantC + 40)}
	case 0x19:
		return []byte{u8(e.coolant0 + 40)}
	case 0x1C:
		return []byte{u8(s.IntakeC + 40)}
	case 0x29:
		return []byte{u8(s.RPM / 40)}
	case 0x2C:
		return []byte{u8(s.RPM / 10)}
	case 0x26:
		return []byte{s8(s.AdvanceDeg * 2)}
	case 0x28:
		return []byte{0}
	case 0x1E:
		return []byte{u8(s.BatteryV * 10)}
	case 0x3F:
		return u16le(s.InjMs * 125)
	case 0x21:
		return u16le(s.MAFkgh * 100)
	case 0x59:
		return u16le(s.MAFkgh * 98)
	case 0x40:
		return u16le(s.FuelLh * 10)
	case 0x08:
		if s.Knock {
			return []byte{0x40}
		}
		return []byte{0}
	case 0x07:
		var f byte
		if s.Idle {
			f |= 0x04
		}
		if s.ThrottlePct > 70 {
			f |= 0x20
		}
		return []byte{f}
	case 0x39:
		return []byte{u8((s.AFR/14.7 - 0.5) * 256)}
	case 0x20:
		return []byte{u8(s.ThrottlePct)}
	case 0x41, 0x42:
		return []byte{128}
	case 0x5B:
		return []byte{u8(s.DesiredIACSteps)}
	case 0x5C:
		return []byte{u8(s.IACSteps)}
	case 0x72:
		faults := e.Faults()
		if len(faults) == 0 {
			return []byte{0}
		}
		min := faults[0].Mikas
		for _, f := range faults {
			if f.Mikas < min {
				min = f.Mikas
			}
		}
		return []byte{min}
	}
	return []byte{0}
}

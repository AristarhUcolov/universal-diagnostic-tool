// Package j1939 implements SAE J1939, the network of trucks, buses,
// tractors and construction machines: parameter group numbers (PGN),
// suspect parameter numbers (SPN), the transport protocol for messages
// longer than 8 bytes (BAM and RTS/CTS), address claiming and the
// diagnostic messages DM1/DM2/DM3/DM11.
package j1939

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
)

// Well-known PGNs.
const (
	PGNRequest      = 59904 // 0xEA00
	PGNAck          = 59392 // 0xE800
	PGNAddressClaim = 60928 // 0xEE00
	PGNTPCM         = 60416 // 0xEC00 transport protocol connection management
	PGNTPDT         = 60160 // 0xEB00 transport protocol data transfer
	PGNDM1          = 65226 // active trouble codes
	PGNDM2          = 65227 // previously active trouble codes
	PGNDM3          = 65228 // clear previously active
	PGNDM11         = 65235 // clear active
	PGNVIN          = 65260
	PGNComponentID  = 65259
	PGNSoftwareID   = 65242
	PGNECUID        = 64965
)

// Addresses.
const (
	AddrGlobal = 0xFF
	AddrNull   = 0xFE
	AddrTool1  = 0xF9 // off-board diagnostic-service tool #1
)

// Message is a complete J1939 message.
type Message struct {
	PGN      uint32    `json:"pgn"`
	Priority byte      `json:"priority"`
	Src      byte      `json:"src"`
	Dst      byte      `json:"dst"`
	Data     []byte    `json:"data"`
	Time     time.Time `json:"time"`
}

// ID builds the 29-bit CAN identifier.
func (m Message) ID() uint32 {
	pf := byte(m.PGN >> 8)
	ps := byte(m.PGN)
	if pf < 240 {
		ps = m.Dst
	}
	dp := (m.PGN >> 16) & 0x03
	return uint32(m.Priority&7)<<26 | dp<<24 | uint32(pf)<<16 | uint32(ps)<<8 | uint32(m.Src)
}

// Decode splits a 29-bit identifier.
func Decode(id uint32) (pgn uint32, prio, src, dst byte) {
	prio = byte(id>>26) & 7
	dp := (id >> 24) & 0x03
	pf := byte(id >> 16)
	ps := byte(id >> 8)
	src = byte(id)
	if pf < 240 {
		return dp<<16 | uint32(pf)<<8, prio, src, ps
	}
	return dp<<16 | uint32(pf)<<8 | uint32(ps), prio, src, AddrGlobal
}

// Link sends and receives J1939 messages.
type Link interface {
	Send(ctx context.Context, m Message) error
	Recv(ctx context.Context, timeout time.Duration) (Message, error)
	Address() byte
	Name() string
	Close() error
}

type tpSession struct {
	pgn     uint32
	size    int
	packets int
	data    []byte
	next    byte
	src     byte
	dst     byte
	last    time.Time
}

// CANLink runs J1939 on a CAN bus, including the transport protocol.
type CANLink struct {
	mux      *can.Mux
	sub      *can.Sub
	addr     byte
	name     uint64
	log      journal.Logger
	mu       sync.Mutex
	sessions map[[2]byte]*tpSession
	ready    []Message
	cts      chan [8]byte // flow control for our RTS/CTS transmissions
}

// ToolName is the 64-bit NAME of this program: arbitrary address
// capable, industry group 0, function 129 (off-board diagnostic tool).
const ToolName = uint64(1)<<63 | uint64(129)<<40 | 0x1F0F0

// NewCANLink attaches to a CAN bus mux using our source address.
func NewCANLink(mux *can.Mux, addr byte, log journal.Logger) *CANLink {
	l := &CANLink{mux: mux, addr: addr, name: ToolName, log: journal.Or(log),
		sessions: map[[2]byte]*tpSession{}, cts: make(chan [8]byte, 4)}
	l.sub = mux.Subscribe(func(f *can.Frame) bool { return f.Ext && !f.Echo }, 4096)
	return l
}

// Address returns our source address.
func (l *CANLink) Address() byte { return l.addr }

// Name describes the link.
func (l *CANLink) Name() string { return "J1939 on " + l.mux.Bus().Info().Name }

// ClaimAddress announces our address and NAME.
func (l *CANLink) ClaimAddress(ctx context.Context) error {
	var d [8]byte
	for i := 0; i < 8; i++ {
		d[i] = byte(l.name >> (8 * i))
	}
	return l.sendFrame(ctx, Message{PGN: PGNAddressClaim, Priority: 6, Src: l.addr, Dst: AddrGlobal, Data: d[:]})
}

func (l *CANLink) sendFrame(ctx context.Context, m Message) error {
	f := can.NewFrame(m.ID(), true, m.Data)
	return l.mux.Send(ctx, f)
}

// Request asks dst (or everybody with AddrGlobal) to send a PGN.
func (l *CANLink) Request(ctx context.Context, pgn uint32, dst byte) error {
	return l.sendFrame(ctx, Message{PGN: PGNRequest, Priority: 6, Src: l.addr, Dst: dst,
		Data: []byte{byte(pgn), byte(pgn >> 8), byte(pgn >> 16)}})
}

// Send transmits a message, using BAM for broadcasts longer than 8
// bytes and RTS/CTS for destination-specific ones.
func (l *CANLink) Send(ctx context.Context, m Message) error {
	if m.Src == 0 && l.addr != 0 {
		m.Src = l.addr
	}
	if m.Priority == 0 {
		m.Priority = 6
	}
	if len(m.Data) <= 8 {
		return l.sendFrame(ctx, m)
	}
	n := len(m.Data)
	packets := (n + 6) / 7
	pgnb := []byte{byte(m.PGN), byte(m.PGN >> 8), byte(m.PGN >> 16)}
	if m.Dst == AddrGlobal {
		cm := append([]byte{32, byte(n), byte(n >> 8), byte(packets), 0xFF}, pgnb...)
		if err := l.sendFrame(ctx, Message{PGN: PGNTPCM, Priority: 7, Src: m.Src, Dst: AddrGlobal, Data: cm}); err != nil {
			return err
		}
		for i := 0; i < packets; i++ {
			time.Sleep(50 * time.Millisecond)
			if err := l.sendDT(ctx, m, i, AddrGlobal); err != nil {
				return err
			}
		}
		return nil
	}
	rts := append([]byte{16, byte(n), byte(n >> 8), byte(packets), 0xFF}, pgnb...)
	if err := l.sendFrame(ctx, Message{PGN: PGNTPCM, Priority: 7, Src: m.Src, Dst: m.Dst, Data: rts}); err != nil {
		return err
	}
	sent := 0
	for sent < packets {
		var cts [8]byte
		select {
		case cts = <-l.cts:
		case <-time.After(1250 * time.Millisecond):
			return fmt.Errorf("j1939: no CTS from 0x%02X", m.Dst)
		case <-ctx.Done():
			return ctx.Err()
		}
		count, next := int(cts[1]), int(cts[2])
		for i := 0; i < count && next-1+i < packets; i++ {
			if err := l.sendDT(ctx, m, next-1+i, m.Dst); err != nil {
				return err
			}
			sent = next + i
		}
	}
	return nil
}

func (l *CANLink) sendDT(ctx context.Context, m Message, i int, dst byte) error {
	dt := make([]byte, 8)
	dt[0] = byte(i + 1)
	for j := 0; j < 7; j++ {
		k := i*7 + j
		if k < len(m.Data) {
			dt[1+j] = m.Data[k]
		} else {
			dt[1+j] = 0xFF
		}
	}
	return l.sendFrame(ctx, Message{PGN: PGNTPDT, Priority: 7, Src: m.Src, Dst: dst, Data: dt})
}

// Recv returns the next complete message.
func (l *CANLink) Recv(ctx context.Context, timeout time.Duration) (Message, error) {
	deadline := time.Now().Add(timeout)
	for {
		l.mu.Lock()
		if len(l.ready) > 0 {
			m := l.ready[0]
			l.ready = l.ready[1:]
			l.mu.Unlock()
			return m, nil
		}
		l.mu.Unlock()
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return Message{}, ErrTimeout
		}
		f, err := l.sub.Recv(ctx, remaining)
		if err != nil {
			if errors.Is(err, can.ErrTimeout) {
				return Message{}, ErrTimeout
			}
			return Message{}, err
		}
		l.process(ctx, f)
	}
}

// ErrTimeout is returned when no message arrived.
var ErrTimeout = errors.New("j1939: no message (timeout)")

func (l *CANLink) process(ctx context.Context, f can.Frame) {
	pgn, prio, src, dst := Decode(f.ID)
	data := f.Payload()
	m := Message{PGN: pgn, Priority: prio, Src: src, Dst: dst, Time: f.Time}
	switch pgn {
	case PGNTPCM:
		if len(data) < 8 {
			return
		}
		key := [2]byte{src, dst}
		tpPGN := uint32(data[5]) | uint32(data[6])<<8 | uint32(data[7])<<16
		switch data[0] {
		case 32, 16: // BAM, RTS
			l.mu.Lock()
			l.sessions[key] = &tpSession{pgn: tpPGN, size: int(data[1]) | int(data[2])<<8, packets: int(data[3]),
				next: 1, src: src, dst: dst, last: time.Now()}
			l.mu.Unlock()
			if data[0] == 16 && dst == l.addr {
				cts := []byte{17, data[3], 1, 0xFF, 0xFF, data[5], data[6], data[7]}
				_ = l.sendFrame(ctx, Message{PGN: PGNTPCM, Priority: 7, Src: l.addr, Dst: src, Data: cts})
			}
		case 17: // CTS for our transmission
			if dst == l.addr {
				var c [8]byte
				copy(c[:], data)
				select {
				case l.cts <- c:
				default:
				}
			}
		case 255: // abort
			l.mu.Lock()
			delete(l.sessions, key)
			l.mu.Unlock()
		}
		return
	case PGNTPDT:
		if len(data) < 8 {
			return
		}
		key := [2]byte{src, dst}
		l.mu.Lock()
		s := l.sessions[key]
		if s == nil || data[0] != s.next {
			l.mu.Unlock()
			return
		}
		s.data = append(s.data, data[1:8]...)
		s.next++
		s.last = time.Now()
		done := int(s.next-1) >= s.packets
		if done {
			delete(l.sessions, key)
			if len(s.data) > s.size {
				s.data = s.data[:s.size]
			}
			l.ready = append(l.ready, Message{PGN: s.pgn, Priority: 7, Src: src, Dst: dst, Data: s.data, Time: f.Time})
		}
		l.mu.Unlock()
		if done && dst == l.addr {
			ack := []byte{19, byte(s.size), byte(s.size >> 8), byte(s.packets), 0xFF, byte(s.pgn), byte(s.pgn >> 8), byte(s.pgn >> 16)}
			_ = l.sendFrame(ctx, Message{PGN: PGNTPCM, Priority: 7, Src: l.addr, Dst: src, Data: ack})
		}
		return
	}
	m.Data = append([]byte(nil), data...)
	l.mu.Lock()
	l.ready = append(l.ready, m)
	l.mu.Unlock()
}

// Close releases the subscription.
func (l *CANLink) Close() error {
	l.sub.Close()
	return nil
}

// Package iso9141 implements OBD-II over ISO 9141-2 (K-Line, 10400 baud,
// 5-baud initialisation with address 0x33). It was used by most
// European and Asian cars from 1996 to about 2004. Messages carry a fixed
// three-byte header (68 6A F1 for requests, 48 6B xx for answers) and no
// length, so the end of an answer is found from its checksum.
package iso9141

import (
	"context"
	"fmt"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kline"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/timing"
)

const maxFrame = 11 // 3 header + 7 data + checksum

// Link is an initialised ISO 9141-2 connection (implements diag.Channel).
type Link struct {
	line   kline.Line
	kb     kline.SlowInitResult
	back   []byte // bytes read ahead that belong to the next frame
	lastIO time.Time
	log    journal.Logger
	closed bool
	// P4 is the gap between request bytes (ISO 9141-2: 5-20 ms).
	P4 time.Duration
	// Quiet is how long the bus must stay silent after a byte that
	// completes a valid checksum before the frame is considered done.
	Quiet time.Duration
}

// Open performs the 5-baud initialisation with address 0x33.
func Open(ctx context.Context, line kline.Line, log journal.Logger) (*Link, error) {
	if err := line.SetBaud(10400); err != nil {
		return nil, err
	}
	r, err := kline.SlowInit(line, 0x33, true, log)
	if err != nil {
		return nil, err
	}
	l := &Link{line: line, kb: r, log: journal.Or(log), P4: 5 * time.Millisecond, Quiet: 25 * time.Millisecond, lastIO: time.Now()}
	if !(r.KB1 == 0x08 && r.KB2 == 0x08) && !(r.KB1 == 0x94 && r.KB2 == 0x94) {
		journal.Warnf(l.log, "iso9141", "key bytes %02X %02X are not the ISO 9141-2 values 08 08 / 94 94", r.KB1, r.KB2)
	}
	return l, nil
}

func checksum(b []byte) byte {
	var s byte
	for _, v := range b {
		s += v
	}
	return s
}

// Send implements diag.Channel.
func (l *Link) Send(ctx context.Context, data []byte) error {
	if l.closed {
		return diag.ErrClosed
	}
	if len(data) == 0 || len(data) > 7 {
		return fmt.Errorf("iso9141: request must be 1..7 bytes, got %d", len(data))
	}
	timing.Until(l.lastIO.Add(55 * time.Millisecond)) // P3min
	frame := append([]byte{0x68, 0x6A, 0xF1}, data...)
	frame = append(frame, checksum(frame))
	journal.Tx(l.log, "iso9141", "0x33", frame, diag.ServiceName(data[0]))
	err := l.line.Send(frame, l.P4)
	l.lastIO = time.Now()
	l.back = l.back[:0]
	return err
}

func (l *Link) readByte(timeout time.Duration) (byte, error) {
	if len(l.back) > 0 {
		b := l.back[0]
		l.back = l.back[1:]
		return b, nil
	}
	return l.line.RecvByte(timeout)
}

// Recv implements diag.Channel.
func (l *Link) Recv(ctx context.Context, timeout time.Duration) (diag.Message, error) {
	if l.closed {
		return diag.Message{}, diag.ErrClosed
	}
	first, err := l.readByte(timeout)
	if err != nil {
		return diag.Message{}, diag.ErrTimeout
	}
	frame := []byte{first}
	for len(frame) < maxFrame {
		valid := len(frame) >= 5 && checksum(frame[:len(frame)-1]) == frame[len(frame)-1]
		wait := 60 * time.Millisecond
		if valid {
			wait = l.Quiet
		}
		b, err := l.readByte(wait)
		if err != nil {
			if valid {
				break
			}
			return diag.Message{}, fmt.Errorf("iso9141: incomplete or corrupt frame % X", frame)
		}
		if valid && b == 0x48 {
			// A new answer header right after a complete frame.
			l.back = append(l.back, b)
			break
		}
		frame = append(frame, b)
	}
	if len(frame) < 5 || checksum(frame[:len(frame)-1]) != frame[len(frame)-1] {
		return diag.Message{}, fmt.Errorf("iso9141: checksum error in % X", frame)
	}
	l.lastIO = time.Now()
	data := frame[3 : len(frame)-1]
	journal.Rx(l.log, "iso9141", fmt.Sprintf("0x%02X", frame[2]), frame, diag.ServiceName(data[0]))
	return diag.Message{Src: uint32(frame[2]), Dst: 0xF1, Data: append([]byte(nil), data...), Time: l.lastIO}, nil
}

// Info implements diag.Channel.
func (l *Link) Info() diag.ChannelInfo {
	return diag.ChannelInfo{
		Protocol: "ISO 9141-2 (OBD-II, 5-baud init), 10400 baud", ProtoID: "iso9141",
		Adapter: l.line.Name(), Target: 0x33, Tester: 0xF1, Functional: true, Baud: 10400,
		KeyBytes: []byte{l.kb.KB1, l.kb.KB2},
	}
}

// Close marks the link closed; ISO 9141-2 has no stop message, the
// control units time out after P3max (5 s).
func (l *Link) Close() error {
	l.closed = true
	return nil
}

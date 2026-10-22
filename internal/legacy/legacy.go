// Package legacy implements the K-Line "monitor" protocol of early
// Russian engine controllers: Январь-4/4.1 (VAZ), Микас-5.4 and
// Микас-7.1/7.2 (GAZ, UAZ with ZMZ-406/409 engines). It predates
// KWP2000: 9600 baud 8N1, no initialisation, and every message is
//
//	[command][parameters...][checksum] 0x0D
//
// where the checksum makes the byte sum zero, a data byte 0x0D is sent as
// 40 CD and 0x40 as 40 00 (each pair sums to the original value, so the
// receiver can check a message by adding up all bytes before 0x0D).
// Two-byte values are little-endian.
//
// Sources: "Протокол обмена по K-Line с блоком Январь 4.1" and
// "Описание протокола взаимодействия с ЭБУ Микас 5.4 и 7.1" (А. Гуляев,
// 2000).
package legacy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kline"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/timing"
)

// Command codes.
const (
	CmdVersion      = 0x01 // controller availability / version
	CmdReadErrors   = 0x02 // fault list (Микас)
	CmdReadRAM      = 0x21 // read N bytes of internal RAM: addr, n
	CmdReadXRAM     = 0x23 // read N bytes of external RAM: addrL, addrH, n
	CmdReadCode     = 0x25 // read N bytes of program memory: addrL, addrH, n
	CmdProgPassport = 0x51 // 0x51..0x53: program passport, 16 chars each
	CmdDataPassport = 0x54 // 0x54..0x58: calibration passport, 16 chars each
	CmdNumParams    = 0x60 // number of parameters in the exchange table
	CmdReadParams   = 0x61 // read parameters by code
	CmdWriteParam   = 0x62 // write a parameter
)

// Errors.
var (
	ErrChecksum = errors.New("legacy: checksum error")
	ErrEscape   = errors.New("legacy: invalid escape sequence")
	ErrTimeout  = errors.New("legacy: no answer from the controller")
)

// Encode builds a complete message from its body.
func Encode(body []byte) []byte {
	var sum byte
	for _, b := range body {
		sum += b
	}
	raw := append(append([]byte(nil), body...), -sum)
	out := make([]byte, 0, len(raw)+4)
	for _, b := range raw {
		switch b {
		case 0x0D:
			out = append(out, 0x40, 0xCD)
		case 0x40:
			out = append(out, 0x40, 0x00)
		default:
			out = append(out, b)
		}
	}
	return append(out, 0x0D)
}

// Decode checks a received message (without the final 0x0D) and returns
// its body.
func Decode(raw []byte) ([]byte, error) {
	var out []byte
	var sum byte
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		sum += b
		if b == 0x40 {
			if i+1 >= len(raw) {
				return nil, ErrEscape
			}
			i++
			n := raw[i]
			sum += n
			switch n {
			case 0xCD:
				b = 0x0D
			case 0x00:
				b = 0x40
			default:
				return nil, fmt.Errorf("%w: 40 %02X", ErrEscape, n)
			}
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: empty message", ErrChecksum)
	}
	if sum != 0 {
		return nil, fmt.Errorf("%w (sum 0x%02X) in % X", ErrChecksum, sum, raw)
	}
	return out[:len(out)-1], nil
}

// Link talks to a controller over the legacy protocol.
type Link struct {
	line kline.Line
	log  journal.Logger
	// FirstByte is how long to wait for the start of an answer.
	FirstByte time.Duration
	// ByteTimeout is the allowed pause inside an answer.
	ByteTimeout time.Duration
	lastIO      time.Time
}

// Open prepares the line (9600 baud unless baud is given). No bytes are
// exchanged; use Version to check that a controller answers.
func Open(line kline.Line, baud int, log journal.Logger) (*Link, error) {
	if baud == 0 {
		baud = 9600
	}
	if err := line.SetBaud(baud); err != nil {
		return nil, err
	}
	line.Drain()
	return &Link{line: line, log: journal.Or(log), FirstByte: 400 * time.Millisecond, ByteTimeout: 60 * time.Millisecond}, nil
}

// Transact sends a command body and returns the body of the answer.
func (l *Link) Transact(ctx context.Context, body []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Leave a short pause between messages.
	timing.Until(l.lastIO.Add(20 * time.Millisecond))
	l.line.Drain()
	frame := Encode(body)
	journal.Tx(l.log, "legacy", "", frame, commandName(body[0]))
	if err := l.line.Send(frame, 0); err != nil {
		return nil, err
	}
	var raw []byte
	timeout := l.FirstByte
	for {
		b, err := l.line.RecvByte(timeout)
		if err != nil {
			l.lastIO = time.Now()
			if len(raw) == 0 {
				return nil, ErrTimeout
			}
			return nil, fmt.Errorf("legacy: answer interrupted after % X", raw)
		}
		timeout = l.ByteTimeout
		if b == 0x0D {
			resp, err := Decode(raw)
			if err == nil {
				l.lastIO = time.Now()
				journal.Rx(l.log, "legacy", "", append(raw, 0x0D), "")
				return resp, nil
			}
			// 0x0D may have been the checksum itself (sent unescaped by
			// some firmware); look for a second terminator.
			if n, err2 := l.line.RecvByte(l.ByteTimeout); err2 == nil && n == 0x0D {
				if resp, err3 := Decode(append(raw, 0x0D)); err3 == nil {
					l.lastIO = time.Now()
					journal.Rx(l.log, "legacy", "", append(raw, 0x0D, 0x0D), "")
					return resp, nil
				}
			}
			l.lastIO = time.Now()
			journal.Warnf(l.log, "legacy", "%v", err)
			return nil, err
		}
		raw = append(raw, b)
		if len(raw) > 600 {
			return nil, fmt.Errorf("legacy: answer too long, no end marker")
		}
	}
}

// Version sends the availability request. Микас-5.4 answers 0x09,
// Микас-7.1 0x0A; Январь-4 returns its monitor version.
func (l *Link) Version(ctx context.Context) (byte, error) {
	resp, err := l.Transact(ctx, []byte{CmdVersion})
	if err != nil {
		return 0, err
	}
	if len(resp) == 0 {
		return 0, fmt.Errorf("legacy: empty answer to the version request")
	}
	return resp[0], nil
}

// ReadParams reads several parameters at once. sizes gives the length of
// each parameter in bytes (1 or 2); the result holds the raw bytes in
// request order.
func (l *Link) ReadParams(ctx context.Context, codes []byte, sizes []int) ([]byte, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	want := 0
	for _, s := range sizes {
		want += s
	}
	resp, err := l.Transact(ctx, append([]byte{CmdReadParams}, codes...))
	if err != nil {
		return nil, err
	}
	if want > 0 && len(resp) < want {
		return nil, fmt.Errorf("legacy: answer has %d bytes, expected %d", len(resp), want)
	}
	return resp, nil
}

// ReadErrors returns the fault numbers reported by command 0x02. The
// answer is [count] followed by pairs [code][separator 0xE0].
func (l *Link) ReadErrors(ctx context.Context) ([]byte, error) {
	resp, err := l.Transact(ctx, []byte{CmdReadErrors})
	if err != nil {
		return nil, err
	}
	return ParseErrors(resp)
}

// ParseErrors decodes the answer to command 0x02.
func ParseErrors(resp []byte) ([]byte, error) {
	if len(resp) == 0 {
		return nil, fmt.Errorf("legacy: empty fault list")
	}
	n := int(resp[0])
	var codes []byte
	for i := 1; i < len(resp) && len(codes) < n; i++ {
		if resp[i] == 0xE0 {
			continue
		}
		codes = append(codes, resp[i])
	}
	return codes, nil
}

// WriteParam writes one byte parameter; the controller answers 0x00 on
// success.
func (l *Link) WriteParam(ctx context.Context, code, value byte) error {
	resp, err := l.Transact(ctx, []byte{CmdWriteParam, code, value})
	if err != nil {
		return err
	}
	if len(resp) == 0 || resp[0] != 0x00 {
		return fmt.Errorf("legacy: controller rejected writing parameter 0x%02X (answer % X)", code, resp)
	}
	return nil
}

// Passport reads the program (0x51-0x53) and calibration (0x54-0x58)
// identification strings.
func (l *Link) Passport(ctx context.Context) (program, data string, err error) {
	read := func(from, to byte) (string, error) {
		var sb strings.Builder
		for c := from; c <= to; c++ {
			resp, err := l.Transact(ctx, []byte{c})
			if err != nil {
				return strings.TrimSpace(sb.String()), err
			}
			sb.WriteString(CP866(resp))
		}
		return strings.TrimSpace(sb.String()), nil
	}
	if program, err = read(0x51, 0x53); err != nil {
		return program, "", err
	}
	data, err = read(0x54, 0x58)
	return program, data, err
}

// ReadMemory reads n bytes of RAM (space 0), XRAM (1) or program memory (2).
func (l *Link) ReadMemory(ctx context.Context, space int, addr uint16, n byte) ([]byte, error) {
	var req []byte
	switch space {
	case 0:
		req = []byte{CmdReadRAM, byte(addr), n}
	case 1:
		req = []byte{CmdReadXRAM, byte(addr), byte(addr >> 8), n}
	case 2:
		req = []byte{CmdReadCode, byte(addr), byte(addr >> 8), n}
	default:
		return nil, fmt.Errorf("legacy: unknown memory space %d", space)
	}
	resp, err := l.Transact(ctx, req)
	if err != nil {
		return nil, err
	}
	head := len(req) - 1 // the answer repeats the address and count
	if len(resp) < head+int(n) {
		return nil, fmt.Errorf("legacy: short memory answer % X", resp)
	}
	return resp[head : head+int(n)], nil
}

func commandName(c byte) string {
	switch {
	case c == CmdVersion:
		return "version request"
	case c == CmdReadErrors:
		return "read faults"
	case c == CmdReadParams:
		return "read parameters"
	case c == CmdWriteParam:
		return "write parameter"
	case c >= 0x51 && c <= 0x53:
		return "read program passport"
	case c >= 0x54 && c <= 0x58:
		return "read calibration passport"
	case c == CmdReadRAM || c == CmdReadXRAM || c == CmdReadCode:
		return "read memory"
	}
	return fmt.Sprintf("command 0x%02X", c)
}

// CP866 converts DOS Cyrillic text (used in controller passports) to a
// Go string, dropping padding zeros.
func CP866(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		switch {
		case c == 0:
			continue
		case c < 0x80:
			sb.WriteByte(c)
		case c <= 0x9F:
			sb.WriteRune(rune('А' + int(c-0x80)))
		case c <= 0xAF:
			sb.WriteRune(rune('а' + int(c-0xA0)))
		case c >= 0xE0 && c <= 0xEF:
			sb.WriteRune(rune('р' + int(c-0xE0)))
		case c == 0xF0:
			sb.WriteRune('Ё')
		case c == 0xF1:
			sb.WriteRune('ё')
		case c == 0xFC:
			sb.WriteRune('№')
		case c == 0xF8:
			sb.WriteRune('°')
		default:
			sb.WriteRune(' ')
		}
	}
	return sb.String()
}

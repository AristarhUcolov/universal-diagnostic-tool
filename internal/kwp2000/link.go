// Package kwp2000 implements the KWP2000 data link of ISO 14230-2 on a
// K-Line: fast and 5-baud initialisation, the format/target/source/length
// header chosen from the control unit's key bytes, checksums and the
// P1..P4 timing. It is used both for manufacturer diagnostics (Bosch
// M1.5.4, Январь-5/7, Микас-10/11 at address 0x10) and for OBD-II over
// ISO 14230-4 (functional address 0x33).
package kwp2000

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kline"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/timing"
)

// Init selects the initialisation procedure.
type Init int

const (
	InitFast Init = iota // 25 ms wake-up pattern + StartCommunication
	InitSlow             // address at 5 baud, key bytes
	InitNone             // no initialisation (bus already awake)
)

func (i Init) String() string {
	switch i {
	case InitFast:
		return "fast init"
	case InitSlow:
		return "5-baud init"
	}
	return "no init"
}

// Config describes the link.
type Config struct {
	Target     byte // control unit address: 0x10 engine, 0x33 OBD functional
	Tester     byte // tester address, normally 0xF1
	Functional bool // functional addressing (ISO 14230-4 OBD)
	Init       Init
	InitAddr   byte // address sent at 5 baud (default Target)
	Baud       int  // default 10400
	StartComm  bool // send StartCommunication after fast init (default true)

	P3Min       time.Duration // pause before every request (default 55 ms)
	P4          time.Duration // gap between request bytes (default 0)
	ByteTimeout time.Duration // inter-byte timeout while receiving (default 30 ms)
	// LengthByte forces the separate length byte even for short
	// requests. Normally chosen from the key bytes.
	LengthByte bool
	// NoAddress sends headers without target/source bytes.
	NoAddress bool
}

// Errors.
var (
	ErrChecksum = errors.New("kwp2000: checksum error in received frame")
	ErrInit     = errors.New("kwp2000: initialisation failed")
)

// Link is an initialised KWP2000 connection; it implements diag.Channel.
type Link struct {
	line   kline.Line
	cfg    Config
	kb1    byte
	kb2    byte
	addr   bool // header carries target and source
	lenFmt bool // length may go into the format byte
	lenB   bool // separate length byte supported
	lastIO time.Time
	log    journal.Logger
	closed bool
}

// Checksum is the modulo-256 sum used by ISO 9141 and ISO 14230.
func Checksum(b []byte) byte {
	var s byte
	for _, v := range b {
		s += v
	}
	return s
}

// Open initialises the bus and returns the link.
func Open(ctx context.Context, line kline.Line, cfg Config, log journal.Logger) (*Link, error) {
	if cfg.Tester == 0 {
		cfg.Tester = 0xF1
	}
	if cfg.Baud == 0 {
		cfg.Baud = 10400
	}
	if cfg.InitAddr == 0 {
		cfg.InitAddr = cfg.Target
	}
	if cfg.P3Min == 0 {
		cfg.P3Min = 55 * time.Millisecond
	}
	if cfg.ByteTimeout == 0 {
		cfg.ByteTimeout = 30 * time.Millisecond
	}
	l := &Link{line: line, cfg: cfg, log: journal.Or(log), addr: !cfg.NoAddress, lenFmt: !cfg.LengthByte, lenB: true}
	if err := line.SetBaud(cfg.Baud); err != nil {
		return nil, err
	}
	switch cfg.Init {
	case InitFast:
		if err := l.fastInit(ctx); err != nil {
			return nil, err
		}
	case InitSlow:
		r, err := kline.SlowInit(line, cfg.InitAddr, true, l.log)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInit, err)
		}
		l.applyKeyBytes(r.KB1, r.KB2)
		l.lastIO = time.Now()
	default:
		l.lastIO = time.Now().Add(-cfg.P3Min)
	}
	return l, nil
}

func (l *Link) fastInit(ctx context.Context) error {
	// TIdle: the bus must have been quiet before the wake-up pattern.
	timing.Sleep(60 * time.Millisecond)
	if err := l.line.WakeUp(25*time.Millisecond, 25*time.Millisecond); err != nil {
		return fmt.Errorf("%w: %v", ErrInit, err)
	}
	l.lastIO = time.Time{}
	if !l.cfg.StartComm {
		return nil
	}
	req := []byte{0x81}
	frame := l.frame(req)
	journal.Tx(l.log, "kwp", l.addrText(), frame, "StartCommunication")
	if err := l.line.Send(frame, l.cfg.P4); err != nil {
		return fmt.Errorf("%w: %v", ErrInit, err)
	}
	deadline := time.Now().Add(400 * time.Millisecond)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("%w: no answer to StartCommunication from address 0x%02X (check K-Line wiring, ignition and the ECU address)", ErrInit, l.cfg.Target)
		}
		src, _, data, raw, err := l.readFrame(remaining)
		if err != nil {
			if errors.Is(err, ErrChecksum) {
				continue
			}
			return fmt.Errorf("%w: no answer to StartCommunication from address 0x%02X: %v", ErrInit, l.cfg.Target, err)
		}
		journal.Rx(l.log, "kwp", fmt.Sprintf("0x%02X", src), raw, describe(data))
		if len(data) >= 1 && data[0] == 0xC1 {
			kb1, kb2 := byte(0xEF), byte(0x8F)
			if len(data) >= 3 {
				kb1, kb2 = data[1], data[2]
			}
			l.applyKeyBytes(kb1, kb2)
			l.lastIO = time.Now()
			return nil
		}
		if len(data) >= 3 && data[0] == 0x7F && data[1] == 0x81 {
			return fmt.Errorf("%w: control unit refused StartCommunication: %s", ErrInit, diag.NRCName(data[2]))
		}
	}
}

// applyKeyBytes reads the header capabilities from key byte 1:
// bit0 AL0 length in format byte, bit1 AL1 separate length byte,
// bit2 HB0 header without addresses, bit3 HB1 header with addresses.
func (l *Link) applyKeyBytes(kb1, kb2 byte) {
	l.kb1, l.kb2 = kb1, kb2
	al0, al1 := kb1&0x01 != 0, kb1&0x02 != 0
	hb0, hb1 := kb1&0x04 != 0, kb1&0x08 != 0
	switch {
	case l.cfg.NoAddress:
		l.addr = false
	case hb1 || !hb0:
		l.addr = true
	default:
		l.addr = false
	}
	l.lenFmt = (al0 || !al1) && !l.cfg.LengthByte
	l.lenB = al1 || !al0
	journal.Infof(l.log, "kwp", "key bytes %02X %02X: header %s, length %s", kb1, kb2,
		map[bool]string{true: "with addresses", false: "without addresses"}[l.addr],
		map[bool]string{true: "in format byte", false: "in separate byte"}[l.lenFmt])
}

// KeyBytes returns the key bytes received during initialisation.
func (l *Link) KeyBytes() []byte { return []byte{l.kb1, l.kb2} }

func (l *Link) addrText() string {
	if l.cfg.Functional {
		return fmt.Sprintf("0x%02X (functional)", l.cfg.Target)
	}
	return fmt.Sprintf("0x%02X", l.cfg.Target)
}

// frame builds a complete request frame including the checksum.
func (l *Link) frame(data []byte) []byte {
	mode := byte(0x80)
	if l.cfg.Functional {
		mode = 0xC0
	}
	if !l.addr {
		mode = 0x00
	}
	b := make([]byte, 0, len(data)+5)
	n := len(data)
	if n <= 63 && l.lenFmt {
		b = append(b, mode|byte(n))
		if l.addr {
			b = append(b, l.cfg.Target, l.cfg.Tester)
		}
	} else {
		b = append(b, mode)
		if l.addr {
			b = append(b, l.cfg.Target, l.cfg.Tester)
		}
		b = append(b, byte(n))
	}
	b = append(b, data...)
	return append(b, Checksum(b))
}

// readFrame reads one frame from the bus and returns its addresses,
// the data bytes and the raw frame.
func (l *Link) readFrame(timeout time.Duration) (src, dst byte, data, raw []byte, err error) {
	first, err := l.line.RecvByte(timeout)
	if err != nil {
		return 0, 0, nil, nil, diag.ErrTimeout
	}
	raw = append(raw, first)
	next := func() (byte, error) {
		b, err := l.line.RecvByte(l.cfg.ByteTimeout)
		if err != nil {
			return 0, fmt.Errorf("kwp2000: frame interrupted after %d bytes (% X): %w", len(raw), raw, err)
		}
		raw = append(raw, b)
		return b, nil
	}
	mode := first >> 6
	n := int(first & 0x3F)
	if mode != 0 {
		if dst, err = next(); err != nil {
			return
		}
		if src, err = next(); err != nil {
			return
		}
	}
	if n == 0 {
		var lb byte
		if lb, err = next(); err != nil {
			return
		}
		n = int(lb)
	}
	data = make([]byte, 0, n)
	for i := 0; i < n; i++ {
		var b byte
		if b, err = next(); err != nil {
			return
		}
		data = append(data, b)
	}
	var cs byte
	if cs, err = next(); err != nil {
		return
	}
	if Checksum(raw[:len(raw)-1]) != cs {
		journal.Warnf(l.log, "kwp", "checksum error in % X", raw)
		return src, dst, nil, raw, ErrChecksum
	}
	return src, dst, data, raw, nil
}

// Send implements diag.Channel.
func (l *Link) Send(ctx context.Context, data []byte) error {
	if l.closed {
		return diag.ErrClosed
	}
	if len(data) == 0 || len(data) > 255 {
		return fmt.Errorf("kwp2000: request length %d out of range", len(data))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// P3: time between the end of the previous message and a new request.
	if !l.lastIO.IsZero() {
		timing.Until(l.lastIO.Add(l.cfg.P3Min))
	}
	frame := l.frame(data)
	journal.Tx(l.log, "kwp", l.addrText(), frame, describe(data))
	err := l.line.Send(frame, l.cfg.P4)
	l.lastIO = time.Now()
	return err
}

// Recv implements diag.Channel.
func (l *Link) Recv(ctx context.Context, timeout time.Duration) (diag.Message, error) {
	if l.closed {
		return diag.Message{}, diag.ErrClosed
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return diag.Message{}, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return diag.Message{}, diag.ErrTimeout
		}
		src, dst, data, raw, err := l.readFrame(remaining)
		if err != nil {
			if errors.Is(err, ErrChecksum) {
				continue
			}
			if errors.Is(err, diag.ErrTimeout) {
				return diag.Message{}, err
			}
			journal.Warnf(l.log, "kwp", "%v", err)
			continue
		}
		l.lastIO = time.Now()
		if l.addr && dst != l.cfg.Tester && dst != 0 {
			continue // traffic for somebody else
		}
		journal.Rx(l.log, "kwp", fmt.Sprintf("0x%02X", src), raw, describe(data))
		return diag.Message{Src: uint32(src), Dst: uint32(dst), Data: data, Time: l.lastIO}, nil
	}
}

// Info implements diag.Channel.
func (l *Link) Info() diag.ChannelInfo {
	id := "kwp-fast"
	if l.cfg.Init == InitSlow {
		id = "kwp-slow"
	}
	name := fmt.Sprintf("ISO 14230 KWP2000, %s, %d baud", l.cfg.Init, l.cfg.Baud)
	if l.cfg.Functional {
		name = fmt.Sprintf("ISO 14230-4 KWP2000 (OBD), %s, %d baud", l.cfg.Init, l.cfg.Baud)
	}
	return diag.ChannelInfo{
		Protocol: name, ProtoID: id, Adapter: l.line.Name(),
		Target: uint32(l.cfg.Target), Tester: uint32(l.cfg.Tester),
		Functional: l.cfg.Functional, Baud: l.cfg.Baud, KeyBytes: l.KeyBytes(),
	}
}

// Close sends StopCommunication and marks the link closed. The K-Line
// itself stays open (it belongs to the adapter).
func (l *Link) Close() error {
	if l.closed {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := l.Send(ctx, []byte{0x82}); err == nil {
		_, _ = l.Recv(ctx, 200*time.Millisecond)
	}
	l.closed = true
	return nil
}

func describe(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if data[0] == 0x7F && len(data) >= 3 {
		return fmt.Sprintf("negative response to %s: %s", diag.ServiceName(data[1]), diag.NRCName(data[2]))
	}
	return diag.ServiceName(data[0])
}

// SetP3Min changes the pause before requests (control unit definitions
// may require more than the ISO default).
func (l *Link) SetP3Min(d time.Duration) {
	if d > 0 {
		l.cfg.P3Min = d
	}
}

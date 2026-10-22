// Package slcan drives CAN adapters that speak the Lawicel ASCII
// protocol over a serial port: Lawicel CANUSB, CANable and CANtact with
// slcan firmware and most cheap "USB-CAN" sticks. Frames are text lines
// such as "t7E0802010C0000000000\r".
package slcan

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/serial"
)

// Port is the serial connection to the adapter.
type Port interface {
	Write(p []byte) (int, error)
	Read(p []byte) (int, error)
	SetReadTimeout(d time.Duration) error
	Close() error
	Name() string
}

// Bitrate codes of the "Sn" command.
var bitrateCodes = map[int]byte{
	10000: '0', 20000: '1', 50000: '2', 100000: '3', 125000: '4',
	250000: '5', 500000: '6', 800000: '7', 1000000: '8',
}

// Bus is an open slcan channel.
type Bus struct {
	port    Port
	bitrate int
	log     journal.Logger
	wmu     sync.Mutex
	rx      chan can.Frame
	done    chan struct{}
	err     error
	errMu   sync.Mutex
	Version string
	once    sync.Once
}

// Open configures the bit rate and opens the CAN channel. listenOnly
// opens it without acknowledging frames (safe sniffing).
func Open(port Port, bitrate int, listenOnly bool, log journal.Logger) (*Bus, error) {
	code, ok := bitrateCodes[bitrate]
	if !ok {
		return nil, fmt.Errorf("slcan: unsupported bit rate %d (use 10k..1M standard rates)", bitrate)
	}
	b := &Bus{port: port, bitrate: bitrate, log: journal.Or(log), rx: make(chan can.Frame, 4096), done: make(chan struct{})}
	// Clear pending input and close a channel left open.
	_, _ = port.Write([]byte("\r\r\r"))
	b.drain(300 * time.Millisecond)
	_, _ = b.command("C", 300*time.Millisecond)
	b.drain(100 * time.Millisecond)
	if v, err := b.command("V", 500*time.Millisecond); err == nil {
		b.Version = strings.TrimSpace(v)
	}
	if _, err := b.command("S"+string(code), 500*time.Millisecond); err != nil {
		return nil, fmt.Errorf("slcan: adapter rejected bit rate %d: %w", bitrate, err)
	}
	open := "O"
	if listenOnly {
		open = "L"
	}
	if _, err := b.command(open, 500*time.Millisecond); err != nil {
		return nil, fmt.Errorf("slcan: cannot open the CAN channel: %w", err)
	}
	journal.Infof(b.log, "slcan", "%s open at %d bit/s (version %s)", port.Name(), bitrate, b.Version)
	go b.reader()
	return b, nil
}

// drain discards input until nothing arrives for quiet.
func (b *Bus) drain(quiet time.Duration) {
	buf := make([]byte, 256)
	for {
		_ = b.port.SetReadTimeout(quiet)
		n, err := b.port.Read(buf)
		if n == 0 || err != nil {
			return
		}
	}
}

// readLine reads one CR-terminated line before the reader goroutine
// starts (used for command answers during setup).
func (b *Bus) readLine(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var line []byte
	one := make([]byte, 1)
	for time.Now().Before(deadline) {
		_ = b.port.SetReadTimeout(time.Until(deadline))
		n, err := b.port.Read(one)
		if n == 1 {
			switch one[0] {
			case '\r':
				return string(line), nil
			case 0x07:
				return string(line), errors.New("slcan: adapter answered with error (BEL)")
			default:
				line = append(line, one[0])
			}
			continue
		}
		if err != nil && !errors.Is(err, serial.ErrTimeout) && !strings.Contains(err.Error(), "timeout") {
			return string(line), err
		}
	}
	return string(line), errors.New("slcan: no answer")
}

func (b *Bus) command(c string, timeout time.Duration) (string, error) {
	if _, err := b.port.Write([]byte(c + "\r")); err != nil {
		return "", err
	}
	return b.readLine(timeout)
}

func (b *Bus) reader() {
	defer close(b.done)
	var line []byte
	buf := make([]byte, 1024)
	for {
		_ = b.port.SetReadTimeout(200 * time.Millisecond)
		n, err := b.port.Read(buf)
		for _, c := range buf[:n] {
			switch c {
			case '\r':
				if f, ok := ParseFrame(string(line)); ok {
					f.Time = time.Now()
					select {
					case b.rx <- f:
					default:
					}
				}
				line = line[:0]
			case 0x07:
				line = line[:0]
			default:
				line = append(line, c)
			}
		}
		if err != nil && !errors.Is(err, serial.ErrTimeout) && !strings.Contains(err.Error(), "timeout") {
			b.errMu.Lock()
			b.err = err
			b.errMu.Unlock()
			return
		}
	}
}

// ParseFrame parses one received slcan line (t, T, r, R, d, D, b, B).
func ParseFrame(s string) (can.Frame, bool) {
	if len(s) < 5 {
		return can.Frame{}, false
	}
	var f can.Frame
	idLen := 3
	switch s[0] {
	case 't':
	case 'T':
		idLen = 8
	case 'r':
		f.RTR = true
	case 'R':
		f.RTR, idLen = true, 8
	case 'd':
		f.FD = true
	case 'D':
		f.FD, idLen = true, 8
	case 'b':
		f.FD, f.BRS = true, true
	case 'B':
		f.FD, f.BRS, idLen = true, true, 8
	default:
		return can.Frame{}, false
	}
	if len(s) < 1+idLen+1 {
		return can.Frame{}, false
	}
	id, err := strconv.ParseUint(s[1:1+idLen], 16, 32)
	if err != nil {
		return can.Frame{}, false
	}
	f.ID, f.Ext = uint32(id), idLen == 8
	dlc, err := strconv.ParseUint(s[1+idLen:2+idLen], 16, 8)
	if err != nil {
		return can.Frame{}, false
	}
	n := int(dlc)
	if f.FD {
		n = can.DLCLen(uint8(dlc))
	} else if n > 8 {
		n = 8
	}
	f.Len = uint8(n)
	if f.RTR {
		return f, true
	}
	hex := s[2+idLen:]
	if len(hex) < 2*n {
		return can.Frame{}, false
	}
	data, err := hexutil.Parse(hex[:2*n])
	if err != nil {
		return can.Frame{}, false
	}
	copy(f.Data[:], data)
	return f, true
}

// FormatFrame renders a frame as an slcan transmit command (without CR).
func FormatFrame(f can.Frame) string {
	cmd := byte('t')
	switch {
	case f.FD && f.BRS:
		cmd = 'b'
	case f.FD:
		cmd = 'd'
	case f.RTR:
		cmd = 'r'
	}
	if f.Ext {
		cmd -= 'a' - 'A'
	}
	var sb strings.Builder
	sb.WriteByte(cmd)
	if f.Ext {
		fmt.Fprintf(&sb, "%08X", f.ID)
	} else {
		fmt.Fprintf(&sb, "%03X", f.ID&0x7FF)
	}
	dlc := f.Len
	if f.FD {
		dlc = can.LenDLC(int(f.Len))
	}
	fmt.Fprintf(&sb, "%X", dlc)
	if !f.RTR {
		sb.WriteString(hexutil.Compact(f.Payload()))
	}
	return sb.String()
}

// Send implements can.Bus.
func (b *Bus) Send(ctx context.Context, f can.Frame) error {
	b.wmu.Lock()
	defer b.wmu.Unlock()
	_, err := b.port.Write([]byte(FormatFrame(f) + "\r"))
	return err
}

// Recv implements can.Bus.
func (b *Bus) Recv(ctx context.Context, timeout time.Duration) (can.Frame, error) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case f := <-b.rx:
		return f, nil
	case <-t.C:
		return can.Frame{}, can.ErrTimeout
	case <-ctx.Done():
		return can.Frame{}, ctx.Err()
	case <-b.done:
		b.errMu.Lock()
		defer b.errMu.Unlock()
		if b.err != nil {
			return can.Frame{}, b.err
		}
		return can.Frame{}, can.ErrClosed
	}
}

// Info implements can.Bus.
func (b *Bus) Info() can.BusInfo {
	return can.BusInfo{Name: "slcan " + b.port.Name(), Bitrate: b.bitrate}
}

// Close closes the CAN channel and the port.
func (b *Bus) Close() error {
	var err error
	b.once.Do(func() {
		b.wmu.Lock()
		_, _ = b.port.Write([]byte("C\r"))
		b.wmu.Unlock()
		err = b.port.Close()
	})
	return err
}

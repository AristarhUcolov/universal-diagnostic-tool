//go:build windows

package pcan

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
)

var (
	dll          = windows.NewLazyDLL("PCANBasic.dll")
	procInit     = dll.NewProc("CAN_Initialize")
	procInitFD   = dll.NewProc("CAN_InitializeFD")
	procUninit   = dll.NewProc("CAN_Uninitialize")
	procRead     = dll.NewProc("CAN_Read")
	procReadFD   = dll.NewProc("CAN_ReadFD")
	procWrite    = dll.NewProc("CAN_Write")
	procWriteFD  = dll.NewProc("CAN_WriteFD")
	procErrorTxt = dll.NewProc("CAN_GetErrorText")
)

const (
	msgStandard  = 0x00
	msgRTR       = 0x01
	msgExtended  = 0x02
	msgFD        = 0x04
	msgBRS       = 0x08
	msgStatus    = 0x80
	errQRcvEmpty = 0x00020
)

type tpcanMsg struct {
	ID      uint32
	MsgType uint8
	Len     uint8
	Data    [8]uint8
}

// tpcanMsgFD is TPCANMsgFD: DLC is the DLC code (0–15), not the length.
type tpcanMsgFD struct {
	ID      uint32
	MsgType uint8
	DLC     uint8
	Data    [64]uint8
}

type tpcanTimestamp struct {
	Millis         uint32
	MillisOverflow uint16
	Micros         uint16
}

// Bus is an open PCAN channel (can.Bus).
type Bus struct {
	ch      uint16
	name    string
	bitrate int
	fd      bool // opened with CAN_InitializeFD: frames go through the FD calls
	mu      sync.Mutex
	closed  bool
}

func errorText(st uintptr) string {
	buf := make([]byte, 256)
	if procErrorTxt.Find() == nil {
		procErrorTxt.Call(st, 0x09, uintptr(unsafe.Pointer(&buf[0])))
	}
	s := string(buf)
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return fmt.Sprintf("0x%X %s", st, s)
}

// Open initialises a channel ("PCAN_USBBUS1") at a bit rate.
func Open(channel string, bitrate int) (can.Bus, error) {
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("pcan: PCANBasic.dll not found (install the PEAK driver): %w", err)
	}
	h, ok := channelHandle(channel)
	if !ok {
		return nil, fmt.Errorf("pcan: unknown channel %q", channel)
	}
	btr, ok := btr0btr1[bitrate]
	if !ok {
		return nil, fmt.Errorf("pcan: unsupported bit rate %d", bitrate)
	}
	st, _, _ := procInit.Call(uintptr(h), uintptr(btr), 0, 0, 0)
	if st != 0 {
		return nil, fmt.Errorf("pcan: CAN_Initialize: %s", errorText(st))
	}
	return &Bus{ch: h, name: channel, bitrate: bitrate}, nil
}

// OpenFD initialises a channel of an FD-capable adapter (PCAN-USB FD,
// PCAN-USB Pro FD) for CAN FD: nominal and data phase bit rates.
func OpenFD(channel string, nominal, data int) (can.Bus, error) {
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("pcan: PCANBasic.dll not found (install the PEAK driver): %w", err)
	}
	if procInitFD.Find() != nil {
		return nil, fmt.Errorf("pcan: this PCAN-Basic version has no CAN FD; update the PEAK driver")
	}
	h, ok := channelHandle(channel)
	if !ok {
		return nil, fmt.Errorf("pcan: unknown channel %q", channel)
	}
	bt, err := FDBitrate(nominal, data)
	if err != nil {
		return nil, err
	}
	cs, err := windows.BytePtrFromString(bt)
	if err != nil {
		return nil, err
	}
	st, _, _ := procInitFD.Call(uintptr(h), uintptr(unsafe.Pointer(cs)))
	runtime.KeepAlive(cs)
	if st != 0 {
		return nil, fmt.Errorf("pcan: CAN_InitializeFD (%s): %s — is it an FD adapter?", bt, errorText(st))
	}
	return &Bus{ch: h, name: channel, bitrate: nominal, fd: true}, nil
}

func (b *Bus) Send(ctx context.Context, f can.Frame) error {
	if b.fd {
		return b.sendFD(f)
	}
	m := tpcanMsg{ID: f.ID, Len: f.Len}
	if f.Ext {
		m.MsgType |= msgExtended
	}
	if f.RTR {
		m.MsgType |= msgRTR
	}
	copy(m.Data[:], f.Payload())
	b.mu.Lock()
	st, _, _ := procWrite.Call(uintptr(b.ch), uintptr(unsafe.Pointer(&m)))
	b.mu.Unlock()
	runtime.KeepAlive(&m)
	if st != 0 {
		return fmt.Errorf("pcan: CAN_Write: %s", errorText(st))
	}
	return nil
}

func (b *Bus) sendFD(f can.Frame) error {
	m := tpcanMsgFD{ID: f.ID, DLC: can.LenDLC(int(f.Len))}
	if f.Ext {
		m.MsgType |= msgExtended
	}
	if f.RTR {
		m.MsgType |= msgRTR
	}
	if f.FD {
		m.MsgType |= msgFD
		if f.BRS {
			m.MsgType |= msgBRS
		}
	}
	copy(m.Data[:], f.Payload())
	b.mu.Lock()
	st, _, _ := procWriteFD.Call(uintptr(b.ch), uintptr(unsafe.Pointer(&m)))
	b.mu.Unlock()
	runtime.KeepAlive(&m)
	if st != 0 {
		return fmt.Errorf("pcan: CAN_WriteFD: %s", errorText(st))
	}
	return nil
}

func (b *Bus) Recv(ctx context.Context, timeout time.Duration) (can.Frame, error) {
	deadline := time.Now().Add(timeout)
	for {
		if b.closed {
			return can.Frame{}, can.ErrClosed
		}
		var f can.Frame
		var got bool
		var st uintptr
		if b.fd {
			var m tpcanMsgFD
			var ts uint64
			b.mu.Lock()
			st, _, _ = procReadFD.Call(uintptr(b.ch), uintptr(unsafe.Pointer(&m)), uintptr(unsafe.Pointer(&ts)))
			b.mu.Unlock()
			if st == 0 && m.MsgType&msgStatus == 0 {
				n := can.DLCLen(m.DLC)
				f = can.NewFrame(m.ID, m.MsgType&msgExtended != 0, m.Data[:n])
				f.FD = m.MsgType&msgFD != 0
				f.BRS = m.MsgType&msgBRS != 0
				f.RTR = m.MsgType&msgRTR != 0
				got = true
			}
		} else {
			var m tpcanMsg
			var ts tpcanTimestamp
			b.mu.Lock()
			st, _, _ = procRead.Call(uintptr(b.ch), uintptr(unsafe.Pointer(&m)), uintptr(unsafe.Pointer(&ts)))
			b.mu.Unlock()
			if st == 0 && m.MsgType&msgStatus == 0 {
				f = can.NewFrame(m.ID, m.MsgType&msgExtended != 0, m.Data[:m.Len])
				f.RTR = m.MsgType&msgRTR != 0
				got = true
			}
		}
		if got {
			f.Time = time.Now()
			return f, nil
		}
		if st != 0 && st&errQRcvEmpty == 0 {
			return can.Frame{}, fmt.Errorf("pcan: CAN_Read: %s", errorText(st))
		}
		if time.Now().After(deadline) {
			return can.Frame{}, can.ErrTimeout
		}
		select {
		case <-ctx.Done():
			return can.Frame{}, ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
}

func (b *Bus) Info() can.BusInfo {
	return can.BusInfo{Name: "PEAK " + b.name, Bitrate: b.bitrate, FD: b.fd}
}

func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		procUninit.Call(uintptr(b.ch))
	}
	return nil
}

// Available reports whether the PCAN-Basic driver is installed.
func Available() bool { return dll.Load() == nil }

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
	procUninit   = dll.NewProc("CAN_Uninitialize")
	procRead     = dll.NewProc("CAN_Read")
	procWrite    = dll.NewProc("CAN_Write")
	procErrorTxt = dll.NewProc("CAN_GetErrorText")
)

const (
	msgStandard  = 0x00
	msgRTR       = 0x01
	msgExtended  = 0x02
	msgStatus    = 0x80
	errQRcvEmpty = 0x00020
)

type tpcanMsg struct {
	ID      uint32
	MsgType uint8
	Len     uint8
	Data    [8]uint8
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

func (b *Bus) Send(ctx context.Context, f can.Frame) error {
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

func (b *Bus) Recv(ctx context.Context, timeout time.Duration) (can.Frame, error) {
	deadline := time.Now().Add(timeout)
	for {
		if b.closed {
			return can.Frame{}, can.ErrClosed
		}
		var m tpcanMsg
		var ts tpcanTimestamp
		b.mu.Lock()
		st, _, _ := procRead.Call(uintptr(b.ch), uintptr(unsafe.Pointer(&m)), uintptr(unsafe.Pointer(&ts)))
		b.mu.Unlock()
		if st == 0 && m.MsgType&msgStatus == 0 {
			f := can.NewFrame(m.ID, m.MsgType&msgExtended != 0, m.Data[:m.Len])
			f.RTR = m.MsgType&msgRTR != 0
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

func (b *Bus) Info() can.BusInfo { return can.BusInfo{Name: "PEAK " + b.name, Bitrate: b.bitrate} }

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

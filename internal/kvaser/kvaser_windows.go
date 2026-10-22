//go:build windows

package kvaser

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
)

var (
	dll           = windows.NewLazyDLL("canlib32.dll")
	procInitLib   = dll.NewProc("canInitializeLibrary")
	procOpen      = dll.NewProc("canOpenChannel")
	procBusParams = dll.NewProc("canSetBusParams")
	procBusOn     = dll.NewProc("canBusOn")
	procBusOff    = dll.NewProc("canBusOff")
	procClose     = dll.NewProc("canClose")
	procWrite     = dll.NewProc("canWrite")
	procReadWait  = dll.NewProc("canReadWait")
	procNumChan   = dll.NewProc("canGetNumberOfChannels")
	initOnce      sync.Once
)

const (
	msgRTR      = 0x0001
	msgStd      = 0x0002
	msgExt      = 0x0004
	msgErrFrame = 0x0020
	errNoMsg    = -2
)

func status(r uintptr) int32 { return int32(uint32(r)) }

// Bus is an open Kvaser channel (can.Bus).
type Bus struct {
	h       int32
	channel int
	bitrate int
	mu      sync.Mutex
}

// Channels returns the number of CANlib channels.
func Channels() int {
	if dll.Load() != nil {
		return 0
	}
	initOnce.Do(func() { procInitLib.Call() })
	var n int32
	procNumChan.Call(uintptr(unsafe.Pointer(&n)))
	return int(n)
}

// Open opens channel n at a bit rate.
func Open(channel, bitrate int) (can.Bus, error) {
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("kvaser: canlib32.dll not found (install Kvaser drivers): %w", err)
	}
	initOnce.Do(func() { procInitLib.Call() })
	freq, ok := bitrates[bitrate]
	if !ok {
		return nil, fmt.Errorf("kvaser: unsupported bit rate %d", bitrate)
	}
	r, _, _ := procOpen.Call(uintptr(channel), 0x0020) // canOPEN_ACCEPT_VIRTUAL
	h := status(r)
	if h < 0 {
		return nil, fmt.Errorf("kvaser: canOpenChannel(%d) failed (%d)", channel, h)
	}
	if r, _, _ := procBusParams.Call(uintptr(h), uintptr(int64(freq)), 0, 0, 0, 0, 0); status(r) < 0 {
		procClose.Call(uintptr(h))
		return nil, fmt.Errorf("kvaser: canSetBusParams failed (%d)", status(r))
	}
	if r, _, _ := procBusOn.Call(uintptr(h)); status(r) < 0 {
		procClose.Call(uintptr(h))
		return nil, fmt.Errorf("kvaser: canBusOn failed (%d)", status(r))
	}
	return &Bus{h: h, channel: channel, bitrate: bitrate}, nil
}

func (b *Bus) Send(ctx context.Context, f can.Frame) error {
	flags := uintptr(msgStd)
	if f.Ext {
		flags = msgExt
	}
	if f.RTR {
		flags |= msgRTR
	}
	data := f.Payload()
	var p uintptr
	if len(data) > 0 {
		p = uintptr(unsafe.Pointer(&data[0]))
	}
	b.mu.Lock()
	r, _, _ := procWrite.Call(uintptr(b.h), uintptr(f.ID), p, uintptr(len(data)), flags)
	b.mu.Unlock()
	runtime.KeepAlive(data)
	if status(r) < 0 {
		return fmt.Errorf("kvaser: canWrite failed (%d)", status(r))
	}
	return nil
}

func (b *Bus) Recv(ctx context.Context, timeout time.Duration) (can.Frame, error) {
	var id int32
	var dlc, flags uint32
	var t uint32
	var data [8]byte
	b.mu.Lock()
	r, _, _ := procReadWait.Call(uintptr(b.h), uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(&dlc)), uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&t)), uintptr(timeout/time.Millisecond))
	b.mu.Unlock()
	if s := status(r); s == errNoMsg {
		return can.Frame{}, can.ErrTimeout
	} else if s < 0 {
		return can.Frame{}, fmt.Errorf("kvaser: canReadWait failed (%d)", s)
	}
	if flags&msgErrFrame != 0 {
		return can.Frame{}, can.ErrTimeout
	}
	if dlc > 8 {
		dlc = 8
	}
	f := can.NewFrame(uint32(id), flags&msgExt != 0, data[:dlc])
	f.RTR = flags&msgRTR != 0
	f.Time = time.Now()
	return f, nil
}

func (b *Bus) Info() can.BusInfo {
	return can.BusInfo{Name: fmt.Sprintf("Kvaser channel %d", b.channel), Bitrate: b.bitrate}
}

func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	procBusOff.Call(uintptr(b.h))
	procClose.Call(uintptr(b.h))
	return nil
}

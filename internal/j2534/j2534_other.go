//go:build !windows

package j2534

import (
	"context"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
)

// ListDevices returns nothing outside Windows.
func ListDevices() ([]Device, error) { return nil, nil }

// API is unavailable outside Windows.
type API struct{}

// Open always fails outside Windows.
func Open(d Device, log journal.Logger) (*API, error) { return nil, ErrUnsupported }

func (a *API) Name() string                     { return "" }
func (a *API) Voltage() (float64, error)        { return 0, ErrUnsupported }
func (a *API) Close() error                     { return nil }
func (a *API) OpenCAN(int) (*CANBus, error)     { return nil, ErrUnsupported }
func (a *API) OpenJ1850(uint32) (*KLine, error) { return nil, ErrUnsupported }
func (a *API) OpenKLine(uint32, int, []byte, bool, byte) (*KLine, error) {
	return nil, ErrUnsupported
}
func (a *API) OpenISO15765(int, bool, uint32, []uint32, func(uint32) uint32, bool) (*ISO15765, error) {
	return nil, ErrUnsupported
}

// ISO15765 is unavailable outside Windows.
type ISO15765 struct{ diag.Channel }

// KLine is unavailable outside Windows.
type KLine struct{ diag.Channel }

// CANBus is unavailable outside Windows.
type CANBus struct{}

func (b *CANBus) Send(context.Context, can.Frame) error { return ErrUnsupported }
func (b *CANBus) Recv(context.Context, time.Duration) (can.Frame, error) {
	return can.Frame{}, ErrUnsupported
}
func (b *CANBus) Info() can.BusInfo { return can.BusInfo{} }
func (b *CANBus) Close() error      { return nil }

//go:build linux

package ble

import (
	"sync"

	"tinygo.org/x/bluetooth"
)

var (
	enableMu sync.Mutex
	enabled  bool
)

// enable connects to BlueZ, again on the next call when it failed (the
// service may be started later).
func enable(a *bluetooth.Adapter) error {
	enableMu.Lock()
	defer enableMu.Unlock()
	if enabled {
		return nil
	}
	if err := a.Enable(); err != nil {
		return err
	}
	enabled = true
	return nil
}

// byProperties is not available through BlueZ in this library: only the
// known serial services are used.
func byProperties(chars []bluetooth.DeviceCharacteristic) (notify, write bluetooth.DeviceCharacteristic, ok bool) {
	return notify, write, false
}

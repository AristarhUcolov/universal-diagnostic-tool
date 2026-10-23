//go:build windows

package ble

import (
	"runtime"
	"sync"

	"tinygo.org/x/bluetooth"
)

var (
	mtaOnce sync.Once
	mtaErr  error
)

// enable joins the Windows Runtime's multithreaded apartment on a thread
// of its own that stays alive (and the apartment with it): the window's
// thread belongs to WebView2's single-threaded apartment.
func enable(a *bluetooth.Adapter) error {
	mtaOnce.Do(func() {
		ch := make(chan error)
		go func() {
			runtime.LockOSThread()
			ch <- a.Enable()
			select {}
		}()
		mtaErr = <-ch
	})
	return mtaErr
}

// WinRT characteristic property flags.
const (
	propWriteWithoutResponse = 0x04
	propWrite                = 0x08
	propNotify               = 0x10
)

// byProperties picks a notifying and a writable characteristic.
func byProperties(chars []bluetooth.DeviceCharacteristic) (notify, write bluetooth.DeviceCharacteristic, ok bool) {
	n, w := -1, -1
	for i, c := range chars {
		p := c.Properties()
		if n < 0 && p&propNotify != 0 {
			n = i
		}
		if w < 0 && p&(propWrite|propWriteWithoutResponse) != 0 {
			w = i
		}
	}
	if n < 0 || w < 0 {
		return notify, write, false
	}
	return chars[n], chars[w], true
}

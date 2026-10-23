// Package ble connects to ELM327-type adapters over Bluetooth Low Energy
// — Vgate iCar Pro BLE, Veepeak OBDCheck BLE, vLinker BLE, OBDLink CX,
// KONNWEI and other "BLE 4.0" adapters — which, unlike Bluetooth Classic
// ones, never appear as a serial port. The adapter's serial service (a
// characteristic that notifies the answers and one that takes the
// commands) is presented as a byte stream for the ELM327 driver.
//
// Windows 10/11 and Linux (BlueZ) are supported; macOS builds have no
// Bluetooth LE, because its system library needs cgo.
package ble

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/serial"
)

// Errors.
var (
	ErrUnsupported = errors.New("ble: Bluetooth LE is not available in this build of the program (macOS)")
	ErrOff         = errors.New("ble: Bluetooth is off or the computer has none: switch it on in the system settings")
	ErrNotFound    = errors.New("ble: the adapter was not found: is it plugged in and close, and not connected to a phone?")
	ErrNoSerial    = errors.New("ble: the device has no serial service of an OBD adapter")
	ErrClosed      = errors.New("ble: connection closed")
)

// Device is an adapter found by a scan.
type Device struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	RSSI    int    `json:"rssi"`
	// Likely marks names of OBD adapters, which are listed first.
	Likely bool `json:"likely"`
}

// likely tells whether a name looks like an OBD adapter.
func likely(name string) bool {
	n := strings.ToLower(name)
	for _, k := range []string{"obd", "elm", "vgate", "vlink", "v-link", "icar", "veepeak", "konnwei", "viecar", "obdlink", "carista", "lelink", "scan"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

// serialPair is a service with the characteristics of an adapter's
// serial line: answers are notified on notify, commands go to write
// (often the same characteristic).
type serialPair struct {
	service, notify, write string
}

// knownPairs are the serial services used by ELM327 BLE adapters.
var knownPairs = []serialPair{
	{"0000fff0-0000-1000-8000-00805f9b34fb", "0000fff1-0000-1000-8000-00805f9b34fb", "0000fff2-0000-1000-8000-00805f9b34fb"},
	{"0000ffe0-0000-1000-8000-00805f9b34fb", "0000ffe1-0000-1000-8000-00805f9b34fb", "0000ffe1-0000-1000-8000-00805f9b34fb"},
	{"000018f0-0000-1000-8000-00805f9b34fb", "00002af0-0000-1000-8000-00805f9b34fb", "00002af1-0000-1000-8000-00805f9b34fb"},
	{"e7810a71-73ae-499d-8c15-faa9aef0c3f2", "bef8d6c9-9c21-4c9e-b632-bd58c1009f9f", "bef8d6c9-9c21-4c9e-b632-bd58c1009f9f"},
}

// Port is a byte stream over the adapter's serial service.
type Port struct {
	name    string
	mu      sync.Mutex
	cond    *sync.Cond
	buf     []byte
	closed  bool
	timeout time.Duration
	write   func([]byte) error
	close   func() error
}

func newPort(name string, write func([]byte) error, close func() error) *Port {
	p := &Port{name: name, timeout: time.Second, write: write, close: close}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// push receives a notification from the adapter.
func (p *Port) push(b []byte) {
	p.mu.Lock()
	p.buf = append(p.buf, b...)
	p.mu.Unlock()
	p.cond.Broadcast()
}

// Write sends bytes in pieces of 20, the payload of the smallest BLE
// packet every adapter takes.
func (p *Port) Write(b []byte) (int, error) {
	for off := 0; off < len(b); off += 20 {
		end := off + 20
		if end > len(b) {
			end = len(b)
		}
		if err := p.write(b[off:end]); err != nil {
			return off, err
		}
	}
	return len(b), nil
}

// Read waits up to the read timeout for notified bytes.
func (p *Port) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	deadline := time.Now().Add(p.timeout)
	stop := time.AfterFunc(p.timeout, p.cond.Broadcast)
	defer stop.Stop()
	for len(p.buf) == 0 {
		if p.closed {
			return 0, ErrClosed
		}
		if !time.Now().Before(deadline) {
			return 0, serial.ErrTimeout
		}
		p.cond.Wait()
	}
	n := copy(b, p.buf)
	p.buf = p.buf[n:]
	return n, nil
}

func (p *Port) SetReadTimeout(d time.Duration) error {
	p.mu.Lock()
	p.timeout = d
	p.mu.Unlock()
	return nil
}

func (p *Port) Name() string { return p.name }

func (p *Port) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()
	p.cond.Broadcast()
	if p.close != nil {
		return p.close()
	}
	return nil
}

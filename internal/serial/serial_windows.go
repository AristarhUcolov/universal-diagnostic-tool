//go:build windows

package serial

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DCB flag bits and comm constants (winbase.h).
const (
	dcbBinary    = 0x00000001
	dcbParity    = 0x00000002
	dcbDTRMask   = 0x00000030
	dcbDTREnable = 0x00000010
	dcbRTSMask   = 0x00003000
	dcbRTSEnable = 0x00001000
	// Flow control and error handling bits that must stay off.
	dcbOutxCTS   = 0x00000004
	dcbOutxDSR   = 0x00000008
	dcbDSRSense  = 0x00000040
	dcbOutX      = 0x00000100
	dcbInX       = 0x00000200
	dcbNull      = 0x00000800
	dcbAbortOnEr = 0x00004000

	escSetRTS = 3
	escClrRTS = 4
	escSetDTR = 5
	escClrDTR = 6

	purgeTxClear = 0x0004
	purgeRxClear = 0x0008

	maxDWORD = 0xFFFFFFFF
)

type winPort struct {
	name string
	h    windows.Handle

	rmu sync.Mutex // serialises reads
	wmu sync.Mutex // serialises writes
	cmu sync.Mutex // serialises configuration changes

	rov windows.Overlapped
	wov windows.Overlapped

	tmu     sync.Mutex
	timeout time.Duration // requested read timeout
	applied time.Duration // timeout currently programmed into the driver

	closed atomic.Bool
	cfg    Config
}

func openPort(name string, cfg Config) (Port, error) {
	path := name
	if !strings.HasPrefix(path, `\\.\`) {
		path = `\\.\` + name
	}
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p16, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return nil, fmt.Errorf("serial: %s is busy (another program is using it): %w", name, err)
		}
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil, fmt.Errorf("serial: %s does not exist (adapter unplugged or driver missing): %w", name, err)
		}
		return nil, fmt.Errorf("serial: open %s: %w", name, err)
	}
	p := &winPort{name: name, h: h, cfg: cfg, timeout: 100 * time.Millisecond, applied: -1}
	if p.rov.HEvent, err = windows.CreateEvent(nil, 1, 0, nil); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	if p.wov.HEvent, err = windows.CreateEvent(nil, 1, 0, nil); err != nil {
		windows.CloseHandle(p.rov.HEvent)
		windows.CloseHandle(h)
		return nil, err
	}
	_ = windows.SetupComm(h, 4096, 4096)
	if err := p.setState(func(d *windows.DCB) { p.fillDCB(d) }); err != nil {
		p.Close()
		return nil, err
	}
	if err := p.applyTimeout(p.timeout); err != nil {
		p.Close()
		return nil, err
	}
	_ = windows.PurgeComm(h, purgeRxClear|purgeTxClear)
	return p, nil
}

func (p *winPort) fillDCB(d *windows.DCB) {
	d.BaudRate = uint32(p.cfg.Baud)
	flags := d.Flags
	flags &^= dcbParity | dcbDTRMask | dcbRTSMask | dcbOutxCTS | dcbOutxDSR | dcbDSRSense |
		dcbOutX | dcbInX | dcbNull | dcbAbortOnEr
	flags |= dcbBinary
	if p.cfg.Parity != ParityNone {
		flags |= dcbParity
	}
	if p.cfg.DTR {
		flags |= dcbDTREnable
	}
	if p.cfg.RTS {
		flags |= dcbRTSEnable
	}
	d.Flags = flags
	d.ByteSize = uint8(p.cfg.DataBits)
	switch p.cfg.Parity {
	case ParityOdd:
		d.Parity = 1
	case ParityEven:
		d.Parity = 2
	default:
		d.Parity = 0
	}
	if p.cfg.StopBits == 2 {
		d.StopBits = 2
	} else {
		d.StopBits = 0
	}
}

func (p *winPort) setState(modify func(*windows.DCB)) error {
	var d windows.DCB
	d.DCBlength = uint32(unsafe.Sizeof(d))
	if err := windows.GetCommState(p.h, &d); err != nil {
		return fmt.Errorf("serial: read settings of %s: %w", p.name, err)
	}
	modify(&d)
	if err := windows.SetCommState(p.h, &d); err != nil {
		return fmt.Errorf("serial: %s rejected settings (%d baud): %w", p.name, d.BaudRate, err)
	}
	return nil
}

func (p *winPort) applyTimeout(d time.Duration) error {
	var t windows.CommTimeouts
	if d <= 0 {
		// Return immediately with whatever is buffered.
		t.ReadIntervalTimeout = maxDWORD
	} else {
		ms := uint64(d / time.Millisecond)
		if ms == 0 {
			ms = 1
		}
		if ms >= maxDWORD {
			ms = maxDWORD - 1
		}
		// Wait up to ms for the first byte, then return what is there.
		t.ReadIntervalTimeout = maxDWORD
		t.ReadTotalTimeoutMultiplier = maxDWORD
		t.ReadTotalTimeoutConstant = uint32(ms)
	}
	t.WriteTotalTimeoutConstant = 5000
	if err := windows.SetCommTimeouts(p.h, &t); err != nil {
		return fmt.Errorf("serial: set timeouts on %s: %w", p.name, err)
	}
	p.applied = d
	return nil
}

func (p *winPort) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	p.rmu.Lock()
	defer p.rmu.Unlock()
	if p.closed.Load() {
		return 0, ErrClosed
	}
	p.tmu.Lock()
	want := p.timeout
	p.tmu.Unlock()
	if want != p.applied {
		if err := p.applyTimeout(want); err != nil {
			return 0, err
		}
	}
	if err := windows.ResetEvent(p.rov.HEvent); err != nil {
		return 0, err
	}
	var n uint32
	err := windows.ReadFile(p.h, b, &n, &p.rov)
	if err != nil && err != windows.ERROR_IO_PENDING {
		return 0, p.ioError("read", err)
	}
	if err := windows.GetOverlappedResult(p.h, &p.rov, &n, true); err != nil {
		return 0, p.ioError("read", err)
	}
	if n == 0 {
		return 0, ErrTimeout
	}
	return int(n), nil
}

func (p *winPort) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if p.closed.Load() {
		return 0, ErrClosed
	}
	if err := windows.ResetEvent(p.wov.HEvent); err != nil {
		return 0, err
	}
	var n uint32
	err := windows.WriteFile(p.h, b, &n, &p.wov)
	if err != nil && err != windows.ERROR_IO_PENDING {
		return 0, p.ioError("write", err)
	}
	if err := windows.GetOverlappedResult(p.h, &p.wov, &n, true); err != nil {
		return int(n), p.ioError("write", err)
	}
	if int(n) < len(b) {
		return int(n), fmt.Errorf("serial: write to %s timed out (%d of %d bytes)", p.name, n, len(b))
	}
	return int(n), nil
}

func (p *winPort) ioError(op string, err error) error {
	if p.closed.Load() || errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
		return ErrClosed
	}
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_BAD_COMMAND) ||
		errors.Is(err, windows.ERROR_DEVICE_NOT_CONNECTED) || errors.Is(err, windows.ERROR_GEN_FAILURE) {
		return fmt.Errorf("serial: %s on %s failed, adapter disconnected? %w", op, p.name, err)
	}
	return fmt.Errorf("serial: %s on %s: %w", op, p.name, err)
}

func (p *winPort) Close() error {
	if p.closed.Swap(true) {
		return nil
	}
	_ = windows.CancelIoEx(p.h, nil)
	err := windows.CloseHandle(p.h)
	p.rmu.Lock()
	windows.CloseHandle(p.rov.HEvent)
	p.rmu.Unlock()
	p.wmu.Lock()
	windows.CloseHandle(p.wov.HEvent)
	p.wmu.Unlock()
	return err
}

func (p *winPort) SetReadTimeout(d time.Duration) error {
	p.tmu.Lock()
	p.timeout = d
	p.tmu.Unlock()
	return nil
}

func (p *winPort) SetBaud(baud int) error {
	if baud <= 0 {
		return fmt.Errorf("serial: invalid baud rate %d", baud)
	}
	p.cmu.Lock()
	defer p.cmu.Unlock()
	p.cfg.Baud = baud
	return p.setState(func(d *windows.DCB) { p.fillDCB(d) })
}

func (p *winPort) SetBreak(on bool) error {
	if on {
		return windows.SetCommBreak(p.h)
	}
	return windows.ClearCommBreak(p.h)
}

func (p *winPort) SetDTR(on bool) error {
	p.cmu.Lock()
	defer p.cmu.Unlock()
	p.cfg.DTR = on
	fn := uint32(escClrDTR)
	if on {
		fn = escSetDTR
	}
	return windows.EscapeCommFunction(p.h, fn)
}

func (p *winPort) SetRTS(on bool) error {
	p.cmu.Lock()
	defer p.cmu.Unlock()
	p.cfg.RTS = on
	fn := uint32(escClrRTS)
	if on {
		fn = escSetRTS
	}
	return windows.EscapeCommFunction(p.h, fn)
}

func (p *winPort) Flush() error {
	return windows.PurgeComm(p.h, purgeRxClear|purgeTxClear)
}

func (p *winPort) Name() string { return p.name }

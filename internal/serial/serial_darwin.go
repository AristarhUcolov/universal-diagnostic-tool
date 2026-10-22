//go:build darwin

package serial

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// IOSSIOSPEED sets an arbitrary speed on macOS serial drivers
// (_IOW('T', 2, speed_t) with an 8-byte speed_t).
const ioSSIOSpeed = 0x80085402

type darwinPort struct {
	name    string
	fd      int
	rmu     sync.Mutex
	wmu     sync.Mutex
	cmu     sync.Mutex
	timeout atomic.Int64
	closed  atomic.Bool
	cfg     Config
}

func openPort(name string, cfg Config) (Port, error) {
	fd, err := unix.Open(name, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.EBUSY) {
			return nil, fmt.Errorf("serial: %s is busy: %w", name, err)
		}
		return nil, fmt.Errorf("serial: open %s: %w", name, err)
	}
	p := &darwinPort{name: name, fd: fd, cfg: cfg}
	p.timeout.Store(int64(100 * time.Millisecond))
	if err := p.configure(); err != nil {
		unix.Close(fd)
		return nil, err
	}
	_ = unix.IoctlSetInt(fd, unix.TIOCEXCL, 0)
	_ = p.SetDTR(cfg.DTR)
	_ = p.SetRTS(cfg.RTS)
	_ = p.Flush()
	return p, nil
}

func (p *darwinPort) configure() error {
	t, err := unix.IoctlGetTermios(p.fd, unix.TIOCGETA)
	if err != nil {
		return fmt.Errorf("serial: %s is not a serial port: %w", p.name, err)
	}
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR |
		unix.ICRNL | unix.IXON | unix.IXOFF | unix.IXANY | unix.INPCK
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB | unix.PARODD | unix.CSTOPB | unix.CRTSCTS
	t.Cflag |= unix.CREAD | unix.CLOCAL
	switch p.cfg.DataBits {
	case 5:
		t.Cflag |= unix.CS5
	case 6:
		t.Cflag |= unix.CS6
	case 7:
		t.Cflag |= unix.CS7
	default:
		t.Cflag |= unix.CS8
	}
	switch p.cfg.Parity {
	case ParityOdd:
		t.Cflag |= unix.PARENB | unix.PARODD
	case ParityEven:
		t.Cflag |= unix.PARENB
	}
	if p.cfg.StopBits == 2 {
		t.Cflag |= unix.CSTOPB
	}
	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = 0
	// On BSD-derived systems the speed fields hold the numeric rate.
	t.Ispeed = uint64(p.cfg.Baud)
	t.Ospeed = uint64(p.cfg.Baud)
	if err := unix.IoctlSetTermios(p.fd, unix.TIOCSETA, t); err != nil {
		// Non-standard rates (10400) need IOSSIOSPEED after setting a
		// standard one.
		t.Ispeed, t.Ospeed = 9600, 9600
		if err := unix.IoctlSetTermios(p.fd, unix.TIOCSETA, t); err != nil {
			return fmt.Errorf("serial: configure %s: %w", p.name, err)
		}
		speed := uint64(p.cfg.Baud)
		if _, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(p.fd), ioSSIOSpeed, uintptr(unsafe.Pointer(&speed))); e != 0 {
			return fmt.Errorf("serial: %s rejected %d baud: %w", p.name, p.cfg.Baud, e)
		}
	}
	return nil
}

func (p *darwinPort) modem(bit int, on bool) error {
	req := uint(unix.TIOCMBIC)
	if on {
		req = unix.TIOCMBIS
	}
	return unix.IoctlSetPointerInt(p.fd, req, bit)
}

func (p *darwinPort) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	p.rmu.Lock()
	defer p.rmu.Unlock()
	deadline := time.Now().Add(time.Duration(p.timeout.Load()))
	for {
		if p.closed.Load() {
			return 0, ErrClosed
		}
		wait := time.Until(deadline)
		if wait < 0 {
			wait = 0
		}
		if wait > 100*time.Millisecond {
			wait = 100 * time.Millisecond
		}
		fds := []unix.PollFd{{Fd: int32(p.fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int(wait/time.Millisecond))
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("serial: poll %s: %w", p.name, err)
		}
		if n == 0 {
			if time.Now().Before(deadline) {
				continue
			}
			return 0, ErrTimeout
		}
		r, err := unix.Read(p.fd, b)
		if err == unix.EAGAIN || err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("serial: read %s: %w", p.name, err)
		}
		if r == 0 {
			return 0, io.EOF
		}
		return r, nil
	}
}

func (p *darwinPort) Write(b []byte) (int, error) {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	written := 0
	deadline := time.Now().Add(5 * time.Second)
	for written < len(b) {
		if p.closed.Load() {
			return written, ErrClosed
		}
		n, err := unix.Write(p.fd, b[written:])
		if err == unix.EAGAIN || err == unix.EINTR {
			if time.Now().After(deadline) {
				return written, fmt.Errorf("serial: write to %s timed out", p.name)
			}
			fds := []unix.PollFd{{Fd: int32(p.fd), Events: unix.POLLOUT}}
			_, _ = unix.Poll(fds, 50)
			continue
		}
		if err != nil {
			return written, fmt.Errorf("serial: write %s: %w", p.name, err)
		}
		written += n
	}
	return written, nil
}

func (p *darwinPort) Close() error {
	if p.closed.Swap(true) {
		return nil
	}
	p.rmu.Lock()
	p.wmu.Lock()
	err := unix.Close(p.fd)
	p.wmu.Unlock()
	p.rmu.Unlock()
	return err
}

func (p *darwinPort) SetReadTimeout(d time.Duration) error {
	p.timeout.Store(int64(d))
	return nil
}

func (p *darwinPort) SetBaud(baud int) error {
	if baud <= 0 {
		return fmt.Errorf("serial: invalid baud rate %d", baud)
	}
	p.cmu.Lock()
	defer p.cmu.Unlock()
	p.cfg.Baud = baud
	return p.configure()
}

func (p *darwinPort) SetBreak(on bool) error {
	if on {
		return unix.IoctlSetInt(p.fd, unix.TIOCSBRK, 0)
	}
	return unix.IoctlSetInt(p.fd, unix.TIOCCBRK, 0)
}

func (p *darwinPort) SetDTR(on bool) error { return p.modem(unix.TIOCM_DTR, on) }
func (p *darwinPort) SetRTS(on bool) error { return p.modem(unix.TIOCM_RTS, on) }

func (p *darwinPort) Flush() error {
	const fread, fwrite = 0x1, 0x2 // sys/fcntl.h
	return unix.IoctlSetPointerInt(p.fd, unix.TIOCFLUSH, fread|fwrite)
}

func (p *darwinPort) Name() string { return p.name }

func listPorts() ([]PortInfo, error) {
	matches, err := filepath.Glob("/dev/cu.*")
	if err != nil {
		return nil, err
	}
	var out []PortInfo
	for _, m := range matches {
		base := strings.TrimPrefix(m, "/dev/cu.")
		if base == "Bluetooth-Incoming-Port" || base == "debug-console" {
			continue
		}
		pi := PortInfo{Name: m}
		switch {
		case strings.HasPrefix(base, "usbserial"):
			pi.Description = "USB serial (FTDI/PL2303)"
		case strings.HasPrefix(base, "wchusbserial"):
			pi.Description = "USB serial (CH340)"
		case strings.HasPrefix(base, "SLAB_USBtoUART"):
			pi.Description = "USB serial (CP210x)"
		case strings.HasPrefix(base, "usbmodem"):
			pi.Description = "USB CDC modem (STN/CANable)"
		default:
			pi.Description = base
		}
		if _, err := os.Stat(m); err == nil {
			out = append(out, pi)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

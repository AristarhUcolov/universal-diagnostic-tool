//go:build linux

package serial

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

type unixPort struct {
	name    string
	fd      int
	rmu     sync.Mutex
	wmu     sync.Mutex
	cmu     sync.Mutex
	timeout atomic.Int64 // nanoseconds
	closed  atomic.Bool
	cfg     Config
}

func openPort(name string, cfg Config) (Port, error) {
	fd, err := unix.Open(name, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.EACCES) {
			return nil, fmt.Errorf("serial: no permission for %s (add your user to the dialout group): %w", name, err)
		}
		if errors.Is(err, unix.EBUSY) {
			return nil, fmt.Errorf("serial: %s is busy: %w", name, err)
		}
		return nil, fmt.Errorf("serial: open %s: %w", name, err)
	}
	p := &unixPort{name: name, fd: fd, cfg: cfg}
	p.timeout.Store(int64(100 * time.Millisecond))
	if err := p.configure(); err != nil {
		unix.Close(fd)
		return nil, err
	}
	_ = unix.IoctlSetInt(fd, unix.TIOCEXCL, 0)
	_ = p.modem(unix.TIOCM_DTR, cfg.DTR)
	_ = p.modem(unix.TIOCM_RTS, cfg.RTS)
	_ = p.Flush()
	lowerLatency(name)
	return p, nil
}

func (p *unixPort) configure() error {
	t, err := unix.IoctlGetTermios(p.fd, unix.TCGETS2)
	if err != nil {
		return fmt.Errorf("serial: %s is not a serial port: %w", p.name, err)
	}
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR |
		unix.ICRNL | unix.IXON | unix.IXOFF | unix.IXANY | unix.INPCK
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB | unix.PARODD | unix.CSTOPB | unix.CRTSCTS | unix.CBAUD | unix.CIBAUD
	t.Cflag |= unix.CREAD | unix.CLOCAL | unix.BOTHER
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
	t.Ispeed = uint32(p.cfg.Baud)
	t.Ospeed = uint32(p.cfg.Baud)
	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(p.fd, unix.TCSETS2, t); err != nil {
		return fmt.Errorf("serial: %s rejected %d baud: %w", p.name, p.cfg.Baud, err)
	}
	return nil
}

func (p *unixPort) modem(bit int, on bool) error {
	req := uint(unix.TIOCMBIC)
	if on {
		req = unix.TIOCMBIS
	}
	return unix.IoctlSetPointerInt(p.fd, req, bit)
}

func (p *unixPort) Read(b []byte) (int, error) {
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
		// Poll in short slices so that Close from another goroutine is
		// noticed quickly.
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
		if fds[0].Revents&unix.POLLIN == 0 && fds[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return 0, fmt.Errorf("serial: %s disconnected", p.name)
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

func (p *unixPort) Write(b []byte) (int, error) {
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
	// Wait until the bytes have physically left the UART so that timing
	// measured by the caller starts at the end of transmission.
	_ = unix.IoctlSetInt(p.fd, unix.TCSBRK, 1)
	return written, nil
}

func (p *unixPort) Close() error {
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

func (p *unixPort) SetReadTimeout(d time.Duration) error {
	p.timeout.Store(int64(d))
	return nil
}

func (p *unixPort) SetBaud(baud int) error {
	if baud <= 0 {
		return fmt.Errorf("serial: invalid baud rate %d", baud)
	}
	p.cmu.Lock()
	defer p.cmu.Unlock()
	p.cfg.Baud = baud
	return p.configure()
}

func (p *unixPort) SetBreak(on bool) error {
	if on {
		return unix.IoctlSetInt(p.fd, unix.TIOCSBRK, 0)
	}
	return unix.IoctlSetInt(p.fd, unix.TIOCCBRK, 0)
}

func (p *unixPort) SetDTR(on bool) error { return p.modem(unix.TIOCM_DTR, on) }
func (p *unixPort) SetRTS(on bool) error { return p.modem(unix.TIOCM_RTS, on) }

func (p *unixPort) Flush() error { return unix.IoctlSetInt(p.fd, unix.TCFLSH, unix.TCIOFLUSH) }

func (p *unixPort) Name() string { return p.name }

// lowerLatency sets the FTDI latency timer to 1 ms when the kernel lets
// us (it usually needs root). The default 16 ms makes K-Line byte-level
// protocols such as KW1281 sluggish.
func lowerLatency(dev string) {
	base := filepath.Base(dev)
	path := filepath.Join("/sys/bus/usb-serial/devices", base, "latency_timer")
	if _, err := os.Stat(path); err == nil {
		_ = os.WriteFile(path, []byte("1"), 0o644)
	}
}

//go:build linux

package socketcan

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
)

const (
	effFlag = 0x80000000
	rtrFlag = 0x40000000
	errFlag = 0x20000000
	effMask = 0x1FFFFFFF
	sffMask = 0x7FF
	fdBRS   = 0x01
)

// Bus is an open SocketCAN interface (can.Bus).
type Bus struct {
	fd     int
	name   string
	fd64   bool
	closed atomic.Bool
}

// Open binds a raw CAN socket to the interface. CAN FD frames are
// enabled when the interface supports them.
func Open(iface string) (can.Bus, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, fmt.Errorf("socketcan: interface %s not found (is it up?): %w", iface, err)
	}
	fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW, unix.CAN_RAW)
	if err != nil {
		return nil, fmt.Errorf("socketcan: socket: %w", err)
	}
	b := &Bus{fd: fd, name: iface}
	if err := unix.SetsockoptInt(fd, unix.SOL_CAN_RAW, unix.CAN_RAW_FD_FRAMES, 1); err == nil {
		b.fd64 = true
	}
	if err := unix.Bind(fd, &unix.SockaddrCAN{Ifindex: ifi.Index}); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("socketcan: bind %s: %w", iface, err)
	}
	return b, nil
}

func (b *Bus) Send(ctx context.Context, f can.Frame) error {
	id := f.ID & sffMask
	if f.Ext {
		id = f.ID&effMask | effFlag
	}
	if f.RTR {
		id |= rtrFlag
	}
	size := 16
	if f.FD {
		size = 72
	}
	buf := make([]byte, size)
	binary.LittleEndian.PutUint32(buf, id)
	buf[4] = f.Len
	if f.FD && f.BRS {
		buf[5] = fdBRS
	}
	copy(buf[8:], f.Payload())
	_, err := unix.Write(b.fd, buf)
	return err
}

func (b *Bus) Recv(ctx context.Context, timeout time.Duration) (can.Frame, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 72)
	for {
		if b.closed.Load() {
			return can.Frame{}, can.ErrClosed
		}
		wait := time.Until(deadline)
		if wait <= 0 {
			return can.Frame{}, can.ErrTimeout
		}
		if wait > 100*time.Millisecond {
			wait = 100 * time.Millisecond
		}
		fds := []unix.PollFd{{Fd: int32(b.fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int(wait/time.Millisecond))
		if err == unix.EINTR || n == 0 {
			continue
		}
		if err != nil {
			return can.Frame{}, err
		}
		r, err := unix.Read(b.fd, buf)
		if err != nil {
			return can.Frame{}, err
		}
		if r < 16 {
			continue
		}
		raw := binary.LittleEndian.Uint32(buf)
		if raw&errFlag != 0 {
			continue
		}
		var f can.Frame
		f.Ext = raw&effFlag != 0
		f.RTR = raw&rtrFlag != 0
		if f.Ext {
			f.ID = raw & effMask
		} else {
			f.ID = raw & sffMask
		}
		n2 := int(buf[4])
		if r == 72 {
			f.FD = true
			f.BRS = buf[5]&fdBRS != 0
		} else if n2 > 8 {
			n2 = 8
		}
		f.Len = uint8(n2)
		copy(f.Data[:], buf[8:8+n2])
		f.Time = time.Now()
		return f, nil
	}
}

func (b *Bus) Info() can.BusInfo { return can.BusInfo{Name: "SocketCAN " + b.name, FD: b.fd64} }

func (b *Bus) Close() error {
	if b.closed.Swap(true) {
		return nil
	}
	return unix.Close(b.fd)
}

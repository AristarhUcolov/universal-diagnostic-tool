//go:build windows

package rp1210

import (
	"context"
	"encoding/binary"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/j1939"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
)

// Link is a J1939 connection through an RP1210 adapter; it implements
// j1939.Link. The adapter's driver does the transport protocol.
type Link struct {
	dll    *windows.LazyDLL
	procs  map[string]*windows.LazyProc
	client int16
	addr   byte
	name   string
	log    journal.Logger
	ch     chan func()
	mu     sync.Mutex
}

func (l *Link) do(f func()) {
	done := make(chan struct{})
	l.ch <- func() { f(); close(done) }
	<-done
}

// OpenJ1939 connects to the adapter's J1939 protocol and claims addr.
func OpenJ1939(a Adapter, addr byte, log journal.Logger) (j1939.Link, error) {
	l := &Link{dll: windows.NewLazyDLL(a.Implementation + ".dll"), procs: map[string]*windows.LazyProc{},
		addr: addr, name: fmt.Sprintf("RP1210 %s (%s)", a.Description, a.Implementation), log: journal.Or(log), ch: make(chan func())}
	if err := l.dll.Load(); err != nil {
		return nil, fmt.Errorf("rp1210: cannot load %s.dll (32-bit drivers need the 32-bit build of this program): %w", a.Implementation, err)
	}
	for _, n := range []string{"RP1210_ClientConnect", "RP1210_ClientDisconnect", "RP1210_SendMessage", "RP1210_ReadMessage", "RP1210_SendCommand", "RP1210_GetErrorMsg"} {
		p := l.dll.NewProc(n)
		if err := p.Find(); err != nil {
			return nil, fmt.Errorf("rp1210: %s.dll lacks %s", a.Implementation, n)
		}
		l.procs[n] = p
	}
	go func() {
		runtime.LockOSThread()
		for f := range l.ch {
			f()
		}
	}()
	proto := []byte("J1939:Baud=Auto\x00")
	var r uintptr
	l.do(func() {
		r, _, _ = l.procs["RP1210_ClientConnect"].Call(0, uintptr(a.DeviceID), uintptr(unsafe.Pointer(&proto[0])), 0, 0, 0)
	})
	runtime.KeepAlive(proto)
	client := int16(uint16(r))
	if client < 0 || client > 127 {
		close(l.ch)
		return nil, fmt.Errorf("rp1210: ClientConnect failed: %s", l.errorText(client))
	}
	l.client = client
	l.command(cmdSetAllFiltersPass, nil)
	// Claim our address: address, NAME (8 bytes), block until done.
	claim := make([]byte, 10)
	claim[0] = addr
	binary.LittleEndian.PutUint64(claim[1:], j1939.ToolName)
	claim[9] = 0
	if st := l.command(cmdProtectJ1939Addr, claim); st != 0 {
		journal.Warnf(l.log, "rp1210", "address claim returned %s", l.errorText(st))
	}
	journal.Infof(l.log, "rp1210", "%s connected as client %d", l.name, client)
	return l, nil
}

func (l *Link) command(cmd int, data []byte) int16 {
	var r uintptr
	var p uintptr
	if len(data) > 0 {
		p = uintptr(unsafe.Pointer(&data[0]))
	}
	l.do(func() {
		r, _, _ = l.procs["RP1210_SendCommand"].Call(uintptr(cmd), uintptr(l.client), p, uintptr(len(data)))
	})
	runtime.KeepAlive(data)
	return int16(uint16(r))
}

func (l *Link) errorText(code int16) string {
	buf := make([]byte, 81)
	l.do(func() {
		l.procs["RP1210_GetErrorMsg"].Call(uintptr(code), uintptr(unsafe.Pointer(&buf[0])))
	})
	s := string(buf)
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		s = fmt.Sprintf("error %d", code)
	}
	return s
}

// Address implements j1939.Link.
func (l *Link) Address() byte { return l.addr }

// Name implements j1939.Link.
func (l *Link) Name() string { return l.name }

// Send implements j1939.Link.
func (l *Link) Send(ctx context.Context, m j1939.Message) error {
	if m.Priority == 0 {
		m.Priority = 6
	}
	buf := []byte{byte(m.PGN), byte(m.PGN >> 8), byte(m.PGN >> 16), m.Priority & 7, l.addr, m.Dst}
	buf = append(buf, m.Data...)
	var r uintptr
	l.do(func() {
		r, _, _ = l.procs["RP1210_SendMessage"].Call(uintptr(l.client), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 1)
	})
	runtime.KeepAlive(buf)
	if st := int16(uint16(r)); st != 0 {
		return fmt.Errorf("rp1210: send failed: %s", l.errorText(st))
	}
	return nil
}

// Recv implements j1939.Link (polls the adapter without blocking).
func (l *Link) Recv(ctx context.Context, timeout time.Duration) (j1939.Message, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 1800)
	for {
		var r uintptr
		l.do(func() {
			r, _, _ = l.procs["RP1210_ReadMessage"].Call(uintptr(l.client), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
		})
		n := int16(uint16(r))
		if n > 10 {
			// timestamp(4) PGN(3) priority(1) source(1) destination(1) data
			b := buf[:n]
			m := j1939.Message{PGN: uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16, Priority: b[7] & 7,
				Src: b[8], Dst: b[9], Data: append([]byte(nil), b[10:]...), Time: time.Now()}
			return m, nil
		}
		if n < 0 {
			return j1939.Message{}, fmt.Errorf("rp1210: read failed: %s", l.errorText(n))
		}
		if time.Now().After(deadline) {
			return j1939.Message{}, j1939.ErrTimeout
		}
		select {
		case <-ctx.Done():
			return j1939.Message{}, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// Request asks for a PGN (used by the J1939 handler).
func (l *Link) Request(ctx context.Context, pgn uint32, dst byte) error {
	return l.Send(ctx, j1939.Message{PGN: j1939.PGNRequest, Priority: 6, Dst: dst, Data: []byte{byte(pgn), byte(pgn >> 8), byte(pgn >> 16)}})
}

// Close implements j1939.Link.
func (l *Link) Close() error {
	l.do(func() { l.procs["RP1210_ClientDisconnect"].Call(uintptr(l.client)) })
	close(l.ch)
	return nil
}

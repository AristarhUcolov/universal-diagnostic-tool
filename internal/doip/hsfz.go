package doip

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
)

// BMW HSFZ control words.
const (
	HSFZPort        = 6801
	HSFZDiscovery   = 6811
	hsfzDiagnostic  = 0x0001
	hsfzAck         = 0x0002
	hsfzIdent       = 0x0011
	hsfzAliveCheck  = 0x0012
	HSFZTester      = 0xF4
	HSFZGateway     = 0x10
	hsfzMaxPayload  = 1 << 20
	hsfzErrorsStart = 0x0040
)

// HSFZConn is a BMW ENET connection (implements diag.Channel). The
// payload is [source][target][UDS data].
type HSFZConn struct {
	c      net.Conn
	target byte
	log    journal.Logger
	rx     chan []byte
	done   chan struct{}
	wmu    sync.Mutex
	addr   string
	closed bool
}

// DiscoverHSFZ asks BMW vehicles on the local network to identify
// themselves and returns "ip: identification" strings.
func DiscoverHSFZ(ctx context.Context, timeout time.Duration) ([]string, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	msg := []byte{0, 0, 0, 0, 0, hsfzIdent}
	if _, err := conn.WriteToUDP(msg, &net.UDPAddr{IP: net.IPv4bcast, Port: HSFZDiscovery}); err != nil {
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var out []string
	buf := make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		if n > 6 {
			out = append(out, addr.IP.String()+": "+strings.TrimSpace(string(buf[6:n])))
		}
	}
	return out, nil
}

// DialHSFZ connects to a BMW gateway (default port 6801).
func DialHSFZ(ctx context.Context, addr string, target byte, log journal.Logger) (*HSFZConn, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, fmt.Sprint(HSFZPort))
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	nc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("hsfz: cannot connect to %s (ENET cable and IP address?): %w", addr, err)
	}
	c := &HSFZConn{c: nc, target: target, log: journal.Or(log), rx: make(chan []byte, 256), done: make(chan struct{}), addr: addr}
	go c.reader()
	return c, nil
}

func (c *HSFZConn) reader() {
	defer close(c.done)
	var h [6]byte
	for {
		if _, err := io.ReadFull(c.c, h[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(h[:4])
		ctl := binary.BigEndian.Uint16(h[4:])
		if n > hsfzMaxPayload {
			return
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(c.c, p); err != nil {
			return
		}
		switch {
		case ctl == hsfzDiagnostic:
			select {
			case c.rx <- p:
			default:
			}
		case ctl == hsfzAliveCheck:
			_ = c.write(hsfzAliveCheck, []byte{0x00, HSFZTester})
		case ctl >= hsfzErrorsStart:
			journal.Warnf(c.log, "hsfz", "gateway reported error 0x%04X", ctl)
		}
	}
}

func (c *HSFZConn) write(ctl uint16, p []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	h := make([]byte, 6, 6+len(p))
	binary.BigEndian.PutUint32(h, uint32(len(p)))
	binary.BigEndian.PutUint16(h[4:], ctl)
	_ = c.c.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err := c.c.Write(append(h, p...))
	return err
}

// Send implements diag.Channel.
func (c *HSFZConn) Send(ctx context.Context, data []byte) error {
	if c.closed {
		return diag.ErrClosed
	}
	journal.Tx(c.log, "hsfz", fmt.Sprintf("%02X", c.target), data, diag.ServiceName(data[0]))
	return c.write(hsfzDiagnostic, append([]byte{HSFZTester, c.target}, data...))
}

// Recv implements diag.Channel.
func (c *HSFZConn) Recv(ctx context.Context, timeout time.Duration) (diag.Message, error) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case p := <-c.rx:
			if len(p) < 3 || p[1] != HSFZTester {
				continue
			}
			m := diag.Message{Src: uint32(p[0]), Dst: uint32(p[1]), Data: append([]byte(nil), p[2:]...), Time: time.Now()}
			journal.Rx(c.log, "hsfz", fmt.Sprintf("%02X", p[0]), m.Data, diag.ServiceName(m.Data[0]))
			return m, nil
		case <-t.C:
			return diag.Message{}, diag.ErrTimeout
		case <-ctx.Done():
			return diag.Message{}, ctx.Err()
		case <-c.done:
			return diag.Message{}, errors.New("hsfz: connection closed")
		}
	}
}

// Info implements diag.Channel.
func (c *HSFZConn) Info() diag.ChannelInfo {
	return diag.ChannelInfo{Protocol: "BMW HSFZ (UDS over ENET)", ProtoID: "hsfz", Adapter: c.addr,
		Target: uint32(c.target), Tester: HSFZTester, CAN: true}
}

// Close implements diag.Channel.
func (c *HSFZConn) Close() error {
	c.closed = true
	return c.c.Close()
}

// Package elm327 drives ELM327-compatible adapters (ELM327 and its many
// clones on USB, Bluetooth and Wi-Fi, OBDLink SX/LX/MX+/EX/CX, STN11xx
// and STN21xx based tools, ELM323) and also contains an ELM327
// emulator, which lets phone apps such as Torque or Car Scanner use any
// adapter and any control unit through this program.
package elm327

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/serial"
)

// Port is the byte stream to the adapter (a serial port or TCP socket).
type Port interface {
	Write(p []byte) (int, error)
	Read(p []byte) (int, error)
	SetReadTimeout(d time.Duration) error
	Close() error
	Name() string
}

// Errors reported by the adapter.
var (
	ErrNoData          = errors.New("elm327: NO DATA (the vehicle did not answer)")
	ErrUnknownCommand  = errors.New("elm327: adapter does not understand the command (?)")
	ErrUnableToConnect = errors.New("elm327: UNABLE TO CONNECT (no OBD protocol found; is the ignition on?)")
	ErrBusInit         = errors.New("elm327: BUS INIT ERROR (K-Line initialisation failed)")
	ErrCAN             = errors.New("elm327: CAN ERROR (bus problem or wrong CAN speed)")
	ErrBufferFull      = errors.New("elm327: BUFFER FULL (adapter too slow; use a higher serial speed)")
	ErrNoPrompt        = errors.New("elm327: adapter did not answer (wrong port or baud rate?)")
	ErrTimeout         = errors.New("elm327: read timeout")
)

// TCPPort is a Wi-Fi adapter connection.
type TCPPort struct {
	c       net.Conn
	timeout time.Duration
	name    string
}

// DialTCP connects to a Wi-Fi adapter, usually 192.168.0.10:35000.
func DialTCP(addr string, timeout time.Duration) (*TCPPort, error) {
	if !strings.Contains(addr, ":") {
		addr += ":35000"
	}
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("elm327: cannot reach %s (connect to the adapter's Wi-Fi first): %w", addr, err)
	}
	return &TCPPort{c: c, timeout: time.Second, name: "tcp://" + addr}, nil
}

// NewConnPort wraps any network connection (also net.Pipe) as a Port.
func NewConnPort(c net.Conn, name string) *TCPPort {
	return &TCPPort{c: c, timeout: time.Second, name: name}
}

func (t *TCPPort) Write(p []byte) (int, error) {
	_ = t.c.SetWriteDeadline(time.Now().Add(3 * time.Second))
	return t.c.Write(p)
}

func (t *TCPPort) Read(p []byte) (int, error) {
	_ = t.c.SetReadDeadline(time.Now().Add(t.timeout))
	n, err := t.c.Read(p)
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return n, ErrTimeout
	}
	return n, err
}

func (t *TCPPort) SetReadTimeout(d time.Duration) error { t.timeout = d; return nil }
func (t *TCPPort) Close() error                         { return t.c.Close() }
func (t *TCPPort) Name() string                         { return t.name }

// Device is an initialised adapter.
type Device struct {
	port    Port
	log     journal.Logger
	mu      sync.Mutex
	buf     []byte
	Version string // ATI, e.g. "ELM327 v1.5"
	STN     string // STI on STN-based tools (OBDLink), empty otherwise
	Desc    string // AT@1
	proto   byte   // protocol currently selected
}

// Open resets the adapter and configures it for this program: echo off,
// headers and spaces on, adaptive timing.
func Open(ctx context.Context, port Port, log journal.Logger) (*Device, error) {
	d := &Device{port: port, log: journal.Or(log)}
	// Cancel any half-entered command and wake the adapter.
	_, _ = port.Write([]byte("\r"))
	d.drain(300 * time.Millisecond)
	lines, err := d.Cmd(ctx, "ATZ", 4*time.Second)
	if err != nil {
		// Some clones answer only after a second try.
		lines, err = d.Cmd(ctx, "ATZ", 4*time.Second)
		if err != nil {
			return nil, err
		}
	}
	banner := strings.Join(lines, " ")
	for _, c := range []string{"ATE0", "ATL0", "ATS1", "ATH1", "ATAT1"} {
		if _, err := d.Cmd(ctx, c, 2*time.Second); err != nil {
			return nil, fmt.Errorf("elm327: %s failed: %w", c, err)
		}
	}
	if l, err := d.Cmd(ctx, "ATI", 2*time.Second); err == nil && len(l) > 0 {
		d.Version = l[0]
	} else {
		d.Version = banner
	}
	if l, err := d.Cmd(ctx, "STI", 2*time.Second); err == nil && len(l) > 0 && !strings.HasPrefix(l[0], "?") {
		d.STN = l[0]
	}
	if l, err := d.Cmd(ctx, "AT@1", 2*time.Second); err == nil && len(l) > 0 {
		d.Desc = l[0]
	}
	journal.Infof(d.log, "elm", "adapter: %s %s %s", d.Version, d.STN, d.Desc)
	if strings.Contains(d.Version, "v2.") && d.STN == "" {
		journal.Warnf(d.log, "elm", "\"%s\" is usually a clone: real ELM327 chips stopped at v2.3 and many cheap v2.1 clones lack K-Line features", d.Version)
	}
	return d, nil
}

// Name describes the adapter.
func (d *Device) Name() string {
	s := d.Version
	if d.STN != "" {
		s += " / " + d.STN
	}
	return s + " on " + d.port.Name()
}

// Close closes the port.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, _ = d.port.Write([]byte("ATPC\r"))
	return d.port.Close()
}

func (d *Device) drain(wait time.Duration) {
	_ = d.port.SetReadTimeout(wait)
	tmp := make([]byte, 256)
	for {
		n, err := d.port.Read(tmp)
		if n == 0 || err != nil {
			break
		}
	}
	d.buf = d.buf[:0]
}

// Cmd sends a command and returns the answer lines (empty lines, the
// echo and "SEARCHING..." removed).
func (d *Device) Cmd(ctx context.Context, cmd string, timeout time.Duration) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cmdLocked(ctx, cmd, timeout)
}

func (d *Device) cmdLocked(ctx context.Context, cmd string, timeout time.Duration) ([]string, error) {
	d.buf = d.buf[:0]
	journal.Tx(d.log, "elm", "", nil, cmd)
	if _, err := d.port.Write([]byte(cmd + "\r")); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	tmp := make([]byte, 512)
	for {
		if i := strings.IndexByte(string(d.buf), '>'); i >= 0 {
			text := string(d.buf[:i])
			d.buf = append(d.buf[:0], d.buf[i+1:]...)
			lines := splitLines(text, cmd)
			journal.Rx(d.log, "elm", "", nil, strings.Join(lines, " | "))
			return lines, lineError(lines)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			journal.Warnf(d.log, "elm", "no prompt after %q (got %q)", cmd, string(d.buf))
			return nil, ErrNoPrompt
		}
		if remaining > 200*time.Millisecond {
			remaining = 200 * time.Millisecond
		}
		_ = d.port.SetReadTimeout(remaining)
		n, err := d.port.Read(tmp)
		if n > 0 {
			d.buf = append(d.buf, tmp[:n]...)
		}
		if err != nil && !errors.Is(err, ErrTimeout) && !errors.Is(err, serial.ErrTimeout) {
			return nil, err
		}
	}
}

func splitLines(text, cmd string) []string {
	text = strings.ReplaceAll(text, "\n", "\r")
	var out []string
	for _, l := range strings.Split(text, "\r") {
		l = strings.TrimSpace(strings.ReplaceAll(l, "\x00", ""))
		if l == "" || strings.EqualFold(strings.ReplaceAll(l, " ", ""), strings.ReplaceAll(cmd, " ", "")) {
			continue
		}
		if strings.HasPrefix(l, "SEARCHING") {
			continue
		}
		out = append(out, l)
	}
	return out
}

// lineError maps the adapter's error messages to errors. Lines with data
// take precedence: an answer may contain both data and a trailing note.
func lineError(lines []string) error {
	hasData := false
	var err error
	for _, l := range lines {
		u := strings.ToUpper(l)
		switch {
		case u == "?":
			err = ErrUnknownCommand
		case strings.HasPrefix(u, "NO DATA"):
			err = ErrNoData
		case strings.Contains(u, "UNABLE TO CONNECT"):
			err = ErrUnableToConnect
		case strings.HasPrefix(u, "BUS INIT") && strings.Contains(u, "ERROR"):
			err = ErrBusInit
		case strings.HasPrefix(u, "BUS INIT"):
		case strings.Contains(u, "CAN ERROR"), strings.HasPrefix(u, "BUS ERROR"), strings.HasPrefix(u, "BUS BUSY"):
			err = ErrCAN
		case strings.Contains(u, "BUFFER FULL"):
			err = ErrBufferFull
		case strings.HasPrefix(u, "STOPPED"), strings.HasPrefix(u, "<DATA ERROR"), strings.HasPrefix(u, "<RX ERROR"),
			strings.HasPrefix(u, "FB ERROR"), strings.HasPrefix(u, "LV RESET"), strings.HasPrefix(u, "ERR"):
			err = fmt.Errorf("elm327: %s", l)
		default:
			hasData = true
		}
	}
	if hasData {
		return nil
	}
	return err
}

// Voltage reads the supply voltage on OBD pin 16.
func (d *Device) Voltage(ctx context.Context) (float64, error) {
	lines, err := d.Cmd(ctx, "ATRV", 2*time.Second)
	if err != nil {
		return 0, err
	}
	for _, l := range lines {
		var v float64
		if _, err := fmt.Sscanf(strings.TrimSuffix(strings.ToUpper(l), "V"), "%f", &v); err == nil {
			return v, nil
		}
	}
	return 0, fmt.Errorf("elm327: unexpected voltage answer %v", lines)
}

// ProtocolName names an ELM327 protocol number.
func ProtocolName(p byte) string {
	switch p {
	case '1':
		return "SAE J1850 PWM (41.6 kbaud)"
	case '2':
		return "SAE J1850 VPW (10.4 kbaud)"
	case '3':
		return "ISO 9141-2 (5 baud init, 10.4 kbaud)"
	case '4':
		return "ISO 14230-4 KWP (5 baud init, 10.4 kbaud)"
	case '5':
		return "ISO 14230-4 KWP (fast init, 10.4 kbaud)"
	case '6':
		return "ISO 15765-4 CAN (11 bit ID, 500 kbaud)"
	case '7':
		return "ISO 15765-4 CAN (29 bit ID, 500 kbaud)"
	case '8':
		return "ISO 15765-4 CAN (11 bit ID, 250 kbaud)"
	case '9':
		return "ISO 15765-4 CAN (29 bit ID, 250 kbaud)"
	case 'A':
		return "SAE J1939 CAN (29 bit ID, 250 kbaud)"
	case 'B':
		return "USER1 CAN (11 bit ID, 125 kbaud)"
	case 'C':
		return "USER2 CAN (11 bit ID, 50 kbaud)"
	}
	return "automatic"
}

// IsCAN reports whether a protocol number is CAN based.
func IsCAN(p byte) bool { return p >= '6' && p <= '9' || p >= 'A' && p <= 'C' }

// Is29Bit reports whether the protocol uses 29-bit identifiers.
func Is29Bit(p byte) bool { return p == '7' || p == '9' || p == 'A' }

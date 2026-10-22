// Package serial opens serial ports (COM ports on Windows, tty devices on
// Linux and macOS) with the features vehicle adapters need and most
// libraries hide: arbitrary baud rates such as 10400 or 360, the break
// condition (to pull the K-Line low for initialisation), direct DTR/RTS
// control and per-call read timeouts. No cgo is used.
package serial

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrTimeout is returned by Read when no byte arrived in time.
var ErrTimeout = errors.New("serial: read timeout")

// ErrClosed is returned after Close.
var ErrClosed = errors.New("serial: port closed")

// Parity of the serial frame.
type Parity int

const (
	ParityNone Parity = iota
	ParityOdd
	ParityEven
)

// Config describes how to open a port.
type Config struct {
	Baud     int    // bits per second, any value the driver accepts
	DataBits int    // 5..8, default 8
	Parity   Parity // default none
	StopBits int    // 1 or 2, default 1
	DTR      bool   // initial DTR level
	RTS      bool   // initial RTS level
}

func (c *Config) normalize() error {
	if c.Baud <= 0 {
		return fmt.Errorf("serial: invalid baud rate %d", c.Baud)
	}
	if c.DataBits == 0 {
		c.DataBits = 8
	}
	if c.DataBits < 5 || c.DataBits > 8 {
		return fmt.Errorf("serial: invalid data bits %d", c.DataBits)
	}
	if c.StopBits == 0 {
		c.StopBits = 1
	}
	if c.StopBits != 1 && c.StopBits != 2 {
		return fmt.Errorf("serial: invalid stop bits %d", c.StopBits)
	}
	return nil
}

// Port is an open serial port. Read and Write may be used from different
// goroutines at the same time.
type Port interface {
	// Read waits up to the read timeout for at least one byte and returns
	// what is available. It returns ErrTimeout when nothing arrived.
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
	// SetReadTimeout sets how long Read waits for the first byte.
	SetReadTimeout(d time.Duration) error
	// SetBaud changes the speed without closing the port.
	SetBaud(baud int) error
	// SetBreak holds TxD in the spacing (low) state while on is true.
	SetBreak(on bool) error
	SetDTR(on bool) error
	SetRTS(on bool) error
	// Flush discards unread input and unsent output.
	Flush() error
	Name() string
}

// PortInfo describes a port found on the system.
type PortInfo struct {
	Name         string `json:"name"`                   // COM3, /dev/ttyUSB0
	Description  string `json:"description,omitempty"`  // friendly name from the OS
	VID          string `json:"vid,omitempty"`          // USB vendor id (hex)
	PID          string `json:"pid,omitempty"`          // USB product id (hex)
	Serial       string `json:"serial,omitempty"`       // USB serial number
	Manufacturer string `json:"manufacturer,omitempty"` // manufacturer string
	Chip         string `json:"chip,omitempty"`         // recognised USB-serial chip
	Hint         string `json:"hint,omitempty"`         // what adapters usually use this chip
}

// usbChips maps VID:PID pairs of common USB-serial converters to a chip
// name and the vehicle adapters that are typically built on them.
var usbChips = map[string][2]string{
	"0403:6001": {"FTDI FT232R/FT232BL", "VAG KKL 409.1, K+DCAN, Galletto, ELM327 USB"},
	"0403:6015": {"FTDI FT231X/FT230X", "OBDLink SX, ELM327 USB, KKL cables"},
	"0403:6010": {"FTDI FT2232", "dual-channel adapters"},
	"0403:6011": {"FTDI FT4232", "multi-channel adapters"},
	"0403:6014": {"FTDI FT232H", "high-speed adapters"},
	"0403:FA20": {"FTDI (Lawicel CANUSB)", "Lawicel CANUSB"},
	"1A86:7523": {"WCH CH340/CH341", "cheap KKL and ELM327 USB clones"},
	"1A86:5523": {"WCH CH341", "cheap KKL and ELM327 USB clones"},
	"1A86:55D3": {"WCH CH343", "newer ELM327 clones"},
	"1A86:7522": {"WCH CH340K", "cheap KKL and ELM327 USB clones"},
	"10C4:EA60": {"Silicon Labs CP210x", "ELM327 USB clones, OBD cables"},
	"067B:2303": {"Prolific PL2303", "old USB-RS232 converters, ELM327 clones"},
	"067B:23A3": {"Prolific PL2303GC", "USB-RS232 converters"},
	"0483:5740": {"STM32 virtual COM", "STN/OBDLink-style adapters, CANable (slcan)"},
	"AD50:60C4": {"CANable (slcan firmware)", "CANable, CANtact"},
	"16D0:117E": {"CANable (slcan)", "CANable"},
	"1D50:606F": {"candleLight (gs_usb)", "CANable with candleLight firmware (use SocketCAN on Linux)"},
	"2341:0043": {"Arduino Uno", "DIY K-Line/CAN adapters"},
	"2341:0042": {"Arduino Mega", "DIY K-Line/CAN adapters"},
}

// identify fills Chip and Hint from the USB ids.
func (pi *PortInfo) identify() {
	key := strings.ToUpper(pi.VID + ":" + pi.PID)
	if c, ok := usbChips[key]; ok {
		pi.Chip, pi.Hint = c[0], c[1]
		return
	}
	d := strings.ToLower(pi.Description)
	switch {
	case strings.Contains(d, "bluetooth"):
		pi.Chip, pi.Hint = "Bluetooth SPP", "ELM327 Bluetooth, OBDLink LX/MX+ (pair first)"
	case strings.Contains(d, "ftdi") || strings.Contains(d, "usb serial port"):
		pi.Chip, pi.Hint = "FTDI", "KKL / ELM327 USB"
	case strings.Contains(d, "ch340") || strings.Contains(d, "ch341"):
		pi.Chip, pi.Hint = "WCH CH340/CH341", "KKL / ELM327 USB clones"
	case strings.Contains(d, "cp210"):
		pi.Chip, pi.Hint = "Silicon Labs CP210x", "ELM327 USB clones"
	case strings.Contains(d, "prolific"):
		pi.Chip, pi.Hint = "Prolific PL2303", "USB-RS232 converters"
	}
}

// List returns the serial ports present on the system.
func List() ([]PortInfo, error) {
	ports, err := listPorts()
	for i := range ports {
		ports[i].identify()
	}
	return ports, err
}

// Open opens the named port.
func Open(name string, cfg Config) (Port, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	return openPort(name, cfg)
}

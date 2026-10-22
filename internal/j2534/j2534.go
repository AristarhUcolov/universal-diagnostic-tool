// Package j2534 uses SAE J2534-1 (v04.04) "PassThru" interfaces through
// the driver DLL their manufacturer installs on Windows: Tactrix OpenPort
// 2.0, Toyota Mini-VCI, Ford VCM II/III, GM MDI, Honda MVCI/DST-i,
// Mazda VCM, Chrysler wiTECH micropod, Bosch MDI, VXDIAG VCX, Scanmatik,
// Godiag GD101, Autel MaxiFlash, TOPDON RLink, Drew Technologies and
// many more. Every J2534 device installed on the computer is found in
// the registry, so the program works with all of them without
// device-specific code.
package j2534

import (
	"errors"
	"fmt"
)

// Protocol identifiers.
const (
	ProtoJ1850VPW   = 1
	ProtoJ1850PWM   = 2
	ProtoISO9141    = 3
	ProtoISO14230   = 4
	ProtoCAN        = 5
	ProtoISO15765   = 6
	ProtoSCIAEngine = 7
)

// Ioctl identifiers.
const (
	ioctlGetConfig        = 0x01
	ioctlSetConfig        = 0x02
	ioctlReadVBatt        = 0x03
	ioctlFiveBaudInit     = 0x04
	ioctlFastInit         = 0x05
	ioctlClearTxBuffer    = 0x07
	ioctlClearRxBuffer    = 0x08
	ioctlClearMsgFilters  = 0x0A
	ioctlAddToFunctLookup = 0x0C
	ioctlReadProgVoltage  = 0x0E
)

// Configuration parameters.
const (
	cfgDataRate    = 0x01
	cfgLoopback    = 0x03
	cfgP1Max       = 0x07
	cfgP3Min       = 0x0A
	cfgP4Min       = 0x0C
	cfgISO15765BS  = 0x1E
	cfgISO15765STm = 0x1F
)

// Flags.
const (
	flagISO15765FramePad  = 0x0040
	flagCAN29Bit          = 0x0100
	flagCANIDBoth         = 0x0800
	flagISO9141NoChecksum = 0x0200

	rxTxMsgType      = 0x0001
	rxStartOfMessage = 0x0002
	rxTxIndication   = 0x0008

	filterPass = 1
	filterFlow = 3
)

// Status codes.
const (
	statusNoError         = 0x00
	errTimeout            = 0x09
	errBufferEmpty        = 0x10
	errDeviceNotConnected = 0x08
)

var statusNames = map[uint32]string{
	0x01: "not supported", 0x02: "invalid channel id", 0x03: "invalid protocol id",
	0x04: "null parameter", 0x05: "invalid ioctl value", 0x06: "invalid flags", 0x07: "failed",
	0x08: "device not connected", 0x09: "timeout", 0x0A: "invalid message", 0x0B: "invalid time interval",
	0x0C: "exceeded limit", 0x0D: "invalid message id", 0x0E: "device in use", 0x0F: "invalid ioctl id",
	0x10: "buffer empty", 0x11: "buffer full", 0x12: "buffer overflow", 0x13: "pin invalid",
	0x14: "channel in use", 0x15: "message protocol id", 0x16: "invalid filter id", 0x17: "no flow control",
	0x18: "not unique", 0x19: "invalid baud rate", 0x1A: "invalid device id",
}

// Error is a J2534 status code with the driver's description.
type Error struct {
	Func   string
	Status uint32
	Detail string
}

func (e *Error) Error() string {
	name := statusNames[e.Status]
	if name == "" {
		name = fmt.Sprintf("status 0x%02X", e.Status)
	}
	if e.Detail != "" {
		return fmt.Sprintf("j2534: %s: %s (%s)", e.Func, name, e.Detail)
	}
	return fmt.Sprintf("j2534: %s: %s", e.Func, name)
}

// IsTimeout reports whether err is a J2534 timeout or empty buffer.
func IsTimeout(err error) bool {
	var e *Error
	return errors.As(err, &e) && (e.Status == errTimeout || e.Status == errBufferEmpty)
}

// Device describes an installed J2534 interface.
type Device struct {
	Name      string          `json:"name"`
	Vendor    string          `json:"vendor"`
	Library   string          `json:"library"`
	Arch      string          `json:"arch"` // x86, x64
	Usable    bool            `json:"usable"`
	Reason    string          `json:"reason,omitempty"`
	Protocols map[string]bool `json:"protocols"`
}

// ErrUnsupported is returned on systems without J2534.
var ErrUnsupported = errors.New("j2534: PassThru interfaces are only available on Windows")

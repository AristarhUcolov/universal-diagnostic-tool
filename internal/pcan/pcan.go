// Package pcan drives PEAK-System PCAN-USB / PCAN-USB FD interfaces
// through the PCAN-Basic API (PCANBasic.dll) on Windows. On Linux the
// same adapters work through SocketCAN.
package pcan

import "errors"

// ErrUnsupported is returned outside Windows.
var ErrUnsupported = errors.New("pcan: PCAN-Basic is used on Windows; on Linux use SocketCAN")

// Channels lists the PCAN-USB channel names the user can choose.
var Channels = []string{"PCAN_USBBUS1", "PCAN_USBBUS2", "PCAN_USBBUS3", "PCAN_USBBUS4",
	"PCAN_USBBUS5", "PCAN_USBBUS6", "PCAN_USBBUS7", "PCAN_USBBUS8"}

// channelHandle maps a name to the PCAN-Basic handle (0x51 = USB bus 1).
func channelHandle(name string) (uint16, bool) {
	for i, n := range Channels {
		if n == name {
			return uint16(0x51 + i), true
		}
	}
	return 0, false
}

// btr0btr1 holds the PCAN-Basic bit rate codes.
var btr0btr1 = map[int]uint16{
	1000000: 0x0014, 800000: 0x0016, 500000: 0x001C, 250000: 0x011C, 125000: 0x031C,
	100000: 0x432F, 95000: 0xC34E, 83333: 0x852B, 50000: 0x472F, 47000: 0x1414,
	33333: 0x8B2F, 20000: 0x532F, 10000: 0x672F, 5000: 0x7F7F,
}

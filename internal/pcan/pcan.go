// Package pcan drives PEAK-System PCAN-USB / PCAN-USB FD interfaces
// through the PCAN-Basic API (PCANBasic.dll) on Windows. On Linux the
// same adapters work through SocketCAN.
package pcan

import (
	"errors"
	"fmt"
)

// ErrUnsupported is returned outside Windows.
var ErrUnsupported = errors.New("pcan: PCAN-Basic is used on Windows; on Linux use SocketCAN")

// FDBitrate returns the PCAN-Basic bit timing string for CAN FD with an
// 80 MHz clock: nominal (arbitration) and data phase bit rates, both
// with an 80 % sample point.
func FDBitrate(nominal, data int) (string, error) {
	const clock = 80000000
	// The nominal bit takes 80 time quanta: 1 + 63 + 16.
	if nominal <= 0 || clock%(nominal*80) != 0 {
		return "", fmt.Errorf("pcan: CAN FD nominal bit rate %d is not supported (125, 250, 500 or 1000 kbit/s)", nominal)
	}
	nomBRP := clock / (nominal * 80)
	// The data bit takes 20 quanta (1 + 15 + 4), or 10 or 16 where 20 do
	// not divide the clock evenly.
	type timing struct{ tq, seg1, seg2 int }
	for _, d := range []timing{{20, 15, 4}, {16, 12, 3}, {10, 7, 2}} {
		if data > 0 && clock%(data*d.tq) == 0 {
			brp := clock / (data * d.tq)
			return fmt.Sprintf("f_clock_mhz=80, nom_brp=%d, nom_tseg1=63, nom_tseg2=16, nom_sjw=16, data_brp=%d, data_tseg1=%d, data_tseg2=%d, data_sjw=%d",
				nomBRP, brp, d.seg1, d.seg2, d.seg2), nil
		}
	}
	return "", fmt.Errorf("pcan: CAN FD data bit rate %d is not supported (1, 2, 4, 5 or 8 Mbit/s)", data)
}

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

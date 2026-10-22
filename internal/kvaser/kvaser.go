// Package kvaser drives Kvaser CAN interfaces (Leaf, USBcan, Memorator)
// through CANlib (canlib32.dll) on Windows. On Linux Kvaser devices work
// through SocketCAN.
package kvaser

import "errors"

// ErrUnsupported is returned outside Windows.
var ErrUnsupported = errors.New("kvaser: CANlib is used on Windows; on Linux use SocketCAN")

// predefined CANlib bit rate constants.
var bitrates = map[int]int32{
	1000000: -1, 500000: -2, 250000: -3, 125000: -4, 100000: -5, 62500: -6, 50000: -7, 83333: -8, 10000: -9,
}

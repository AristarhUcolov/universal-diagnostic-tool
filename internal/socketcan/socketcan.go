// Package socketcan uses Linux SocketCAN interfaces (can0, vcan0...):
// PEAK, Kvaser, CANable/candleLight (gs_usb), MCP2515 HATs on a
// Raspberry Pi, Intrepid and others with in-kernel drivers. The bit rate
// is set with "ip link set can0 type can bitrate 500000; ip link set can0
// up".
package socketcan

import (
	"errors"
	"net"
	"sort"
	"strings"
)

// ErrUnsupported is returned on systems without SocketCAN.
var ErrUnsupported = errors.New("socketcan: SocketCAN exists only on Linux")

// Interfaces lists CAN network interfaces.
func Interfaces() []string {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, i := range ifs {
		if strings.HasPrefix(i.Name, "can") || strings.HasPrefix(i.Name, "vcan") || strings.HasPrefix(i.Name, "slcan") {
			out = append(out, i.Name)
		}
	}
	sort.Strings(out)
	return out
}

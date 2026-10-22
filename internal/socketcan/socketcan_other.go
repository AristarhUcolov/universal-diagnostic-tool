//go:build !linux

package socketcan

import "github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"

// Open is unavailable outside Linux.
func Open(iface string) (can.Bus, error) { return nil, ErrUnsupported }

//go:build !windows

package pcan

import "github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"

// Open is unavailable outside Windows.
func Open(channel string, bitrate int) (can.Bus, error) { return nil, ErrUnsupported }

// Available reports whether PCAN-Basic can be used.
func Available() bool { return false }

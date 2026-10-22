//go:build !windows

package kvaser

import "github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"

// Open is unavailable outside Windows.
func Open(channel, bitrate int) (can.Bus, error) { return nil, ErrUnsupported }

// Channels returns 0 outside Windows.
func Channels() int { return 0 }

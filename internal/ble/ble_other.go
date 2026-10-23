//go:build !windows && !linux

package ble

import (
	"context"
	"time"
)

// Available reports whether this build can use Bluetooth LE.
func Available() bool { return false }

// Scan is unavailable in this build.
func Scan(ctx context.Context, d time.Duration) ([]Device, error) { return nil, ErrUnsupported }

// Open is unavailable in this build.
func Open(ctx context.Context, address string) (*Port, error) { return nil, ErrUnsupported }

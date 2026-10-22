//go:build !windows

package rp1210

import (
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/j1939"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
)

// OpenJ1939 is unavailable outside Windows.
func OpenJ1939(a Adapter, addr byte, log journal.Logger) (j1939.Link, error) {
	return nil, ErrUnsupported
}

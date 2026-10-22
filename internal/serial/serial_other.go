//go:build !windows && !linux && !darwin

package serial

import (
	"fmt"
	"runtime"
)

func openPort(name string, cfg Config) (Port, error) {
	return nil, fmt.Errorf("serial: ports are not supported on %s", runtime.GOOS)
}

func listPorts() ([]PortInfo, error) { return nil, nil }

//go:build linux

package serial

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func listPorts() ([]PortInfo, error) {
	entries, err := os.ReadDir("/sys/class/tty")
	if err != nil {
		return nil, err
	}
	var out []PortInfo
	for _, e := range entries {
		name := e.Name()
		if !(strings.HasPrefix(name, "ttyUSB") || strings.HasPrefix(name, "ttyACM") ||
			strings.HasPrefix(name, "rfcomm") || strings.HasPrefix(name, "ttyAMA") ||
			strings.HasPrefix(name, "ttyTHS") || strings.HasPrefix(name, "ttyXRUSB")) {
			continue
		}
		pi := PortInfo{Name: "/dev/" + name}
		dev, err := filepath.EvalSymlinks(filepath.Join("/sys/class/tty", name, "device"))
		if err == nil {
			// Walk up from the interface to the USB device directory.
			for dir, i := dev, 0; i < 5 && dir != "/"; dir, i = filepath.Dir(dir), i+1 {
				if v := readSys(dir, "idVendor"); v != "" {
					pi.VID = strings.ToUpper(v)
					pi.PID = strings.ToUpper(readSys(dir, "idProduct"))
					pi.Manufacturer = readSys(dir, "manufacturer")
					pi.Description = readSys(dir, "product")
					pi.Serial = readSys(dir, "serial")
					break
				}
			}
		}
		if strings.HasPrefix(name, "rfcomm") && pi.Description == "" {
			pi.Description = "Bluetooth serial (rfcomm)"
		}
		out = append(out, pi)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func readSys(dir, file string) string {
	b, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

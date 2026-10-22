// Package rp1210 uses TMC RP1210 adapters, the Windows API of the truck
// and heavy equipment world: Nexiq USB-Link, Noregon DLA+, Dearborn DPA
// 4/5/XL, Cummins INLINE, CAT Communication Adapter, Jaltest Link, TEXA,
// Volvo VOCOM, DAF/MAN/Iveco tools and others. Installed adapters are
// listed in C:\Windows\RP121032.INI.
package rp1210

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Command numbers of RP1210_SendCommand (RP1210C).
const (
	cmdSetAllFiltersPass = 3
	cmdProtectJ1939Addr  = 19
	cmdReleaseJ1939Addr  = 31
)

// Adapter is one device of an installed RP1210 implementation.
type Adapter struct {
	Implementation string   `json:"implementation"` // DLL / INI base name
	Vendor         string   `json:"vendor"`
	DeviceID       int      `json:"deviceId"`
	Description    string   `json:"description"`
	Protocols      []string `json:"protocols"`
}

// ErrUnsupported is returned outside Windows.
var ErrUnsupported = errors.New("rp1210: RP1210 adapters are only available on Windows")

// ListAdapters reads RP121032.INI and the vendor INI files.
func ListAdapters() ([]Adapter, error) {
	win := os.Getenv("WINDIR")
	if win == "" {
		win = `C:\Windows`
	}
	main, err := readINI(filepath.Join(win, "RP121032.INI"))
	if err != nil {
		return nil, nil
	}
	var out []Adapter
	for _, impl := range strings.Split(main["rp1210support"]["apiimplementations"], ",") {
		impl = strings.TrimSpace(impl)
		if impl == "" {
			continue
		}
		ini, err := readINI(filepath.Join(win, impl+".INI"))
		if err != nil {
			continue
		}
		vendor := ini["vendorinformation"]["name"]
		protos := map[int][]string{}
		for sec, kv := range ini {
			if !strings.HasPrefix(sec, "protocolinformation") {
				continue
			}
			name := kv["protocolstring"]
			for _, d := range strings.Split(kv["devices"], ",") {
				if id, err := strconv.Atoi(strings.TrimSpace(d)); err == nil {
					protos[id] = append(protos[id], name)
				}
			}
		}
		for sec, kv := range ini {
			if !strings.HasPrefix(sec, "deviceinformation") {
				continue
			}
			id, err := strconv.Atoi(kv["deviceid"])
			if err != nil {
				continue
			}
			desc := kv["devicedescription"]
			if desc == "" {
				desc = kv["devicename"]
			}
			out = append(out, Adapter{Implementation: impl, Vendor: vendor, DeviceID: id, Description: desc, Protocols: protos[id]})
		}
	}
	return out, nil
}

// readINI parses a Windows INI file into lower-case sections and keys.
func readINI(path string) (map[string]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]map[string]string{}
	sec := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			sec = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if out[sec] == nil {
				out[sec] = map[string]string{}
			}
			continue
		}
		if i := strings.IndexByte(line, '='); i > 0 && sec != "" {
			out[sec][strings.ToLower(strings.TrimSpace(line[:i]))] = strings.TrimSpace(line[i+1:])
		}
	}
	return out, sc.Err()
}

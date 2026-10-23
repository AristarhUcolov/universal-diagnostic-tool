// Package settings stores the user's preferences and keeps the program's
// data folder: "udt-data" next to the program, so the program and
// everything it saved travel together (a flash drive, a workshop laptop).
// Where that folder cannot be written, the user profile is used
// (%APPDATA%\udt on Windows, ~/.config/udt on Linux); -data or UDT_DATA
// choose any folder. See location.go.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/vehicle"
)

// Settings are the persisted preferences.
type Settings struct {
	Language   string              `json:"language"`   // "" = system, en, ru
	Theme      string              `json:"theme"`      // auto, dark, light
	Units      string              `json:"units"`      // metric, imperial
	Expert     bool                `json:"expert"`     // show raw/flash tools
	LogToFile  bool                `json:"logToFile"`  // journal to a file in logs/
	BridgeAddr string              `json:"bridgeAddr"` // ELM327 bridge listen address
	Last       vehicle.Config      `json:"last"`       // last connection
	LiveParams map[string][]string `json:"liveParams"` // selected parameters per control unit description
	Gauges     map[string][]string `json:"gauges"`     // parameters on the gauges page per control unit description
	Window     bool                `json:"window"`     // open a desktop window (Windows) instead of the browser
	Alerts     bool                `json:"alerts"`     // warn about dangerous live values
	AlertSound bool                `json:"alertSound"` // with a sound
}

var mu sync.Mutex

// Defaults returns the initial settings.
func Defaults() *Settings {
	return &Settings{Theme: "auto", Units: "metric", LogToFile: true, BridgeAddr: "0.0.0.0:35000", Window: true,
		Alerts: true, AlertSound: true,
		Last: vehicle.Config{Adapter: vehicle.AdapterSim, Profile: vehicle.ProfileRuAuto}, LiveParams: map[string][]string{},
		Gauges: map[string][]string{}}
}

// Load reads the settings (defaults when the file does not exist).
func Load() *Settings {
	mu.Lock()
	defer mu.Unlock()
	s := Defaults()
	data, err := os.ReadFile(filepath.Join(Dir(), "settings.json"))
	if err == nil {
		_ = json.Unmarshal(data, s)
	}
	if s.LiveParams == nil {
		s.LiveParams = map[string][]string{}
	}
	if s.Gauges == nil {
		s.Gauges = map[string][]string{}
	}
	return s
}

// Save writes the settings atomically.
func (s *Settings) Save() error {
	mu.Lock()
	defer mu.Unlock()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(Dir(), "settings.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(Dir(), "settings.json"))
}

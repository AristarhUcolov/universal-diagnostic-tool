package settings

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Where the data folder is.
const (
	ModePortable = "portable" // "udt-data" next to the program
	ModeProfile  = "profile"  // the user profile (%APPDATA%\udt, ~/.config/udt)
	ModeCustom   = "custom"   // chosen with -data or UDT_DATA
)

// PortableName is the data folder created next to the program.
const PortableName = "udt-data"

// Location tells where the data folder is and why.
type Location struct {
	Dir  string `json:"dir"`
	Mode string `json:"mode"`
	// Note explains why the user profile is used instead of the folder
	// next to the program.
	Note string `json:"note,omitempty"`
	// Copied names the folder whose settings and journals were copied
	// into a new data folder.
	Copied string `json:"copied,omitempty"`
}

var (
	locMu sync.Mutex
	loc   *Location // chosen by Init; without Init (tests) it is resolved on every call
)

// Init chooses the data folder for this run: custom when it is not
// empty, else the UDT_DATA environment variable, else "udt-data" next to
// the program, else the user profile when the program's folder cannot be
// written (Program Files, read-only media). A new folder next to the
// program receives a copy of the settings and journals kept in the user
// profile, so nothing is lost when the program becomes portable.
func Init(custom string) Location {
	l := resolve(custom, executable(), os.Getenv("UDT_DATA"), profileDir())
	if l.Mode == ModePortable {
		l.Copied = copyData(profileDir(), l.Dir)
	}
	locMu.Lock()
	loc = &l
	locMu.Unlock()
	return l
}

// Current returns the data folder in use.
func Current() Location {
	locMu.Lock()
	defer locMu.Unlock()
	if loc != nil {
		return *loc
	}
	return resolve("", executable(), os.Getenv("UDT_DATA"), profileDir())
}

// Dir returns the data folder, creating it.
func Dir() string {
	d := Current().Dir
	_ = os.MkdirAll(d, 0o755)
	return d
}

// Sub returns a sub-folder of the data folder (logs, recordings, reports,
// definitions), creating it.
func Sub(name string) string {
	d := filepath.Join(Dir(), name)
	_ = os.MkdirAll(d, 0o755)
	return d
}

// resolve picks the data folder. exe is the program's path ("" when
// unknown); custom and env are the folders asked for with -data and
// UDT_DATA.
func resolve(custom, exe, env, profile string) Location {
	if custom == "" {
		custom = env
	}
	if custom != "" {
		if abs, err := filepath.Abs(custom); err == nil {
			custom = abs
		}
		return Location{Dir: custom, Mode: ModeCustom}
	}
	switch {
	case exe == "":
		return Location{Dir: profile, Mode: ModeProfile, Note: "the program's folder is unknown"}
	case strings.Contains(filepath.ToSlash(exe), "/go-build"):
		// go run and go test build into a temporary folder that is
		// deleted afterwards.
		return Location{Dir: profile, Mode: ModeProfile}
	}
	d := filepath.Join(filepath.Dir(exe), PortableName)
	if err := writable(d); err != nil {
		return Location{Dir: profile, Mode: ModeProfile, Note: "the program's folder cannot be written: " + err.Error()}
	}
	return Location{Dir: d, Mode: ModePortable}
}

// executable returns the program's real path, or "" when it is unknown.
func executable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe
}

// profileDir is the data folder in the user profile.
func profileDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "udt")
}

// writable creates d when needed and checks that files can be written
// there.
func writable(d string) error {
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(d, ".write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// copyData copies the settings and the logs, recordings, reports and
// user descriptions from an old data folder into a new one that has no
// settings yet. The old folder is left as it is. It returns from when
// anything was copied.
func copyData(from, to string) string {
	if from == "" || filepath.Clean(from) == filepath.Clean(to) {
		return ""
	}
	if _, err := os.Stat(filepath.Join(to, "settings.json")); err == nil {
		return "" // the new folder is already in use
	}
	if _, err := os.Stat(filepath.Join(from, "settings.json")); err != nil {
		return ""
	}
	if copyFile(filepath.Join(from, "settings.json"), filepath.Join(to, "settings.json")) != nil {
		return ""
	}
	for _, sub := range []string{"logs", "recordings", "reports", "definitions"} {
		root := filepath.Join(from, sub)
		_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(from, p)
			if err != nil {
				return nil
			}
			_ = copyFile(p, filepath.Join(to, rel))
			return nil
		})
	}
	return from
}

// copyFile copies one file, keeping its modification time; an existing
// target is left alone.
func copyFile(from, to string) error {
	if _, err := os.Stat(to); err == nil {
		return nil
	}
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	dst, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(to)
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}

package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	tmp := t.TempDir()
	prof := filepath.Join(tmp, "profile")
	tools := filepath.Join(tmp, "Tools")
	if err := os.MkdirAll(tools, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(tools, "udt.exe")

	if l := resolve("", exe, "", prof); l.Mode != ModePortable || l.Dir != filepath.Join(tools, PortableName) {
		t.Errorf("next to the program: %+v", l)
	}
	// -data wins over UDT_DATA, and both over the folder next to the program.
	if l := resolve(filepath.Join(tmp, "a"), exe, filepath.Join(tmp, "b"), prof); l.Mode != ModeCustom || l.Dir != filepath.Join(tmp, "a") {
		t.Errorf("-data: %+v", l)
	}
	if l := resolve("", exe, filepath.Join(tmp, "b"), prof); l.Mode != ModeCustom || l.Dir != filepath.Join(tmp, "b") {
		t.Errorf("UDT_DATA: %+v", l)
	}
	// go run and go test build into a temporary folder.
	if l := resolve("", filepath.Join(tmp, "go-build123", "b001", "exe", "udt.exe"), "", prof); l.Mode != ModeProfile || l.Dir != prof {
		t.Errorf("go run: %+v", l)
	}
	if l := resolve("", "", "", prof); l.Mode != ModeProfile || l.Note == "" {
		t.Errorf("unknown program folder: %+v", l)
	}
	// A folder that cannot be written (here a file stands in the way).
	blocked := filepath.Join(tmp, "Blocked")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, PortableName), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if l := resolve("", filepath.Join(blocked, "udt.exe"), "", prof); l.Mode != ModeProfile || !strings.Contains(l.Note, "cannot be written") {
		t.Errorf("unwritable folder: %+v", l)
	}
}

func TestCopyData(t *testing.T) {
	old := t.TempDir()
	put := func(rel, content string) string {
		p := filepath.Join(old, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	put("settings.json", `{"theme":"dark"}`)
	journal := put(filepath.Join("logs", "udt-20261022-115739.jsonl"), "{}\n")
	put(filepath.Join("definitions", "my-unit.json"), "{}")
	put(filepath.Join("webview", "EBWebView", "cache.bin"), "cache")
	stamp := time.Date(2026, 10, 22, 11, 57, 39, 0, time.Local)
	if err := os.Chtimes(journal, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	to := filepath.Join(t.TempDir(), PortableName)
	if got := copyData(old, to); got != old {
		t.Fatalf("copyData = %q, want %q", got, old)
	}
	if b, err := os.ReadFile(filepath.Join(to, "settings.json")); err != nil || string(b) != `{"theme":"dark"}` {
		t.Errorf("settings: %q, %v", b, err)
	}
	info, err := os.Stat(filepath.Join(to, "logs", "udt-20261022-115739.jsonl"))
	if err != nil || !info.ModTime().Equal(stamp) {
		t.Errorf("journal not copied with its time: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(to, "definitions", "my-unit.json")); err != nil {
		t.Errorf("user description not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(to, "webview")); err == nil {
		t.Error("the WebView2 cache was copied")
	}
	if _, err := os.Stat(journal); err != nil {
		t.Error("the old folder was changed")
	}
	// Only a new folder receives a copy.
	if got := copyData(old, to); got != "" {
		t.Errorf("second copy: %q", got)
	}
	if got := copyData(t.TempDir(), filepath.Join(t.TempDir(), PortableName)); got != "" {
		t.Errorf("copy from a folder without settings: %q", got)
	}
}

func TestSaveLoadInDataFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	t.Setenv("UDT_DATA", dir)
	// Nothing saved yet: the first start, with the update check on.
	if s := Load(); !s.Fresh() || !s.Updates {
		t.Errorf("first start: fresh %v, updates %v", s.Fresh(), s.Updates)
	}
	s := Defaults()
	s.Theme = "dark"
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if Load().Fresh() {
		t.Error("a start with saved settings is taken for the first one")
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatalf("settings not in the data folder: %v", err)
	}
	if got := Load().Theme; got != "dark" {
		t.Errorf("loaded theme %q", got)
	}
	if got := Sub("logs"); got != filepath.Join(dir, "logs") {
		t.Errorf("Sub = %q", got)
	}
}

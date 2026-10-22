//go:build !windows

package desktop

// SystemLanguage reads the language from the environment.
func SystemLanguage() string { return envLanguage() }

// HideOwnConsole does nothing outside Windows.
func HideOwnConsole() {}

// RunWindow is not available; the caller opens the browser.
func RunWindow(url, title, dataDir string, onOpen func()) bool { return false }

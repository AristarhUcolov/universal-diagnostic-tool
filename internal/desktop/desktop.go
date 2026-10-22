// Package desktop shows the user interface: in a native window (WebView2
// on Windows) when possible, otherwise in the default browser.
package desktop

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OpenBrowser opens url in the default browser.
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// envLanguage reads the language from LANG / LC_ALL.
func envLanguage() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return strings.ToLower(v)
		}
	}
	return ""
}

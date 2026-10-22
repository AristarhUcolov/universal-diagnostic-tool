//go:build windows

package desktop

import (
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

var (
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGetUILanguage     = kernel32.NewProc("GetUserDefaultUILanguage")
	procGetConsoleProcess = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole       = kernel32.NewProc("FreeConsole")
)

// SystemLanguage returns the Windows display language ("ru", "en"...).
func SystemLanguage() string {
	if procGetUILanguage.Find() != nil {
		return envLanguage()
	}
	r, _, _ := procGetUILanguage.Call()
	switch uint16(r) & 0x3FF { // primary language id
	case 0x19:
		return "ru"
	case 0x22:
		return "uk"
	case 0x23:
		return "be"
	case 0x3F:
		return "kk"
	}
	return "en"
}

// HideOwnConsole closes the console window when the program was started
// by double-clicking it (the console then belongs to this process only).
// Started from a terminal, the console stays.
func HideOwnConsole() {
	if procGetConsoleProcess.Find() != nil {
		return
	}
	var pids [4]uint32
	n, _, _ := procGetConsoleProcess.Call(uintptr(unsafe.Pointer(&pids[0])), 4)
	if n == 1 {
		procFreeConsole.Call()
	}
}

// RunWindow shows url in a WebView2 window and blocks until it is
// closed. onOpen runs once the window exists. It returns false when the
// WebView2 runtime is missing.
func RunWindow(url, title, dataDir string, onOpen func()) bool {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  dataDir,
		WindowOptions: webview2.WindowOptions{
			Title:  title,
			Width:  1280,
			Height: 860,
			Center: true,
		},
	})
	if w == nil {
		return false
	}
	defer w.Destroy()
	w.SetSize(1024, 680, webview2.HintMin)
	w.Navigate(url)
	if onOpen != nil {
		onOpen()
	}
	w.Run()
	return true
}

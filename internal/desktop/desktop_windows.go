//go:build windows

package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

var (
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	user32                = windows.NewLazySystemDLL("user32.dll")
	shell32               = windows.NewLazySystemDLL("shell32.dll")
	procGetUILanguage     = kernel32.NewProc("GetUserDefaultUILanguage")
	procGetConsoleProcess = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole       = kernel32.NewProc("FreeConsole")
	procGetDpiForSystem   = user32.NewProc("GetDpiForSystem")
	procGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	procShowWindow        = user32.NewProc("ShowWindow")
	procGetPlacement      = user32.NewProc("GetWindowPlacement")
	procSetPlacement      = user32.NewProc("SetWindowPlacement")
	procMonitorFromRect   = user32.NewProc("MonitorFromRect")
	procCallWindowProc    = user32.NewProc("CallWindowProcW")
	procSetWindowLongPtr  = user32.NewProc("SetWindowLongPtrW")
	procSetWindowLong     = user32.NewProc("SetWindowLongW") // 32-bit Windows has no ...Ptr
	procSetAppID          = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
)

// appID groups the program's window and its pinned taskbar icon as one
// application, whichever folder the program runs from.
const appID = "AristarhUcolov.UniversalDiagnosticTool"

const (
	swShowNormal    = 1
	swShowMinimized = 2
	swShowMaximized = 3
	wmClose         = 0x0010
	wpfMaximized    = 0x2         // WPF_RESTORETOMAXIMIZED
	gwlpWndProc     = ^uintptr(3) // GWLP_WNDPROC, -4
	smCxScreen      = 0
	smCyScreen      = 1
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
// closed. dataDir keeps the browser engine's profile and the window's
// place on the screen. onOpen runs once the window exists. It returns
// false when the WebView2 runtime is missing.
func RunWindow(url, title, dataDir string, onOpen func()) bool {
	if procSetAppID.Find() == nil {
		id, _ := windows.UTF16PtrFromString(appID)
		procSetAppID.Call(uintptr(unsafe.Pointer(id)))
	}
	scale := dpiScale()
	width, height := fit(1280*scale, smCxScreen), fit(860*scale, smCyScreen)
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  filepath.Join(dataDir, "webview"),
		WindowOptions: webview2.WindowOptions{
			Title:  title,
			Width:  uint(width),
			Height: uint(height),
			IconId: 1, // from rsrc_windows_*.syso
			Center: true,
		},
	})
	if w == nil {
		return false
	}
	defer w.Destroy()
	w.SetSize(int(1024*scale), int(640*scale), webview2.HintMin)
	pl := &placement{hwnd: uintptr(w.Window()), path: filepath.Join(dataDir, "window.json")}
	pl.restore()
	pl.watchClose()
	// The page names the open section in the title bar and the taskbar.
	_ = w.Bind("udtTitle", func(t string) { w.SetTitle(t) })
	w.Navigate(url)
	if onOpen != nil {
		onOpen()
	}
	w.Run()
	return true
}

// dpiScale is the screen scale (1.5 at 150 %); window sizes are given
// for 100 %.
func dpiScale() float64 {
	if procGetDpiForSystem.Find() != nil {
		return 1
	}
	if dpi, _, _ := procGetDpiForSystem.Call(); dpi >= 96 {
		return float64(dpi) / 96
	}
	return 1
}

// fit keeps a window side within 90 % of the screen.
func fit(size float64, metric uintptr) float64 {
	if screen, _, _ := procGetSystemMetrics.Call(metric); screen > 0 && size > float64(screen)*0.9 {
		return float64(screen) * 0.9
	}
	return size
}

// placement keeps the window where the user left it - size, position and
// whether it was maximized - in window.json of the data folder. The first
// start opens maximized: diagnostics shows a lot of numbers at once.
type placement struct {
	hwnd uintptr
	path string
	orig uintptr
}

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

// windowPlacement is WINDOWPLACEMENT.
type windowPlacement struct {
	Length   uint32
	Flags    uint32
	ShowCmd  uint32
	PtMin    point
	PtMax    point
	RcNormal rect
}

type savedPlace struct {
	X, Y, W, H int32
	Maximized  bool
}

func (p *placement) restore() {
	var s savedPlace
	data, err := os.ReadFile(p.path)
	if err != nil || json.Unmarshal(data, &s) != nil || s.W < 400 || s.H < 300 {
		procShowWindow.Call(p.hwnd, swShowMaximized)
		return
	}
	rc := rect{s.X, s.Y, s.X + s.W, s.Y + s.H}
	// The screen it was on may have been unplugged since: then the window
	// stays in the middle of the main screen.
	if mon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&rc)), 0); mon == 0 {
		if s.Maximized {
			procShowWindow.Call(p.hwnd, swShowMaximized)
		}
		return
	}
	wp := windowPlacement{ShowCmd: swShowNormal, RcNormal: rc}
	if s.Maximized {
		wp.ShowCmd = swShowMaximized
	}
	wp.Length = uint32(unsafe.Sizeof(wp))
	procSetPlacement.Call(p.hwnd, uintptr(unsafe.Pointer(&wp)))
}

func (p *placement) save() {
	var wp windowPlacement
	wp.Length = uint32(unsafe.Sizeof(wp))
	if r, _, _ := procGetPlacement.Call(p.hwnd, uintptr(unsafe.Pointer(&wp))); r == 0 {
		return
	}
	r := wp.RcNormal
	s := savedPlace{X: r.Left, Y: r.Top, W: r.Right - r.Left, H: r.Bottom - r.Top,
		Maximized: wp.ShowCmd == swShowMaximized || wp.ShowCmd == swShowMinimized && wp.Flags&wpfMaximized != 0}
	if data, err := json.Marshal(s); err == nil {
		_ = os.WriteFile(p.path, data, 0o644)
	}
}

// window is the program's one window, for its close handler.
var window *placement

// watchClose saves the place when the window is being closed, while it
// still exists.
func (p *placement) watchClose() {
	window = p
	set := procSetWindowLongPtr
	if set.Find() != nil {
		set = procSetWindowLong
	}
	p.orig, _, _ = set.Call(p.hwnd, gwlpWndProc, windows.NewCallback(closeProc))
}

func closeProc(hwnd, msg, wp, lp uintptr) uintptr {
	if msg == wmClose {
		window.save()
	}
	r, _, _ := procCallWindowProc.Call(window.orig, hwnd, msg, wp, lp)
	return r
}

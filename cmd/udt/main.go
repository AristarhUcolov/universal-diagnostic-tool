// Command udt is the Universal Diagnostic Tool: vehicle diagnostics for
// cars, trucks and machinery with almost any adapter. Without arguments
// it opens the graphical interface; subcommands give command-line access
// (run "udt help").
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/desktop"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/server"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/settings"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/version"
)

func main() {
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		os.Exit(runCommand(os.Args[1], os.Args[2:]))
	}
	os.Exit(runGUI(os.Args[1:]))
}

func runGUI(args []string) int {
	fs := flag.NewFlagSet("udt", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:0", "address of the web interface; 0.0.0.0:8080 allows phones and tablets on the network")
	noWindow := fs.Bool("no-window", false, "do not open a window or a browser (server only)")
	browser := fs.Bool("browser", false, "open the interface in the browser instead of a window")
	data := fs.String("data", "", "folder for settings, journals, recordings and reports (default: "+settings.PortableName+" next to the program)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s %s\n\nUsage: udt [flags]   (graphical interface)\n       udt <command> [flags]   (run \"udt help\")\n\nFlags:\n", version.Name, version.Version)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	loc := settings.Init(*data)
	st := settings.Load()
	j := journal.New(20000)
	if st.LogToFile {
		logPath := filepath.Join(settings.Sub("logs"), "udt-"+time.Now().Format("20060102-150405")+".jsonl")
		if err := j.OpenFile(logPath); err != nil {
			fmt.Fprintln(os.Stderr, "journal file:", err)
		}
		pruneLogs(settings.Sub("logs"), 50)
	}
	journal.Infof(j, "app", "%s %s started", version.Name, version.Version)
	journal.Infof(j, "app", "data folder: %s (%s)", loc.Dir, loc.Mode)
	if loc.Note != "" {
		journal.Warnf(j, "app", "data folder in the user profile: %s", loc.Note)
	}
	if loc.Copied != "" {
		journal.Infof(j, "app", "settings and journals copied from %s", loc.Copied)
	}
	srv, err := server.New(st, j)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	url, err := srv.Listen(*listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot start the interface:", err)
		return 1
	}
	fmt.Printf("%s %s\nInterface: %s\nData: %s\n", version.Name, version.Version, url, loc.Dir)
	if lan := srv.LANURL(); lan != "" {
		fmt.Printf("Other devices on the network: %s\n", lan)
	}
	defer srv.Close()

	if !*noWindow {
		if st.Window && !*browser {
			go func() {
				// Ctrl+C in the console also ends the program.
				sig := make(chan os.Signal, 1)
				signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
				<-sig
				srv.Close()
				os.Exit(0)
			}()
			// The console window is only hidden once the program's own
			// window is open, so a failing WebView2 never leaves a
			// server running invisibly.
			if desktop.RunWindow(url, version.Name, filepath.Join(settings.Dir(), "webview"), desktop.HideOwnConsole) {
				return 0 // the window was closed
			}
			fmt.Println("WebView2 is not available; opening the browser.")
		}
		if err := desktop.OpenBrowser(url); err != nil {
			fmt.Println("Open this address in your browser:", url)
		}
	}
	fmt.Println("Press Ctrl+C to stop.")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	return 0
}

// pruneLogs keeps the newest n journal files.
func pruneLogs(dir string, n int) {
	files, _ := filepath.Glob(filepath.Join(dir, "udt-*.jsonl"))
	if len(files) <= n {
		return
	}
	// Names sort by time.
	for _, f := range files[:len(files)-n] {
		_ = os.Remove(f)
	}
}

// signalContext returns a context cancelled by Ctrl+C.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

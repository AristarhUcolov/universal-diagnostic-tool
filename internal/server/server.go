// Package server serves the user interface and its JSON API on a local
// HTTP port. The interface is a single page embedded in the executable,
// so the program works offline in a garage.
package server

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/bridge"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/settings"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/update"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/vehicle"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/version"
)

//go:embed web
var webFS embed.FS

// Server is the HTTP server of the user interface.
type Server struct {
	Token    string
	settings *settings.Settings
	journal  *journal.Journal
	mgr      *manager
	srv      *http.Server
	ln       net.Listener
	index    *template.Template
	lan      bool

	statusMu   sync.Mutex
	statusSubs map[chan struct{}]bool

	bridgeMu sync.Mutex
	bridge   *bridge.Server

	updates  *update.Checker
	firstRun bool // the program starts for the first time
}

// New creates the server.
func New(st *settings.Settings, j *journal.Journal) (*Server, error) {
	tok := make([]byte, 16)
	if _, err := rand.Read(tok); err != nil {
		return nil, err
	}
	s := &Server{Token: hex.EncodeToString(tok), settings: st, journal: j, statusSubs: map[chan struct{}]bool{},
		updates: &update.Checker{Current: version.Version}, firstRun: st.Fresh()}
	s.mgr = newManager(j, s.notifyStatus)
	idx, err := fs.ReadFile(webFS, "web/index.html")
	if err != nil {
		return nil, err
	}
	s.index, err = template.New("index").Delims("[[", "]]").Parse(string(idx))
	if err != nil {
		return nil, err
	}
	// User definitions extend the built-in ones.
	ecu.Builtin().LoadDir(settings.Sub("definitions"))
	return s, nil
}

// Listen starts serving on addr ("127.0.0.1:0" picks a free port) and
// returns the URL of the interface.
func (s *Server) Listen(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	s.ln = ln
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	s.lan = host != "127.0.0.1" && host != "::1" && host != "localhost"
	mux := http.NewServeMux()
	s.routes(mux)
	s.srv = &http.Server{Handler: s.middleware(mux), ReadHeaderTimeout: 10 * time.Second}
	go s.srv.Serve(ln)
	return fmt.Sprintf("http://127.0.0.1:%s/", port), nil
}

// LANURL returns the address for other devices when listening on all
// interfaces (with the access token in the fragment).
func (s *Server) LANURL() string {
	if !s.lan {
		return ""
	}
	_, port, _ := net.SplitHostPort(s.ln.Addr().String())
	ip := localIP()
	if ip == "" {
		return ""
	}
	return fmt.Sprintf("http://%s:%s/#t=%s", ip, port, s.Token)
}

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil && n.IP.IsPrivate() {
			return n.IP.String()
		}
	}
	return ""
}

// Close stops everything.
func (s *Server) Close() error {
	s.mgr.disconnect()
	s.stopBridge()
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// middleware protects the API: the Host header must be a local name
// (DNS rebinding) and API calls must carry the session token, which
// only pages served by this program know.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		local := host == "127.0.0.1" || host == "localhost" || host == "::1" || host == "[::1]"
		if !local && !s.lan {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/health" {
			tok := r.Header.Get("X-UDT-Token")
			if tok == "" {
				tok = r.URL.Query().Get("t")
			}
			if tok != s.Token {
				http.Error(w, "missing or wrong access token", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	tok := ""
	if isLoopback(r) {
		tok = s.Token
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
	_ = s.index.Execute(w, map[string]string{"Token": tok, "Version": version.Version})
}

func (s *Server) routes(mux *http.ServeMux) {
	static, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(static))
	mux.HandleFunc("GET /{$}", s.serveIndex)
	mux.HandleFunc("GET /index.html", s.serveIndex)
	mux.Handle("GET /assets/", http.StripPrefix("/", noCache(files)))

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "version": version.Version, "definitionErrors": ecu.Builtin().Errors()})
	})
	mux.HandleFunc("GET /api/info", s.apiInfo)
	mux.HandleFunc("GET /api/ports", s.apiPorts)
	mux.HandleFunc("GET /api/adapters", s.apiAdapters)
	mux.HandleFunc("GET /api/definitions", s.apiDefinitions)
	mux.HandleFunc("GET /api/settings", s.apiGetSettings)
	mux.HandleFunc("POST /api/settings", s.apiSetSettings)
	mux.HandleFunc("POST /api/connect", s.apiConnect)
	mux.HandleFunc("POST /api/disconnect", s.apiDisconnect)
	mux.HandleFunc("GET /api/status", s.apiStatus)
	mux.HandleFunc("GET /api/voltage", s.apiVoltage)
	mux.HandleFunc("GET /api/ecu/{id}/info", s.apiIdentify)
	mux.HandleFunc("GET /api/ecu/{id}/dtc", s.apiDTC)
	mux.HandleFunc("POST /api/ecu/{id}/dtc/clear", s.apiClearDTC)
	mux.HandleFunc("GET /api/ecu/{id}/freeze", s.apiFreeze)
	mux.HandleFunc("GET /api/ecu/{id}/readiness", s.apiReadiness)
	mux.HandleFunc("GET /api/ecu/{id}/tests", s.apiTests)
	mux.HandleFunc("GET /api/ble/scan", s.apiBLEScan)
	mux.HandleFunc("GET /api/ecu/{id}/params", s.apiParams)
	mux.HandleFunc("POST /api/ecu/{id}/read", s.apiRead)
	mux.HandleFunc("GET /api/ecu/{id}/actuators", s.apiActuators)
	mux.HandleFunc("POST /api/ecu/{id}/actuate", s.apiActuate)
	mux.HandleFunc("POST /api/ecu/{id}/reset", s.apiReset)
	mux.HandleFunc("POST /api/ecu/{id}/raw", s.apiRaw)
	mux.HandleFunc("GET /api/ecu/{id}/blocks", s.apiBlocks)
	mux.HandleFunc("POST /api/ecu/{id}/block", s.apiBlock)
	mux.HandleFunc("POST /api/live/start", s.apiLiveStart)
	mux.HandleFunc("POST /api/live/stop", s.apiLiveStop)
	mux.HandleFunc("POST /api/record/start", s.apiRecordStart)
	mux.HandleFunc("POST /api/record/stop", s.apiRecordStop)
	mux.HandleFunc("POST /api/monitor/start", s.apiMonitorStart)
	mux.HandleFunc("POST /api/monitor/stop", s.apiMonitorStop)
	mux.HandleFunc("POST /api/monitor/send", s.apiMonitorSend)
	mux.HandleFunc("GET /api/dtc/search", s.apiDTCSearch)
	mux.HandleFunc("GET /api/repair", s.apiRepair)
	mux.HandleFunc("GET /api/journal", s.apiJournal)
	mux.HandleFunc("GET /api/journal/export.csv", s.apiJournalCSV)
	mux.HandleFunc("POST /api/journal/clear", s.apiJournalClear)
	mux.HandleFunc("GET /api/events", s.apiEvents)
	mux.HandleFunc("GET /api/report", s.apiReport)
	mux.HandleFunc("POST /api/bridge/start", s.apiBridgeStart)
	mux.HandleFunc("POST /api/bridge/stop", s.apiBridgeStop)
	mux.HandleFunc("POST /api/sim/control", s.apiSimControl)
	mux.HandleFunc("POST /api/open-folder", s.apiOpenFolder)
	mux.HandleFunc("POST /api/open-url", s.apiOpenURL)
	mux.HandleFunc("GET /api/update", s.apiUpdate)
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func writeError(w http.ResponseWriter, status int, err error) {
	// The interface explains coded errors in the user's language.
	e := apiError{Error: err.Error(), Code: vehicle.ErrorCode(err)}
	var ce *ecu.ConditionError
	if errors.As(err, &ce) {
		status = http.StatusConflict
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(e)
}

func readJSON(r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("expected application/json")
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}

// notifyStatus wakes the event streams after a state change.
func (s *Server) notifyStatus() {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	for ch := range s.statusSubs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// openFolder returns the folder for logs and recordings.
func openFolderPath(kind string) string {
	switch kind {
	case "logs", "recordings", "reports", "definitions":
		return settings.Sub(kind)
	}
	return filepath.Clean(settings.Dir())
}

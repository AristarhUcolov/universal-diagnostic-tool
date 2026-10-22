package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/settings"
)

// newTestServer starts a server on a free loopback port. The configuration
// directory points into a temporary folder so tests never touch the
// user's real settings.
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	s, err := New(settings.Defaults(), journal.New(1000))
	if err != nil {
		t.Fatal(err)
	}
	base, err := s.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, strings.TrimSuffix(base, "/")
}

func do(t *testing.T, method, url, host, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	if token != "" {
		req.Header.Set("X-UDT-Token", token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func TestAccessControl(t *testing.T) {
	s, base := newTestServer(t)
	cases := []struct {
		name, path, host, token string
		want                    int
	}{
		{"health needs no token", "/api/health", "", "", http.StatusOK},
		{"api without token", "/api/status", "", "", http.StatusUnauthorized},
		{"api with a wrong token", "/api/status", "", "0123456789abcdef0123456789abcdef", http.StatusUnauthorized},
		{"api with the token", "/api/status", "", s.Token, http.StatusOK},
		{"token in the query", "/api/status?t=" + s.Token, "", "", http.StatusOK},
		{"foreign host (DNS rebinding)", "/api/health", "evil.example:80", "", http.StatusForbidden},
		{"foreign host with the token", "/api/status", "evil.example", s.Token, http.StatusForbidden},
		{"localhost name", "/api/health", "localhost:1234", "", http.StatusOK},
		{"page", "/", "", "", http.StatusOK},
		{"assets", "/assets/app.css", "", "", http.StatusOK},
	}
	for _, c := range cases {
		got, _ := do(t, "GET", base+c.path, c.host, c.token, "")
		if got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
}

func TestIndexToken(t *testing.T) {
	s, base := newTestServer(t)
	// A loopback client (this program's own window) gets the token.
	code, body := do(t, "GET", base+"/", "", "", "")
	if code != http.StatusOK || !strings.Contains(body, s.Token) {
		t.Fatalf("loopback page: status %d, token present %v", code, strings.Contains(body, s.Token))
	}
	// A device on the network must not: it has to bring the token from
	// the link shown in the program.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.50:51000"
	rec := httptest.NewRecorder()
	s.serveIndex(rec, req)
	if strings.Contains(rec.Body.String(), s.Token) {
		t.Fatal("page served to a network client contains the access token")
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("page has no Content-Security-Policy")
	}
}

// TestOpenURL checks that the program window can open only its own
// pages in the system browser: the URL goes to the shell, so anything
// else could start a program.
func TestOpenURL(t *testing.T) {
	s, _ := newTestServer(t)
	var opened []string
	old := openExternal
	openExternal = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openExternal = old })
	cases := []struct {
		url, from string
		want      int
	}{
		{"https://ko-fi.com/AristarhUcolov", "127.0.0.1:50000", http.StatusOK},
		{"https://buymeacoffee.com/aristarh.ucolov", "127.0.0.1:50000", http.StatusOK},
		{"https://www.donationalerts.com/c/aristarh_ucolov", "127.0.0.1:50000", http.StatusOK},
		{"https://github.com/AristarhUcolov/universal-diagnostic-tool/issues", "127.0.0.1:50000", http.StatusOK},
		{"https://ko-fi.com/AristarhUcolov.evil.example", "127.0.0.1:50000", http.StatusForbidden},
		{"https://evil.example/", "127.0.0.1:50000", http.StatusForbidden},
		{"file:///C:/Windows/System32/calc.exe", "127.0.0.1:50000", http.StatusForbidden},
		{"https://github.com/AristarhUcolov/universal-diagnostic-tool/\" & calc", "127.0.0.1:50000", http.StatusForbidden},
		{"https://ko-fi.com/AristarhUcolov", "192.168.1.50:50000", http.StatusForbidden},
	}
	for _, c := range cases {
		body, _ := json.Marshal(map[string]string{"url": c.url})
		req := httptest.NewRequest("POST", "/api/open-url", strings.NewReader(string(body)))
		req.RemoteAddr = c.from
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.apiOpenURL(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s from %s: status %d, want %d", c.url, c.from, rec.Code, c.want)
		}
	}
	if len(opened) != 4 {
		t.Errorf("opened %v, want the 4 allowed links", opened)
	}
}

// TestServiceEndpoints covers the service page and the repair advice:
// an output test refused with a reason code, the restart and the advice
// attached to trouble codes.
func TestServiceEndpoints(t *testing.T) {
	s, base := newTestServer(t)
	tok := s.Token
	if code, body := do(t, "POST", base+"/api/connect", "", tok, `{"adapter":"sim","profile":"ru-kwp","simVariant":"vaz-m154"}`); code != http.StatusOK {
		t.Fatalf("connect: %d %s", code, body)
	}
	deadline := time.Now().Add(40 * time.Second)
	for {
		_, body := do(t, "GET", base+"/api/status", "", tok, "")
		if strings.Contains(body, `"state":"connected"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not connected: %s", body)
		}
		time.Sleep(200 * time.Millisecond)
	}

	code, body := do(t, "GET", base+"/api/repair", "", tok, "")
	var rep struct {
		Hints []struct {
			ID string `json:"id"`
		} `json:"hints"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &rep) != nil || len(rep.Hints) < 20 {
		t.Fatalf("repair advice: %d, %d hints", code, len(rep.Hints))
	}

	code, body = do(t, "GET", base+"/api/ecu/e1/dtc", "", tok, "")
	var codes struct {
		Codes []struct {
			Code string `json:"code"`
			Hint string `json:"hint"`
		} `json:"codes"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &codes) != nil || len(codes.Codes) == 0 {
		t.Fatalf("dtc: %d %s", code, body)
	}
	for _, c := range codes.Codes {
		if c.Hint == "" {
			t.Errorf("%s has no repair advice", c.Code)
		}
	}

	// The simulated engine runs: an injector pulse must be refused with
	// a code the interface can explain.
	code, body = do(t, "POST", base+"/api/ecu/e1/actuate", "", tok, `{"id":"inj1","action":"pulse"}`)
	if code != http.StatusConflict || !strings.Contains(body, `"code":"engine_off"`) {
		t.Fatalf("injector pulse with the engine running: %d %s", code, body)
	}

	if code, body := do(t, "POST", base+"/api/ecu/e1/reset", "", tok, `{}`); code != http.StatusOK {
		t.Fatalf("reset: %d %s", code, body)
	}
	if code, body := do(t, "GET", base+"/api/ecu/e1/dtc", "", tok, ""); code != http.StatusOK {
		t.Fatalf("dtc after the restart: %d %s", code, body)
	}
}

// TestSimulatedSession drives the API like the interface does: connect to
// the simulated GAZ-3110 (Микас-7.1), read the trouble codes, poll values
// and build the report.
func TestSimulatedSession(t *testing.T) {
	s, base := newTestServer(t)
	tok := s.Token
	if code, body := do(t, "POST", base+"/api/connect", "", tok, `{"adapter":"sim","profile":"ru-auto","simVariant":"gaz-mikas"}`); code != http.StatusOK {
		t.Fatalf("connect: %d %s", code, body)
	}
	var status struct {
		State string `json:"state"`
		Error string `json:"error"`
		ECUs  []struct {
			ID string `json:"id"`
		} `json:"ecus"`
	}
	deadline := time.Now().Add(40 * time.Second)
	for {
		_, body := do(t, "GET", base+"/api/status", "", tok, "")
		if err := json.Unmarshal([]byte(body), &status); err != nil {
			t.Fatal(err)
		}
		if status.State == "connected" || status.State == "error" || time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if status.State != "connected" || len(status.ECUs) == 0 {
		t.Fatalf("state %q (%s), %d control units", status.State, status.Error, len(status.ECUs))
	}
	id := status.ECUs[0].ID

	code, body := do(t, "GET", base+"/api/ecu/"+id+"/dtc", "", tok, "")
	if code != http.StatusOK {
		t.Fatalf("dtc: %d %s", code, body)
	}
	var codes struct {
		Codes []struct {
			Code  string `json:"code"`
			Known bool   `json:"known"`
		} `json:"codes"`
	}
	if err := json.Unmarshal([]byte(body), &codes); err != nil {
		t.Fatal(err)
	}
	if len(codes.Codes) == 0 {
		t.Fatalf("the simulated car stores trouble codes, got none: %s", body)
	}
	for _, c := range codes.Codes {
		if !c.Known {
			t.Errorf("code %s has no description", c.Code)
		}
	}

	code, body = do(t, "POST", base+"/api/ecu/"+id+"/read", "", tok, `{"ids":["freq","twat"]}`)
	var read struct {
		Values []struct {
			ID string  `json:"id"`
			V  float64 `json:"v"`
			OK bool    `json:"ok"`
		} `json:"values"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &read) != nil || len(read.Values) != 2 {
		t.Fatalf("read: %d %s", code, body)
	}
	for _, v := range read.Values {
		if !v.OK {
			t.Errorf("value %s not read", v.ID)
		}
	}
	if rpm := read.Values[0].V; rpm < 500 || rpm > 7000 {
		t.Errorf("engine speed %v rpm is not plausible for a running engine", rpm)
	}

	code, body = do(t, "GET", base+"/api/report?t="+tok, "", "", "")
	if code != http.StatusOK || !strings.Contains(body, codes.Codes[0].Code) {
		t.Fatalf("report: status %d, contains %s: %v", code, codes.Codes[0].Code, strings.Contains(body, codes.Codes[0].Code))
	}

	if code, body := do(t, "POST", base+"/api/disconnect", "", tok, `{}`); code != http.StatusOK {
		t.Fatalf("disconnect: %d %s", code, body)
	}
}

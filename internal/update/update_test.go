package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.4.0", "0.3.0", true},
		{"0.3.1", "0.3.0-dev", true},
		{"0.3.0", "0.3.0-dev", false},
		{"0.3.0", "0.3.0", false},
		{"0.2.9", "0.3.0", false},
		{"1.0", "0.9.9", true},
		{"0.10.0", "0.9.0", true},
		{"v0.4.0", "0.3.0", true},
		{"0.3.0.1", "0.3.0", true},
		{"", "0.3.0", false},
		{"0.3.0", "dev", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestCheck(t *testing.T) {
	var calls atomic.Int32
	tag := `{"tag_name":"v0.4.0","html_url":"https://example.com/elsewhere"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("User-Agent") == "" {
			t.Error("no User-Agent: GitHub refuses such requests")
		}
		_, _ = w.Write([]byte(tag))
	}))
	defer srv.Close()

	c := &Checker{Current: "0.3.0", API: srv.URL}
	r := c.Check(context.Background(), false)
	if !r.Available || r.Latest != "0.4.0" || r.Error != "" {
		t.Fatalf("result %+v", r)
	}
	// The link is built here, never taken from the answer.
	if r.URL != Repo+"/releases/tag/v0.4.0" {
		t.Errorf("url %q", r.URL)
	}
	c.Check(context.Background(), false)
	if calls.Load() != 1 {
		t.Errorf("asked %d times, the answer should be cached", calls.Load())
	}
	c.Check(context.Background(), true)
	if calls.Load() != 2 {
		t.Errorf("a forced check should ask again")
	}

	tag = `{"tag_name":"v0.4.0; rm -rf"}`
	if r := c.Check(context.Background(), true); r.Error == "" || r.Available {
		t.Errorf("odd tag accepted: %+v", r)
	}
}

func TestCheckFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	c := &Checker{Current: "0.3.0", API: srv.URL}
	if r := c.Check(context.Background(), false); r.Error == "" || r.Available {
		t.Errorf("result %+v", r)
	}

	srv.Close()
	if r := c.Check(context.Background(), false); r.Error == "" {
		t.Errorf("no error without a server: %+v", r)
	}
}

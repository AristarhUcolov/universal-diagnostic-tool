// Package update asks GitHub whether a newer release of the program is
// out. It never downloads or replaces anything: the user decides when to
// update, and the interface only shows where to get the new version.
// Without internet the answer is simply "unknown".
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Repo is the project's page; releases are under it.
const Repo = "https://github.com/AristarhUcolov/universal-diagnostic-tool"

const latestAPI = "https://api.github.com/repos/AristarhUcolov/universal-diagnostic-tool/releases/latest"

// cacheFor is how long an answer is kept: opening the interface again
// does not ask GitHub every time.
const cacheFor = 6 * time.Hour

// Result is what the interface shows.
type Result struct {
	Current   string    `json:"current"`
	Latest    string    `json:"latest,omitempty"`
	Available bool      `json:"available"`
	URL       string    `json:"url,omitempty"`
	Checked   time.Time `json:"checked"`
	Error     string    `json:"error,omitempty"`
}

// Checker asks for the latest release and keeps the answer.
type Checker struct {
	Current string
	API     string       // the releases API address; empty means GitHub
	Client  *http.Client // nil means a client with a short timeout

	mu   sync.Mutex
	last Result
}

// Check returns the cached answer, or asks GitHub when there is none, it
// is old or failed, or force is set.
func (c *Checker) Check(ctx context.Context, force bool) Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.last.Error == "" && !c.last.Checked.IsZero() && time.Since(c.last.Checked) < cacheFor {
		return c.last
	}
	c.last = c.fetch(ctx)
	return c.last
}

var tagRe = regexp.MustCompile(`^v?\d+(\.\d+){0,3}$`)

func (c *Checker) fetch(ctx context.Context) Result {
	res := Result{Current: c.Current, Checked: time.Now()}
	api := c.API
	if api == "" {
		api = latestAPI
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "universal-diagnostic-tool/"+c.Current)
	resp, err := client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		res.Error = "GitHub: " + resp.Status
		return res
	}
	var rel struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		res.Error = err.Error()
		return res
	}
	if !tagRe.MatchString(rel.Tag) {
		res.Error = fmt.Sprintf("unexpected release tag %q", rel.Tag)
		return res
	}
	res.Latest = strings.TrimPrefix(rel.Tag, "v")
	res.URL = Repo + "/releases/tag/v" + res.Latest
	res.Available = Newer(res.Latest, c.Current)
	return res
}

// Newer reports whether version a is later than b. Only the numbers
// count: "0.3.0-dev" (a build between releases) equals "0.3.0".
func Newer(a, b string) bool {
	pa, pb := parts(a), parts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func parts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

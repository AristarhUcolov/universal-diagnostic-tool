// Package journal records everything that happens on the diagnostic
// link: raw bytes and frames in both directions, protocol events and
// errors. Entries go to an in-memory ring (for the user interface), to
// live subscribers (the bus monitor) and optionally to a JSON-lines file
// so that a session can be reviewed or shared later.
package journal

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
)

// Kind classifies an entry.
type Kind string

const (
	TX    Kind = "tx"    // bytes or a message sent to the vehicle
	RX    Kind = "rx"    // bytes or a message received from the vehicle
	Info  Kind = "info"  // protocol event (init done, ECU found...)
	Warn  Kind = "warn"  // something unexpected but recoverable
	Error Kind = "error" // an operation failed
)

// Entry is one journal record.
type Entry struct {
	Seq   uint64    `json:"seq"`
	Time  time.Time `json:"time"`
	Kind  Kind      `json:"kind"`
	Layer string    `json:"layer"`          // serial, kline, can, isotp, kwp, uds, obd, elm, app...
	Addr  string    `json:"addr,omitempty"` // address or CAN id when relevant
	Data  []byte    `json:"-"`
	Hex   string    `json:"hex,omitempty"`
	Text  string    `json:"text,omitempty"`
}

// Logger receives entries. Every transport and protocol layer accepts a
// Logger; nil is allowed and means "discard".
type Logger interface {
	Log(Entry)
}

type discard struct{}

func (discard) Log(Entry) {}

// Discard is a Logger that drops everything.
var Discard Logger = discard{}

// Or returns l, or Discard when l is nil.
func Or(l Logger) Logger {
	if l == nil {
		return Discard
	}
	return l
}

// Tx logs bytes sent at the given layer.
func Tx(l Logger, layer, addr string, data []byte, text string) {
	if l == nil {
		return
	}
	l.Log(Entry{Kind: TX, Layer: layer, Addr: addr, Data: data, Text: text})
}

// Rx logs bytes received at the given layer.
func Rx(l Logger, layer, addr string, data []byte, text string) {
	if l == nil {
		return
	}
	l.Log(Entry{Kind: RX, Layer: layer, Addr: addr, Data: data, Text: text})
}

// Infof logs an informational event.
func Infof(l Logger, layer, format string, args ...any) {
	if l == nil {
		return
	}
	l.Log(Entry{Kind: Info, Layer: layer, Text: fmt.Sprintf(format, args...)})
}

// Warnf logs a recoverable problem.
func Warnf(l Logger, layer, format string, args ...any) {
	if l == nil {
		return
	}
	l.Log(Entry{Kind: Warn, Layer: layer, Text: fmt.Sprintf(format, args...)})
}

// Errorf logs a failure.
func Errorf(l Logger, layer, format string, args ...any) {
	if l == nil {
		return
	}
	l.Log(Entry{Kind: Error, Layer: layer, Text: fmt.Sprintf(format, args...)})
}

// Journal is the concrete store used by the application.
type Journal struct {
	mu      sync.Mutex
	seq     uint64
	ring    []Entry
	start   int // index of the oldest entry
	count   int
	subs    map[int]chan Entry
	nextSub int
	dropped uint64
	file    *os.File
	enc     *json.Encoder
	path    string
}

// New creates a journal that keeps the last capacity entries in memory.
func New(capacity int) *Journal {
	if capacity < 16 {
		capacity = 16
	}
	return &Journal{ring: make([]Entry, capacity), subs: map[int]chan Entry{}}
}

// Log stores the entry, assigning its sequence number and time.
func (j *Journal) Log(e Entry) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if len(e.Data) > 0 {
		e.Data = append([]byte(nil), e.Data...)
		if e.Hex == "" {
			e.Hex = hexutil.Format(e.Data)
		}
	}
	j.mu.Lock()
	j.seq++
	e.Seq = j.seq
	idx := (j.start + j.count) % len(j.ring)
	if j.count < len(j.ring) {
		j.count++
	} else {
		j.start = (j.start + 1) % len(j.ring)
	}
	j.ring[idx] = e
	for _, ch := range j.subs {
		select {
		case ch <- e:
		default:
			j.dropped++
		}
	}
	if j.enc != nil {
		_ = j.enc.Encode(e)
	}
	j.mu.Unlock()
}

// Since returns up to limit entries with a sequence number above seq,
// oldest first. limit <= 0 means no limit.
func (j *Journal) Since(seq uint64, limit int) []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []Entry
	for i := 0; i < j.count; i++ {
		e := j.ring[(j.start+i)%len(j.ring)]
		if e.Seq > seq {
			out = append(out, e)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// Last returns the sequence number of the newest entry.
func (j *Journal) Last() uint64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.seq
}

// Subscribe returns a channel that receives every new entry. Slow
// subscribers lose entries rather than blocking the vehicle link.
func (j *Journal) Subscribe(buffer int) (int, <-chan Entry) {
	if buffer < 1 {
		buffer = 256
	}
	ch := make(chan Entry, buffer)
	j.mu.Lock()
	j.nextSub++
	id := j.nextSub
	j.subs[id] = ch
	j.mu.Unlock()
	return id, ch
}

// Unsubscribe stops delivery to the subscription and closes its channel.
func (j *Journal) Unsubscribe(id int) {
	j.mu.Lock()
	if ch, ok := j.subs[id]; ok {
		delete(j.subs, id)
		close(ch)
	}
	j.mu.Unlock()
}

// Dropped reports how many entries slow subscribers missed.
func (j *Journal) Dropped() uint64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.dropped
}

// Clear empties the in-memory ring (the file, if any, is kept).
func (j *Journal) Clear() {
	j.mu.Lock()
	j.start, j.count = 0, 0
	j.mu.Unlock()
}

// OpenFile starts appending entries to a JSON-lines file.
func (j *Journal) OpenFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	j.mu.Lock()
	if j.file != nil {
		j.file.Close()
	}
	j.file, j.enc, j.path = f, json.NewEncoder(f), path
	j.mu.Unlock()
	return nil
}

// FilePath returns the current log file, if any.
func (j *Journal) FilePath() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.path
}

// CloseFile stops writing to the log file.
func (j *Journal) CloseFile() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.file == nil {
		return nil
	}
	err := j.file.Close()
	j.file, j.enc, j.path = nil, nil, ""
	return err
}

// WriteCSV exports the in-memory entries as CSV.
func (j *Journal) WriteCSV(w io.Writer) error {
	entries := j.Since(0, 0)
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"seq", "time", "kind", "layer", "addr", "hex", "text"}); err != nil {
		return err
	}
	for _, e := range entries {
		rec := []string{
			strconv.FormatUint(e.Seq, 10),
			e.Time.Format("2006-01-02 15:04:05.000"),
			string(e.Kind), e.Layer, e.Addr, e.Hex, e.Text,
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// Prefixed wraps a logger and fills in the layer when it is empty.
type Prefixed struct {
	L     Logger
	Layer string
}

// Log implements Logger.
func (p Prefixed) Log(e Entry) {
	if e.Layer == "" {
		e.Layer = p.Layer
	}
	Or(p.L).Log(e)
}

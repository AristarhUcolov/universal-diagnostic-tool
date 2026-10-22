package vehicle

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/sim"
)

// ECUEntry is one control unit of a session.
type ECUEntry struct {
	ID      string          `json:"id"`
	Desc    ecu.Description `json:"desc"`
	handler ecu.Handler
}

// Handler returns the control unit handler.
func (e *ECUEntry) Handler() ecu.Handler { return e.handler }

// Event is pushed to subscribers (the web interface).
type Event struct {
	Type   string      `json:"type"` // status, live, can, progress
	ECU    string      `json:"ecu,omitempty"`
	Values []ecu.Value `json:"values,omitempty"`
	Text   string      `json:"text,omitempty"`
	Time   time.Time   `json:"time"`
	Data   any         `json:"data,omitempty"`
}

// closer releases an adapter.
type closer interface{ Close() error }

// Session is a connection to a vehicle.
type Session struct {
	Config  Config
	Adapter string // adapter description
	Started time.Time

	log     journal.Logger
	mu      sync.Mutex
	ecus    []*ECUEntry
	closers []closer
	mux     *can.Mux
	world   *sim.World
	voltage func(context.Context) (float64, error)

	subMu  sync.Mutex
	subs   map[int]chan Event
	nextID int

	liveMu     sync.Mutex
	liveCancel context.CancelFunc
	liveDone   chan struct{}
	liveCfg    LiveConfig
	rec        *recorder

	kaCancel context.CancelFunc
	kaDone   chan struct{}
	lastUse  time.Time
	useMu    sync.Mutex

	monCancel context.CancelFunc
}

// ECUs returns the control units found.
func (s *Session) ECUs() []*ECUEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*ECUEntry(nil), s.ecus...)
}

// ECU returns a control unit by id ("" = first).
func (s *Session) ECU(id string) (*ECUEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ecus) == 0 {
		return nil, errors.New("no control unit")
	}
	if id == "" {
		return s.ecus[0], nil
	}
	for _, e := range s.ecus {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, fmt.Errorf("unknown control unit %q", id)
}

func (s *Session) addECU(h ecu.Handler) *ECUEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := &ECUEntry{ID: fmt.Sprintf("e%d", len(s.ecus)+1), Desc: h.Describe(), handler: h}
	s.ecus = append(s.ecus, e)
	journal.Infof(s.log, "app", "control unit %s: %s (%s, %s)", e.ID, e.Desc.Name, e.Desc.Protocol, e.Desc.Status)
	return e
}

// Touch records diagnostic activity (keep-alive is skipped while busy).
func (s *Session) Touch() {
	s.useMu.Lock()
	s.lastUse = time.Now()
	s.useMu.Unlock()
}

func (s *Session) idleFor() time.Duration {
	s.useMu.Lock()
	defer s.useMu.Unlock()
	return time.Since(s.lastUse)
}

// Bus returns the CAN mux for the bus monitor (nil for non-CAN adapters).
func (s *Session) Bus() *can.Mux { return s.mux }

// World returns the simulator (nil for real vehicles).
func (s *Session) World() *sim.World { return s.world }

// Voltage reads the supply voltage when the adapter can measure it.
func (s *Session) Voltage(ctx context.Context) (float64, error) {
	if s.voltage == nil {
		return 0, errors.New("the adapter cannot measure voltage")
	}
	return s.voltage(ctx)
}

// Subscribe receives session events.
func (s *Session) Subscribe() (int, <-chan Event) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if s.subs == nil {
		s.subs = map[int]chan Event{}
	}
	s.nextID++
	ch := make(chan Event, 256)
	s.subs[s.nextID] = ch
	return s.nextID, ch
}

// Unsubscribe stops event delivery.
func (s *Session) Unsubscribe(id int) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if ch, ok := s.subs[id]; ok {
		delete(s.subs, id)
		close(ch)
	}
}

func (s *Session) emit(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// startKeepAlive keeps sessions open while the user is reading results.
func (s *Session) startKeepAlive() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.kaCancel, s.kaDone = cancel, done
	go func() {
		defer close(done)
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			if s.idleFor() < 2*time.Second {
				continue
			}
			for _, e := range s.ECUs() {
				kctx, kcancel := context.WithTimeout(ctx, 3*time.Second)
				if err := e.handler.KeepAlive(kctx); err != nil && ctx.Err() == nil {
					journal.Warnf(s.log, "app", "keep-alive for %s failed: %v", e.ID, err)
				}
				kcancel()
			}
			s.Touch()
		}
	}()
}

// LiveConfig selects parameters to poll.
type LiveConfig struct {
	ECU      string   `json:"ecu"`
	Params   []string `json:"params"`
	Interval int      `json:"intervalMs"`
}

// StartLive starts polling parameters.
func (s *Session) StartLive(cfg LiveConfig) error {
	e, err := s.ECU(cfg.ECU)
	if err != nil {
		return err
	}
	s.StopLive()
	ctx, cancel := context.WithCancel(context.Background())
	s.liveMu.Lock()
	s.liveCancel, s.liveDone, s.liveCfg = cancel, make(chan struct{}), cfg
	done := s.liveDone
	s.liveMu.Unlock()
	interval := time.Duration(cfg.Interval) * time.Millisecond
	go func() {
		defer close(done)
		fails := 0
		for ctx.Err() == nil {
			start := time.Now()
			rctx, rcancel := context.WithTimeout(ctx, 10*time.Second)
			vals, err := e.handler.Read(rctx, cfg.Params)
			rcancel()
			s.Touch()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				fails++
				s.emit(Event{Type: "live", ECU: e.ID, Text: err.Error()})
				if fails > 5 {
					journal.Errorf(s.log, "app", "live data stopped after repeated errors: %v", err)
					s.emit(Event{Type: "status", Text: "live-stopped"})
					return
				}
				time.Sleep(500 * time.Millisecond)
				continue
			}
			fails = 0
			s.emit(Event{Type: "live", ECU: e.ID, Values: vals})
			s.liveMu.Lock()
			if s.rec != nil {
				s.rec.write(vals)
			}
			s.liveMu.Unlock()
			if wait := interval - time.Since(start); wait > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
				}
			}
		}
	}()
	return nil
}

// StopLive stops polling.
func (s *Session) StopLive() {
	s.liveMu.Lock()
	cancel, done := s.liveCancel, s.liveDone
	s.liveCancel, s.liveDone = nil, nil
	s.liveMu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

// LiveRunning reports the active live configuration.
func (s *Session) LiveRunning() (LiveConfig, bool) {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	return s.liveCfg, s.liveCancel != nil
}

type recorder struct {
	f    *os.File
	w    *csv.Writer
	ids  []string
	path string
	n    int
}

func (r *recorder) write(vals []ecu.Value) {
	if r.ids == nil {
		r.ids = []string{"time"}
		for _, v := range vals {
			r.ids = append(r.ids, v.ID)
		}
		_ = r.w.Write(r.ids)
	}
	rec := []string{time.Now().Format("2006-01-02 15:04:05.000")}
	for _, v := range vals {
		switch {
		case !v.OK:
			rec = append(rec, "")
		case v.Text != "":
			rec = append(rec, v.Text)
		default:
			rec = append(rec, strconv.FormatFloat(v.V, 'f', -1, 64))
		}
	}
	_ = r.w.Write(rec)
	r.n++
	if r.n%20 == 0 {
		r.w.Flush()
	}
}

// StartRecording writes live values to a CSV file.
func (s *Session) StartRecording(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if s.rec != nil {
		s.rec.w.Flush()
		s.rec.f.Close()
	}
	s.rec = &recorder{f: f, w: csv.NewWriter(f), path: path}
	journal.Infof(s.log, "app", "recording live data to %s", path)
	return nil
}

// StopRecording closes the CSV file and returns its path.
func (s *Session) StopRecording() string {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if s.rec == nil {
		return ""
	}
	s.rec.w.Flush()
	s.rec.f.Close()
	p := s.rec.path
	s.rec = nil
	return p
}

// Close ends the session and releases the adapter.
func (s *Session) Close() error {
	s.StopLive()
	s.StopMonitor()
	s.StopRecording()
	// Background requests must be over before the handlers say goodbye
	// to their control units: on a K-Line a closing message sent in the
	// middle of another exchange would garble both.
	if s.kaCancel != nil {
		s.kaCancel()
		<-s.kaDone
		s.kaCancel = nil
	}
	for _, e := range s.ECUs() {
		_ = e.handler.Close()
	}
	if s.mux != nil {
		_ = s.mux.Close()
	}
	for i := len(s.closers) - 1; i >= 0; i-- {
		_ = s.closers[i].Close()
	}
	if s.world != nil {
		s.world.Close()
	}
	s.subMu.Lock()
	for id, ch := range s.subs {
		close(ch)
		delete(s.subs, id)
	}
	s.subMu.Unlock()
	journal.Infof(s.log, "app", "disconnected")
	return nil
}

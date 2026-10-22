package vehicle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/j1939"
)

// MonitorEntry summarises one CAN identifier for the bus monitor.
type MonitorEntry struct {
	ID      string  `json:"id"`
	Ext     bool    `json:"ext"`
	Data    string  `json:"data"`
	Len     int     `json:"len"`
	Count   int     `json:"count"`
	Period  float64 `json:"periodMs"`
	Changed uint64  `json:"changed"` // bit i set = byte i changed recently
	PGN     uint32  `json:"pgn,omitempty"`
	Source  string  `json:"source,omitempty"`
	ASCII   string  `json:"ascii,omitempty"`
}

type monState struct {
	MonitorEntry
	last    time.Time
	prev    []byte
	dirty   bool
	changes [64]time.Time
}

// StartMonitor starts the CAN bus monitor; snapshots of changed
// identifiers are sent as "can" events four times a second.
func (s *Session) StartMonitor() error {
	if s.mux == nil {
		return errors.New("the bus monitor needs a CAN adapter")
	}
	s.StopMonitor()
	ctx, cancel := context.WithCancel(context.Background())
	s.monCancel = cancel
	sub := s.mux.Subscribe(nil, 8192)
	var mu sync.Mutex
	state := map[string]*monState{}
	go func() {
		defer sub.Close()
		for ctx.Err() == nil {
			f, err := sub.Recv(ctx, 200*time.Millisecond)
			if err != nil {
				if errors.Is(err, can.ErrTimeout) {
					continue
				}
				return
			}
			key := f.IDText()
			data := f.Payload()
			mu.Lock()
			st := state[key]
			now := time.Now()
			if st == nil {
				st = &monState{MonitorEntry: MonitorEntry{ID: key, Ext: f.Ext}}
				if f.Ext {
					pgn, _, src, _ := j1939.Decode(f.ID)
					st.PGN, st.Source = pgn, j1939.SourceName(src)
				}
				state[key] = st
			} else if !st.last.IsZero() {
				p := float64(now.Sub(st.last).Microseconds()) / 1000
				if st.Period == 0 {
					st.Period = p
				} else {
					st.Period = st.Period*0.9 + p*0.1
				}
			}
			for i := range data {
				if i < len(st.prev) && st.prev[i] != data[i] && i < 64 {
					st.changes[i] = now
				}
			}
			st.prev = append(st.prev[:0], data...)
			st.Data, st.Len, st.ASCII = hexutil.Format(data), len(data), hexutil.Printable(data)
			st.Count++
			st.last = now
			st.dirty = true
			mu.Unlock()
		}
	}()
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			mu.Lock()
			var out []MonitorEntry
			now := time.Now()
			for _, st := range state {
				if !st.dirty {
					continue
				}
				st.dirty = false
				var mask uint64
				for i := 0; i < st.Len && i < 64; i++ {
					if now.Sub(st.changes[i]) < 2*time.Second {
						mask |= 1 << i
					}
				}
				st.Changed = mask
				out = append(out, st.MonitorEntry)
			}
			mu.Unlock()
			if len(out) > 0 {
				sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
				s.emit(Event{Type: "can", Data: out})
			}
		}
	}()
	return nil
}

// StopMonitor stops the bus monitor.
func (s *Session) StopMonitor() {
	if s.monCancel != nil {
		s.monCancel()
		s.monCancel = nil
	}
}

// SendFrame transmits a raw CAN frame (expert function of the monitor).
func (s *Session) SendFrame(ctx context.Context, text string) error {
	if s.mux == nil {
		return errors.New("no CAN adapter")
	}
	f, err := can.ParseFrame(text)
	if err != nil {
		return err
	}
	if err := s.mux.Send(ctx, f); err != nil {
		return fmt.Errorf("send failed: %w", err)
	}
	return nil
}

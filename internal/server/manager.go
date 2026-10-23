package server

import (
	"context"
	"sync"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/vehicle"
)

// Connection states.
const (
	StateIdle       = "idle"
	StateConnecting = "connecting"
	StateConnected  = "connected"
	StateError      = "error"
)

// manager owns the current vehicle session.
type manager struct {
	mu       sync.Mutex
	sess     *vehicle.Session
	state    string
	err      string
	errCode  string // see vehicle.ErrorCode
	progress []string
	cancel   context.CancelFunc
	log      *journal.Journal
	onChange func()
}

func newManager(log *journal.Journal, onChange func()) *manager {
	return &manager{state: StateIdle, log: log, onChange: onChange}
}

func (m *manager) session() *vehicle.Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sess
}

func (m *manager) changed() {
	if m.onChange != nil {
		m.onChange()
	}
}

// connect starts connecting in the background.
func (m *manager) connect(cfg vehicle.Config, done func(*vehicle.Session)) {
	m.disconnect()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	m.mu.Lock()
	m.state, m.err, m.errCode, m.progress, m.cancel = StateConnecting, "", "", nil, cancel
	m.mu.Unlock()
	m.changed()
	go func() {
		defer cancel()
		s, err := vehicle.Connect(ctx, cfg, m.log, func(step string) {
			m.mu.Lock()
			m.progress = append(m.progress, step)
			if len(m.progress) > 50 {
				m.progress = m.progress[len(m.progress)-50:]
			}
			m.mu.Unlock()
			m.changed()
		})
		m.mu.Lock()
		if ctx.Err() == context.Canceled {
			m.mu.Unlock()
			if s != nil {
				s.Close()
			}
			return
		}
		if err != nil {
			m.state, m.err, m.errCode = StateError, err.Error(), vehicle.ErrorCode(err)
		} else {
			m.state, m.sess = StateConnected, s
		}
		m.cancel = nil
		m.mu.Unlock()
		if s != nil && done != nil {
			done(s)
		}
		m.changed()
	}()
}

// disconnect closes the session or aborts a connection attempt.
func (m *manager) disconnect() {
	m.mu.Lock()
	s, cancel := m.sess, m.cancel
	m.sess, m.cancel = nil, nil
	m.state, m.err, m.errCode = StateIdle, "", ""
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if s != nil {
		s.Close()
	}
	m.changed()
}

func (m *manager) status() (state, err, code string, progress []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, m.err, m.errCode, append([]string(nil), m.progress...)
}

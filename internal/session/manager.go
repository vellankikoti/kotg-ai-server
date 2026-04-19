// Package session is the in-memory session manager backing the kotg-ai-server
// Chat service. State is bounded by TTL eviction and hard caps; nothing is
// persisted. All sessions are lost when the process restarts (by design —
// the supervisor's idle-shutdown wipes them anyway, and the desktop chat
// panel handles the spawn_changed reset cleanly).
package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
)

const (
	DefaultTTL                   = 15 * time.Minute
	DefaultMaxSessions           = 1000
	DefaultMaxMessagesPerSession = 100
	DefaultReaperInterval        = 60 * time.Second
)

type Config struct {
	TTL                   time.Duration
	MaxSessions           int
	MaxMessagesPerSession int
	ReaperInterval        time.Duration
}

func (c Config) withDefaults() Config {
	if c.TTL <= 0 {
		c.TTL = DefaultTTL
	}
	if c.MaxSessions <= 0 {
		c.MaxSessions = DefaultMaxSessions
	}
	if c.MaxMessagesPerSession <= 0 {
		c.MaxMessagesPerSession = DefaultMaxMessagesPerSession
	}
	if c.ReaperInterval <= 0 {
		c.ReaperInterval = DefaultReaperInterval
	}
	return c
}

// Session is the in-memory record. Messages excludes the system prompt
// (rebuilt per turn from the cluster ID).
type Session struct {
	ID             string
	FocusClusterID string
	Title          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Messages       []provider.Message
	activeCancel   func()
}

var ErrCapExceeded = errors.New("session: max sessions exceeded")
var ErrNotFound = errors.New("session: not found")

type Manager struct {
	cfg     Config
	mu      sync.Mutex
	by      map[string]*Session
	stop    chan struct{}
	stopped bool
}

func New(cfg Config) *Manager {
	m := &Manager{
		cfg:  cfg.withDefaults(),
		by:   make(map[string]*Session),
		stop: make(chan struct{}),
	}
	go m.reaperLoop()
	return m
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	m.stopped = true
	close(m.stop)
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (m *Manager) Create(focusClusterID, title string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.by) >= m.cfg.MaxSessions {
		return nil, fmt.Errorf("%w (max %d)", ErrCapExceeded, m.cfg.MaxSessions)
	}
	now := time.Now()
	s := &Session{
		ID:             newID(),
		FocusClusterID: focusClusterID,
		Title:          title,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	m.by[s.ID] = s
	return s, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.by[id]
	if !ok {
		return nil, false
	}
	out := *s
	out.Messages = append([]provider.Message{}, s.Messages...)
	return &out, true
}

func (m *Manager) Append(id string, msg provider.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.by[id]
	if !ok {
		return ErrNotFound
	}
	s.Messages = append(s.Messages, msg)
	if over := len(s.Messages) - m.cfg.MaxMessagesPerSession; over > 0 {
		s.Messages = s.Messages[over:]
	}
	s.UpdatedAt = time.Now()
	return nil
}

func (m *Manager) SetTurnCancel(id string, cancel func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.by[id]
	if !ok {
		return
	}
	s.activeCancel = cancel
}

// CancelTurn fires the registered cancel func once and clears it.
// Calling on a session with no active turn is a no-op.
func (m *Manager) CancelTurn(id string) error {
	m.mu.Lock()
	s, ok := m.by[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	cancel := s.activeCancel
	s.activeCancel = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (m *Manager) List(limit int, sinceUnix int64) []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Session, 0, len(m.by))
	for _, s := range m.by {
		if sinceUnix > 0 && s.UpdatedAt.Unix() <= sinceUnix {
			continue
		}
		copy := *s
		copy.Messages = nil
		out = append(out, &copy)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (m *Manager) reaperLoop() {
	t := time.NewTicker(m.cfg.ReaperInterval)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
			m.evictExpired()
		}
	}
}

func (m *Manager) evictExpired() {
	cutoff := time.Now().Add(-m.cfg.TTL)
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.by {
		if s.UpdatedAt.Before(cutoff) {
			delete(m.by, id)
		}
	}
}

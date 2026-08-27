// Package session implements the Preview Session state machine
// (spec.md ss9). The Preview Server owns a single global session: Start /
// Pause / Resume / Stop toggles whether Collectors are told to forward
// Shadow Traffic at all (spec.md ss32).
package session

import (
	"errors"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

type State int

const (
	StateIdle State = iota
	StateStreaming
	StatePaused
)

func (s State) String() string {
	switch s {
	case StateStreaming:
		return "STREAMING"
	case StatePaused:
		return "PAUSED"
	default:
		return "IDLE"
	}
}

var (
	ErrNotIdle    = errors.New("session: not idle")
	ErrNotStream  = errors.New("session: not streaming")
	ErrNotPaused  = errors.New("session: not paused")
	ErrNoneActive = errors.New("session: no active session")
)

// Snapshot is a point-in-time, read-only view of the session (spec.md ss9
// UI panel).
type Snapshot struct {
	State     State
	SessionID string
	StartedAt time.Time
}

// Manager owns the current Preview Session and notifies listeners
// (typically the collector ConnectionManager and the browser broadcaster)
// on every transition.
type Manager struct {
	mu        sync.Mutex
	state     State
	sessionID string
	startedAt time.Time

	listeners []func(Snapshot)
}

func NewManager() *Manager {
	return &Manager{state: StateIdle}
}

// OnChange registers a callback invoked (synchronously, under lock-free
// context) after every state transition.
func (m *Manager) OnChange(fn func(Snapshot)) {
	m.mu.Lock()
	m.listeners = append(m.listeners, fn)
	m.mu.Unlock()
}

// Start begins a new session from IDLE (spec.md ss9).
func (m *Manager) Start() (Snapshot, error) {
	m.mu.Lock()
	if m.state != StateIdle {
		m.mu.Unlock()
		return Snapshot{}, ErrNotIdle
	}
	m.state = StateStreaming
	m.sessionID = ulid.Make().String()
	m.startedAt = time.Now()
	snap := m.snapshotLocked()
	m.mu.Unlock()
	m.notify(snap)
	return snap, nil
}

// Pause suspends streaming without ending the session (spec.md ss32): the
// Collector must drop new traces without buffering them.
func (m *Manager) Pause() (Snapshot, error) {
	m.mu.Lock()
	if m.state != StateStreaming {
		m.mu.Unlock()
		return Snapshot{}, ErrNotStream
	}
	m.state = StatePaused
	snap := m.snapshotLocked()
	m.mu.Unlock()
	m.notify(snap)
	return snap, nil
}

// Resume returns from PAUSED to STREAMING.
func (m *Manager) Resume() (Snapshot, error) {
	m.mu.Lock()
	if m.state != StatePaused {
		m.mu.Unlock()
		return Snapshot{}, ErrNotPaused
	}
	m.state = StateStreaming
	snap := m.snapshotLocked()
	m.mu.Unlock()
	m.notify(snap)
	return snap, nil
}

// Stop ends the session and returns to IDLE from either STREAMING or
// PAUSED.
func (m *Manager) Stop() (Snapshot, error) {
	m.mu.Lock()
	if m.state != StateStreaming && m.state != StatePaused {
		m.mu.Unlock()
		return Snapshot{}, ErrNoneActive
	}
	m.state = StateIdle
	id := m.sessionID
	m.sessionID = ""
	snap := m.snapshotLocked()
	snap.SessionID = id // report the ID of the session that just ended
	m.mu.Unlock()
	m.notify(snap)
	return snap, nil
}

// Snapshot returns the current state.
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

func (m *Manager) snapshotLocked() Snapshot {
	return Snapshot{State: m.state, SessionID: m.sessionID, StartedAt: m.startedAt}
}

func (m *Manager) notify(snap Snapshot) {
	m.mu.Lock()
	listeners := append([]func(Snapshot){}, m.listeners...)
	m.mu.Unlock()
	for _, fn := range listeners {
		fn(snap)
	}
}

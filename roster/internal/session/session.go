package session

import (
	"sync"

	"github.com/roster-io/roster/internal/store/state"
)

// Session is the shared communication space auto-created when a group is activated.
// It wraps GroupStore and scopes all operations to a single groupID.
// Sessions are managed by the Manager; user YAML never references them directly.
type Session struct {
	groupID string
	store   state.GroupStore
}

func (s *Session) Post(msg state.Message) error {
	return s.store.Append(s.groupID, msg)
}

func (s *Session) History() ([]state.Message, error) {
	return s.store.History(s.groupID)
}

func (s *Session) Close() {
	s.store.Clear(s.groupID)
}

// Manager creates and destroys Sessions for groups.
type Manager struct {
	mu       sync.Mutex
	store    state.GroupStore
	sessions map[string]*Session // key: groupID
}

func NewManager(store state.GroupStore) *Manager {
	return &Manager{
		store:    store,
		sessions: make(map[string]*Session),
	}
}

// Activate creates a shared communication space for the group if one does not exist.
func (m *Manager) Activate(groupID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[groupID]; ok {
		return s
	}
	s := &Session{groupID: groupID, store: m.store}
	m.sessions[groupID] = s
	return s
}

// Get returns the active session for a group, or nil if the group is not active.
// Not called by hub.go after the runDesk refactor; retained for external test use.
func (m *Manager) Get(groupID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[groupID]
}

// Deactivate closes the group's session and removes it from the manager.
func (m *Manager) Deactivate(groupID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[groupID]; ok {
		s.Close()
		delete(m.sessions, groupID)
	}
}

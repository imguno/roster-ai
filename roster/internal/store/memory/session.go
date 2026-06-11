package memory

import (
	"sync"

	"github.com/roster-io/roster/internal/store"
)

type deskSessionStore struct {
	mu   sync.RWMutex
	data map[string][]store.SessionEntry
}

func (s *deskSessionStore) Append(deskID, _ string, entry store.SessionEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[deskID] = append(s.data[deskID], entry)
}

func (s *deskSessionStore) Load(deskID string) []store.SessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[deskID]
}

func (s *deskSessionStore) Summarize(deskID string, summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[deskID] = []store.SessionEntry{{Role: "system", Content: summary}}
}

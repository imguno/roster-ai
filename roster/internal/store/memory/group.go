package memory

import (
	"sync"

	"github.com/roster-io/roster/internal/store"
)

type groupStore struct {
	mu   sync.RWMutex
	data map[string][]store.Message
}

func (s *groupStore) Append(groupID string, msg store.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[groupID] = append(s.data[groupID], msg)
	return nil
}

func (s *groupStore) History(groupID string) ([]store.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[groupID], nil
}

func (s *groupStore) Clear(groupID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, groupID)
}

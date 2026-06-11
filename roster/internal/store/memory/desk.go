package memory

import (
	"sync"

	"github.com/roster-io/roster/pkg/types"
)

type deskStore struct {
	mu   sync.RWMutex
	data map[string][]*types.Artifact
}

func (s *deskStore) Save(deskID string, artifact *types.Artifact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[deskID] = append(s.data[deskID], artifact)
}

func (s *deskStore) Get(deskID string) ([]*types.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.data[deskID]
	return a, ok
}

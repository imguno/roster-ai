package memory

import (
	"sync"

	"github.com/roster-io/roster/pkg/types"
)

type runStore struct {
	mu   sync.RWMutex
	data map[string]*types.Artifact
}

func (s *runStore) SaveStep(runID, groupID, deskID string, artifact *types.Artifact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[runID+"/"+groupID+"/"+deskID] = artifact
}

func (s *runStore) LoadStep(runID, groupID, deskID string) (*types.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.data[runID+"/"+groupID+"/"+deskID]
	return a, ok
}

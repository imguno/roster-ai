package file

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/roster-io/roster/pkg/types"
)

type deskStore struct {
	mu   sync.RWMutex
	path string
	data map[string][]*fileArtifact
}

type fileArtifact struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Schema    string    `json:"schema"`
	Payload   []byte    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

func newDeskStore(path string) (*deskStore, error) {
	s := &deskStore{path: path, data: make(map[string][]*fileArtifact)}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *deskStore) Save(deskID string, artifact *types.Artifact) {
	if artifact == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[deskID] = append(s.data[deskID], &fileArtifact{
		ID: artifact.ID, AgentID: artifact.AgentID, Schema: artifact.Schema,
		Payload: artifact.Payload, CreatedAt: artifact.CreatedAt,
	})
	_ = s.flush()
}

func (s *deskStore) Get(deskID string) ([]*types.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok := s.data[deskID]
	if !ok {
		return nil, false
	}
	out := make([]*types.Artifact, len(raw))
	for i, r := range raw {
		out[i] = &types.Artifact{
			ID: r.ID, AgentID: r.AgentID, Schema: r.Schema,
			Payload: r.Payload, CreatedAt: r.CreatedAt,
		}
	}
	return out, true
}

func (s *deskStore) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.data)
}

func (s *deskStore) flush() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0640)
}

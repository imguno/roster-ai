package state

import (
	"sync"

	"github.com/roster-io/roster/pkg/types"
)

// MemoryStore is an in-process Store implementation for tests and ephemeral use.
type MemoryStore struct {
	desk        *memoryDeskStore
	deskSession *memoryDeskSessionStore
	group       *memoryGroupStore
	run         *memoryRunStore
	notes       *memoryNoteStore
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		desk:        &memoryDeskStore{data: make(map[string][]*types.Artifact)},
		deskSession: &memoryDeskSessionStore{data: make(map[string][]SessionEntry)},
		group:       &memoryGroupStore{data: make(map[string][]Message)},
		run:         &memoryRunStore{data: make(map[string]*types.Artifact)},
		notes:       &memoryNoteStore{data: make(map[string]map[string][]byte)},
	}
}

func (m *MemoryStore) Desk() DeskStore              { return m.desk }
func (m *MemoryStore) DeskSession() DeskSessionStore { return m.deskSession }
func (m *MemoryStore) Group() GroupStore             { return m.group }
func (m *MemoryStore) Run() RunStore                 { return m.run }
func (m *MemoryStore) Notes() NoteStore              { return m.notes }

// --- memoryDeskStore ---

type memoryDeskStore struct {
	mu   sync.RWMutex
	data map[string][]*types.Artifact
}

func (s *memoryDeskStore) Save(deskID string, artifact *types.Artifact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[deskID] = append(s.data[deskID], artifact)
}

func (s *memoryDeskStore) Get(deskID string) ([]*types.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.data[deskID]
	return a, ok
}

// --- memoryDeskSessionStore ---

type memoryDeskSessionStore struct {
	mu   sync.RWMutex
	data map[string][]SessionEntry
}

func (s *memoryDeskSessionStore) Append(deskID, _ string, entry SessionEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[deskID] = append(s.data[deskID], entry)
}

func (s *memoryDeskSessionStore) Load(deskID string) []SessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[deskID]
}

func (s *memoryDeskSessionStore) Summarize(deskID string, summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[deskID] = []SessionEntry{{Role: "system", Content: summary}}
}

// --- memoryGroupStore ---

type memoryGroupStore struct {
	mu   sync.RWMutex
	data map[string][]Message
}

func (s *memoryGroupStore) Append(groupID string, msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[groupID] = append(s.data[groupID], msg)
	return nil
}

func (s *memoryGroupStore) History(groupID string) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[groupID], nil
}

func (s *memoryGroupStore) Clear(groupID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, groupID)
}

// --- memoryNoteStore ---

type memoryNoteStore struct {
	mu   sync.RWMutex
	data map[string]map[string][]byte // scopeID → key → value
}

func (s *memoryNoteStore) Set(scopeID, key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[scopeID] == nil {
		s.data[scopeID] = make(map[string][]byte)
	}
	s.data[scopeID][key] = value
}

func (s *memoryNoteStore) Get(scopeID, key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[scopeID][key]
	return v, ok
}

func (s *memoryNoteStore) Delete(scopeID, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data[scopeID], key)
}

func (s *memoryNoteStore) All(scopeID string) map[string][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.data[scopeID]
	out := make(map[string][]byte, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// --- memoryRunStore ---

type memoryRunStore struct {
	mu   sync.RWMutex
	data map[string]*types.Artifact // key: runID+"/"+groupID+"/"+deskID
}

func (s *memoryRunStore) SaveStep(runID, groupID, deskID string, artifact *types.Artifact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[runID+"/"+groupID+"/"+deskID] = artifact
}

func (s *memoryRunStore) LoadStep(runID, groupID, deskID string) (*types.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.data[runID+"/"+groupID+"/"+deskID]
	return a, ok
}

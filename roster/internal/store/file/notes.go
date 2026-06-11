package file

import (
	"encoding/json"
	"os"
	"sync"
)

type noteStore struct {
	mu   sync.RWMutex
	path string
	data map[string]map[string][]byte
}

func newNoteStore(path string) *noteStore {
	s := &noteStore{path: path, data: make(map[string]map[string][]byte)}
	_ = s.load()
	return s
}

func (s *noteStore) Set(scopeID, key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[scopeID] == nil {
		s.data[scopeID] = make(map[string][]byte)
	}
	s.data[scopeID][key] = value
	_ = s.flush()
}

func (s *noteStore) Get(scopeID, key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[scopeID][key]
	return v, ok
}

func (s *noteStore) Delete(scopeID, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data[scopeID], key)
	_ = s.flush()
}

func (s *noteStore) All(scopeID string) map[string][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.data[scopeID]
	out := make(map[string][]byte, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (s *noteStore) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.data)
}

func (s *noteStore) flush() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0640)
}

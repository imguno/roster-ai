package memory

import "sync"

type noteStore struct {
	mu   sync.RWMutex
	data map[string]map[string][]byte
}

func (s *noteStore) Set(scopeID, key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[scopeID] == nil {
		s.data[scopeID] = make(map[string][]byte)
	}
	s.data[scopeID][key] = value
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

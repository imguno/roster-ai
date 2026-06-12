package memory

import (
	"sync"
	"time"

	"github.com/roster-io/roster/internal/store"
)

var _ store.Store = (*Store)(nil)

type Store struct {
	mu       sync.RWMutex
	sessions map[string][]store.SessionEntry
	logs     map[string][]store.LogEntry
	notes    map[string]map[string][]byte
	metrics  []metricEntry
}

type metricEntry struct {
	runID, scopeID, agentID, name string
	value                         float64
}

func New() *Store {
	return &Store{
		sessions: make(map[string][]store.SessionEntry),
		logs:     make(map[string][]store.LogEntry),
		notes:    make(map[string]map[string][]byte),
	}
}

// ── Session ──

func (s *Store) AppendSession(scopeID, runID string, entry store.SessionEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.RunID = runID
	s.sessions[scopeID] = append(s.sessions[scopeID], entry)
}

func (s *Store) LoadSession(scopeID string, limit int) []store.SessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := s.sessions[scopeID]
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	out := make([]store.SessionEntry, len(entries))
	copy(out, entries)
	return out
}

func (s *Store) ClearSession(scopeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, scopeID)
}

func (s *Store) SummarizeSession(scopeID, summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[scopeID] = []store.SessionEntry{
		{RunID: "_summary", Role: "system", Content: summary, At: time.Now()},
	}
}

// ── Logs ──

func (s *Store) AppendLog(scopeID, runID string, entry store.LogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.RunID = runID
	if entry.At.IsZero() {
		entry.At = time.Now()
	}
	s.logs[scopeID] = append(s.logs[scopeID], entry)
}

func (s *Store) LoadLogs(scopeID string) []store.LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.LogEntry, len(s.logs[scopeID]))
	copy(out, s.logs[scopeID])
	return out
}

// ── Notes ──

func (s *Store) SetNote(scopeID, key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.notes[scopeID] == nil {
		s.notes[scopeID] = make(map[string][]byte)
	}
	s.notes[scopeID][key] = value
}

func (s *Store) GetNote(scopeID, key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.notes[scopeID][key]
	return v, ok
}

func (s *Store) DeleteNote(scopeID, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.notes[scopeID], key)
}

func (s *Store) AllNotes(scopeID string) map[string][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.notes[scopeID]
	out := make(map[string][]byte, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// ── Metrics ──

func (s *Store) RecordMetric(runID, scopeID, agentID, name string, value float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = append(s.metrics, metricEntry{runID, scopeID, agentID, name, value})
	return nil
}

func (s *Store) MetricsByScope(scopeID string) ([]store.MetricRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agg := map[string]float64{}
	for _, m := range s.metrics {
		if scopeID == "" || m.scopeID == scopeID {
			agg[m.scopeID+"|"+m.name] += m.value
		}
	}
	var out []store.MetricRow
	for k, v := range agg {
		parts := splitFirst(k, "|")
		out = append(out, store.MetricRow{ScopeID: parts[0], Name: parts[1], Value: v})
	}
	return out, nil
}

func (s *Store) MetricsByAgent(agentID string) ([]store.MetricRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agg := map[string]float64{}
	for _, m := range s.metrics {
		if agentID == "" || m.agentID == agentID {
			agg[m.agentID+"|"+m.name] += m.value
		}
	}
	var out []store.MetricRow
	for k, v := range agg {
		parts := splitFirst(k, "|")
		out = append(out, store.MetricRow{AgentID: parts[0], Name: parts[1], Value: v})
	}
	return out, nil
}

func (s *Store) MetricsByRun(runID string) ([]store.MetricRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agg := map[string]float64{}
	for _, m := range s.metrics {
		if m.runID == runID {
			agg[m.scopeID+"|"+m.agentID+"|"+m.name] += m.value
		}
	}
	var out []store.MetricRow
	for k, v := range agg {
		i1 := indexOf(k, "|")
		rest := k[i1+1:]
		i2 := indexOf(rest, "|")
		out = append(out, store.MetricRow{
			RunID:   runID,
			ScopeID: k[:i1],
			AgentID: rest[:i2],
			Name:    rest[i2+1:],
			Value:   v,
		})
	}
	return out, nil
}

func splitFirst(s, sep string) [2]string {
	i := indexOf(s, sep)
	if i < 0 {
		return [2]string{s, ""}
	}
	return [2]string{s[:i], s[i+len(sep):]}
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

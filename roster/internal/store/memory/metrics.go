package memory

import (
	"sync"

	"github.com/roster-io/roster/internal/store"
)

type metricRow struct {
	runID, deskID, agentID, name string
	value                        float64
}

type metricStore struct {
	mu   sync.RWMutex
	rows []metricRow
}

func (s *metricStore) Record(runID, deskID, agentID, name string, value float64) error {
	s.mu.Lock()
	s.rows = append(s.rows, metricRow{runID, deskID, agentID, name, value})
	s.mu.Unlock()
	return nil
}

func (s *metricStore) SumByAgent(agentID string) ([]store.MetricRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agg := map[string]*store.MetricRow{}
	for _, r := range s.rows {
		if agentID != "" && r.agentID != agentID {
			continue
		}
		k := r.agentID + "\x00" + r.name
		if agg[k] == nil {
			agg[k] = &store.MetricRow{AgentID: r.agentID, Name: r.name}
		}
		agg[k].Value += r.value
	}
	return toSlice(agg), nil
}

func (s *metricStore) SumByDesk(deskID string) ([]store.MetricRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agg := map[string]*store.MetricRow{}
	for _, r := range s.rows {
		if deskID != "" && r.deskID != deskID {
			continue
		}
		k := r.deskID + "\x00" + r.name
		if agg[k] == nil {
			agg[k] = &store.MetricRow{DeskID: r.deskID, Name: r.name}
		}
		agg[k].Value += r.value
	}
	return toSlice(agg), nil
}

func (s *metricStore) SumByRun(runID string) ([]store.MetricRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agg := map[string]*store.MetricRow{}
	for _, r := range s.rows {
		if r.runID != runID {
			continue
		}
		k := r.deskID + "\x00" + r.agentID + "\x00" + r.name
		if agg[k] == nil {
			agg[k] = &store.MetricRow{RunID: runID, DeskID: r.deskID, AgentID: r.agentID, Name: r.name}
		}
		agg[k].Value += r.value
	}
	return toSlice(agg), nil
}

func toSlice(m map[string]*store.MetricRow) []store.MetricRow {
	out := make([]store.MetricRow, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	return out
}

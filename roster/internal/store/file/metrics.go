package file

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/roster-io/roster/internal/store"
)

type metricRecord struct {
	At      int64   `json:"at"`
	RunID   string  `json:"run_id"`
	DeskID  string  `json:"desk_id"`
	AgentID string  `json:"agent_id"`
	Name    string  `json:"name"`
	Value   float64 `json:"value"`
}

type metricStore struct {
	mu   sync.Mutex
	path string
	rows []metricRecord
}

func newMetricStore(path string) *metricStore {
	s := &metricStore{path: path}
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range splitLines(data) {
			var r metricRecord
			if json.Unmarshal(line, &r) == nil {
				s.rows = append(s.rows, r)
			}
		}
	}
	return s
}

func (s *metricStore) Record(runID, deskID, agentID, name string, value float64) error {
	r := metricRecord{
		At: time.Now().UnixMilli(), RunID: runID, DeskID: deskID,
		AgentID: agentID, Name: name, Value: value,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, r)
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	data, _ := json.Marshal(r)
	_, err = f.Write(append(data, '\n'))
	return err
}

func (s *metricStore) SumByAgent(agentID string) ([]store.MetricRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	agg := map[string]*store.MetricRow{}
	for _, r := range s.rows {
		if agentID != "" && r.AgentID != agentID {
			continue
		}
		k := r.AgentID + "\x00" + r.Name
		if agg[k] == nil {
			agg[k] = &store.MetricRow{AgentID: r.AgentID, Name: r.Name}
		}
		agg[k].Value += r.Value
	}
	return rowMapToSlice(agg), nil
}

func (s *metricStore) SumByDesk(deskID string) ([]store.MetricRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	agg := map[string]*store.MetricRow{}
	for _, r := range s.rows {
		if deskID != "" && r.DeskID != deskID {
			continue
		}
		k := r.DeskID + "\x00" + r.Name
		if agg[k] == nil {
			agg[k] = &store.MetricRow{DeskID: r.DeskID, Name: r.Name}
		}
		agg[k].Value += r.Value
	}
	return rowMapToSlice(agg), nil
}

func (s *metricStore) SumByRun(runID string) ([]store.MetricRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	agg := map[string]*store.MetricRow{}
	for _, r := range s.rows {
		if r.RunID != runID {
			continue
		}
		k := r.DeskID + "\x00" + r.AgentID + "\x00" + r.Name
		if agg[k] == nil {
			agg[k] = &store.MetricRow{RunID: runID, DeskID: r.DeskID, AgentID: r.AgentID, Name: r.Name}
		}
		agg[k].Value += r.Value
	}
	return rowMapToSlice(agg), nil
}

func rowMapToSlice(m map[string]*store.MetricRow) []store.MetricRow {
	out := make([]store.MetricRow, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	return out
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

package sqlite

import (
	"database/sql"
	"time"

	"github.com/roster-io/roster/internal/store"
)

type metricStore struct{ db *sql.DB }

func (s *metricStore) Record(runID, deskID, agentID, name string, value float64) error {
	_, err := s.db.Exec(
		`INSERT INTO metrics(at, run_id, desk_id, agent_id, name, value) VALUES(?,?,?,?,?,?)`,
		time.Now().UnixMilli(), runID, deskID, agentID, name, value,
	)
	return err
}

func (s *metricStore) SumByAgent(agentID string) ([]store.MetricRow, error) {
	rows, err := s.db.Query(
		`SELECT agent_id, name, SUM(value) FROM metrics WHERE (? = '' OR agent_id = ?) GROUP BY agent_id, name`,
		agentID, agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.MetricRow
	for rows.Next() {
		var r store.MetricRow
		if err := rows.Scan(&r.AgentID, &r.Name, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *metricStore) SumByDesk(deskID string) ([]store.MetricRow, error) {
	rows, err := s.db.Query(
		`SELECT desk_id, name, SUM(value) FROM metrics WHERE (? = '' OR desk_id = ?) GROUP BY desk_id, name`,
		deskID, deskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.MetricRow
	for rows.Next() {
		var r store.MetricRow
		if err := rows.Scan(&r.DeskID, &r.Name, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *metricStore) SumByRun(runID string) ([]store.MetricRow, error) {
	rows, err := s.db.Query(
		`SELECT desk_id, agent_id, name, SUM(value) FROM metrics WHERE run_id = ? GROUP BY desk_id, agent_id, name`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.MetricRow
	for rows.Next() {
		var r store.MetricRow
		if err := rows.Scan(&r.DeskID, &r.AgentID, &r.Name, &r.Value); err != nil {
			return nil, err
		}
		r.RunID = runID
		out = append(out, r)
	}
	return out, rows.Err()
}

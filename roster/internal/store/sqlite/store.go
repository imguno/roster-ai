package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/roster-io/roster/internal/store"
)

const maxSessionEntries = 200

var _ store.Store = (*Store)(nil)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", dbPath, err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: enable WAL: %w", err)
	}
	if err := createSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ── Session ──

func (s *Store) AppendSession(scopeID, runID string, entry store.SessionEntry) {
	_, _ = s.db.Exec(
		`INSERT INTO sessions (scope_id, run_id, source_id, role, type, content, at) VALUES (?,?,?,?,?,?,?)`,
		scopeID, runID, entry.SourceID, entry.Role, entry.Type, entry.Content, entry.At.UTC(),
	)
}

func (s *Store) LoadSession(scopeID string, limit int) []store.SessionEntry {
	if limit <= 0 {
		limit = maxSessionEntries
	}

	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE scope_id = ?`, scopeID).Scan(&count)
	offset := 0
	if count > limit {
		offset = count - limit
	}

	rows, err := s.db.Query(
		`SELECT source_id, run_id, role, type, content, at FROM sessions
		 WHERE scope_id = ? ORDER BY id LIMIT ? OFFSET ?`,
		scopeID, limit, offset,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []store.SessionEntry
	for rows.Next() {
		var e store.SessionEntry
		var ts time.Time
		if err := rows.Scan(&e.SourceID, &e.RunID, &e.Role, &e.Type, &e.Content, &ts); err != nil {
			continue
		}
		e.At = ts
		result = append(result, e)
	}
	return result
}

func (s *Store) ClearSession(scopeID string) {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE scope_id = ?`, scopeID)
}

func (s *Store) SummarizeSession(scopeID, summary string) {
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM sessions WHERE scope_id = ?`, scopeID)
	tx.Exec(
		`INSERT INTO sessions (scope_id, run_id, role, content, at) VALUES (?,?,?,?,?)`,
		scopeID, "_summary", "system", summary, time.Now().UTC(),
	)
	_ = tx.Commit()
}

// ── Logs ──

func (s *Store) AppendLog(scopeID, runID string, entry store.LogEntry) {
	at := entry.At
	if at.IsZero() {
		at = time.Now()
	}
	_, _ = s.db.Exec(
		`INSERT INTO logs (scope_id, run_id, type, content, at) VALUES (?,?,?,?,?)`,
		scopeID, runID, entry.Type, entry.Content, at.UTC(),
	)
}

func (s *Store) LoadLogs(scopeID string) []store.LogEntry {
	rows, err := s.db.Query(
		`SELECT run_id, type, content, at FROM logs WHERE scope_id = ? ORDER BY id`,
		scopeID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []store.LogEntry
	for rows.Next() {
		var e store.LogEntry
		var ts time.Time
		if err := rows.Scan(&e.RunID, &e.Type, &e.Content, &ts); err != nil {
			continue
		}
		e.At = ts
		result = append(result, e)
	}
	return result
}

// ── Notes ──

func (s *Store) SetNote(scopeID, key string, value []byte) {
	_, _ = s.db.Exec(
		`INSERT OR REPLACE INTO notes (scope_id, key, value) VALUES (?,?,?)`,
		scopeID, key, value,
	)
}

func (s *Store) GetNote(scopeID, key string) ([]byte, bool) {
	var v []byte
	err := s.db.QueryRow(`SELECT value FROM notes WHERE scope_id = ? AND key = ?`, scopeID, key).Scan(&v)
	if err != nil {
		return nil, false
	}
	return v, true
}

func (s *Store) DeleteNote(scopeID, key string) {
	_, _ = s.db.Exec(`DELETE FROM notes WHERE scope_id = ? AND key = ?`, scopeID, key)
}

func (s *Store) AllNotes(scopeID string) map[string][]byte {
	rows, err := s.db.Query(`SELECT key, value FROM notes WHERE scope_id = ?`, scopeID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make(map[string][]byte)
	for rows.Next() {
		var k string
		var v []byte
		if err := rows.Scan(&k, &v); err == nil {
			out[k] = v
		}
	}
	return out
}

// ── Metrics ──

func (s *Store) RecordMetric(runID, scopeID, agentID, name string, value float64) error {
	_, err := s.db.Exec(
		`INSERT INTO metrics(at, run_id, scope_id, agent_id, name, value) VALUES(?,?,?,?,?,?)`,
		time.Now().UnixMilli(), runID, scopeID, agentID, name, value,
	)
	return err
}

func (s *Store) MetricsByScope(scopeID string) ([]store.MetricRow, error) {
	rows, err := s.db.Query(
		`SELECT scope_id, name, SUM(value) FROM metrics WHERE (? = '' OR scope_id = ?) GROUP BY scope_id, name`,
		scopeID, scopeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.MetricRow
	for rows.Next() {
		var r store.MetricRow
		if err := rows.Scan(&r.ScopeID, &r.Name, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) MetricsByAgent(agentID string) ([]store.MetricRow, error) {
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

func (s *Store) MetricsByRun(runID string) ([]store.MetricRow, error) {
	rows, err := s.db.Query(
		`SELECT scope_id, agent_id, name, SUM(value) FROM metrics WHERE run_id = ? GROUP BY scope_id, agent_id, name`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.MetricRow
	for rows.Next() {
		var r store.MetricRow
		if err := rows.Scan(&r.ScopeID, &r.AgentID, &r.Name, &r.Value); err != nil {
			return nil, err
		}
		r.RunID = runID
		out = append(out, r)
	}
	return out, rows.Err()
}

package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/roster-io/roster/pkg/types"
)

// Compile-time interface check.
var _ Store = (*SQLiteStore)(nil)

// SQLiteStore is a production Store backed by a SQLite database.
type SQLiteStore struct {
	db          *sql.DB
	desk        *sqliteDeskStore
	deskSession *sqliteDeskSessionStore
	group       *sqliteGroupStore
	run         *sqliteRunStore
	notes       *sqliteNoteStore
}

// NewSQLiteStore opens (or creates) a SQLite database at dbPath and
// initialises the required tables. The database uses WAL mode for better
// concurrent read performance.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", dbPath, err)
	}

	// Enable WAL mode.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: enable WAL: %w", err)
	}

	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	s := &SQLiteStore{db: db}
	s.desk = &sqliteDeskStore{db: db}
	s.deskSession = &sqliteDeskSessionStore{db: db}
	s.group = &sqliteGroupStore{db: db}
	s.run = &sqliteRunStore{db: db}
	s.notes = &sqliteNoteStore{db: db}
	return s, nil
}

// Close releases the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Desk() DeskStore              { return s.desk }
func (s *SQLiteStore) DeskSession() DeskSessionStore { return s.deskSession }
func (s *SQLiteStore) Group() GroupStore             { return s.group }
func (s *SQLiteStore) Run() RunStore                 { return s.run }
func (s *SQLiteStore) Notes() NoteStore              { return s.notes }

// createTables initialises all required tables and indexes.
func createTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS desk_artifacts (
			id TEXT PRIMARY KEY,
			desk_id TEXT NOT NULL,
			agent_id TEXT,
			schema TEXT,
			payload BLOB,
			meta TEXT,
			created_at DATETIME NOT NULL,
			UNIQUE(desk_id, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_desk_artifacts_desk ON desk_artifacts(desk_id)`,

		`CREATE TABLE IF NOT EXISTS desk_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			desk_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_desk_sessions_desk ON desk_sessions(desk_id)`,

		`CREATE TABLE IF NOT EXISTS group_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id TEXT NOT NULL,
			desk_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			payload BLOB
		)`,
		`CREATE INDEX IF NOT EXISTS idx_group_messages_group ON group_messages(group_id)`,

		`CREATE TABLE IF NOT EXISTS run_steps (
			run_id TEXT NOT NULL,
			group_id TEXT NOT NULL,
			desk_id TEXT NOT NULL,
			artifact_id TEXT,
			agent_id TEXT,
			schema TEXT,
			payload BLOB,
			meta TEXT,
			created_at DATETIME,
			PRIMARY KEY (run_id, group_id, desk_id)
		)`,

		`CREATE TABLE IF NOT EXISTS notes (
			scope_id TEXT NOT NULL,
			key TEXT NOT NULL,
			value BLOB NOT NULL,
			PRIMARY KEY (scope_id, key)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("sqlite: create table: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// sqliteDeskStore
// ---------------------------------------------------------------------------

type sqliteDeskStore struct {
	db *sql.DB
}

func (s *sqliteDeskStore) Save(deskID string, artifact *types.Artifact) {
	if artifact == nil {
		return
	}
	metaJSON, _ := json.Marshal(artifact.Meta)
	_, _ = s.db.Exec(
		`INSERT OR REPLACE INTO desk_artifacts (id, desk_id, agent_id, schema, payload, meta, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		artifact.ID, deskID, artifact.AgentID, artifact.Schema,
		artifact.Payload, string(metaJSON), artifact.CreatedAt.UTC(),
	)
}

func (s *sqliteDeskStore) Get(deskID string) ([]*types.Artifact, bool) {
	rows, err := s.db.Query(
		`SELECT id, agent_id, schema, payload, meta, created_at
		 FROM desk_artifacts WHERE desk_id = ? ORDER BY created_at`,
		deskID,
	)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	var out []*types.Artifact
	for rows.Next() {
		var (
			a       types.Artifact
			metaStr sql.NullString
			ts      time.Time
		)
		if err := rows.Scan(&a.ID, &a.AgentID, &a.Schema, &a.Payload, &metaStr, &ts); err != nil {
			continue
		}
		a.CreatedAt = ts
		if metaStr.Valid && metaStr.String != "" && metaStr.String != "null" {
			_ = json.Unmarshal([]byte(metaStr.String), &a.Meta)
		}
		out = append(out, &a)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// ---------------------------------------------------------------------------
// sqliteDeskSessionStore
// ---------------------------------------------------------------------------

type sqliteDeskSessionStore struct {
	db *sql.DB
}

func (s *sqliteDeskSessionStore) Append(deskID, runID string, entry SessionEntry) {
	_, _ = s.db.Exec(
		`INSERT INTO desk_sessions (desk_id, run_id, role, content, at)
		 VALUES (?, ?, ?, ?, ?)`,
		deskID, runID, entry.Role, entry.Content, entry.At.UTC(),
	)
}

func (s *sqliteDeskSessionStore) Load(deskID string) []SessionEntry {
	// Count total entries to apply windowing.
	var count int
	_ = s.db.QueryRow(
		`SELECT COUNT(*) FROM desk_sessions WHERE desk_id = ?`, deskID,
	).Scan(&count)

	offset := 0
	if count > maxSessionEntries {
		offset = count - maxSessionEntries
	}

	rows, err := s.db.Query(
		`SELECT role, content, at FROM desk_sessions
		 WHERE desk_id = ? ORDER BY id LIMIT ? OFFSET ?`,
		deskID, maxSessionEntries, offset,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []SessionEntry
	for rows.Next() {
		var (
			e  SessionEntry
			ts time.Time
		)
		if err := rows.Scan(&e.Role, &e.Content, &ts); err != nil {
			continue
		}
		e.At = ts
		result = append(result, e)
	}
	return result
}

func (s *sqliteDeskSessionStore) Summarize(deskID string, summary string) {
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()

	// Delete all entries for this desk.
	if _, err := tx.Exec(`DELETE FROM desk_sessions WHERE desk_id = ?`, deskID); err != nil {
		return
	}

	// Insert a single system summary entry.
	if _, err := tx.Exec(
		`INSERT INTO desk_sessions (desk_id, run_id, role, content, at)
		 VALUES (?, ?, ?, ?, ?)`,
		deskID, "_summary", "system", summary, time.Now().UTC(),
	); err != nil {
		return
	}

	_ = tx.Commit()
}

// ---------------------------------------------------------------------------
// sqliteGroupStore
// ---------------------------------------------------------------------------

type sqliteGroupStore struct {
	db *sql.DB
}

func (s *sqliteGroupStore) Append(groupID string, msg Message) error {
	_, err := s.db.Exec(
		`INSERT INTO group_messages (group_id, desk_id, role, content, payload)
		 VALUES (?, ?, ?, ?, ?)`,
		groupID, msg.DeskID, msg.Role, msg.Content, msg.Payload,
	)
	if err != nil {
		return fmt.Errorf("sqlite: append group message: %w", err)
	}
	return nil
}

func (s *sqliteGroupStore) History(groupID string) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT desk_id, role, content, payload
		 FROM group_messages WHERE group_id = ? ORDER BY id`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read group history: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.DeskID, &m.Role, &m.Content, &m.Payload); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (s *sqliteGroupStore) Clear(groupID string) {
	_, _ = s.db.Exec(`DELETE FROM group_messages WHERE group_id = ?`, groupID)
}

// ---------------------------------------------------------------------------
// sqliteRunStore
// ---------------------------------------------------------------------------

type sqliteRunStore struct {
	db *sql.DB
}

func (s *sqliteRunStore) SaveStep(runID, groupID, deskID string, artifact *types.Artifact) {
	if artifact == nil {
		return
	}
	metaJSON, _ := json.Marshal(artifact.Meta)
	_, _ = s.db.Exec(
		`INSERT OR REPLACE INTO run_steps (run_id, group_id, desk_id, artifact_id, agent_id, schema, payload, meta, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, groupID, deskID,
		artifact.ID, artifact.AgentID, artifact.Schema,
		artifact.Payload, string(metaJSON), artifact.CreatedAt.UTC(),
	)
}

func (s *sqliteRunStore) LoadStep(runID, groupID, deskID string) (*types.Artifact, bool) {
	var (
		a       types.Artifact
		metaStr sql.NullString
		ts      time.Time
	)
	err := s.db.QueryRow(
		`SELECT artifact_id, agent_id, schema, payload, meta, created_at
		 FROM run_steps WHERE run_id = ? AND group_id = ? AND desk_id = ?`,
		runID, groupID, deskID,
	).Scan(&a.ID, &a.AgentID, &a.Schema, &a.Payload, &metaStr, &ts)
	if err != nil {
		return nil, false
	}
	a.CreatedAt = ts
	if metaStr.Valid && metaStr.String != "" && metaStr.String != "null" {
		_ = json.Unmarshal([]byte(metaStr.String), &a.Meta)
	}
	return &a, true
}

// ---------------------------------------------------------------------------
// sqliteNoteStore
// ---------------------------------------------------------------------------

type sqliteNoteStore struct {
	db *sql.DB
}

func (s *sqliteNoteStore) Set(scopeID, key string, value []byte) {
	_, _ = s.db.Exec(
		`INSERT OR REPLACE INTO notes (scope_id, key, value) VALUES (?, ?, ?)`,
		scopeID, key, value,
	)
}

func (s *sqliteNoteStore) Get(scopeID, key string) ([]byte, bool) {
	var v []byte
	err := s.db.QueryRow(`SELECT value FROM notes WHERE scope_id = ? AND key = ?`, scopeID, key).Scan(&v)
	if err != nil {
		return nil, false
	}
	return v, true
}

func (s *sqliteNoteStore) Delete(scopeID, key string) {
	_, _ = s.db.Exec(`DELETE FROM notes WHERE scope_id = ? AND key = ?`, scopeID, key)
}

func (s *sqliteNoteStore) All(scopeID string) map[string][]byte {
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

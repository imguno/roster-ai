package sqlite

import (
	"database/sql"
	"fmt"
)

func createSchema(db *sql.DB) error {
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

		`CREATE TABLE IF NOT EXISTS metrics (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			at       INTEGER NOT NULL,
			run_id   TEXT NOT NULL,
			desk_id  TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			name     TEXT NOT NULL,
			value    REAL NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_agent ON metrics(agent_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_desk  ON metrics(desk_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_run   ON metrics(run_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("sqlite: create schema: %w", err)
		}
	}
	return nil
}

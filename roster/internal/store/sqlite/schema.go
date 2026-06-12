package sqlite

import (
	"database/sql"
	"fmt"
)

func createSchema(db *sql.DB) error {
	stmts := []string{
		// Unified session table — replaces desk_sessions + group_messages.
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scope_id  TEXT NOT NULL,
			run_id    TEXT NOT NULL DEFAULT '',
			source_id TEXT NOT NULL DEFAULT '',
			role      TEXT NOT NULL,
			type      TEXT NOT NULL DEFAULT '',
			content   TEXT NOT NULL,
			at        DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_scope ON sessions(scope_id)`,

		// Logs — execution progress.
		`CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scope_id TEXT NOT NULL,
			run_id   TEXT NOT NULL DEFAULT '',
			type     TEXT NOT NULL,
			content  TEXT NOT NULL,
			at       DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_scope ON logs(scope_id)`,

		// Notes — key-value state.
		`CREATE TABLE IF NOT EXISTS notes (
			scope_id TEXT NOT NULL,
			key      TEXT NOT NULL,
			value    BLOB NOT NULL,
			PRIMARY KEY (scope_id, key)
		)`,

		// Metrics.
		`CREATE TABLE IF NOT EXISTS metrics (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			at       INTEGER NOT NULL,
			run_id   TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			name     TEXT NOT NULL,
			value    REAL NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_scope ON metrics(scope_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_agent ON metrics(agent_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_run   ON metrics(run_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("sqlite: create schema: %w", err)
		}
	}

	// Migrate legacy tables if they exist.
	migrateLegacy(db)

	return nil
}

// migrateLegacy copies data from old tables (desk_sessions, group_messages,
// desk_artifacts, run_steps) into the new unified tables, then drops them.
func migrateLegacy(db *sql.DB) {
	// desk_sessions → sessions
	if tableExists(db, "desk_sessions") {
		db.Exec(`INSERT OR IGNORE INTO sessions (scope_id, run_id, role, content, at)
			SELECT desk_id, run_id, role, content, at FROM desk_sessions`)
		db.Exec(`DROP TABLE desk_sessions`)
	}
	// group_messages → sessions
	if tableExists(db, "group_messages") {
		db.Exec(`INSERT OR IGNORE INTO sessions (scope_id, source_id, role, content, at)
			SELECT group_id, desk_id, role, content, CURRENT_TIMESTAMP FROM group_messages`)
		db.Exec(`DROP TABLE group_messages`)
	}
	// Drop legacy artifact tables.
	if tableExists(db, "desk_artifacts") {
		db.Exec(`DROP TABLE desk_artifacts`)
	}
	if tableExists(db, "run_steps") {
		db.Exec(`DROP TABLE run_steps`)
	}
	if tableExists(db, "artifacts") {
		db.Exec(`DROP TABLE artifacts`)
	}
	if tableExists(db, "checkpoints") {
		db.Exec(`DROP TABLE checkpoints`)
	}
}

func tableExists(db *sql.DB, name string) bool {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return err == nil && n > 0
}

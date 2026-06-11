package sqlite

import (
	"database/sql"
	"time"

	"github.com/roster-io/roster/internal/store"
)

const maxSessionEntries = 200

type deskSessionStore struct{ db *sql.DB }

func (s *deskSessionStore) Append(deskID, runID string, entry store.SessionEntry) {
	_, _ = s.db.Exec(
		`INSERT INTO desk_sessions (desk_id, run_id, role, content, at) VALUES (?, ?, ?, ?, ?)`,
		deskID, runID, entry.Role, entry.Content, entry.At.UTC(),
	)
}

func (s *deskSessionStore) Load(deskID string) []store.SessionEntry {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM desk_sessions WHERE desk_id = ?`, deskID).Scan(&count)

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

	var result []store.SessionEntry
	for rows.Next() {
		var (
			e  store.SessionEntry
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

func (s *deskSessionStore) Summarize(deskID string, summary string) {
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM desk_sessions WHERE desk_id = ?`, deskID); err != nil {
		return
	}
	_, _ = tx.Exec(
		`INSERT INTO desk_sessions (desk_id, run_id, role, content, at) VALUES (?, ?, ?, ?, ?)`,
		deskID, "_summary", "system", summary, time.Now().UTC(),
	)
	_ = tx.Commit()
}

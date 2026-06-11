package sqlite

import "database/sql"

type noteStore struct{ db *sql.DB }

func (s *noteStore) Set(scopeID, key string, value []byte) {
	_, _ = s.db.Exec(
		`INSERT OR REPLACE INTO notes (scope_id, key, value) VALUES (?, ?, ?)`,
		scopeID, key, value,
	)
}

func (s *noteStore) Get(scopeID, key string) ([]byte, bool) {
	var v []byte
	err := s.db.QueryRow(`SELECT value FROM notes WHERE scope_id = ? AND key = ?`, scopeID, key).Scan(&v)
	if err != nil {
		return nil, false
	}
	return v, true
}

func (s *noteStore) Delete(scopeID, key string) {
	_, _ = s.db.Exec(`DELETE FROM notes WHERE scope_id = ? AND key = ?`, scopeID, key)
}

func (s *noteStore) All(scopeID string) map[string][]byte {
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

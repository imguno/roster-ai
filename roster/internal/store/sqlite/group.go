package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/roster-io/roster/internal/store"
)

type groupStore struct{ db *sql.DB }

func (s *groupStore) Append(groupID string, msg store.Message) error {
	_, err := s.db.Exec(
		`INSERT INTO group_messages (group_id, desk_id, role, content, payload) VALUES (?, ?, ?, ?, ?)`,
		groupID, msg.DeskID, msg.Role, msg.Content, msg.Payload,
	)
	if err != nil {
		return fmt.Errorf("sqlite: append group message: %w", err)
	}
	return nil
}

func (s *groupStore) History(groupID string) ([]store.Message, error) {
	rows, err := s.db.Query(
		`SELECT desk_id, role, content, payload FROM group_messages WHERE group_id = ? ORDER BY id`,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read group history: %w", err)
	}
	defer rows.Close()

	var msgs []store.Message
	for rows.Next() {
		var m store.Message
		if err := rows.Scan(&m.DeskID, &m.Role, &m.Content, &m.Payload); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (s *groupStore) Clear(groupID string) {
	_, _ = s.db.Exec(`DELETE FROM group_messages WHERE group_id = ?`, groupID)
}

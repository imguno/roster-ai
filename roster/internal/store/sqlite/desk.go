package sqlite

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/roster-io/roster/pkg/types"
)

type deskStore struct{ db *sql.DB }

func (s *deskStore) Save(deskID string, artifact *types.Artifact) {
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

func (s *deskStore) Get(deskID string) ([]*types.Artifact, bool) {
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

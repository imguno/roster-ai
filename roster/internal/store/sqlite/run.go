package sqlite

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/roster-io/roster/pkg/types"
)

type runStore struct{ db *sql.DB }

func (s *runStore) SaveStep(runID, groupID, deskID string, artifact *types.Artifact) {
	if artifact == nil {
		return
	}
	metaJSON, _ := json.Marshal(artifact.Meta)
	_, _ = s.db.Exec(
		`INSERT OR REPLACE INTO run_steps
		 (run_id, group_id, desk_id, artifact_id, agent_id, schema, payload, meta, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, groupID, deskID,
		artifact.ID, artifact.AgentID, artifact.Schema,
		artifact.Payload, string(metaJSON), artifact.CreatedAt.UTC(),
	)
}

func (s *runStore) LoadStep(runID, groupID, deskID string) (*types.Artifact, bool) {
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

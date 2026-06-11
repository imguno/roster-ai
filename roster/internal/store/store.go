package store

import (
	"time"

	"github.com/roster-io/roster/pkg/types"
)

// Store is the top-level state interface.
type Store interface {
	Desk() DeskStore
	DeskSession() DeskSessionStore
	Group() GroupStore
	Run() RunStore
	Notes() NoteStore
	Metrics() MetricStore
}

// DeskStore persists output artifacts for a desk.
type DeskStore interface {
	Save(deskID string, artifact *types.Artifact)
	Get(deskID string) ([]*types.Artifact, bool)
}

// DeskSessionStore persists a desk's working conversation history.
type DeskSessionStore interface {
	Append(deskID, runID string, entry SessionEntry)
	Load(deskID string) []SessionEntry
	Summarize(deskID string, summary string)
}

// GroupStore manages the shared communication space for a group.
type GroupStore interface {
	Append(groupID string, msg Message) error
	History(groupID string) ([]Message, error)
	Clear(groupID string)
}

// RunStore persists per-desk checkpoints within a group run.
type RunStore interface {
	SaveStep(runID, groupID, deskID string, artifact *types.Artifact)
	LoadStep(runID, groupID, deskID string) (*types.Artifact, bool)
}

// NoteStore persists key-value notes for a desk or group.
type NoteStore interface {
	Set(scopeID, key string, value []byte)
	Get(scopeID, key string) ([]byte, bool)
	Delete(scopeID, key string)
	All(scopeID string) map[string][]byte
}

// MetricStore persists agent execution metrics.
type MetricStore interface {
	Record(runID, deskID, agentID, name string, value float64) error
	SumByAgent(agentID string) ([]MetricRow, error)
	SumByDesk(deskID string) ([]MetricRow, error)
	SumByRun(runID string) ([]MetricRow, error)
}

// SessionEntry is one turn in a desk's session history.
type SessionEntry struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

// Message is a unit of communication in a group shared space.
type Message struct {
	DeskID  string `json:"desk_id"`
	Role    string `json:"role"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Payload []byte `json:"payload,omitempty"`
}

// MetricRow is one aggregated metric result.
type MetricRow struct {
	RunID   string  `json:"run_id,omitempty"`
	DeskID  string  `json:"desk_id,omitempty"`
	AgentID string  `json:"agent_id,omitempty"`
	Name    string  `json:"name"`
	Value   float64 `json:"value"`
}

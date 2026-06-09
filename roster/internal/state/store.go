package state

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
}

// RunStore persists per-desk checkpoints within a group run so that
// interrupted group executions can skip already-completed desks on restart.
type RunStore interface {
	// SaveStep persists an artifact for a completed desk step.
	SaveStep(runID, groupID, deskID string, artifact *types.Artifact)
	// LoadStep retrieves a previously saved artifact, or returns false if not found.
	LoadStep(runID, groupID, deskID string) (*types.Artifact, bool)
}

// DeskStore persists output artifacts for a desk.
type DeskStore interface {
	Save(deskID string, artifact *types.Artifact)
	Get(deskID string) ([]*types.Artifact, bool)
}

// DeskSessionStore persists a desk's working conversation history.
type DeskSessionStore interface {
	// Append adds an entry to the session for the given desk and run.
	// Each runID produces a separate file, enabling per-task context isolation.
	Append(deskID, runID string, entry SessionEntry)
	Load(deskID string) []SessionEntry
	// Summarize replaces history with a compact summary, keeping it from growing unbounded.
	Summarize(deskID string, summary string)
}

// GroupStore manages the shared communication space for a group.
type GroupStore interface {
	Append(groupID string, msg Message) error
	History(groupID string) ([]Message, error)
	Clear(groupID string)
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
	Content string `json:"content"`
	Payload []byte `json:"payload,omitempty"`
}

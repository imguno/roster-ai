package types

import "time"

// Artifact is the output produced by a desk after handling an event.
// It is carried within the event payload to downstream subscribers.
type Artifact struct {
	ID        string            `json:"id"`
	AgentID   string            `json:"agent_id"`
	Schema    string            `json:"schema"` // artifact schema name, e.g. "figma-spec-v1"
	Payload   []byte            `json:"payload"`
	Meta      map[string]string `json:"meta,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

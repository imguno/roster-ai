package store

import "time"

// Store is the unified storage interface.
// All data is keyed by scopeID — desk IDs, group IDs, or any logical scope.
// The same interface applies regardless of scope level; only the sharing
// boundary differs.
type Store interface {
	// Session — conversation history (desk or group scope).
	AppendSession(scopeID, runID string, entry SessionEntry)
	LoadSession(scopeID string, limit int) []SessionEntry // limit <= 0 means all
	ClearSession(scopeID string)
	SummarizeSession(scopeID, summary string)

	// Logs — execution progress.
	AppendLog(scopeID, runID string, entry LogEntry)
	LoadLogs(scopeID string) []LogEntry

	// Notes — key-value state.
	SetNote(scopeID, key string, value []byte)
	GetNote(scopeID, key string) ([]byte, bool)
	DeleteNote(scopeID, key string)
	AllNotes(scopeID string) map[string][]byte

	// Metrics.
	RecordMetric(runID, scopeID, agentID, name string, value float64) error
	MetricsByScope(scopeID string) ([]MetricRow, error)
	MetricsByAgent(agentID string) ([]MetricRow, error)
	MetricsByRun(runID string) ([]MetricRow, error)
}

// SessionEntry is one turn in a conversation history.
type SessionEntry struct {
	SourceID string    `json:"source_id,omitempty"` // originating desk (for group-scope)
	RunID    string    `json:"run_id,omitempty"`
	Role     string    `json:"role"`    // "user" | "assistant" | "system" | "agent"
	Type     string    `json:"type,omitempty"` // "llm" | "script" | "resource" etc.
	Content  string    `json:"content"`
	At       time.Time `json:"at"`
}

// LogEntry is a single progress or result log from execution.
type LogEntry struct {
	RunID   string    `json:"run_id,omitempty"`
	Type    string    `json:"type"`    // "step" | "result"
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

// MetricRow is one aggregated metric result.
type MetricRow struct {
	RunID   string  `json:"run_id,omitempty"`
	ScopeID string  `json:"scope_id,omitempty"`
	AgentID string  `json:"agent_id,omitempty"`
	Name    string  `json:"name"`
	Value   float64 `json:"value"`
}

package store

import "time"

// SessionStore handles conversation history (desk or group scope).
type SessionStore interface {
	AppendSession(scopeID, runID string, entry SessionEntry)
	LoadSession(scopeID string, limit int) []SessionEntry // limit <= 0 means all
	ClearSession(scopeID string)
	SummarizeSession(scopeID, summary string)
}

// LogStore handles execution progress logs.
type LogStore interface {
	AppendLog(scopeID, runID string, entry LogEntry)
	LoadLogs(scopeID string) []LogEntry
}

// NoteStore handles key-value state.
type NoteStore interface {
	SetNote(scopeID, key string, value []byte)
	GetNote(scopeID, key string) ([]byte, bool)
	DeleteNote(scopeID, key string)
	AllNotes(scopeID string) map[string][]byte
}

// MetricStore handles metrics recording and querying.
type MetricStore interface {
	RecordMetric(runID, scopeID, agentID, name string, value float64) error
	MetricsByScope(scopeID string) ([]MetricRow, error)
	MetricsByAgent(agentID string) ([]MetricRow, error)
	MetricsByRun(runID string) ([]MetricRow, error)
}

// KnowhowStore handles accumulated learning entries.
type KnowhowStore interface {
	SaveKnowhow(deskID, content string) error
	LoadKnowhow(deskID string, limit int) []KnowhowEntry
	PruneKnowhow(deskID string, keepLatest int) error
}

// KnowhowEntry is a single piece of accumulated learning.
type KnowhowEntry struct {
	DeskID  string    `json:"desk_id"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

// Store is the composite interface for backward compatibility.
type Store interface {
	SessionStore
	LogStore
	NoteStore
	MetricStore
	KnowhowStore
}

// SessionEntry is one turn in a conversation history.
// Content is the human-readable message (displayed in UI).
// Meta holds structured context fields for programmatic access (filtering, querying).
type SessionEntry struct {
	SourceID string            `json:"source_id,omitempty"` // originating desk (for group-scope)
	RunID    string            `json:"run_id,omitempty"`
	Role     string            `json:"role"`    // "user" | "assistant" | "system" | "agent"
	Type     string            `json:"type,omitempty"` // "llm" | "script" | "resource" etc.
	Content  string            `json:"content"`
	Meta     map[string]any    `json:"meta,omitempty"` // structured context: trigger, skills, resources, etc.
	At       time.Time         `json:"at"`
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

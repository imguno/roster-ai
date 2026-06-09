package observe

import "time"

type EventType string

const (
	EventStepStarted        EventType = "step.started"
	EventStepCompleted      EventType = "step.completed"
	EventStepFailed         EventType = "step.failed"
	EventStepSkipped        EventType = "step.skipped"
	EventHumanInputWaiting  EventType = "human.waiting"
	EventHumanInputReceived EventType = "human.received"
)

// Event is a single observation emitted during execution.
type Event struct {
	RunID  string    `json:"run_id,omitempty"`
	StepID string    `json:"step_id,omitempty"` // desk or group ID
	Type   EventType `json:"type"`
	At     time.Time `json:"at"`

	DurationMs int64 `json:"duration_ms,omitempty"`

	// LLM usage
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Model        string `json:"model,omitempty"`

	// Artifact summary
	InputBytes  int `json:"input_bytes,omitempty"`
	OutputBytes int `json:"output_bytes,omitempty"`

	// Output holds a truncated preview of the step's output artifact (up to 2048 bytes).
	// Only populated on step.completed events.
	Output string `json:"output,omitempty"`

	Error string `json:"error,omitempty"`

	// Metrics holds arbitrary key-value metrics reported by executors or external tools.
	// Keys are metric names (e.g. "tokens", "cost", "lines_changed"), values are numeric.
	// Scripts report via stderr: METRIC:{"tokens":1234,"cost":0.05}
	// External tools report via POST /api/metrics.
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

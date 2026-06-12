package observe

import "time"

type EventType string

const (
	EventStepStarted   EventType = "step.started"
	EventStepCompleted EventType = "step.completed"
	EventStepFailed    EventType = "step.failed"
	EventStepLog       EventType = "step.log"

	EventHumanInputWaiting  EventType = "human.waiting"
	EventHumanInputReceived EventType = "human.received"

	EventQueuePushed    EventType = "queue.pushed"
	EventQueueRecovered EventType = "queue.recovered"
	EventQueueCollapsed EventType = "queue.collapsed"
	EventQueueGC        EventType = "queue.gc"

	EventHubStarted    EventType = "hub.started"
	EventHubReloaded   EventType = "hub.reloaded"
	EventPublished     EventType = "event.published"
	EventMetrics       EventType = "metrics.reported"
	EventEmitRejected  EventType = "emit.rejected"
	EventStepTimedOut  EventType = "step.timed_out"
	EventLoopBreaker   EventType = "loop.breaker"
)

// Event is a single observation emitted during execution.
type Event struct {
	RunID  string    `json:"run_id,omitempty"`
	StepID string    `json:"step_id,omitempty"` // desk ID
	Type   EventType `json:"type"`
	At     time.Time `json:"at"`

	DurationMs int64 `json:"duration_ms,omitempty"`

	// LLM usage
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Model        string `json:"model,omitempty"`

	// Output summary
	InputBytes  int `json:"input_bytes,omitempty"`
	OutputBytes int `json:"output_bytes,omitempty"`

	// Input holds a truncated preview of the step's input (up to 512 bytes).
	Input string `json:"input,omitempty"`

	// Output holds a truncated preview of the step's output (up to 2048 bytes).
	Output string `json:"output,omitempty"`

	Error string `json:"error,omitempty"`

	// Metrics holds arbitrary key-value metrics.
	Metrics map[string]float64 `json:"metrics,omitempty"`

	// LogType categorizes a step.log event: "step" (progress) or "result" (final output).
	LogType string `json:"log_type,omitempty"`
	// LogContent holds the log message text for step.log events.
	LogContent string `json:"log_content,omitempty"`
}

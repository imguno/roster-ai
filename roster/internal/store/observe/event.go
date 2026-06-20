package observe

import "time"

type EventType string

const (
	EventDeskStarted   EventType = "desk.started"
	EventDeskCompleted EventType = "desk.completed"
	EventDeskFailed    EventType = "desk.failed"
	EventDeskLog       EventType = "desk.log"
	EventDeskTimedOut  EventType = "desk.timed_out"

	EventHumanWaiting  EventType = "human.waiting"
	EventHumanReceived EventType = "human.received"

	EventQueuePushed    EventType = "queue.pushed"
	EventQueueRecovered EventType = "queue.recovered"
	EventQueueCollapsed EventType = "queue.collapsed"
	EventQueueGC        EventType = "queue.gc"

	EventHubStarted  EventType = "hub.started"
	EventHubReloaded EventType = "hub.reloaded"
	EventPublished   EventType = "event.published"
	EventMetrics     EventType = "metrics.reported"
	EventEmitRejected EventType = "emit.rejected"
	EventLoopBreaker EventType = "loop.breaker"

	// Deprecated aliases — kept for backward compatibility with existing log parsers.
	EventStepStarted   = EventDeskStarted
	EventStepCompleted = EventDeskCompleted
	EventStepFailed    = EventDeskFailed
	EventStepLog       = EventDeskLog
	EventStepTimedOut  = EventDeskTimedOut
	EventHumanInputWaiting  = EventHumanWaiting
	EventHumanInputReceived = EventHumanReceived
)

// Event is a single observation emitted during execution.
type Event struct {
	RunID  string    `json:"run_id,omitempty"`
	DeskID string    `json:"desk_id,omitempty"`
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

	// Input holds a truncated preview of the input (up to 512 bytes).
	Input string `json:"input,omitempty"`

	// Output holds a truncated preview of the output (up to 2048 bytes).
	Output string `json:"output,omitempty"`

	Error string `json:"error,omitempty"`

	// Metrics holds arbitrary key-value metrics.
	Metrics map[string]float64 `json:"metrics,omitempty"`

	// LogType categorizes a desk.log event: "progress" or "result".
	LogType string `json:"log_type,omitempty"`
	// LogContent holds the log message text.
	LogContent string `json:"log_content,omitempty"`
}

package types

// Event is the universal message in Roster.
// Everything that flows through the system is an event — a new task arriving,
// a desk finishing its work, a resource changing, a person responding.
// Desks subscribe to events. Desks emit events. That's the whole communication model.
type Event struct {
	// ID uniquely identifies this event instance. Set by the hub on emit if empty.
	// Used for deduplication — the same event ID will not be enqueued twice
	// for the same subscriber, even if multiple routing paths deliver it.
	ID string `yaml:"id,omitempty" json:"id,omitempty"`

	// Type identifies the event (e.g. "task.created", "review.done", "pr.opened").
	Type string `yaml:"type" json:"type"`

	// Source is what produced this event (desk ID, resource name, or "system").
	Source string `yaml:"source,omitempty" json:"source,omitempty"`

	// Payload carries the event data. Opaque to the routing layer.
	Payload []byte `yaml:"-" json:"payload,omitempty"`
}

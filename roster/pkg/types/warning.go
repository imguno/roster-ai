package types

// Warning represents a configuration or runtime issue detected by the hub.
type Warning struct {
	Level   string `json:"level"`   // severity: "warn", "error"
	Source  string `json:"source"`  // origin, e.g. "agent:architect"
	Message string `json:"message"` // human-readable description
}

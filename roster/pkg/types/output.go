package types

// Output is the result of executing a desk.
// It replaces the old Artifact type — desk outputs are plain text,
// not structured artifacts. Structured data sharing happens via resources.
type Output struct {
	Content string             `json:"content"`
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

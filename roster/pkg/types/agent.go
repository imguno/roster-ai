package types

// Agent defines who an agent is — identity and logic.
// This is the unit sold/shared on the marketplace.
// All execution concerns (executor, subscribe, emit) live in Desk.
type Agent struct {
	Kind        Kind     `yaml:"kind" json:"kind"`
	ID          string   `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	SDK         string   `yaml:"sdk,omitempty" json:"sdk,omitempty"`
	Skills      []string `yaml:"skills,omitempty" json:"skills,omitempty"`
}

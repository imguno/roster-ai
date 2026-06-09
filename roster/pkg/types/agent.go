package types

// Agent defines who a person is — identity, role, and skills.
// All execution concerns (executor, concurrency, channels) live in Desk.
type Agent struct {
	Kind        Kind     `yaml:"kind" json:"kind"`
	ID          string   `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Skills      []string `yaml:"skills,omitempty" json:"skills,omitempty"`
	Knowhow     []string `yaml:"knowhow,omitempty" json:"knowhow,omitempty"`
}

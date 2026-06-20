package types

// Group is a session sharing scope. It does not receive events or execute anything.
// Desks declare membership via their `groups` field. A group can nest inside another
// group via the `parent` field, widening the session sharing boundary.
type Group struct {
	Kind        Kind     `yaml:"kind" json:"kind"`
	ID          string   `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Parent      string   `yaml:"parent,omitempty" json:"parent,omitempty"`
	Resources   []string `yaml:"resources,omitempty" json:"resources,omitempty"`
}

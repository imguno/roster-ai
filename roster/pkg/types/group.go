package types

// Group is a team container. Desks declare membership via their `parent` field.
type Group struct {
	Kind        Kind     `yaml:"kind" json:"kind"`
	ID          string   `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Parent      string   `yaml:"parent,omitempty" json:"parent,omitempty"`
	Resources   []string `yaml:"resources,omitempty" json:"resources,omitempty"`
	Subscribe   []string `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`
	Emit        []string `yaml:"emit,omitempty" json:"emit,omitempty"`
}

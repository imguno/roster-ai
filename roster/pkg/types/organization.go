package types

// Org is the root container — always running, always ready.
// There is exactly one Org per project. Children (groups, desks) declare
// their membership via the `parent` field.
type Org struct {
	Kind        Kind        `yaml:"kind" json:"kind"`
	ID          string      `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string      `yaml:"name,omitempty" json:"name,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Resources   []string    `yaml:"resources,omitempty" json:"resources,omitempty"`
	Store       StoreConfig `yaml:"store,omitempty" json:"store,omitempty"`
	Subscribe   []string    `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`
	Emit        []string    `yaml:"emit,omitempty" json:"emit,omitempty"`
}

// Organization is kept as an alias for backwards compatibility with v1 configs.
type Organization = Org

// StoreConfig configures which storage backend the hub uses.
type StoreConfig struct {
	Backend string `yaml:"backend,omitempty" json:"backend,omitempty"`
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`
}

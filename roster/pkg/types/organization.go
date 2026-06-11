package types

// RoutingRule declares a top-level org routing rule (declarative documentation;
// actual subscription is set on each group/desk via their `subscribe:` field).
type RoutingRule struct {
	On   string `yaml:"on" json:"on"`
	To   string `yaml:"to" json:"to"`
	When string `yaml:"when,omitempty" json:"when,omitempty"`
}

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

	// Event subscriptions: which event types this org listens to.
	Subscribe []string `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`
	// Event emissions: which event types this org declares as outputs.
	Emit []string `yaml:"emit,omitempty" json:"emit,omitempty"`

	// Defaults define fallback values applied to all desks in the org.
	Defaults *DeskDefaults `yaml:"defaults,omitempty" json:"defaults,omitempty"`

	// Groups lists group IDs that belong to this org (declarative; membership
	// is enforced via each group/desk's parent field at runtime).
	Groups []string `yaml:"groups,omitempty" json:"groups,omitempty"`
	// Routing declares org-level event routing rules (declarative documentation).
	// Actual subscriptions are configured on each group/desk via their subscribe field.
	Routing []RoutingRule `yaml:"routing,omitempty" json:"routing,omitempty"`
}

// Organization is kept as an alias for backwards compatibility with v1 configs.
type Organization = Org

// DeskDefaults holds default values inherited by desks when not explicitly set.
type DeskDefaults struct {
	Executor *ExecutorConfig `yaml:"executor,omitempty" json:"executor,omitempty"`
	Policy   string          `yaml:"policy,omitempty" json:"policy,omitempty"`
	Tags     []string        `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// StoreConfig configures which storage backend the hub uses.
type StoreConfig struct {
	Backend string `yaml:"backend,omitempty" json:"backend,omitempty"`
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`
}

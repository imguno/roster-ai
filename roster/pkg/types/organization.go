package types

// Organization is the top-level system — always running, always ready.
// It defines which groups exist, which resources they connect to,
// and how events route between them.
//
// Example:
//
//	kind: organization
//	name: engineering
//
//	groups:
//	  - strategy-team
//	  - dev-team
//	  - ops-team
//
//	routing:
//	  - on: task.created
//	    to: strategy-team
//	  - on: plan.ready
//	    to: dev-team
//	  - on: code.ready
//	    to: ops-team
type Organization struct {
	Kind        Kind           `yaml:"kind" json:"kind"`
	ID          string         `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string         `yaml:"name,omitempty" json:"name,omitempty"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Groups      []string       `yaml:"groups,omitempty" json:"groups,omitempty"`
	Resources   []string       `yaml:"resources,omitempty" json:"resources,omitempty"`
	Routing     []RoutingRule  `yaml:"routing,omitempty" json:"routing,omitempty"`
	Store       StoreConfig    `yaml:"store,omitempty" json:"store,omitempty"`

	// Defaults define fallback values applied to all desks in the organization.
	// Desks only need to specify fields that differ from these defaults.
	Defaults *DeskDefaults `yaml:"defaults,omitempty" json:"defaults,omitempty"`
}

// DeskDefaults holds default values inherited by desks when not explicitly set.
// Applied at organization level and optionally overridden at group level.
//
// Example:
//
//	defaults:
//	  executor:
//	    type: exec
//	    params:
//	      command: scripts/claude-code.sh
//	    env:
//	      CLAUDE_MODEL: claude-sonnet-4-6
//	  policy: standard
//	  tags: [team-member]
type DeskDefaults struct {
	Executor *ExecutorConfig `yaml:"executor,omitempty" json:"executor,omitempty"`
	Policy   string          `yaml:"policy,omitempty" json:"policy,omitempty"`
	Tags     []string        `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// RoutingRule maps an event type to a target group (or desk).
type RoutingRule struct {
	On   string `yaml:"on" json:"on"`
	To   string `yaml:"to" json:"to"`
	When string `yaml:"when,omitempty" json:"when,omitempty"`
}

// StoreConfig configures which storage backend the hub uses.
// Defaults to "file" if omitted.
type StoreConfig struct {
	// Backend selects the storage implementation: "file" (default), "sqlite", "memory".
	Backend string `yaml:"backend,omitempty" json:"backend,omitempty"`
	// Path is the data directory (for file backend) or database file path (for sqlite).
	// Defaults to ".roster/data" relative to project dir.
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
}

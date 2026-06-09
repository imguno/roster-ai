package types

// Resource is a connection to an external system. It has two sides:
//
// Watch — emits events when something changes (GitHub PR opened, Figma updated, etc.).
// Actions — desks can request operations, backed by scripts or skills.
//
// Permissions are rule-based — match by desk ID, group ID, or tag:
//
//	permissions:
//	  - allow: [commit, read]
//	    desks: [backend-a, backend-b]
//	  - allow: [commit, read, review, deploy]
//	    groups: [dev-team]
//	  - allow: [read]
//	    tags: [viewer]
//	  - allow: ["*"]
//	    desks: [admin]
type Resource struct {
	Kind        Kind              `yaml:"kind" json:"kind"`
	ID          string            `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string            `yaml:"name,omitempty" json:"name,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Type        string            `yaml:"type,omitempty" json:"type,omitempty"`
	Config      map[string]string `yaml:"config,omitempty" json:"config,omitempty"`

	Watch   []string                   `yaml:"watch,omitempty" json:"watch,omitempty"`
	Actions map[string]*ResourceAction `yaml:"actions,omitempty" json:"actions,omitempty"`

	// Permissions is a list of rules. Each rule grants actions to desks/groups/tags.
	// If empty, all actions are open to everyone.
	Permissions []PermissionRule `yaml:"permissions,omitempty" json:"permissions,omitempty"`

	// Polling interval for watch (e.g. "5m"). Only used when watch is non-empty.
	Interval string `yaml:"interval,omitempty" json:"interval,omitempty"`
}

// PermissionRule grants a set of actions to matched desks, groups, or tags.
// "*" in Allow means all actions.
type PermissionRule struct {
	Allow  []string `yaml:"allow" json:"allow"`
	Desks  []string `yaml:"desks,omitempty" json:"desks,omitempty"`
	Groups []string `yaml:"groups,omitempty" json:"groups,omitempty"`
	Tags   []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// ResourceAction is a named operation that desks can invoke on a resource.
type ResourceAction struct {
	Exec        string            `yaml:"exec,omitempty" json:"exec,omitempty"`
	Skill       string            `yaml:"skill,omitempty" json:"skill,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Params      map[string]string `yaml:"params,omitempty" json:"params,omitempty"`
}

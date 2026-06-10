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

	Watch     []string                   `yaml:"watch,omitempty" json:"watch,omitempty"`
	Actions   map[string]*ResourceAction `yaml:"actions,omitempty" json:"actions,omitempty"`

	// Subscribe wires this resource as an event handler.
	// Each entry is an event type that triggers the named action.
	// e.g. "hello.done -> write" means: on hello.done, run the write action
	// with the event payload as ROSTER_EVENT_PAYLOAD env var.
	Subscribe []ResourceSubscription `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`

	// SDK is the sdk: reference for SDK-backed resources (e.g. "local:../sdk").
	SDK      string         `yaml:"sdk,omitempty" json:"sdk,omitempty"`
	Executor *ExecutorConfig `yaml:"executor,omitempty" json:"executor,omitempty"`

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

// ResourceSubscription wires a resource action to fire on a specific event type.
type ResourceSubscription struct {
	On     string `yaml:"on" json:"on"`         // event type to listen for
	Action string `yaml:"action" json:"action"` // action name to execute
}

// ResourceAction is a named operation that desks can invoke on a resource.
type ResourceAction struct {
	Exec        string            `yaml:"exec,omitempty" json:"exec,omitempty"`
	Skill       string            `yaml:"skill,omitempty" json:"skill,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Params      map[string]string `yaml:"params,omitempty" json:"params,omitempty"`
}

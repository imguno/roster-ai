package types

// Resource is a connection to an external system.
// It is pure configuration — no logic, no actions, no SDK.
// The agent reads resource config and handles all interaction itself.
//
// Supported resource types:
//
//	type: mcp      — MCP server (mcp: field holds the command to start it)
//	type: local    — local filesystem path
//	type: remote   — remote API or service (connection: field holds the URL/DSN)
//	(any string)   — user-defined type, config is passed as-is to the agent
type Resource struct {
	Kind        Kind              `yaml:"kind" json:"kind"`
	ID          string            `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string            `yaml:"name,omitempty" json:"name,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Type        string            `yaml:"type,omitempty" json:"type,omitempty"`

	// MCP is the command used to start an MCP server (e.g. "npx @modelcontextprotocol/server-figma").
	MCP string `yaml:"mcp,omitempty" json:"mcp,omitempty"`

	// Connection is a DSN or URL for database / remote API resources.
	Connection string `yaml:"connection,omitempty" json:"connection,omitempty"`

	// Config is arbitrary key-value configuration passed to the agent at runtime.
	Config map[string]string `yaml:"config,omitempty" json:"config,omitempty"`

	// Watch lists event types this resource emits when it detects external changes.
	Watch []string `yaml:"watch,omitempty" json:"watch,omitempty"`

	// Interval controls how often the watcher polls (e.g. "5m").
	Interval string `yaml:"interval,omitempty" json:"interval,omitempty"`
}

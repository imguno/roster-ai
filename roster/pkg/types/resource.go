package types

// Resource is a connection to an external system.
// It is pure configuration — no logic, no actions, no SDK.
// The agent reads resource config and handles all interaction itself.
type Resource struct {
	Kind        Kind              `yaml:"kind" json:"kind"`
	ID          string            `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string            `yaml:"name,omitempty" json:"name,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Type        string            `yaml:"type,omitempty" json:"type,omitempty"`
	MCP         string            `yaml:"mcp,omitempty" json:"mcp,omitempty"`
	Connection  string            `yaml:"connection,omitempty" json:"connection,omitempty"`
	Config      map[string]string `yaml:"config,omitempty" json:"config,omitempty"`
}

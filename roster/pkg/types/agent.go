package types

// Agent defines who an agent is — identity, model, skills, and event contract.
// This is the unit sold/shared on the marketplace.
// All execution concerns (executor, concurrency) live in Desk.
type Agent struct {
	Kind        Kind     `yaml:"kind" json:"kind"`
	ID          string   `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Model       string   `yaml:"model,omitempty" json:"model,omitempty"`
	// SDK is the package name or local path of the gRPC agent process.
	// e.g. "@myorg/ui-designer", "./greeter-sdk", "roster-greeter"
	SDK         string   `yaml:"sdk,omitempty" json:"sdk,omitempty"`
	Skills      []string `yaml:"skills,omitempty" json:"skills,omitempty"`
	Resources   []string `yaml:"resources,omitempty" json:"resources,omitempty"`
	Knowhow     []string `yaml:"knowhow,omitempty" json:"knowhow,omitempty"`

	// Subscribe/Emit declare the agent's event contract.
	// When placed on a Desk, the desk inherits these if not overridden.
	Subscribe []string `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`
	Emit      []string `yaml:"emit,omitempty" json:"emit,omitempty"`
}

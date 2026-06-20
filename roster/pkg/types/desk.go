package types

import "fmt"

// Desk is the execution unit — one agent, one job, one set of events.
// It can belong to multiple groups via the `groups` field.
type Desk struct {
	Kind        Kind           `yaml:"kind" json:"kind"`
	ID          string         `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string         `yaml:"name,omitempty" json:"name,omitempty"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Groups      []string       `yaml:"groups,omitempty" json:"groups,omitempty"`
	Agent       AgentRef       `yaml:"agent,omitempty" json:"agent,omitempty"`
	SourcePath  string         `yaml:"-" json:"-"`
	Executor    ExecutorConfig `yaml:"executor" json:"executor"`

	Subscribe []string `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`
	Emit      []string `yaml:"emit,omitempty" json:"emit,omitempty"`

	Role      string        `yaml:"role,omitempty" json:"role,omitempty"`
	Goal      string        `yaml:"goal,omitempty" json:"goal,omitempty"`
	Skills    []string      `yaml:"skills,omitempty" json:"skills,omitempty"`
	Resources []string      `yaml:"resources,omitempty" json:"resources,omitempty"`
	Session   SessionConfig `yaml:"session,omitempty" json:"session,omitempty"`
	Timeout   string        `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Budget    BudgetConfig  `yaml:"budget,omitempty" json:"budget,omitempty"`
}

// PrimaryGroup returns the first group ID, or empty string if no groups.
// Convenience for backward compatibility during migration.
func (d *Desk) PrimaryGroup() string {
	if len(d.Groups) > 0 {
		return d.Groups[0]
	}
	return ""
}

// BelongsTo checks if the desk is a member of the given group.
func (d *Desk) BelongsTo(groupID string) bool {
	for _, g := range d.Groups {
		if g == groupID {
			return true
		}
	}
	return false
}

// AgentRef is either a local agent ID (string) or a remote agent spec (object).
//
//	agent: developer            # local
//	agent:                      # remote
//	  type: remote
//	  address: api.vendor.io/agents/ux-v1
//	  api_key: ${KEY}
type AgentRef struct {
	ID      string `yaml:"-" json:"id,omitempty"`
	Type    string `yaml:"type,omitempty" json:"type,omitempty"`
	Address string `yaml:"address,omitempty" json:"address,omitempty"`
	APIKey  string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}

func (a *AgentRef) IsRemote() bool { return a.Type == "remote" }
func (a *AgentRef) IsLocal() bool  { return a.ID != "" }

func (a *AgentRef) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var id string
	if err := unmarshal(&id); err == nil {
		a.ID = id
		return nil
	}
	type agentRefAlias AgentRef
	var alias agentRefAlias
	if err := unmarshal(&alias); err != nil {
		return fmt.Errorf("agent: expected string or object: %w", err)
	}
	*a = AgentRef(alias)
	return nil
}

// ExecutorType identifies the execution backend.
type ExecutorType string

const (
	ExecutorTypeAPI    ExecutorType = "api"
	ExecutorTypeExec   ExecutorType = "exec"
	ExecutorTypeDocker ExecutorType = "docker"
	ExecutorTypeRemote ExecutorType = "remote"
	ExecutorTypeHuman  ExecutorType = "human"
	ExecutorTypeSDK    ExecutorType = "sdk"
)

// SDKType identifies which built-in AI SDK to use when executor type is "api".
type SDKType string

const (
	SDKAnthropic SDKType = "anthropic"
	SDKOpenAI    SDKType = "openai"
	SDKGemini    SDKType = "gemini"
)

// ExecutorConfig defines how a desk executes tasks.
type ExecutorConfig struct {
	Type    ExecutorType      `yaml:"type" json:"type"`
	SDK     SDKType           `yaml:"sdk,omitempty" json:"sdk,omitempty"`
	Address string            `yaml:"address,omitempty" json:"address,omitempty"`
	Params  map[string]string `yaml:"params,omitempty" json:"params,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

// SessionConfig controls session history behavior for a desk.
type SessionConfig struct {
	MaxEntries *int `yaml:"max_entries,omitempty" json:"max_entries,omitempty"`
	MaxKnowhow *int `yaml:"max_knowhow,omitempty" json:"max_knowhow,omitempty"`
}

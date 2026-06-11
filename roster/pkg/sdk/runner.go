package sdk

import (
	"context"

	"github.com/roster-io/roster/pkg/types"
)

// Executor is the interface every execution backend must implement.
// Community members implement this to connect any AI, CLI, or service to Roster.
type Executor interface {
	Run(ctx context.Context, task Task) (*types.Artifact, error)
}

// Task is the unit of work handed to an Executor.
// The hub resolves all skill refs and builds Prompt before dispatching,
// so executors receive a ready-to-use prompt without knowing about skill resolution.
type Task struct {
	RunID     string
	AgentID   string
	DeskID    string
	GroupID   string            // empty if desk is not inside a group
	EventType string            // the event type that triggered this desk
	Prompt    string            // skill prompts merged + input context — ready to send to any AI or CLI
	Input     *types.Artifact   // nil for the first step
	Options   map[string]string // executor configuration (command, image, sdk, etc.)
	Env       map[string]string // environment variables to inject into the subprocess
	WorkDir   string            // working directory for exec runner (project dir)

	// Notes is the current note store snapshot for this scope (desk or group).
	Notes map[string][]byte

	// Session is the desk's persistent conversation history.
	// Executors may include this as prior context when calling their AI backend.
	Session []SessionEntry

	// GroupHistory is the shared communication log for the active group, if any.
	// Lets group members see what teammates have said before acting.
	GroupHistory []GroupMessage

	// Resources lists the resources available to this desk with their config.
	// The agent reads Config to connect to the external system itself.
	Resources []TaskResource

	// Skills maps skill name → resolved prompt content.
	Skills map[string]string
}

// TaskResource describes a resource available to the desk during execution.
type TaskResource struct {
	ID     string            // resource ID
	Type   string            // resource type (mcp, local, remote, etc.)
	Config map[string]string // resource configuration (path, connection, etc.)
}

// SessionEntry is one turn in a desk's persistent session.
type SessionEntry struct {
	Role    string // "user" or "assistant" or "system"
	Content string
}

// GroupMessage is one message in a group's shared communication space.
type GroupMessage struct {
	DeskID  string
	Role    string
	Content string
}

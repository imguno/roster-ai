package types

// Desk is an independent actor. It receives events, does one job, and emits events.
// The agent field defines *who* sits at this desk. The executor defines *how* it runs.
type Desk struct {
	Kind        Kind              `yaml:"kind" json:"kind"`
	ID          string            `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string            `yaml:"name,omitempty" json:"name,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Agent       string            `yaml:"-" json:"agent,omitempty"`
	SourcePath  string            `yaml:"-" json:"-"`
	Executor    ExecutorConfig    `yaml:"executor" json:"executor"`
	Concurrency ConcurrencyConfig `yaml:"concurrency,omitempty" json:"concurrency,omitempty"`

	// Event subscriptions: which event types this desk listens to.
	Subscribe []string `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`
	// Event emissions: which event types this desk produces on completion.
	Emit []string `yaml:"emit,omitempty" json:"emit,omitempty"`

	// Cron schedule expression (e.g. "*/30 * * * *" = every 30 minutes).
	// When set, the desk auto-triggers on this schedule.
	Cron string `yaml:"cron,omitempty" json:"cron,omitempty"`

	// Resources bound to this desk (private — only this desk can access).
	Resources []string `yaml:"resources,omitempty" json:"resources,omitempty"`

	// Tags for role-based permission matching (e.g. ["backend", "senior"]).
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// Policy reference (optional).
	Policy string `yaml:"policy,omitempty" json:"policy,omitempty"`

	// Triggers define automated event sources for this desk.
	Triggers []TriggerConfig `yaml:"triggers,omitempty" json:"triggers,omitempty"`

	// Session configures this desk's session behavior.
	Session SessionConfig `yaml:"session,omitempty" json:"session,omitempty"`
}

// ExecutorType identifies the execution backend.
type ExecutorType string

const (
	ExecutorTypeAPI    ExecutorType = "api"    // built-in SDK (anthropic, openai, gemini)
	ExecutorTypeExec   ExecutorType = "exec"   // arbitrary command via stdin/stdout
	ExecutorTypeDocker ExecutorType = "docker" // docker container
	ExecutorTypeRemote ExecutorType = "remote" // remote worker via gRPC (operator-controlled)
	ExecutorTypeHuman  ExecutorType = "human"  // human participant — produces output via web UI
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

// ConcurrencyMode defines how the hub handles simultaneous requests to this desk.
type ConcurrencyMode string

const (
	ConcurrencyQueue  ConcurrencyMode = "queue"  // queue requests (default)
	ConcurrencySpawn  ConcurrencyMode = "spawn"  // spawn parallel workers up to Max
	ConcurrencyReject ConcurrencyMode = "reject" // reject when busy
)

// TriggerConfig defines an automated event source.
type TriggerConfig struct {
	// Type: "exec", "poll". Cron uses the existing desk.cron field.
	Type string `yaml:"type" json:"type"`
	// Exec: command to run. Fires event if exit code 0.
	Command string `yaml:"command,omitempty" json:"command,omitempty"`
	// Poll: URL to GET. Fires event if status 200.
	URL string `yaml:"url,omitempty" json:"url,omitempty"`
	// Interval between checks (default: "30s").
	Interval string `yaml:"interval,omitempty" json:"interval,omitempty"`
	// Event type to emit when triggered.
	Event string `yaml:"event,omitempty" json:"event,omitempty"`
}

// ConcurrencyConfig declares how the hub manages simultaneous calls to this desk.
type ConcurrencyConfig struct {
	Mode ConcurrencyMode `yaml:"mode,omitempty" json:"mode,omitempty"`
	Max  int             `yaml:"max,omitempty" json:"max,omitempty"`
}

// SessionConfig controls session history behavior for a desk.
type SessionConfig struct {
	// MaxEntries limits how many session entries are loaded as context.
	// Default: 40 (from store's maxSessionEntries constant).
	// Set to 0 to disable session history entirely.
	MaxEntries *int `yaml:"max_entries,omitempty" json:"max_entries,omitempty"`
}

// CronInfo represents a cron schedule's status.
type CronInfo struct {
	ID      string `json:"id"`
	Cron    string `json:"cron"`
	Type    string `json:"type"` // "desk" or "group"
	NextRun string `json:"next_run,omitempty"`
	LastRun string `json:"last_run,omitempty"`
}

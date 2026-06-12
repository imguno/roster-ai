package types

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
	Subscribe   []string    `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`
	Emit        []string    `yaml:"emit,omitempty" json:"emit,omitempty"`
	Cron        []CronEntry `yaml:"cron,omitempty" json:"cron,omitempty"`
	Limits      LoopLimits  `yaml:"limits,omitempty" json:"limits,omitempty"`
}

// CronEntry schedules periodic event emission.
type CronEntry struct {
	Schedule string `yaml:"schedule" json:"schedule"` // cron expression (e.g. "*/30 * * * *")
	Event    string `yaml:"event" json:"event"`       // event type to emit
	Payload  string `yaml:"payload,omitempty" json:"payload,omitempty"`
}

// Organization is kept as an alias for backwards compatibility with v1 configs.
type Organization = Org

// LoopLimits configures circuit-breaker thresholds for self-improvement cycles.
type LoopLimits struct {
	MaxIterations int    `yaml:"max_iterations,omitempty" json:"max_iterations,omitempty"` // max times any event type can fire before tripping
	Cooldown      string `yaml:"cooldown,omitempty" json:"cooldown,omitempty"`             // duration to block after tripping (e.g. "5m")
}

// StoreConfig configures which storage backend the hub uses.
type StoreConfig struct {
	Backend string `yaml:"backend,omitempty" json:"backend,omitempty"`
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`
}

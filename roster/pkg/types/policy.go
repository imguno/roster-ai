package types

import "time"

// Policy is a set of operational rules attached to a desk or group.
//
// Example:
//
//	kind: policy
//	name: careful
//	retry: 3
//	timeout: 5m
//	cost_limit: $0.10
type Policy struct {
	Kind        Kind   `yaml:"kind"`
	ID          string `yaml:"id,omitempty"`
	Name        string `yaml:"name,omitempty"`
	Description string `yaml:"description,omitempty"`

	Retry      int           `yaml:"retry,omitempty"`        // max retry attempts on failure
	Timeout    time.Duration `yaml:"timeout,omitempty"`      // max execution time
	CostLimit  string        `yaml:"cost_limit,omitempty"`   // max cost per invocation (e.g. "$0.10")
	EscalateTo string        `yaml:"escalate_to,omitempty"`  // desk ID to escalate to on failure
	OnTimeout  string        `yaml:"on_timeout,omitempty"`   // "fail" (default), "retry", "escalate"
	OnError    string        `yaml:"on_error,omitempty"`     // "fail" (default), "retry", "escalate"

	// Budget defines cost limits at multiple granularities.
	// When a group has a budget, all member desk costs roll up.
	Budget BudgetConfig `yaml:"budget,omitempty" json:"budget,omitempty"`

	// EscalationChain defines multi-level escalation: L1 → L2 → L3.
	// Each entry is a desk ID. On repeated failure, escalation walks up the chain.
	EscalationChain []string `yaml:"escalation_chain,omitempty" json:"escalation_chain,omitempty"`

	// RequireSchema enforces that artifacts produced under this policy
	// must match the declared schema (e.g. "json-v1", "code-v1").
	RequireSchema string `yaml:"require_schema,omitempty" json:"require_schema,omitempty"`
}

// BudgetConfig defines cost limits at multiple granularities.
//
// Example:
//
//	budget:
//	  total: "$500.00"
//	  per_run: "$5.00"
//	  daily: "$50.00"
//	  warn_at: 0.8
type BudgetConfig struct {
	// Total is the cumulative cost limit (lifetime). Format: "$500.00".
	Total string `yaml:"total,omitempty" json:"total,omitempty"`
	// PerRun is the max cost per single run invocation. Format: "$5.00".
	PerRun string `yaml:"per_run,omitempty" json:"per_run,omitempty"`
	// Daily is the max cost per 24-hour rolling window. Format: "$50.00".
	Daily string `yaml:"daily,omitempty" json:"daily,omitempty"`
	// WarnAt is the fraction (0.0–1.0) of any limit at which a warning is emitted.
	// Default: 0 (no warning before limit). Example: 0.8 = warn at 80%.
	WarnAt float64 `yaml:"warn_at,omitempty" json:"warn_at,omitempty"`
}

// IsEmpty reports whether no budget limits are configured.
func (b BudgetConfig) IsEmpty() bool {
	return b.Total == "" && b.PerRun == "" && b.Daily == ""
}

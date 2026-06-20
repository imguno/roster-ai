package types

// BudgetConfig defines spending limits for a desk or organization.
type BudgetConfig struct {
	MaxPerRun  float64 `yaml:"max_per_run,omitempty" json:"max_per_run,omitempty"`   // max USD per execution
	MaxDaily   float64 `yaml:"max_daily,omitempty" json:"max_daily,omitempty"`       // max USD per 24h
	MaxMonthly float64 `yaml:"max_monthly,omitempty" json:"max_monthly,omitempty"`   // max USD per 30d
}

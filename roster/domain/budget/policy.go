// Package budget contains domain logic for cost control.
// No external dependencies — pure Go types and functions.
package budget

// Limit defines spending caps at different granularities.
type Limit struct {
	PerRun  float64 // max USD per single execution
	Daily   float64 // max USD per 24h rolling window
	Monthly float64 // max USD per 30d rolling window
}

// HasLimits returns true if any limit is configured.
func (l Limit) HasLimits() bool {
	return l.PerRun > 0 || l.Daily > 0 || l.Monthly > 0
}

// Violation describes which limit was exceeded.
type Violation struct {
	LimitType string  // "per_run", "daily", "monthly"
	Limit     float64 // the configured cap
	Actual    float64 // the actual or projected amount
	ScopeID   string  // desk or org ID
}

// Evaluate checks whether a proposed cost would violate any limit.
// Returns nil if no limit is violated.
func Evaluate(limit Limit, proposedCost, currentRunTotal, dailyTotal, monthlyTotal float64) *Violation {
	if limit.PerRun > 0 && currentRunTotal+proposedCost > limit.PerRun {
		return &Violation{
			LimitType: "per_run",
			Limit:     limit.PerRun,
			Actual:    currentRunTotal + proposedCost,
		}
	}
	if limit.Daily > 0 && dailyTotal+proposedCost > limit.Daily {
		return &Violation{
			LimitType: "daily",
			Limit:     limit.Daily,
			Actual:    dailyTotal + proposedCost,
		}
	}
	if limit.Monthly > 0 && monthlyTotal+proposedCost > limit.Monthly {
		return &Violation{
			LimitType: "monthly",
			Limit:     limit.Monthly,
			Actual:    monthlyTotal + proposedCost,
		}
	}
	return nil
}

// EvaluatePreRun checks daily/monthly limits before execution starts (no per-run cost yet).
func EvaluatePreRun(limit Limit, dailyTotal, monthlyTotal float64) *Violation {
	if limit.Daily > 0 && dailyTotal >= limit.Daily {
		return &Violation{
			LimitType: "daily",
			Limit:     limit.Daily,
			Actual:    dailyTotal,
		}
	}
	if limit.Monthly > 0 && monthlyTotal >= limit.Monthly {
		return &Violation{
			LimitType: "monthly",
			Limit:     limit.Monthly,
			Actual:    monthlyTotal,
		}
	}
	return nil
}

package hub

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	budgetdomain "github.com/roster-io/roster/domain/budget"
	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/pkg/types"
)

// BudgetTracker tracks accumulated costs at multiple granularities.
type BudgetTracker struct {
	mu     sync.Mutex
	totals map[string]float64     // scopeID → lifetime accumulated cost
	daily  map[string][]costEntry // scopeID → recent cost entries (for daily window)
	perRun map[string]float64     // runID:scopeID → per-run accumulated cost
}

type costEntry struct {
	at   time.Time
	cost float64
}

func newBudgetTracker() *BudgetTracker {
	return &BudgetTracker{
		totals: make(map[string]float64),
		daily:  make(map[string][]costEntry),
		perRun: make(map[string]float64),
	}
}

func (bt *BudgetTracker) Add(scopeID, runID string, cost float64) float64 {
	return bt.AddAt(scopeID, runID, cost, time.Now())
}

// AddAt records a cost entry with an explicit timestamp (used for rehydration).
func (bt *BudgetTracker) AddAt(scopeID, runID string, cost float64, at time.Time) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.totals[scopeID] += cost
	bt.daily[scopeID] = append(bt.daily[scopeID], costEntry{at: at, cost: cost})
	bt.perRun[runID+":"+scopeID] += cost
	return bt.totals[scopeID]
}

func (bt *BudgetTracker) Total(scopeID string) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return bt.totals[scopeID]
}

// DailyTotal returns the total cost within the last 24 hours for a scope.
func (bt *BudgetTracker) DailyTotal(scopeID string) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	cutoff := time.Now().Add(-24 * time.Hour)
	var total float64
	for _, e := range bt.daily[scopeID] {
		if e.at.After(cutoff) {
			total += e.cost
		}
	}
	return total
}

// MonthlyTotal returns the total cost within the last 30 days for a scope.
func (bt *BudgetTracker) MonthlyTotal(scopeID string) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	var total float64
	for _, e := range bt.daily[scopeID] {
		if e.at.After(cutoff) {
			total += e.cost
		}
	}
	return total
}

// RunTotal returns the accumulated cost for a specific run+scope.
func (bt *BudgetTracker) RunTotal(runID, scopeID string) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return bt.perRun[runID+":"+scopeID]
}

func (bt *BudgetTracker) All() map[string]float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	out := make(map[string]float64, len(bt.totals))
	for k, v := range bt.totals {
		out[k] = v
	}
	return out
}

// checkBudgetPreRun checks daily/monthly limits before execution starts.
// Returns error if limits are already exceeded.
func (h *Hub) checkBudgetPreRun(deskID string, desk *types.Desk) error {
	limit := toBudgetLimit(desk.Budget)
	if !limit.HasLimits() {
		return nil
	}
	scope := "desk:" + deskID
	v := budgetdomain.EvaluatePreRun(limit, h.budget.DailyTotal(scope), h.budget.MonthlyTotal(scope))
	if v != nil {
		h.recorder.Record(observe.Event{
			DeskID: deskID,
			Type:   observe.EventStepFailed,
			Error:  fmt.Sprintf("budget %s limit $%.2f already reached ($%.2f)", v.LimitType, v.Limit, v.Actual),
		})
		h.bus.PublishAsync(nil, types.Event{Type: budgetdomain.EventBudgetExceeded, Source: deskID})
		return fmt.Errorf("budget exceeded: %s limit $%.2f, actual $%.2f", v.LimitType, v.Limit, v.Actual)
	}
	return nil
}

// checkBudget records cost and checks per-run limits after execution.
func (h *Hub) checkBudget(runID, deskID string, cost float64) error {
	if cost <= 0 {
		return nil
	}
	scope := "desk:" + deskID
	h.budget.Add(scope, runID, cost)
	for gid := range h.reg.groups {
		if h.deskInGroup(gid, deskID) {
			h.budget.Add("group:"+gid, runID, cost)
		}
	}

	desk := h.reg.desks[deskID]
	if desk == nil {
		return nil
	}
	limit := toBudgetLimit(desk.Budget)
	if !limit.HasLimits() {
		return nil
	}
	v := budgetdomain.Evaluate(limit, cost, h.budget.RunTotal(runID, scope), h.budget.DailyTotal(scope), h.budget.MonthlyTotal(scope))
	if v != nil {
		h.bus.PublishAsync(nil, types.Event{Type: budgetdomain.EventBudgetExceeded, Source: deskID})
		return fmt.Errorf("budget exceeded: %s limit $%.2f, actual $%.2f", v.LimitType, v.Limit, v.Actual)
	}
	return nil
}

// BudgetStatus returns accumulated costs per scope for the API.
func (h *Hub) BudgetStatus() map[string]float64 {
	return h.budget.All()
}

func toBudgetLimit(cfg types.BudgetConfig) budgetdomain.Limit {
	return budgetdomain.Limit{
		PerRun:  cfg.MaxPerRun,
		Daily:   cfg.MaxDaily,
		Monthly: cfg.MaxMonthly,
	}
}

func parseBudgetAmount(s string) float64 {
	s = strings.TrimPrefix(s, "$")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

package hub

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/roster-io/roster/internal/observe"
	"github.com/roster-io/roster/pkg/types"
)

// BudgetTracker tracks accumulated costs at multiple granularities.
// Supports: total (lifetime), per-run, daily (rolling 24h window).
type BudgetTracker struct {
	mu       sync.Mutex
	totals   map[string]float64    // scopeID → lifetime accumulated cost
	daily    map[string][]costEntry // scopeID → recent cost entries (for daily window)
	perRun   map[string]float64    // runID:scopeID → per-run accumulated cost
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

// Add records a cost for a scope and run. Returns the new lifetime total.
func (bt *BudgetTracker) Add(scopeID, runID string, cost float64) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.totals[scopeID] += cost
	bt.daily[scopeID] = append(bt.daily[scopeID], costEntry{at: time.Now(), cost: cost})
	bt.perRun[runID+":"+scopeID] += cost
	return bt.totals[scopeID]
}

// Total returns the lifetime accumulated cost for a scope.
func (bt *BudgetTracker) Total(scopeID string) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return bt.totals[scopeID]
}

// DailyTotal returns the cost in the last 24 hours for a scope.
func (bt *BudgetTracker) DailyTotal(scopeID string) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	cutoff := time.Now().Add(-24 * time.Hour)
	var total float64
	var kept []costEntry
	for _, e := range bt.daily[scopeID] {
		if e.at.After(cutoff) {
			total += e.cost
			kept = append(kept, e)
		}
	}
	bt.daily[scopeID] = kept // prune old entries
	return total
}

// RunTotal returns the accumulated cost for a specific run+scope.
func (bt *BudgetTracker) RunTotal(runID, scopeID string) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return bt.perRun[runID+":"+scopeID]
}

// All returns a snapshot of all tracked costs for the API.
func (bt *BudgetTracker) All() map[string]float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	out := make(map[string]float64, len(bt.totals))
	for k, v := range bt.totals {
		out[k] = v
	}
	return out
}

// checkBudget verifies desk and group budgets at all granularities.
// Emits warning events at warn_at threshold and exceeded events at limits.
func (h *Hub) checkBudget(runID, deskID string, cost float64) {
	if cost <= 0 {
		return
	}

	scope := "desk:" + deskID
	h.budget.Add(scope, runID, cost)

	// Check desk-level policy budget.
	if desk, ok := h.desks[deskID]; ok && desk.Policy != "" {
		if pol, ok := h.policies[desk.Policy]; ok && !pol.Budget.IsEmpty() {
			h.enforceBudget(runID, deskID, scope, pol.Budget)
		}
	}

	// Roll up to group-level budget.
	for gid, group := range h.groups {
		if !h.deskInGroup(gid, deskID) {
			continue
		}
		gScope := "group:" + gid
		h.budget.Add(gScope, runID, cost)

		if group.Policy != "" {
			if pol, ok := h.policies[group.Policy]; ok && !pol.Budget.IsEmpty() {
				h.enforceBudget(runID, gid, gScope, pol.Budget)
			}
		}
	}
}

// enforceBudget checks all three budget levels (total, daily, per_run) and emits events.
func (h *Hub) enforceBudget(runID, stepID, scope string, cfg types.BudgetConfig) {
	// Total (lifetime) budget.
	if cfg.Total != "" {
		limit := parseBudgetAmount(cfg.Total)
		current := h.budget.Total(scope)
		if limit > 0 {
			h.checkLimit(runID, stepID, "total", current, limit, cfg.WarnAt, cfg.Total)
		}
	}

	// Daily (rolling 24h) budget.
	if cfg.Daily != "" {
		limit := parseBudgetAmount(cfg.Daily)
		current := h.budget.DailyTotal(scope)
		if limit > 0 {
			h.checkLimit(runID, stepID, "daily", current, limit, cfg.WarnAt, cfg.Daily)
		}
	}

	// Per-run budget.
	if cfg.PerRun != "" {
		limit := parseBudgetAmount(cfg.PerRun)
		current := h.budget.RunTotal(runID, scope)
		if limit > 0 {
			h.checkLimit(runID, stepID, "per_run", current, limit, cfg.WarnAt, cfg.PerRun)
		}
	}
}

// checkLimit emits warning and exceeded events for a single budget dimension.
func (h *Hub) checkLimit(runID, stepID, dimension string, current, limit, warnAt float64, limitStr string) {
	if current > limit {
		h.recorder.Record(observe.Event{
			RunID:  runID,
			StepID: stepID,
			Type:   observe.EventType("policy.budget_exceeded"),
			Error:  fmt.Sprintf("%s budget: $%.4f exceeds %s limit %s", dimension, current, dimension, limitStr),
		})
	} else if warnAt > 0 && current > limit*warnAt {
		h.recorder.Record(observe.Event{
			RunID:  runID,
			StepID: stepID,
			Type:   observe.EventType("policy.budget_warning"),
			Error:  fmt.Sprintf("%s budget: $%.4f reached %.0f%% of %s limit %s", dimension, current, (current/limit)*100, dimension, limitStr),
		})
	}
}

// checkArtifactSchema verifies that a produced artifact matches the policy's required schema.
func (h *Hub) checkArtifactSchema(runID, deskID, schema string) {
	desk, ok := h.desks[deskID]
	if !ok || desk.Policy == "" {
		return
	}
	pol, ok := h.policies[desk.Policy]
	if !ok || pol.RequireSchema == "" {
		return
	}
	if schema != pol.RequireSchema {
		h.recorder.Record(observe.Event{
			RunID:  runID,
			StepID: deskID,
			Type:   observe.EventType("policy.schema_mismatch"),
			Error:  fmt.Sprintf("expected schema %q, got %q", pol.RequireSchema, schema),
		})
	}
}

// escalateChain walks a multi-level escalation chain.
func escalateChain(chain []string, currentDesk string) string {
	if len(chain) == 0 {
		return ""
	}
	for i, desk := range chain {
		if desk == currentDesk && i+1 < len(chain) {
			return chain[i+1]
		}
	}
	return chain[0]
}

func parseBudgetAmount(s string) float64 {
	s = strings.TrimPrefix(s, "$")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// BudgetStatus returns accumulated costs per scope for the API.
func (h *Hub) BudgetStatus() map[string]float64 {
	return h.budget.All()
}

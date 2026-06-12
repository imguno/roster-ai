package hub

import (
	"strconv"
	"strings"
	"sync"
	"time"
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
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.totals[scopeID] += cost
	bt.daily[scopeID] = append(bt.daily[scopeID], costEntry{at: time.Now(), cost: cost})
	bt.perRun[runID+":"+scopeID] += cost
	return bt.totals[scopeID]
}

func (bt *BudgetTracker) Total(scopeID string) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return bt.totals[scopeID]
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

// checkBudget records cost for a desk and rolls up to parent group.
func (h *Hub) checkBudget(runID, deskID string, cost float64) {
	if cost <= 0 {
		return
	}
	h.budget.Add("desk:"+deskID, runID, cost)
	for gid := range h.groups {
		if h.deskInGroup(gid, deskID) {
			h.budget.Add("group:"+gid, runID, cost)
		}
	}
}

// BudgetStatus returns accumulated costs per scope for the API.
func (h *Hub) BudgetStatus() map[string]float64 {
	return h.budget.All()
}

func parseBudgetAmount(s string) float64 {
	s = strings.TrimPrefix(s, "$")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

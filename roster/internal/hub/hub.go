package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	resourcedomain "github.com/roster-io/roster/domain/resource"
	"github.com/roster-io/roster/internal/event"
	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/internal/event/queue"
	"github.com/roster-io/roster/internal/resource"
	"github.com/roster-io/roster/internal/exec/sdkproc"
	"github.com/roster-io/roster/internal/agent/skill"
	"github.com/roster-io/roster/internal/store"
	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

// Dispatcher routes tasks to executors.
type Dispatcher interface {
	Dispatch(ctx context.Context, t types.ExecutorType, task sdk.Task) (*types.Output, error)
}

// Hub is the event-driven orchestrator.
// Events are queued per subscriber and processed sequentially.
// Queues persist to disk — on restart, unfinished work resumes.
type Hub struct {
	dispatcher Dispatcher
	sessions   store.SessionStore
	logs       store.LogStore
	notes      store.NoteStore
	metrics    store.MetricStore
	knowhow    store.KnowhowStore
	skills     *skill.Resolver
	bus        *event.Bus
	recorder   *observe.Recorder
	reg        *Registry

	projectDir string
	queueDir   string

	mu             sync.RWMutex
	deskEmitters   map[string]*event.DeskEmitter
	queues         map[string]queue.Queue
	humanInputs    map[string]chan string
	runningWorkers map[string]struct{}

	activeRuns   map[string]context.CancelFunc
	activeRunsMu sync.Mutex

	breaker  *CircuitBreaker
	budget   *BudgetTracker
	sdkProcs *sdkproc.ProcessManager
}

func New(dispatcher Dispatcher, s store.Store, skills *skill.Resolver, recorder *observe.Recorder) *Hub {
	return &Hub{
		dispatcher:     dispatcher,
		sessions:       s,
		logs:           s,
		notes:          s,
		metrics:        s,
		knowhow:        s,
		skills:         skills,
		bus:            event.NewBus(10000),
		recorder:       recorder,
		reg:            newRegistry(),
		deskEmitters:   make(map[string]*event.DeskEmitter),
		queues:         make(map[string]queue.Queue),
		humanInputs:    make(map[string]chan string),
		runningWorkers: make(map[string]struct{}),
		activeRuns:     make(map[string]context.CancelFunc),
		breaker:        newCircuitBreaker(recorder),
		budget:         newBudgetTracker(),
		sdkProcs:       sdkproc.NewProcessManager(0),
	}
}

// Load registers all config into the hub.
func (h *Hub) Load(org *types.Organization, agents map[string]*types.Agent, desks map[string]*types.Desk, groups map[string]*types.Group, resources map[string]*types.Resource) {
	h.reg.Load(org, agents, desks, groups, resources)
}

// Start wires up event subscriptions and starts queue workers.
func (h *Hub) Start(ctx context.Context) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if err := h.sdkProcs.EnsureSDK(ctx, h.reg.agents, h.reg.resources); err != nil {
		return fmt.Errorf("hub: sdk setup: %w", err)
	}

	// Mark any runs left in "running" state from a previous hub session as failed.
	h.cleanupStaleRuns()

	// Rebuild budget counters from persisted events so limits survive restarts.
	h.rehydrateBudget()

	for id, desk := range h.reg.desks {
		if len(desk.Subscribe) > 0 {
			id := id
			h.ensureQueue(id)
			h.bus.Subscribe(id, desk.Subscribe, func(_ context.Context, ev types.Event) error {
				h.enqueue(id, ev)
				return nil
			})
		}
	}

	// Groups are session scopes only — they do not subscribe to events.

	// Start resource watchers for resources with watch config.
	for id, res := range h.reg.resources {
		if resourcedomain.NeedsWatch(res.Watch, res.Config["path"]) {
			go h.watchResource(ctx, id, res)
		}
	}

	for id, q := range h.queues {
		if recovered := q.RequeueProcessing(); recovered > 0 {
			h.recorder.Record(observe.Event{DeskID: id, Type: observe.EventQueueRecovered})
		}
		if collapsed := q.CollapseIDlessPending(); collapsed > 0 {
			h.recorder.Record(observe.Event{DeskID: id, Type: observe.EventQueueCollapsed, OutputBytes: collapsed})
		}
	}

	for id := range h.queues {
		h.runningWorkers[id] = struct{}{}
		go h.queueWorker(ctx, id)
	}

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.mu.RLock()
				for id, q := range h.queues {
					if removed := q.GC(time.Hour); removed > 0 {
						h.recorder.Record(observe.Event{DeskID: id, Type: observe.EventQueueGC, OutputBytes: removed})
					}
				}
				h.mu.RUnlock()
			}
		}
	}()

	// Cron: schedule periodic event emission.
	if h.reg.organization != nil && len(h.reg.organization.Cron) > 0 {
		for _, entry := range h.reg.organization.Cron {
			entry := entry
			go func() {
				d, err := parseCronDuration(entry.Schedule)
				if err != nil {
					return
				}
				ticker := time.NewTicker(d)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						h.Emit(ctx, types.Event{
							Type:    entry.Event,
							Source:  "cron",
							Payload: []byte(entry.Payload),
						})
					}
				}
			}()
		}
	}

	h.bus.PublishAsync(ctx, types.Event{Type: "hub.started", Source: "hub"})

	return nil
}

// parseCronDuration parses simple interval expressions like "*/30 * * * *" (every 30 min)
// or shorthand: "5m", "1h", "30s".
func parseCronDuration(schedule string) (time.Duration, error) {
	// Try Go duration first: "5m", "1h", "30s"
	if d, err := time.ParseDuration(schedule); err == nil {
		return d, nil
	}
	// Simple cron: "*/N * * * *" → every N minutes
	var n int
	if _, err := fmt.Sscanf(schedule, "*/%d * * * *", &n); err == nil && n > 0 {
		return time.Duration(n) * time.Minute, nil
	}
	return 0, fmt.Errorf("unsupported cron schedule: %s", schedule)
}

func (h *Hub) startWorkerOnce(ctx context.Context, id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, running := h.runningWorkers[id]; running {
		return
	}
	h.runningWorkers[id] = struct{}{}
	go h.queueWorker(ctx, id)
}

func (h *Hub) ensureQueue(subscriberID string) {
	if _, ok := h.queues[subscriberID]; ok {
		return
	}
	q, err := queue.NewQueue(h.queueDir, subscriberID)
	if err != nil {
		return
	}
	h.queues[subscriberID] = q
}

func (h *Hub) enqueue(subscriberID string, ev types.Event) {
	q, ok := h.queues[subscriberID]
	if !ok {
		return
	}
	if q.ContainsEventID(ev.ID) {
		return
	}
	if ev.ID == "" && q.ContainsPendingType(ev.Type) {
		return
	}
	q.Push(ev)
	h.recorder.Record(observe.Event{DeskID: subscriberID, Type: observe.EventQueuePushed})
}

func (h *Hub) directlySubscribes(id, eventType string) bool {
	if d, ok := h.reg.desks[id]; ok {
		for _, s := range d.Subscribe {
			if s == eventType {
				return true
			}
		}
	}
	return false
}

func (h *Hub) queueWorker(ctx context.Context, subscriberID string) {
	q := h.queues[subscriberID]
	signal := q.Signal()

	for {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			entry := q.Take()
			if entry == nil {
				break
			}

			err := h.deliverToTarget(ctx, subscriberID, entry.Event, entry.ID)
			if err != nil {
				q.Fail(entry.ID, err.Error())
			} else {
				q.Complete(entry.ID)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-signal:
		}

		for {
			select {
			case <-signal:
			default:
				goto done
			}
		}
	done:
	}
}

// Reload merges new config into a running hub without restarting.
// Desks, groups, resources, and agents that no longer appear in the
// new config are removed from the registry.
func (h *Hub) Reload(ctx context.Context, org *types.Organization, agents map[string]*types.Agent, desks map[string]*types.Desk, groups map[string]*types.Group, resources map[string]*types.Resource) {
	h.mu.Lock()

	if org != nil {
		h.reg.organization = org
	}

	for id, a := range agents {
		h.reg.agents[id] = a
	}
	for id := range h.reg.agents {
		if _, ok := agents[id]; !ok {
			delete(h.reg.agents, id)
		}
	}

	// Remove desks that no longer exist in config.
	for id := range h.reg.desks {
		if _, ok := desks[id]; !ok {
			delete(h.reg.desks, id)
			h.bus.Unsubscribe(id)
			delete(h.deskEmitters, id)
			// Queue worker will idle — it exits on ctx cancellation.
			fmt.Fprintf(os.Stderr, "  ↺ Removed desk %q\n", id)
		}
	}

	for id, d := range desks {
		if old, exists := h.reg.desks[id]; exists {
			h.reg.desks[id] = d
			// Re-register subscriptions if they changed.
			if !slicesEqual(old.Subscribe, d.Subscribe) {
				h.bus.Unsubscribe(id)
				if len(d.Subscribe) > 0 {
					h.ensureQueue(id)
					h.bus.Subscribe(id, d.Subscribe, func(_ context.Context, ev types.Event) error {
						h.enqueue(id, ev)
						return nil
					})
				}
			}
			continue
		}
		h.reg.desks[id] = d
		if len(d.Subscribe) > 0 {
			h.ensureQueue(id)
			h.bus.Subscribe(id, d.Subscribe, func(_ context.Context, ev types.Event) error {
				h.enqueue(id, ev)
				return nil
			})
		}
	}

	for id := range h.reg.groups {
		if _, ok := groups[id]; !ok {
			delete(h.reg.groups, id)
		}
	}
	for id, g := range groups {
		h.reg.groups[id] = g
	}

	for id := range h.reg.resources {
		if _, ok := resources[id]; !ok {
			delete(h.reg.resources, id)
		}
	}
	for id, r := range resources {
		h.reg.resources[id] = r
	}

	allQueues := make([]string, 0, len(h.queues))
	for id := range h.queues {
		allQueues = append(allQueues, id)
	}
	h.mu.Unlock()

	for _, id := range allQueues {
		h.startWorkerOnce(ctx, id)
	}

	h.recorder.Record(observe.Event{Type: observe.EventHubReloaded})
}

func (h *Hub) Emit(ctx context.Context, ev types.Event) {
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}

	// Loop circuit breaker: check org-level limits before publishing.
	org := h.reg.organization
	if org != nil && h.breaker.Tripped(ev.Type, org.Limits) {
		return
	}

	h.recorder.Record(observe.Event{
		Type:   observe.EventPublished,
		DeskID: ev.Source,
	})
	h.bus.PublishAsync(ctx, ev)
}

// loopBreakerTripped returns true if the event type should be suppressed.
func (h *Hub) EmitSync(ctx context.Context, ev types.Event) []error {
	return h.bus.Publish(ctx, ev)
}

func (h *Hub) Bus() *event.Bus { return h.bus }

// DeskEmitter returns a scoped emitter for the given desk, creating one if needed.
func (h *Hub) DeskEmitter(deskID string) *event.DeskEmitter {
	h.mu.Lock()
	defer h.mu.Unlock()
	if em, ok := h.deskEmitters[deskID]; ok {
		return em
	}
	em := event.NewDeskEmitter(h.bus, deskID)
	h.deskEmitters[deskID] = em
	return em
}
func (h *Hub) SetProjectDir(dir string) {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	h.projectDir = dir
	h.sdkProcs.SetProjectDir(dir)
}

func (h *Hub) SetSDKPort(port int)     { h.sdkProcs.SetBasePort(port) }
func (h *Hub) SetSDKPython(bin string) { h.sdkProcs.SetPythonBin(bin) }
func (h *Hub) SetSDKNode(bin string)   { h.sdkProcs.SetNodeBin(bin) }
func (h *Hub) SetQueueDir(dir string)  { h.queueDir = dir }
func (h *Hub) SDKReady() bool                          { return h.sdkProcs.IsReady() }
func (h *Hub) Events() []observe.Event                 { return h.recorder.Events() }
func (h *Hub) Subscribe() (chan observe.Event, func()) { return h.recorder.Subscribe() }

func (h *Hub) RecordMetrics(deskID string, m map[string]float64) {
	h.recordMetricsFull("", deskID, "", m)
}

func (h *Hub) recordMetricsFull(runID, deskID, agentID string, m map[string]float64) {
	h.recorder.Record(observe.Event{
		DeskID:  deskID,
		Type:    observe.EventMetrics,
		Metrics: m,
	})
	for name, value := range m {
		_ = h.metrics.RecordMetric(runID, deskID, agentID, name, value)
	}
}

func (h *Hub) GetMetrics(deskID string) map[string]map[string]float64 {
	rows, err := h.metrics.MetricsByScope(deskID)
	if err != nil {
		return nil
	}
	out := make(map[string]map[string]float64)
	for _, r := range rows {
		if out[r.ScopeID] == nil {
			out[r.ScopeID] = make(map[string]float64)
		}
		out[r.ScopeID][r.Name] = r.Value
	}
	return out
}

func (h *Hub) Desks() map[string]*types.Desk       { return h.reg.Desks() }
func (h *Hub) Groups() map[string]*types.Group      { return h.reg.Groups() }
func (h *Hub) Resources() map[string]*types.Resource { return h.reg.Resources() }
func (h *Hub) Organization() *types.Organization    { return h.reg.Organization() }

func (h *Hub) QueueStatus() map[string]int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]int)
	for id, q := range h.queues {
		out[id] = q.PendingCount()
	}
	return out
}

func (h *Hub) SubmitHumanInput(deskID, content string) bool {
	h.mu.RLock()
	ch, ok := h.humanInputs[deskID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	ch <- content
	return true
}

func (h *Hub) registerRun(runID string, cancel context.CancelFunc) {
	h.activeRunsMu.Lock()
	defer h.activeRunsMu.Unlock()
	h.activeRuns[runID] = cancel
}

func (h *Hub) deregisterRun(runID string) {
	h.activeRunsMu.Lock()
	defer h.activeRunsMu.Unlock()
	delete(h.activeRuns, runID)
}

func (h *Hub) CancelRun(runID string) bool {
	h.activeRunsMu.Lock()
	fn, ok := h.activeRuns[runID]
	h.activeRunsMu.Unlock()
	if ok {
		fn()
		return true
	}
	return false
}

// cleanupStaleRuns scans recorded events for runs that have a desk.started
// but no terminal event (completed/failed/timed_out). These are leftovers from
// a previous hub session that crashed or was restarted. We record a desk.failed
// event for each so they stop showing as "running" in the dashboard.
func (h *Hub) cleanupStaleRuns() {
	events := h.recorder.Events()

	// Track which (runID, deskID) pairs have started vs terminated.
	type key struct{ runID, deskID string }
	started := map[key]time.Time{}
	terminated := map[key]bool{}

	for _, ev := range events {
		if ev.RunID == "" || ev.DeskID == "" {
			continue
		}
		k := key{ev.RunID, ev.DeskID}
		switch ev.Type {
		case observe.EventDeskStarted:
			if _, ok := started[k]; !ok {
				started[k] = ev.At
			}
		case observe.EventDeskCompleted, observe.EventDeskFailed, observe.EventDeskTimedOut:
			terminated[k] = true
		}
	}

	now := time.Now()
	cleaned := 0
	for k, startedAt := range started {
		if terminated[k] {
			continue
		}
		h.recorder.Record(observe.Event{
			RunID:      k.runID,
			DeskID:     k.deskID,
			Type:       observe.EventDeskFailed,
			At:         now,
			DurationMs: now.Sub(startedAt).Milliseconds(),
			Error:      "hub restarted — run was orphaned",
		})
		cleaned++
	}
	if cleaned > 0 {
		h.recorder.Record(observe.Event{
			Type:        observe.EventHubStarted,
			At:          now,
			Output:      fmt.Sprintf("cleaned up %d stale runs", cleaned),
			OutputBytes: cleaned,
		})
	}
}

// rehydrateBudget replays cost data from persisted events so budget limits
// survive hub restarts. It scans for desk.completed events with token counts
// and re-estimates the cost using the same model pricing.
func (h *Hub) rehydrateBudget() {
	events := h.recorder.Events()
	rehydrated := 0
	for _, ev := range events {
		if ev.Type != observe.EventDeskCompleted {
			continue
		}
		if ev.InputTokens == 0 && ev.OutputTokens == 0 {
			continue
		}
		cost := estimateCost(ev.Model, ev.InputTokens, ev.OutputTokens)
		if cost <= 0 {
			continue
		}
		scope := "desk:" + ev.DeskID
		h.budget.AddAt(scope, ev.RunID, cost, ev.At)
		rehydrated++
	}
	if rehydrated > 0 {
		h.recorder.Record(observe.Event{
			Type:        observe.EventHubStarted,
			At:          time.Now(),
			Output:      fmt.Sprintf("rehydrated budget from %d events", rehydrated),
			OutputBytes: rehydrated,
		})
	}
}

func (h *Hub) deliverToTarget(ctx context.Context, targetID string, ev types.Event, stableRunID string) error {
	// Only desks are executable. Groups are session scopes only.
	if desk, ok := h.reg.desks[targetID]; ok {
		return h.runDeskActor(ctx, targetID, desk, ev)
	}
	return fmt.Errorf("hub: routing target %q not found", targetID)
}

func (h *Hub) DeskSession(deskID string) ([]store.SessionEntry, bool) {
	h.reg.mu.RLock()
	_, known := h.reg.desks[deskID]
	h.reg.mu.RUnlock()
	if !known {
		return nil, false
	}
	entries := h.sessions.LoadSession(deskID, 0)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, true
}

func (h *Hub) DeskLogs(deskID string) []store.LogEntry {
	return h.logs.LoadLogs(deskID)
}

func (h *Hub) Warnings() []types.Warning {
	h.reg.mu.RLock()
	defer h.reg.mu.RUnlock()

	var warnings []types.Warning
	ctx := context.Background()
	for id, agent := range h.reg.agents {
		for _, ref := range agent.Skills {
			if _, err := h.skills.Resolve(ctx, ref); err != nil {
				warnings = append(warnings, types.Warning{
					Level:   "warn",
					Source:  "agent:" + id,
					Message: fmt.Sprintf("skill %q not found", ref),
				})
			}
		}
	}
	for id, desk := range h.reg.desks {
		for _, ref := range desk.Skills {
			if _, err := h.skills.Resolve(ctx, ref); err != nil {
				warnings = append(warnings, types.Warning{
					Level:   "warn",
					Source:  "desk:" + id,
					Message: fmt.Sprintf("skill %q not found", ref),
				})
			}
		}
	}
	return warnings
}

func (h *Hub) groupDesks(groupID string) []string    { return h.reg.GroupDesks(groupID) }
func (h *Hub) groupSubGroups(groupID string) []string { return h.reg.GroupSubGroups(groupID) }
func (h *Hub) deskInGroup(groupID, deskID string) bool { return h.reg.DeskInGroup(groupID, deskID) }

func (h *Hub) watchResource(ctx context.Context, resourceID string, res *types.Resource) {
	w := resource.NewWatcher(res)
	go func() {
		if err := w.Start(ctx); err != nil && err != context.Canceled {
			fmt.Fprintf(os.Stderr, "  ⚠ resource watcher %q stopped: %v\n", resourceID, err)
		}
	}()
	for ev := range w.Events() {
		h.Emit(ctx, ev)
	}
}

// mergeBatch is kept for compatibility but no longer used in queue worker.
func (h *Hub) mergeBatch(entries []*queue.Entry) types.Event {
	if len(entries) == 1 {
		return entries[0].Event
	}

	type batchItem struct {
		Type    string `json:"type"`
		Source  string `json:"source"`
		Payload string `json:"payload"`
	}
	var items []batchItem
	for _, e := range entries {
		items = append(items, batchItem{
			Type:    e.Event.Type,
			Source:  e.Event.Source,
			Payload: string(e.Event.Payload),
		})
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"batch_size": len(entries),
		"events":     items,
	})

	return types.Event{
		Type:    entries[0].Event.Type,
		Source:  "queue:batch",
		Payload: payload,
	}
}

// slicesEqual returns true if two string slices have the same elements in the same order.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}


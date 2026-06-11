package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/roster-io/roster/internal/event"
	"github.com/roster-io/roster/internal/agent/knowhow"
	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/internal/event/queue"
	"github.com/roster-io/roster/internal/resource"
	"github.com/roster-io/roster/internal/event/routing"
	"github.com/roster-io/roster/internal/exec/sdkproc"
	"github.com/roster-io/roster/internal/session"
	"github.com/roster-io/roster/internal/agent/skill"
	"github.com/roster-io/roster/internal/store"
	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

// Dispatcher routes tasks to executors.
type Dispatcher interface {
	Dispatch(ctx context.Context, t types.ExecutorType, task sdk.Task) (*types.Artifact, error)
}

// Hub is the event-driven orchestrator.
// Events are queued per subscriber and processed sequentially.
// Queues persist to disk — on restart, unfinished work resumes.
type Hub struct {
	registry Dispatcher
	store    store.Store
	skills   *skill.Resolver
	sessions *session.Manager
	bus      *event.Bus
	recorder *observe.Recorder

	mu           sync.RWMutex
	organization *types.Organization
	desks        map[string]*types.Desk
	agents       map[string]*types.Agent
	groups       map[string]*types.Group
	resources    map[string]*types.Resource
	policies     map[string]*types.Policy
	projectDir   string
	queueDir     string

	queues         map[string]queue.Queue // subscriberID → queue
	humanInputs    map[string]chan *types.Artifact
	runningWorkers map[string]struct{} // subscriberID → worker started

	activeRuns   map[string]context.CancelFunc
	activeRunsMu sync.Mutex

	scheduler   *cron.Cron
	cronEntries map[string]cron.EntryID // subscriberID → cron entry ID

	budget *BudgetTracker // hierarchical cost tracking (desk → group → org)

	sdkProcs *sdkproc.ProcessManager
}

func New(registry Dispatcher, store store.Store, skills *skill.Resolver, recorder *observe.Recorder) *Hub {
	return &Hub{
		registry:    registry,
		store:       store,
		skills:      skills,
		sessions:    session.NewManager(store.Group()),
		bus:         event.NewBus(10000),
		recorder:    recorder,
		desks:       make(map[string]*types.Desk),
		agents:      make(map[string]*types.Agent),
		groups:      make(map[string]*types.Group),
		resources:   make(map[string]*types.Resource),
		policies:    make(map[string]*types.Policy),
		queues:         make(map[string]queue.Queue),
		humanInputs:    make(map[string]chan *types.Artifact),
		runningWorkers: make(map[string]struct{}),
		activeRuns:     make(map[string]context.CancelFunc),
		cronEntries:    make(map[string]cron.EntryID),
		budget:         newBudgetTracker(),
		sdkProcs:       sdkproc.NewProcessManager(50100),
	}
}

// Load registers all config into the hub.
func (h *Hub) Load(org *types.Organization, agents map[string]*types.Agent, desks map[string]*types.Desk, groups map[string]*types.Group, resources map[string]*types.Resource, policies map[string]*types.Policy) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.organization = org
	for id, a := range agents {
		h.agents[id] = a
	}
	for id, d := range desks {
		h.desks[id] = d
	}
	for id, g := range groups {
		h.groups[id] = g
	}
	for id, r := range resources {
		h.resources[id] = r
	}
	for id, p := range policies {
		h.policies[id] = p
	}
}

// Start wires up event subscriptions and starts queue workers.
// On restart, interrupted events are requeued and processed.
func (h *Hub) Start(ctx context.Context) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Install any SDK packages declared in agent configs.
	if err := h.sdkProcs.EnsureSDK(ctx, h.agents, h.resources); err != nil {
		return fmt.Errorf("hub: sdk setup: %w", err)
	}

	// Wire individual desk subscriptions → queue.
	for id, desk := range h.desks {
		if len(desk.Subscribe) > 0 {
			id := id
			h.ensureQueue(id)
			h.bus.Subscribe(id, desk.Subscribe, func(_ context.Context, ev types.Event) error {
				h.enqueue(id, ev)
				return nil
			})
		}
	}

	// Wire individual group subscriptions → queue.
	for id, group := range h.groups {
		if len(group.Subscribe) > 0 {
			id := id
			h.ensureQueue(id)
			h.bus.Subscribe(id, group.Subscribe, func(_ context.Context, ev types.Event) error {
				h.enqueue(id, ev)
				return nil
			})
		}
	}

	// Recover interrupted entries from previous run.
	for id, q := range h.queues {
		if recovered := q.RequeueProcessing(); recovered > 0 {
			h.recorder.Record(observe.Event{StepID: id, Type: observe.EventType("queue.recovered")})
		}
		// Collapse duplicate ID-less events (hub.started, cron ticks) accumulated
		// across restarts — only the latest occurrence matters.
		if collapsed := q.CollapseIDlessPending(); collapsed > 0 {
			h.recorder.Record(observe.Event{StepID: id, Type: observe.EventType("queue.collapsed"), OutputBytes: collapsed})
		}
	}

	// Start a worker goroutine for each subscriber queue.
	for id := range h.queues {
		h.runningWorkers[id] = struct{}{}
		go h.queueWorker(ctx, id)
	}

	// Start periodic queue GC (every 10 minutes, remove entries older than 1 hour).
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
						h.recorder.Record(observe.Event{StepID: id, Type: observe.EventType("queue.gc"), OutputBytes: removed})
					}
				}
				h.mu.RUnlock()
			}
		}
	}()

	// Start cron scheduler for groups and desks with cron expressions.
	scheduler := cron.New()
	for id, group := range h.groups {
		if group.Cron != "" {
			id := id
			h.ensureQueue(id)
			entryID, _ := scheduler.AddFunc(group.Cron, func() {
				h.enqueue(id, types.Event{Type: id + ".cron", Source: "cron"})
			})
			h.cronEntries[id] = entryID
			h.recorder.Record(observe.Event{StepID: id, Type: observe.EventType("cron.registered")})
		}
	}
	for id, desk := range h.desks {
		if desk.Cron != "" {
			id := id
			h.ensureQueue(id)
			entryID, _ := scheduler.AddFunc(desk.Cron, func() {
				h.enqueue(id, types.Event{Type: id + ".cron", Source: "cron"})
			})
			h.cronEntries[id] = entryID
			h.recorder.Record(observe.Event{StepID: id, Type: observe.EventType("cron.registered")})
		}
	}
	h.scheduler = scheduler
	scheduler.Start()
	go func() {
		<-ctx.Done()
		scheduler.Stop()
	}()

	// Start exec/poll triggers for desks and groups.
	for id, desk := range h.desks {
		for _, trig := range desk.Triggers {
			h.startTrigger(ctx, id, trig)
		}
	}
	for id, group := range h.groups {
		for _, trig := range group.Triggers {
			h.startTrigger(ctx, id, trig)
		}
	}

	// Start resource watchers.
	for id, res := range h.resources {
		if len(res.Watch) > 0 {
			go h.watchResource(ctx, id, res)
		}
	}

	// Emit hub.started — any group/desk subscribing to this fires immediately.
	h.bus.PublishAsync(ctx, types.Event{Type: "hub.started", Source: "hub"})

	return nil
}

// startWorkerOnce starts a queue worker goroutine if one isn't already running.
func (h *Hub) startWorkerOnce(ctx context.Context, id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, running := h.runningWorkers[id]; running {
		return
	}
	h.runningWorkers[id] = struct{}{}
	go h.queueWorker(ctx, id)
}

// ensureQueue creates a persistent queue for the subscriber if it doesn't exist.
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

// enqueue pushes an event to a subscriber's queue.
// If the event carries an ID that is already pending/processing in the queue,
// the push is skipped to prevent double-delivery from overlapping routing paths.
func (h *Hub) enqueue(subscriberID string, ev types.Event) {
	q, ok := h.queues[subscriberID]
	if !ok {
		return
	}
	if q.ContainsEventID(ev.ID) {
		return
	}
	// For ID-less events (e.g. hub.started, cron ticks), deduplicate by type:
	// if one is already pending or processing, skip the new one.
	if ev.ID == "" && q.ContainsPendingType(ev.Type) {
		return
	}
	q.Push(ev)
	h.recorder.Record(observe.Event{StepID: subscriberID, Type: observe.EventType("queue.pushed")})
}

// directlySubscribes reports whether the group or desk named id has eventType
// in its own subscribe list (as opposed to receiving it via org routing).
func (h *Hub) directlySubscribes(id, eventType string) bool {
	if g, ok := h.groups[id]; ok {
		for _, s := range g.Subscribe {
			if s == eventType {
				return true
			}
		}
	}
	if d, ok := h.desks[id]; ok {
		for _, s := range d.Subscribe {
			if s == eventType {
				return true
			}
		}
	}
	return false
}

// queueWorker processes events from a subscriber's queue.
// It uses the queue's Signal channel for push-based wake-up instead of polling.
// For groups with a lead, it batches pending events when multiple are queued.
func (h *Hub) queueWorker(ctx context.Context, subscriberID string) {
	q := h.queues[subscriberID]
	signal := q.Signal()

	for {
		// Process all available items first (handles startup with recovered items).
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Check if this is a group with a lead — batch mode.
			if group, ok := h.groups[subscriberID]; ok && group.Lead != nil {
				batch := q.TakeAll()
				if len(batch) == 0 {
					break // back to outer wait loop
				}
				h.recorder.Record(observe.Event{StepID: subscriberID, Type: observe.EventType("queue.batch"), OutputBytes: len(batch)})

				merged := h.mergeBatch(batch)
				stableID := ""
				if len(batch) == 1 {
					stableID = batch[0].ID
				}
				err := h.deliverToTarget(ctx, subscriberID, merged, stableID)
				for _, entry := range batch {
					if err != nil {
						q.Fail(entry.ID, err.Error())
					} else {
						q.Complete(entry.ID)
					}
				}
				continue
			}

			// Single-event mode for desks and groups without a lead.
			entry := q.Take()
			if entry == nil {
				break // back to outer wait loop
			}

			err := h.deliverToTarget(ctx, subscriberID, entry.Event, entry.ID)
			if err != nil {
				q.Fail(entry.ID, err.Error())
			} else {
				q.Complete(entry.ID)
			}
		}

		// Wait for a signal or context cancellation.
		select {
		case <-ctx.Done():
			return
		case <-signal:
		}

		// Drain any extra signals that arrived while we were waiting.
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

// mergeBatch combines multiple queued events into a single event.
// The lead desk sees all pending requests as context to plan/prioritize.
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

// Reload merges new config into a running hub without restarting.
// New desks, groups, and routing rules are wired up immediately.
// Existing queues and running workers are not interrupted.
func (h *Hub) Reload(ctx context.Context, org *types.Organization, agents map[string]*types.Agent, desks map[string]*types.Desk, groups map[string]*types.Group, resources map[string]*types.Resource, policies map[string]*types.Policy) {
	h.mu.Lock()

	if org != nil {
		h.organization = org
	}

	// Merge agents.
	for id, a := range agents {
		h.agents[id] = a
	}

	// Wire new desks.
	for id, d := range desks {
		if _, exists := h.desks[id]; exists {
			h.desks[id] = d // update config in place
			continue
		}
		h.desks[id] = d
		if len(d.Subscribe) > 0 {
			h.ensureQueue(id)
			h.bus.Subscribe(id, d.Subscribe, func(_ context.Context, ev types.Event) error {
				h.enqueue(id, ev)
				return nil
			})
		}
	}

	// Wire new groups.
	for id, g := range groups {
		if _, exists := h.groups[id]; exists {
			h.groups[id] = g
			continue
		}
		h.groups[id] = g
		if len(g.Subscribe) > 0 {
			h.ensureQueue(id)
			h.bus.Subscribe(id, g.Subscribe, func(_ context.Context, ev types.Event) error {
				h.enqueue(id, ev)
				return nil
			})
		}
	}

	for id, r := range resources {
		h.resources[id] = r
	}
	for id, p := range policies {
		h.policies[id] = p
	}
	// Collect queue IDs before releasing the lock.
	allQueues := make([]string, 0, len(h.queues))
	for id := range h.queues {
		allQueues = append(allQueues, id)
	}
	h.mu.Unlock()

	// Start workers for any queues that don't have one yet.
	for _, id := range allQueues {
		h.startWorkerOnce(ctx, id)
	}

	h.recorder.Record(observe.Event{Type: observe.EventType("hub.reloaded")})
}

// Emit publishes an event to the bus.
func (h *Hub) Emit(ctx context.Context, ev types.Event) {
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	h.recorder.Record(observe.Event{
		Type:   observe.EventType("event.published"),
		StepID: ev.Source,
	})
	h.bus.PublishAsync(ctx, ev)
}

// EmitSync publishes an event and waits for all handlers to complete.
func (h *Hub) EmitSync(ctx context.Context, ev types.Event) []error {
	return h.bus.Publish(ctx, ev)
}

func (h *Hub) Bus() *event.Bus                       { return h.bus }
func (h *Hub) SetProjectDir(dir string) {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	h.projectDir = dir
	h.sdkProcs.SetProjectDir(dir)
}

func (h *Hub) SetSDKPython(bin string) { h.sdkProcs.SetPythonBin(bin) }
func (h *Hub) SetSDKNode(bin string)   { h.sdkProcs.SetNodeBin(bin) }
func (h *Hub) SetQueueDir(dir string)                  { h.queueDir = dir }
func (h *Hub) Events() []observe.Event                        { return h.recorder.Events() }
func (h *Hub) Subscribe() (chan observe.Event, func())         { return h.recorder.Subscribe() }

// RecordMetrics persists metrics for a single execution to the store and the event recorder.
func (h *Hub) RecordMetrics(deskID string, m map[string]float64) {
	h.recordMetricsFull("", deskID, "", m)
}

func (h *Hub) recordMetricsFull(runID, deskID, agentID string, m map[string]float64) {
	h.recorder.Record(observe.Event{
		StepID:  deskID,
		Type:    observe.EventType("metrics.reported"),
		Metrics: m,
	})
	for name, value := range m {
		_ = h.store.Metrics().Record(runID, deskID, agentID, name, value)
	}
}

// GetMetrics returns aggregated metric totals by desk from the store.
// If deskID is empty, all desks are returned.
func (h *Hub) GetMetrics(deskID string) map[string]map[string]float64 {
	rows, err := h.store.Metrics().SumByDesk(deskID)
	if err != nil {
		return nil
	}
	out := make(map[string]map[string]float64)
	for _, r := range rows {
		if out[r.DeskID] == nil {
			out[r.DeskID] = make(map[string]float64)
		}
		out[r.DeskID][r.Name] = r.Value
	}
	return out
}

func (h *Hub) Desks() map[string]*types.Desk {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.desks
}

func (h *Hub) Groups() map[string]*types.Group {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.groups
}

func (h *Hub) Resources() map[string]*types.Resource {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.resources
}

func (h *Hub) Organization() *types.Organization {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.organization
}

// QueueStatus returns pending counts per subscriber.
func (h *Hub) QueueStatus() map[string]int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]int)
	for id, q := range h.queues {
		out[id] = q.PendingCount()
	}
	return out
}

// CronStatus returns all registered cron schedules with their next run time.
func (h *Hub) CronStatus() []types.CronInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var infos []types.CronInfo
	for id, group := range h.groups {
		if group.Cron != "" {
			info := types.CronInfo{ID: id, Cron: group.Cron, Type: "group"}
			if h.scheduler != nil {
				if entryID, ok := h.cronEntries[id]; ok {
					entry := h.scheduler.Entry(entryID)
					if !entry.Next.IsZero() {
						info.NextRun = entry.Next.Format(time.RFC3339)
					}
					if !entry.Prev.IsZero() {
						info.LastRun = entry.Prev.Format(time.RFC3339)
					}
				}
			}
			infos = append(infos, info)
		}
	}
	for id, desk := range h.desks {
		if desk.Cron != "" {
			info := types.CronInfo{ID: id, Cron: desk.Cron, Type: "desk"}
			if h.scheduler != nil {
				if entryID, ok := h.cronEntries[id]; ok {
					entry := h.scheduler.Entry(entryID)
					if !entry.Next.IsZero() {
						info.NextRun = entry.Next.Format(time.RFC3339)
					}
					if !entry.Prev.IsZero() {
						info.LastRun = entry.Prev.Format(time.RFC3339)
					}
				}
			}
			infos = append(infos, info)
		}
	}
	return infos
}

// SubmitHumanInput provides input for a human desk that is waiting.
func (h *Hub) SubmitHumanInput(deskID, content string) bool {
	h.mu.RLock()
	ch, ok := h.humanInputs[deskID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	ch <- &types.Artifact{ID: uuid.NewString(), CreatedAt: time.Now(), Schema: "text-v1", Payload: []byte(content)}
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

// CancelRun cancels an active run by ID. Returns false if the run is not found.
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

// deliverToTarget routes an event to a group or desk by ID.
// stableRunID, when non-empty, is used as the run ID for group runs so that
// a recovered queue entry resumes under the same run ID and can reload checkpoints.
func (h *Hub) deliverToTarget(ctx context.Context, targetID string, ev types.Event, stableRunID string) error {
	if group, ok := h.groups[targetID]; ok {
		return h.runGroupActor(ctx, targetID, group, ev, stableRunID)
	}
	if desk, ok := h.desks[targetID]; ok {
		return h.runDeskActor(ctx, targetID, desk, ev)
	}
	return fmt.Errorf("hub: routing target %q not found", targetID)
}

// runGroupActor executes a group in response to an event.
// stableRunID, when non-empty, is used verbatim so a recovered queue entry
// resumes the same run and can reload per-desk checkpoints.
func (h *Hub) runGroupActor(ctx context.Context, groupID string, group *types.Group, ev types.Event, stableRunID string) error {
	sess := h.sessions.Activate(groupID)
	defer h.sessions.Deactivate(groupID)

	input := &types.Artifact{Payload: ev.Payload}
	runID := stableRunID
	if runID == "" {
		runID = newRunID(groupID)
	}

	ctx, cancel := context.WithCancel(ctx)
	h.registerRun(runID, cancel)
	defer h.deregisterRun(runID)
	defer cancel()

	h.recorder.Record(observe.Event{RunID: runID, StepID: groupID, Type: observe.EventStepStarted})

	var result *types.Artifact
	var err error

	if group.Lead != nil {
		pos := group.Lead.Position
		if pos == "" {
			pos = "both"
		}
		switch pos {
		case "both":
			result, err = h.runGroupCoordinate(ctx, runID, groupID, group, input, sess)
		case "first":
			result, err = h.runGroupDecompose(ctx, runID, groupID, group, input, sess)
		case "last":
			result, err = h.runGroupSynthesize(ctx, runID, groupID, group, input, sess)
		}
	} else {
		result, err = h.runGroupSequential(ctx, runID, groupID, group, input, sess)
	}

	if err != nil {
		h.recorder.Record(observe.Event{RunID: runID, StepID: groupID, Type: observe.EventStepFailed, Error: err.Error()})
		h.bus.PublishAsync(ctx, types.Event{Type: "step.failed", Source: groupID, Payload: []byte(err.Error())})
		return err
	}

	h.recorder.Record(observe.Event{RunID: runID, StepID: groupID, Type: observe.EventStepCompleted})

	// Emit declared events from the group automatically on completion.
	payload := []byte{}
	if result != nil {
		payload = result.Payload
	}
	for _, emitType := range group.Emit {
		actualType := routing.DetermineEventType(emitType, payload)
		h.bus.PublishAsync(ctx, types.Event{
			Type:    actualType,
			Source:  groupID,
			Payload: payload,
		})
	}

	return nil
}

func (h *Hub) runGroupSequential(ctx context.Context, runID, groupID string, group *types.Group, input *types.Artifact, sess *session.Session) (*types.Artifact, error) {
	current := input

	// Run nested sub-groups first, then member desks.
	for _, subGroupID := range h.groupSubGroups(groupID) {
		subGroup, ok := h.groups[subGroupID]
		if !ok {
			continue
		}
		err := h.runGroupActor(ctx, subGroupID, subGroup, types.Event{
			Type:    groupID + ".subtask",
			Source:  groupID,
			Payload: current.Payload,
		}, "")
		if err != nil {
			return nil, err
		}
	}

	// Conversation mode: members run multiple rounds so they can respond to each other.
	// Round 1: each member sees the input + group history.
	// Round 2: each member sees round 1 outputs via group history — can respond, refine, or SKIP.
	maxRounds := 1
	if group.Dispatch == "conversation" {
		maxRounds = 2
	}

	leadDeskID := ""
	if group.Lead != nil {
		leadDeskID = group.Lead.Desk
	}

	for round := 0; round < maxRounds; round++ {
		for _, deskID := range h.groupDesks(groupID) {
			// Skip the lead desk — it is handled separately by runGroupCoordinate/Decompose/Synthesize.
			if deskID == leadDeskID {
				continue
			}
			// Group-level checkpoint: resume completed desks without re-running them.
			// GroupStore already has messages from before the crash, so skip sess.Post to avoid duplicates.
			checkpointKey := fmt.Sprintf("%s-round%d", deskID, round)
			if saved, ok := h.store.Run().LoadStep(runID, groupID, checkpointKey); ok {
				current = saved
				continue
			}

			artifact, err := h.runGroupDesk(ctx, runID, groupID, deskID, current, sess)
			if err != nil {
				// Record failure but continue with remaining desks.
				h.recorder.Record(observe.Event{
					RunID:  runID,
					StepID: deskID,
					Type:   observe.EventType("step.failed.continued"),
					Error:  err.Error(),
				})
				continue
			}
			if artifact != nil {
				h.store.Run().SaveStep(runID, groupID, checkpointKey, artifact)
				current = artifact
			}
		}
	}

	return current, nil
}

// runGroupMembers dispatches member desks either sequentially or in parallel
// depending on group.Dispatch.
func (h *Hub) runGroupMembers(ctx context.Context, runID, groupID string, group *types.Group, input *types.Artifact, sess *session.Session) (*types.Artifact, error) {
	if group.Dispatch == "parallel" {
		return h.runGroupParallel(ctx, runID, groupID, group, input, sess)
	}
	return h.runGroupSequential(ctx, runID, groupID, group, input, sess)
}

// runGroupParallel runs all member desks concurrently with the same input artifact.
// Their outputs are collected in declaration order and concatenated into one artifact.
func (h *Hub) runGroupParallel(ctx context.Context, runID, groupID string, group *types.Group, input *types.Artifact, sess *session.Session) (*types.Artifact, error) {
	memberDesks := h.groupDesks(groupID)
	if len(h.groupSubGroups(groupID)) > 0 {
		h.recorder.Record(observe.Event{RunID: runID, StepID: groupID, Type: observe.EventType("group.parallel.subgroups_ignored"), Error: "dispatch:parallel ignores nested groups; use dispatch:sequential for sub-group support"})
	}
	if len(memberDesks) == 0 {
		return input, nil
	}

	type result struct {
		idx      int
		artifact *types.Artifact
		err      error
	}

	results := make([]result, len(memberDesks))
	ch := make(chan result, len(memberDesks))
	var wg sync.WaitGroup

	for i, deskID := range memberDesks {
		checkpointKey := deskID + "-parallel"
		if saved, ok := h.store.Run().LoadStep(runID, groupID, checkpointKey); ok {
			results[i] = result{idx: i, artifact: saved}
			continue
		}
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			artifact, err := h.runGroupDesk(ctx, runID, groupID, id, input, sess)
			ch <- result{idx: idx, artifact: artifact, err: err}
		}(i, deskID)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for r := range ch {
		if r.err != nil {
			h.recorder.Record(observe.Event{
				RunID:  runID,
				StepID: memberDesks[r.idx],
				Type:   observe.EventType("step.failed.continued"),
				Error:  r.err.Error(),
			})
			continue
		}
		results[r.idx] = r
		if r.artifact != nil {
			h.store.Run().SaveStep(runID, groupID, memberDesks[r.idx]+"-parallel", r.artifact)
		}
	}

	// Merge artifacts in declaration order.
	var parts []string
	for _, r := range results {
		if r.artifact != nil && len(r.artifact.Payload) > 0 {
			parts = append(parts, string(r.artifact.Payload))
		}
	}
	if len(parts) == 0 {
		return input, nil
	}
	return &types.Artifact{
		Schema:  "text-v1",
		Payload: []byte(strings.Join(parts, "\n\n---\n\n")),
	}, nil
}

func (h *Hub) runGroupCoordinate(ctx context.Context, runID, groupID string, group *types.Group, input *types.Artifact, sess *session.Session) (*types.Artifact, error) {
	// Solo lead: no member desks — just run the lead once.
	if len(h.groupDesks(groupID)) == 0 {
		return h.runGroupDecompose(ctx, runID, groupID, group, input, sess)
	}

	// Lead plan phase — checkpointed so restart skips if already done.
	planKey := group.Lead.Desk + "-plan"
	plan, ok := h.store.Run().LoadStep(runID, groupID, planKey)
	if !ok {
		var err error
		plan, err = h.runGroupDesk(ctx, runID, groupID, group.Lead.Desk, input, sess)
		if err != nil {
			return nil, err
		}
		if plan != nil {
			h.store.Run().SaveStep(runID, groupID, planKey, plan)
		}
	}

	membersResult, err := h.runGroupMembers(ctx, runID, groupID, group, plan, sess)
	if err != nil {
		return nil, err
	}

	// Lead synthesize phase — checkpointed separately.
	synthKey := group.Lead.Desk + "-synthesize"
	if synth, ok := h.store.Run().LoadStep(runID, groupID, synthKey); ok {
		return synth, nil
	}
	result, err := h.runGroupDesk(ctx, runID, groupID, group.Lead.Desk, membersResult, sess)
	if err != nil {
		return nil, err
	}
	if result != nil {
		h.store.Run().SaveStep(runID, groupID, synthKey, result)
	}
	return result, nil
}

func (h *Hub) runGroupDecompose(ctx context.Context, runID, groupID string, group *types.Group, input *types.Artifact, sess *session.Session) (*types.Artifact, error) {
	decompKey := group.Lead.Desk + "-decompose"
	leadArtifact, ok := h.store.Run().LoadStep(runID, groupID, decompKey)
	if !ok {
		var err error
		leadArtifact, err = h.runGroupDesk(ctx, runID, groupID, group.Lead.Desk, input, sess)
		if err != nil {
			return nil, err
		}
		if leadArtifact != nil {
			h.store.Run().SaveStep(runID, groupID, decompKey, leadArtifact)
		}
	}
	if leadArtifact == nil || len(h.groupDesks(groupID)) == 0 {
		return leadArtifact, nil
	}
	return h.runGroupMembers(ctx, runID, groupID, group, leadArtifact, sess)
}

func (h *Hub) runGroupSynthesize(ctx context.Context, runID, groupID string, group *types.Group, input *types.Artifact, sess *session.Session) (*types.Artifact, error) {
	current, err := h.runGroupMembers(ctx, runID, groupID, group, input, sess)
	if err != nil {
		return nil, err
	}

	synthKey := group.Lead.Desk + "-synthesize"
	if synth, ok := h.store.Run().LoadStep(runID, groupID, synthKey); ok {
		return synth, nil
	}
	result, err := h.runGroupDesk(ctx, runID, groupID, group.Lead.Desk, current, sess)
	if err != nil {
		return nil, err
	}
	if result != nil {
		h.store.Run().SaveStep(runID, groupID, synthKey, result)
	}
	return result, nil
}

func (h *Hub) runGroupDesk(ctx context.Context, runID, groupID, deskID string, input *types.Artifact, sess *session.Session) (*types.Artifact, error) {
	desk, ok := h.desks[deskID]
	if !ok {
		return nil, fmt.Errorf("hub: group %q: desk %q not found", groupID, deskID)
	}

	if desk.Executor.Type == types.ExecutorTypeHuman {
		artifact, err := h.waitHumanInput(ctx, deskID)
		if err != nil {
			return nil, err
		}
		sess.Post(store.Message{DeskID: deskID, Role: "user", Content: string(artifact.Payload)})
		h.store.Desk().Save(deskID, artifact)
		return artifact, nil
	}

	artifact, err := h.executeDesk(ctx, runID, deskID, groupID, "", desk, input, sess)
	if err != nil {
		return nil, err
	}
	if artifact != nil {
		// Self-governed skip: if output starts with "SKIP", treat as no-op.
		if routing.IsSkip(artifact.Payload) {
			h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventType("step.skipped")})
			return nil, nil
		}
		sess.Post(store.Message{DeskID: deskID, Role: "assistant", Content: string(artifact.Payload)})
		h.store.Desk().Save(deskID, artifact)
	}
	return artifact, nil
}

func (h *Hub) runDeskActor(ctx context.Context, deskID string, desk *types.Desk, ev types.Event) error {
	input := &types.Artifact{Payload: ev.Payload}
	runID := newRunID(deskID)

	ctx, cancel := context.WithCancel(ctx)
	h.registerRun(runID, cancel)
	defer h.deregisterRun(runID)
	defer cancel()

	h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepStarted})

	artifact, err := h.executeDesk(ctx, runID, deskID, "", ev.Type, desk, input, nil)
	if err != nil {
		// Context cancellation means the hub is shutting down or restarting itself
		// (e.g. upgrade.sh writes a restart marker → syscall.Exec kills in-flight processes).
		// This is not a real desk failure — suppress the step.failed event and return quietly.
		if ctx.Err() == context.Canceled {
			return nil
		}
		h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepFailed, Error: err.Error()})
		h.bus.PublishAsync(ctx, types.Event{Type: "step.failed", Source: deskID, Payload: []byte(err.Error())})
		return err
	}

	// Self-governed skip for standalone desks.
	if artifact != nil && routing.IsSkip(artifact.Payload) {
		h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventType("step.skipped")})
		return nil
	}

	h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepCompleted})

	if artifact != nil {
		h.store.Desk().Save(deskID, artifact)
		for _, emitType := range desk.Emit {
			// Determine the actual event type based on payload content
			actualType := routing.DetermineEventType(emitType, artifact.Payload)
			h.bus.PublishAsync(ctx, types.Event{
				Type:    actualType,
				Source:  deskID,
				Payload: artifact.Payload,
			})
		}
	}

	return nil
}

// executeDesk builds the task and dispatches to the executor.
func (h *Hub) executeDesk(ctx context.Context, runID, deskID, groupID, eventType string, desk *types.Desk, input *types.Artifact, groupSession *session.Session) (*types.Artifact, error) {
	agent := h.agents[desk.Agent.ID]

	var prompt string
	{
		var skills []string
		if agent != nil {
			skills = append(skills, agent.Skills...)
		}
		skills = append(skills, desk.Skills...)
		var knowhow []string
		if agent != nil {
			knowhow = agent.Knowhow
		}
		if len(skills) > 0 || len(knowhow) > 0 {
			p, err := skill.BuildPrompt(ctx, h.skills, skills, knowhow, input)
			if err != nil {
				return nil, fmt.Errorf("hub: desk %s: %w", deskID, err)
			}
			prompt = p
		}
	}

	options := make(map[string]string, len(desk.Executor.Params)+1)
	for k, v := range desk.Executor.Params {
		options[k] = v
	}
	if desk.Executor.SDK != "" {
		options["sdk"] = string(desk.Executor.SDK)
	}

	agentID := ""
	if agent != nil {
		agentID = agent.ID
	}

	var sessionEntries []sdk.SessionEntry
	if desk.Session.MaxEntries == nil || *desk.Session.MaxEntries != 0 {
		loaded := h.store.DeskSession().Load(deskID)
		// Apply per-desk max_entries limit if configured.
		if desk.Session.MaxEntries != nil && *desk.Session.MaxEntries > 0 && len(loaded) > *desk.Session.MaxEntries {
			loaded = loaded[len(loaded)-*desk.Session.MaxEntries:]
		}
		for _, e := range loaded {
			sessionEntries = append(sessionEntries, sdk.SessionEntry{Role: e.Role, Content: e.Content})
		}
	}

	var groupHistory []sdk.GroupMessage
	if groupSession != nil {
		msgs, _ := groupSession.History()
		for _, msg := range msgs {
			groupHistory = append(groupHistory, sdk.GroupMessage{DeskID: msg.DeskID, Role: msg.Role, Content: msg.Content})
		}
	}

	taskResources := h.resolveResources(deskID)

	execCtx := ctx
	if desk.Policy != "" {
		if pol, ok := h.policies[desk.Policy]; ok && pol.Timeout > 0 {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, pol.Timeout)
			defer cancel()
		}
	}

	// Load notes for the effective scope (group if inside one, otherwise desk).
	noteScope := deskID
	if groupID != "" {
		noteScope = groupID
	}
	notes := h.store.Notes().All(noteScope)

	task := sdk.Task{
		RunID:          runID,
		AgentID:        agentID,
		DeskID:         deskID,
		GroupID:        groupID,
		EventType:      eventType,
		Prompt:         prompt,
		Input:          input,
		Options:        options,
		Env:            desk.Executor.Env,
		WorkDir:        h.projectDir,
		Session:        sessionEntries,
		GroupHistory:   groupHistory,
		Resources:      taskResources,
		Notes:          notes,
	}

	started := time.Now()
	h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepStarted, InputBytes: len(prompt), Model: options["model"]})

	var artifact *types.Artifact
	var execErr error

	maxAttempts := 1
	if desk.Policy != "" {
		if pol, ok := h.policies[desk.Policy]; ok && pol.Retry > 0 {
			maxAttempts = pol.Retry + 1
		}
	}

	// SDK executor: use shared gRPC process (entry-point discovery).
	if desk.Executor.Type == types.ExecutorTypeSDK {
		client, err := h.sdkProcs.GetOrStart(execCtx)
		if err != nil {
			return nil, err
		}
		result, err := sdkproc.Execute(execCtx, client, task)
		if err != nil {
			return nil, err
		}
		// Apply note updates returned by the SDK agent.
		for _, u := range result.NoteUpdates {
			if u.Operation == "delete" {
				h.store.Notes().Delete(noteScope, u.Key)
			} else {
				h.store.Notes().Set(noteScope, u.Key, u.Value)
			}
		}
		// Publish emitted events to the bus.
		for _, em := range result.Emissions {
			h.bus.PublishAsync(ctx, types.Event{
				Type:    em.EventType,
				Payload: em.Payload,
			})
		}
		// Record metrics reported by the SDK agent.
		if len(result.Metrics) > 0 {
			h.recordMetricsFull(runID, deskID, task.AgentID, result.Metrics)
		}
		// Save step/result logs to session for dashboard display.
		if len(result.Logs) > 0 && desk.Parent != "" {
			if sess := h.sessions.Get(desk.Parent); sess != nil {
				for _, l := range result.Logs {
					_ = sess.Post(store.Message{
						DeskID:  deskID,
						Role:    "agent",
						Type:    l.Type,
						Content: l.Content,
					})
				}
			}
		}
		return result.Artifact, nil
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		artifact, execErr = h.registry.Dispatch(execCtx, desk.Executor.Type, task)
		if execErr == nil {
			break
		}
		if attempt < maxAttempts-1 {
			h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepFailed, Error: fmt.Sprintf("attempt %d: %v", attempt+1, execErr)})
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}

	elapsed := time.Since(started).Milliseconds()
	if execErr != nil {
		h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepFailed, DurationMs: elapsed, Error: execErr.Error()})

		// Escalation: if policy defines escalate_to, emit escalation event.
		if desk.Policy != "" {
			if pol, ok := h.policies[desk.Policy]; ok && pol.EscalateTo != "" {
				action := pol.OnError
				if action == "" {
					action = "fail"
				}
				if action == "escalate" {
					h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventType("policy.escalated"), Error: fmt.Sprintf("escalating to %s", pol.EscalateTo)})
					h.bus.PublishAsync(ctx, types.Event{
						Type:    "escalation",
						Source:  deskID,
						Payload: []byte(fmt.Sprintf("Desk %s failed after %d attempts: %s", deskID, maxAttempts, execErr.Error())),
					})
				}
			}
		}

		return nil, fmt.Errorf("hub: desk %s: %w", deskID, execErr)
	}

	outBytes := 0
	inputTokens := 0
	outputTokens := 0
	var metrics map[string]float64
	if artifact != nil {
		outBytes = len(artifact.Payload)
		if artifact.Meta != nil {
			if v, err := strconv.Atoi(artifact.Meta["input_tokens"]); err == nil {
				inputTokens = v
			}
			if v, err := strconv.Atoi(artifact.Meta["output_tokens"]); err == nil {
				outputTokens = v
			}
			// Collect custom metrics from artifact.Meta (keys prefixed with "metric:").
			for k, v := range artifact.Meta {
				if strings.HasPrefix(k, "metric:") {
					if metrics == nil {
						metrics = make(map[string]float64)
					}
					name := strings.TrimPrefix(k, "metric:")
					if f, err := strconv.ParseFloat(v, 64); err == nil {
						metrics[name] = f
					}
				}
			}
		}
	}
	var outputPreview string
	if artifact != nil && len(artifact.Payload) > 0 {
		raw := artifact.Payload
		if len(raw) > 2048 {
			raw = raw[:2048]
		}
		outputPreview = string(raw)
	}
	h.recorder.Record(observe.Event{
		RunID: runID, StepID: deskID, Type: observe.EventStepCompleted,
		DurationMs: elapsed, OutputBytes: outBytes,
		InputTokens: inputTokens, OutputTokens: outputTokens,
		Model: options["model"], Metrics: metrics, Output: outputPreview,
	})

	// Check cost limit enforcement.
	cost := estimateCost(options["model"], inputTokens, outputTokens)
	if desk.Policy != "" {
		if pol, ok := h.policies[desk.Policy]; ok && pol.CostLimit != "" {
			limit := parseCostLimit(pol.CostLimit)
			if limit > 0 && cost > limit {
				h.recorder.Record(observe.Event{
					RunID: runID, StepID: deskID,
					Type:  observe.EventType("policy.cost_exceeded"),
					Error: fmt.Sprintf("cost $%.4f exceeds limit %s", cost, pol.CostLimit),
				})
			}
		}
	}

	// Hierarchical budget tracking: desk cost → group budget → org budget.
	h.checkBudget(runID, deskID, cost)

	// Artifact schema enforcement.
	if artifact != nil {
		h.checkArtifactSchema(runID, deskID, artifact.Schema)
	}

	if prompt != "" {
		h.store.DeskSession().Append(deskID, runID, store.SessionEntry{Role: "user", Content: prompt, At: started})
	}
	if artifact != nil && len(artifact.Payload) > 0 {
		h.store.DeskSession().Append(deskID, runID, store.SessionEntry{Role: "assistant", Content: string(artifact.Payload), At: time.Now()})

		// Extract and save knowhow from the result.
		if kh := knowhow.Extract(string(artifact.Payload)); kh != "" {
			if err := knowhow.Save(h.projectDir, deskID, kh); err != nil {
				h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventType("knowhow.save.failed"), Error: err.Error()})
			} else {
				h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventType("knowhow.saved")})
			}
		}
	}

	return artifact, nil
}

// resolveResources collects resources accessible to a desk and returns their config.
// Resources are pure configuration — the agent handles all interaction itself.
func (h *Hub) resolveResources(deskID string) []sdk.TaskResource {
	accessibleIDs := make(map[string]bool)

	if desk, ok := h.desks[deskID]; ok {
		for _, resID := range desk.Resources {
			accessibleIDs[resID] = true
		}
	}
	for gid := range h.groups {
		if h.deskInGroup(gid, deskID) {
			for _, resID := range h.groups[gid].Resources {
				accessibleIDs[resID] = true
			}
		}
	}
	if h.organization != nil {
		for _, resID := range h.organization.Resources {
			accessibleIDs[resID] = true
		}
	}

	var taskResources []sdk.TaskResource
	for resID := range accessibleIDs {
		res, ok := h.resources[resID]
		if !ok {
			continue
		}
		cfg := make(map[string]string, len(res.Config)+2)
		for k, v := range res.Config {
			cfg[k] = v
		}
		if res.MCP != "" {
			cfg["mcp"] = res.MCP
		}
		if res.Connection != "" {
			cfg["connection"] = res.Connection
		}
		taskResources = append(taskResources, sdk.TaskResource{
			ID:     resID,
			Type:   res.Type,
			Config: cfg,
		})
	}
	return taskResources
}


func (h *Hub) waitHumanInput(ctx context.Context, deskID string) (*types.Artifact, error) {
	ch := make(chan *types.Artifact, 1)
	h.mu.Lock()
	h.humanInputs[deskID] = ch
	h.mu.Unlock()

	h.recorder.Record(observe.Event{StepID: deskID, Type: observe.EventHumanInputWaiting})

	select {
	case <-ctx.Done():
		h.mu.Lock()
		delete(h.humanInputs, deskID)
		h.mu.Unlock()
		return nil, ctx.Err()
	case artifact := <-ch:
		h.mu.Lock()
		delete(h.humanInputs, deskID)
		h.mu.Unlock()
		h.recorder.Record(observe.Event{StepID: deskID, Type: observe.EventHumanInputReceived, OutputBytes: len(artifact.Payload)})
		return artifact, nil
	}
}

// DeskSession returns the session history for a desk (most recent entries first).
func (h *Hub) DeskSession(deskID string) ([]store.SessionEntry, bool) {
	h.mu.RLock()
	_, known := h.desks[deskID]
	h.mu.RUnlock()
	if !known {
		return nil, false
	}
	entries := h.store.DeskSession().Load(deskID)
	// Reverse so most recent entries come first.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, true
}

// DeskArtifact returns the content of the most recent artifact stored for a desk.
func (h *Hub) DeskArtifact(deskID string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, known := h.desks[deskID]
	artifacts, ok := h.store.Desk().Get(deskID)
	if !ok || len(artifacts) == 0 {
		return "", known
	}
	last := artifacts[len(artifacts)-1]
	if last == nil {
		return "", true
	}
	return string(last.Payload), true
}

// Warnings checks all agents' skill references against the resolver
// and returns warnings for any that cannot be found.
func (h *Hub) Warnings() []types.Warning {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var warnings []types.Warning
	ctx := context.Background()
	for id, agent := range h.agents {
		for _, ref := range agent.Skills {
			if _, err := h.skills.Resolve(ctx, ref); err != nil {
				warnings = append(warnings, types.Warning{
					Level:   "warn",
					Source:  "agent:" + id,
					Message: fmt.Sprintf("skill %q not found", ref),
				})
			}
		}
		for _, ref := range agent.Knowhow {
			if _, err := h.skills.Resolve(ctx, ref); err != nil {
				warnings = append(warnings, types.Warning{
					Level:   "info",
					Source:  "agent:" + id,
					Message: fmt.Sprintf("knowhow %q not found (will accumulate over time)", ref),
				})
			}
		}
	}
	for id, desk := range h.desks {
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


// groupDesks returns IDs of desks whose parent is groupID.
func (h *Hub) groupDesks(groupID string) []string {
	var ids []string
	for id, d := range h.desks {
		if d.Parent == groupID {
			ids = append(ids, id)
		}
	}
	return ids
}

// groupSubGroups returns IDs of groups whose parent is groupID.
func (h *Hub) groupSubGroups(groupID string) []string {
	var ids []string
	for id, g := range h.groups {
		if g.Parent == groupID {
			ids = append(ids, id)
		}
	}
	return ids
}

// deskInGroup reports whether deskID is a member or lead of groupID.
func (h *Hub) deskInGroup(groupID, deskID string) bool {
	group, ok := h.groups[groupID]
	if !ok {
		return false
	}
	if group.Lead != nil && group.Lead.Desk == deskID {
		return true
	}
	if desk, ok := h.desks[deskID]; ok {
		return desk.Parent == groupID
	}
	return false
}

func (h *Hub) watchResource(ctx context.Context, resourceID string, res *types.Resource) {
	w := resource.NewWatcher(res)
	go w.Start(ctx) //nolint:errcheck
	for ev := range w.Events() {
		h.bus.PublishAsync(ctx, ev)
	}
}

func newRunID(prefix string) string {
	ts := time.Now().Format("20060102-150405")
	short := uuid.NewString()[:7]
	return prefix + "-" + ts + "-" + short
}



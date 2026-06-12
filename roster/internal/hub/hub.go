package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/roster-io/roster/internal/event"
	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/internal/event/queue"
	"github.com/roster-io/roster/internal/resource"
	"github.com/roster-io/roster/internal/exec/sdkproc"
	"github.com/roster-io/roster/internal/agent/skill"
	"github.com/roster-io/roster/proto"
	"github.com/roster-io/roster/internal/store"
	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// Dispatcher routes tasks to executors.
type Dispatcher interface {
	Dispatch(ctx context.Context, t types.ExecutorType, task sdk.Task) (*types.Output, error)
}

// Hub is the event-driven orchestrator.
// Events are queued per subscriber and processed sequentially.
// Queues persist to disk — on restart, unfinished work resumes.
type Hub struct {
	registry Dispatcher
	store    store.Store
	skills   *skill.Resolver
	bus      *event.Bus
	recorder *observe.Recorder

	mu           sync.RWMutex
	organization *types.Organization
	desks        map[string]*types.Desk
	agents       map[string]*types.Agent
	groups       map[string]*types.Group
	resources    map[string]*types.Resource
	projectDir   string
	queueDir     string

	deskEmitters   map[string]*event.DeskEmitter
	queues         map[string]queue.Queue
	humanInputs    map[string]chan string
	runningWorkers map[string]struct{}

	activeRuns   map[string]context.CancelFunc
	activeRunsMu sync.Mutex

	// Loop circuit breaker: per-event-type emission counts and cooldowns.
	loopCounts   map[string]int
	loopCooldown map[string]time.Time

	budget   *BudgetTracker
	sdkProcs *sdkproc.ProcessManager
}

func New(registry Dispatcher, store store.Store, skills *skill.Resolver, recorder *observe.Recorder) *Hub {
	return &Hub{
		registry:       registry,
		store:          store,
		skills:         skills,
		bus:            event.NewBus(10000),
		recorder:       recorder,
		deskEmitters:   make(map[string]*event.DeskEmitter),
		desks:          make(map[string]*types.Desk),
		agents:         make(map[string]*types.Agent),
		groups:         make(map[string]*types.Group),
		resources:      make(map[string]*types.Resource),
		queues:         make(map[string]queue.Queue),
		humanInputs:    make(map[string]chan string),
		runningWorkers: make(map[string]struct{}),
		activeRuns:     make(map[string]context.CancelFunc),
		loopCounts:     make(map[string]int),
		loopCooldown:   make(map[string]time.Time),
		budget:         newBudgetTracker(),
		sdkProcs:       sdkproc.NewProcessManager(50100),
	}
}

// Load registers all config into the hub.
func (h *Hub) Load(org *types.Organization, agents map[string]*types.Agent, desks map[string]*types.Desk, groups map[string]*types.Group, resources map[string]*types.Resource) {
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
}

// Start wires up event subscriptions and starts queue workers.
func (h *Hub) Start(ctx context.Context) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if err := h.sdkProcs.EnsureSDK(ctx, h.agents, h.resources); err != nil {
		return fmt.Errorf("hub: sdk setup: %w", err)
	}

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

	for id, q := range h.queues {
		if recovered := q.RequeueProcessing(); recovered > 0 {
			h.recorder.Record(observe.Event{StepID: id, Type: observe.EventQueueRecovered})
		}
		if collapsed := q.CollapseIDlessPending(); collapsed > 0 {
			h.recorder.Record(observe.Event{StepID: id, Type: observe.EventQueueCollapsed, OutputBytes: collapsed})
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
						h.recorder.Record(observe.Event{StepID: id, Type: observe.EventQueueGC, OutputBytes: removed})
					}
				}
				h.mu.RUnlock()
			}
		}
	}()

	// Cron: schedule periodic event emission.
	if h.organization != nil && len(h.organization.Cron) > 0 {
		for _, entry := range h.organization.Cron {
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
	h.recorder.Record(observe.Event{StepID: subscriberID, Type: observe.EventQueuePushed})
}

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
func (h *Hub) Reload(ctx context.Context, org *types.Organization, agents map[string]*types.Agent, desks map[string]*types.Desk, groups map[string]*types.Group, resources map[string]*types.Resource) {
	h.mu.Lock()

	if org != nil {
		h.organization = org
	}

	for id, a := range agents {
		h.agents[id] = a
	}

	for id, d := range desks {
		if _, exists := h.desks[id]; exists {
			h.desks[id] = d
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
	if h.loopBreakerTripped(ev.Type) {
		return
	}

	h.recorder.Record(observe.Event{
		Type:   observe.EventPublished,
		StepID: ev.Source,
	})
	h.bus.PublishAsync(ctx, ev)
}

// loopBreakerTripped returns true if the event type should be suppressed.
func (h *Hub) loopBreakerTripped(eventType string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.organization == nil || h.organization.Limits.MaxIterations <= 0 {
		return false
	}
	limits := h.organization.Limits

	// Check cooldown: if this event type was previously tripped and is still cooling down, suppress.
	if until, ok := h.loopCooldown[eventType]; ok {
		if time.Now().Before(until) {
			return true
		}
		// Cooldown expired — reset counter and remove cooldown.
		delete(h.loopCooldown, eventType)
		h.loopCounts[eventType] = 0
	}

	h.loopCounts[eventType]++
	if h.loopCounts[eventType] <= limits.MaxIterations {
		return false
	}

	// Tripped — record event and start cooldown.
	h.recorder.Record(observe.Event{
		Type:  observe.EventLoopBreaker,
		Error: fmt.Sprintf("event %q exceeded max_iterations (%d); suppressed", eventType, limits.MaxIterations),
	})

	if limits.Cooldown != "" {
		if d, err := time.ParseDuration(limits.Cooldown); err == nil {
			h.loopCooldown[eventType] = time.Now().Add(d)
		}
	}
	return true
}

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
		StepID:  deskID,
		Type:    observe.EventMetrics,
		Metrics: m,
	})
	for name, value := range m {
		_ = h.store.RecordMetric(runID, deskID, agentID, name, value)
	}
}

func (h *Hub) GetMetrics(deskID string) map[string]map[string]float64 {
	rows, err := h.store.MetricsByScope(deskID)
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

func (h *Hub) deliverToTarget(ctx context.Context, targetID string, ev types.Event, stableRunID string) error {
	// Groups fan-out to member desks — group is just a session scope, not an executor.
	if _, ok := h.groups[targetID]; ok {
		for _, deskID := range h.groupDesks(targetID) {
			desk, ok := h.desks[deskID]
			if !ok {
				continue
			}
			go func(did string, d *types.Desk) {
				if err := h.runDeskActor(ctx, did, d, ev); err != nil {
					// Individual desk errors don't fail the group fan-out.
				}
			}(deskID, desk)
		}
		return nil
	}
	if desk, ok := h.desks[targetID]; ok {
		return h.runDeskActor(ctx, targetID, desk, ev)
	}
	return fmt.Errorf("hub: routing target %q not found", targetID)
}

func (h *Hub) runDeskActor(ctx context.Context, deskID string, desk *types.Desk, ev types.Event) error {
	runID := newRunID(deskID)
	groupID := desk.Parent // empty if standalone, group ID if member

	if desk.Timeout != "" {
		dur, parseErr := time.ParseDuration(desk.Timeout)
		if parseErr != nil {
			return fmt.Errorf("hub: desk %s: invalid timeout %q: %w", deskID, desk.Timeout, parseErr)
		}
		var tCancel context.CancelFunc
		ctx, tCancel = context.WithTimeout(ctx, dur)
		defer tCancel()
	}

	ctx, cancel := context.WithCancel(ctx)
	h.registerRun(runID, cancel)
	defer h.deregisterRun(runID)
	defer cancel()

	h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepStarted})

	emitNames, err := h.executeDesk(ctx, runID, deskID, groupID, ev.Type, desk)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded && desk.Timeout != "" {
			h.recorder.Record(observe.Event{
				RunID:  runID,
				StepID: deskID,
				Type:   observe.EventStepTimedOut,
				Error:  fmt.Sprintf("desk timed out after %s", desk.Timeout),
			})
			h.bus.PublishAsync(ctx, types.Event{Type: deskID + ".timed_out", Source: deskID})
			if groupID != "" {
				h.bus.PublishAsync(ctx, types.Event{Type: groupID + "." + deskID + ".timed_out", Source: deskID})
			}
			return fmt.Errorf("hub: desk %s timed out after %s", deskID, desk.Timeout)
		}
		if ctx.Err() == context.Canceled {
			return nil
		}
		h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepFailed, Error: err.Error()})
		h.bus.PublishAsync(ctx, types.Event{Type: deskID + ".failed", Source: deskID})
		if groupID != "" {
			h.bus.PublishAsync(ctx, types.Event{Type: groupID + "." + deskID + ".failed", Source: deskID})
		}
		return err
	}

	h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepCompleted})

	// Events are trigger-only — no payload.
	for _, name := range emitNames {
		h.bus.PublishAsync(ctx, types.Event{Type: deskID + "." + name, Source: deskID})
		if groupID != "" {
			h.bus.PublishAsync(ctx, types.Event{Type: groupID + "." + deskID + "." + name, Source: deskID})
		}
	}

	return nil
}

// executeDesk runs a desk and returns (emitNames, error).
// emitNames are bare event names from the SDK (e.g. "done") — callers add namespace prefixes.
func (h *Hub) executeDesk(ctx context.Context, runID, deskID, groupID, eventType string, desk *types.Desk) ([]string, error) {
	agent := h.agents[desk.Agent.ID]

	var prompt string
	{
		var skills []string
		if agent != nil {
			skills = append(skills, agent.Skills...)
		}
		skills = append(skills, desk.Skills...)
		if len(skills) > 0 {
			p, err := skill.BuildPrompt(ctx, h.skills, skills, nil)
			if err != nil {
				return nil, fmt.Errorf("hub: desk %s: %w", deskID, err)
			}
			prompt = p
		}
	}

	options := make(map[string]string, len(desk.Executor.Params)+2)
	for k, v := range desk.Executor.Params {
		options[k] = v
	}
	if desk.Executor.SDK != "" {
		options["sdk"] = string(desk.Executor.SDK)
	}
	if desk.Role != "" {
		options["role"] = desk.Role
	}
	if desk.Goal != "" {
		options["goal"] = desk.Goal
	}

	agentID := desk.Agent.ID
	if agent != nil {
		agentID = agent.ID
	}

	var sessionEntries []sdk.SessionEntry
	if desk.Session.MaxEntries == nil || *desk.Session.MaxEntries != 0 {
		limit := 0
		if desk.Session.MaxEntries != nil && *desk.Session.MaxEntries > 0 {
			limit = *desk.Session.MaxEntries
		}
		loaded := h.store.LoadSession(deskID, limit)
		for _, e := range loaded {
			sessionEntries = append(sessionEntries, sdk.SessionEntry{Role: e.Role, Content: e.Content})
		}
	}

	var groupHistory []sdk.GroupMessage
	if groupID != "" {
		groupEntries := h.store.LoadSession(groupID, 0)
		for _, e := range groupEntries {
			groupHistory = append(groupHistory, sdk.GroupMessage{DeskID: e.SourceID, Role: e.Role, Content: e.Content})
		}
	}

	taskResources := h.resolveResources(deskID)

	// Inject [Resources] section into the prompt so agents see what they can access.
	if len(taskResources) > 0 {
		var rb strings.Builder
		rb.WriteString("\n[Resources]\n")
		for _, r := range taskResources {
			rb.WriteString("  " + r.ID + ": {")
			first := true
			for _, k := range []string{"description", "path", "connection"} {
				if v, ok := r.Config[k]; ok && v != "" {
					if !first {
						rb.WriteString(", ")
					}
					rb.WriteString(fmt.Sprintf("%q: %q", k, v))
					first = false
				}
			}
			rb.WriteString("}\n")
		}
		prompt += rb.String()
	}

	noteScope := deskID
	if groupID != "" {
		noteScope = groupID
	}
	notes := h.store.AllNotes(noteScope)

	skillContents := make(map[string]string)
	allSkills := desk.Skills
	if agent != nil {
		allSkills = append(allSkills, agent.Skills...)
	}
	for _, skillName := range allSkills {
		if skill, err := h.skills.Resolve(ctx, skillName); err == nil {
			skillContents[skillName] = skill.Prompt
		}
	}

	task := sdk.Task{
		RunID:        runID,
		AgentID:      agentID,
		DeskID:       deskID,
		GroupID:      groupID,
		EventType:    eventType,
		Prompt:       prompt,
		Options:      options,
		Env:          desk.Executor.Env,
		WorkDir:      h.projectDir,
		Session:      sessionEntries,
		GroupHistory: groupHistory,
		Resources:    taskResources,
		Notes:        notes,
		Skills:       skillContents,
	}

	started := time.Now()

	// SDK executor: use shared gRPC process.
	// On connection failure, reset the process and retry once.
	if desk.Executor.Type == types.ExecutorTypeSDK {
		// Real-time log callback: record each log as it streams from the SDK agent.
		onLog := func(l *proto.LogEntry) {
			logAt := time.Now()
			if l.Timestamp > 0 {
				logAt = time.UnixMilli(l.Timestamp)
			}
			h.store.AppendLog(deskID, runID, store.LogEntry{Type: l.Type, Content: l.Content, At: logAt})
			h.recorder.Record(observe.Event{
				RunID:      runID,
				StepID:     deskID,
				Type:       observe.EventStepLog,
				LogType:    l.Type,
				LogContent: l.Content,
			})
		}
		var result *sdkproc.Result
		for attempt := 0; attempt < 2; attempt++ {
			client, err := h.sdkProcs.GetOrStart(ctx)
			if err != nil {
				if attempt == 0 {
					h.sdkProcs.Reset()
					continue
				}
				return nil, err
			}
			result, err = sdkproc.Execute(ctx, client, task, onLog)
			if err != nil {
				if attempt == 0 && isGRPCConnError(err) {
					h.sdkProcs.Reset()
					continue
				}
				return nil, err
			}
			break
		}
		for _, u := range result.NoteUpdates {
			if u.Operation == "delete" {
				h.store.DeleteNote(noteScope, u.Key)
			} else {
				h.store.SetNote(noteScope, u.Key, u.Value)
			}
		}
		var emitNames []string
		for _, em := range result.Emissions {
			if em.EventType != "" {
				emitNames = append(emitNames, em.EventType)
			}
		}
		if len(emitNames) == 0 {
			emitNames = []string{"done"}
		}
		if len(desk.Emit) > 0 {
			allowed := make(map[string]bool, len(desk.Emit))
			for _, e := range desk.Emit {
				allowed[e] = true
			}
			var valid []string
			var rejected []string
			for _, name := range emitNames {
				if allowed[name] {
					valid = append(valid, name)
				} else {
					rejected = append(rejected, name)
				}
			}
			if len(rejected) > 0 {
				h.recorder.Record(observe.Event{
					RunID:  runID,
					StepID: deskID,
					Type:   observe.EventEmitRejected,
					Error:  fmt.Sprintf("rejected emissions [%s]; allowed: [%s]", strings.Join(rejected, ", "), strings.Join(desk.Emit, ", ")),
				})
			}
			if len(valid) > 0 {
				emitNames = valid
			} else {
				emitNames = []string{"done"}
			}
		}
		if len(result.Metrics) > 0 {
			h.recordMetricsFull(runID, deskID, task.AgentID, result.Metrics)
		}
		var outputContent string
		if result.Output != nil {
			outputContent = result.Output.Content
		}
		h.saveSession(deskID, groupID, runID, prompt, outputContent, started)
		return emitNames, nil
	}

	output, execErr := h.registry.Dispatch(ctx, desk.Executor.Type, task)

	elapsed := time.Since(started).Milliseconds()
	if execErr != nil {
		h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepFailed, DurationMs: elapsed, Error: execErr.Error()})
		return nil, fmt.Errorf("hub: desk %s: %w", deskID, execErr)
	}

	var outBytes int
	var outputContent string
	var inputTokens, outputTokens int
	var metrics map[string]float64
	if output != nil {
		outputContent = output.Content
		outBytes = len(outputContent)
		metrics = output.Metrics
		if metrics != nil {
			if v, ok := metrics["input_tokens"]; ok {
				inputTokens = int(v)
			}
			if v, ok := metrics["output_tokens"]; ok {
				outputTokens = int(v)
			}
		}
	}
	var outputPreview string
	if outBytes > 0 {
		outputPreview = outputContent
		if len(outputPreview) > 2048 {
			outputPreview = outputPreview[:2048]
		}
	}
	h.recorder.Record(observe.Event{
		RunID: runID, StepID: deskID, Type: observe.EventStepCompleted,
		DurationMs: elapsed, OutputBytes: outBytes,
		InputTokens: inputTokens, OutputTokens: outputTokens,
		Model: options["model"], Metrics: metrics, Output: outputPreview,
	})

	cost := estimateCost(options["model"], inputTokens, outputTokens)
	h.checkBudget(runID, deskID, cost)

	h.saveSession(deskID, groupID, runID, prompt, outputContent, started)
	return []string{"done"}, nil
}

func (h *Hub) resolveResources(deskID string) []sdk.TaskResource {
	accessibleIDs := make(map[string]bool)

	// Desk-level resources.
	if desk, ok := h.desks[deskID]; ok {
		for _, resID := range desk.Resources {
			accessibleIDs[resID] = true
		}
	}
	// Walk up the group hierarchy: desk.Parent → parent.Parent → ...
	if desk, ok := h.desks[deskID]; ok {
		gid := desk.Parent
		for gid != "" {
			if g, ok := h.groups[gid]; ok {
				for _, resID := range g.Resources {
					accessibleIDs[resID] = true
				}
				gid = g.Parent
			} else {
				break
			}
		}
	}
	// Org-level resources.
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
			if k == "path" && v != "" && !filepath.IsAbs(v) && h.projectDir != "" {
				v = filepath.Join(h.projectDir, v)
			}
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

func (h *Hub) waitHumanInput(ctx context.Context, deskID string) (string, error) {
	ch := make(chan string, 1)
	h.mu.Lock()
	h.humanInputs[deskID] = ch
	h.mu.Unlock()

	h.recorder.Record(observe.Event{StepID: deskID, Type: observe.EventHumanInputWaiting})

	select {
	case <-ctx.Done():
		h.mu.Lock()
		delete(h.humanInputs, deskID)
		h.mu.Unlock()
		return "", ctx.Err()
	case content := <-ch:
		h.mu.Lock()
		delete(h.humanInputs, deskID)
		h.mu.Unlock()
		h.recorder.Record(observe.Event{StepID: deskID, Type: observe.EventHumanInputReceived, OutputBytes: len(content)})
		return content, nil
	}
}

func (h *Hub) DeskSession(deskID string) ([]store.SessionEntry, bool) {
	h.mu.RLock()
	_, known := h.desks[deskID]
	h.mu.RUnlock()
	if !known {
		return nil, false
	}
	entries := h.store.LoadSession(deskID, 0)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, true
}

// saveSession stores prompt + result in desk session (and group session if grouped).
func (h *Hub) saveSession(deskID, groupID, runID, prompt, output string, started time.Time) {
	if prompt != "" {
		h.store.AppendSession(deskID, runID, store.SessionEntry{Role: "user", Content: prompt, At: started})
	}
	if output != "" {
		h.store.AppendSession(deskID, runID, store.SessionEntry{Role: "assistant", Content: output, At: time.Now()})
	}
	// Also append to group session scope so group members share context.
	if groupID != "" {
		if prompt != "" {
			h.store.AppendSession(groupID, runID, store.SessionEntry{SourceID: deskID, Role: "user", Content: prompt, At: started})
		}
		if output != "" {
			h.store.AppendSession(groupID, runID, store.SessionEntry{SourceID: deskID, Role: "assistant", Content: output, At: time.Now()})
		}
	}
}

func (h *Hub) DeskLogs(deskID string) []store.LogEntry {
	return h.store.LoadLogs(deskID)
}

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

func (h *Hub) groupDesks(groupID string) []string {
	var ids []string
	for id, d := range h.desks {
		if d.Parent == groupID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (h *Hub) groupSubGroups(groupID string) []string {
	var ids []string
	for id, g := range h.groups {
		if g.Parent == groupID {
			ids = append(ids, id)
		}
	}
	return ids
}

func (h *Hub) deskInGroup(groupID, deskID string) bool {
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

// isGRPCConnError reports whether err is a gRPC connectivity failure
// (Unavailable or connection-closed) that warrants an SDK restart.
func isGRPCConnError(err error) bool {
	if s, ok := grpcstatus.FromError(err); ok {
		switch s.Code() {
		case codes.Unavailable, codes.Internal:
			return true
		}
	}
	return false
}

func newRunID(prefix string) string {
	ts := time.Now().Format("20060102-150405")
	short := uuid.NewString()[:7]
	return prefix + "-" + ts + "-" + short
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


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
	projectDir   string
	queueDir     string

	queues         map[string]queue.Queue
	humanInputs    map[string]chan *types.Artifact
	runningWorkers map[string]struct{}

	activeRuns   map[string]context.CancelFunc
	activeRunsMu sync.Mutex

	budget   *BudgetTracker
	sdkProcs *sdkproc.ProcessManager
}

func New(registry Dispatcher, store store.Store, skills *skill.Resolver, recorder *observe.Recorder) *Hub {
	return &Hub{
		registry:       registry,
		store:          store,
		skills:         skills,
		sessions:       session.NewManager(store.Group()),
		bus:            event.NewBus(10000),
		recorder:       recorder,
		desks:          make(map[string]*types.Desk),
		agents:         make(map[string]*types.Agent),
		groups:         make(map[string]*types.Group),
		resources:      make(map[string]*types.Resource),
		queues:         make(map[string]queue.Queue),
		humanInputs:    make(map[string]chan *types.Artifact),
		runningWorkers: make(map[string]struct{}),
		activeRuns:     make(map[string]context.CancelFunc),
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
			h.recorder.Record(observe.Event{StepID: id, Type: observe.EventType("queue.recovered")})
		}
		if collapsed := q.CollapseIDlessPending(); collapsed > 0 {
			h.recorder.Record(observe.Event{StepID: id, Type: observe.EventType("queue.collapsed"), OutputBytes: collapsed})
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
						h.recorder.Record(observe.Event{StepID: id, Type: observe.EventType("queue.gc"), OutputBytes: removed})
					}
				}
				h.mu.RUnlock()
			}
		}
	}()

	h.bus.PublishAsync(ctx, types.Event{Type: "hub.started", Source: "hub"})

	return nil
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
	h.recorder.Record(observe.Event{StepID: subscriberID, Type: observe.EventType("queue.pushed")})
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

	h.recorder.Record(observe.Event{Type: observe.EventType("hub.reloaded")})
}

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

func (h *Hub) EmitSync(ctx context.Context, ev types.Event) []error {
	return h.bus.Publish(ctx, ev)
}

func (h *Hub) Bus() *event.Bus { return h.bus }
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
func (h *Hub) Events() []observe.Event                 { return h.recorder.Events() }
func (h *Hub) Subscribe() (chan observe.Event, func()) { return h.recorder.Subscribe() }

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
	if group, ok := h.groups[targetID]; ok {
		return h.runGroupActor(ctx, targetID, group, ev, stableRunID)
	}
	if desk, ok := h.desks[targetID]; ok {
		return h.runDeskActor(ctx, targetID, desk, ev)
	}
	return fmt.Errorf("hub: routing target %q not found", targetID)
}

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

	result, err := h.runGroupSequential(ctx, runID, groupID, group, input, sess)
	if err != nil {
		h.recorder.Record(observe.Event{RunID: runID, StepID: groupID, Type: observe.EventStepFailed, Error: err.Error()})
		h.bus.PublishAsync(ctx, types.Event{Type: "step.failed", Source: groupID, Payload: []byte(err.Error())})
		return err
	}

	h.recorder.Record(observe.Event{RunID: runID, StepID: groupID, Type: observe.EventStepCompleted})

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

	for _, deskID := range h.groupDesks(groupID) {
		checkpointKey := deskID
		if saved, ok := h.store.Run().LoadStep(runID, groupID, checkpointKey); ok {
			current = saved
			continue
		}

		artifact, err := h.runGroupDesk(ctx, runID, groupID, deskID, current, sess)
		if err != nil {
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

	return current, nil
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
		if ctx.Err() == context.Canceled {
			return nil
		}
		h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepFailed, Error: err.Error()})
		h.bus.PublishAsync(ctx, types.Event{Type: "step.failed", Source: deskID, Payload: []byte(err.Error())})
		return err
	}

	if artifact != nil && routing.IsSkip(artifact.Payload) {
		h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventType("step.skipped")})
		return nil
	}

	h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepCompleted})

	if artifact != nil {
		h.store.Desk().Save(deskID, artifact)
		for _, emitType := range desk.Emit {
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

func (h *Hub) executeDesk(ctx context.Context, runID, deskID, groupID, eventType string, desk *types.Desk, input *types.Artifact, groupSession *session.Session) (*types.Artifact, error) {
	agent := h.agents[desk.Agent.ID]

	var prompt string
	{
		var skills []string
		if agent != nil {
			skills = append(skills, agent.Skills...)
		}
		skills = append(skills, desk.Skills...)
		if len(skills) > 0 {
			p, err := skill.BuildPrompt(ctx, h.skills, skills, nil, input)
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

	agentID := ""
	if agent != nil {
		agentID = agent.ID
	}

	var sessionEntries []sdk.SessionEntry
	if desk.Session.MaxEntries == nil || *desk.Session.MaxEntries != 0 {
		loaded := h.store.DeskSession().Load(deskID)
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

	noteScope := deskID
	if groupID != "" {
		noteScope = groupID
	}
	notes := h.store.Notes().All(noteScope)

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
		Input:        input,
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
	h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepStarted, InputBytes: len(prompt), Model: options["model"]})

	// SDK executor: use shared gRPC process.
	if desk.Executor.Type == types.ExecutorTypeSDK {
		client, err := h.sdkProcs.GetOrStart(ctx)
		if err != nil {
			return nil, err
		}
		result, err := sdkproc.Execute(ctx, client, task)
		if err != nil {
			return nil, err
		}
		for _, u := range result.NoteUpdates {
			if u.Operation == "delete" {
				h.store.Notes().Delete(noteScope, u.Key)
			} else {
				h.store.Notes().Set(noteScope, u.Key, u.Value)
			}
		}
		for _, em := range result.Emissions {
			h.bus.PublishAsync(ctx, types.Event{
				Type:    em.EventType,
				Payload: em.Payload,
			})
		}
		if len(result.Metrics) > 0 {
			h.recordMetricsFull(runID, deskID, task.AgentID, result.Metrics)
		}
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

	artifact, execErr := h.registry.Dispatch(ctx, desk.Executor.Type, task)

	elapsed := time.Since(started).Milliseconds()
	if execErr != nil {
		h.recorder.Record(observe.Event{RunID: runID, StepID: deskID, Type: observe.EventStepFailed, DurationMs: elapsed, Error: execErr.Error()})
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

	cost := estimateCost(options["model"], inputTokens, outputTokens)
	h.checkBudget(runID, deskID, cost)

	if artifact != nil {
		h.checkArtifactSchema(runID, deskID, artifact.Schema)
	}

	if prompt != "" {
		h.store.DeskSession().Append(deskID, runID, store.SessionEntry{Role: "user", Content: prompt, At: started})
	}
	if artifact != nil && len(artifact.Payload) > 0 {
		h.store.DeskSession().Append(deskID, runID, store.SessionEntry{Role: "assistant", Content: string(artifact.Payload), At: time.Now()})

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

func (h *Hub) DeskSession(deskID string) ([]store.SessionEntry, bool) {
	h.mu.RLock()
	_, known := h.desks[deskID]
	h.mu.RUnlock()
	if !known {
		return nil, false
	}
	entries := h.store.DeskSession().Load(deskID)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, true
}

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


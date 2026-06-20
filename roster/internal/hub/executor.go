package hub

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	deskdomain "github.com/roster-io/roster/domain/desk"
	knowhowdomain "github.com/roster-io/roster/domain/knowhow"
	sessiondomain "github.com/roster-io/roster/domain/session"
	"github.com/roster-io/roster/internal/exec/sdkproc"
	"github.com/roster-io/roster/internal/store"
	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/proto"
	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// defaultDeskTimeout is applied when a desk has no explicit timeout configured.
// Human executor desks are exempt (they legitimately wait for user input).
const defaultDeskTimeout = 30 * time.Minute

func (h *Hub) runDeskActor(ctx context.Context, deskID string, desk *types.Desk, ev types.Event) error {
	runID := newRunID(deskID)
	groupIDs := desk.Groups

	// Apply timeout: explicit > default (skip human desks).
	timeoutStr := desk.Timeout
	if timeoutStr == "" && desk.Executor.Type != types.ExecutorTypeHuman {
		timeoutStr = defaultDeskTimeout.String()
	}
	if timeoutStr != "" {
		dur, parseErr := time.ParseDuration(timeoutStr)
		if parseErr != nil {
			return fmt.Errorf("hub: desk %s: invalid timeout %q: %w", deskID, timeoutStr, parseErr)
		}
		var tCancel context.CancelFunc
		ctx, tCancel = context.WithTimeout(ctx, dur)
		defer tCancel()
	}

	ctx, cancel := context.WithCancel(ctx)
	h.registerRun(runID, cancel)
	defer h.deregisterRun(runID)
	defer cancel()

	h.recorder.Record(observe.Event{RunID: runID, DeskID: deskID, Type: observe.EventStepStarted})

	emitNames, err := h.executeDesk(ctx, runID, deskID, groupIDs, ev.Type, desk)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			h.recorder.Record(observe.Event{
				RunID:  runID,
				DeskID: deskID,
				Type:   observe.EventStepTimedOut,
				Error:  fmt.Sprintf("desk timed out after %s", timeoutStr),
			})
			for _, fqn := range deskdomain.EventNames(deskID, groupIDs, "timed_out") {
				h.bus.PublishAsync(ctx, types.Event{Type: fqn, Source: deskID})
			}
			return fmt.Errorf("hub: desk %s timed out after %s", deskID, timeoutStr)
		}
		if ctx.Err() == context.Canceled {
			return nil
		}
		h.recorder.Record(observe.Event{RunID: runID, DeskID: deskID, Type: observe.EventStepFailed, Error: err.Error()})
		for _, fqn := range deskdomain.EventNames(deskID, groupIDs, "failed") {
			h.bus.PublishAsync(ctx, types.Event{Type: fqn, Source: deskID})
		}
		return err
	}

	h.recorder.Record(observe.Event{RunID: runID, DeskID: deskID, Type: observe.EventStepCompleted})

	// Events are trigger-only — no payload.
	for _, name := range emitNames {
		for _, fqn := range deskdomain.EventNames(deskID, groupIDs, name) {
			h.bus.PublishAsync(ctx, types.Event{Type: fqn, Source: deskID})
		}
	}

	return nil
}

// executeDesk runs a desk and returns (emitNames, error).
func (h *Hub) executeDesk(ctx context.Context, runID, deskID string, groupIDs []string, eventType string, desk *types.Desk) ([]string, error) {
	agent := h.reg.agents[desk.Agent.ID]

	// --- Budget pre-run check ---
	if err := h.checkBudgetPreRun(deskID, desk); err != nil {
		return nil, err
	}

	// --- Options ---
	options := make(map[string]string, len(desk.Executor.Params)+2)
	for k, v := range desk.Executor.Params {
		options[k] = v
	}
	// Resolve relative script command paths against the project directory
	// so SDK agents can find them regardless of their working directory.
	if cmd := options["command"]; cmd != "" && !filepath.IsAbs(cmd) && h.projectDir != "" {
		// Only resolve the executable portion (first token before spaces).
		// Handles "scripts/foo.sh --flag" by resolving only "scripts/foo.sh".
		if idx := strings.IndexByte(cmd, ' '); idx > 0 {
			exe := cmd[:idx]
			if !filepath.IsAbs(exe) {
				options["command"] = filepath.Join(h.projectDir, exe) + cmd[idx:]
			}
		} else {
			options["command"] = filepath.Join(h.projectDir, cmd)
		}
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

	// --- Skills: resolve once, skip for script agents ---
	isScript := deskdomain.IsScript(desk.Executor.Params)
	skillContents := make(map[string]string)
	if !isScript {
		var allSkills []string
		if agent != nil {
			allSkills = append(allSkills, agent.Skills...)
		}
		allSkills = append(allSkills, desk.Skills...)
		seen := make(map[string]bool, len(allSkills))
		for _, name := range allSkills {
			if seen[name] {
				continue
			}
			seen[name] = true
			if s, err := h.skills.Resolve(ctx, name); err == nil {
				skillContents[name] = s.Prompt
			}
		}
		// Load accumulated knowhow for this desk.
		maxKH := 10
		if desk.Session.MaxKnowhow != nil {
			maxKH = *desk.Session.MaxKnowhow
		}
		khEntries := h.knowhow.LoadKnowhow(deskID, maxKH)
		if len(khEntries) > 0 {
			var khParts []string
			for _, e := range khEntries {
				khParts = append(khParts, e.Content)
			}
			skillContents["__knowhow"] = "## Knowhow (learned from past work)\n\n" + strings.Join(khParts, "\n\n---\n\n")
		}
	}

	// --- Session ---
	var sessionEntries []sdk.SessionEntry
	if desk.Session.MaxEntries == nil || *desk.Session.MaxEntries != 0 {
		limit := 0
		if desk.Session.MaxEntries != nil && *desk.Session.MaxEntries > 0 {
			limit = *desk.Session.MaxEntries
		}
		loaded := h.sessions.LoadSession(deskID, limit)
		for _, e := range loaded {
			sessionEntries = append(sessionEntries, sdk.SessionEntry{Role: e.Role, Content: e.Content})
		}
	}

	var groupHistory []sdk.GroupMessage
	for _, gid := range groupIDs {
		groupEntries := h.sessions.LoadSession(gid, 0)
		for _, e := range groupEntries {
			groupHistory = append(groupHistory, sdk.GroupMessage{DeskID: e.SourceID, Role: e.Role, Content: e.Content})
		}
	}

	// --- Resources ---
	taskResources := h.resolveResources(deskID)

	// --- Notes ---
	noteScope := deskID
	if len(groupIDs) > 0 {
		noteScope = groupIDs[0]
	}
	notes := h.notes.AllNotes(noteScope)

	// --- Build prompt ---
	var prompt string
	if desk.Executor.Type != types.ExecutorTypeSDK && len(skillContents) > 0 {
		var parts []string
		for _, content := range skillContents {
			parts = append(parts, strings.TrimSpace(content))
		}
		prompt = strings.Join(parts, "\n\n")
	}

	task := sdk.Task{
		RunID:        runID,
		AgentID:      agentID,
		DeskID:       deskID,
		GroupID:      desk.PrimaryGroup(),
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
	if desk.Executor.Type == types.ExecutorTypeSDK {
		onLog := func(l *proto.LogEntry) {
			logAt := time.Now()
			if l.Timestamp > 0 {
				logAt = time.UnixMilli(l.Timestamp)
			}
			h.logs.AppendLog(deskID, runID, store.LogEntry{Type: l.Type, Content: l.Content, At: logAt})
			h.recorder.Record(observe.Event{
				RunID: runID, DeskID: deskID, Type: observe.EventStepLog,
				LogType: l.Type, LogContent: l.Content,
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
				h.notes.DeleteNote(noteScope, u.Key)
			} else {
				h.notes.SetNote(noteScope, u.Key, u.Value)
			}
		}
		var rawEmits []string
		for _, em := range result.Emissions {
			if em.EventType != "" {
				rawEmits = append(rawEmits, em.EventType)
			}
		}
		emitNames, rejected := deskdomain.ResolveEmissions(rawEmits, desk.Emit)
		if len(rejected) > 0 {
			h.recorder.Record(observe.Event{
				RunID: runID, DeskID: deskID, Type: observe.EventEmitRejected,
				Error: deskdomain.FormatRejectedError(rejected, desk.Emit),
			})
		}
		if len(result.Metrics) > 0 {
			h.recordMetricsFull(runID, deskID, task.AgentID, result.Metrics)
		}
		var outputContent string
		if result.Output != nil {
			outputContent = result.Output.Content
		}
		sc := buildSessionContext(eventType, skillContents, taskResources)
		h.saveSession(deskID, groupIDs, runID, sc, outputContent, started)
		h.extractKnowhow(deskID, desk, outputContent)
		return emitNames, nil
	}

	// Human executor: block until a human submits input via the API.
	if desk.Executor.Type == types.ExecutorTypeHuman {
		content, waitErr := h.waitHumanInput(ctx, deskID)
		if waitErr != nil {
			return nil, fmt.Errorf("hub: desk %s: waiting for human input: %w", deskID, waitErr)
		}
		sc := buildSessionContext(eventType, skillContents, taskResources)
		h.saveSession(deskID, groupIDs, runID, sc, content, started)
		emitNames, rejected := deskdomain.ResolveEmissions([]string{"done"}, desk.Emit)
		if len(rejected) > 0 {
			h.recorder.Record(observe.Event{
				RunID: runID, DeskID: deskID, Type: observe.EventEmitRejected,
				Error: deskdomain.FormatRejectedError(rejected, desk.Emit),
			})
		}
		return emitNames, nil
	}

	// Non-SDK executor path.
	output, execErr := h.dispatcher.Dispatch(ctx, desk.Executor.Type, task)

	elapsed := time.Since(started).Milliseconds()
	if execErr != nil {
		h.recorder.Record(observe.Event{RunID: runID, DeskID: deskID, Type: observe.EventStepFailed, DurationMs: elapsed, Error: execErr.Error()})
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
		RunID: runID, DeskID: deskID, Type: observe.EventStepCompleted,
		DurationMs: elapsed, OutputBytes: outBytes,
		InputTokens: inputTokens, OutputTokens: outputTokens,
		Model: options["model"], Metrics: metrics, Output: outputPreview,
	})

	cost := estimateCost(options["model"], inputTokens, outputTokens)
	if err := h.checkBudget(runID, deskID, cost); err != nil {
		return nil, err
	}

	sc := buildSessionContext(eventType, skillContents, taskResources)
	h.saveSession(deskID, groupIDs, runID, sc, outputContent, started)
	h.extractKnowhow(deskID, desk, outputContent)
	return []string{"done"}, nil
}

// extractKnowhow checks output for knowhow sections and saves them.
func (h *Hub) extractKnowhow(deskID string, desk *types.Desk, output string) {
	if output == "" {
		return
	}
	kh := knowhowdomain.Extract(output)
	if kh == "" {
		return
	}
	_ = h.knowhow.SaveKnowhow(deskID, kh)
	maxKH := 10
	if desk.Session.MaxKnowhow != nil {
		maxKH = *desk.Session.MaxKnowhow
	}
	_ = h.knowhow.PruneKnowhow(deskID, maxKH)
}

func buildSessionContext(eventType string, skills map[string]string, resources []sdk.TaskResource) sessiondomain.Context {
	skillNames := make([]string, 0, len(skills))
	for name := range skills {
		skillNames = append(skillNames, name)
	}
	resourceIDs := make([]string, 0, len(resources))
	for _, r := range resources {
		resourceIDs = append(resourceIDs, r.ID)
	}
	return sessiondomain.BuildContext(eventType, skillNames, resourceIDs)
}

func (h *Hub) resolveResources(deskID string) []sdk.TaskResource {
	accessibleIDs := make(map[string]bool)

	if desk, ok := h.reg.desks[deskID]; ok {
		for _, resID := range desk.Resources {
			accessibleIDs[resID] = true
		}
	}
	if desk, ok := h.reg.desks[deskID]; ok {
		for _, gid := range desk.Groups {
			for cur := gid; cur != ""; {
				g, ok := h.reg.groups[cur]
				if !ok {
					break
				}
				for _, resID := range g.Resources {
					accessibleIDs[resID] = true
				}
				cur = g.Parent
			}
		}
	}
	if h.reg.organization != nil {
		for _, resID := range h.reg.organization.Resources {
			accessibleIDs[resID] = true
		}
	}

	var taskResources []sdk.TaskResource
	for resID := range accessibleIDs {
		res, ok := h.reg.resources[resID]
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

// saveSession stores execution context + result in desk session and all group sessions.
func (h *Hub) saveSession(deskID string, groupIDs []string, runID string, sc sessiondomain.Context, output string, started time.Time) {
	inputEntry := store.SessionEntry{Role: "user", Content: sc.Message, Meta: sc.Meta, At: started}
	h.sessions.AppendSession(deskID, runID, inputEntry)

	if output != "" {
		h.sessions.AppendSession(deskID, runID, store.SessionEntry{Role: "assistant", Content: output, At: time.Now()})
	}
	for _, gid := range groupIDs {
		groupInput := inputEntry
		groupInput.SourceID = deskID
		h.sessions.AppendSession(gid, runID, groupInput)
		if output != "" {
			h.sessions.AppendSession(gid, runID, store.SessionEntry{SourceID: deskID, Role: "assistant", Content: output, At: time.Now()})
		}
	}
}

func (h *Hub) waitHumanInput(ctx context.Context, deskID string) (string, error) {
	ch := make(chan string, 1)
	h.mu.Lock()
	h.humanInputs[deskID] = ch
	h.mu.Unlock()

	h.recorder.Record(observe.Event{DeskID: deskID, Type: observe.EventHumanInputWaiting})

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
		h.recorder.Record(observe.Event{DeskID: deskID, Type: observe.EventHumanInputReceived, OutputBytes: len(content)})
		return content, nil
	}
}

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

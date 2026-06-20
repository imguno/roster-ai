package hub

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/roster-io/roster/internal/agent/skill"
	"github.com/roster-io/roster/internal/event/routing"
	"github.com/roster-io/roster/internal/store/memory"
	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

type fakeDispatcher struct {
	mu    sync.Mutex
	tasks []sdk.Task
}

func (f *fakeDispatcher) Dispatch(ctx context.Context, t types.ExecutorType, task sdk.Task) (*types.Output, error) {
	f.mu.Lock()
	f.tasks = append(f.tasks, task)
	f.mu.Unlock()
	return &types.Output{
		Content: "output from " + task.DeskID,
	}, nil
}

func newTestHub(t *testing.T, dispatcher *fakeDispatcher) *Hub {
	store := memory.New()
	recorder := observe.NewRecorder()
	resolver := skill.NewResolver(".")
	h := New(dispatcher, store, resolver, recorder)
	h.SetQueueDir(t.TempDir())
	return h
}

// waitFor polls until condition is true or timeout.
func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestDeskSubscription(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	h.Load(
		&types.Organization{ID: "test-org"},
		map[string]*types.Agent{"review-agent": {ID: "review-agent"}},
		map[string]*types.Desk{
			"reviewer": {
				ID:        "reviewer",
				Agent:     types.AgentRef{ID: "review-agent"},
				Subscribe: []string{"task.created"},
				Executor:  types.ExecutorConfig{Type: types.ExecutorTypeExec, Params: map[string]string{"command": "echo test"}},
			},
		},
		nil, nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.Start(ctx)

	h.Emit(ctx, types.Event{Type: "task.created", Source: "test", Payload: []byte("review this")})

	waitFor(t, 8*time.Second, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.tasks) >= 1
	})

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tasks[0].DeskID != "reviewer" {
		t.Errorf("expected desk 'reviewer', got %q", d.tasks[0].DeskID)
	}
}

func TestDesksSubscribeDirectly(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	h.Load(
		&types.Organization{ID: "test-org"},
		map[string]*types.Agent{
			"lead-agent":   {ID: "lead-agent"},
			"worker-agent": {ID: "worker-agent"},
		},
		map[string]*types.Desk{
			"lead":     {ID: "lead", Groups: []string{"dev-team"}, Agent: types.AgentRef{ID: "lead-agent"}, Subscribe: []string{"work.start"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
			"worker-a": {ID: "worker-a", Groups: []string{"dev-team"}, Agent: types.AgentRef{ID: "worker-agent"}, Subscribe: []string{"work.start"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
		},
		map[string]*types.Group{
			"dev-team": {ID: "dev-team"},
		},
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.Start(ctx)

	h.Emit(ctx, types.Event{Type: "work.start", Source: "test"})

	// Both desks subscribe directly and should be dispatched.
	waitFor(t, 8*time.Second, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.tasks) >= 2
	})

	d.mu.Lock()
	defer d.mu.Unlock()
	dispatched := map[string]bool{}
	for _, task := range d.tasks {
		dispatched[task.DeskID] = true
	}
	if !dispatched["lead"] {
		t.Error("expected lead to be dispatched")
	}
	if !dispatched["worker-a"] {
		t.Error("expected worker-a to be dispatched")
	}
}

func TestDeskEmitChain(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	h.Load(
		&types.Organization{ID: "test-org"},
		map[string]*types.Agent{"a": {ID: "a"}},
		map[string]*types.Desk{
			"worker":   {ID: "worker", Groups: []string{"dev-team"}, Agent: types.AgentRef{ID: "a"}, Subscribe: []string{"plan.ready"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
			"reporter": {ID: "reporter", Agent: types.AgentRef{ID: "a"}, Subscribe: []string{"worker.done"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
		},
		map[string]*types.Group{
			"dev-team": {ID: "dev-team"},
		},
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.Start(ctx)

	h.Emit(ctx, types.Event{Type: "plan.ready", Source: "strategy"})

	// worker subscribes directly, then emits worker.done which triggers reporter.
	waitFor(t, 8*time.Second, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.tasks) >= 2
	})

	d.mu.Lock()
	defer d.mu.Unlock()
	dispatched := map[string]bool{}
	for _, task := range d.tasks {
		dispatched[task.DeskID] = true
	}
	if !dispatched["worker"] {
		t.Error("expected worker to be dispatched")
	}
	if !dispatched["reporter"] {
		t.Error("expected reporter to be dispatched via worker.done")
	}
}

func TestResourceBindingInGroup(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	h.Load(
		&types.Organization{ID: "test-org"},
		map[string]*types.Agent{"a": {ID: "a"}},
		map[string]*types.Desk{
			"worker": {ID: "worker", Groups: []string{"dev-team"}, Agent: types.AgentRef{ID: "a"}, Subscribe: []string{"work.start"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
		},
		map[string]*types.Group{
			"dev-team": {ID: "dev-team", Resources: []string{"codebase"}},
		},
		map[string]*types.Resource{
			"codebase": {
				ID: "codebase", Type: "github",
				Config: map[string]string{"repo": "my-org/my-repo"},
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.Start(ctx)

	h.Emit(ctx, types.Event{Type: "work.start", Source: "test"})

	waitFor(t, 8*time.Second, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.tasks) >= 1
	})

	d.mu.Lock()
	defer d.mu.Unlock()
	task := d.tasks[0]
	hasCodebase := false
	for _, r := range task.Resources {
		if r.ID == "codebase" {
			hasCodebase = true
		}
	}
	if !hasCodebase {
		t.Errorf("expected codebase resource in %v", task.Resources)
	}
}

func TestDetermineEventType(t *testing.T) {
	quotedFailureButActualSuccess := []byte(`
--- input from previous step ---
Build is clean. The fix was already committed — "failed" removed from failurePatterns
in hub.go:856, with explicit structured patterns remaining ("=== build failed ===",
"build failed", etc.).
---
=== Build succeeded ===
binary: ./bin/roster-new
---
=== All tests passed ===
`)

	tests := []struct {
		name         string
		declaredType string
		payload      []byte
		want         string
	}{
		{
			name:         "quoted failure but actual success → convert build.failed",
			declaredType: "build.failed",
			payload:      quotedFailureButActualSuccess,
			want:         "build.succeeded",
		},
		{
			name:         "quoted failure but actual success → convert test.failed",
			declaredType: "test.failed",
			payload:      quotedFailureButActualSuccess,
			want:         "test.passed",
		},
		{
			name:         "clear success → convert build.failed",
			declaredType: "build.failed",
			payload:      []byte("=== Build succeeded ===\nbinary: ./bin/roster"),
			want:         "build.succeeded",
		},
		{
			name:         "clear failure → keep build.failed",
			declaredType: "build.failed",
			payload:      []byte("=== Build failed ===\nsyntax error"),
			want:         "build.failed",
		},
		{
			name:         "clear failure → convert build.succeeded",
			declaredType: "build.succeeded",
			payload:      []byte("=== Build failed ===\nsyntax error"),
			want:         "build.failed",
		},
		{
			name:         "no markers → return declared type unchanged",
			declaredType: "build.failed",
			payload:      []byte("something happened"),
			want:         "build.failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routing.DetermineEventType(tt.declaredType, tt.payload)
			if got != tt.want {
				t.Errorf("DetermineEventType(%q, ...) = %q, want %q", tt.declaredType, got, tt.want)
			}
		})
	}
}

func TestDeskEmit(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	h.Load(
		&types.Organization{ID: "test-org"},
		map[string]*types.Agent{"a": {ID: "a"}},
		map[string]*types.Desk{
			"producer": {ID: "producer", Agent: types.AgentRef{ID: "a"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
		},
		nil, nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Subscribe to result.text on the raw bus to capture the emitted event.
	var captured []types.Event
	var capMu sync.Mutex
	h.Bus().Subscribe("test-listener", []string{"result.text"}, func(_ context.Context, ev types.Event) error {
		capMu.Lock()
		captured = append(captured, ev)
		capMu.Unlock()
		return nil
	})

	// Use DeskEmitter to emit a typed event from the producer desk.
	emitter := h.DeskEmitter("producer")
	err := emitter.Emit(ctx, "result.text", map[string]string{"text": "hello"})
	if err != nil {
		t.Fatalf("DeskEmitter.Emit failed: %v", err)
	}

	waitFor(t, 3*time.Second, func() bool {
		capMu.Lock()
		defer capMu.Unlock()
		return len(captured) > 0
	})

	capMu.Lock()
	defer capMu.Unlock()
	if len(captured) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(captured))
	}
	ev := captured[0]
	if ev.Type != "result.text" {
		t.Errorf("expected type result.text, got %s", ev.Type)
	}
	if ev.Source != "producer" {
		t.Errorf("expected source producer, got %s", ev.Source)
	}
	if string(ev.Payload) != `{"text":"hello"}` {
		t.Errorf("unexpected payload: %s", string(ev.Payload))
	}
}

func TestEventNotRouted(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	h.Load(
		&types.Organization{ID: "test-org"},
		map[string]*types.Agent{"a": {ID: "a"}},
		map[string]*types.Desk{
			"reviewer": {ID: "reviewer", Agent: types.AgentRef{ID: "a"}, Subscribe: []string{"task.created"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
		},
		nil, nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.Start(ctx)

	h.Emit(ctx, types.Event{Type: "unknown.event"})
	time.Sleep(3 * time.Second)

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.tasks) != 0 {
		t.Errorf("expected 0 dispatches for unmatched event, got %d", len(d.tasks))
	}
}

func TestCleanupStaleRuns(t *testing.T) {
	recorder := observe.NewRecorder()

	// Simulate a previous session: two started runs, one completed, one orphaned.
	past := time.Now().Add(-10 * time.Minute)
	recorder.Record(observe.Event{RunID: "run-1", DeskID: "desk-a", Type: observe.EventDeskStarted, At: past})
	recorder.Record(observe.Event{RunID: "run-1", DeskID: "desk-a", Type: observe.EventDeskCompleted, At: past.Add(time.Minute)})
	recorder.Record(observe.Event{RunID: "run-2", DeskID: "desk-b", Type: observe.EventDeskStarted, At: past})
	// run-2 has no terminal event — it's orphaned.

	store := memory.New()
	resolver := skill.NewResolver(".")
	h := New(&fakeDispatcher{}, store, resolver, recorder)
	h.SetQueueDir(t.TempDir())

	h.cleanupStaleRuns()

	events := recorder.Events()
	var failedCount int
	for _, ev := range events {
		if ev.RunID == "run-2" && ev.Type == observe.EventDeskFailed {
			failedCount++
			if ev.Error != "hub restarted — run was orphaned" {
				t.Errorf("unexpected error message: %s", ev.Error)
			}
		}
	}
	if failedCount != 1 {
		t.Errorf("expected 1 stale run cleanup for run-2, got %d", failedCount)
	}

	// Verify run-1 was NOT marked as failed (it already completed).
	for _, ev := range events {
		if ev.RunID == "run-1" && ev.Type == observe.EventDeskFailed {
			t.Error("run-1 should not have been marked as failed — it was already completed")
		}
	}
}

func TestReloadUpdatesSubscriptions(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	desks := map[string]*types.Desk{
		"writer": {
			Name:      "writer",
			Groups:    []string{"team"},
			Executor:  types.ExecutorConfig{Type: types.ExecutorTypeExec, Params: map[string]string{"command": "echo hi"}},
			Subscribe: []string{"task.created"},
			Emit:      []string{"done"},
		},
	}
	groups := map[string]*types.Group{
		"team": {Name: "team"},
	}
	agents := map[string]*types.Agent{}
	resources := map[string]*types.Resource{}
	org := &types.Organization{Name: "test"}

	h.Load(org, agents, desks, groups, resources)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Emit task.created — writer should receive it.
	h.Emit(ctx, types.Event{Type: "task.created", Source: "test"})
	waitFor(t, 5*time.Second, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.tasks) == 1
	})

	// Reload with updated subscriptions: writer now listens to plan.approved instead.
	updatedDesks := map[string]*types.Desk{
		"writer": {
			Name:      "writer",
			Groups:    []string{"team"},
			Executor:  types.ExecutorConfig{Type: types.ExecutorTypeExec, Params: map[string]string{"command": "echo hi"}},
			Subscribe: []string{"plan.approved"},
			Emit:      []string{"done"},
		},
	}
	h.Reload(ctx, org, agents, updatedDesks, groups, resources)

	// Clear dispatched tasks.
	d.mu.Lock()
	d.tasks = nil
	d.mu.Unlock()

	// Emit task.created — should NOT reach writer anymore.
	h.Emit(ctx, types.Event{Type: "task.created", Source: "test"})
	time.Sleep(500 * time.Millisecond)
	d.mu.Lock()
	if len(d.tasks) != 0 {
		t.Errorf("writer should not receive task.created after reload, got %d tasks", len(d.tasks))
	}
	d.mu.Unlock()

	// Emit plan.approved — writer SHOULD receive it now.
	h.Emit(ctx, types.Event{Type: "plan.approved", Source: "test"})
	waitFor(t, 5*time.Second, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.tasks) == 1
	})
}

func TestRehydrateBudget(t *testing.T) {
	recorder := observe.NewRecorder()

	// Simulate completed events with token usage from a previous session.
	past := time.Now().Add(-2 * time.Hour)
	recorder.Record(observe.Event{
		RunID: "run-1", DeskID: "dev", Type: observe.EventDeskCompleted,
		At: past, InputTokens: 1000, OutputTokens: 500, Model: "claude-sonnet-4-6",
	})
	recorder.Record(observe.Event{
		RunID: "run-2", DeskID: "dev", Type: observe.EventDeskCompleted,
		At: past.Add(time.Hour), InputTokens: 2000, OutputTokens: 1000, Model: "claude-sonnet-4-6",
	})
	// A completed event with no tokens should be skipped.
	recorder.Record(observe.Event{
		RunID: "run-3", DeskID: "reviewer", Type: observe.EventDeskCompleted,
		At: past,
	})

	s := memory.New()
	resolver := skill.NewResolver(".")
	h := New(&fakeDispatcher{}, s, resolver, recorder)
	h.SetQueueDir(t.TempDir())

	h.rehydrateBudget()

	total := h.budget.Total("desk:dev")
	if total <= 0 {
		t.Fatalf("expected positive budget total for desk:dev, got %f", total)
	}

	// Should have two cost entries, both contributing.
	cost1 := estimateCost("claude-sonnet-4-6", 1000, 500)
	cost2 := estimateCost("claude-sonnet-4-6", 2000, 1000)
	expected := cost1 + cost2
	if total != expected {
		t.Errorf("budget total: got %f, want %f", total, expected)
	}

	// Reviewer should have zero (no tokens).
	if rev := h.budget.Total("desk:reviewer"); rev != 0 {
		t.Errorf("expected 0 for desk:reviewer, got %f", rev)
	}

	// Daily totals should also work (events were 2h ago, within 24h window).
	daily := h.budget.DailyTotal("desk:dev")
	if daily != expected {
		t.Errorf("daily total: got %f, want %f", daily, expected)
	}
}

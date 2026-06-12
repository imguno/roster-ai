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

func TestGroupFanOut(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	h.Load(
		&types.Organization{ID: "test-org"},
		map[string]*types.Agent{
			"lead-agent":   {ID: "lead-agent"},
			"worker-agent": {ID: "worker-agent"},
		},
		map[string]*types.Desk{
			"lead":     {ID: "lead", Parent: "dev-team", Agent: types.AgentRef{ID: "lead-agent"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
			"worker-a": {ID: "worker-a", Parent: "dev-team", Agent: types.AgentRef{ID: "worker-agent"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
		},
		map[string]*types.Group{
			"dev-team": {ID: "dev-team", Subscribe: []string{"work.start"}},
		},
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.Start(ctx)

	h.Emit(ctx, types.Event{Type: "work.start", Source: "test"})

	// Fan-out: both lead and worker-a should be dispatched.
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

func TestGroupDeskEmit(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	h.Load(
		&types.Organization{ID: "test-org"},
		map[string]*types.Agent{"a": {ID: "a"}},
		map[string]*types.Desk{
			"worker":   {ID: "worker", Parent: "dev-team", Agent: types.AgentRef{ID: "a"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
			"reporter": {ID: "reporter", Agent: types.AgentRef{ID: "a"}, Subscribe: []string{"worker.done"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
		},
		map[string]*types.Group{
			"dev-team": {ID: "dev-team", Subscribe: []string{"plan.ready"}},
		},
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.Start(ctx)

	h.Emit(ctx, types.Event{Type: "plan.ready", Source: "strategy"})

	// worker runs (from dev-team fan-out) then emits worker.done which triggers reporter.
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
			"worker": {ID: "worker", Parent: "dev-team", Agent: types.AgentRef{ID: "a"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
		},
		map[string]*types.Group{
			"dev-team": {ID: "dev-team", Subscribe: []string{"work.start"}, Resources: []string{"codebase"}},
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

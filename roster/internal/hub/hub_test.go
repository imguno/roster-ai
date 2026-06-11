package hub

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/internal/event/routing"
	"github.com/roster-io/roster/internal/agent/skill"
	"github.com/roster-io/roster/internal/store/memory"
	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

type fakeDispatcher struct {
	mu    sync.Mutex
	tasks []sdk.Task
}

func (f *fakeDispatcher) Dispatch(ctx context.Context, t types.ExecutorType, task sdk.Task) (*types.Artifact, error) {
	f.mu.Lock()
	f.tasks = append(f.tasks, task)
	f.mu.Unlock()
	return &types.Artifact{
		ID:      "art-" + task.DeskID,
		Payload: []byte("output from " + task.DeskID),
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
		nil, nil, nil,
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

func TestGroupCoordination(t *testing.T) {
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
			"dev-team": {ID: "dev-team", Subscribe: []string{"work.start"}, Lead: &types.GroupLead{Desk: "lead", Position: "both"}},
		},
		nil, nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.Start(ctx)

	h.Emit(ctx, types.Event{Type: "work.start", Source: "test"})

	// "both" mode: lead(plan) → worker-a → lead(synthesize) = 3 dispatches
	waitFor(t, 8*time.Second, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.tasks) >= 3
	})

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tasks[0].DeskID != "lead" {
		t.Errorf("first dispatch should be lead, got %q", d.tasks[0].DeskID)
	}
	if d.tasks[1].DeskID != "worker-a" {
		t.Errorf("second dispatch should be worker-a, got %q", d.tasks[1].DeskID)
	}
	if d.tasks[2].DeskID != "lead" {
		t.Errorf("third dispatch should be lead, got %q", d.tasks[2].DeskID)
	}
}

func TestGroupEmit(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	h.Load(
		&types.Organization{ID: "test-org"},
		map[string]*types.Agent{"a": {ID: "a"}},
		map[string]*types.Desk{
			"worker":   {ID: "worker", Parent: "dev-team", Agent: types.AgentRef{ID: "a"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
			"reporter": {ID: "reporter", Agent: types.AgentRef{ID: "a"}, Subscribe: []string{"dev.done"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
		},
		map[string]*types.Group{
			"dev-team": {ID: "dev-team", Subscribe: []string{"plan.ready"}, Emit: []string{"dev.done"}},
		},
		nil, nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.Start(ctx)

	h.Emit(ctx, types.Event{Type: "plan.ready", Source: "strategy"})

	// worker ran (from dev-team) + reporter ran (from dev.done emit)
	waitFor(t, 8*time.Second, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.tasks) >= 2
	})

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tasks[0].DeskID != "worker" {
		t.Errorf("expected first dispatch to worker, got %q", d.tasks[0].DeskID)
	}
	if d.tasks[1].DeskID != "reporter" {
		t.Errorf("expected second dispatch to reporter, got %q", d.tasks[1].DeskID)
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
		nil,
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

func TestGroupCheckpointResume(t *testing.T) {
	d := &fakeDispatcher{}
	h := newTestHub(t, d)

	h.Load(
		nil,
		map[string]*types.Agent{"a": {ID: "a"}},
		map[string]*types.Desk{
			"desk1": {ID: "desk1", Parent: "my-group", Agent: types.AgentRef{ID: "a"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
			"desk2": {ID: "desk2", Parent: "my-group", Agent: types.AgentRef{ID: "a"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
			"desk3": {ID: "desk3", Parent: "my-group", Agent: types.AgentRef{ID: "a"}, Executor: types.ExecutorConfig{Type: types.ExecutorTypeExec}},
		},
		map[string]*types.Group{
			"my-group": {ID: "my-group"},
		},
		nil, nil,
	)

	stableRunID := "my-group-1"
	h.store.Run().SaveStep(stableRunID, "my-group", "desk1-round0", &types.Artifact{Payload: []byte("desk1 output")})
	h.store.Run().SaveStep(stableRunID, "my-group", "desk2-round0", &types.Artifact{Payload: []byte("desk2 output")})

	group := h.groups["my-group"]
	sess := h.sessions.Activate("my-group")
	defer h.sessions.Deactivate("my-group")

	ctx := context.Background()
	_, err := h.runGroupSequential(ctx, stableRunID, "my-group", group, &types.Artifact{Payload: []byte("input")}, sess)
	if err != nil {
		t.Fatalf("runGroupSequential: %v", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.tasks) != 1 {
		t.Fatalf("expected 1 dispatch (desk3 only), got %d: %v", len(d.tasks), func() []string {
			ids := make([]string, len(d.tasks))
			for i, t := range d.tasks {
				ids[i] = t.DeskID
			}
			return ids
		}())
	}
	if d.tasks[0].DeskID != "desk3" {
		t.Errorf("expected desk3, got %q", d.tasks[0].DeskID)
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
		nil, nil, nil,
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

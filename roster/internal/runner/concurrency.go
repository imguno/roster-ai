package runner

import (
	"context"
	"fmt"
	"sync"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

// ConcurrentRegistry wraps Registry and enforces per-desk concurrency policies.
type ConcurrentRegistry struct {
	inner *Registry
	mu    sync.Mutex
	desks map[string]*deskLimiter // key: deskID
}

func NewConcurrentRegistry(inner *Registry) *ConcurrentRegistry {
	return &ConcurrentRegistry{
		inner: inner,
		desks: make(map[string]*deskLimiter),
	}
}

func (r *ConcurrentRegistry) Register(t types.ExecutorType, exec sdk.Executor) {
	r.inner.Register(t, exec)
}

// ConfigureDesk sets the concurrency policy for a desk.
func (r *ConcurrentRegistry) ConfigureDesk(deskID string, cfg types.ConcurrencyConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.desks[deskID] = newDeskLimiter(cfg)
}

// Dispatch routes a task with concurrency enforcement.
func (r *ConcurrentRegistry) Dispatch(ctx context.Context, t types.ExecutorType, task sdk.Task) (*types.Artifact, error) {
	r.mu.Lock()
	limiter, ok := r.desks[task.DeskID]
	r.mu.Unlock()

	if !ok || limiter == nil {
		return r.inner.Dispatch(ctx, t, task)
	}
	return limiter.run(ctx, func() (*types.Artifact, error) {
		return r.inner.Dispatch(ctx, t, task)
	})
}

type deskLimiter struct {
	mode types.ConcurrencyMode
	sem  chan struct{} // buffered: capacity = max concurrent slots
}

func newDeskLimiter(cfg types.ConcurrencyConfig) *deskLimiter {
	mode := cfg.Mode
	if mode == "" {
		mode = types.ConcurrencyQueue
	}
	max := cfg.Max
	if max <= 0 {
		max = 1
	}

	var sem chan struct{}
	switch mode {
	case types.ConcurrencySpawn:
		sem = make(chan struct{}, max)
	default: // queue and reject both use a single slot
		sem = make(chan struct{}, 1)
		sem <- struct{}{} // pre-fill so first caller blocks immediately when busy
	}
	// For queue mode, channel starts empty; caller sends to acquire.
	// Actually: semaphore pattern — send to acquire, recv to release.
	// Reinitialise as empty so "send = acquire":
	sem = make(chan struct{}, max)

	return &deskLimiter{mode: mode, sem: sem}
}

func (l *deskLimiter) run(ctx context.Context, fn func() (*types.Artifact, error)) (*types.Artifact, error) {
	switch l.mode {
	case types.ConcurrencyReject:
		select {
		case l.sem <- struct{}{}:
		default:
			return nil, fmt.Errorf("desk is busy (concurrency: reject)")
		}
	case types.ConcurrencyQueue, types.ConcurrencySpawn:
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case l.sem <- struct{}{}:
		}
	}
	defer func() { <-l.sem }()
	return fn()
}

package runner

import (
	"context"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

// ConcurrentRegistry wraps Registry. Concurrency is managed by the hub queue,
// so this is now a thin pass-through kept for interface compatibility.
type ConcurrentRegistry struct {
	inner *Registry
}

func NewConcurrentRegistry(inner *Registry) *ConcurrentRegistry {
	return &ConcurrentRegistry{inner: inner}
}

func (r *ConcurrentRegistry) Register(t types.ExecutorType, exec sdk.Executor) {
	r.inner.Register(t, exec)
}

func (r *ConcurrentRegistry) Dispatch(ctx context.Context, t types.ExecutorType, task sdk.Task) (*types.Artifact, error) {
	return r.inner.Dispatch(ctx, t, task)
}

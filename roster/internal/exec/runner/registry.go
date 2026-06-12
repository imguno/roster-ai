package runner

import (
	"context"
	"fmt"
	"sync"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

// Registry holds all registered Executor implementations keyed by ExecutorType.
type Registry struct {
	mu        sync.RWMutex
	executors map[types.ExecutorType]sdk.Executor
}

func NewRegistry() *Registry {
	return &Registry{executors: make(map[types.ExecutorType]sdk.Executor)}
}

// Register associates an ExecutorType with an Executor implementation.
func (r *Registry) Register(t types.ExecutorType, exec sdk.Executor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[t] = exec
}

// Dispatch routes a task to the executor registered for the given type.
func (r *Registry) Dispatch(ctx context.Context, t types.ExecutorType, task sdk.Task) (*types.Output, error) {
	r.mu.RLock()
	exec, ok := r.executors[t]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no executor registered for type %q", t)
	}
	return exec.Run(ctx, task)
}

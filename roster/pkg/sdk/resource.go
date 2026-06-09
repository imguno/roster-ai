package sdk

import (
	"context"

	"github.com/roster-io/roster/pkg/types"
)

// ResourceWatcher is the interface for resource watch adapters.
// Community members implement this to connect external systems (GitHub, Figma, Slack, etc.)
// that emit events when something changes.
type ResourceWatcher interface {
	// Watch starts listening for changes and emits events on the returned channel.
	// Stops when ctx is cancelled.
	Watch(ctx context.Context) (<-chan types.Event, error)
}

// ResourceActor is the interface for resource action executors.
// Each action defined on a Resource can be backed by a ResourceActor.
type ResourceActor interface {
	// Execute runs the action with the given parameters and returns any output.
	Execute(ctx context.Context, params map[string]string) (*types.Artifact, error)
}

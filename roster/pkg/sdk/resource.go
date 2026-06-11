package sdk

import (
	"context"

	"github.com/roster-io/roster/pkg/types"
)

// ResourceWatcher is the interface for resource watch adapters.
// Implement this to connect external systems (GitHub, Figma, Slack, etc.)
// that emit events when something changes.
type ResourceWatcher interface {
	Watch(ctx context.Context) (<-chan types.Event, error)
}

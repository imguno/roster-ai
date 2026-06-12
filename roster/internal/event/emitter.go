package event

import "context"

// DeskEmitter is a scoped handle that lets a desk emit typed events
// without knowing about the full bus or its own scope ID.
type DeskEmitter struct {
	bus     *Bus
	scopeID string
}

// NewDeskEmitter creates a DeskEmitter bound to a specific desk/scope.
func NewDeskEmitter(bus *Bus, scopeID string) *DeskEmitter {
	return &DeskEmitter{bus: bus, scopeID: scopeID}
}

// Emit publishes a typed result event on behalf of the desk.
func (e *DeskEmitter) Emit(ctx context.Context, eventType string, payload any) error {
	return e.bus.Emit(ctx, e.scopeID, eventType, payload)
}

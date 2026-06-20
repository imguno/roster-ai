package hub

import (
	"fmt"
	"sync"
	"time"

	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/pkg/types"
)

// CircuitBreaker prevents runaway event loops by tracking per-event-type
// emission counts and applying cooldowns when limits are exceeded.
type CircuitBreaker struct {
	mu       sync.Mutex
	counts   map[string]int
	cooldown map[string]time.Time
	recorder *observe.Recorder
}

func newCircuitBreaker(recorder *observe.Recorder) *CircuitBreaker {
	return &CircuitBreaker{
		counts:   make(map[string]int),
		cooldown: make(map[string]time.Time),
		recorder: recorder,
	}
}

// Tripped returns true if the event type should be suppressed.
func (cb *CircuitBreaker) Tripped(eventType string, limits types.LoopLimits) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if limits.MaxIterations <= 0 {
		return false
	}

	// Check cooldown.
	if until, ok := cb.cooldown[eventType]; ok {
		if time.Now().Before(until) {
			return true
		}
		delete(cb.cooldown, eventType)
		cb.counts[eventType] = 0
	}

	cb.counts[eventType]++
	if cb.counts[eventType] <= limits.MaxIterations {
		return false
	}

	// Tripped — record and start cooldown.
	cb.recorder.Record(observe.Event{
		Type:  observe.EventLoopBreaker,
		Error: fmt.Sprintf("event %q exceeded max_iterations (%d); suppressed", eventType, limits.MaxIterations),
	})

	if limits.Cooldown != "" {
		if d, err := time.ParseDuration(limits.Cooldown); err == nil {
			cb.cooldown[eventType] = time.Now().Add(d)
		}
	}
	return true
}

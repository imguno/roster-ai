package event

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/roster-io/roster/pkg/types"
)

func TestBusPublishSubscribe(t *testing.T) {
	bus := NewBus(100)
	var received []types.Event

	bus.Subscribe("desk-a", []string{"task.created"}, func(ctx context.Context, ev types.Event) error {
		received = append(received, ev)
		return nil
	})

	bus.Publish(context.Background(), types.Event{Type: "task.created", Source: "user"})
	bus.Publish(context.Background(), types.Event{Type: "plan.ready", Source: "desk-b"})

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != "task.created" {
		t.Fatalf("expected task.created, got %s", received[0].Type)
	}
}

func TestBusWildcard(t *testing.T) {
	bus := NewBus(100)
	var count int

	bus.Subscribe("observer", nil, func(ctx context.Context, ev types.Event) error {
		count++
		return nil
	})

	bus.Publish(context.Background(), types.Event{Type: "a"})
	bus.Publish(context.Background(), types.Event{Type: "b"})

	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}
}

func TestBusUnsubscribe(t *testing.T) {
	bus := NewBus(100)
	var count int

	bus.Subscribe("desk-a", nil, func(ctx context.Context, ev types.Event) error {
		count++
		return nil
	})
	bus.Unsubscribe("desk-a")
	bus.Publish(context.Background(), types.Event{Type: "x"})

	if count != 0 {
		t.Fatalf("expected 0 after unsubscribe, got %d", count)
	}
}

func TestBusPublishAsync(t *testing.T) {
	bus := NewBus(100)
	var count atomic.Int32

	bus.Subscribe("desk-a", []string{"ping"}, func(ctx context.Context, ev types.Event) error {
		count.Add(1)
		return nil
	})

	bus.PublishAsync(context.Background(), types.Event{Type: "ping"})
	// Give async goroutine time to run.
	for i := 0; i < 100; i++ {
		if count.Load() > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if count.Load() != 1 {
		t.Fatalf("expected 1, got %d", count.Load())
	}
}

func TestBusEmit(t *testing.T) {
	bus := NewBus(100)
	var received []types.Event

	bus.Subscribe("listener", []string{"result.text"}, func(ctx context.Context, ev types.Event) error {
		received = append(received, ev)
		return nil
	})

	payload := map[string]string{"text": "hello"}
	if err := bus.Emit(context.Background(), "desk-a", "result.text", payload); err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	// Give async goroutine time to deliver.
	for i := 0; i < 100; i++ {
		if len(received) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != "result.text" {
		t.Fatalf("expected type result.text, got %s", received[0].Type)
	}
	if received[0].Source != "desk-a" {
		t.Fatalf("expected source desk-a, got %s", received[0].Source)
	}
	if string(received[0].Payload) != `{"text":"hello"}` {
		t.Fatalf("unexpected payload: %s", string(received[0].Payload))
	}
}

func TestBusHistory(t *testing.T) {
	bus := NewBus(3)
	bus.Publish(context.Background(), types.Event{Type: "a"})
	bus.Publish(context.Background(), types.Event{Type: "b"})
	bus.Publish(context.Background(), types.Event{Type: "c"})
	bus.Publish(context.Background(), types.Event{Type: "d"})

	history := bus.History()
	if len(history) != 3 {
		t.Fatalf("expected 3 (maxHistory), got %d", len(history))
	}
	if history[0].Type != "b" {
		t.Fatalf("expected oldest to be 'b', got %s", history[0].Type)
	}
}

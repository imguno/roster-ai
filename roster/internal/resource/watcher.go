package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/roster-io/roster/pkg/types"
)

// Watcher monitors a resource and emits events when changes are detected.
type Watcher struct {
	resource *types.Resource
	events   chan types.Event
}

// NewWatcher creates a watcher for the given resource.
func NewWatcher(res *types.Resource) *Watcher {
	return &Watcher{
		resource: res,
		events:   make(chan types.Event, 100),
	}
}

// Events returns the channel of emitted events.
func (w *Watcher) Events() <-chan types.Event {
	return w.events
}

// Start begins watching. Blocks until ctx is cancelled.
// If the resource config has a "path" key, uses filesystem watching (fsnotify).
func (w *Watcher) Start(ctx context.Context) error {
	if watchPath := w.resource.Config["path"]; watchPath != "" {
		return w.startFileWatch(ctx, watchPath)
	}
	<-ctx.Done()
	return ctx.Err()
}

// startFileWatch uses fsnotify to watch a directory for file changes.
func (w *Watcher) startFileWatch(ctx context.Context, watchPath string) error {
	defer close(w.events)

	// Ensure directory exists.
	if err := os.MkdirAll(watchPath, 0750); err != nil {
		return fmt.Errorf("resource watch: mkdir %s: %w", watchPath, err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("resource watch: fsnotify: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(watchPath); err != nil {
		return fmt.Errorf("resource watch: add %s: %w", watchPath, err)
	}

	// Debounce: batch rapid writes into a single event.
	var debounce *time.Timer
	var lastFile string

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			lastFile = event.Name
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(500*time.Millisecond, func() {
				w.emitFileEvent(lastFile)
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			_ = err // log if needed
		}
	}
}

// emitFileEvent reads the changed file and emits an event.
func (w *Watcher) emitFileEvent(filePath string) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return
	}

	// Event type: "{resourceID}.{filename-without-ext}"
	// e.g. "task-board.dev-team" when dev-team.md is written.
	base := filepath.Base(filePath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	eventType := w.resource.ID + "." + name

	w.events <- types.Event{
		Type:    eventType,
		Source:  w.resource.ID,
		Payload: content,
	}
}



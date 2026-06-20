package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	resourcedomain "github.com/roster-io/roster/domain/resource"
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
// Dispatches to the appropriate watch strategy based on resource type.
func (w *Watcher) Start(ctx context.Context) error {
	watchType := resourcedomain.ClassifyWatch(w.resource.Type)
	switch watchType {
	case resourcedomain.WatchLocal:
		if watchPath := w.resource.Config["path"]; watchPath != "" {
			return w.startFileWatch(ctx, watchPath)
		}
	case resourcedomain.WatchSDK, resourcedomain.WatchMCP:
		// SDK/MCP watching will be implemented via gRPC streaming.
		// For now, fall through to wait on context.
	}
	<-ctx.Done()
	return ctx.Err()
}

// startFileWatch uses fsnotify to watch a directory tree for file changes.
// It recursively adds all subdirectories and watches for new directories.
// If the resource has watch patterns (e.g. "**/*.go"), only matching files
// trigger events.
func (w *Watcher) startFileWatch(ctx context.Context, watchPath string) error {
	defer close(w.events)

	if err := os.MkdirAll(watchPath, 0750); err != nil {
		return fmt.Errorf("resource watch: mkdir %s: %w", watchPath, err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("resource watch: fsnotify: %w", err)
	}
	defer watcher.Close()

	// Recursively add all directories under watchPath.
	if err := addDirRecursive(watcher, watchPath); err != nil {
		return err
	}

	// Debounce: batch rapid writes into a single event.
	var debounce *time.Timer

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// Watch for new directories to add them dynamically.
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = addDirRecursive(watcher, event.Name)
					continue
				}
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			if !w.matchesWatch(event.Name) {
				continue
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(500*time.Millisecond, func() {
				w.emitChange()
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			_ = err
		}
	}
}

// addDirRecursive adds dir and all its subdirectories to the watcher.
func addDirRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}
		if !d.IsDir() {
			return nil
		}
		// Skip hidden directories (e.g. .git, .venv, node_modules).
		name := d.Name()
		if name != "." && (name[0] == '.' || name == "node_modules" || name == "__pycache__") {
			return filepath.SkipDir
		}
		return watcher.Add(path)
	})
}

// matchesWatch checks if a file path matches the resource's watch patterns.
// If no patterns are configured, all files match.
func (w *Watcher) matchesWatch(filePath string) bool {
	if len(w.resource.Watch) == 0 {
		return true
	}
	base := filepath.Base(filePath)
	for _, pattern := range w.resource.Watch {
		// Strip leading "**/" prefix to get a base-name pattern
		// (e.g. "**/*.go" → "*.go").
		p := pattern
		for len(p) >= 3 && p[0] == '*' && p[1] == '*' && p[2] == '/' {
			p = p[3:]
		}
		if p == "" || p == "**" {
			return true
		}
		if matched, _ := filepath.Match(p, base); matched {
			return true
		}
	}
	return false
}

// emitChange emits a resource change event. Events are triggers only — no payload.
func (w *Watcher) emitChange() {
	w.events <- types.Event{
		Type:   resourcedomain.EventType(w.resource.ID),
		Source: w.resource.ID,
	}
}

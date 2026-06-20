package resource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/roster-io/roster/pkg/types"
)

func TestMatchesWatch(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		file     string
		want     bool
	}{
		{"no patterns matches all", nil, "/a/b/foo.txt", true},
		{"empty patterns matches all", []string{}, "/a/b/foo.txt", true},
		{"glob star go", []string{"**/*.go"}, "/a/b/main.go", true},
		{"glob star go no match", []string{"**/*.go"}, "/a/b/main.py", false},
		{"simple glob", []string{"*.md"}, "/a/ideas/001.md", true},
		{"simple glob no match", []string{"*.md"}, "/a/ideas/001.txt", false},
		{"multiple patterns", []string{"*.go", "*.md"}, "/a/README.md", true},
		{"multiple patterns no match", []string{"*.go", "*.md"}, "/a/foo.js", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Watcher{resource: &types.Resource{Watch: tt.patterns}}
			if got := w.matchesWatch(tt.file); got != tt.want {
				t.Errorf("matchesWatch(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestWatcherEmitsOnFileChange(t *testing.T) {
	dir := t.TempDir()

	res := &types.Resource{
		ID:    "test-res",
		Watch: []string{"*.txt"},
		Config: map[string]string{
			"path": dir,
		},
		Type: "local",
	}

	w := NewWatcher(res)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- w.Start(ctx) }()

	// Give the watcher time to initialize.
	time.Sleep(100 * time.Millisecond)

	// Write a matching file.
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0640); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events():
		if ev.Type != "resource.test-res.changed" {
			t.Fatalf("unexpected event type: %s", ev.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watcher event")
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("watcher error: %v", err)
	}
}

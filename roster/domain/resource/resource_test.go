package resource

import "testing"

func TestEventType(t *testing.T) {
	if got := EventType("codebase"); got != "resource.codebase.changed" {
		t.Errorf("expected resource.codebase.changed, got %s", got)
	}
}

func TestNeedsWatch(t *testing.T) {
	if !NeedsWatch([]string{"*.go"}, "") {
		t.Error("should need watch with patterns")
	}
	if !NeedsWatch(nil, "/some/path") {
		t.Error("should need watch with path")
	}
	if NeedsWatch(nil, "") {
		t.Error("should not need watch without patterns or path")
	}
}

func TestClassifyWatch(t *testing.T) {
	tests := []struct {
		typ  string
		want WatchType
	}{
		{"local", WatchLocal},
		{"", WatchLocal},
		{"mcp", WatchMCP},
		{"sdk", WatchSDK},
		{"unknown", WatchLocal},
	}
	for _, tt := range tests {
		if got := ClassifyWatch(tt.typ); got != tt.want {
			t.Errorf("ClassifyWatch(%q) = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

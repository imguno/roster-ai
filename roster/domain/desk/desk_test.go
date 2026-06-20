package desk

import (
	"testing"
)

func TestResolveEmissions_NoAllowlist(t *testing.T) {
	valid, rejected := ResolveEmissions([]string{"done", "failed"}, nil)
	if len(valid) != 2 || len(rejected) != 0 {
		t.Errorf("expected all valid, got valid=%v rejected=%v", valid, rejected)
	}
}

func TestResolveEmissions_DefaultDone(t *testing.T) {
	valid, _ := ResolveEmissions(nil, nil)
	if len(valid) != 1 || valid[0] != "done" {
		t.Errorf("expected [done], got %v", valid)
	}
}

func TestResolveEmissions_Filtering(t *testing.T) {
	valid, rejected := ResolveEmissions([]string{"done", "deploy", "notify"}, []string{"done", "deploy"})
	if len(valid) != 2 {
		t.Errorf("expected 2 valid, got %v", valid)
	}
	if len(rejected) != 1 || rejected[0] != "notify" {
		t.Errorf("expected [notify] rejected, got %v", rejected)
	}
}

func TestResolveEmissions_AllRejected(t *testing.T) {
	valid, rejected := ResolveEmissions([]string{"unknown"}, []string{"done"})
	if len(valid) != 1 || valid[0] != "done" {
		t.Errorf("expected fallback [done], got %v", valid)
	}
	if len(rejected) != 1 {
		t.Errorf("expected 1 rejected, got %v", rejected)
	}
}

func TestEventNames(t *testing.T) {
	names := EventNames("reviewer", []string{"dev-team", "qa"}, "done")
	expected := []string{"reviewer.done", "dev-team.reviewer.done", "qa.reviewer.done"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d: %v", len(expected), len(names), names)
	}
	for i, n := range names {
		if n != expected[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, expected[i])
		}
	}
}

func TestEventNames_NoGroups(t *testing.T) {
	names := EventNames("worker", nil, "done")
	if len(names) != 1 || names[0] != "worker.done" {
		t.Errorf("expected [worker.done], got %v", names)
	}
}

func TestIsScript(t *testing.T) {
	if !IsScript(map[string]string{"command": "echo hello"}) {
		t.Error("expected true for command param")
	}
	if IsScript(map[string]string{"model": "claude"}) {
		t.Error("expected false without command param")
	}
	if IsScript(nil) {
		t.Error("expected false for nil params")
	}
}

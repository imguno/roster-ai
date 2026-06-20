package session

import "testing"

func TestBuildContext(t *testing.T) {
	ctx := BuildContext("backend-dev.done", []string{"go-coding", "roster-internals"}, []string{"codebase"})

	if ctx.Message != "triggered by backend-dev.done" {
		t.Errorf("unexpected message: %s", ctx.Message)
	}
	if ctx.Meta["trigger"] != "backend-dev.done" {
		t.Errorf("unexpected trigger: %v", ctx.Meta["trigger"])
	}
	skills := ctx.Meta["skills"].([]string)
	if len(skills) != 2 || skills[0] != "go-coding" {
		t.Errorf("unexpected skills: %v", skills)
	}
}

func TestBuildContext_Empty(t *testing.T) {
	ctx := BuildContext("hub.started", nil, nil)
	if _, ok := ctx.Meta["skills"]; ok {
		t.Error("expected no skills key")
	}
	if _, ok := ctx.Meta["resources"]; ok {
		t.Error("expected no resources key")
	}
}

func TestScopes(t *testing.T) {
	s := Scopes("reviewer", []string{"qa-team", "dev-team"})
	if len(s) != 3 || s[0] != "reviewer" || s[1] != "qa-team" || s[2] != "dev-team" {
		t.Errorf("unexpected scopes: %v", s)
	}
}

func TestScopes_NoGroups(t *testing.T) {
	s := Scopes("worker", nil)
	if len(s) != 1 || s[0] != "worker" {
		t.Errorf("unexpected scopes: %v", s)
	}
}

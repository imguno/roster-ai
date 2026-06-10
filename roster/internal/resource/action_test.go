package resource

import (
	"context"
	"testing"

	"github.com/roster-io/roster/pkg/types"
)

func TestCheckPermission_Open(t *testing.T) {
	r := NewRegistry(
		map[string]*types.Resource{
			"repo": {
				ID:      "repo",
				Actions: map[string]*types.ResourceAction{"commit": {Exec: "echo ok"}},
				// No permissions = open to all
			},
		}, nil, nil,
	)
	if err := r.CheckPermission("repo", "commit", "any-desk"); err != nil {
		t.Errorf("expected open access, got: %v", err)
	}
}

func TestCheckPermission_ByDesk(t *testing.T) {
	r := NewRegistry(
		map[string]*types.Resource{
			"repo": {
				ID: "repo",
				Actions: map[string]*types.ResourceAction{
					"commit": {Exec: "echo ok"},
					"deploy": {Exec: "echo deploy"},
				},
				Permissions: []types.PermissionRule{
					{Allow: []string{"commit"}, Desks: []string{"backend-a", "backend-b"}},
					{Allow: []string{"deploy"}, Desks: []string{"ops"}},
				},
			},
		},
		map[string]*types.Desk{
			"backend-a": {ID: "backend-a"},
			"backend-b": {ID: "backend-b"},
			"ops":       {ID: "ops"},
			"frontend":  {ID: "frontend"},
		}, nil,
	)

	// backend-a can commit
	if err := r.CheckPermission("repo", "commit", "backend-a"); err != nil {
		t.Errorf("backend-a should commit: %v", err)
	}
	// backend-a cannot deploy
	if err := r.CheckPermission("repo", "deploy", "backend-a"); err == nil {
		t.Error("backend-a should NOT deploy")
	}
	// ops can deploy
	if err := r.CheckPermission("repo", "deploy", "ops"); err != nil {
		t.Errorf("ops should deploy: %v", err)
	}
	// frontend denied
	if err := r.CheckPermission("repo", "commit", "frontend"); err == nil {
		t.Error("frontend should be denied")
	}
}

func TestCheckPermission_ByGroup(t *testing.T) {
	r := NewRegistry(
		map[string]*types.Resource{
			"repo": {
				ID:      "repo",
				Actions: map[string]*types.ResourceAction{"read": {Exec: "echo ok"}},
				Permissions: []types.PermissionRule{
					{Allow: []string{"read"}, Groups: []string{"dev-team"}},
				},
			},
		},
		map[string]*types.Desk{
			"backend-a": {ID: "backend-a", Parent: "dev-team"},
			"designer":  {ID: "designer"},
		},
		map[string]*types.Group{
			"dev-team": {ID: "dev-team"},
		},
	)

	// backend-a is in dev-team → allowed
	if err := r.CheckPermission("repo", "read", "backend-a"); err != nil {
		t.Errorf("backend-a (dev-team member) should read: %v", err)
	}
	// designer is not in dev-team → denied
	if err := r.CheckPermission("repo", "read", "designer"); err == nil {
		t.Error("designer should be denied")
	}
}

func TestCheckPermission_ByGroupLead(t *testing.T) {
	r := NewRegistry(
		map[string]*types.Resource{
			"repo": {
				ID:      "repo",
				Actions: map[string]*types.ResourceAction{"review": {Exec: "echo ok"}},
				Permissions: []types.PermissionRule{
					{Allow: []string{"review"}, Groups: []string{"dev-team"}},
				},
			},
		},
		map[string]*types.Desk{
			"lead":   {ID: "lead"},
			"worker": {ID: "worker", Parent: "dev-team"},
		},
		map[string]*types.Group{
			"dev-team": {ID: "dev-team", Lead: &types.GroupLead{Desk: "lead"}},
		},
	)

	// lead is group lead → allowed
	if err := r.CheckPermission("repo", "review", "lead"); err != nil {
		t.Errorf("lead (group lead) should review: %v", err)
	}
}

func TestCheckPermission_ByTag(t *testing.T) {
	r := NewRegistry(
		map[string]*types.Resource{
			"repo": {
				ID: "repo",
				Actions: map[string]*types.ResourceAction{
					"read":   {Exec: "echo ok"},
					"deploy": {Exec: "echo deploy"},
				},
				Permissions: []types.PermissionRule{
					{Allow: []string{"read"}, Tags: []string{"viewer"}},
					{Allow: []string{"deploy"}, Tags: []string{"senior"}},
				},
			},
		},
		map[string]*types.Desk{
			"intern":    {ID: "intern", Tags: []string{"viewer"}},
			"tech-lead": {ID: "tech-lead", Tags: []string{"senior", "viewer"}},
			"outsider":  {ID: "outsider"},
		}, nil,
	)

	// intern has "viewer" tag → can read
	if err := r.CheckPermission("repo", "read", "intern"); err != nil {
		t.Errorf("intern (viewer) should read: %v", err)
	}
	// intern cannot deploy
	if err := r.CheckPermission("repo", "deploy", "intern"); err == nil {
		t.Error("intern should NOT deploy")
	}
	// tech-lead has "senior" tag → can deploy
	if err := r.CheckPermission("repo", "deploy", "tech-lead"); err != nil {
		t.Errorf("tech-lead (senior) should deploy: %v", err)
	}
	// tech-lead also has "viewer" → can read
	if err := r.CheckPermission("repo", "read", "tech-lead"); err != nil {
		t.Errorf("tech-lead (viewer) should read: %v", err)
	}
	// outsider has no tags → denied
	if err := r.CheckPermission("repo", "read", "outsider"); err == nil {
		t.Error("outsider should be denied")
	}
}

func TestCheckPermission_Wildcard(t *testing.T) {
	r := NewRegistry(
		map[string]*types.Resource{
			"repo": {
				ID: "repo",
				Actions: map[string]*types.ResourceAction{
					"read":   {Exec: "echo ok"},
					"commit": {Exec: "echo ok"},
					"deploy": {Exec: "echo ok"},
				},
				Permissions: []types.PermissionRule{
					{Allow: []string{"*"}, Desks: []string{"admin"}},
				},
			},
		},
		map[string]*types.Desk{"admin": {ID: "admin"}}, nil,
	)

	for _, action := range []string{"read", "commit", "deploy"} {
		if err := r.CheckPermission("repo", action, "admin"); err != nil {
			t.Errorf("admin should have wildcard access to %q: %v", action, err)
		}
	}
}

func TestAvailableActions(t *testing.T) {
	r := NewRegistry(
		map[string]*types.Resource{
			"repo": {
				ID: "repo",
				Actions: map[string]*types.ResourceAction{
					"commit": {Exec: "echo ok"},
					"deploy": {Exec: "echo ok"},
					"read":   {Exec: "echo ok"},
				},
				Permissions: []types.PermissionRule{
					{Allow: []string{"commit", "read"}, Tags: []string{"dev"}},
				},
			},
		},
		map[string]*types.Desk{"worker": {ID: "worker", Tags: []string{"dev"}}}, nil,
	)

	actions := r.AvailableActions("repo", "worker")
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d: %v", len(actions), actions)
	}
}

func TestExecuteAction(t *testing.T) {
	r := NewRegistry(
		map[string]*types.Resource{
			"repo": {
				ID:      "repo",
				Actions: map[string]*types.ResourceAction{"greet": {Exec: "echo hello"}},
			},
		}, nil, nil,
	)

	out, err := r.ExecuteAction(context.Background(), "repo", "greet", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", out)
	}
}

package resource

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/roster-io/roster/pkg/types"
)

// Registry holds all loaded resources and provides action execution + permission checks.
// It needs desk and group maps to resolve group membership and tags.
type Registry struct {
	resources map[string]*types.Resource
	desks     map[string]*types.Desk
	groups    map[string]*types.Group
}

// NewRegistry creates a resource registry with desk/group context for permission resolution.
func NewRegistry(resources map[string]*types.Resource, desks map[string]*types.Desk, groups map[string]*types.Group) *Registry {
	if resources == nil {
		resources = make(map[string]*types.Resource)
	}
	if desks == nil {
		desks = make(map[string]*types.Desk)
	}
	if groups == nil {
		groups = make(map[string]*types.Group)
	}
	return &Registry{resources: resources, desks: desks, groups: groups}
}

// Get returns a resource by ID.
func (r *Registry) Get(id string) (*types.Resource, bool) {
	res, ok := r.resources[id]
	return res, ok
}

// All returns all resources.
func (r *Registry) All() map[string]*types.Resource {
	return r.resources
}

// CheckPermission verifies that the given desk is allowed to invoke the action.
// Permission rules are matched by: desk ID, group membership, or desk tags.
// Returns nil if allowed, error if denied.
func (r *Registry) CheckPermission(resourceID, actionName, deskID string) error {
	res, ok := r.resources[resourceID]
	if !ok {
		return fmt.Errorf("resource %q not found", resourceID)
	}
	if _, ok := res.Actions[actionName]; !ok {
		return fmt.Errorf("resource %q has no action %q", resourceID, actionName)
	}

	// No permissions declared = open to all.
	if len(res.Permissions) == 0 {
		return nil
	}

	desk := r.desks[deskID]

	for _, rule := range res.Permissions {
		if !r.ruleMatchesDesk(rule, deskID, desk) {
			continue
		}
		if ruleAllowsAction(rule, actionName) {
			return nil
		}
	}

	return fmt.Errorf("resource %q: desk %q not allowed to invoke %q", resourceID, deskID, actionName)
}

// ruleMatchesDesk checks if a permission rule matches the given desk.
func (r *Registry) ruleMatchesDesk(rule types.PermissionRule, deskID string, desk *types.Desk) bool {
	// Match by desk ID.
	for _, id := range rule.Desks {
		if id == deskID {
			return true
		}
	}

	// Match by group membership.
	for _, groupID := range rule.Groups {
		group, ok := r.groups[groupID]
		if !ok {
			continue
		}
		for _, memberID := range group.Desks {
			if memberID == deskID {
				return true
			}
		}
		if group.Lead != nil && group.Lead.Desk == deskID {
			return true
		}
	}

	// Match by tag.
	if desk != nil && len(rule.Tags) > 0 {
		for _, ruleTag := range rule.Tags {
			for _, deskTag := range desk.Tags {
				if ruleTag == deskTag {
					return true
				}
			}
		}
	}

	return false
}

// ruleAllowsAction checks if a permission rule allows the given action.
func ruleAllowsAction(rule types.PermissionRule, actionName string) bool {
	for _, a := range rule.Allow {
		if a == actionName || a == "*" {
			return true
		}
	}
	return false
}

// ExecuteAction runs a resource action.
func (r *Registry) ExecuteAction(ctx context.Context, resourceID, actionName string, params map[string]string) (string, error) {
	res, ok := r.resources[resourceID]
	if !ok {
		return "", fmt.Errorf("resource %q not found", resourceID)
	}
	action, ok := res.Actions[actionName]
	if !ok {
		return "", fmt.Errorf("resource %q has no action %q", resourceID, actionName)
	}

	if action.Exec == "" {
		return "", fmt.Errorf("resource %q action %q has no exec command", resourceID, actionName)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", action.Exec)

	// Inject resource config as ROSTER_RESOURCE_* env vars.
	for k, v := range res.Config {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ROSTER_RESOURCE_%s=%s", strings.ToUpper(k), v))
	}
	// Inject action params as ROSTER_PARAM_* env vars.
	for k, v := range action.Params {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ROSTER_PARAM_%s=%s", strings.ToUpper(k), v))
	}
	// Inject caller params.
	for k, v := range params {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ROSTER_PARAM_%s=%s", strings.ToUpper(k), v))
	}

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resource %q action %q: %w", resourceID, actionName, err)
	}
	return string(out), nil
}

// AvailableActions returns the action names a desk is allowed to invoke on a resource.
func (r *Registry) AvailableActions(resourceID, deskID string) []string {
	res, ok := r.resources[resourceID]
	if !ok {
		return nil
	}

	var actions []string
	for name := range res.Actions {
		if r.CheckPermission(resourceID, name, deskID) == nil {
			actions = append(actions, name)
		}
	}
	return actions
}

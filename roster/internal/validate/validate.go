package validate

import (
	"fmt"
	"strings"

	"github.com/roster-io/roster/internal/config"
	"github.com/roster-io/roster/pkg/types"
)

// Project validates referential integrity across all loaded config.
func Project(p *config.Project) error {
	var errs []string
	for _, desk := range p.Desks {
		errs = append(errs, checkDesk(desk, p)...)
	}
	for _, group := range p.Groups {
		errs = append(errs, checkGroup(group, p)...)
	}
	if p.Organization != nil {
		errs = append(errs, checkOrganization(p.Organization, p)...)
	}
	for _, policy := range p.Policies {
		errs = append(errs, checkPolicy(policy)...)
	}
	if len(errs) > 0 {
		return fmt.Errorf("validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func checkDesk(d *types.Desk, p *config.Project) []string {
	var errs []string
	needsAgent := d.Executor.Type == types.ExecutorTypeAPI || d.Executor.Type == types.ExecutorTypeSDK
	if d.Agent.IsLocal() {
		if _, ok := p.Agents[d.Agent.ID]; !ok {
			errs = append(errs, fmt.Sprintf("desk %q: agent %q not found", d.ID, d.Agent.ID))
		}
	}
	if needsAgent && !d.Agent.IsLocal() && !d.Agent.IsRemote() {
		errs = append(errs, fmt.Sprintf("desk %q: agent is required for executor type %q", d.ID, d.Executor.Type))
	}
	if d.Executor.Type == "" {
		errs = append(errs, fmt.Sprintf("desk %q: executor.type is required", d.ID))
	}
	if d.Executor.Type == types.ExecutorTypeAPI && d.Executor.SDK == "" {
		errs = append(errs, fmt.Sprintf("desk %q: executor.sdk is required when type is api", d.ID))
	}
	if d.Policy != "" {
		if _, ok := p.Policies[d.Policy]; !ok {
			errs = append(errs, fmt.Sprintf("desk %q: policy %q not found", d.ID, d.Policy))
		}
	}
	// Validate parent reference.
	if d.Parent != "" {
		_, isGroup := p.Groups[d.Parent]
		if !isGroup && (p.Organization == nil || p.Organization.ID != d.Parent) {
			errs = append(errs, fmt.Sprintf("desk %q: parent %q not found (must be a group or org id)", d.ID, d.Parent))
		}
	}
	if d.Concurrency.Mode != "" {
		switch d.Concurrency.Mode {
		case types.ConcurrencyQueue, types.ConcurrencySpawn, types.ConcurrencyReject:
		default:
			errs = append(errs, fmt.Sprintf("desk %q: invalid concurrency mode %q", d.ID, d.Concurrency.Mode))
		}
	}
	if d.Executor.Type == types.ExecutorTypeAPI && d.Executor.SDK != "" {
		switch d.Executor.SDK {
		case types.SDKAnthropic, types.SDKOpenAI, types.SDKGemini:
		default:
			errs = append(errs, fmt.Sprintf("desk %q: invalid executor.sdk %q", d.ID, d.Executor.SDK))
		}
	}
	for i, trig := range d.Triggers {
		if trig.Type != "exec" && trig.Type != "poll" {
			errs = append(errs, fmt.Sprintf("desk %q: trigger[%d] invalid type %q", d.ID, i, trig.Type))
		}
	}
	for _, resID := range d.Resources {
		if _, ok := p.Resources[resID]; !ok {
			errs = append(errs, fmt.Sprintf("desk %q: resource %q not found", d.ID, resID))
		}
	}
	return errs
}

func checkGroup(g *types.Group, p *config.Project) []string {
	var errs []string
	// Validate parent reference.
	if g.Parent != "" {
		_, isGroup := p.Groups[g.Parent]
		if !isGroup && (p.Organization == nil || p.Organization.ID != g.Parent) {
			errs = append(errs, fmt.Sprintf("group %q: parent %q not found", g.ID, g.Parent))
		}
	}
	if g.Lead != nil {
		if _, ok := p.Desks[g.Lead.Desk]; !ok {
			errs = append(errs, fmt.Sprintf("group %q: lead desk %q not found", g.ID, g.Lead.Desk))
		}
	}
	for _, resID := range g.Resources {
		if _, ok := p.Resources[resID]; !ok {
			errs = append(errs, fmt.Sprintf("group %q: resource %q not found", g.ID, resID))
		}
	}
	if g.Policy != "" {
		if _, ok := p.Policies[g.Policy]; !ok {
			errs = append(errs, fmt.Sprintf("group %q: policy %q not found", g.ID, g.Policy))
		}
	}
	if g.Dispatch != "" && g.Dispatch != "sequential" && g.Dispatch != "parallel" && g.Dispatch != "conversation" {
		errs = append(errs, fmt.Sprintf("group %q: invalid dispatch mode %q", g.ID, g.Dispatch))
	}
	if g.Lead != nil && g.Lead.Position != "" {
		switch g.Lead.Position {
		case "both", "first", "last":
		default:
			errs = append(errs, fmt.Sprintf("group %q: invalid lead position %q", g.ID, g.Lead.Position))
		}
	}
	for i, trig := range g.Triggers {
		if trig.Type != "exec" && trig.Type != "poll" {
			errs = append(errs, fmt.Sprintf("group %q: trigger[%d] invalid type %q", g.ID, i, trig.Type))
		}
	}
	return errs
}

func checkPolicy(pol *types.Policy) []string {
	var errs []string
	if pol.OnTimeout != "" {
		switch pol.OnTimeout {
		case "fail", "retry", "escalate":
		default:
			errs = append(errs, fmt.Sprintf("policy %q: invalid on_timeout %q", pol.ID, pol.OnTimeout))
		}
	}
	if pol.OnError != "" {
		switch pol.OnError {
		case "fail", "retry", "escalate":
		default:
			errs = append(errs, fmt.Sprintf("policy %q: invalid on_error %q", pol.ID, pol.OnError))
		}
	}
	if pol.EscalateTo != "" && pol.OnError != "escalate" && pol.OnTimeout != "escalate" {
		errs = append(errs, fmt.Sprintf("policy %q: escalate_to set but neither on_error nor on_timeout is 'escalate'", pol.ID))
	}
	return errs
}

func checkOrganization(org *types.Organization, p *config.Project) []string {
	var errs []string
	for _, resID := range org.Resources {
		if _, ok := p.Resources[resID]; !ok {
			errs = append(errs, fmt.Sprintf("org %q: resource %q not found", org.ID, resID))
		}
	}
	return errs
}

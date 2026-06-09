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
	// Agent is required for API desks. Exec/docker/remote can run without an agent (pure script).
	// Human desks never need an agent.
	needsAgent := d.Executor.Type == types.ExecutorTypeAPI
	if d.Agent != "" {
		// If an agent is specified, it must exist regardless of executor type.
		if _, ok := p.Agents[d.Agent]; !ok {
			errs = append(errs, fmt.Sprintf("desk %q: agent %q not found", d.ID, d.Agent))
		}
	}
	if needsAgent && d.Agent == "" {
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
	// Validate concurrency mode.
	if d.Concurrency.Mode != "" {
		switch d.Concurrency.Mode {
		case types.ConcurrencyQueue, types.ConcurrencySpawn, types.ConcurrencyReject:
		default:
			errs = append(errs, fmt.Sprintf("desk %q: invalid concurrency mode %q (must be queue, spawn, or reject)", d.ID, d.Concurrency.Mode))
		}
	}
	// Validate executor SDK when type is API.
	if d.Executor.Type == types.ExecutorTypeAPI && d.Executor.SDK != "" {
		switch d.Executor.SDK {
		case types.SDKAnthropic, types.SDKOpenAI, types.SDKGemini:
		default:
			errs = append(errs, fmt.Sprintf("desk %q: invalid executor.sdk %q (must be anthropic, openai, or gemini)", d.ID, d.Executor.SDK))
		}
	}
	// Validate trigger types.
	for i, trig := range d.Triggers {
		if trig.Type != "exec" && trig.Type != "poll" {
			errs = append(errs, fmt.Sprintf("desk %q: trigger[%d] invalid type %q (must be exec or poll)", d.ID, i, trig.Type))
		}
	}
	// Validate desk resources exist.
	for _, resID := range d.Resources {
		if _, ok := p.Resources[resID]; !ok {
			errs = append(errs, fmt.Sprintf("desk %q: resource %q not found", d.ID, resID))
		}
	}
	return errs
}

func checkGroup(g *types.Group, p *config.Project) []string {
	var errs []string
	for _, deskID := range g.Desks {
		if _, ok := p.Desks[deskID]; !ok {
			errs = append(errs, fmt.Sprintf("group %q: desk %q not found", g.ID, deskID))
		}
	}
	// Validate nested group references.
	for _, subID := range g.Groups {
		if _, ok := p.Groups[subID]; !ok {
			errs = append(errs, fmt.Sprintf("group %q: sub-group %q not found", g.ID, subID))
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
	// Validate dispatch mode.
	if g.Dispatch != "" && g.Dispatch != "sequential" && g.Dispatch != "parallel" && g.Dispatch != "conversation" {
		errs = append(errs, fmt.Sprintf("group %q: invalid dispatch mode %q (must be sequential, parallel, or conversation)", g.ID, g.Dispatch))
	}
	// Validate lead position.
	if g.Lead != nil && g.Lead.Position != "" {
		switch g.Lead.Position {
		case "both", "first", "last":
		default:
			errs = append(errs, fmt.Sprintf("group %q: invalid lead position %q (must be both, first, or last)", g.ID, g.Lead.Position))
		}
	}
	// Validate trigger types.
	for i, trig := range g.Triggers {
		if trig.Type != "exec" && trig.Type != "poll" {
			errs = append(errs, fmt.Sprintf("group %q: trigger[%d] invalid type %q (must be exec or poll)", g.ID, i, trig.Type))
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
			errs = append(errs, fmt.Sprintf("policy %q: invalid on_timeout %q (must be fail, retry, or escalate)", pol.ID, pol.OnTimeout))
		}
	}
	if pol.OnError != "" {
		switch pol.OnError {
		case "fail", "retry", "escalate":
		default:
			errs = append(errs, fmt.Sprintf("policy %q: invalid on_error %q (must be fail, retry, or escalate)", pol.ID, pol.OnError))
		}
	}
	if pol.EscalateTo != "" && pol.OnError != "escalate" && pol.OnTimeout != "escalate" {
		errs = append(errs, fmt.Sprintf("policy %q: escalate_to is set but neither on_error nor on_timeout is 'escalate'", pol.ID))
	}
	return errs
}

func checkOrganization(org *types.Organization, p *config.Project) []string {
	var errs []string
	for _, groupID := range org.Groups {
		if _, ok := p.Groups[groupID]; !ok {
			errs = append(errs, fmt.Sprintf("organization %q: group %q not found", org.ID, groupID))
		}
	}
	for _, resID := range org.Resources {
		if _, ok := p.Resources[resID]; !ok {
			errs = append(errs, fmt.Sprintf("organization %q: resource %q not found", org.ID, resID))
		}
	}
	for i, rule := range org.Routing {
		// Check that routing target exists as a group or desk.
		_, isGroup := p.Groups[rule.To]
		_, isDesk := p.Desks[rule.To]
		if !isGroup && !isDesk {
			errs = append(errs, fmt.Sprintf("organization %q: routing[%d] target %q not found (must be a group or desk)", org.ID, i, rule.To))
		}
	}
	return errs
}

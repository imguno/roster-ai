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
	if d.Parent != "" {
		_, isGroup := p.Groups[d.Parent]
		if !isGroup && (p.Organization == nil || p.Organization.ID != d.Parent) {
			errs = append(errs, fmt.Sprintf("desk %q: parent %q not found (must be a group or org id)", d.ID, d.Parent))
		}
	}
	if d.Executor.Type == types.ExecutorTypeAPI && d.Executor.SDK != "" {
		switch d.Executor.SDK {
		case types.SDKAnthropic, types.SDKOpenAI, types.SDKGemini:
		default:
			errs = append(errs, fmt.Sprintf("desk %q: invalid executor.sdk %q", d.ID, d.Executor.SDK))
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
	if g.Parent != "" {
		_, isGroup := p.Groups[g.Parent]
		if !isGroup && (p.Organization == nil || p.Organization.ID != g.Parent) {
			errs = append(errs, fmt.Sprintf("group %q: parent %q not found", g.ID, g.Parent))
		}
	}
	for _, resID := range g.Resources {
		if _, ok := p.Resources[resID]; !ok {
			errs = append(errs, fmt.Sprintf("group %q: resource %q not found", g.ID, resID))
		}
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

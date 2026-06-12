package skill

import (
	"context"
	"strings"

	"github.com/roster-io/roster/pkg/types"
)

// BuildPrompt resolves all skill refs for an agent and merges them into a
// single prompt string. Skills teach the agent HOW to do things;
// resources (not skills) tell it WHAT to access.
func BuildPrompt(ctx context.Context, resolver *Resolver, skills []types.SkillRef, knowhow []types.SkillRef) (string, error) {
	var parts []string

	for _, ref := range skills {
		skill, err := resolver.Resolve(ctx, ref)
		if err != nil {
			// Skill not found — skip it silently so agents can still run
			// without all skills being present.
			continue
		}
		parts = append(parts, strings.TrimSpace(skill.Prompt))
	}

	// Knowhow: accumulated learning, appended after skills.
	if len(knowhow) > 0 {
		var khParts []string
		for _, ref := range knowhow {
			kh, err := resolver.Resolve(ctx, ref)
			if err != nil {
				continue
			}
			khParts = append(khParts, strings.TrimSpace(kh.Prompt))
		}
		if len(khParts) > 0 {
			parts = append(parts, "## Knowhow (learned from past work)\n\n"+strings.Join(khParts, "\n\n---\n\n"))
		}
	}

	// Standard skip instruction — every desk can self-govern participation.
	parts = append(parts, `---
If you have nothing meaningful to add given the current input and context, respond with exactly "SKIP" on the first line, optionally followed by a brief reason. Do not force output when you have no actionable contribution.`)

	return strings.Join(parts, "\n\n"), nil
}

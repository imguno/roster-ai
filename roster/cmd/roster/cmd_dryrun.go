package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/roster-io/roster/internal/agent/skill"
	"github.com/roster-io/roster/internal/config"
	"github.com/roster-io/roster/internal/validate"
)

func runDryRun(args []string) error {
	dir := "."
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			dir = args[i+1]
			i++
		} else if !strings.HasPrefix(args[i], "-") {
			dir = args[i]
		}
	}

	fmt.Println("Dry-run: validating", dir)
	fmt.Println()

	// 1. Load config
	project, err := config.LoadProject(dir)
	if err != nil {
		return fmt.Errorf("config load failed: %w", err)
	}
	fmt.Printf("  ✓ Config loaded: %d desks, %d groups, %d resources\n",
		len(project.Desks), len(project.Groups), len(project.Resources))

	// 2. Validate
	if err := validate.Project(project); err != nil {
		fmt.Printf("  ✗ Validation errors:\n")
		for _, line := range strings.Split(err.Error(), "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if line != "" && !strings.HasPrefix(line, "validation") {
				fmt.Printf("      %s\n", line)
			}
		}
	} else {
		fmt.Println("  ✓ Validation passed")
	}

	// 3. Check skill resolution
	resolver := skill.NewResolver(dir)
	ctx := context.Background()
	skillIssues := 0
	for id, agent := range project.Agents {
		for _, ref := range agent.Skills {
			if _, err := resolver.Resolve(ctx, ref); err != nil {
				fmt.Printf("  ⚠ Agent %q: skill %q not found\n", id, ref)
				skillIssues++
			}
		}
	}
	if skillIssues == 0 {
		fmt.Println("  ✓ All skills resolved")
	}

	// 4. Simulate event flow (subscribe/emit based)
	{
		fmt.Println()
		fmt.Println("  Event flow:")

		// Build emit map: event → emitters (only desks emit; groups are session scopes)
		emitMap := map[string][]string{}
		for id, d := range project.Desks {
			for _, ev := range d.Emit {
				emitMap[ev] = append(emitMap[ev], id)
			}
		}

		// Build subscribe map: event → subscribers (only desks subscribe)
		subMap := map[string][]string{}
		for id, d := range project.Desks {
			for _, ev := range d.Subscribe {
				subMap[ev] = append(subMap[ev], id)
			}
		}

		// Print connections
		allEvents := map[string]bool{}
		for ev := range emitMap {
			allEvents[ev] = true
		}
		for ev := range subMap {
			allEvents[ev] = true
		}
		evList := make([]string, 0, len(allEvents))
		for ev := range allEvents {
			evList = append(evList, ev)
		}
		sort.Strings(evList)
		for _, ev := range evList {
			emitters := emitMap[ev]
			subs := subMap[ev]
			emStr := "(external)"
			if len(emitters) > 0 {
				emStr = strings.Join(emitters, ", ")
			}
			subStr := "(none)"
			if len(subs) > 0 {
				subStr = strings.Join(subs, ", ")
			}
			if len(subs) == 0 {
				fmt.Printf("    ⚠ [%s] emitted by %s — no subscribers\n", ev, emStr)
			} else {
				fmt.Printf("    [%s]: %s → %s\n", ev, emStr, subStr)
			}
		}
	}

	// 5. Check executor connectivity (basic)
	fmt.Println()
	for id, desk := range project.Desks {
		switch desk.Executor.Type {
		case "exec":
			cmd := desk.Executor.Params["command"]
			if cmd == "" {
				fmt.Printf("  ✗ Desk %q: exec type but no command\n", id)
			}
		case "api":
			if desk.Executor.SDK == "" {
				fmt.Printf("  ✗ Desk %q: api type but no sdk\n", id)
			}
		case "remote":
			addr := desk.Executor.Params["address"]
			if addr == "" {
				fmt.Printf("  ✗ Desk %q: remote type but no address\n", id)
			}
		}
	}

	// 6. Summary
	fmt.Println()
	fmt.Printf("  Summary: %d desks, %d groups\n", len(project.Desks), len(project.Groups))
	fmt.Println("  ✓ Dry-run complete")

	return nil
}

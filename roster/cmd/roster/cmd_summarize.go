package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	filestore "github.com/roster-io/roster/internal/store/file"
)

// runSummarize compacts a desk's session history into a summary.
// Usage: roster summarize [--dir .] [--desk <id>] [--all]
func runSummarize(args []string) error {
	dir := "."
	deskID := ""
	all := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case "--desk":
			if i+1 < len(args) {
				deskID = args[i+1]
				i++
			}
		case "--all":
			all = true
		}
	}

	if deskID == "" && !all {
		return fmt.Errorf("usage: roster summarize --desk <id> or --all")
	}

	dataDir := filepath.Join(dir, ".roster", "data")
	store, err := filestore.New(dataDir)
	if err != nil {
		return fmt.Errorf("state store: %w", err)
	}

	summarizeDesk := func(id string) {
		entries := store.LoadSession(id, 0)
		if len(entries) == 0 {
			fmt.Printf("  %s: no session data\n", id)
			return
		}

		// Build a simple extractive summary: keep first and last entries,
		// count total, list roles involved.
		var roles []string
		roleSet := map[string]bool{}
		for _, e := range entries {
			if !roleSet[e.Role] {
				roleSet[e.Role] = true
				roles = append(roles, e.Role)
			}
		}

		summary := fmt.Sprintf("Session summary: %d entries from %s to %s. Roles: %s.\n\nMost recent exchange:\n\n",
			len(entries),
			entries[0].At.Format(time.RFC3339),
			entries[len(entries)-1].At.Format(time.RFC3339),
			strings.Join(roles, ", "),
		)

		// Keep last 4 entries as context.
		start := len(entries) - 4
		if start < 0 {
			start = 0
		}
		for _, e := range entries[start:] {
			summary += fmt.Sprintf("## %s\n%s\n\n", e.Role, e.Content)
		}

		store.SummarizeSession(id, summary)
		fmt.Printf("  %s: %d entries → summarized\n", id, len(entries))
	}

	if all {
		// Summarize all desks that have session data.
		sessionDir := filepath.Join(dataDir, "sessions")
		entries, err := os.ReadDir(sessionDir)
		if err != nil {
			return fmt.Errorf("no session data found")
		}
		fmt.Println("Summarizing all desks:")
		for _, e := range entries {
			if e.IsDir() {
				summarizeDesk(e.Name())
			}
		}
	} else {
		fmt.Println("Summarizing desk:")
		summarizeDesk(deskID)
	}

	return nil
}

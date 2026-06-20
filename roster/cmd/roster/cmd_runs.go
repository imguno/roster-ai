package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/roster-io/roster/internal/store/observe"
)

// runRuns shows recent runs grouped by run ID.
// Usage: roster runs [--dir .] [--n 20] [--output]
func runRuns(args []string) error {
	dir, n, showOutput := ".", 20, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case "--n":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil {
					n = v
				}
				i++
			}
		case "--output", "-o":
			showOutput = true
		}
	}

	logFile := filepath.Join(dir, ".roster", "data", "events.jsonl")
	data, err := os.ReadFile(logFile)
	if os.IsNotExist(err) {
		fmt.Println("no log file found:", logFile)
		return nil
	} else if err != nil {
		return err
	}

	type runInfo struct {
		id      string
		group   string
		startAt time.Time
		endAt   time.Time
		status  string // started, completed, failed, skipped
		output  string
		desks   []string
		deskSet map[string]struct{}
	}

	// Parse events and group by RunID.
	runs := map[string]*runInfo{}
	var order []string // insertion order (first seen)
	for _, line := range splitLines(data) {
		var e observe.Event
		if json.Unmarshal(line, &e) != nil || e.RunID == "" {
			continue
		}
		r, ok := runs[e.RunID]
		if !ok {
			r = &runInfo{id: e.RunID, status: "started", deskSet: map[string]struct{}{}}
			runs[e.RunID] = r
			order = append(order, e.RunID)

			// Extract group from run ID prefix (e.g. "strategy-team-20260609-…")
			// Run IDs follow: <prefix>-YYYYMMDD-HHMMSS-<hex>
			// Everything before the date segment is the group/desk name.
			if idx := findRunIDDateIndex(e.RunID); idx > 0 {
				r.group = e.RunID[:idx-1]
			} else {
				r.group = e.DeskID
			}
		}
		if e.At.Before(r.startAt) || r.startAt.IsZero() {
			r.startAt = e.At
		}
		if e.At.After(r.endAt) {
			r.endAt = e.At
		}
		switch e.Type {
		case observe.EventStepCompleted:
			r.status = "completed"
			if e.Output != "" {
				r.output = e.Output
			}
		case observe.EventStepFailed:
			r.status = "failed"
		case "step.skipped":
			if r.status == "started" {
				r.status = "skipped"
			}
		}
		if e.DeskID != "" {
			if _, seen := r.deskSet[e.DeskID]; !seen {
				r.deskSet[e.DeskID] = struct{}{}
				r.desks = append(r.desks, e.DeskID)
			}
		}
	}

	// Show the last n runs (most recent first).
	start := len(order) - n
	if start < 0 {
		start = 0
	}
	subset := order[start:]
	// Print most-recent first.
	for i := len(subset) - 1; i >= 0; i-- {
		r := runs[subset[i]]
		dur := ""
		if !r.endAt.IsZero() && !r.startAt.IsZero() {
			d := r.endAt.Sub(r.startAt).Round(time.Millisecond)
			if d >= time.Minute {
				dur = fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
			} else {
				dur = fmt.Sprintf("%.1fs", d.Seconds())
			}
		}
		statusSymbol := map[string]string{
			"completed": "✓",
			"failed":    "✗",
			"skipped":   "~",
			"started":   "⏳",
		}[r.status]
		if statusSymbol == "" {
			statusSymbol = "?"
		}
		groupStr := r.group
		if len(r.desks) > 0 && r.group != r.desks[0] {
			groupStr = r.group
		}
		fmt.Printf("%s  %s  %-30s  %-10s  %s\n",
			r.startAt.Format("2006-01-02 15:04:05"),
			statusSymbol,
			truncate(groupStr, 30),
			r.status,
			dur,
		)
		if showOutput && r.output != "" {
			lines := strings.SplitN(strings.TrimSpace(r.output), "\n", 6)
			for j, l := range lines {
				if j == 5 {
					fmt.Printf("  │ …\n")
					break
				}
				fmt.Printf("  │ %s\n", l)
			}
		}
	}
	if len(order) == 0 {
		fmt.Println("no runs found")
	}
	return nil
}

// findRunIDDateIndex returns the index of the date segment (YYYYMMDD) in a run ID,
// or -1 if not found. This lets us strip the date suffix to get the group/desk prefix.
func findRunIDDateIndex(runID string) int {
	// Date segment is 8 digits: YYYYMMDD
	for i := 0; i+8 <= len(runID); i++ {
		allDigits := true
		for _, c := range runID[i : i+8] {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return i
		}
	}
	return -1
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

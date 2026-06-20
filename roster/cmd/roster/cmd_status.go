package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func runStatus(args []string) error {
	hubURL := "http://localhost:8080"
	for i := 0; i < len(args); i++ {
		if args[i] == "--hub" && i+1 < len(args) {
			hubURL = args[i+1]
			i++
		}
	}

	// Fetch org, desks, groups, queues, warnings in parallel
	type result struct {
		org      map[string]any
		desks    map[string]any
		groups   map[string]any
		queues   map[string]float64
		warnings []any
		events   []map[string]any
	}

	var r result
	var fetchErr error

	fetch := func(path string, target any) {
		resp, err := http.Get(hubURL + path)
		if err != nil {
			fetchErr = fmt.Errorf("connect to hub at %s: %w", hubURL, err)
			return
		}
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(target)
	}

	fetch("/api/organization", &r.org)
	if fetchErr != nil {
		return fetchErr
	}
	fetch("/api/desks", &r.desks)
	fetch("/api/groups", &r.groups)
	fetch("/api/queues", &r.queues)
	fetch("/api/warnings", &r.warnings)
	fetch("/api/events", &r.events)

	// Organization
	orgName := "—"
	if name, ok := r.org["name"].(string); ok && name != "" {
		orgName = name
	}
	fmt.Printf("Organization: %s\n", orgName)
	fmt.Printf("Desks: %d  Groups: %d\n", len(r.desks), len(r.groups))
	fmt.Println()

	// Active work — scan events for current desk states
	deskStates := map[string]string{} // deskID → status
	for _, ev := range r.events {
		id, _ := ev["step_id"].(string)
		t, _ := ev["type"].(string)
		if id == "" {
			continue
		}
		switch t {
		case "step.started":
			deskStates[id] = "working"
		case "step.completed":
			deskStates[id] = "idle"
		case "step.failed":
			deskStates[id] = "error"
		case "human.waiting":
			deskStates[id] = "human"
		}
	}

	working := []string{}
	errors := []string{}
	human := []string{}
	for id, st := range deskStates {
		switch st {
		case "working":
			working = append(working, id)
		case "error":
			errors = append(errors, id)
		case "human":
			human = append(human, id)
		}
	}

	if len(working) > 0 {
		fmt.Printf("Working (%d):\n", len(working))
		for _, id := range working {
			fmt.Printf("  ⏳ %s\n", id)
		}
	}
	if len(human) > 0 {
		fmt.Printf("Waiting for human (%d):\n", len(human))
		for _, id := range human {
			fmt.Printf("  👤 %s\n", id)
		}
	}
	if len(errors) > 0 {
		fmt.Printf("Errors (%d):\n", len(errors))
		for _, id := range errors {
			fmt.Printf("  ✗ %s\n", id)
		}
	}
	if len(working) == 0 && len(human) == 0 && len(errors) == 0 {
		fmt.Println("All idle.")
	}

	// Queue depth
	totalQueued := 0.0
	for _, n := range r.queues {
		totalQueued += n
	}
	if totalQueued > 0 {
		fmt.Printf("\nQueued events: %.0f\n", totalQueued)
		for id, n := range r.queues {
			if n > 0 {
				fmt.Printf("  %s: %.0f pending\n", id, n)
			}
		}
	}

	// Warnings
	if len(r.warnings) > 0 {
		fmt.Printf("\nWarnings (%d):\n", len(r.warnings))
		for _, w := range r.warnings {
			if wm, ok := w.(map[string]any); ok {
				fmt.Printf("  ⚠ %v\n", wm["message"])
			}
		}
	}

	return nil
}

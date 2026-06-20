package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// runLoad sends a load request to a running hub via the API.
// Usage: roster load --dir ./my-org [--hub http://localhost:8080]
func runLoad(args []string) error {
	dir := ""
	hubURL := "http://localhost:8080"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case "--hub":
			if i+1 < len(args) {
				hubURL = args[i+1]
				i++
			}
		default:
			if dir == "" {
				dir = args[i] // positional arg
			}
		}
	}
	if dir == "" {
		return fmt.Errorf("usage: roster load <dir> or roster load --dir <dir>")
	}

	body := fmt.Sprintf(`{"dir":%q}`, dir)
	resp, err := http.Post(hubURL+"/api/load", "application/json", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("connect to hub at %s: %w", hubURL, err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode != http.StatusOK {
		if msg, ok := result["error"]; ok {
			return fmt.Errorf("hub: %v", msg)
		}
		return fmt.Errorf("hub returned %d", resp.StatusCode)
	}

	fmt.Printf("Loaded %s into hub:\n", dir)
	if d, ok := result["desks"]; ok {
		fmt.Printf("  Desks:  %.0f\n", d)
	}
	if g, ok := result["groups"]; ok {
		fmt.Printf("  Groups: %.0f\n", g)
	}
	if w, ok := result["warnings"]; ok {
		if ws, ok := w.([]any); ok && len(ws) > 0 {
			fmt.Printf("  Warnings: %d\n", len(ws))
			for _, ww := range ws {
				fmt.Printf("    ⚠ %v\n", ww)
			}
		}
	}
	return nil
}

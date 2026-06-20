// Package desk contains domain logic for the Desk entity.
// No external dependencies — pure Go types and functions.
package desk

import (
	"strings"
	"time"
)

// ResolveEmissions filters raw emission names against an allowlist.
// Returns (valid, rejected). If allowlist is empty, all are valid.
// If no raw emissions, defaults to ["done"].
func ResolveEmissions(raw []string, allowlist []string) (valid, rejected []string) {
	if len(raw) == 0 {
		raw = []string{"done"}
	}
	if len(allowlist) == 0 {
		return raw, nil
	}
	allowed := make(map[string]bool, len(allowlist))
	for _, e := range allowlist {
		allowed[e] = true
	}
	for _, name := range raw {
		if allowed[name] {
			valid = append(valid, name)
		} else {
			rejected = append(rejected, name)
		}
	}
	if len(valid) == 0 {
		valid = []string{"done"}
	}
	return valid, rejected
}

// ParseTimeout parses a desk timeout string into a duration.
// Returns zero duration and nil error if timeout is empty.
func ParseTimeout(timeout string) (time.Duration, error) {
	if timeout == "" {
		return 0, nil
	}
	return time.ParseDuration(timeout)
}

// IsScript returns true if the executor params indicate a script agent.
func IsScript(executorParams map[string]string) bool {
	return executorParams["command"] != ""
}

// EventNames returns the fully-qualified event names for a desk emission.
// Example: deskID="reviewer", groupIDs=["dev-team"], baseName="done"
// → ["reviewer.done", "dev-team.reviewer.done"]
func EventNames(deskID string, groupIDs []string, baseName string) []string {
	names := []string{deskID + "." + baseName}
	for _, gid := range groupIDs {
		names = append(names, gid+"."+deskID+"."+baseName)
	}
	return names
}

// FormatRejectedError creates an error message for rejected emissions.
func FormatRejectedError(rejected, allowed []string) string {
	return "rejected emissions [" + strings.Join(rejected, ", ") + "]; allowed: [" + strings.Join(allowed, ", ") + "]"
}

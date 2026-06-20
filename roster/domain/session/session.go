// Package session contains domain logic for session management.
// No external dependencies — pure Go types and functions.
package session

import (
	"fmt"
	"sort"
)

// Context holds structured metadata for a session entry.
type Context struct {
	Message string         // human-readable summary for UI
	Meta    map[string]any // structured fields for programmatic access
}

// BuildContext creates a structured session context from execution state.
func BuildContext(eventType string, skillNames []string, resourceIDs []string) Context {
	meta := map[string]any{
		"trigger": eventType,
	}
	if len(skillNames) > 0 {
		sorted := make([]string, len(skillNames))
		copy(sorted, skillNames)
		sort.Strings(sorted)
		meta["skills"] = sorted
	}
	if len(resourceIDs) > 0 {
		meta["resources"] = resourceIDs
	}

	return Context{
		Message: fmt.Sprintf("triggered by %s", eventType),
		Meta:    meta,
	}
}

// Scopes returns all scope IDs a desk execution should write to:
// the desk itself plus all its groups.
func Scopes(deskID string, groupIDs []string) []string {
	scopes := []string{deskID}
	scopes = append(scopes, groupIDs...)
	return scopes
}

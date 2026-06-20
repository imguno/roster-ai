// Package knowhow contains domain logic for accumulated learning.
// No external dependencies — pure Go types and functions.
package knowhow

import (
	"regexp"
	"strings"
	"time"
)

// Knowhow is a piece of learned knowledge extracted from execution output.
type Knowhow struct {
	DeskID      string
	Content     string
	ExtractedAt time.Time
}

var knowhowRe = regexp.MustCompile(`(?ms)^## Knowhow\s*\n(.+?)(?:\n## |\z)`)

// Extract parses output text for a "## Knowhow" section.
// Returns empty string if none found.
func Extract(output string) string {
	m := knowhowRe.FindStringSubmatch(output)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// Cap trims a knowhow list to at most maxEntries, keeping the most recent.
func Cap(items []Knowhow, maxEntries int) []Knowhow {
	if maxEntries <= 0 || len(items) <= maxEntries {
		return items
	}
	return items[len(items)-maxEntries:]
}

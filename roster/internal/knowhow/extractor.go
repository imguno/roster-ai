package knowhow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// marker pattern: a ## Knowhow section in the output
var knowhowRe = regexp.MustCompile(`(?ms)^## Knowhow\s*\n(.+?)(?:\n## |\z)`)

// Extract parses an artifact payload for knowhow sections.
// Returns the extracted knowhow text, or empty if none found.
func Extract(payload string) string {
	m := knowhowRe.FindStringSubmatch(payload)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// Save writes extracted knowhow to the knowhow directory.
// File is named {deskID}-{timestamp}.md to avoid collisions.
func Save(projectDir, deskID, content string) error {
	if content == "" {
		return nil
	}
	dir := filepath.Join(projectDir, "knowhow")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("knowhow: mkdir: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.md", deskID, ts)
	path := filepath.Join(dir, filename)

	header := fmt.Sprintf("# Knowhow: %s\n_Extracted: %s_\n\n", deskID, time.Now().Format(time.RFC3339))
	return os.WriteFile(path, []byte(header+content+"\n"), 0640)
}

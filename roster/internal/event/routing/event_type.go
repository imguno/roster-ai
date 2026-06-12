package routing

import (
	"regexp"
	"strings"
)

// DetermineEventType analyzes payload content to determine if an event represents
// success or failure, adjusting the declared event type accordingly.
// When both markers appear, the last-occurring one wins.
func DetermineEventType(declaredType string, payload []byte) string {
	content := strings.ToLower(string(payload))

	if strings.HasSuffix(declaredType, ".approved") {
		if strings.Contains(content, "request changes") || strings.Contains(content, "needs discussion") {
			return strings.TrimSuffix(declaredType, ".approved") + ".changes_requested"
		}
	}

	successPatterns := []string{
		"=== build succeeded ===",
		"=== all tests passed ===",
		"build succeeded",
		"tests passed",
		"completed successfully",
		"✓",
	}
	failurePatterns := []string{
		"=== build failed ===",
		"=== tests failed ===",
		"=== tests skipped — build failed ===",
		"build failed",
		"tests failed",
		"tests skipped",
		"✗",
		"panic:",
		"fatal:",
	}

	lastSuccess, lastFailure := -1, -1
	for _, p := range successPatterns {
		if idx := strings.LastIndex(content, p); idx > lastSuccess {
			lastSuccess = idx
		}
	}
	for _, p := range failurePatterns {
		if idx := strings.LastIndex(content, p); idx > lastFailure {
			lastFailure = idx
		}
	}

	if lastSuccess >= 0 && lastSuccess > lastFailure {
		if strings.HasSuffix(declaredType, ".failed") {
			base := strings.TrimSuffix(declaredType, ".failed")
			if strings.Contains(base, "test") {
				return base + ".passed"
			}
			return base + ".succeeded"
		}
	}

	if lastFailure >= 0 && lastFailure > lastSuccess {
		if strings.HasSuffix(declaredType, ".succeeded") || strings.HasSuffix(declaredType, ".passed") {
			return regexp.MustCompile(`\.(succeeded|passed)$`).ReplaceAllString(declaredType, "") + ".failed"
		}
	}

	return declaredType
}


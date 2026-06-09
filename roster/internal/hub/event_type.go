package hub

import (
	"regexp"
	"strings"
)

// DetermineEventType analyzes payload content to determine if an event represents success or failure.
// It looks for common success/failure patterns and adjusts the declared event type accordingly.
// When both success and failure markers appear (e.g. agent commentary quoting old failure messages
// alongside actual successful output), the last-occurring marker wins — since build/test output
// always appears after any explanatory text.
func DetermineEventType(declaredType string, payload []byte) string {
	content := strings.ToLower(string(payload))

	// Structured terminal markers from the build/test scripts themselves.
	// Finding the last occurrence of any success vs failure marker breaks ties.
	// Review verdict patterns — checked before generic success/failure.
	if strings.HasSuffix(declaredType, ".approved") {
		if strings.Contains(content, "request changes") || strings.Contains(content, "needs discussion") {
			base := strings.TrimSuffix(declaredType, ".approved")
			return base + ".changes_requested"
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

	lastSuccess := -1
	lastFailure := -1

	for _, pattern := range successPatterns {
		if idx := strings.LastIndex(content, pattern); idx > lastSuccess {
			lastSuccess = idx
		}
	}
	for _, pattern := range failurePatterns {
		if idx := strings.LastIndex(content, pattern); idx > lastFailure {
			lastFailure = idx
		}
	}

	isSuccess := lastSuccess >= 0 && lastSuccess > lastFailure
	isFailure := lastFailure >= 0 && lastFailure > lastSuccess

	if isSuccess {
		if strings.HasSuffix(declaredType, ".failed") {
			base := strings.TrimSuffix(declaredType, ".failed")
			if strings.Contains(base, "test") {
				return base + ".passed"
			}
			return base + ".succeeded"
		}
	}

	if isFailure {
		if strings.HasSuffix(declaredType, ".succeeded") || strings.HasSuffix(declaredType, ".passed") {
			base := regexp.MustCompile(`\.(succeeded|passed)$`).ReplaceAllString(declaredType, "")
			return base + ".failed"
		}
	}

	// Default: return the original declared type
	return declaredType
}

// isSkip returns true if the artifact output starts with "SKIP".
func isSkip(payload []byte) bool {
	s := strings.TrimSpace(string(payload))
	return strings.HasPrefix(s, "SKIP")
}

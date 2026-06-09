package hub

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/roster-io/roster/internal/observe"
	"github.com/roster-io/roster/pkg/types"
)

// handleRosterAction processes built-in roster resource actions for desk-to-desk communication.
// Actions:
//   - call(desk, prompt): synchronously execute a desk and return its output
//   - artifact(desk): return the latest artifact for a desk
//   - session(desk, limit): return recent session entries for a desk
func (h *Hub) handleRosterAction(ctx context.Context, callerDeskID, action string, params map[string]string) (string, error) {
	switch action {
	case "call":
		return h.rosterCall(ctx, callerDeskID, params)
	case "artifact":
		return h.rosterArtifact(params)
	case "session":
		return h.rosterSession(params)
	default:
		return "", fmt.Errorf("roster: unknown action %q", action)
	}
}

// rosterCall synchronously executes a target desk and returns its output.
func (h *Hub) rosterCall(ctx context.Context, callerDeskID string, params map[string]string) (string, error) {
	targetDeskID := params["desk"]
	if targetDeskID == "" {
		return "", fmt.Errorf("roster.call: missing 'desk' param")
	}

	// Prevent self-calls.
	if targetDeskID == callerDeskID {
		return "", fmt.Errorf("roster.call: desk %q cannot call itself", callerDeskID)
	}

	desk, ok := h.desks[targetDeskID]
	if !ok {
		return "", fmt.Errorf("roster.call: desk %q not found", targetDeskID)
	}

	// Build input from caller's prompt param.
	prompt := params["prompt"]
	input := &types.Artifact{
		Schema:  "text-v1",
		Payload: []byte(prompt),
	}

	runID := newRunID("call-" + targetDeskID)
	h.recorder.Record(observe.Event{
		RunID:  runID,
		StepID: targetDeskID,
		Type:   observe.EventType("roster.call"),
		Error:  fmt.Sprintf("called by %s", callerDeskID),
	})

	artifact, err := h.executeDesk(ctx, runID, targetDeskID, desk, input, nil)
	if err != nil {
		return "", fmt.Errorf("roster.call: desk %q: %w", targetDeskID, err)
	}
	if artifact == nil {
		return "", nil
	}

	return string(artifact.Payload), nil
}

// rosterArtifact returns the latest artifact stored for a desk.
func (h *Hub) rosterArtifact(params map[string]string) (string, error) {
	targetDeskID := params["desk"]
	if targetDeskID == "" {
		return "", fmt.Errorf("roster.artifact: missing 'desk' param")
	}

	content, known := h.DeskArtifact(targetDeskID)
	if !known {
		return "", fmt.Errorf("roster.artifact: desk %q not found", targetDeskID)
	}
	if content == "" {
		return "(no artifact)", nil
	}
	return content, nil
}

// rosterSession returns recent session entries for a desk.
func (h *Hub) rosterSession(params map[string]string) (string, error) {
	targetDeskID := params["desk"]
	if targetDeskID == "" {
		return "", fmt.Errorf("roster.session: missing 'desk' param")
	}

	entries := h.store.DeskSession().Load(targetDeskID)
	if len(entries) == 0 {
		return "(no session history)", nil
	}

	// Apply limit if specified.
	limit := len(entries)
	if limitStr := params["limit"]; limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n < limit {
			entries = entries[len(entries)-n:]
			limit = n
		}
	}
	_ = limit // used only for slicing above

	// Format as readable text.
	var sb strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&sb, "[%s] %s:\n%s\n\n", e.At.Format(time.RFC3339), e.Role, e.Content)
	}
	return sb.String(), nil
}

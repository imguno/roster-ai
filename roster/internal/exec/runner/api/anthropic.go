package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

type anthropicRunner struct {
	client *http.Client
}

func newAnthropicRunner() *anthropicRunner {
	return &anthropicRunner{client: &http.Client{Timeout: 120 * time.Second}}
}

func (r *anthropicRunner) Run(ctx context.Context, task sdk.Task) (*types.Output, error) {
	apiKey := task.Options["api_key"]
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic: missing api_key in desk runner params")
	}
	model := task.Options["model"]
	if model == "" {
		model = "claude-sonnet-4-5"
	}

	// Build message history from prior desk session turns.
	messages := make([]map[string]string, 0, len(task.Session)+1)
	for _, e := range task.Session {
		if e.Role == "user" || e.Role == "assistant" {
			messages = append(messages, map[string]string{"role": e.Role, "content": e.Content})
		}
	}

	// Prepend group context to the current user message so the desk sees
	// what teammates have already produced in this group run.
	userContent := buildUserContent(task.Prompt, task.GroupHistory)
	messages = append(messages, map[string]string{"role": "user", "content": userContent})

	body, _ := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 8096,
		"messages":   messages,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("anthropic: decode response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("anthropic: %s", result.Error.Message)
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("anthropic: empty response")
	}

	return &types.Output{
		Content: result.Content[0].Text,
		Metrics: map[string]float64{
			"input_tokens":  float64(result.Usage.InputTokens),
			"output_tokens": float64(result.Usage.OutputTokens),
		},
	}, nil
}

// buildUserContent prepends a formatted team context block when teammates have
// already contributed to the group session, so the current desk can build on
// their work rather than starting from scratch.
func buildUserContent(prompt string, groupHistory []sdk.GroupMessage) string {
	if len(groupHistory) == 0 {
		return prompt
	}
	var sb strings.Builder
	sb.WriteString("## Team Context\n\nThe following teammates have already contributed:\n\n")
	for _, msg := range groupHistory {
		fmt.Fprintf(&sb, "### %s (%s)\n%s\n\n", msg.DeskID, msg.Role, msg.Content)
	}
	sb.WriteString("---\n\n")
	sb.WriteString(prompt)
	return sb.String()
}

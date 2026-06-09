package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

type openaiRunner struct {
	client *http.Client
}

func newOpenAIRunner() *openaiRunner {
	return &openaiRunner{client: &http.Client{Timeout: 120 * time.Second}}
}

func (r *openaiRunner) Run(ctx context.Context, task sdk.Task) (*types.Artifact, error) {
	apiKey := task.Options["api_key"]
	if apiKey == "" {
		return nil, fmt.Errorf("openai: missing api_key in desk runner params")
	}
	model := task.Options["model"]
	if model == "" {
		model = "gpt-4o"
	}

	// Build message history from prior desk session turns.
	messages := make([]map[string]string, 0, len(task.Session)+1)
	for _, e := range task.Session {
		if e.Role == "user" || e.Role == "assistant" {
			messages = append(messages, map[string]string{"role": e.Role, "content": e.Content})
		}
	}

	userContent := buildUserContent(task.Prompt, task.GroupHistory)
	messages = append(messages, map[string]string{"role": "user", "content": userContent})

	body, _ := json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("content-type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai: decode response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("openai: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("openai: empty response")
	}

	return &types.Artifact{
		AgentID: task.AgentID,
		Schema:  "text-v1",
		Payload: []byte(result.Choices[0].Message.Content),
		Meta: map[string]string{
			"input_tokens":  strconv.Itoa(result.Usage.PromptTokens),
			"output_tokens": strconv.Itoa(result.Usage.CompletionTokens),
			"model":         model,
		},
		CreatedAt: time.Now(),
	}, nil
}

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

type geminiRunner struct {
	client *http.Client
}

func newGeminiRunner() *geminiRunner {
	return &geminiRunner{client: &http.Client{Timeout: 120 * time.Second}}
}

func (r *geminiRunner) Run(ctx context.Context, task sdk.Task) (*types.Artifact, error) {
	apiKey := task.Options["api_key"]
	if apiKey == "" {
		return nil, fmt.Errorf("gemini: missing api_key in desk runner params")
	}
	model := task.Options["model"]
	if model == "" {
		model = "gemini-1.5-pro"
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model, apiKey,
	)

	// Build conversation history from prior desk session turns.
	// Gemini uses "user"/"model" roles (not "user"/"assistant").
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}

	contents := make([]content, 0, len(task.Session)+1)
	for _, e := range task.Session {
		role := e.Role
		if role == "assistant" {
			role = "model"
		}
		if role != "user" && role != "model" {
			continue
		}
		contents = append(contents, content{Role: role, Parts: []part{{Text: e.Content}}})
	}

	userContent := buildUserContent(task.Prompt, task.GroupHistory)
	contents = append(contents, content{Role: "user", Parts: []part{{Text: userContent}}})

	body, _ := json.Marshal(map[string]any{"contents": contents})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	req.Header.Set("content-type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("gemini: decode response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("gemini: %s", result.Error.Message)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini: empty response")
	}

	return &types.Artifact{
		AgentID: task.AgentID,
		Schema:  "text-v1",
		Payload: []byte(result.Candidates[0].Content.Parts[0].Text),
		Meta: map[string]string{
			"input_tokens":  strconv.Itoa(result.UsageMetadata.PromptTokenCount),
			"output_tokens": strconv.Itoa(result.UsageMetadata.CandidatesTokenCount),
			"model":         model,
		},
		CreatedAt: time.Now(),
	}, nil
}

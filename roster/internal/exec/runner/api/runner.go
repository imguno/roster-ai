package api

import (
	"context"
	"fmt"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

// Runner dispatches to the appropriate built-in SDK implementation
// based on task.Options["sdk"].
type Runner struct {
	backends map[types.SDKType]sdk.Executor
}

func New() *Runner {
	r := &Runner{backends: make(map[types.SDKType]sdk.Executor)}
	r.backends[types.SDKAnthropic] = newAnthropicRunner()
	r.backends[types.SDKOpenAI] = newOpenAIRunner()
	r.backends[types.SDKGemini] = newGeminiRunner()
	return r
}

func (r *Runner) Run(ctx context.Context, task sdk.Task) (*types.Output, error) {
	sdkName := types.SDKType(task.Options["sdk"])
	backend, ok := r.backends[sdkName]
	if !ok {
		return nil, fmt.Errorf("api runner: unsupported sdk %q (supported: anthropic, openai, gemini)", sdkName)
	}
	return backend.Run(ctx, task)
}

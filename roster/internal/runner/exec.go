package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

// maxStdoutBytes caps subprocess stdout to prevent unbounded memory growth.
// Exceeded outputs produce a clear error rather than an OOM kill.
const maxStdoutBytes = 10 * 1024 * 1024 // 10 MB

// limitedWriter is an io.Writer that accepts up to max bytes and then discards,
// returning len(p) always so cmd.Run does not see a write error.
type limitedWriter struct {
	buf bytes.Buffer
	n   int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	remaining := maxStdoutBytes - lw.n
	lw.n += len(p)
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		lw.buf.Write(p[:remaining]) //nolint:errcheck // bytes.Buffer.Write never errors
		return len(p), nil
	}
	lw.buf.Write(p) //nolint:errcheck
	return len(p), nil
}

func (lw *limitedWriter) exceeded() bool { return lw.n > maxStdoutBytes }
func (lw *limitedWriter) Bytes() []byte  { return lw.buf.Bytes() }

// ExecRunner runs an arbitrary command as a Roster executor.
//
// # Exec Protocol (stdin → stdout)
//
// The hub writes a JSON object to stdin:
//
//	{
//	  "run_id":       "...",
//	  "agent_id":     "researcher",
//	  "desk_id":      "researcher-desk",
//	  "prompt":       "<merged skill prompts + input>",
//	  "session":      [{"role":"user","content":"..."},...],
//	  "group_history":[{"desk_id":"...","role":"...","content":"..."},...],
//	  "input":        "<input artifact payload as string, empty if none>",
//	  "resources": [
//	    {"id":"codebase","type":"github","actions":["commit","notify"]}
//	  ]
//	}
//
// The command must write a JSON object to stdout:
//
//	{"schema": "text-v1", "payload": "<output>"}
//
// To invoke a resource action, write a line prefixed with "ACTION:" to stderr:
//
//	ACTION:{"resource":"codebase","action":"commit","params":{"message":"fix bug"}}
//
// The hub executes the action and the result is available via ROSTER_ACTION_RESULT env var
// on the next invocation, or immediately if the process supports the streaming protocol.
//
// Exit code 0 = success. Any other exit code = error (stderr is captured).
type ExecRunner struct{}

func NewExecRunner() *ExecRunner { return &ExecRunner{} }

// execInput is the JSON envelope written to the subprocess's stdin.
type execInput struct {
	RunID        string              `json:"run_id"`
	AgentID      string              `json:"agent_id"`
	DeskID       string              `json:"desk_id"`
	Prompt       string              `json:"prompt"`
	Session      []sdk.SessionEntry  `json:"session,omitempty"`
	GroupHistory []sdk.GroupMessage  `json:"group_history,omitempty"`
	Input        string              `json:"input,omitempty"` // input artifact payload as UTF-8 string
	Resources    []sdk.TaskResource  `json:"resources,omitempty"`
}

// actionRequest is parsed from stderr lines prefixed with "ACTION:".
type actionRequest struct {
	Resource string            `json:"resource"`
	Action   string            `json:"action"`
	Params   map[string]string `json:"params,omitempty"`
}

// execOutput is the JSON envelope expected from the subprocess's stdout.
type execOutput struct {
	Schema  string `json:"schema"`
	Payload string `json:"payload"` // plain string; stored as []byte in Artifact
}

func (e *ExecRunner) Run(ctx context.Context, task sdk.Task) (*types.Artifact, error) {
	command, ok := task.Options["command"]
	if !ok || command == "" {
		return nil, fmt.Errorf("exec: missing 'command' param for desk %s", task.DeskID)
	}

	// Build stdin envelope.
	in := execInput{
		RunID:        task.RunID,
		AgentID:      task.AgentID,
		DeskID:       task.DeskID,
		Prompt:       task.Prompt,
		Session:      task.Session,
		GroupHistory: task.GroupHistory,
		Resources:    task.Resources,
	}
	if task.Input != nil {
		in.Input = string(task.Input.Payload)
	}
	stdinData, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("exec: marshal stdin: %w", err)
	}

	parts, err := shellSplit(command)
	if err != nil {
		return nil, fmt.Errorf("exec: parse command %q: %w", command, err)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("exec: empty command")
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	if task.WorkDir != "" {
		cmd.Dir = task.WorkDir
	}

	cmd.Env = append(os.Environ(),
		"ROSTER_AGENT_ID="+task.AgentID,
		"ROSTER_DESK_ID="+task.DeskID,
		"ROSTER_RUN_ID="+task.RunID,
	)
	for k, v := range task.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	cmd.Stdin = bytes.NewReader(stdinData)

	var stdout limitedWriter
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("exec [%s]: %w\nstderr: %s", command, err, stderr.String())
	}
	if stdout.exceeded() {
		return nil, fmt.Errorf("exec [%s]: stdout exceeded %d MB limit (%d bytes written)", command, maxStdoutBytes/1024/1024, stdout.n)
	}

	// Process stderr protocol lines (ACTION:, METRIC:).
	metrics := e.processStderr(stderr.String(), task)

	// Convert metrics to artifact Meta for hub to pick up.
	meta := map[string]string{}
	for k, v := range metrics {
		meta["metric:"+k] = fmt.Sprintf("%g", v)
	}

	var out execOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		// Fallback: treat raw stdout as plain-text payload.
		return &types.Artifact{
			AgentID:   task.AgentID,
			Schema:    "text-v1",
			Payload:   bytes.TrimSpace(stdout.Bytes()),
			Meta:      meta,
			CreatedAt: time.Now(),
		}, nil
	}

	return &types.Artifact{
		AgentID:   task.AgentID,
		Schema:    out.Schema,
		Payload:   []byte(out.Payload),
		Meta:      meta,
		CreatedAt: time.Now(),
	}, nil
}

// processStderr scans stderr for protocol lines:
//   ACTION:{json}  — invoke a resource action
//   METRIC:{json}  — report metrics (key→float64 pairs)
// Returns any metrics reported by the process.
func (e *ExecRunner) processStderr(stderrOutput string, task sdk.Task) map[string]float64 {
	var metrics map[string]float64
	for _, line := range strings.Split(stderrOutput, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "ACTION:") && task.ActionCallback != nil {
			payload := strings.TrimPrefix(line, "ACTION:")
			var req actionRequest
			if json.Unmarshal([]byte(payload), &req) != nil {
				continue
			}
			result, err := task.ActionCallback(req.Resource, req.Action, req.Params)
			if err != nil {
				fmt.Fprintf(os.Stderr, "roster: action %s.%s failed: %v\n", req.Resource, req.Action, err)
			}
			_ = result
			continue
		}

		if strings.HasPrefix(line, "METRIC:") {
			payload := strings.TrimPrefix(line, "METRIC:")
			var m map[string]float64
			if json.Unmarshal([]byte(payload), &m) == nil {
				if metrics == nil {
					metrics = make(map[string]float64)
				}
				for k, v := range m {
					metrics[k] += v
				}
			}
			continue
		}
	}
	return metrics
}

// shellSplit tokenizes a command string respecting single- and double-quoted
// spans, so paths like `"C:\Program Files\app.exe" --flag` parse correctly.
func shellSplit(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	for _, r := range s {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t') && !inSingle && !inDouble:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote")
	}
	return tokens, nil
}

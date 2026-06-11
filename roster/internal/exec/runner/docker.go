package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
)

// DockerExecutor runs a Docker container as a Roster task.
// The container receives the prompt via stdin and must write a JSON artifact to stdout.
//
// Desk config example:
//
//	executor:
//	  type: docker
//	  params:
//	    image: "my-org/researcher:latest"
type DockerExecutor struct{}

func NewDockerRunner() *DockerExecutor { return &DockerExecutor{} }

func (d *DockerExecutor) Run(ctx context.Context, task sdk.Task) (*types.Artifact, error) {
	image, ok := task.Options["image"]
	if !ok || image == "" {
		return nil, fmt.Errorf("docker executor: missing 'image' param for desk %s", task.DeskID)
	}

	args := []string{"run", "--rm", "-i",
		"-e", "ROSTER_AGENT_ID=" + task.AgentID,
		"-e", "ROSTER_DESK_ID=" + task.DeskID,
	}
	for k, v := range task.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, image)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader(task.Prompt)

	var stdout limitedWriter
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker executor [%s]: %w\nstderr: %s", image, err, stderr.String())
	}
	if stdout.exceeded() {
		return nil, fmt.Errorf("docker executor [%s]: stdout exceeded %d MB limit (%d bytes written)", image, maxStdoutBytes/1024/1024, stdout.n)
	}

	var out struct {
		Schema  string `json:"schema"`
		Payload []byte `json:"payload"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("docker executor [%s]: stdout is not valid artifact JSON: %w", image, err)
	}

	return &types.Artifact{
		AgentID:   task.AgentID,
		Schema:    out.Schema,
		Payload:   out.Payload,
		CreatedAt: time.Now(),
	}, nil
}

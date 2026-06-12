package runner

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
	pb "github.com/roster-io/roster/proto"
)

// RemoteExecutor calls a Roster worker running in worker mode via gRPC.
type RemoteExecutor struct {
	address string
}

func NewRemoteRunner(address string) *RemoteExecutor {
	return &RemoteExecutor{address: address}
}

func (r *RemoteExecutor) Run(ctx context.Context, task sdk.Task) (*types.Output, error) {
	addr := r.address
	if a, ok := task.Options["address"]; ok && a != "" {
		addr = a
	}
	if addr == "" {
		return nil, fmt.Errorf("remote executor: no address configured for desk %s", task.DeskID)
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("remote executor dial %s: %w", addr, err)
	}
	defer conn.Close()

	client := pb.NewWorkerClient(conn)
	resp, err := client.Execute(ctx, &pb.ExecuteRequest{
		AgentId: task.AgentID,
		DeskId:  task.DeskID,
		Prompt:  task.Prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("remote executor execute: %w", err)
	}

	return &types.Output{
		Content: string(resp.Payload),
	}, nil
}

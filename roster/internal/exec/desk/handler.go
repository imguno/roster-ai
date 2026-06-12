package desk

import (
	"context"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
	pb "github.com/roster-io/roster/proto"
)

type handler struct {
	pb.UnimplementedWorkerServer
	registry Dispatcher
}

func (h *handler) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	session := make([]sdk.SessionEntry, len(req.Session))
	for i, e := range req.Session {
		session[i] = sdk.SessionEntry{Role: e.Role, Content: e.Content}
	}
	groupHistory := make([]sdk.GroupMessage, len(req.GroupHistory))
	for i, m := range req.GroupHistory {
		groupHistory[i] = sdk.GroupMessage{DeskID: m.DeskId, Role: m.Role, Content: m.Content}
	}

	task := sdk.Task{
		AgentID:      req.AgentId,
		DeskID:       req.DeskId,
		Prompt:       req.Prompt,
		Options:      req.Options,
		Env:          req.Env,
		Session:      session,
		GroupHistory: groupHistory,
	}

	output, err := h.registry.Dispatch(ctx, types.ExecutorType(req.ExecutorType), task)
	if err != nil {
		return &pb.ExecuteResponse{Error: err.Error()}, nil
	}
	if output == nil {
		return &pb.ExecuteResponse{}, nil
	}
	return &pb.ExecuteResponse{
		Payload: []byte(output.Content),
	}, nil
}

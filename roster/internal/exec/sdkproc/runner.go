package sdkproc

import (
	"context"
	"fmt"
	"io"

	pkgsdk "github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
	"github.com/roster-io/roster/proto"
)

// LogCallback is called in real-time for each log entry streamed from the SDK agent.
type LogCallback func(entry *proto.LogEntry)

// Result holds the output, emissions, note updates, and metrics from an SDK execution.
type Result struct {
	Output      *types.Output
	Emissions   []*proto.EmitEvent
	NoteUpdates []*proto.NoteUpdate
	Metrics     map[string]float64
}

// ExecuteResource calls an SDK resource action via gRPC.
func ExecuteResource(ctx context.Context, client proto.ResourceServiceClient, resourceID, action string, params map[string]string) (string, error) {
	req := &proto.ResourceActionRequest{
		ResourceId: resourceID,
		Action:     action,
		Params:     params,
	}
	resp, err := client.ExecuteAction(ctx, req)
	if err != nil {
		return "", fmt.Errorf("sdk resource execute: %w", err)
	}
	if resp.Error != "" {
		return "", fmt.Errorf("sdk resource error: %s", resp.Error)
	}
	return string(resp.Output), nil
}

// Execute calls the SDK agent via streaming gRPC.
// onLog is called in real-time for each intermediate log entry; may be nil.
func Execute(ctx context.Context, client proto.AgentServiceClient, task pkgsdk.Task, onLog LogCallback) (*Result, error) {
	eventType := task.EventType
	if eventType == "" {
		eventType = task.Options["event_type"]
	}

	req := &proto.TaskRequest{
		EventType:    eventType,
		DeskId:       task.DeskID,
		AgentId:      task.AgentID,
		GroupId:      task.GroupID,
	}

	for _, e := range task.Session {
		req.Session = append(req.Session, &proto.Message{
			DeskId: task.DeskID, Role: e.Role, Content: e.Content, Type: "session",
		})
	}
	for _, m := range task.GroupHistory {
		req.Session = append(req.Session, &proto.Message{
			DeskId: m.DeskID, Role: m.Role, Content: m.Content, Type: "group",
		})
	}
	for k, v := range task.Notes {
		req.Notes = append(req.Notes, &proto.Note{Key: k, Value: v})
	}
	for _, r := range task.Resources {
		res := &proto.Resource{Id: r.ID, Type: r.Type}
		for k, v := range r.Config {
			res.Actions = append(res.Actions, &proto.Action{Name: k, Exec: v})
		}
		req.Resources = append(req.Resources, res)
	}
	if len(task.Options) > 0 {
		req.Options = task.Options
	}
	if len(task.Env) > 0 {
		req.Env = task.Env
	}
	for name, content := range task.Skills {
		req.Skills = append(req.Skills, &proto.Skill{Name: name, Content: content})
	}

	// Open streaming RPC.
	stream, err := client.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sdk execute: %w", err)
	}

	var finalResult *proto.TaskResult
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sdk stream: %w", err)
		}

		// Intermediate log event — forward in real-time.
		if ev.Log != nil && onLog != nil {
			onLog(ev.Log)
		}

		// Final result event.
		if ev.Result != nil {
			finalResult = ev.Result
		}
	}

	if finalResult == nil {
		return nil, fmt.Errorf("sdk: no result received")
	}
	if finalResult.Error != "" {
		return nil, fmt.Errorf("sdk agent error: %s", finalResult.Error)
	}

	// No emissions = agent didn't produce output → nil output, no events.
	if len(finalResult.Emissions) == 0 {
		return &Result{
			NoteUpdates: finalResult.NoteUpdates,
			Metrics:     finalResult.Metrics,
		}, nil
	}

	var content string
	for _, em := range finalResult.Emissions {
		if em.Payload != nil {
			content = string(em.Payload)
			break
		}
	}

	return &Result{
		Output:      &types.Output{Content: content},
		Emissions:   finalResult.Emissions,
		NoteUpdates: finalResult.NoteUpdates,
		Metrics:     finalResult.Metrics,
	}, nil
}

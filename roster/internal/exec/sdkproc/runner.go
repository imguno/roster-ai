package sdkproc

import (
	"context"
	"fmt"

	pkgsdk "github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
	"github.com/roster-io/roster/proto"
)

// Result holds the artifact, emissions, logs, note updates, and metrics from an SDK execution.
type Result struct {
	Artifact    *types.Artifact
	Emissions   []*proto.EmitEvent
	Logs        []*proto.LogEntry
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

// Execute calls the SDK agent process via gRPC and returns the result.
func Execute(ctx context.Context, client proto.AgentServiceClient, task pkgsdk.Task) (*Result, error) {
	eventType := task.EventType
	if eventType == "" {
		eventType = task.Options["event_type"]
	}

	req := &proto.TaskRequest{
		EventType:    eventType,
		EventPayload: task.Input.Payload,
		DeskId:       task.DeskID,
		AgentId:      task.AgentID,
		GroupId:      task.GroupID,
	}

	// Attach desk session history.
	for _, e := range task.Session {
		req.Session = append(req.Session, &proto.Message{
			DeskId:  task.DeskID,
			Role:    e.Role,
			Content: e.Content,
			Type:    "session",
		})
	}

	// Attach group history as peer messages.
	for _, m := range task.GroupHistory {
		req.Session = append(req.Session, &proto.Message{
			DeskId:  m.DeskID,
			Role:    m.Role,
			Content: m.Content,
			Type:    "group",
		})
	}

	// Attach notes.
	for k, v := range task.Notes {
		req.Notes = append(req.Notes, &proto.Note{Key: k, Value: v})
	}

	// Attach resources — encode config as Action{Name: key, Exec: value} entries.
	for _, r := range task.Resources {
		res := &proto.Resource{Id: r.ID, Type: r.Type}
		for k, v := range r.Config {
			res.Actions = append(res.Actions, &proto.Action{Name: k, Exec: v})
		}
		req.Resources = append(req.Resources, res)
	}

	resp, err := client.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sdk execute: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("sdk agent error: %s", resp.Error)
	}

	// Rewrite bare "done" to "{deskID}.done".
	for _, em := range resp.Emissions {
		if em.EventType == "done" {
			em.EventType = task.DeskID + ".done"
		}
	}

	var payload []byte
	for _, em := range resp.Emissions {
		if em.Payload != nil {
			payload = em.Payload
			break
		}
	}
	if payload == nil {
		payload = []byte("ok")
	}

	return &Result{
		Artifact:    &types.Artifact{Schema: "sdk-v1", Payload: payload},
		Emissions:   resp.Emissions,
		Logs:        resp.Logs,
		NoteUpdates: resp.NoteUpdates,
		Metrics:     resp.Metrics,
	}, nil
}

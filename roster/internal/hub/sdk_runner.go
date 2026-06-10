package hub

import (
	"context"
	"fmt"
	"sync"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
	"github.com/roster-io/roster/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// sdkProcessManager manages a single shared SDK gRPC process.
// All agents are loaded by entry-point discovery inside that one process.
type sdkProcessManager struct {
	mu         sync.Mutex
	basePort   int
	pythonBin  string
	nodeBin    string
	projectDir string
	entry      *sdkEntry
}

type sdkEntry struct {
	proc           SDKProcess
	client         proto.AgentServiceClient
	resourceClient proto.ResourceServiceClient
	conn           *grpc.ClientConn
}

func newSDKProcessManager(basePort int) *sdkProcessManager {
	return &sdkProcessManager{basePort: basePort}
}

func (m *sdkProcessManager) SetPythonBin(bin string)   { m.pythonBin = bin }
func (m *sdkProcessManager) SetNodeBin(bin string)     { m.nodeBin = bin }
func (m *sdkProcessManager) SetProjectDir(dir string)  { m.projectDir = dir }

// GetOrStart returns a shared gRPC client, starting the process on first call.
// One process serves all agents via entry-point discovery.
func (m *sdkProcessManager) GetOrStart(ctx context.Context) (proto.AgentServiceClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.entry != nil {
		return m.entry.client, nil
	}

	var proc SDKProcess
	switch {
	case m.pythonBin != "":
		proc = NewPythonSDKProcess(m.basePort, m.pythonBin)
	case m.nodeBin != "":
		proc = NewNodeSDKProcess(m.basePort, m.nodeBin)
	default:
		return nil, fmt.Errorf("sdk: no runtime configured (use --sdk-python or --sdk-node)")
	}

	if err := proc.Start(ctx); err != nil {
		return nil, fmt.Errorf("sdk: start process: %w", err)
	}

	conn, err := grpc.NewClient(proc.Address(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		proc.Stop()
		return nil, fmt.Errorf("sdk: grpc dial: %w", err)
	}

	m.entry = &sdkEntry{
		proc:           proc,
		client:         proto.NewAgentServiceClient(conn),
		resourceClient: proto.NewResourceServiceClient(conn),
		conn:           conn,
	}
	return m.entry.client, nil
}

// GetOrStartResource returns a shared gRPC ResourceServiceClient.
func (m *sdkProcessManager) GetOrStartResource(ctx context.Context) (proto.ResourceServiceClient, error) {
	_, err := m.GetOrStart(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entry.resourceClient, nil
}

// StopAll shuts down the SDK process.
func (m *sdkProcessManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entry != nil {
		m.entry.conn.Close()
		m.entry.proc.Stop()
		m.entry = nil
	}
}

// SDKResult holds the artifact, emissions, and logs returned from an SDK execution.
type SDKResult struct {
	Artifact  *types.Artifact
	Emissions []*proto.EmitEvent
	Logs      []*proto.LogEntry
}

// executeResourceSDK calls the SDK resource process via gRPC.
func executeResourceSDK(ctx context.Context, client proto.ResourceServiceClient, resourceID, action string, params map[string]string) (string, error) {
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

// executeSDK calls the SDK agent process via gRPC and returns the result.
func executeSDK(ctx context.Context, client proto.AgentServiceClient, task sdk.Task) (*SDKResult, error) {
	req := &proto.TaskRequest{
		EventType:    task.Options["event_type"],
		EventPayload: task.Input.Payload,
		DeskId:       task.DeskID,
		AgentId:      task.AgentID,
	}

	resp, err := client.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sdk execute: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("sdk agent error: %s", resp.Error)
	}

	// Rewrite bare "done" event type to "{deskID}.done".
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

	return &SDKResult{
		Artifact:  &types.Artifact{Schema: "sdk-v1", Payload: payload},
		Emissions: resp.Emissions,
		Logs:      resp.Logs,
	}, nil
}

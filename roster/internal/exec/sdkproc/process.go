package sdkproc

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/roster-io/roster/pkg/types"
	"github.com/roster-io/roster/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Process manages an external SDK server process (Python or Node.js).
type Process interface {
	Start(ctx context.Context) error
	Stop() error
	Address() string
}

// NewPythonProcess creates a Python SDK process.
func NewPythonProcess(port int, pythonBin string) Process {
	return &pythonProcess{port: port, pythonBin: pythonBin}
}

// NewNodeProcess creates a Node.js SDK process.
func NewNodeProcess(port int, nodeBin string) Process {
	return &nodeProcess{port: port, nodeBin: nodeBin}
}

// --- Python ---

type pythonProcess struct {
	port      int
	pythonBin string
	cmd       *exec.Cmd
}

func (p *pythonProcess) Address() string {
	return fmt.Sprintf("localhost:%d", p.port)
}

func (p *pythonProcess) Start(ctx context.Context) error {
	p.cmd = exec.CommandContext(ctx, p.pythonBin, "-P", "-m", "roster",
		"--port", fmt.Sprintf("%d", p.port),
	)
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("python sdk start: %w", err)
	}
	return waitReady(p.Address())
}

func (p *pythonProcess) Stop() error {
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// --- Node.js ---

type nodeProcess struct {
	port    int
	nodeBin string
	cmd     *exec.Cmd
}

func (n *nodeProcess) Address() string {
	return fmt.Sprintf("localhost:%d", n.port)
}

func (n *nodeProcess) Start(ctx context.Context) error {
	n.cmd = exec.CommandContext(ctx, n.nodeBin, "-e", fmt.Sprintf(
		`require("roster-sdk").serve({ port: %d })`, n.port,
	))
	n.cmd.Stdout = os.Stdout
	n.cmd.Stderr = os.Stderr
	if err := n.cmd.Start(); err != nil {
		return fmt.Errorf("node sdk start: %w", err)
	}
	return waitReady(n.Address())
}

func (n *nodeProcess) Stop() error {
	if n.cmd != nil && n.cmd.Process != nil {
		return n.cmd.Process.Kill()
	}
	return nil
}

// waitReady polls addr until TCP connects or 15s elapses.
func waitReady(addr string) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("sdk at %s not ready within 15s", addr)
}

// --- ProcessManager ---

// ProcessManager manages a single shared SDK gRPC process.
// All agents are loaded by entry-point discovery inside that one process.
type ProcessManager struct {
	mu         sync.Mutex
	basePort   int
	pythonBin  string
	nodeBin    string
	projectDir string
	entry      *entry
}

type entry struct {
	proc           Process
	client         proto.AgentServiceClient
	resourceClient proto.ResourceServiceClient
	conn           *grpc.ClientConn
}

// NewProcessManager creates a ProcessManager using the given base port.
func NewProcessManager(basePort int) *ProcessManager {
	return &ProcessManager{basePort: basePort}
}

func (m *ProcessManager) SetPythonBin(bin string)  { m.pythonBin = bin }
func (m *ProcessManager) SetNodeBin(bin string)    { m.nodeBin = bin }
func (m *ProcessManager) SetProjectDir(dir string) { m.projectDir = dir }

// EnsureSDK installs missing SDK packages and auto-configures the runtime bin.
func (m *ProcessManager) EnsureSDK(ctx context.Context, agents map[string]*types.Agent, resources map[string]*types.Resource) error {
	refs := collectRefs(agents, resources)
	if len(refs) == 0 {
		return nil
	}

	for _, r := range refs {
		switch r.prefix {
		case "pip":
			pyBin := m.effectivePythonBin(r.pyVer)
			if err := pipInstall(ctx, pyBin, r.pkg); err != nil {
				return err
			}
			if m.pythonBin == "" {
				m.pythonBin = pyBin
			}
		case "npm":
			if err := npmInstall(ctx, r.pkg); err != nil {
				return err
			}
			if m.nodeBin == "" {
				m.nodeBin = resolveNodeBin()
			}
		case "git":
			pyBin := m.effectivePythonBin(r.pyVer)
			if err := gitInstall(ctx, pyBin, r.pkg, r.gitRef); err != nil {
				return err
			}
			if m.pythonBin == "" {
				m.pythonBin = pyBin
			}
		case "local":
			pyBin := m.effectivePythonBin(r.pyVer)
			localPath := resolvePath(m.projectDir, r.pkg)
			if err := localInstall(ctx, pyBin, localPath); err != nil {
				return err
			}
			if m.pythonBin == "" {
				m.pythonBin = pyBin
			}
		}
	}
	return nil
}

func (m *ProcessManager) effectivePythonBin(ver string) string {
	if ver == "" && m.pythonBin != "" {
		return m.pythonBin
	}
	return resolvePythonBin(ver)
}

// GetOrStart returns a shared gRPC AgentServiceClient, starting the process on first call.
func (m *ProcessManager) GetOrStart(ctx context.Context) (proto.AgentServiceClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.entry != nil {
		return m.entry.client, nil
	}

	var proc Process
	switch {
	case m.pythonBin != "":
		proc = NewPythonProcess(m.basePort, m.pythonBin)
	case m.nodeBin != "":
		proc = NewNodeProcess(m.basePort, m.nodeBin)
	default:
		return nil, fmt.Errorf("sdk: no runtime configured (set sdk: in agent/resource config)")
	}

	if err := proc.Start(ctx); err != nil {
		return nil, fmt.Errorf("sdk: start process: %w", err)
	}

	conn, err := grpc.NewClient(proc.Address(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		proc.Stop()
		return nil, fmt.Errorf("sdk: grpc dial: %w", err)
	}

	m.entry = &entry{
		proc:           proc,
		client:         proto.NewAgentServiceClient(conn),
		resourceClient: proto.NewResourceServiceClient(conn),
		conn:           conn,
	}
	return m.entry.client, nil
}

// GetOrStartResource returns a shared gRPC ResourceServiceClient.
func (m *ProcessManager) GetOrStartResource(ctx context.Context) (proto.ResourceServiceClient, error) {
	if _, err := m.GetOrStart(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entry.resourceClient, nil
}

// StopAll shuts down the SDK process and releases the gRPC connection.
func (m *ProcessManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entry != nil {
		m.entry.conn.Close()
		m.entry.proc.Stop()
		m.entry = nil
	}
}

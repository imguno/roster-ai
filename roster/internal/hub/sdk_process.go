package hub

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
)

// SDKProcess manages an external SDK server process (Python or Node.js).
type SDKProcess interface {
	Start(ctx context.Context) error
	Stop() error
	Address() string
}

// NewPythonSDKProcess creates a Python SDK process for the given module.
// pythonBin is the python executable (from --sdk-python flag).
// module is the Python module to load (from agent.sdk field).
func NewPythonSDKProcess(port int, pythonBin string) SDKProcess {
	return &PythonSDKProcess{port: port, pythonBin: pythonBin}
}

// NewNodeSDKProcess creates a Node.js SDK process for the given package.
// nodeBin is the node executable (from --sdk-node flag).
// pkg is the npm package to load (from agent.sdk field).
func NewNodeSDKProcess(port int, nodeBin string) SDKProcess {
	return &NodeSDKProcess{port: port, nodeBin: nodeBin}
}

// --- Python ---

type PythonSDKProcess struct {
	port      int
	pythonBin string
	cmd       *exec.Cmd
}

func (p *PythonSDKProcess) Address() string {
	return fmt.Sprintf("localhost:%d", p.port)
}

func (p *PythonSDKProcess) Start(ctx context.Context) error {
	p.cmd = exec.CommandContext(ctx, p.pythonBin, "-P", "-m", "roster",
		"--port", fmt.Sprintf("%d", p.port),
	)
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("python sdk start: %w", err)
	}
	return sdkWaitReady(p.Address())
}

func (p *PythonSDKProcess) Stop() error {
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// --- Node.js ---

type NodeSDKProcess struct {
	port    int
	nodeBin string
	cmd     *exec.Cmd
}

func (n *NodeSDKProcess) Address() string {
	return fmt.Sprintf("localhost:%d", n.port)
}

func (n *NodeSDKProcess) Start(ctx context.Context) error {
	n.cmd = exec.CommandContext(ctx, n.nodeBin, "-e", fmt.Sprintf(
		`require("roster-sdk").serve({ port: %d })`, n.port,
	))
	n.cmd.Stdout = os.Stdout
	n.cmd.Stderr = os.Stderr
	if err := n.cmd.Start(); err != nil {
		return fmt.Errorf("node sdk start: %w", err)
	}
	return sdkWaitReady(n.Address())
}

func (n *NodeSDKProcess) Stop() error {
	if n.cmd != nil && n.cmd.Process != nil {
		return n.cmd.Process.Kill()
	}
	return nil
}

// sdkWaitReady polls addr until TCP connects or 15s elapses.
func sdkWaitReady(addr string) error {
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

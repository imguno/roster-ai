package main

import (
	"context"
	"fmt"

	"github.com/roster-io/roster/internal/exec/desk"
	"github.com/roster-io/roster/internal/exec/runner"
	runnerapi "github.com/roster-io/roster/internal/exec/runner/api"
	"github.com/roster-io/roster/pkg/types"
)

func startWorker(ctx context.Context, addr string) error {
	reg := runner.NewRegistry()
	reg.Register(types.ExecutorTypeAPI, runnerapi.New())
	reg.Register(types.ExecutorTypeExec, runner.NewExecRunner())
	reg.Register(types.ExecutorTypeDocker, runner.NewDockerRunner())
	s := desk.NewServer(reg)
	fmt.Printf("roster worker listening on %s\n", addr)
	go func() {
		<-ctx.Done()
		s.Stop()
	}()
	return s.Listen(addr)
}

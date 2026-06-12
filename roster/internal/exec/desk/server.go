package desk

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"

	"github.com/roster-io/roster/pkg/sdk"
	"github.com/roster-io/roster/pkg/types"
	pb "github.com/roster-io/roster/proto"
)

// Dispatcher is the dispatch interface accepted by the worker server.
type Dispatcher interface {
	Dispatch(ctx context.Context, t types.ExecutorType, task sdk.Task) (*types.Output, error)
}

// Server runs Roster in worker mode, receiving tasks from the Hub via gRPC.
type Server struct {
	registry Dispatcher
	grpc     *grpc.Server
}

func NewServer(registry Dispatcher) *Server {
	s := &Server{
		registry: registry,
		grpc:     grpc.NewServer(),
	}
	pb.RegisterWorkerServer(s.grpc, &handler{registry: registry})
	return s
}

func (s *Server) Listen(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("desk: listen %s: %w", addr, err)
	}
	return s.grpc.Serve(lis)
}

func (s *Server) Stop() { s.grpc.GracefulStop() }

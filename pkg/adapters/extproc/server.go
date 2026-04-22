// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extproc

import (
	"fmt"
	"net"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/leseb/openresponses-gw/pkg/core/engine"
	"github.com/leseb/openresponses-gw/pkg/observability/logging"
)

// Server wraps the gRPC server for the ExtProc service.
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	logger     *logging.Logger
}

// NewServer creates a new ExtProc gRPC server.
func NewServer(eng *engine.Engine, logger *logging.Logger) *Server {
	gs := grpc.NewServer()
	processor := NewProcessor(eng, logger)
	extprocv3.RegisterExternalProcessorServer(gs, processor)

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(gs, healthSrv)
	healthSrv.SetServingStatus("envoy.service.ext_proc.v3.ExternalProcessor", healthpb.HealthCheckResponse_SERVING)

	return &Server{
		grpcServer: gs,
		logger:     logger,
	}
}

// Start begins listening on the given address. Blocks until stopped.
func (s *Server) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = lis
	s.logger.Info("ExtProc gRPC server listening", "address", addr)
	return s.grpcServer.Serve(lis)
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop() {
	s.logger.Info("Stopping ExtProc gRPC server")
	s.grpcServer.GracefulStop()
}

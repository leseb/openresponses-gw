package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	extproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/leseb/openai-responses-gateway/pkg/adapters/envoy"
	"github.com/leseb/openai-responses-gateway/pkg/core/config"
	"github.com/leseb/openai-responses-gateway/pkg/core/engine"
	"github.com/leseb/openai-responses-gateway/pkg/storage/memory"
)

func main() {
	// Parse flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	port := flag.Int("port", 10000, "gRPC port for ExtProc")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Setup structured logging
	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	logger.Info("starting envoy extproc server",
		"config_path", *configPath,
		"port", *port,
		"log_level", *logLevel,
	)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize in-memory session store
	store := memory.New()

	// Initialize engine
	eng, err := engine.New(&cfg.Engine, store)
	if err != nil {
		logger.Error("failed to create engine", "error", err)
		os.Exit(1)
	}

	// Create ExtProc processor
	processor := envoy.NewProcessor(eng, logger)

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Register ExtProc service
	extproc.RegisterExternalProcessorServer(grpcServer, processor)

	// Register health check service
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("envoy.service.ext_proc.v3.ExternalProcessor", healthpb.HealthCheckResponse_SERVING)

	// Create listener
	addr := fmt.Sprintf(":%d", *port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", "error", err, "addr", addr)
		os.Exit(1)
	}

	logger.Info("extproc server listening", "addr", addr)

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			errChan <- err
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		logger.Error("server error", "error", err)
		os.Exit(1)
	case sig := <-sigChan:
		logger.Info("received shutdown signal", "signal", sig)
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Info("shutting down gracefully")

	// Stop accepting new connections
	grpcServer.GracefulStop()

	// Wait for shutdown or timeout
	<-ctx.Done()
	logger.Info("server stopped")
}

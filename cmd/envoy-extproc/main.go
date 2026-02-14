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

	"github.com/leseb/openresponses-gw/pkg/adapters/envoy"
	httpAdapter "github.com/leseb/openresponses-gw/pkg/adapters/http"
	"github.com/leseb/openresponses-gw/pkg/core/config"
	"github.com/leseb/openresponses-gw/pkg/core/engine"
	"github.com/leseb/openresponses-gw/pkg/core/services"
	fsmemory "github.com/leseb/openresponses-gw/pkg/filestore/memory"
	"github.com/leseb/openresponses-gw/pkg/observability/logging"
	"github.com/leseb/openresponses-gw/pkg/storage/memory"
	"github.com/leseb/openresponses-gw/pkg/storage/sqlite"
	"github.com/leseb/openresponses-gw/pkg/vectorstore"
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

	slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(slogLogger)

	slogLogger.Info("starting envoy extproc server",
		"config_path", *configPath,
		"port", *port,
		"log_level", *logLevel,
	)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		slogLogger.Warn("failed to load config, using defaults", "error", err)
		cfg = config.Default()
	}

	// Initialize SQLite session store
	store, err := sqlite.New(cfg.SessionStore.DSN)
	if err != nil {
		slogLogger.Error("failed to initialize SQLite session store", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize stores (mirrors cmd/server/main.go)
	connectorsStore := memory.NewConnectorsStore()
	promptsStore := memory.NewPromptsStore()
	filesStore := fsmemory.New()
	defer filesStore.Close(context.Background())
	vectorStoresStore := memory.NewVectorStoresStore()

	// Initialize vector store service with in-memory backend (no embedding by default)
	vsBackend := vectorstore.NewMemoryBackend()
	vectorStoreService := services.NewVectorStoreService(filesStore, nil, vsBackend)

	// Initialize engine
	var vectorSearcher engine.VectorSearcher
	if vectorStoreService != nil {
		vectorSearcher = vectorStoreService
	}
	eng, err := engine.New(&cfg.Engine, store, connectorsStore, vectorSearcher)
	if err != nil {
		slogLogger.Error("failed to create engine", "error", err)
		os.Exit(1)
	}

	// Create HTTP handler for delegation
	logger := logging.New(logging.Config{
		Level:  *logLevel,
		Format: "json",
	})
	handler := httpAdapter.New(eng, logger, promptsStore, filesStore, vectorStoresStore, connectorsStore, vectorStoreService)

	// Create ExtProc processor
	processor := envoy.NewProcessor(handler, slogLogger)

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
		slogLogger.Error("failed to listen", "error", err, "addr", addr)
		os.Exit(1)
	}

	slogLogger.Info("extproc server listening", "addr", addr)

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
		slogLogger.Error("server error", "error", err)
		os.Exit(1)
	case sig := <-sigChan:
		slogLogger.Info("received shutdown signal", "signal", sig)
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slogLogger.Info("shutting down gracefully")

	// Stop accepting new connections
	grpcServer.GracefulStop()

	// Wait for shutdown or timeout
	<-ctx.Done()
	slogLogger.Info("server stopped")
}

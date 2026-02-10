// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpAdapter "github.com/leseb/openresponses-gw/pkg/adapters/http"
	"github.com/leseb/openresponses-gw/pkg/core/config"
	"github.com/leseb/openresponses-gw/pkg/core/engine"
	"github.com/leseb/openresponses-gw/pkg/core/services"
	"github.com/leseb/openresponses-gw/pkg/observability/logging"
	"github.com/leseb/openresponses-gw/pkg/storage/memory"
)

var (
	// Version is set via ldflags during build
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	port := flag.Int("port", 8080, "HTTP port to listen on")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	// Print version
	if *version {
		fmt.Printf("Open Responses Gateway Server\nVersion: %s\nBuild Time: %s\n", Version, BuildTime)
		os.Exit(0)
	}

	// Initialize logger
	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "json",
	})
	logger.Info("Starting Open Responses Gateway Server",
		"version", Version,
		"build_time", BuildTime)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		// If config file doesn't exist, use defaults
		logger.Warn("Failed to load config, using defaults", "error", err)
		cfg = config.Default()
	}

	// Override port if specified
	if *port != 8080 {
		cfg.Server.Port = *port
	}

	// Initialize storage (start with in-memory for Phase 1)
	store := memory.New()
	logger.Info("Initialized in-memory storage")

	// Initialize engine
	eng, err := engine.New(&cfg.Engine, store)
	if err != nil {
		logger.Error("Failed to initialize engine", "error", err)
		os.Exit(1)
	}
	logger.Info("Initialized engine")

	// Initialize services
	modelsService := services.NewModelsService(eng.LLMClient())
	logger.Info("Initialized models service")

	// Initialize prompts store
	promptsStore := memory.NewPromptsStore()
	logger.Info("Initialized prompts store")

	// Initialize files store
	filesStore := memory.NewFilesStore()
	logger.Info("Initialized files store")

	// Initialize vector stores store
	vectorStoresStore := memory.NewVectorStoresStore()
	logger.Info("Initialized vector stores store")

	// Initialize HTTP adapter
	handler := httpAdapter.New(eng, logger, modelsService, promptsStore, filesStore, vectorStoresStore)
	logger.Info("Initialized HTTP adapter")

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  cfg.Server.Timeout,
		WriteTimeout: cfg.Server.Timeout,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server in goroutine
	go func() {
		logger.Info("Server listening", "address", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	<-ctx.Done()
	logger.Info("Shutdown signal received")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", "error", err)
		os.Exit(1)
	}

	logger.Info("Server stopped gracefully")
}

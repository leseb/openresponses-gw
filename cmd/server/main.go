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
	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/core/config"
	"github.com/leseb/openresponses-gw/pkg/core/engine"
	"github.com/leseb/openresponses-gw/pkg/core/services"
	"github.com/leseb/openresponses-gw/pkg/filestore"
	"github.com/leseb/openresponses-gw/pkg/filestore/filesystem"
	fsmemory "github.com/leseb/openresponses-gw/pkg/filestore/memory"
	fss3 "github.com/leseb/openresponses-gw/pkg/filestore/s3"
	"github.com/leseb/openresponses-gw/pkg/observability/logging"
	"github.com/leseb/openresponses-gw/pkg/storage/memory"
	"github.com/leseb/openresponses-gw/pkg/vectorstore"
	milvusbackend "github.com/leseb/openresponses-gw/pkg/vectorstore/milvus"
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

	// Initialize connectors store (needed by engine for MCP tool support)
	connectorsStore := memory.NewConnectorsStore()
	logger.Info("Initialized connectors store")

	// Initialize prompts store
	promptsStore := memory.NewPromptsStore()
	logger.Info("Initialized prompts store")

	// Initialize files store
	var filesStore filestore.FileStore
	switch cfg.FileStore.Type {
	case "filesystem":
		fs, fsErr := filesystem.New(cfg.FileStore.BaseDir)
		if fsErr != nil {
			logger.Error("Failed to initialize filesystem file store", "error", fsErr)
			os.Exit(1)
		}
		filesStore = fs
		logger.Info("Initialized filesystem file store", "base_dir", cfg.FileStore.BaseDir)
	case "s3":
		s3Store, s3Err := fss3.New(context.Background(), fss3.Options{
			Bucket:   cfg.FileStore.S3Bucket,
			Region:   cfg.FileStore.S3Region,
			Prefix:   cfg.FileStore.S3Prefix,
			Endpoint: cfg.FileStore.S3Endpoint,
		})
		if s3Err != nil {
			logger.Error("Failed to initialize S3 file store", "error", s3Err)
			os.Exit(1)
		}
		filesStore = s3Store
		logger.Info("Initialized S3 file store", "bucket", cfg.FileStore.S3Bucket)
	default:
		filesStore = fsmemory.New()
		logger.Info("Initialized in-memory file store")
	}
	defer filesStore.Close(context.Background())

	// Initialize vector stores store
	vectorStoresStore := memory.NewVectorStoresStore()
	logger.Info("Initialized vector stores store")

	// Initialize embedding client (optional)
	var embedder api.EmbeddingClient
	if cfg.Embedding.Endpoint != "" {
		embedder = api.NewOpenAIEmbeddingClient(
			cfg.Embedding.Endpoint,
			cfg.Embedding.APIKey,
			cfg.Embedding.Model,
			cfg.Embedding.Dimensions,
		)
		logger.Info("Initialized embedding client", "endpoint", cfg.Embedding.Endpoint, "model", cfg.Embedding.Model)
	}

	// Initialize vector store backend
	initCtx := context.Background()
	var vsBackend vectorstore.Backend
	switch cfg.VectorStore.Type {
	case "milvus":
		mb, err := milvusbackend.NewBackend(initCtx, cfg.VectorStore.MilvusAddress)
		if err != nil {
			logger.Error("Failed to connect to Milvus", "error", err)
			os.Exit(1)
		}
		defer mb.Close(context.Background())
		vsBackend = mb
		logger.Info("Initialized Milvus vector store backend", "address", cfg.VectorStore.MilvusAddress)
	default:
		vsBackend = vectorstore.NewMemoryBackend()
		logger.Info("Initialized memory vector store backend")
	}

	// Initialize vector store service (nil if embedding not configured)
	vectorStoreService := services.NewVectorStoreService(filesStore, embedder, vsBackend)
	if vectorStoreService != nil {
		logger.Info("Initialized vector store service")
	}

	// Initialize engine (pass vectorStoreService as VectorSearcher)
	var vectorSearcher engine.VectorSearcher
	if vectorStoreService != nil {
		vectorSearcher = vectorStoreService
	}
	eng, err := engine.New(&cfg.Engine, store, connectorsStore, vectorSearcher)
	if err != nil {
		logger.Error("Failed to initialize engine", "error", err)
		os.Exit(1)
	}
	logger.Info("Initialized engine")

	// Initialize services
	modelsService := services.NewModelsService()
	logger.Info("Initialized models service")

	// Initialize HTTP adapter
	handler := httpAdapter.New(eng, logger, modelsService, promptsStore, filesStore, vectorStoresStore, connectorsStore, vectorStoreService)
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

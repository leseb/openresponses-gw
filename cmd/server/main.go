// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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
	"github.com/leseb/openresponses-gw/pkg/core/state"
	"github.com/leseb/openresponses-gw/pkg/filestore"
	"github.com/leseb/openresponses-gw/pkg/observability/logging"
	"github.com/leseb/openresponses-gw/pkg/storage/memory"
	"github.com/leseb/openresponses-gw/pkg/vectorstore"
	"github.com/leseb/openresponses-gw/pkg/websearch"

	// Blank imports register provider implementations via init().
	// Remove any of these to exclude the provider from the binary.
	_ "github.com/leseb/openresponses-gw/pkg/filestore/filesystem"
	_ "github.com/leseb/openresponses-gw/pkg/filestore/memory"
	_ "github.com/leseb/openresponses-gw/pkg/filestore/s3"
	_ "github.com/leseb/openresponses-gw/pkg/storage/postgres"
	_ "github.com/leseb/openresponses-gw/pkg/storage/sqlite"
	_ "github.com/leseb/openresponses-gw/pkg/vectorstore/milvus"
)

var (
	// Version is set via ldflags during build
	Version   = "dev"
	BuildTime = "unknown"
)

// @title						Open Responses Gateway API
// @version					1.0.0
// @description				100% Open Responses Specification Compliant Gateway.
// @description				Based on: https://github.com/openresponses/openresponses
// @description
// @description				This gateway provides:
// @description				- **Core API**: Full Open Responses spec compliance (POST /v1/responses)
// @description				- **Extended APIs**: Conversations, Prompts, Files, Vector Stores, Connectors
// @description
// @description				Streaming: All 24 event types from Open Responses spec
// @description				Request Echo: All request parameters returned in response
// @description				Multimodal: Support for text, images, files, video
//
// @contact.name				Open Responses Gateway
// @contact.url				https://github.com/leseb/openresponses-gw
//
// @servers.url				http://localhost:8080
// @servers.description		Local development server
//
// @tag.name					Health
// @tag.description			Health check and API documentation
// @tag.name					Responses
// @tag.description			Open Responses API (100% spec compliant)
// @tag.name					Conversations
// @tag.description			Extended - Conversation state management
// @tag.name					Prompts
// @tag.description			Extended - Prompt template management
// @tag.name					Files
// @tag.description			Extended - File upload and management
// @tag.name					Vector Stores
// @tag.description			Extended - Vector store and embeddings
// @tag.name					Connectors
// @tag.description			Extended - MCP connector management
func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	port := flag.Int("port", 0, "HTTP port to listen on (overrides config)")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	// Print version
	if *version {
		fmt.Printf("Open Responses Gateway\nVersion: %s\nBuild Time: %s\n", Version, BuildTime)
		os.Exit(0)
	}

	// Initialize logger
	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "json",
	})
	logger.Info("Starting Open Responses Gateway",
		"version", Version,
		"build_time", BuildTime)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		// If config file doesn't exist, use defaults
		logger.Warn("Failed to load config, using defaults", "error", err)
		cfg = config.Default()
	}

	// Override ports from flags
	if *port != 0 {
		cfg.Server.Port = *port
	}
	// Initialize session store via provider registry
	initCtx := context.Background()
	store, err := state.Providers.New(initCtx, cfg.SessionStore.Type, map[string]string{
		"dsn": cfg.SessionStore.DSN,
	})
	if err != nil {
		logger.Error("Failed to initialize session store", "error", err)
		os.Exit(1)
	}
	if closer, ok := store.(io.Closer); ok {
		defer closer.Close()
	}
	logger.Info("Initialized session store", "type", cfg.SessionStore.Type)

	// Initialize connectors store (needed by engine for MCP tool support)
	connectorsStore := memory.NewConnectorsStore()
	logger.Info("Initialized connectors store")

	// Initialize prompts store
	promptsStore := memory.NewPromptsStore()
	logger.Info("Initialized prompts store")

	// Initialize files store via provider registry
	filesStore, err := filestore.Providers.New(initCtx, cfg.FileStore.Type, map[string]string{
		"base_dir": cfg.FileStore.BaseDir,
		"bucket":   cfg.FileStore.S3Bucket,
		"region":   cfg.FileStore.S3Region,
		"prefix":   cfg.FileStore.S3Prefix,
		"endpoint": cfg.FileStore.S3Endpoint,
	})
	if err != nil {
		logger.Error("Failed to initialize file store", "error", err)
		os.Exit(1)
	}
	defer filesStore.Close(context.Background())
	logger.Info("Initialized file store", "type", cfg.FileStore.Type)

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

	// Initialize vector store backend via provider registry
	vsBackend, err := vectorstore.Providers.New(initCtx, cfg.VectorStore.Type, map[string]string{
		"address": cfg.VectorStore.MilvusAddress,
	})
	if err != nil {
		logger.Error("Failed to initialize vector store backend", "error", err)
		os.Exit(1)
	}
	defer vsBackend.Close(context.Background())
	logger.Info("Initialized vector store backend", "type", cfg.VectorStore.Type)

	// Initialize vector store service (nil if embedding not configured)
	vectorStoreService := services.NewVectorStoreService(filesStore, embedder, vsBackend)
	if vectorStoreService != nil {
		logger.Info("Initialized vector store service")
	}

	// Initialize web search provider via registry (optional)
	var webSearchProvider engine.WebSearcher
	if cfg.WebSearch.Provider != "" && cfg.WebSearch.APIKey != "" {
		wsProvider, wsErr := websearch.Providers.New(initCtx, cfg.WebSearch.Provider, map[string]string{
			"api_key": cfg.WebSearch.APIKey,
		})
		if wsErr != nil {
			logger.Error("Failed to initialize web search provider", "error", wsErr)
			os.Exit(1)
		}
		webSearchProvider = &webSearchAdapter{provider: wsProvider}
		logger.Info("Initialized web search provider", "provider", cfg.WebSearch.Provider)
	}

	// Initialize engine (pass vectorStoreService as VectorSearcher)
	var vectorSearcher engine.VectorSearcher
	if vectorStoreService != nil {
		vectorSearcher = vectorStoreService
	}
	eng, err := engine.New(&cfg.Engine, store, connectorsStore, vectorSearcher, webSearchProvider)
	if err != nil {
		logger.Error("Failed to initialize engine", "error", err)
		os.Exit(1)
	}
	logger.Info("Initialized engine")

	// Initialize HTTP adapter
	handler := httpAdapter.New(eng, logger, promptsStore, filesStore, vectorStoresStore, connectorsStore, vectorStoreService)
	logger.Info("Initialized HTTP adapter")

	// Create HTTP server
	httpAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         httpAddr,
		Handler:      handler,
		ReadTimeout:  cfg.Server.Timeout,
		WriteTimeout: cfg.Server.Timeout,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start HTTP server
	go func() {
		logger.Info("HTTP server listening", "address", httpAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
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
		logger.Error("HTTP server shutdown error", "error", err)
	}

	logger.Info("Server stopped gracefully")
}

// webSearchAdapter adapts websearch.Provider to engine.WebSearcher.
type webSearchAdapter struct {
	provider websearch.Provider
}

func (a *webSearchAdapter) Search(ctx context.Context, query string, maxResults int) ([]engine.WebSearchResult, error) {
	results, err := a.provider.Search(ctx, query, maxResults)
	if err != nil {
		return nil, err
	}
	out := make([]engine.WebSearchResult, len(results))
	for i, r := range results {
		out[i] = engine.WebSearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Snippet,
		}
	}
	return out, nil
}

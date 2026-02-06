# OpenAI Responses API Gateway - Project Plan

**Version:** 1.0
**Date:** February 6, 2026
**Status:** Planning Phase

## Table of Contents

- [Executive Summary](#executive-summary)
- [High-Level Architecture](#high-level-architecture)
- [Project Structure](#project-structure)
- [Core Design Principles](#core-design-principles)
- [Implementation Phases](#implementation-phases)
- [Key Design Decisions](#key-design-decisions)
- [Technology Stack](#technology-stack)
- [Getting Started](#getting-started)

## Executive Summary

This project aims to build a production-ready implementation of the OpenAI Responses API with a **gateway-agnostic architecture**. The system will support multiple deployment modes (standalone HTTP server, Envoy ExtProc, Kong plugin, etc.) while maintaining a clean separation between core business logic and gateway-specific adapters.

### Key Features

- **Gateway-Agnostic Core**: 80% of code is reusable across different gateways
- **Stateful API**: Full support for conversations, sessions, and response history
- **Tool Execution**: Built-in tools (file search, web search, code interpreter) + custom tools (functions, MCP)
- **Streaming Support**: Server-Sent Events (SSE) for real-time responses
- **Production-Ready**: Observability, security, performance optimization built-in

### System Components

```
┌─────────────────────────────────────────────────────────────────┐
│ Responses API                                                    │
│   - Multi-turn conversations                                     │
│   - Tool execution (file search, web search, code, MCP)         │
│   - Streaming & non-streaming modes                             │
└──────────────┬──────────────────────────────────────────────────┘
               │
┌──────────────▼──────────────────────────────────────────────────┐
│ Supporting APIs                                                  │
│   - Conversations API: Message history & state                   │
│   - Files API: Upload, storage, retrieval                       │
│   - Vector Stores API: Embeddings & search                      │
│   - Search API: Semantic & keyword search                       │
│   - Prompts API: Reusable prompt templates                      │
│   - File Processor API: Document processing pipeline            │
└─────────────────────────────────────────────────────────────────┘
```

## High-Level Architecture

### Layered Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Gateway Layer                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ Envoy ExtProc│  │ Kong Plugin  │  │ HTTP Server  │  ...     │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
│         └──────────────────┴──────────────────┘                  │
└─────────────────────────┬───────────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────────┐
│                    Adapter Layer (HTTP Interface)                │
│  - Request/Response translation                                  │
│  - Gateway-specific protocol handling                            │
│  - Streaming coordination                                        │
└─────────────────────────┬───────────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────────┐
│                      Core Engine                                 │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐    │
│  │ Responses API  │  │ Session Mgr    │  │ Tool Executor  │    │
│  │   Handler      │  │  - State       │  │  - Registry    │    │
│  │   - Sync       │  │  - Cache       │  │  - Built-in    │    │
│  │   - Streaming  │  │  - History     │  │  - Custom      │    │
│  └────────┬───────┘  └────────┬───────┘  └────────┬───────┘    │
│           └──────────────────┬┴──────────────────┬─┘            │
└───────────────────────────┬──┴───────────────────┴──────────────┘
                            │
┌───────────────────────────▼──────────────────────────────────────┐
│                   Supporting Services Layer                       │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │Files API │ │Convs API │ │Vector API│ │Search API│           │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │
│  ┌──────────┐ ┌──────────┐                                      │
│  │Prompts   │ │Processor │                                      │
│  │API       │ │API       │                                      │
│  └──────────┘ └──────────┘                                      │
└───────────────────────────┬──────────────────────────────────────┘
                            │
┌───────────────────────────▼──────────────────────────────────────┐
│                      Storage Layer                                │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                         │
│  │PostgreSQL│ │  Redis   │ │   S3     │  (pluggable)            │
│  │- Sessions│ │- Cache   │ │- Files   │                         │
│  │- Convs   │ │- Hot     │ │- Vectors │                         │
│  │- Responses│ │  State   │ │          │                         │
│  └──────────┘ └──────────┘ └──────────┘                         │
└──────────────────────────────────────────────────────────────────┘
```

### Data Flow: Request Processing

```
Client Request (HTTP)
    │
    ▼
┌───────────────────────┐
│ Gateway Adapter       │ ◄── Envoy/Kong/HTTP
│ - Parse request       │
│ - Extract headers     │
└───────┬───────────────┘
        │
        ▼
┌───────────────────────┐
│ Responses Engine      │
│ 1. Validate request   │
│ 2. Load/create session│
│ 3. Get conversation   │ ──► Conversations API
│ 4. Load prompts       │ ──► Prompts API
│ 5. Prepare LLM req    │
│ 6. Execute tools      │ ──► Tool Registry
│    - File search      │ ──► Files API + Vector API + Search API
│    - Web search       │ ──► Search API
│    - Code interpreter │ ──► Sandbox
│    - Functions/MCP    │ ──► Custom tools
│ 7. Call LLM           │ ──► OpenAI/Compatible API
│ 8. Process response   │
│ 9. Save state         │ ──► Session Store
└───────┬───────────────┘
        │
        ▼
┌───────────────────────┐
│ Gateway Adapter       │
│ - Format response     │
│ - Set headers         │
│ - Handle streaming    │ ──► SSE stream to client
└───────────────────────┘
```

## Project Structure

```
openai-responses-gateway/
├── README.md
├── PROJECT_PLAN.md          # This file
├── ARCHITECTURE.md          # Detailed architecture docs
├── go.mod
├── go.sum
├── Makefile
├── .gitignore
├── .golangci.yml           # Linter configuration
│
├── cmd/
│   ├── server/                    # Standalone HTTP server
│   │   ├── main.go
│   │   └── config.yaml.example
│   │
│   ├── envoy-extproc/             # Envoy External Processor
│   │   ├── main.go
│   │   └── extproc-config.yaml.example
│   │
│   └── cli/                       # Admin CLI tools
│       ├── main.go
│       └── commands/
│           ├── migrate.go         # DB migrations
│           ├── seed.go            # Seed test data
│           └── version.go         # Version info
│
├── pkg/
│   ├── core/                      # Gateway-agnostic core
│   │   │
│   │   ├── engine/                # Main orchestration engine
│   │   │   ├── engine.go          # Engine interface
│   │   │   ├── responses.go       # Responses API handler
│   │   │   ├── responses_stream.go # Streaming handler
│   │   │   ├── session.go         # Session management
│   │   │   ├── executor.go        # Tool execution coordinator
│   │   │   └── llm.go             # LLM client abstraction
│   │   │
│   │   ├── api/                   # OpenAI API clients
│   │   │   ├── client.go          # Base HTTP client
│   │   │   ├── conversations.go   # Conversations API
│   │   │   ├── files.go           # Files API
│   │   │   ├── vectors.go         # Vector Stores API
│   │   │   ├── search.go          # Search API
│   │   │   ├── prompts.go         # Prompts API
│   │   │   └── processor.go       # File Processor API
│   │   │
│   │   ├── tools/                 # Tool execution framework
│   │   │   ├── registry.go        # Tool registry
│   │   │   ├── executor.go        # Tool executor interface
│   │   │   ├── result.go          # Tool result types
│   │   │   │
│   │   │   ├── builtin/           # Built-in tools
│   │   │   │   ├── file_search.go
│   │   │   │   ├── web_search.go
│   │   │   │   ├── code_interpreter.go
│   │   │   │   └── image_generation.go
│   │   │   │
│   │   │   ├── custom/            # Custom tool support
│   │   │   │   ├── function.go    # Function calling
│   │   │   │   └── mcp.go         # MCP connector
│   │   │   │
│   │   │   └── computer/          # Computer use tools
│   │   │       ├── shell.go
│   │   │       ├── patch.go
│   │   │       └── desktop.go
│   │   │
│   │   ├── schema/                # OpenAI API schemas
│   │   │   ├── responses.go       # Response types
│   │   │   ├── conversations.go   # Conversation types
│   │   │   ├── files.go           # File types
│   │   │   ├── vectors.go         # Vector types
│   │   │   ├── tools.go           # Tool types
│   │   │   └── validation.go      # Request validation
│   │   │
│   │   ├── state/                 # State management
│   │   │   ├── store.go           # Storage interface
│   │   │   ├── session.go         # Session state
│   │   │   ├── conversation.go    # Conversation state
│   │   │   ├── response.go        # Response state
│   │   │   └── cache.go           # Caching layer
│   │   │
│   │   └── config/                # Configuration
│   │       ├── config.go          # Main config
│   │       ├── backends.go        # Backend config
│   │       └── validation.go      # Config validation
│   │
│   ├── adapters/                  # Gateway-specific adapters
│   │   ├── interface.go           # Adapter interface
│   │   │
│   │   ├── http/                  # Standard HTTP adapter
│   │   │   ├── handler.go         # HTTP handler
│   │   │   ├── middleware.go      # Middleware chain
│   │   │   ├── router.go          # Route setup
│   │   │   ├── streaming.go       # SSE handler
│   │   │   └── errors.go          # Error responses
│   │   │
│   │   ├── envoy/                 # Envoy ExtProc adapter
│   │   │   ├── processor.go       # ExtProc implementation
│   │   │   ├── grpc.go            # gRPC service
│   │   │   ├── streaming.go       # SSE handling for ExtProc
│   │   │   └── proto/             # ExtProc proto definitions
│   │   │
│   │   └── kong/                  # Kong plugin adapter
│   │       ├── plugin.go          # Kong plugin interface
│   │       ├── lifecycle.go       # Plugin lifecycle
│   │       └── schema.lua         # Kong schema
│   │
│   ├── storage/                   # Storage implementations
│   │   ├── interface.go           # Storage interface
│   │   │
│   │   ├── postgres/              # PostgreSQL impl
│   │   │   ├── postgres.go        # Connection setup
│   │   │   ├── sessions.go        # Session queries
│   │   │   ├── conversations.go   # Conversation queries
│   │   │   ├── responses.go       # Response queries
│   │   │   ├── files.go           # File metadata queries
│   │   │   └── migrations/        # SQL migrations
│   │   │       ├── 001_init.sql
│   │   │       ├── 002_conversations.sql
│   │   │       └── ...
│   │   │
│   │   ├── redis/                 # Redis impl (cache)
│   │   │   ├── redis.go           # Connection setup
│   │   │   ├── cache.go           # Cache operations
│   │   │   └── sessions.go        # Hot session cache
│   │   │
│   │   └── memory/                # In-memory impl (dev)
│   │       └── memory.go          # In-memory store
│   │
│   ├── streaming/                 # SSE streaming support
│   │   ├── sse.go                 # SSE encoder/decoder
│   │   ├── buffer.go              # Response buffering
│   │   ├── events.go              # Event types
│   │   └── writer.go              # Stream writer
│   │
│   ├── observability/             # Observability
│   │   ├── metrics/               # Prometheus metrics
│   │   │   ├── metrics.go
│   │   │   ├── responses.go       # Response-specific metrics
│   │   │   └── tools.go           # Tool execution metrics
│   │   │
│   │   ├── tracing/               # OpenTelemetry tracing
│   │   │   ├── tracer.go
│   │   │   ├── spans.go
│   │   │   └── attributes.go
│   │   │
│   │   └── logging/               # Structured logging
│   │       ├── logger.go
│   │       └── context.go         # Context-aware logging
│   │
│   └── client/                    # Client SDK (optional)
│       ├── client.go              # HTTP client
│       ├── responses.go           # Responses API client
│       ├── conversations.go       # Conversations client
│       └── files.go               # Files client
│
├── internal/                      # Internal packages
│   ├── testutil/                  # Test utilities
│   │   ├── fixtures.go            # Test fixtures
│   │   └── assert.go              # Custom assertions
│   │
│   └── mocks/                     # Mock implementations
│       ├── storage.go             # Mock storage
│       ├── llm.go                 # Mock LLM
│       └── tools.go               # Mock tools
│
├── examples/
│   ├── standalone/                # Standalone server example
│   │   ├── README.md
│   │   ├── config.yaml
│   │   ├── docker-compose.yaml
│   │   └── requests.sh            # Example curl requests
│   │
│   ├── envoy/                     # Envoy deployment
│   │   ├── README.md
│   │   ├── envoy.yaml
│   │   ├── extproc-config.yaml
│   │   └── docker-compose.yaml
│   │
│   └── kubernetes/                # K8s manifests
│       ├── README.md
│       ├── namespace.yaml
│       ├── configmap.yaml
│       ├── deployment.yaml
│       ├── service.yaml
│       └── ingress.yaml
│
├── deployments/
│   ├── docker/
│   │   ├── Dockerfile
│   │   ├── Dockerfile.envoy-extproc
│   │   └── .dockerignore
│   │
│   └── helm/                      # Helm chart
│       └── responses-gateway/
│           ├── Chart.yaml
│           ├── values.yaml
│           ├── templates/
│           │   ├── deployment.yaml
│           │   ├── service.yaml
│           │   ├── configmap.yaml
│           │   └── ingress.yaml
│           └── README.md
│
└── tests/
    ├── integration/               # Integration tests
    │   ├── responses_test.go
    │   ├── conversations_test.go
    │   └── tools_test.go
    │
    ├── e2e/                       # End-to-end tests
    │   ├── envoy_test.go
    │   └── http_test.go
    │
    └── fixtures/                  # Test fixtures
        ├── requests/
        └── responses/
```

## Core Design Principles

### 1. Gateway-Agnostic Core

**All business logic lives in `pkg/core/` with zero gateway dependencies.**

```go
// pkg/core/engine/responses.go
package engine

import (
    "context"
    "github.com/yourorg/responses-gateway/pkg/core/schema"
    "github.com/yourorg/responses-gateway/pkg/core/state"
    "github.com/yourorg/responses-gateway/pkg/core/api"
    "github.com/yourorg/responses-gateway/pkg/core/tools"
)

type ResponsesEngine struct {
    sessions      state.SessionStore
    conversations api.ConversationsAPI
    files         api.FilesAPI
    vectors       api.VectorStoresAPI
    search        api.SearchAPI
    prompts       api.PromptsAPI
    tools         *tools.Registry
    llm           LLMClient
}

// ProcessRequest handles a Responses API request (gateway-agnostic)
func (e *ResponsesEngine) ProcessRequest(
    ctx context.Context,
    req *schema.ResponseRequest,
) (*schema.Response, error) {
    // Pure business logic - no HTTP, no gRPC, no gateway concerns
    // 1. Validate
    // 2. Load state
    // 3. Execute tools
    // 4. Call LLM
    // 5. Save state
    // 6. Return response
}

// ProcessRequestStream handles streaming requests
func (e *ResponsesEngine) ProcessRequestStream(
    ctx context.Context,
    req *schema.ResponseRequest,
) (<-chan *schema.ResponseStreamEvent, error) {
    // Streaming logic - returns a channel
    // Gateway adapters handle the actual SSE/gRPC streaming
}
```

### 2. Adapter Pattern for Gateways

**Each gateway implements a common interface:**

```go
// pkg/adapters/interface.go
package adapters

import "context"

type Adapter interface {
    // Serve starts the adapter
    Serve(ctx context.Context) error

    // Health check
    Health(ctx context.Context) error

    // Shutdown gracefully
    Shutdown(ctx context.Context) error
}

type RequestAdapter interface {
    // Extract request from gateway-specific format
    ExtractRequest(gatewayReq interface{}) (*schema.ResponseRequest, error)

    // Send response in gateway-specific format
    SendResponse(gatewayResp interface{}, resp *schema.Response) error
}
```

**Example HTTP Adapter:**

```go
// pkg/adapters/http/handler.go
package http

import (
    "encoding/json"
    "net/http"

    "github.com/yourorg/responses-gateway/pkg/core/engine"
    "github.com/yourorg/responses-gateway/pkg/core/schema"
)

type Handler struct {
    engine *engine.ResponsesEngine
}

func New(engine *engine.ResponsesEngine) *Handler {
    return &Handler{engine: engine}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request
    var req schema.ResponseRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // 2. Determine if streaming
    if req.Stream {
        h.handleStream(w, r, &req)
        return
    }

    // 3. Process through engine
    resp, err := h.engine.ProcessRequest(r.Context(), &req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // 4. Send response
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request, req *schema.ResponseRequest) {
    // SSE streaming implementation
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming not supported", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    events, err := h.engine.ProcessRequestStream(r.Context(), req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    for event := range events {
        data, _ := json.Marshal(event)
        fmt.Fprintf(w, "data: %s\n\n", data)
        flusher.Flush()
    }
}
```

**Example Envoy ExtProc Adapter:**

```go
// pkg/adapters/envoy/processor.go
package envoy

import (
    extproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

    "github.com/yourorg/responses-gateway/pkg/core/engine"
)

type Processor struct {
    engine *engine.ResponsesEngine
}

func New(engine *engine.ResponsesEngine) *Processor {
    return &Processor{engine: engine}
}

func (p *Processor) Process(
    srv extproc.ExternalProcessor_ProcessServer,
) error {
    // 1. Receive request from Envoy
    req, err := srv.Recv()
    if err != nil {
        return err
    }

    // 2. Extract body
    body := extractBody(req)

    // 3. Parse to ResponseRequest
    var respReq schema.ResponseRequest
    json.Unmarshal(body, &respReq)

    // 4. Process through engine
    resp, err := p.engine.ProcessRequest(srv.Context(), &respReq)
    if err != nil {
        return err
    }

    // 5. Send back to Envoy
    respBody, _ := json.Marshal(resp)
    return srv.Send(createExtProcResponse(respBody))
}
```

### 3. Stateful Session Management

**Session storage abstraction:**

```go
// pkg/core/state/store.go
package state

import (
    "context"
    "time"
)

type SessionStore interface {
    // Session lifecycle
    CreateSession(ctx context.Context, session *Session) error
    GetSession(ctx context.Context, sessionID string) (*Session, error)
    UpdateSession(ctx context.Context, session *Session) error
    DeleteSession(ctx context.Context, sessionID string) error

    // Conversation management
    GetConversation(ctx context.Context, conversationID string) (*Conversation, error)
    SaveConversation(ctx context.Context, conv *Conversation) error
    ListConversations(ctx context.Context, sessionID string) ([]*Conversation, error)

    // Response history
    GetResponse(ctx context.Context, responseID string) (*Response, error)
    SaveResponse(ctx context.Context, resp *Response) error
    ListResponses(ctx context.Context, conversationID string) ([]*Response, error)
    LinkResponses(ctx context.Context, currentID, previousID string) error

    // File management
    SaveFile(ctx context.Context, file *File) error
    GetFile(ctx context.Context, fileID string) (*File, error)
    DeleteFile(ctx context.Context, fileID string) error
}

type Session struct {
    ID             string
    ConversationID string
    State          map[string]interface{}
    Metadata       map[string]string
    CreatedAt      time.Time
    UpdatedAt      time.Time
    ExpiresAt      time.Time
}

type Conversation struct {
    ID        string
    SessionID string
    Messages  []Message
    Metadata  map[string]string
    CreatedAt time.Time
    UpdatedAt time.Time
}

type Response struct {
    ID                string
    ConversationID    string
    PreviousResponseID string
    Request           interface{}
    Output            interface{}
    Status            string
    Error             *ErrorDetails
    Usage             *UsageInfo
    CreatedAt         time.Time
    CompletedAt       *time.Time
}
```

### 4. Tool Execution Framework

**Tool registry and execution:**

```go
// pkg/core/tools/registry.go
package tools

import "context"

type Tool interface {
    Name() string
    Description() string
    Parameters() Schema
    Execute(ctx context.Context, params map[string]interface{}) (*Result, error)
}

type Schema struct {
    Type       string                 `json:"type"`
    Properties map[string]Property    `json:"properties"`
    Required   []string               `json:"required"`
}

type Property struct {
    Type        string `json:"type"`
    Description string `json:"description"`
}

type Result struct {
    Success bool
    Data    interface{}
    Error   error
}

type Registry struct {
    tools map[string]Tool
}

func NewRegistry() *Registry {
    return &Registry{
        tools: make(map[string]Tool),
    }
}

func (r *Registry) Register(tool Tool) error {
    if _, exists := r.tools[tool.Name()]; exists {
        return fmt.Errorf("tool %s already registered", tool.Name())
    }
    r.tools[tool.Name()] = tool
    return nil
}

func (r *Registry) Get(name string) (Tool, error) {
    tool, exists := r.tools[name]
    if !exists {
        return nil, fmt.Errorf("tool %s not found", name)
    }
    return tool, nil
}

func (r *Registry) Execute(ctx context.Context, name string, params map[string]interface{}) (*Result, error) {
    tool, err := r.Get(name)
    if err != nil {
        return nil, err
    }
    return tool.Execute(ctx, params)
}

func (r *Registry) List() []string {
    names := make([]string, 0, len(r.tools))
    for name := range r.tools {
        names = append(names, name)
    }
    return names
}
```

**Example built-in tool:**

```go
// pkg/core/tools/builtin/file_search.go
package builtin

import (
    "context"

    "github.com/yourorg/responses-gateway/pkg/core/api"
    "github.com/yourorg/responses-gateway/pkg/core/tools"
)

type FileSearchTool struct {
    filesAPI  api.FilesAPI
    vectorAPI api.VectorStoresAPI
    searchAPI api.SearchAPI
}

func NewFileSearchTool(
    filesAPI api.FilesAPI,
    vectorAPI api.VectorStoresAPI,
    searchAPI api.SearchAPI,
) *FileSearchTool {
    return &FileSearchTool{
        filesAPI:  filesAPI,
        vectorAPI: vectorAPI,
        searchAPI: searchAPI,
    }
}

func (t *FileSearchTool) Name() string {
    return "file_search"
}

func (t *FileSearchTool) Description() string {
    return "Search through uploaded files using semantic search"
}

func (t *FileSearchTool) Parameters() tools.Schema {
    return tools.Schema{
        Type: "object",
        Properties: map[string]tools.Property{
            "query": {
                Type:        "string",
                Description: "Search query",
            },
            "vector_store_id": {
                Type:        "string",
                Description: "Vector store to search in",
            },
        },
        Required: []string{"query", "vector_store_id"},
    }
}

func (t *FileSearchTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
    query := params["query"].(string)
    vectorStoreID := params["vector_store_id"].(string)

    // 1. Search vector store
    results, err := t.searchAPI.Search(ctx, &api.SearchRequest{
        Query:         query,
        VectorStoreID: vectorStoreID,
        TopK:          5,
    })
    if err != nil {
        return &tools.Result{Success: false, Error: err}, err
    }

    // 2. Retrieve full files
    var files []interface{}
    for _, result := range results.Results {
        file, err := t.filesAPI.Get(ctx, result.FileID)
        if err != nil {
            continue
        }
        files = append(files, map[string]interface{}{
            "file_id":  file.ID,
            "filename": file.Filename,
            "content":  result.Content,
            "score":    result.Score,
        })
    }

    return &tools.Result{
        Success: true,
        Data:    files,
    }, nil
}
```

### 5. API Client Abstractions

**Supporting API interfaces:**

```go
// pkg/core/api/conversations.go
package api

import "context"

type ConversationsAPI interface {
    Create(ctx context.Context, req *ConversationCreateRequest) (*Conversation, error)
    Get(ctx context.Context, id string) (*Conversation, error)
    Update(ctx context.Context, id string, req *ConversationUpdateRequest) (*Conversation, error)
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, opts *ListOptions) ([]*Conversation, error)

    // Message operations
    AddMessage(ctx context.Context, conversationID string, message *Message) error
    GetMessages(ctx context.Context, conversationID string, opts *ListOptions) ([]*Message, error)
}

type ConversationCreateRequest struct {
    Metadata map[string]string `json:"metadata,omitempty"`
}

type ConversationUpdateRequest struct {
    Metadata map[string]string `json:"metadata,omitempty"`
}

type Conversation struct {
    ID        string            `json:"id"`
    Object    string            `json:"object"`
    CreatedAt int64             `json:"created_at"`
    Metadata  map[string]string `json:"metadata,omitempty"`
}

type Message struct {
    ID          string                 `json:"id"`
    Role        string                 `json:"role"`
    Content     []ContentPart          `json:"content"`
    Attachments []Attachment           `json:"attachments,omitempty"`
    Metadata    map[string]string      `json:"metadata,omitempty"`
    CreatedAt   int64                  `json:"created_at"`
}

// Implementation can be:
// 1. Real OpenAI API client (http client to api.openai.com)
// 2. Local storage-backed implementation (uses SessionStore)
// 3. Mock for testing
```

```go
// pkg/core/api/files.go
package api

import (
    "context"
    "io"
)

type FilesAPI interface {
    Upload(ctx context.Context, req *FileUploadRequest) (*File, error)
    Get(ctx context.Context, id string) (*File, error)
    GetContent(ctx context.Context, id string) (io.ReadCloser, error)
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, opts *ListOptions) ([]*File, error)
}

type FileUploadRequest struct {
    Filename    string
    Content     io.Reader
    Purpose     string            // "assistants", "vision", etc.
    Metadata    map[string]string
}

type File struct {
    ID        string            `json:"id"`
    Object    string            `json:"object"`
    Bytes     int64             `json:"bytes"`
    CreatedAt int64             `json:"created_at"`
    Filename  string            `json:"filename"`
    Purpose   string            `json:"purpose"`
    Metadata  map[string]string `json:"metadata,omitempty"`
}
```

## Implementation Phases

### Phase 1: Foundation (Week 1-2)

**Goal:** Core architecture + HTTP server

#### Milestone 1.1: Project Setup
- [x] Initialize Go module
- [x] Set up project structure
- [x] Add Makefile, Docker configs
- [x] Define core interfaces
- [x] CI/CD pipeline (GitHub Actions)

```bash
# Commands
make init          # Initialize project
make test          # Run tests
make lint          # Run linter
make build         # Build binaries
```

**Deliverable:** Project skeleton with passing CI/CD

---

#### Milestone 1.2: Basic HTTP Server
- [x] Implement `pkg/adapters/http/`
- [x] Simple in-memory state
- [x] Basic request/response handling
- [x] Health checks
- [x] Graceful shutdown

**Code Example:**
```go
// cmd/server/main.go
package main

import (
    "context"
    "log"
    "net/http"

    httpAdapter "github.com/yourorg/responses-gateway/pkg/adapters/http"
    "github.com/yourorg/responses-gateway/pkg/core/engine"
    "github.com/yourorg/responses-gateway/pkg/storage/memory"
)

func main() {
    // Storage
    store := memory.New()

    // Engine
    eng := engine.New(&engine.Config{
        ModelEndpoint: "https://api.openai.com/v1",
    }, store)

    // HTTP adapter
    handler := httpAdapter.New(eng)

    // Server
    srv := &http.Server{
        Addr:    ":8080",
        Handler: handler,
    }

    log.Printf("Server listening on %s", srv.Addr)
    srv.ListenAndServe()
}
```

**Test:**
```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "Hello, world!"
  }'
```

**Deliverable:** Working HTTP server with basic request handling

---

#### Milestone 1.3: Core Engine Basics
- [x] ResponsesEngine skeleton
- [x] Session management
- [x] Basic error handling
- [x] Request validation

**Deliverable:** Engine processes requests end-to-end

---

### Phase 2: State & Storage (Week 3-4)

**Goal:** Persistent state management

#### Milestone 2.1: Storage Abstraction
- [ ] Define storage interfaces
- [ ] Implement PostgreSQL backend
- [ ] Session persistence
- [ ] Conversation storage
- [ ] Response history

**Database Schema:**
```sql
-- migrations/001_init.sql
CREATE TABLE sessions (
    id VARCHAR(255) PRIMARY KEY,
    conversation_id VARCHAR(255),
    state JSONB,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP
);

CREATE TABLE conversations (
    id VARCHAR(255) PRIMARY KEY,
    session_id VARCHAR(255) REFERENCES sessions(id),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE messages (
    id VARCHAR(255) PRIMARY KEY,
    conversation_id VARCHAR(255) REFERENCES conversations(id),
    role VARCHAR(50) NOT NULL,
    content JSONB NOT NULL,
    attachments JSONB,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE responses (
    id VARCHAR(255) PRIMARY KEY,
    conversation_id VARCHAR(255) REFERENCES conversations(id),
    previous_response_id VARCHAR(255) REFERENCES responses(id),
    request JSONB,
    output JSONB,
    status VARCHAR(50),
    error JSONB,
    usage JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP
);

CREATE INDEX idx_sessions_conversation ON sessions(conversation_id);
CREATE INDEX idx_messages_conversation ON messages(conversation_id);
CREATE INDEX idx_responses_conversation ON responses(conversation_id);
CREATE INDEX idx_responses_previous ON responses(previous_response_id);
```

**Deliverable:** PostgreSQL storage working with migrations

---

#### Milestone 2.2: Redis Caching
- [ ] Implement Redis cache
- [ ] Response caching
- [ ] Hot state management
- [ ] Cache invalidation

**Deliverable:** Redis cache reduces DB load

---

#### Milestone 2.3: State Machine
- [ ] Multi-turn conversation handling
- [ ] Response linking (`previous_response_id`)
- [ ] State transitions
- [ ] Cleanup/expiration

**Test:**
```bash
# Create initial response
resp1=$(curl -X POST http://localhost:8080/v1/responses \
  -d '{"model":"gpt-4","input":"What is 2+2?"}' | jq -r .id)

# Follow-up using previous_response_id
curl -X POST http://localhost:8080/v1/responses \
  -d "{\"model\":\"gpt-4\",\"input\":\"What about 3+3?\",\"previous_response_id\":\"$resp1\"}"
```

**Deliverable:** Multi-turn conversations work, state persists across restarts

---

### Phase 3: Supporting APIs (Week 5-6)

**Goal:** Integrate Files, Conversations, Vectors, etc.

#### Milestone 3.1: Files API
- [ ] File upload/download
- [ ] File metadata storage
- [ ] S3/object storage integration
- [ ] Integration with Responses

**Deliverable:** Files can be uploaded and referenced in responses

---

#### Milestone 3.2: Conversations API
- [ ] Full CRUD operations
- [ ] Message history
- [ ] Integration with Responses
- [ ] List/pagination

**Deliverable:** Conversations API fully functional

---

#### Milestone 3.3: Vector Stores & Search
- [ ] Vector store management
- [ ] Search API implementation
- [ ] File indexing pipeline
- [ ] Embedding generation

**Deliverable:**
```bash
# Upload file → Create vector store → Search works
curl -X POST http://localhost:8080/v1/files \
  -F file=@document.pdf \
  -F purpose=assistants

curl -X POST http://localhost:8080/v1/vector-stores \
  -d '{"file_ids":["file_123"]}'

curl -X GET "http://localhost:8080/v1/vector-stores/vs_456/search?q=important+topic"
```

---

### Phase 4: Tool Execution (Week 7-8)

**Goal:** Built-in and custom tools

#### Milestone 4.1: Tool Registry
- [ ] Tool registration system
- [ ] Parameter validation
- [ ] Result handling
- [ ] Error handling

**Deliverable:** Tool registry working

---

#### Milestone 4.2: Built-in Tools
- [ ] File Search
- [ ] Web Search (integration with search provider)
- [ ] Code Interpreter (sandboxed execution)
- [ ] Image Generation

**Deliverable:** All built-in tools functional

---

#### Milestone 4.3: Custom Tools
- [ ] Function calling support
- [ ] MCP connector integration
- [ ] Tool result streaming

**Test:**
```bash
curl -X POST http://localhost:8080/v1/responses \
  -d '{
    "model": "gpt-4",
    "input": "Search my documents for budget information",
    "tools": [
      {
        "type": "file_search",
        "vector_store_id": "vs_123"
      }
    ]
  }'
```

**Deliverable:** Responses can invoke file search, function calling works, MCP tools registered

---

### Phase 5: Streaming (Week 9)

**Goal:** SSE streaming support

#### Milestone 5.1: SSE Infrastructure
- [ ] Event stream handling
- [ ] Buffering and flushing
- [ ] Error handling in streams
- [ ] Connection management

**Deliverable:** SSE infrastructure working

---

#### Milestone 5.2: Streaming Responses
- [ ] Stream response chunks
- [ ] Tool call events
- [ ] Completion events
- [ ] Usage reporting

**Test:**
```bash
curl -N -X POST http://localhost:8080/v1/responses \
  -d '{
    "model": "gpt-4",
    "input": "Write a poem",
    "stream": true
  }'

# Expected output:
# event: response.created
# data: {"type":"response.created","response":{"id":"resp_123","status":"in_progress"}}
#
# event: response.output_item.added
# data: {"type":"response.output_item.added","item":{"type":"message"}}
#
# event: response.output_text.delta
# data: {"type":"response.output_text.delta","delta":"Roses"}
# ...
```

**Deliverable:** Streaming works for both HTTP and Envoy

---

### Phase 6: Envoy Integration (Week 10-11)

**Goal:** ExtProc adapter

#### Milestone 6.1: ExtProc Adapter
- [ ] gRPC service implementation
- [ ] Request/response translation
- [ ] Stream handling
- [ ] Error propagation

**Code:**
```go
// pkg/adapters/envoy/processor.go
package envoy

import (
    extproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
)

type Processor struct {
    extproc.UnimplementedExternalProcessorServer
    engine *engine.ResponsesEngine
}

func (p *Processor) Process(srv extproc.ExternalProcessor_ProcessServer) error {
    // Handle ExtProc protocol
}
```

**Deliverable:** ExtProc adapter working

---

#### Milestone 6.2: Envoy Configuration
- [ ] Example Envoy configs
- [ ] ExtProc filter setup
- [ ] Integration testing
- [ ] Docker Compose example

**Envoy Config:**
```yaml
# examples/envoy/envoy.yaml
static_resources:
  listeners:
  - name: listener_0
    address:
      socket_address:
        address: 0.0.0.0
        port_value: 8080
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          stat_prefix: ingress_http
          http_filters:
          - name: envoy.filters.http.ext_proc
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor
              grpc_service:
                envoy_grpc:
                  cluster_name: ext_proc_cluster
              processing_mode:
                request_body_mode: BUFFERED
                response_body_mode: BUFFERED
          - name: envoy.filters.http.router
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
          route_config:
            name: local_route
            virtual_hosts:
            - name: backend
              domains: ["*"]
              routes:
              - match:
                  prefix: "/v1/responses"
                route:
                  cluster: openai_backend

  clusters:
  - name: ext_proc_cluster
    type: STATIC
    connect_timeout: 0.25s
    load_assignment:
      cluster_name: ext_proc_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 9001

  - name: openai_backend
    type: LOGICAL_DNS
    connect_timeout: 0.25s
    dns_lookup_family: V4_ONLY
    load_assignment:
      cluster_name: openai_backend
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: api.openai.com
                port_value: 443
    transport_socket:
      name: envoy.transport_sockets.tls
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
        sni: api.openai.com
```

**Deliverable:** Works with Envoy as external processor, handles both sync and streaming

---

### Phase 7: Production Readiness (Week 12-14)

**Goal:** Observability, security, performance

#### Milestone 7.1: Observability
- [ ] Prometheus metrics
- [ ] OpenTelemetry tracing
- [ ] Structured logging
- [ ] Access logs
- [ ] Dashboards (Grafana)

**Metrics:**
```go
// pkg/observability/metrics/responses.go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    ResponsesTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "responses_total",
            Help: "Total number of response requests",
        },
        []string{"model", "status"},
    )

    ResponseDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "response_duration_seconds",
            Help:    "Response processing duration",
            Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60},
        },
        []string{"model"},
    )

    ToolExecutionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "tool_execution_duration_seconds",
            Help:    "Tool execution duration",
            Buckets: []float64{0.01, 0.1, 0.5, 1, 5, 10},
        },
        []string{"tool_name"},
    )
)
```

**Deliverable:** Full observability stack

---

#### Milestone 7.2: Security
- [ ] Authentication/authorization
- [ ] API key management
- [ ] Rate limiting
- [ ] Input validation
- [ ] Secret management (Vault integration)

**Deliverable:** Production-grade security

---

#### Milestone 7.3: Performance
- [ ] Connection pooling
- [ ] Caching strategies
- [ ] Load testing (k6)
- [ ] Optimization
- [ ] Profiling

**Load Test:**
```javascript
// tests/load/responses.js
import http from 'k6/http';
import { check } from 'k6';

export let options = {
  vus: 100,
  duration: '5m',
};

export default function() {
  let res = http.post('http://localhost:8080/v1/responses', JSON.stringify({
    model: 'gpt-4',
    input: 'Hello',
  }), {
    headers: { 'Content-Type': 'application/json' },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 2s': (r) => r.timings.duration < 2000,
  });
}
```

**Deliverable:** Handles 1000 RPS with <2s latency

---

#### Milestone 7.4: Documentation
- [ ] API documentation (OpenAPI spec)
- [ ] Deployment guides
- [ ] Architecture docs
- [ ] Examples
- [ ] Tutorials

**Deliverable:** Complete documentation

---

## Key Design Decisions

### 1. Storage Backend Strategy

**Primary:** PostgreSQL for durable state
- Sessions, conversations, responses
- Full ACID guarantees
- Relational queries (join responses with conversations)

**Cache:** Redis for hot state
- Active sessions (TTL-based)
- Response caching
- Distributed locks

**Object Store:** S3/MinIO for files
- File content
- Vector embeddings
- Large payloads

**Why:** Each storage type optimized for its use case

---

### 2. Tool Execution Isolation

**Sandboxing Strategy:**
- Code Interpreter: Docker containers with resource limits
- Shell/Computer use: Firecracker microVMs
- Function calling: Process isolation with timeouts

**Resource Limits:**
```yaml
code_interpreter:
  cpu_limit: "1.0"
  memory_limit: "512Mi"
  timeout: "30s"
  network: false
```

**Why:** Security and reliability

---

### 3. Streaming Architecture

**Implementation:**
- Client-facing: SSE (Server-Sent Events)
- Internal: Go channels
- Buffering: Line-buffered for SSE

**Event Types:**
```
response.created
response.in_progress
response.output_item.added
response.content_part.added
response.output_text.delta
response.tool_call.created
response.tool_call.result
response.completed
response.failed
```

**Why:** SSE is standard for LLM streaming, channels are Go-native

---

### 4. API Client Strategy

**Phase 1:** OpenAI-compatible backends only
```go
type LLMClient interface {
    CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)
    CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (<-chan *ChatCompletionChunk, error)
}
```

**Phase 2:** Multi-provider abstraction
```go
type LLMClient interface {
    Provider() string  // "openai", "anthropic", "gemini"
    // ...
}
```

**Why:** Start simple, add complexity when needed

---

### 5. State Management

**Session Lifecycle:**
1. Create session on first request
2. Store in Redis with TTL (30 minutes)
3. Extend TTL on activity
4. Persist to PostgreSQL periodically
5. Clean up expired sessions

**Conversation Linking:**
```
Response A (id: resp_1)
    ↓ previous_response_id
Response B (id: resp_2, previous_response_id: resp_1)
    ↓
Response C (id: resp_3, previous_response_id: resp_2)
```

**Why:** Efficient for hot paths, durable for history

---

## Technology Stack

### Core
- **Language:** Go 1.23+
- **HTTP Framework:** Standard library `net/http` (no heavy frameworks)
- **gRPC:** `google.golang.org/grpc` (for Envoy ExtProc)
- **JSON:** `encoding/json` + `github.com/tidwall/gjson` (fast parsing)

### Storage
- **Database:** PostgreSQL 16+
- **Cache:** Redis 7+
- **Object Store:** S3 / MinIO
- **Migrations:** `golang-migrate/migrate`

### Observability
- **Metrics:** Prometheus + `prometheus/client_golang`
- **Tracing:** OpenTelemetry + Jaeger
- **Logging:** `slog` (structured logging)

### Gateway Integration
- **Envoy ExtProc:** `github.com/envoyproxy/go-control-plane`
- **Kong Plugin:** Kong PDK (Lua/Go hybrid)

### Testing
- **Unit Tests:** `testing` + `github.com/stretchr/testify`
- **Integration:** `testcontainers-go`
- **Load Testing:** k6
- **E2E:** Custom test suite

### DevOps
- **Containerization:** Docker
- **Orchestration:** Kubernetes
- **CI/CD:** GitHub Actions
- **Package Management:** Helm

---

## Getting Started

### Prerequisites

```bash
# Required
go 1.23+
docker & docker-compose
make

# Optional (for development)
postgresql 16+
redis 7+
k6 (load testing)
```

### Quick Start (Standalone)

```bash
# 1. Clone repository
git clone https://github.com/yourorg/openai-responses-gateway
cd openai-responses-gateway

# 2. Install dependencies
go mod download

# 3. Start dependencies (PostgreSQL, Redis)
docker-compose up -d postgres redis

# 4. Run migrations
make migrate-up

# 5. Start server
go run cmd/server/main.go

# 6. Test
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "Hello, world!"
  }'
```

### Quick Start (with Envoy)

```bash
# 1. Start full stack
cd examples/envoy
docker-compose up -d

# 2. Test through Envoy
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "Hello through Envoy!"
  }'
```

### Configuration

```yaml
# config.yaml
server:
  host: 0.0.0.0
  port: 8080
  timeout: 60s

storage:
  postgres:
    host: localhost
    port: 5432
    database: responses_gateway
    user: postgres
    password: postgres
    max_connections: 100

  redis:
    host: localhost
    port: 6379
    db: 0
    ttl: 1800s  # 30 minutes

  s3:
    endpoint: s3.amazonaws.com
    bucket: responses-gateway-files
    region: us-east-1

llm:
  provider: openai
  endpoint: https://api.openai.com/v1
  api_key: ${OPENAI_API_KEY}
  timeout: 60s
  max_retries: 3

tools:
  code_interpreter:
    enabled: true
    docker_image: python:3.11-slim
    cpu_limit: "1.0"
    memory_limit: 512Mi
    timeout: 30s

  web_search:
    enabled: true
    provider: google
    api_key: ${GOOGLE_API_KEY}

observability:
  metrics:
    enabled: true
    port: 9090

  tracing:
    enabled: true
    endpoint: http://localhost:14268/api/traces
    sample_rate: 0.1

  logging:
    level: info
    format: json
```

---

## Success Criteria

### Phase 1 (Week 2)
- ✅ HTTP server responds to `/v1/responses` POST
- ✅ In-memory state works
- ✅ Basic error handling

### Phase 2 (Week 4)
- ✅ Multi-turn conversations work
- ✅ State persists across restarts (PostgreSQL)
- ✅ Redis cache reduces DB queries

### Phase 3 (Week 6)
- ✅ Files can be uploaded and searched
- ✅ Conversations API CRUD works
- ✅ Vector search returns relevant results

### Phase 4 (Week 8)
- ✅ File search tool works
- ✅ Function calling works
- ✅ MCP tools can be registered

### Phase 5 (Week 9)
- ✅ Streaming responses work via SSE
- ✅ Tool calls streamed in real-time

### Phase 6 (Week 11)
- ✅ Envoy ExtProc adapter works
- ✅ Both sync and streaming work through Envoy

### Phase 7 (Week 14)
- ✅ Production observability (metrics, traces, logs)
- ✅ Load tested: 1000 RPS, <2s p99 latency
- ✅ Security audit passed
- ✅ Documentation complete

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Streaming complexity | Start with non-streaming, add streaming in Phase 5 |
| Tool execution security | Strict sandboxing, resource limits, timeouts |
| State consistency | Use PostgreSQL transactions, Redis for caching only |
| Performance at scale | Early load testing, connection pooling, caching |
| API compatibility | Follow OpenAI spec strictly, add tests |

---

## Next Steps

1. **Review this plan** with team
2. **Create GitHub repository**
3. **Set up project structure** (Phase 1.1)
4. **Begin implementation** following phase order
5. **Weekly check-ins** to track progress

---

## References

- [OpenAI Responses API Docs](https://platform.openai.com/docs/api-reference/responses)
- [OpenAI Conversations API Docs](https://platform.openai.com/docs/api-reference/conversations)
- [Envoy External Processing](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_proc_filter)
- [Model Context Protocol (MCP)](https://modelcontextprotocol.io/)
- [Gateway API](https://gateway-api.sigs.k8s.io/)

---

**Document Version:** 1.0
**Last Updated:** February 6, 2026
**Author:** Project Planning Team
**Status:** Ready for Implementation

# Architecture

## Overview

The gateway implements the **stateful tier** of the Open Responses API. Inference
backends provide stateless LLM generation via either `/v1/chat/completions` (default)
or `/v1/responses`. The gateway adds state management, tool execution, and storage on top.

```
┌──────────────────────────────────────────────────────────────┐
│                       HTTP Server                            │
│  ┌──────────────────────────────────────────────────┐        │
│  │ HTTP Adapter (pkg/adapters/http/)                │        │
│  │  - Routing, SSE streaming, OpenAPI serving       │        │
│  └──────────────────────┬───────────────────────────┘        │
└─────────────────────────┼────────────────────────────────────┘
                          │
┌─────────────────────────▼────────────────────────────────────┐
│                  Core Engine (Stateful Tier)                  │
│  • Response & Conversation storage                           │
│  • Agentic tool loop (MCP, file_search)                      │
│  • Connectors (MCP registry)                                 │
│  • Files + Vector Stores                                     │
│  • Prompts API                                               │
│  • Streaming (SSE)                                           │
└─────────────────────────┬────────────────────────────────────┘
                          │
               ResponsesAPIClient
                ┌─────────┴────────┐
                │                  │
          ChatCompletions     Responses
            Adapter            Client
          /v1/chat/           /v1/
          completions       responses
                │                  │
┌───────────────▼──────────────────▼───────────────────────────┐
│                       Inference Backend                       │
│  • /v1/chat/completions (vLLM, Ollama, TGI, etc.)            │
│  • /v1/responses (vLLM, Ollama, OpenAI)                      │
└──────────────────────────────┬───────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────┐
│                 Vector Store Layer (optional)                 │
│  • Embedding Client (OpenAI-compatible)                      │
│  • Milvus Backend (HNSW + cosine similarity)                 │
│  • Memory Backend (no-op, default)                           │
└──────────────────────────────┬───────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────┐
│                         Storage Layer                        │
│  • In-Memory Store (default)                                 │
│  • SQLite Store (persistent, pure Go)                        │
│  • PostgreSQL Store (persistent, pgx/v5)                     │
└──────────────────────────────────────────────────────────────┘
```

## Layers

### HTTP Adapter

The HTTP adapter (`pkg/adapters/http/`) provides routing, SSE streaming, and OpenAPI spec serving. It translates HTTP requests into core engine calls and handles response formatting.

### Core Engine

Gateway-agnostic business logic (`pkg/core/`):

- **engine/** — Main orchestration: forwards requests to the inference backend, handles agentic tool calling loops (MCP, file_search), manages streaming by forwarding native SSE events
- **schema/** — API type definitions for Responses, Files, Vector Stores, Conversations, Prompts
- **config/** — Configuration loading from YAML files and environment variables
- **api/** — Backend client interface and format conversion:
  - `ResponsesAPIClient` with two implementations:
    - `ChatCompletionsAdapter` — calls `/v1/chat/completions` (default, works with vLLM, Ollama, TGI, etc.)
    - `OpenAIResponsesClient` — calls `/v1/responses` (for backends that support the Responses API)
  - `ConvertToChatRequest` / `ConvertFromChatResponse` — format converters
- **services/** — Higher-level service layer (vector store ingestion and search)
- **state/** — State management interfaces

### Vector Store Layer

Pluggable vector search backends (`pkg/vectorstore/`):

- **Backend interface** — `CreateStore`, `DeleteStore`, `InsertChunks`, `DeleteFileChunks`, `Search`, `Close`
- **memory/** — No-op backend (default when vector search is not configured)
- **milvus/** — Milvus implementation with HNSW index and cosine similarity

**VectorStoreService** (`pkg/core/services/`) coordinates the full ingestion pipeline: read file content → chunk text → embed via OpenAI-compatible API → insert into backend. Search follows the reverse path: embed query → vector similarity search → return ranked results.

**Engine integration:** The engine intercepts `file_search` tool calls (like MCP tools) and executes them server-side when a `VectorSearcher` is configured.

### Storage Layer

Pluggable storage backends (`pkg/storage/`):

- **memory/** — In-memory store for sessions, files, vectors, conversations, and prompts (default)
- **sqlite/** — SQLite persistent store for sessions, conversations, and responses (pure Go via `modernc.org/sqlite`, no CGO required)
- **postgres/** — PostgreSQL persistent store for sessions, conversations, and responses (via `pgx/v5`, supports connection pooling and concurrent writers)

## Deployment

The gateway runs as a standalone HTTP server. Clients connect directly. The gateway handles the full request lifecycle: parsing, state resolution, backend calls, tool loops, streaming, and response assembly.

```
┌────────┐         ┌──────────────────────────┐         ┌─────────┐
│ Client │──HTTP──▶│  Gateway (HTTP Server)   │──HTTP──▶│ Backend │
│        │◀──SSE───│  :8080                   │◀──SSE───│ (vLLM)  │
└────────┘         └──────────────────────────┘         └─────────┘
```

This supports full SSE streaming, all API endpoints (Responses, Files, Vector Stores, Conversations, Prompts), and the agentic tool loop.

The gateway can be deployed behind any reverse proxy (Envoy, nginx, HAProxy) as a regular upstream service for TLS termination, load balancing, rate limiting, and observability. For inference-aware routing, consider [Gateway API Inference Extension (GIE)](https://gateway-api-inference-extension.sigs.k8s.io/) which handles Envoy-based model routing and scheduling.

## Request Flow

1. Request arrives at the HTTP server
2. Handler parses and validates the request
3. Core engine resolves conversation context (previous_response_id)
4. `file_search` and MCP tools are expanded into function definitions
5. Engine sends the request to the inference backend via `ResponsesAPIClient`:
   - `ChatCompletionsAdapter` (default): translates to `/v1/chat/completions` format
   - `OpenAIResponsesClient`: forwards to `/v1/responses` as-is
6. For tool calls: engine executes the agentic loop (call → result → call)
   - MCP tools: executed via MCP client
   - file_search: query embedded → vector search → results fed back to LLM
   - Client-side function tools: returned to the caller for execution
7. SSE events from the backend are normalized and forwarded through the adapter
8. Gateway manages response lifecycle events (created, completed)
9. Session state is persisted after streaming completes

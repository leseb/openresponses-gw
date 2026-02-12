# Architecture

## Overview

The gateway implements the **stateful tier** of the Open Responses API. Inference
backends (vLLM, OpenAI, etc.) provide stateless LLM generation via `/v1/responses`.
The gateway adds state management, tool execution, and storage on top.

```
┌─────────────────────────────────────────────────────────────────┐
│                         Adapter Layer                            │
│  ┌──────────────┐  ┌──────────────┐                             │
│  │ HTTP Server  │  │ Envoy ExtProc│  (extensible)               │
│  └──────────────┘  └──────────────┘                             │
└─────────────────────────┬───────────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────────┐
│                    Core Engine (Stateful Tier)                    │
│  • Response & Conversation storage                               │
│  • Agentic tool loop (MCP, file_search)                          │
│  • Connectors (MCP registry)                                     │
│  • Files + Vector Stores                                         │
│  • Prompts API                                                   │
│  • Streaming (SSE) — forwards native backend events              │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                   POST /v1/responses
                   (inference only)
                          │
┌─────────────────────────▼───────────────────────────────────────┐
│                   Inference Backend                               │
│  • Any /v1/responses-compatible server                           │
│  • vLLM, OpenAI, etc.                                            │
└──────────┬──────────────────────────────────────────────────────┘
           │
┌──────────▼──────────────────────────────────────────────────────┐
│                  Vector Store Layer (optional)                    │
│  • Embedding Client (OpenAI-compatible)                          │
│  • Milvus Backend (HNSW + cosine similarity)                     │
│  • Memory Backend (no-op, default)                               │
└──────────┬──────────────────────────────────────────────────────┘
           │
┌──────────▼──────────────────────────────────────────────────────┐
│                       Storage Layer                              │
│  • In-Memory Store (current)                                     │
│  • PostgreSQL, Redis (planned)                                   │
└──────────────────────────────────────────────────────────────────┘
```

## Layers

### Adapter Layer

Gateway-specific adapters that translate between the gateway's protocol and the core engine. The design is extensible — new adapters can be added under `pkg/adapters/`.

**HTTP Server** (`pkg/adapters/http/`) — Standalone HTTP server with full routing, SSE streaming, and OpenAPI spec serving.

**Envoy ExtProc** (`pkg/adapters/envoy/`) — gRPC External Processor for Envoy proxy. Supports request/response extraction, content decompression (gzip, brotli), and health checks.

### Core Engine

Gateway-agnostic business logic (`pkg/core/`):

- **engine/** — Main orchestration: forwards requests to the backend's `/v1/responses` endpoint, handles agentic tool calling loops (MCP, file_search), manages streaming by forwarding native SSE events
- **schema/** — API type definitions for Responses, Files, Vector Stores, Conversations, Prompts
- **config/** — Configuration loading from YAML files and environment variables
- **api/** — Responses API client (`ResponsesAPIClient` interface) for calling the inference backend
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

- **memory/** — In-memory store for sessions, files, vectors, conversations, and prompts (current default)
- PostgreSQL and Redis backends are planned

## Request Flow

1. Request arrives at an adapter (HTTP or Envoy ExtProc)
2. Adapter parses and validates the request
3. Core engine resolves conversation context (previous_response_id)
4. `file_search` and MCP tools are expanded into function definitions
5. Engine sends a `/v1/responses` request to the inference backend
6. For tool calls: engine executes the agentic loop (call → result → call)
   - MCP tools: executed via MCP client
   - file_search: query embedded → vector search → results fed back to LLM
   - Client-side function tools: returned to the caller for execution
7. Native SSE events from the backend are forwarded through the adapter
8. Gateway manages response lifecycle events (created, completed)
9. Session state is persisted after streaming completes

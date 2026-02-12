# Architecture

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         Adapter Layer                            │
│  ┌──────────────┐  ┌──────────────┐                             │
│  │ HTTP Server  │  │ Envoy ExtProc│  (extensible)               │
│  └──────────────┘  └──────────────┘                             │
└─────────────────────────┬───────────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────────┐
│                    Core Engine (Gateway-Agnostic)                │
│  • Responses API → Chat Completions Translation                 │
│  • Request Validation & Parameter Handling                       │
│  • Streaming (SSE) Support                                       │
└─────────────────────────┬───────────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────────┐
│                   LLM Backend Integration                        │
│  • OpenAI Client (via openai-go SDK)                            │
│  • Supports: OpenAI, Ollama, vLLM, etc.                         │
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

- **engine/** — Main orchestration: translates Responses API requests to Chat Completions, handles tool calling loops, manages streaming
- **schema/** — API type definitions for Responses, Files, Vector Stores, Conversations, Prompts
- **config/** — Configuration loading from YAML files and environment variables
- **api/** — OpenAI-compatible LLM client (via `openai-go` SDK)
- **services/** — Higher-level service layer (models discovery)
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
3. Core engine translates Responses API format to Chat Completions
4. `file_search` and MCP tools are expanded into function definitions
5. LLM client sends the request to the configured backend
6. For tool calls: engine executes the agentic loop (call → result → call)
   - MCP tools: executed via MCP client
   - file_search: query embedded → vector search → results fed back to LLM
   - Client-side tools: returned to the caller for execution
7. Response is streamed back through the adapter as SSE events
8. Session state is persisted incrementally during streaming

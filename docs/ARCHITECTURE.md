# Architecture

## Overview

The gateway implements the **stateful tier** of the Open Responses API. Inference
backends provide stateless LLM generation via either `/v1/chat/completions` (default)
or `/v1/responses`. The gateway adds state management, tool execution, and storage on top.

```
┌──────────────────────────────────────────────────────────────┐
│                         Adapter Layer                        │
│  ┌──────────────┐  ┌───────────────┐                         │
│  │ HTTP Server  │  │ Envoy ExtProc │  (extensible)           │
│  └──────────────┘  └───────────────┘                         │
└───────────────────────────────┬──────────────────────────────┘
                                │
┌───────────────────────────────▼──────────────────────────────┐
│                  Core Engine (Stateful Tier)                  │
│  • Response & Conversation storage                           │
│  • Agentic tool loop (MCP, file_search)                      │
│  • Connectors (MCP registry)                                 │
│  • Files + Vector Stores                                     │
│  • Prompts API                                               │
│  • Streaming (SSE) — forwards native backend events          │
└───────────────────────────────┬──────────────────────────────┘
                                │
                   ResponsesAPIClient interface
                    ┌───────────┴───────────┐
                    │                       │
     ChatCompletionsAdapter     OpenAIResponsesClient
     POST /v1/chat/completions  POST /v1/responses
           (default)
                    │                       │
┌───────────────────▼───────────────────────▼──────────────────┐
│                       Inference Backend                      │
│  • /v1/chat/completions (vLLM, Ollama, TGI, etc.)            │
│  • /v1/responses (vLLM, Ollama, OpenAI)                      │
└───────────────────────────────┬──────────────────────────────┘
                                │
┌───────────────────────────────▼──────────────────────────────┐
│                 Vector Store Layer (optional)                 │
│  • Embedding Client (OpenAI-compatible)                      │
│  • Milvus Backend (HNSW + cosine similarity)                 │
│  • Memory Backend (no-op, default)                           │
└───────────────────────────────┬──────────────────────────────┘
                                │
┌───────────────────────────────▼──────────────────────────────┐
│                         Storage Layer                        │
│  • In-Memory Store (default)                                 │
│  • SQLite Store (persistent, pure Go)                        │
└──────────────────────────────────────────────────────────────┘
```

## Layers

### Adapter Layer

Gateway-specific adapters that translate between the gateway's protocol and the core engine. The design is extensible — new adapters can be added under `pkg/adapters/`.

**HTTP Server** (`pkg/adapters/http/`) — Standalone HTTP server with full routing, SSE streaming, and OpenAPI spec serving.

**Envoy ExtProc** (`pkg/adapters/envoy/`) — gRPC External Processor for Envoy proxy. Supports request/response extraction, content decompression (gzip, brotli), and health checks.

### Core Engine

Gateway-agnostic business logic (`pkg/core/`):

- **engine/** — Main orchestration: forwards requests to the inference backend, handles agentic tool calling loops (MCP, file_search), manages streaming by forwarding native SSE events
- **schema/** — API type definitions for Responses, Files, Vector Stores, Conversations, Prompts
- **config/** — Configuration loading from YAML files and environment variables
- **api/** — Backend client interface (`ResponsesAPIClient`) with two implementations:
  - `ChatCompletionsAdapter` — calls `/v1/chat/completions` (default, works with vLLM, Ollama, TGI, etc.)
  - `OpenAIResponsesClient` — calls `/v1/responses` (for backends that support the Responses API)
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

## Deployment Modes

The gateway supports two deployment modes. Both use the same core engine; they differ in how traffic reaches the backend.

### Mode 1: Standalone HTTP Server

The gateway runs as a standalone HTTP server. Clients connect directly. The gateway handles the full request lifecycle: parsing, state resolution, backend calls, tool loops, streaming, and response assembly.

```
┌────────┐         ┌──────────────────────────┐         ┌─────────┐
│ Client │──HTTP──▶│  Gateway (HTTP Adapter)  │──HTTP──▶│ Backend │
│        │◀──SSE───│  :8080                   │◀──SSE───│ (vLLM)  │
└────────┘         └──────────────────────────┘         └─────────┘
```

This is the default mode (`cmd/server`). It supports full SSE streaming, all API endpoints (Responses, Files, Vector Stores, Conversations, Prompts), and the agentic tool loop.

### Mode 2: Envoy Filter Chain (ExtProc)

The gateway runs as an Envoy External Processor (gRPC). Envoy sits in front of the backend and routes traffic through the ExtProc filter before forwarding to the backend. The gateway never makes its own backend calls — Envoy handles routing.

```
 Client             Envoy            Gateway (ExtProc)      Backend (vLLM)
   │                  │                     │                     │
   │──HTTP request───▶│                     │                     │
   │                  │──req headers (gRPC)▶│                     │
   │                  │──req body (gRPC)───▶│                     │
   │                  │                     │ parse request       │
   │                  │                     │ resolve state       │
   │                  │                     │ expand tools        │
   │                  │◀─modified req+state─│                     │
   │                  │                                           │
   │                  │─────HTTP (enriched request)──────────────▶│
   │                  │◀────HTTP response─────────────────────────│
   │                  │                                           │
   │                  │──resp body (gRPC)──▶│                     │
   │                  │                     │ save state          │
   │                  │                     │ rewrite IDs         │
   │                  │◀─modified response──│                     │
   │                  │                     │                     │
   │◀─HTTP response───│                     │                     │
   │                  │                     │                     │
```

**How it works:**

1. **Request headers** — Envoy sends request headers to the ExtProc. The processor acknowledges and waits for the body.
2. **Request body** — The processor parses the client's Responses API request, resolves conversation context (multi-turn history), expands tool definitions, and builds a `ConversationState`. It then:
   - Injects the state into an `x-openresponses-state` HTTP header (base64-encoded JSON)
   - Modifies the request body with expanded tools and conversation messages
   - Returns the modified request to Envoy
3. **Envoy forwards** — Envoy routes the enriched request to the backend cluster (vLLM). The backend processes inference normally, unaware of the gateway.
4. **Response headers** — The processor acknowledges without modification.
5. **Response body** — The processor parses the backend's response, saves the response and conversation state to the session store, rewrites the response with the gateway's response/conversation IDs, and returns the modified response to Envoy.
6. **Envoy returns** — The client receives the final response.

**Key design choice:** The gateway acts as a pure filter — it enriches requests and post-processes responses but never makes its own HTTP calls to the backend. This keeps the data path through Envoy, enabling Envoy's native features (load balancing, retries, circuit breaking, observability) to apply to backend traffic.

**Trade-offs vs. standalone mode:**
- Streaming is limited to what Envoy's `BUFFERED` body mode supports (no incremental SSE)
- The agentic tool loop (multi-round tool calling) is not supported — only single-turn inference
- Only the Responses API is handled; other endpoints (Files, Vector Stores, etc.) require separate routing

## Request Flow

1. Request arrives at an adapter (HTTP or Envoy ExtProc)
2. Adapter parses and validates the request
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

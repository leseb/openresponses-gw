# Architecture

## Overview

The gateway implements the **stateful tier** of the Open Responses API. Inference
backends provide stateless LLM generation via either `/v1/chat/completions` (default)
or `/v1/responses`. The gateway adds state management, tool execution, and storage on top.

```
┌──────────────────────────────────────────────────────────────┐
│                         Adapter Layer                        │
│  ┌──────────────┐    ┌───────────────┐                       │
│  │ HTTP Server  │    │ Envoy ExtProc │   (extensible)        │
│  └──────┬───────┘    └───────┬───────┘                       │
│         │    shared engine   │                               │
└─────────┼────────────────────┼───────────────────────────────┘
          │                    │
┌─────────▼────────────────────▼───────────────────────────────┐
│                  Core Engine (Stateful Tier)                 │
│  • Response & Conversation storage                           │
│  • Agentic tool loop (MCP, file_search)                      │
│  • Connectors (MCP registry)                                 │
│  • Files + Vector Stores                                     │
│  • Prompts API                                               │
│  • Streaming (SSE)                                           │
└─────────┬────────────────────┬───────────────────────────────┘
          │                    │
   (HTTP adapter)       (ExtProc adapter)
          │                    │
 ResponsesAPIClient     Format conversion in
  ┌───────┴────────┐    processor + Envoy
  │                │    forwards to backend
ChatCompletions  Responses     │
  Adapter        Client        │
/v1/chat/       /v1/           │
completions   responses        │
  │                │           │
┌─▼────────────────▼───────────▼───────────────────────────────┐
│                       Inference Backend                      │
│  • /v1/chat/completions (vLLM, Ollama, TGI, etc.)            │
│  • /v1/responses (vLLM, Ollama, OpenAI)                      │
└──────────────────────────────┬───────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────┐
│                 Vector Store Layer (optional)                │
│  • Embedding Client (OpenAI-compatible)                      │
│  • Milvus Backend (HNSW + cosine similarity)                 │
│  • Memory Backend (no-op, default)                           │
└──────────────────────────────┬───────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────┐
│                         Storage Layer                        │
│  • In-Memory Store (default)                                 │
│  • SQLite Store (persistent, pure Go)                        │
└──────────────────────────────────────────────────────────────┘
```

## Layers

### Adapter Layer

Gateway-specific adapters that translate between the gateway's protocol and the core engine. The design is extensible — new adapters can be added under `pkg/adapters/`.

**HTTP Server** (`pkg/adapters/http/`) — HTTP server with full routing, SSE streaming, and OpenAPI spec serving. Always active.

**Envoy ExtProc** (`pkg/adapters/envoy/`) — gRPC External Processor for Envoy proxy. Enabled when `extproc.port` is configured (or `--extproc-port` flag). Runs in the same process as the HTTP server, sharing all stores and the engine. For simple requests (non-streaming, no server-side tools), the ExtProc operates in filter chain mode: it calls `engine.PrepareRequest()` and `engine.ProcessResponse()` directly for state management, performs format conversion itself (chat completions or responses API based on `backend_api` config), and lets Envoy forward the enriched request to the backend. For streaming requests or those with server-side tools (MCP, file_search), the ExtProc uses an ImmediateResponse fallback: it calls `engine.ProcessRequestStream()` or `engine.ProcessRequest()` to handle the full request lifecycle internally, bypassing the filter chain entirely. Streaming responses are buffered (all SSE events collected before delivery). Supports request/response extraction, content decompression (gzip, brotli), and health checks.

### Core Engine

Gateway-agnostic business logic (`pkg/core/`):

- **engine/** — Main orchestration: forwards requests to the inference backend, handles agentic tool calling loops (MCP, file_search), manages streaming by forwarding native SSE events
- **schema/** — API type definitions for Responses, Files, Vector Stores, Conversations, Prompts
- **config/** — Configuration loading from YAML files and environment variables
- **api/** — Backend client interface and format conversion:
  - `ResponsesAPIClient` (used by HTTP adapter) with two implementations:
    - `ChatCompletionsAdapter` — calls `/v1/chat/completions` (default, works with vLLM, Ollama, TGI, etc.)
    - `OpenAIResponsesClient` — calls `/v1/responses` (for backends that support the Responses API)
  - `ConvertToChatRequest` / `ConvertFromChatResponse` — format converters shared by both adapters
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

### Mode 2: Envoy Dual-Cluster (ExtProc + Gateway)

Envoy acts as the single entrypoint for all APIs. Traffic is split into two clusters based on route:

- **POST `/v1/responses`** → ExtProc filter → backend inference server (vLLM)
- **Everything else** → gateway HTTP server (CRUD APIs)

Both the ExtProc and the gateway HTTP server share the same file-based SQLite database for conversation/response history. SQLite WAL mode handles concurrent access safely.

```
Client → Envoy:8081 ─┬─ POST /v1/responses ──→ ExtProc:10000 ──→ vLLM:8000
                     │                              ↑
                     │                    (process response)
                     │
                     └─ Everything else ──→ HTTP Server:8082
                         /v1/files              (CRUD APIs)
                         /v1/vector_stores       shared SQLite DB
                         /v1/prompts
                         /v1/conversations
                         /v1/connectors
                         GET/DELETE /v1/responses/*
                         /health
```

**Inference path (POST /v1/responses):**

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

**CRUD path (everything else):**

```
 Client             Envoy            Gateway (HTTP Server)
   │                  │                          │
   │──HTTP request───▶│                          │
   │                  │─── proxy (no ExtProc) ──▶│
   │                  │◀── HTTP response ────────│
   │◀─HTTP response───│                          │
```

**How the inference path works:**

1. **Request headers** — Envoy sends request headers to the ExtProc. The processor removes `content-length` (the body will be mutated to a different size) and, for `chat_completions` backends, rewrites `:path` from `/v1/responses` to `/v1/chat/completions`.
2. **Request body** — The processor parses the client's Responses API request, calls `engine.PrepareRequest()` to resolve conversation context (multi-turn history), expand tool definitions, and build a `ConversationState`. It then:
   - Converts the prepared request to the backend format (`ConvertToChatRequest` for chat completions, or serializes the Responses API request directly)
   - Replaces the request body with the converted payload
   - Injects the state into an `x-openresponses-state` HTTP header (base64-encoded JSON) for retrieval in the response phase
   - Returns the modified request to Envoy
3. **Envoy forwards** — Envoy routes the enriched request to the backend cluster (vLLM). The backend processes inference normally, unaware of the gateway.
4. **Response headers** — The processor acknowledges without modification.
5. **Response body** — The processor parses the backend's response (using `ConvertFromChatResponse` for chat completions backends, or unmarshalling the Responses API response directly), calls `engine.ProcessResponse()` to save state and rewrite IDs, and returns the modified response body to Envoy.
6. **Envoy returns** — The client receives the final Responses API response.

**Key design choices:**

- **Single entrypoint** — Envoy handles all traffic, so cross-cutting concerns (rate limiting, auth, TLS, observability) apply uniformly to every API.
- **Per-route ExtProc disable** — The ExtProc filter is disabled via `ExtProcPerRoute` for non-inference routes. Traffic to the gateway HTTP server passes through Envoy as a simple reverse proxy.
- **Shared state** — Both processes point to the same file-based SQLite database. The ExtProc writes response/conversation state; the gateway HTTP server reads and writes CRUD data. WAL mode ensures safe concurrent access.
- **Pure filter for inference** — The ExtProc enriches requests and post-processes responses but never makes its own HTTP calls to the backend. This keeps the inference data path through Envoy, enabling load balancing, retries, circuit breaking, and observability.

**Trade-offs vs. standalone mode:**
- Streaming and agentic tool loop requests bypass the Envoy filter chain via ImmediateResponse — the ExtProc calls the backend directly instead of letting Envoy forward the request. This means Envoy's load balancing, retries, and circuit breaking do not apply for these request types.
- Streaming responses are buffered — all SSE events are collected before delivery to the client (not incremental). True incremental streaming via Envoy's `STREAMED` body mode is a future improvement.
- Requires running two processes (gateway + Envoy) instead of one, though the gateway is a single binary serving both HTTP and ExtProc

## Request Flow

### HTTP Adapter

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

### ExtProc Adapter

**Filter chain mode** (non-streaming, no server-side tools):

1. Request arrives at Envoy, which streams headers and body to the ExtProc via gRPC
2. Processor parses the Responses API request and calls `engine.PrepareRequest()`
3. Engine resolves conversation context and expands tools (same as HTTP path)
4. Processor converts the prepared request to backend format and mutates the body
5. Envoy forwards the enriched request to the backend — the gateway never makes the HTTP call
6. Backend response flows back through Envoy to the ExtProc
7. Processor parses the response, calls `engine.ProcessResponse()` to save state, and rewrites IDs
8. Envoy returns the final Responses API response to the client

**ImmediateResponse mode** (streaming or server-side tools):

1. Request arrives at Envoy, which streams headers and body to the ExtProc via gRPC
2. Processor parses the Responses API request and detects `stream: true` or server-side tools (MCP, file_search)
3. For streaming: calls `engine.ProcessRequestStream()`, collects all SSE events, returns buffered SSE as ImmediateResponse
4. For tool loop: calls `engine.ProcessRequest()` (full agentic loop), returns JSON as ImmediateResponse
5. Envoy returns the ImmediateResponse directly to the client — the backend request is handled by the engine internally

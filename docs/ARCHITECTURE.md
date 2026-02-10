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
└─────────────────────────┬───────────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────────┐
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

### Storage Layer

Pluggable storage backends (`pkg/storage/`):

- **memory/** — In-memory store for sessions, files, vectors, conversations, and prompts (current default)
- PostgreSQL and Redis backends are planned

## Request Flow

1. Request arrives at an adapter (HTTP or Envoy ExtProc)
2. Adapter parses and validates the request
3. Core engine translates Responses API format to Chat Completions
4. LLM client sends the request to the configured backend
5. For tool calls: engine executes the agentic loop (call → result → call)
6. Response is streamed back through the adapter as SSE events
7. Session state is persisted incrementally during streaming

# Design: Ecosystem Alignment — HTTP Server as the Right Abstraction

**Status:** Accepted
**Date:** 2026-02-19

## Context

The Kubernetes AI networking ecosystem has converged around a layered architecture
with the Gateway API Inference Extension (GIE) as the standard for inference-aware
routing. This document analyzes the ecosystem, identifies where openresponses-gw
fits, and explains why a standalone HTTP server — rather than an Envoy ExtProc
filter — is the correct integration point.

## Ecosystem Map

The upstream AI inference stack on Kubernetes is coalescing around these projects:

| Component | Role | Project |
|-----------|------|---------|
| Gateway API Inference Extension | Inference-aware scheduling: KV-cache affinity, LoRA routing, priority queuing, body-based routing (BBR) | [kubernetes-sigs/gateway-api-inference-extension](https://github.com/kubernetes-sigs/gateway-api-inference-extension) |
| Semantic Router (vSR) | ML-based model selection, API translation, guardrails | [vllm-project/semantic-router](https://github.com/vllm-project/semantic-router) |
| MCP Gateway | MCP server aggregation and federation | [Kuadrant/mcp-gateway](https://github.com/Kuadrant/mcp-gateway) |
| Kube Agentic Networking | Tool-level authorization for MCP servers | [kubernetes-sigs/kube-agentic-networking](https://github.com/kubernetes-sigs/kube-agentic-networking) |
| WG AI Gateway | Standards: Payload Processing, Egress Gateways, Backend CRD | [kubernetes-sigs/wg-ai-gateway](https://github.com/kubernetes-sigs/wg-ai-gateway) |
| **openresponses-gw** | **Responses API: stateful conversations, agentic loops, protocol translation** | This project |

### Gateway API Inference Extension (GIE)

GIE is the gravitational center. It is GA, backed by Google, Red Hat, vLLM, and
multiple gateway vendors. Key facts:

- Extends Kubernetes Gateway API with `InferencePool` and `InferenceModel` CRDs.
- Uses Envoy ExtProc for inference-aware endpoint selection (KV-cache utilization,
  LoRA adapter affinity, prefix cache routing, priority queuing).
- Defines a **pluggable BBR (Body-Based Router) framework** as the standard
  extension point for body-level request processing.
- The Semantic Router is being integrated into GIE's ExtProc binary as a
  `BBRPlugin` — compiled in-process, not as a separate service.

### Semantic Router (vSR)

vLLM's Semantic Router provides ML-based model selection, API translation
(OpenAI/Anthropic adapters), guardrails (PII detection, jailbreak prevention),
and semantic caching. The key development: **vSR is being folded into GIE as a
BBR plugin**. API translation happens in-process within `Execute()`, not as a
standalone ExtProc. This validates the pattern of composing capabilities within
GIE rather than adding parallel ExtProc services.

### MCP Gateway (Kuadrant)

Envoy-based MCP gateway using ExtProc for routing `tools/call` requests to
federated backend MCP servers. Tool aggregation with prefix-based namespacing.
Kubernetes-native with `MCPServerRegistration` CRDs.

### Kube Agentic Networking

Tool-level authorization for MCP servers via Envoy + xDS. `XBackend` and
`XAccessPolicy` CRDs enforce which agents can call which MCP tools.
Deny-by-default for `tools/call`.

### WG AI Gateway

Standards working group (not shipping code). Two active proposals:
- **Payload Processing**: Declarative request/response body inspection pipelines
  on `HTTPRoute` filters.
- **Egress Gateways**: `Backend` CRD for external destinations with TLS,
  credential injection, and protocol support (HTTP, MCP).

## openresponses-gw's Unique Value

### Responses API coverage in the ecosystem

vSR provides partial Responses API support: it translates `/v1/responses`
requests to Chat Completions format, persists responses (memory or Redis),
and chains conversations via `previous_response_id` with linked-list traversal.
This is a **stateful translation layer** — it handles format conversion and
conversation persistence but delegates all LLM interaction to the backend as
a single Chat Completions call.

What vSR does not implement — and what defines the full Responses API surface:

| Capability | vSR | openresponses-gw |
|------------|-----|-----------------|
| Responses ↔ Chat Completions translation | Yes | Yes |
| `previous_response_id` chaining | Yes | Yes |
| Response persistence | Yes (memory, Redis) | Yes (memory, SQLite) |
| Vector store backends | Memory, Milvus, Llama Stack | Memory, Milvus |
| Files API | Yes | Yes |
| Vector Stores API | Yes | Yes |
| SSE streaming (24 event types) | No | Yes |
| Agentic tool loops (multi-turn) | No | Yes (up to 10 iterations) |
| Server-side `file_search` execution | No (delegates to upstream or pre-injects via RAG plugin) | Yes (local vector store query + embedding) |
| MCP tool execution in agentic loop | No (MCP used for classification) | Yes |
| Prompts API | No | Yes |
| Conversations API (CRUD) | No | Yes |
| Connectors API (MCP registry) | No | Yes |

The Responses API's full value comes from the **agentic, streaming features**
that sit above format translation:

- **Streaming**: 24 SSE event types (`response.created`,
  `response.output_text.delta`, `response.completed`, etc.) enabling real-time
  incremental delivery
- **Agentic loops**: the engine calls the LLM, intercepts tool calls
  (`file_search`, MCP), executes them server-side, feeds results back, and
  repeats — up to 10 iterations per request
- **Server-side tool execution**: `file_search` queries are embedded and run
  against vector stores locally; MCP tools are dispatched to registered
  connectors
- **Higher-level APIs**: Prompts (versioned templates), Conversations (CRUD +
  item management), Connectors (MCP server registry)

This is the project's unique contribution to the ecosystem. vSR provides
Responses API format compatibility; openresponses-gw provides the full
stateful, agentic, streaming Responses API surface.

## Why an ExtProc Adapter Is the Wrong Abstraction

### 1. We already bypass Envoy for the valuable features

Streaming and agentic tool loops use `ImmediateResponse` — the ExtProc makes
backend calls directly, skipping Envoy's load balancing, retries, and
observability. For the most valuable features, we are an HTTP server with extra
steps.

### 2. GIE owns the ExtProc space

GIE's ExtProc handles inference-aware scheduling (KV-cache affinity, LoRA
routing, priority queuing). Its BBR plugin framework is the standard extension
point for body-level processing. A parallel ExtProc implementation solving a
different problem in the same slot creates confusion and integration conflicts.

### 3. Stateful APIs don't fit the filter chain model

ExtProc processes individual requests statelessly. The Responses API's
conversations, vector stores, and multi-turn tool loops require persistent state.
Shoehorning that into a request filter adds complexity for no benefit.

### 4. The ecosystem shows the pattern

The vSR integration design demonstrates how body-level processing belongs in
GIE as a BBR plugin — not as a standalone ExtProc. If Responses-to-Chat
Completions translation ever belongs in the data plane, it should follow the
same pattern: a compiled-in BBR plugin implementing the `BBRPlugin` interface,
not a separate gRPC service.

### 5. A BBR plugin is not a viable alternative

The natural question is whether openresponses-gw should be repackaged as a GIE
BBR plugin instead of a standalone service. The BBR plugin interface makes this
impractical:

```go
type BBRPlugin interface {
    plugins.Plugin
    RequiresFullParsing() bool
    Execute(requestBodyBytes []byte) (headers map[string]string, mutatedBodyBytes []byte, err error)
}
```

`Execute()` is a **stateless, single-pass request body transformation**: bytes
in, headers + mutated bytes out. This works well for vSR because its core
operations (extract model name, classify request, map API fields) are stateless
transformations. openresponses-gw's core operations are none of these things:

| Capability | What it needs | BBR plugin support |
|------------|--------------|-------------------|
| **SSE streaming** | Incremental event forwarding from backend to client | No — `Execute()` operates on complete request bodies. No streaming hook exists. The old ExtProc adapter already proved this: it had to use `ImmediateResponse` to bypass the filter chain for streaming, buffering all events before delivery. |
| **Agentic tool loops** | Multiple sequential backend calls per single client request (call LLM → tool call → execute tool → call LLM → ..., up to 10 iterations) | No — `Execute()` is called once per request. There is no loop primitive, no ability to make outbound calls, and no mechanism to interleave backend requests with tool execution. |
| **Response processing** | Save conversation state, rewrite IDs, manage lifecycle events, append to vector stores after the backend responds | No — `Execute()` only processes the inbound request body. The signature has no response hook. |
| **Persistent state** | SQLite/memory stores for conversations and responses, Milvus connections for vector search, MCP client sessions, file storage backends (S3, filesystem) | No — BBR plugins are stateless transformations. Embedding database connections, storage backends, and long-lived client sessions inside a request processing plugin conflates concerns and couples failure modes. |
| **Outbound HTTP calls** | Backend inference calls, MCP tool execution, embedding API calls for vector search | No — BBR plugins return mutated bytes and headers. They do not make outbound network calls. |

Beyond the interface limitations, compiling into IGW creates operational
problems: our dependency tree (SQLite, Milvus client, MCP client, S3 client,
embedding client) would be linked into the inference scheduler binary, and a
panic in our code would take down model routing for the entire cluster.

### Where BBR and openresponses-gw overlap

It is not a total mismatch. A narrow stateless subset of our functionality
could fit the BBR model:

- Extract model name from a Responses API request body
- Convert Responses API request → Chat Completions format
  (`ConvertToChatRequest` — a pure body transformation)
- Set routing headers for backend selection

But this only covers the simplest case: a non-streaming, single-turn request
with no server-side tools, no conversation history, and no
`previous_response_id`. The moment any stateful feature is needed, the plugin
would need to call out to an external service anyway — which is what an HTTP
upstream already provides. And this stateless translation is already what vSR
does for its supported providers.

### What BBR would need to support our full feature set

| BBR capability gap | What we need | Consequence of adding it |
|-------------------|-------------|------------------------|
| **Response hooks** | Process responses after the backend replies (save state, rewrite IDs, manage lifecycle events) | Doubles the plugin interface surface; plugins become bidirectional filters rather than request transformers |
| **Streaming primitives** | Incremental SSE event forwarding between backend and client, or a "take over this request" escape hatch | Fundamentally changes BBR's execution model from synchronous body transformation to asynchronous event streaming |
| **Outbound call capability** | Make HTTP calls to backends, MCP servers, embedding APIs during `Execute()` | Turns plugins from pure transformations into networked services with their own failure modes, timeouts, and retry logic |
| **Persistent state access** | Shared storage interface for conversations, responses, vector stores, files | Introduces statefulness into a stateless plugin chain; requires lifecycle management (connections, migrations, cleanup) |
| **Multi-call orchestration** | Execute multiple sequential backend calls per client request (agentic tool loops) | Requires a loop primitive that does not exist; the plugin would need to control the request lifecycle, not just transform a body |

Adding all of these would turn BBR into a general-purpose application server
framework — defeating its purpose as a lightweight, composable request
transformation chain. The right boundary is clear: BBR handles stateless
request-level transformations; services behind the gateway handle stateful,
multi-turn, streaming workloads.

### The right integration point

The integration between GIE/BBR and openresponses-gw already works without any
BBR changes: standard `HTTPRoute` matching routes `/v1/responses` traffic to
openresponses-gw as an upstream HTTP service. GIE provides scheduling and
infrastructure; openresponses-gw provides state and orchestration. No plugin
needed — just routing.

## Architecture

```
Client (Responses API)
    |
    v
+---------------------------------------------+
|  Envoy + GIE ExtProc                        |
|  (TLS, auth, rate limiting, scheduling,     |
|   model routing, LoRA affinity, BBR plugins)|
+--------------------+------------------------+
                     |
                     v
+---------------------------------------------+
|  openresponses-gw  (HTTP server)            |
|  - Responses API endpoint                   |
|  - Conversations, files, vector stores      |
|  - Agentic tool loops (MCP, file_search)    |
|  - Streaming SSE                            |
|  - Responses -> Chat Completions translation|
+--------------------+------------------------+
                     |
                     v
+---------------------------------------------+
|  vLLM / LLM backends                        |
|  (via Chat Completions or Responses API)    |
+---------------------------------------------+
```

openresponses-gw runs as an **HTTP service behind GIE**, not inside Envoy's
filter chain. Benefits:

- **All Envoy infrastructure for free** — TLS, auth, rate limiting,
  observability, circuit breaking apply uniformly without any code in our project
- **GIE scheduling** — inference-aware load balancing on backend calls
  (KV-cache affinity, LoRA routing) without any integration work
- **Full streaming and tool loops** — no ImmediateResponse hacks, no body
  buffering constraints, full SSE pass-through
- **Clean separation** — GIE handles network/scheduling; we handle
  protocol/state

The gateway can also run standalone (without GIE/Envoy) for development, testing,
or deployments that don't need inference-aware scheduling.

## Ecosystem Synergies

| Integration | Value |
|-------------|-------|
| **GIE** | Deploy as an `InferencePool` backend. GIE schedules our backend calls to vLLM with inference-aware load balancing. Multi-model routing and LoRA affinity without code. |
| **Semantic Router (vSR)** | Deployed behind GIE+vSR, our backend calls get semantically routed to the optimal model in a multi-model pool. vSR handles API translation for external providers (OpenAI, Anthropic); we handle stateful Responses API semantics. |
| **MCP Gateway** | They aggregate and federate MCP servers; we consume MCP tools in agentic loops. Our MCP connector points at their gateway instead of individual servers. |
| **Agentic Networking** | Our MCP tool calls pass through their policy layer for tool-level authorization. They enforce which tools; we handle the agentic loop. |
| **WG AI Gateway** | As the Payload Processing and Backend CRD proposals mature, they may provide declarative alternatives for some of our translation and egress logic. |

## Decision

The ExtProc adapter has been removed. openresponses-gw is an HTTP service that
provides the stateful Responses API tier. It sits alongside GIE, vSR, MCP
Gateway, and Agentic Networking as a complementary component in the Kubernetes
AI inference stack. Each project owns a distinct layer:

- **GIE**: scheduling and routing (ExtProc + BBR plugins)
- **vSR**: model selection and API translation (BBR plugin within GIE)
- **MCP Gateway**: tool aggregation and federation
- **Agentic Networking**: tool-level authorization
- **openresponses-gw**: stateful Responses API, agentic loops, protocol translation

The HTTP server is the right abstraction for stateful, agentic API surfaces.

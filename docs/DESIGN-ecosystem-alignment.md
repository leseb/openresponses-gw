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
| Response persistence | Yes (memory, Redis) | Yes (memory, SQLite, PostgreSQL) |
| Vector store backends | Memory, Milvus, Llama Stack | Memory, Milvus |
| Files API | Yes | Yes |
| Vector Stores API | Yes | Yes |
| SSE streaming (24 event types) | No | Yes |
| Agentic tool loops (multi-turn) | No | Yes (up to 10 iterations) |
| Server-side `file_search` execution | No (delegates to upstream or pre-injects via RAG plugin) | Yes (local vector store query + embedding) |
| Server-side `web_search` execution | No | Yes (Brave, Tavily) |
| MCP tool execution in agentic loop | No (MCP used for classification) | Yes |
| Citation annotations | No | Yes (url_citation, file_citation) |
| Prompts API | No | Yes |
| Conversations API (CRUD) | No | Yes |
| Connectors API (MCP registry) | No | Yes |

The Responses API's full value comes from the **agentic, streaming features**
that sit above format translation:

- **Streaming**: 24 SSE event types (`response.created`,
  `response.output_text.delta`, `response.completed`, etc.) enabling real-time
  incremental delivery
- **Agentic loops**: the engine calls the LLM, intercepts tool calls
  (`file_search`, `web_search`, MCP), executes them server-side, feeds results
  back, and repeats — up to 10 iterations per request
- **Server-side tool execution**: `file_search` queries are embedded and run
  against vector stores locally; `web_search` queries are executed via Brave or
  Tavily; MCP tools are dispatched to registered connectors
- **Citation annotations**: `url_citation` (from web_search) and `file_citation`
  (from file_search) annotations attached to output text
- **Higher-level APIs**: Prompts (versioned templates), Conversations (CRUD +
  item management), Connectors (MCP server registry)

This is the project's unique contribution to the ecosystem. vSR provides
Responses API format compatibility; openresponses-gw provides the full
stateful, agentic, streaming Responses API surface.

### Comparison with Llama Stack

[Llama Stack](https://github.com/meta-llama/llama-stack) is Meta's full-stack
platform for building AI applications. It implements the Responses API alongside
a broad set of supporting APIs. An honest comparison reveals that Llama Stack is
more feature-complete in nearly every dimension — it is a mature, multi-provider
platform while openresponses-gw is a focused gateway. Understanding the gaps
helps prioritize development.

#### Responses API core

| Capability | Llama Stack | openresponses-gw |
|------------|-------------|-----------------|
| Responses ↔ Chat Completions translation | Yes | Yes |
| `previous_response_id` chaining | Yes | Yes |
| Response persistence | Yes (SQLite, Postgres) | Yes (memory, SQLite, PostgreSQL) |
| SSE streaming | 36 event types | 24 event types |
| Agentic tool loops (multi-turn) | Yes (configurable max) | Yes (up to 10 iterations) |
| `tool_choice` options | `auto`, `required`, `none`, named function | `auto`, `required`, `none` (no named function) |
| Structured output (`text.format`) | `json_schema`, `json_object` | `json_schema`, `json_object` |
| Reasoning (`reasoning.effort`) | Mapped to `thinking.budget_tokens` | Mapped to `thinking.budget_tokens` |
| `prompt` parameter (template reference) | Yes — resolves prompt by ID, substitutes variables | No |
| `instructions` parameter | Yes | Yes |
| `parallel_tool_calls` | Forwarded to backend | Not implemented |
| `max_output_tokens` | Yes | Yes |
| `temperature`, `top_p` | Yes | Yes |
| Incremental persistence during streaming | Yes — saves state after each tool loop iteration | No — persists only after full completion |

#### Server-side tool execution

| Tool | Llama Stack | openresponses-gw |
|------|-------------|-----------------|
| `file_search` | Yes — embeds query, searches vector store, injects ranked results | Yes — same pattern |
| `web_search` | Yes — executes via Brave/Tavily, injects results | Yes — executes via Brave/Tavily, injects results, attaches url_citation annotations |
| MCP tools | Yes — with human-in-the-loop approval flows and tool listing reuse | Yes — basic execution only |
| `code_interpreter` | Not in Responses API path | Not implemented |
| `computer_use` | Not in Responses API path | Not implemented |

#### Vector Stores API

| Capability | Llama Stack | openresponses-gw |
|------------|-------------|-----------------|
| Vector store backends | 10+ (Faiss, ChromaDB, Milvus, Qdrant, pgvector, SQLite-vec, Weaviate, inline) | 2 (memory, Milvus) |
| Search types | Vector, keyword, hybrid | Vector only |
| Search filters | Working implementation (comparison + compound filters) | Schema accepted but **silently ignored** |
| Chunking strategies | Auto, static (configurable) | Auto, static |
| Embedding providers | Multiple (sentence-transformers, OpenAI, inline) | Single configurable endpoint |
| Ranking/reranking | Yes (configurable) | No |
| `file_search` annotations in output | Yes — includes file_id, filename, score | Yes — includes file_id, filename, score |

The search filter gap is a correctness issue: openresponses-gw accepts filter
parameters in search requests and returns results without applying them, which
can produce silently incorrect results.

#### Files API

| Capability | Llama Stack | openresponses-gw |
|------------|-------------|-----------------|
| Upload/retrieve/delete/list | Yes | Yes |
| Storage backends | Disk, memory, S3-compatible | Filesystem, memory, S3 |
| Content extraction | PDF, HTML, text, CSV, JSON, JSONL | PDF, HTML, text, CSV, JSON, JSONL |
| Purpose filtering | `assistants`, `fine-tune`, `user_data`, `responses` | `assistants`, `user_data` |

#### Higher-level APIs

| API | Llama Stack | openresponses-gw |
|-----|-------------|-----------------|
| Conversations (CRUD + items) | No (uses `previous_response_id` chain) | Yes |
| Prompts (versioned templates) | Yes — via `prompt` parameter on Responses API | Yes — standalone CRUD API |
| Connectors (MCP registry) | Configured via provider config | Yes — CRUD API |
| Safety / Guardrails | Yes — Llama Guard, prompt injection detection, code scanning | No |
| Access control | ABAC with resource-level `owner_type`/`owner_id` | No |

#### Architectural differences

| Aspect | Llama Stack | openresponses-gw |
|--------|-------------|-----------------|
| Language | Python (async) | Go |
| Deployment model | Multi-provider framework with pluggable backends | Single-binary HTTP service |
| Provider ecosystem | 30+ providers across inference, safety, memory, tools | Single backend (vLLM/OpenAI-compatible) |
| API specification | Hand-maintained OpenAPI spec | Auto-generated with conformance testing against OpenAI spec |
| Request path | Client → Llama Stack → vLLM (via provider) | Client → openresponses-gw → vLLM (direct HTTP) |

Both projects sit in the request path the same way: they receive Responses API
requests, translate them, and make backend inference calls. Neither is "thinner"
than the other. Both can be deployed behind Envoy/GIE if desired — that is a
deployment choice, not an architectural property of either project.

#### Key takeaways

1. **Llama Stack is more complete in some areas**: more vector store backends,
   working search filters, keyword/hybrid search, MCP approval flows,
   guardrails, ABAC, prompt parameter resolution, incremental streaming
   persistence.

2. **openresponses-gw has closed several gaps**: web_search execution (Brave,
   Tavily), content extraction (PDF, HTML, CSV, JSON/JSONL), citation
   annotations (url_citation, file_citation), pass-through inference fields
   (seed, stop, service_tier), and a generic provider registry for all pluggable
   backends. Go single-binary deployment, auto-generated OpenAPI spec with
   conformance testing, and focused simplicity remain architectural advantages.

3. **Remaining gaps**: search filters (currently silently ignored — correctness
   issue), keyword/hybrid search, prompt parameter support, incremental
   persistence during streaming, named function tool_choice, MCP approval flows.

4. **Different positioning**: Llama Stack is a full application platform with
   multi-provider support, safety, and access control. openresponses-gw is a
   focused Responses API service for environments that want to add Responses API
   support to existing vLLM deployments without adopting a full platform.

## Why an HTTP Service Is Preferred Over ExtProc

> **Update (2026-02-23):** Envoy v1.37.0 introduced
> [`StreamedImmediateResponse`](https://www.envoyproxy.io/docs/envoy/v1.37.0/api-v3/service/ext_proc/v3/external_processor.proto#envoy-v3-api-msg-service-ext-proc-v3-streamedimmediateresponse),
> which allows an ExtProc to stream locally-generated responses incrementally.
> This removes the streaming limitation that was the strongest technical
> argument against ExtProc during prototyping (the old `ImmediateResponse`
> buffered the entire response). The vSR project is pursuing this for streaming
> Chat Completions
> ([vllm-project/semantic-router#1082](https://github.com/vllm-project/semantic-router/issues/1082)).
>
> With `StreamedImmediateResponse`, ExtProc is **technically viable** for
> everything openresponses-gw does — including SSE streaming and agentic tool
> loops. The remaining arguments below are **architectural preferences**
> (decoupling, simplicity, independent failure domains) rather than hard
> technical constraints. Both ExtProc and HTTP upstream achieve the same
> deployment topology (behind Envoy with auth and rate limiting); the HTTP
> service is simpler and more decoupled.

### 1. Outbound calls bypass Envoy — solvable, but converges to HTTP upstream

The ExtProc makes its own HTTP calls to inference backends, MCP servers,
embedding APIs, and search providers. These outbound calls bypass Envoy's
backend management — no load balancing, no retries, no observability from
Envoy's perspective. With `StreamedImmediateResponse`, client-facing SSE
streaming flows through Envoy properly, but the backend calls that generate
that content do not.

**This is solvable.** Envoy supports per-route ExtProc configuration via
[`ExtProcPerRoute`](https://www.envoyproxy.io/docs/envoy/v1.37.0/api-v3/extensions/filters/http/ext_proc/v3/ext_proc.proto#envoy-v3-api-msg-extensions-filters-http-ext-proc-v3-extprocperroute).
The approach:

1. Enable the Responses API ExtProc filter globally on the listener.
2. Add a per-route override on `/v1/chat/completions` (and other backend paths)
   that sets `disabled: true`, disabling the ExtProc for those routes. These
   routes are handled by BBR+vSR as normal inference traffic.
3. The ExtProc, when processing a `/v1/responses` request, makes its backend
   LLM calls to `localhost:<envoy-port>/v1/chat/completions`. Because the
   ExtProc is disabled on that route, the request flows through Envoy's normal
   proxy pipeline (BBR, vSR, load balancing, observability) without triggering
   a recursive ExtProc invocation.
4. Alternatively, Envoy
   [internal listeners](https://www.envoyproxy.io/docs/envoy/v1.37.0/configuration/other_features/internal_listener)
   could route ExtProc outbound calls through a separate filter chain within
   the same Envoy process, avoiding the external network hop entirely.

This gives the ExtProc access to Envoy's backend management for its outbound
calls. However, **the resulting topology converges to the same architecture as
the HTTP service approach**: in both cases, a service behind Envoy makes backend
calls that flow through Envoy. The ExtProc version routes through per-route
filter chain rules and gRPC transport; the HTTP upstream version routes through
standard HTTP upstream configuration. The functional outcome is identical — the
difference is transport complexity (gRPC + per-route rules vs. plain HTTP).

An HTTP upstream service has the same outbound routing option (point the backend
URL at the Envoy endpoint) without requiring per-route ExtProc disabling rules
or internal listener configuration.

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
| **SSE streaming** | Incremental event forwarding from backend to client | No — `Execute()` operates on complete request bodies. No streaming hook exists in the BBR plugin interface. (Note: Envoy v1.37.0's `StreamedImmediateResponse` solves this at the ExtProc protocol level, but BBR plugins do not have access to it — they return bytes, not streaming responses.) |
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
| **Streaming primitives** | Incremental SSE event forwarding between backend and client, or a "take over this request" escape hatch | Fundamentally changes BBR's execution model from synchronous body transformation to asynchronous event streaming. (Note: `StreamedImmediateResponse` provides this at the ExtProc level but not within BBR's plugin chain.) |
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

openresponses-gw runs as an **HTTP service**, not inside Envoy's filter chain.
It can be deployed behind GIE/Envoy or standalone.

When deployed behind Envoy:

- **Envoy infrastructure applies to inbound traffic** — TLS termination, auth,
  rate limiting, observability on client-facing requests
- **Full streaming and tool loops** — no ImmediateResponse hacks, no body
  buffering constraints, full SSE pass-through

Note: GIE's inference-aware scheduling (KV-cache affinity, LoRA routing) does
**not** automatically apply to openresponses-gw's backend calls to vLLM. Those
calls are direct HTTP requests from our process. To get GIE scheduling on
backend calls, you would need to route them through the GIE-managed Envoy by
pointing the backend URL at the Envoy endpoint — a deployment configuration
choice, not an architectural property. Llama Stack or any other service could
be configured the same way.

The service can also run standalone (without GIE/Envoy) for development,
testing, or simpler deployments.

## Ecosystem Synergies

| Integration | Value |
|-------------|-------|
| **GIE** | Deploy behind GIE-managed Envoy. GIE handles inference-aware scheduling for inbound traffic. Backend calls from openresponses-gw to vLLM only benefit from GIE scheduling if explicitly routed through the Envoy endpoint. |
| **Semantic Router (vSR)** | vSR handles API translation for external providers (OpenAI, Anthropic) and model selection as a BBR plugin. openresponses-gw handles the stateful Responses API surface (streaming, tool loops, conversations). Both can coexist behind the same Envoy. |
| **MCP Gateway** | They aggregate and federate MCP servers; we consume MCP tools in agentic loops. Our MCP connector points at their gateway instead of individual servers. |
| **Agentic Networking** | Our MCP tool calls can pass through their policy layer for tool-level authorization. They enforce which tools; we handle the agentic loop. |
| **WG AI Gateway** | As the Payload Processing and Backend CRD proposals mature, they may provide declarative alternatives for some of our translation and egress logic. |

## Decision

The ExtProc adapter has been removed. openresponses-gw is an HTTP service that
implements the Responses API — stateful conversations, agentic tool loops,
streaming, and protocol translation. It is not a gateway in the traditional
sense (it does not route, load-balance, or enforce policy); it is a focused
Responses API service that can be deployed standalone or behind any reverse
proxy.

It sits alongside other projects in the Kubernetes AI inference ecosystem, each
covering a distinct concern:

- **GIE**: inference-aware scheduling and routing (ExtProc + BBR plugins)
- **vSR**: model selection and API translation (BBR plugin within GIE)
- **MCP Gateway**: tool aggregation and federation
- **Agentic Networking**: tool-level authorization
- **Llama Stack**: full Responses API platform with multi-provider support,
  safety, and access control
- **openresponses-gw**: focused Responses API service for existing vLLM
  deployments

The HTTP server is the right abstraction for stateful, agentic API surfaces.
Several feature gaps identified in the original Llama Stack comparison have been
closed (web_search execution, content extraction, citation annotations,
pass-through inference fields, provider registry). The remaining gaps — search
filter correctness, keyword/hybrid search, MCP approval flows, and incremental
streaming persistence — are the next priorities for reaching full parity.

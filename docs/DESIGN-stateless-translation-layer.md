# Design: Stateless Responses API Translation as a Separate Concern

**Status:** Draft
**Date:** 2026-02-24

## Context

openresponses-gw currently contains two distinct responsibilities:

1. **Stateful orchestration** — conversations, agentic tool loops, streaming SSE
   lifecycle, persistence, file_search/web_search/MCP execution, files, vector
   stores, prompts, annotations.

2. **Protocol translation** — converting Responses API requests to Chat
   Completions format (`ChatCompletionsAdapter`, ~770 lines), normalizing
   backend-specific streaming quirks (vLLM `content_index` remapping, lifecycle
   event rewriting, ~100 lines in `engine.go`).

These are orthogonal concerns. The translation layer is stateless and
request-scoped. The orchestration layer is stateful and multi-turn. Mixing them
means every new backend (Bedrock, Gemini, provider-specific formats) adds
translation code to a project whose value is statefulness and agentic
execution.

This document proposes separating stateless Responses API translation into its
own layer — and argues that vSR (the Semantic Router) is the natural home for
it.

## The Translation Problem

Translating between the Responses API and other inference protocols is
non-trivial. The Responses API differs structurally from Chat Completions:

| Aspect | Responses API | Chat Completions |
|--------|--------------|-----------------|
| Input format | `input` items (typed: `message`, `function_call`, `function_call_output`) | `messages` array (role-based) |
| Output format | `output` items with stable IDs | `choices[].message` |
| Tool calls | Output items (`type: "function_call"`) | `choices[].message.tool_calls[]` |
| Tool results | Input items (`type: "function_call_output"`) | `messages` with `role: "tool"` |
| Streaming | 24+ SSE event types with lifecycle semantics (`output_item.added`, `content_part.added`, deltas, `.done` events) | Simple `chat.completion.chunk` deltas |
| Instructions | `instructions` field | System message |

And each backend has its own quirks:

- **vLLM** emits per-token `content_index` (0, 1, 2, ...) instead of the
  standard `content_index=0` for all deltas in one content part. It also emits
  its own lifecycle events that don't match the Responses API spec.
- **Provider-specific APIs** (Bedrock, Gemini, etc.) have entirely different
  request/response schemas, auth mechanisms, and streaming formats.

This translation surface is O(backends x features) and grows independently of
stateful features.

## What openresponses-gw Contains Today

### ChatCompletionsAdapter (~770 lines)

`pkg/core/api/chat_completions_adapter.go`:

- `ConvertToChatRequest()` — Responses API request → Chat Completions request
  (input items → messages, tools mapping, tool_choice conversion, multimodal
  content handling, instructions → system message)
- `ConvertFromChatResponse()` — Chat Completions response → Responses API
  response (choices → output items, usage mapping, finish_reason → status)
- `processSSEStream()` — Chat Completions SSE chunks → Responses API SSE
  events (text delta accumulation, tool call fragment accumulation, lifecycle
  event synthesis, final response construction)
- Supporting types: `ChatCompletionRequest`, `ChatCompletionResponse`,
  `ChatCompletionChunk`, etc.

### Engine Normalizations (~100 lines)

`pkg/core/engine/engine.go` lines ~1840-1940:

- Skips backend-emitted lifecycle events and re-emits correct ones
- Rewrites `content_index` from per-token (vLLM) to standard (always 0)
- Tracks `announcedOutputs` / `announcedContent` for proper SSE event
  sequencing
- Synthesizes `output_item.added` and `content_part.added` events

These normalizations exist because the `ChatCompletionsAdapter` can't fully
handle backend-specific streaming behavior — the quirks leak through.

## Why This Should Be a Separate Layer

### 1. Different change rates

Translation code changes when:
- A new backend is added (Bedrock, Gemini, etc.)
- A backend changes its streaming behavior (vLLM update)
- The Responses API spec adds new fields

Orchestration code changes when:
- New server-side tools are added
- Persistence logic changes
- Conversation chaining evolves
- New SSE lifecycle events are needed

These are independent axes. A change to Bedrock translation should not require
touching the agentic loop.

### 2. Different execution models

Translation is **stateless and request-scoped**: one request in, one
request/stream out. No database, no conversation history, no tool execution.
This maps directly to Envoy's ExtProc model.

Orchestration is **stateful and multi-turn**: multiple backend calls per client
request, persistent state across requests, long-lived SSE connections, server-
side tool execution with external calls.

### 3. Combinatorial complexity

Each new backend adds translation code for:
- Request format mapping
- Response format mapping
- Streaming chunk translation
- Auth mechanism (Bearer tokens, SigV4 for Bedrock, API keys)
- Backend-specific quirks

This is O(backends) work. If it lives inside openresponses-gw, every backend
adds code to the wrong project. If it lives in a dedicated translator, backends
are added without touching the stateful layer.

### 4. openresponses-gw becomes simpler

With translation extracted:
- Delete `ChatCompletionsAdapter` (~770 lines)
- Delete `chat_completions_types.go`
- Delete vLLM normalization code in `engine.go` (~100 lines)
- Delete `BackendAPI` config option (always "responses")
- `ResponsesAPIClient` interface stays, only `OpenAIResponsesClient` remains
- The engine becomes purely a stateful orchestrator speaking one protocol

## vSR as the Stateless Translator

### Why vSR is a good candidate

vSR (the Semantic Router) already has both translation layers:

1. **Responses API → Chat Completions**
   (`pkg/responseapi/translator.go`, `pkg/extproc/req_filter_response_api.go`)
   — parses Responses API requests, maps fields to Chat Completions format.

2. **Chat Completions → Provider-specific formats**
   (`pkg/extproc/req_filter_openai.go`, `pkg/extproc/req_filter_anthropic.go`,
   etc.) — translates Chat Completions to Anthropic Messages API, Bedrock, and
   other provider formats.

vSR already runs as an ExtProc / BBR plugin in Envoy's data plane — exactly
where a stateless translator belongs. Its `Execute()` interface is a stateless,
single-pass body transformation:

```go
type BBRPlugin interface {
    Execute(requestBodyBytes []byte) (headers map[string]string, mutatedBodyBytes []byte, err error)
}
```

This is the right execution model for protocol translation.

### What vSR would need to add

vSR already has request-side Responses API → Chat Completions translation.
What's missing for it to be a complete stateless Responses API translator:

| Capability | vSR Status | Gap |
|---|---|---|
| Request translation (Responses → Chat Completions) | Exists (`translator.go`) | Done |
| Response translation (Chat Completions → Responses) | Not implemented | Build reverse mapping for non-streaming |
| SSE stream translation (Chat Completions chunks → Responses API events) | Not implemented | Requires `StreamedImmediateResponse` (Envoy v1.37.0) + chunk-by-chunk event rewriting |
| Provider API translation (Chat Completions → Anthropic/Bedrock/Gemini) | Exists | Done — this is vSR's core |
| vLLM quirk handling (content_index, lifecycle events) | Not in vSR | Move from openresponses-gw's engine.go |
| Native Responses API passthrough | Not needed | If backend speaks Responses API natively, translator is a no-op |

The SSE stream translation is the hardest piece. It requires:
- Accumulating Chat Completions deltas
- Synthesizing Responses API lifecycle events (`output_item.added`,
  `content_part.added`, etc.)
- Translating `chat.completion.chunk` → `response.output_text.delta`
- Accumulating tool call fragments and emitting
  `response.function_call_arguments.delta`
- Building the final `response.completed` event with full output

vSR is already tracking `StreamedImmediateResponse` support in
[vllm-project/semantic-router#1082](https://github.com/vllm-project/semantic-router/issues/1082),
which is the prerequisite for streaming translation in ExtProc.

### What vSR would drop

Per the
[separation proposal](proposal-vsr-responses-api-separation.md):

- `pkg/responsestore/` — response persistence (memory, Redis, Milvus stores).
  Persistence is a stateful concern handled by the upstream service.
- `previous_response_id` resolution — conversation chaining is stateful.
- Any attempt at agentic loops or tool execution in the Responses API path.

vSR keeps the **translation** part of its Responses API code, drops the
**stateful** part.

### What vSR keeps

- All Chat Completions ↔ provider translation
- Model selection and semantic routing
- Guardrails (PII, jailbreak, hallucination, fact-check)
- Semantic caching
- RAG pre-injection
- BBR plugin interface
- Responses API ↔ Chat Completions translation (the stateless part)

## Architecture

### Current State

```
Client (Responses API)
    |
    v
openresponses-gw
    |  Stateful: conversations, agentic loops, tools, persistence
    |  ALSO: Responses API <-> Chat Completions translation (baked in)
    |  ALSO: vLLM SSE normalization (baked in)
    |
    v
Single backend (vLLM / OpenAI-compatible, static config)
```

Translation and statefulness are tangled. Every new backend adds code here.

### Proposed State

```
Client
    |  POST /v1/responses
    v
+--------------------------------------------------------------+
|  Envoy (MaaS Gateway, inbound)                               |
|    Kuadrant auth, rate limits                                 |
|    HTTPRoute:                                                 |
|      /v1/responses ---------> openresponses-gw               |
|      /v1/files -------------> openresponses-gw               |
|      /v1/vector_stores -----> openresponses-gw               |
|      /v1/chat/completions --> BBR ext-proc (vSR) --> backend  |
+---------------------------+----------------------------------+
                            |
                            v
+--------------------------------------------------------------+
|  openresponses-gw  (STATEFUL TIER)                           |
|                                                               |
|    Conversations, agentic loops, tool execution,             |
|    streaming SSE lifecycle, persistence, files,              |
|    vector stores, prompts, MCP, annotations                  |
|                                                               |
|    When it needs inference, it emits:                         |
|      POST /v1/responses  (stateless, single-turn)            |
|                                                               |
|    NO translation code. NO backend awareness.                |
+---------------------------+----------------------------------+
                            |
                            v
+--------------------------------------------------------------+
|  Envoy (re-entry via ExtProcPerRoute or internal listener)   |
|                                                               |
|    vSR ExtProc / BBR plugin:                                 |
|                                                               |
|    Phase 1: Responses API --> Chat Completions               |
|      input items --> messages                                |
|      function tools mapping                                  |
|      instructions --> system message                         |
|      SSE stream translation (StreamedImmediateResponse)      |
|                                                               |
|    Phase 2: Chat Completions --> Provider format             |
|      Passthrough for OpenAI-compatible backends (vLLM, TGI)  |
|      Chat Completions --> Bedrock InvokeModel                |
|      Chat Completions --> Gemini generateContent             |
|      etc.                                                    |
|                                                               |
|    Credential injection (Lua filter, K8s Secrets)            |
|    Model-based routing (XBackendDestination)                 |
+---------------------------+----------------------------------+
                            |
                            v
                    LLM Backend / Provider
```

openresponses-gw always speaks Responses API to its backend. vSR translates
that to whatever the backend actually speaks. Neither project needs to know
about the other's internals.

## Complete Request Flow

```
Client: POST /v1/responses
  {"model":"meta-llama/Llama-3.3-70B-Instruct",
   "input":"Find recent papers on RAG",
   "tools":[{"type":"web_search"}],
   "previous_response_id":"resp_abc123",
   "stream":true}
        |
        v
  Envoy inbound (Kuadrant auth, rate limits)
  HTTPRoute: /v1/responses --> openresponses-gw
        |
        v
  openresponses-gw
   1. Load conversation from resp_abc123 (PostgreSQL)
   2. Reconstruct message history from conversation chain
   3. Expand web_search --> synthetic function tool
   4. Open SSE stream to client
   5. <-- response.created
      <-- response.in_progress
      <-- response.output_item.added
        |
        |  AGENTIC LOOP iteration 1:
        |
        |  POST /v1/responses (stateless, single-turn)
        |  {"model":"meta-llama/Llama-3.3-70B-Instruct",
        |   "input":[...full message history...],
        |   "tools":[{"type":"function","name":"web_search",...}],
        |   "stream":true}
        |
        v
  Envoy re-entry (ExtProcPerRoute: Responses API ext-proc enabled)
  vSR ExtProc:
   a. Responses API --> Chat Completions
      (input items --> messages, tools mapping)
   b. Chat Completions passthrough (vLLM speaks Chat Completions)
      OR Chat Completions --> provider format (if external provider)
   c. Credential injection (Lua: Bearer token from K8s Secret)
   d. Route to backend via XBackendDestination
        |
        v
  Backend returns: tool call
   {"type":"function_call", "name":"web_search",
    "arguments":"{\"query\":\"RAG papers 2026\"}"}
        |
        v
  vSR ExtProc (response path):
   a. Provider response --> Chat Completions (if needed)
   b. Chat Completions --> Responses API events
      (SSE chunk translation via StreamedImmediateResponse)
        |
        v
  openresponses-gw receives: Responses API tool call
   6. Intercept: name="web_search"
   7. Execute: Brave/Tavily API --> search results
   8. Stream to client:
      <-- response.web_search_call.in_progress
      <-- response.web_search_call.searching
      <-- response.web_search_call.completed
      <-- response.output_item.added (function_call_output)
   9. Inject search results into messages
   10. Save intermediate state (incremental persistence)
        |
        |  AGENTIC LOOP iteration 2:
        |
        |  POST /v1/responses (stateless, single-turn)
        |  {"model":"meta-llama/Llama-3.3-70B-Instruct",
        |   "input":[...messages + search results...],
        |   "stream":true}
        |
        v
  Envoy --> vSR --> backend --> vSR --> openresponses-gw
   (same translation path, LLM generates final text response)
        |
        v
  openresponses-gw:
   11. Stream final text to client:
       <-- response.output_text.delta (token by token)
   12. Attach annotations:
       <-- url_citation annotations from search results
   13. Complete lifecycle:
       <-- response.output_text.done
       <-- response.content_part.done
       <-- response.output_item.done
       <-- response.completed
   14. Persist final state to PostgreSQL
        |
        v
  Client receives complete Responses API SSE stream
  with citations, tool call records, and a response_id
  for the next turn
```

## Inference Endpoint Resolution

In this architecture, openresponses-gw no longer needs a static
`OPENAI_API_ENDPOINT` pointing at a single backend. The MaaS Gateway (Envoy +
vSR) is the inference endpoint. openresponses-gw sends every inference request
to the same Envoy re-entry point, and vSR + the routing layer handles:

- **Where** to send it (model name → XBackendDestination)
- **How** to authenticate (credential injection from K8s Secrets)
- **What protocol** to speak (Chat Completions passthrough, or provider-
  specific translation)

The `model` field in the request is the only routing signal. openresponses-gw
passes it through unchanged. It never sees provider credentials, never knows
which backend serves which model, and never needs to change when backends are
added or moved.

```bash
# The only config openresponses-gw needs:
OPENAI_API_ENDPOINT=http://envoy.maas-gateway.svc:8080/v1
```

For standalone development (no Envoy), the same endpoint points directly at a
backend that speaks Responses API natively, or at a lightweight translation
proxy.

## Why ExtProc Fits Here (But Not for openresponses-gw)

The [ecosystem alignment design](DESIGN-ecosystem-alignment.md) concluded that
ExtProc is the wrong abstraction for openresponses-gw. That conclusion stands.
But the **stateless translator** is a different problem — and ext_proc is a
natural fit for it.

| Property | Stateless translator (vSR) | Stateful orchestrator (openresponses-gw) |
|---|---|---|
| State | None | Conversations, files, vector stores, prompts |
| Execution model | Single request in, single response out | Multiple backend calls, loops, persistence |
| Outbound calls | None (Envoy forwards to backend) | Web search, MCP, embedding APIs |
| Streaming | Chunk-by-chunk SSE rewriting | Long-lived SSE connection to client |
| Fits ext_proc? | Yes | No |
| Fits BBR? | Partially (non-streaming request path) | No |

The key insight: ext_proc failed for **stateful** Responses API handling (the
agentic, streaming, persistent features). It is the right model for
**stateless** protocol translation (one request in, one response out, no side
effects).

With Envoy v1.37.0's `StreamedImmediateResponse`, ext_proc can also handle the
streaming translation path — rewriting Chat Completions SSE chunks into
Responses API SSE events on the fly without buffering.

### ExtProcPerRoute for re-entry

openresponses-gw's backend inference calls re-enter Envoy. To avoid recursive
ExtProc invocation, `ExtProcPerRoute` selectively enables the Responses API
translator on routes that need it:

1. Enable the Responses API ExtProc filter globally on the listener.
2. On `/v1/chat/completions` routes (direct client traffic), disable the
   Responses API ExtProc — these are handled by BBR+vSR as normal Chat
   Completions traffic.
3. openresponses-gw's backend calls to `/v1/responses` on the re-entry path
   pass through the Responses API ExtProc, which translates to Chat
   Completions, then continues through the normal BBR+vSR pipeline for
   provider translation and routing.

Alternatively, Envoy
[internal listeners](https://www.envoyproxy.io/docs/envoy/v1.37.0/configuration/other_features/internal_listener)
can route re-entry calls through a separate filter chain within the same Envoy
process.

## Component Responsibilities

| Component | Responsibility |
|-----------|----------------|
| Kuadrant | Inbound auth (MaaS token validation), per-user rate limiting |
| vSR (ExtProc / BBR) | Stateless translation: Responses API <-> Chat Completions <-> provider formats. Model selection. Guardrails. Semantic caching. |
| Lua filter | Provider credential injection (K8s Secret → Authorization header) |
| XBackendDestination | Egress TLS, FQDN, port for each provider |
| openresponses-gw | Stateful Responses API: conversations, agentic tool loops (up to 10 iterations), streaming SSE (24+ event types), server-side tool execution (file_search, web_search, MCP), persistence (PostgreSQL/SQLite), Files/Vector Stores/Prompts/Connectors APIs, citation annotations |

## Impact on openresponses-gw

### Code removed

| File | Lines | Content |
|------|-------|---------|
| `pkg/core/api/chat_completions_adapter.go` | ~770 | Request/response/streaming translation |
| `pkg/core/api/chat_completions_types.go` | ~120 | Chat Completions type definitions |
| `pkg/core/engine/engine.go` (normalization) | ~100 | vLLM content_index rewriting, lifecycle event management, `announcedOutputs`/`announcedContent` tracking |

### Code simplified

| Change | Effect |
|--------|--------|
| Drop `BackendAPI` config option | Always "responses" — no Chat Completions path |
| Drop `ChatCompletionsAdapter` from `engine.New()` | Only `OpenAIResponsesClient` remains |
| Remove all backend-specific SSE normalization | Engine processes standard Responses API events only |
| `ResponsesAPIClient` interface unchanged | Same interface, one implementation |

### Config simplified

```yaml
# Before: must choose backend protocol
engine:
  model_endpoint: http://vllm:8000/v1
  backend_api: chat_completions  # or "responses"

# After: always speaks Responses API, backend handles translation
engine:
  model_endpoint: http://envoy.maas-gateway.svc:8080/v1
  # backend_api is gone — always Responses API
```

## Impact on vSR

### Code retained (translation)

- `pkg/responseapi/translator.go` — Responses API → Chat Completions request
  translation
- `pkg/extproc/req_filter_response_api.go` — ExtProc filter for Responses API
  requests
- All Chat Completions ↔ provider translation filters
- Model selection, guardrails, semantic caching, RAG

### Code added

- Response-path translation: Chat Completions response → Responses API
  response format
- SSE stream translation: Chat Completions chunks → Responses API events (using
  `StreamedImmediateResponse`)
- vLLM quirk handling: `content_index` normalization, lifecycle event
  rewriting (moved from openresponses-gw)

### Code removed (stateful)

- `pkg/responsestore/` — response persistence (memory, Redis, Milvus)
- `previous_response_id` conversation chaining
- Any stateful Responses API features

## Alternatives Considered

### 1. Standalone translation service (litellm-like)

A new project or litellm extension that provides a Responses API frontend
translating to any backend.

**Pros:** No Envoy dependency. Can run standalone. Simpler to develop and test.
**Cons:** Extra network hop. Needs its own credential management. Duplicates
what vSR already does for provider translation.

### 2. Keep translation in openresponses-gw

The status quo. Each backend adds translation code to openresponses-gw.

**Pros:** Single binary, no external dependencies.
**Cons:** Translation complexity grows with each backend. vLLM-specific quirks
leak into the engine. Duplicates vSR's provider translation work. Two projects
(openresponses-gw and vSR) both doing Responses API → Chat Completions
translation independently.

### 3. vSR as the translator (proposed)

vSR handles all stateless translation. openresponses-gw handles all stateful
orchestration.

**Pros:** Each project does what it's architecturally suited for. No
duplication. vSR already has both translation layers. ext_proc is the right
execution model for stateless translation. New backends only require changes
in vSR.
**Cons:** Requires vSR to add response-path translation and SSE stream
translation (currently missing). Requires `StreamedImmediateResponse` (Envoy
v1.37.0+). Standalone development without Envoy needs a lightweight fallback.

## Open Questions

1. **Standalone development path** — without Envoy, openresponses-gw needs a
   backend that speaks Responses API. Options: (a) keep a minimal translation
   shim for dev mode, (b) use a native Responses API backend (OpenAI, future
   vLLM native support), (c) run a lightweight translation proxy alongside.

2. **SSE stream translation complexity** — the streaming translation (Chat
   Completions chunks → Responses API events) is the hardest piece. It requires
   `StreamedImmediateResponse` support in vSR and correct lifecycle event
   synthesis. This is active work in
   [vllm-project/semantic-router#1082](https://github.com/vllm-project/semantic-router/issues/1082).

3. **Native Responses API backends** — as backends add native Responses API
   support (vLLM is exploring this), the translator becomes a passthrough. The
   architecture should handle this gracefully — if the backend already speaks
   Responses API, vSR detects this and skips translation.

4. **Guardrails for Responses API traffic** — vSR's guardrails (PII, jailbreak
   detection) are valuable for Responses API traffic too. With this
   architecture, guardrails apply at the ExtProc level before/after translation,
   so both Chat Completions and Responses API traffic benefit from the same
   guardrail pipeline.

5. **Transition plan** — openresponses-gw's `ChatCompletionsAdapter` cannot be
   removed until vSR's response-path and streaming translation are complete.
   During the transition, both paths coexist: openresponses-gw can use its
   built-in adapter for direct backend access, or speak Responses API through
   vSR when deployed behind Envoy.

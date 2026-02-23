# Proposal: Clean Separation of Responses API Responsibilities

**Author:** Sebastien Han
**Date:** 2026-02-19
**Status:** Draft for discussion

## Problem

Three projects in the ecosystem currently implement overlapping subsets of the
OpenAI Responses API:

| Project | Scope | Limitations |
|---------|-------|-------------|
| **vSR** | Translation (Responses → Chat Completions), persistence, `previous_response_id` chaining | No SSE streaming, no agentic tool loops, no server-side tool execution. Built-in tools (`web_search`, `mcp`, `code_interpreter`) stripped during translation. |
| **Llama Stack** | Full Responses API surface: 36 SSE event types, agentic tool loops, server-side `file_search`/`web_search`/MCP execution, prompt resolution, ABAC, guardrails | Full application platform — large dependency surface |
| **openresponses-gw** | Mid-level: 24 SSE event types, agentic tool loops, `file_search`/MCP execution, Conversations/Prompts/Connectors APIs | Fewer vector store backends, search filters accepted but not applied, no `web_search` execution |

This fragmentation creates confusion: users don't know which project to use,
and each implements a different subset. A user who starts with vSR's Responses
API support hits a cliff when they need streaming or tool loops.

## vSR's core value is not Responses API

vSR's strongest contributions are:

- **Model selection** — semantic classification to route requests to the right
  model in a multi-model pool
- **API translation at the Chat Completions level** — field mapping between
  OpenAI, Anthropic, and other provider formats
- **Guardrails** — PII detection, jailbreak/prompt guard, hallucination
  detection, fact-checking
- **Semantic caching** — cache hits based on semantic similarity
- **RAG integration** — pre-injection of context at the routing level

These are all **stateless request transformations** that fit the BBR plugin
model perfectly. They operate on a single request, produce a transformed
request, and don't need to maintain state or make multiple backend calls.

The Responses API is fundamentally different. It requires:

- **Streaming** — 24-36 SSE event types delivered incrementally as the backend
  generates tokens
- **Agentic tool loops** — multiple sequential LLM calls per client request
  (call LLM → intercept tool call → execute tool → call LLM again, up to N
  iterations)
- **Server-side tool execution** — `file_search` (embed query, search vector
  store, inject results), `web_search` (call search API, inject results), MCP
  (dispatch to registered servers)
- **Persistent state** — conversations, responses, vector stores, files across
  requests
- **Response processing** — save state, rewrite IDs, manage lifecycle events
  after the backend responds

None of these fit `Execute(requestBodyBytes) → (headers, mutatedBytes, err)`.

## What vSR implements today

Based on a code review of vSR's Responses API implementation:

**Translation layer** (`pkg/responseapi/translator.go`,
`pkg/extproc/req_filter_response_api.go`):
- Parses Responses API request, maps fields to Chat Completions format
- Forwards `model`, `input` → `messages`, `instructions` → system message,
  `temperature`, `top_p`, `max_output_tokens`, `stream`
- Converts function-type tools; strips built-in tools (`web_search`, `mcp`,
  `code_interpreter`)

**Persistence** (`pkg/responsestore/`):
- `ResponseStore` interface with memory, Redis, Milvus backends
- Stores responses with `previous_response_id` linkage
- Linked-list traversal for conversation history reconstruction

**What it does not do:**
- No Responses API SSE events (`response.created`, `response.output_text.delta`,
  etc.) — `stream` param forwarded to Chat Completions but response is not
  streamed as Responses API events
- No tool execution — tool calls returned to client, not re-submitted to LLM
- No agentic loops — single request → single response
- No server-side `file_search`, `web_search`, or MCP execution in the Responses
  API path

This is a **compatibility shim**: it lets clients send Responses API format and
get a response back, but the response is a single-turn Chat Completions result
wrapped in Responses API format. The stateful, streaming, agentic features that
define the Responses API's value are not implemented.

## The BBR constraint

vSR is being integrated into GIE as a BBR plugin. The BBR plugin interface:

```go
type BBRPlugin interface {
    plugins.Plugin
    RequiresFullParsing() bool
    Execute(requestBodyBytes []byte) (headers map[string]string, mutatedBodyBytes []byte, err error)
}
```

This is a stateless, single-pass request body transformation. vSR's current
Responses API translation fits this model because it is already stateless
single-turn translation. But this also means vSR's Responses API support
**cannot grow** into streaming, tool loops, or stateful features without
breaking the BBR abstraction.

The Responses API features that users actually need — streaming and agentic
tool execution — will always require a separate service.

## Proposal: clean routing boundary

**vSR keeps what it does well:**
- Chat Completions API translation (OpenAI, Anthropic, etc.)
- Model selection and semantic routing
- Guardrails (PII, jailbreak, hallucination, fact-check)
- Semantic caching
- RAG pre-injection

**Responses API handled by a dedicated service:**
- Full SSE streaming
- Agentic tool loops with server-side execution
- `file_search`, `web_search`, MCP tool dispatch
- Conversation persistence and `previous_response_id` chaining
- Vector Stores, Files, and supporting APIs

**Routing:**

```
Client
  |
  v
Envoy + GIE
  |
  |-- /v1/chat/completions --> GIE ExtProc + vSR (BBR plugin)
  |                            (model selection, translation, guardrails)
  |
  |-- /v1/responses ---------> Responses API service
  |                            (streaming, tool loops, state)
  |
  |-- /v1/vector_stores -----> Responses API service (or shared store)
  |-- /v1/files -------------> Responses API service (or shared store)
```

Standard `HTTPRoute` rules. No plugin integration needed. Each service handles
what it's architecturally suited for.

### What vSR could drop

- `pkg/responseapi/` — translator, types, content types
- `pkg/responsestore/` — response persistence (memory, Redis, Milvus stores)
- `pkg/extproc/req_filter_response_api.go` — Response API ExtProc filter
- `pkg/apiserver/route_responses.go` — Response API HTTP handler (if it exists
  as a standalone route)
- Response API routes in `pkg/apiserver/server.go`

vSR would still handle `/v1/files` and `/v1/vector_stores` if those are needed
independently of the Responses API, or those could also be consolidated into
the Responses API service.

### What vSR keeps

- All Chat Completions translation (`req_filter_openai.go`,
  `req_filter_anthropic.go`, etc.)
- Model selection and routing logic
- Guardrails (PII, jailbreak, hallucination, fact-check)
- Semantic caching
- RAG integration
- BBR plugin interface

### Llama Stack as the Responses API service

[Llama Stack](https://github.com/meta-llama/llama-stack) already implements the
most complete Responses API surface in the ecosystem:

- 36 SSE event types with incremental persistence during streaming
- Agentic tool loops with configurable iteration limits
- Server-side `file_search` with 10+ vector store backends, keyword/hybrid
  search, working search filters
- Server-side `web_search` execution (Brave, Tavily)
- MCP tool execution with human-in-the-loop approval flows
- Prompt parameter resolution with variable substitution
- ABAC access control with resource-level ownership
- Guardrails (Llama Guard, prompt injection detection)

It can be deployed as the `/v1/responses` upstream behind Envoy/GIE, receiving
traffic that vSR's routing layer directs to it.

## Benefits

1. **No more fragmentation** — one project owns the full Responses API surface
   instead of three partial implementations
2. **vSR stays focused** — model selection, translation, and guardrails are its
   core value; Responses API compatibility was a detour from that
3. **BBR integration is cleaner** — vSR as a BBR plugin handles stateless
   request transformations without carrying stateful API baggage
4. **Users get a clear answer** — "use vSR for routing and guardrails, use
   Llama Stack for Responses API" instead of "vSR does some of it but not
   streaming or tools"
5. **Less code to maintain** — vSR drops ~2000+ lines of Responses API code
   that will always be a partial implementation

## Open questions

1. **Files and Vector Stores APIs** — should these stay in vSR as shared
   infrastructure, or consolidate into the Responses API service? If vSR's RAG
   plugin uses vector stores independently of the Responses API, keeping them
   in vSR may make sense.

2. **Response persistence for routing** — vSR uses `previous_response_id` to
   reconstruct conversation history for translation. If Responses API moves to
   a separate service, vSR no longer needs its own response store — the
   Responses API service handles persistence and conversation chaining.

3. **Transition period** — should vSR's Responses API support be deprecated
   immediately or maintained as a compatibility shim until the dedicated service
   is deployed in the same environments?

4. **Guardrails integration** — vSR's guardrails (PII, jailbreak detection) are
   valuable for Responses API traffic too. With routing separation, guardrails
   could apply at the Envoy level (before routing) so both Chat Completions and
   Responses API traffic benefit.

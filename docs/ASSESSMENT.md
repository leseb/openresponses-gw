# Staff Engineer Assessment: Project Direction

## The thesis

The gateway's thesis is: **inference backends handle generation, we handle everything else**. This is a clean separation of concerns and architecturally sound. The question is whether it holds up against reality.

## vLLM's Responses API: current state

vLLM ships `/v1/responses` since v0.10.0 (mid-2025) and has been iterating aggressively. Here is what actually works today (v0.15.1):

**Works well:**

- Non-streaming responses with tool calling
- Streaming text generation (all event types)
- Streaming tool calls for Harmony/gpt-oss models
- Reasoning output with streaming events
- Image input
- Multi-turn via `previous_response_id` (with vLLM's own response store)

**Broken or incomplete:**

- Streaming tool calls for non-Harmony models emit wrong event types ([#29725](https://github.com/vllm-project/vllm/issues/29725) -- open)
- Multi-turn breaks for some agents on the second turn ([#33089](https://github.com/vllm-project/vllm/issues/33089) -- open)
- MCP tool execution without Harmony may not work ([#28173](https://github.com/vllm-project/vllm/issues/28173) -- open)
- `reasoning_tokens` always reports zero ([#33512](https://github.com/vllm-project/vllm/issues/33512) -- open)
- Many sampling parameters from chat completions are not available yet ([#32850](https://github.com/vllm-project/vllm/issues/32850))
- Structured output limited to `json_schema` (grammar/regex/choice [PR #33709](https://github.com/vllm-project/vllm/pull/33709) not merged)
- Multi-modal inputs (video) unsupported in Responses API ([#32685](https://github.com/vllm-project/vllm/issues/32685))

## Where the gateway adds clear value

These are capabilities vLLM will never implement because they are outside the scope of an inference server:

1. **Persistent storage** -- vLLM's response/message stores are in-memory and ephemeral. They do not survive restarts, cannot be shared across replicas, and are not designed for production state management. The gateway's storage (with planned PostgreSQL/Redis) solves this.

2. **Files API + Vector Stores API** -- vLLM does not have these. Upload documents, chunk, embed, search -- this is application-layer functionality. The gateway's Milvus-backed vector search and pluggable file storage (filesystem, S3) are real differentiators.

3. **Server-side file_search** -- Executing RAG queries over the gateway's own vector stores, feeding results back to the LLM. vLLM cannot do this because it does not own the document store.

4. **Conversations API** -- Multi-turn state management as a first-class API. vLLM's `previous_response_id` works within a single process; the gateway provides durable, queryable conversation history.

5. **Prompts API** -- Versioned prompt templates with default version management. Not in vLLM's scope.

6. **Connectors API** -- MCP server registry and discovery. vLLM has MCP support but does not have a connector management API.

## Where the overlap is concerning

This is the part that needs honest scrutiny.

### MCP tool execution -- who owns the loop?

vLLM is building its own MCP tool execution (merged in v0.13-0.14, with streaming in v0.15). The gateway also runs an agentic loop with MCP tools. If both layers execute tools, you get a double-loop problem: the gateway intercepts function_call events, executes MCP tools, and re-sends to the backend, while vLLM might also be trying to execute tools internally.

Currently the gateway sends `store: false` to the backend and manages its own tool loop. This works, but it means the gateway must perfectly parse every streaming event type to detect tool calls -- and vLLM's streaming tool call events are broken for non-Harmony models. This is a fragile dependency.

**Recommendation:** The gateway should be explicit about its role. For tools the gateway owns (file_search over its vector stores, MCP connectors registered with the gateway), the gateway should execute them. For tools the backend owns (built-in vLLM MCP tools), the gateway should pass through. Right now the boundary is not well-defined.

### Multi-turn -- who owns state?

The gateway chains conversation history by building `[]api.Message` from stored responses and converting them to Responses API input. vLLM also supports `previous_response_id` with its own internal store. By setting `store: false`, the gateway tells vLLM not to store anything -- so the gateway owns state. This is correct, but it means the gateway must faithfully reconstruct the full conversation context on every request. If the format conversion (`convertMessagesToResponsesInput`) has subtle mismatches with what vLLM expects, multi-turn will break silently.

### Streaming event forwarding -- brittleness

The gateway forwards raw SSE events from vLLM and patches `response_id`. This zero-copy approach is good in theory, but it couples the gateway tightly to vLLM's event format. vLLM's event types are still evolving (new reasoning events, annotation events, etc.), and the gateway's `extractEventType` switch statement needs to keep up. More importantly, the gateway's agentic loop depends on parsing `response.completed` events to extract final output -- if vLLM changes the payload shape, the gateway breaks.

## What is heading in the right direction

1. **The stateful-layer positioning** is correct and durable. Inference servers will not become application platforms. Someone needs to own state, tools, and RAG. This gateway fills that role.

2. **Dropping chat completions translation** was the right call. The old translation layer was lossy and would have become increasingly painful to maintain as the Responses API evolves. Direct forwarding is cleaner.

3. **The Open Responses spec alignment** is well-timed. The spec has real backing (OpenAI, Hugging Face, vLLM), and being an early, compliant implementation creates positioning value.

4. **The adapter pattern** (HTTP + Envoy ExtProc) gives deployment flexibility without duplicating business logic.

## What needs rethinking

### 1. The agentic loop ownership needs a clear policy

Today the gateway always intercepts tool calls and runs its own loop. It should instead:

- Execute tools it owns (file_search, gateway-registered MCP connectors)
- Pass through tools it does not own (client-side function tools, backend-native tools)
- Have a clear way to distinguish between these categories

### 2. Resilience against vLLM's immaturity

The streaming tool call bug ([#29725](https://github.com/vllm-project/vllm/issues/29725)) means the gateway's streaming agentic loop will silently fail for non-Harmony models (Qwen3, Llama, Mistral). The gateway should either detect this and fall back, or document the limitation clearly.

### 3. Production readiness gaps are the real blocker

The API surface is feature-rich, but the infrastructure is immature:

- In-memory-only storage (no PostgreSQL/Redis)
- No metrics or tracing

Auth, rate limiting, TLS termination, and similar concerns are explicitly out of scope -- they belong to the deployment layer (Envoy, API gateway, service mesh). The gateway should not duplicate what a reverse proxy already provides.

What the gateway does need to own:

- **Durable storage backends** -- PostgreSQL or Redis so state survives restarts and works across replicas
- **Observability** -- Prometheus metrics and OpenTelemetry tracing so operators can monitor the gateway in production

## Bottom line

The project is heading in the right direction architecturally. The stateful-layer thesis is sound, the API surface is comprehensive, and the Open Responses spec alignment is well-timed. The main risks are:

1. **vLLM's Responses API is still unstable** -- streaming tool calls and multi-turn have open bugs that directly affect the gateway
2. **Tool execution overlap** with vLLM's own MCP support needs a clear boundary
3. **Production readiness** (persistent storage, observability) is the gap that matters most for adoption

The priority should be making the existing functionality production-grade (persistent storage, metrics/tracing) rather than expanding the API surface further.

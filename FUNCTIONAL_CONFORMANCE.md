# Functional Conformance Status

**Last Updated:** 2026-02-10

This document tracks the **actual implementation status** of the Responses API, distinguishing between:
- ✅ **Fully Implemented** - Parameter works as expected
- ⚠️ **Schema Only** - Accepted and echoed, but NOT used in LLM calls
- 🔄 **Mocked** - Simulated behavior for testing
- ❌ **Not Implemented** - Not supported at all

---

## API Conformance Summary

| Metric | Score | Notes |
|--------|-------|-------|
| **OpenAPI Schema Conformance** | 99.5% | OpenAPI spec matches OpenAI |
| **Functional Conformance** | ~85% | Full parameter passthrough, real tool calling, multi-turn |
| **Endpoint Coverage** | 100% | All 41 endpoints schema-complete |

---

## Responses API - Parameter Implementation

### ✅ Fully Implemented (16 parameters)

| Parameter | Status | Implementation |
|-----------|--------|----------------|
| `model` | ✅ | Passed to LLM backend |
| `input` | ✅ | Parsed as string, message array, function_call, or function_call_output items |
| `instructions` | ✅ | Converted to system message |
| `temperature` | ✅ | Passed to LLM via openai-go SDK |
| `max_output_tokens` | ✅ | Passed as `max_completion_tokens` (preferred) with fallback to `max_tokens` |
| `top_p` | ✅ | Passed to LLM via openai-go SDK |
| `frequency_penalty` | ✅ | Passed to LLM via openai-go SDK |
| `presence_penalty` | ✅ | Passed to LLM via openai-go SDK |
| `tools` | ✅ | Function tools converted and passed to LLM; real tool calls returned |
| `tool_choice` | ✅ | Supports "none", "auto", "required", and named function choice |
| `parallel_tool_calls` | ✅ | Passed to LLM via openai-go SDK |
| `previous_response_id` | ✅ | Loads stored conversation history for multi-turn |
| `reasoning` | ✅ | Effort mapped to openai-go SDK `reasoning_effort` |
| `prompt_cache_key` | ✅ | Passed to LLM via openai-go SDK |
| `safety_identifier` | ✅ | Passed to LLM via openai-go SDK |
| `max_tool_calls` | ✅ | Controls agentic loop iteration limit (default 10) |

**Code Location:** `pkg/core/engine/engine.go` (`buildLLMRequest()`) and `pkg/core/api/openai_client.go` (`buildParams()`)

---

### ⚠️ Schema Only - NOT Passed to LLM (5 parameters)

These are **accepted, validated, and echoed** in the response, but **NOT used** in LLM calls:

| Parameter | Echoed? | Why Not Used |
|-----------|---------|--------------|
| `truncation` | ✅ | No direct chat completions equivalent |
| `top_logprobs` | ✅ | Passed to SDK but logprobs not surfaced in response |
| `service_tier` | ✅ | OpenAI-specific billing, not applicable to all backends |
| `background` | ✅ | Async processing not yet implemented |
| `store` | ✅ | Session storage only, not LLM param |

**Note:** `metadata` and `include` are correctly not passed to LLM (they are gateway-level params).

---

### ✅ Real Tool Calling

Tool calling is fully implemented with an agentic loop:

1. **Function tools** (`type="function"`) are converted to chat completion tool parameters
2. Tools are passed to the LLM via the openai-go SDK
3. When the LLM returns `finish_reason: "tool_calls"`, function_call output items are emitted
4. Function tools are client-side — the loop breaks to let the client execute and send results back
5. Clients send results via `function_call_output` items in the input array
6. The agentic loop respects `max_tool_calls` (default 10) and `max_output_tokens` budget

**Streaming tool calls** emit proper SSE events:
- `response.function_call_arguments.delta` — argument chunks as they arrive
- `response.function_call_arguments.done` — final arguments
- `response.output_item.added` / `response.output_item.done` — tool call items

**Code Location:** `pkg/core/engine/engine.go` (agentic loop in `ProcessRequest()` and `ProcessRequestStream()`)

---

### ✅ Multi-Turn Conversations

Multi-turn is fully implemented via `previous_response_id`:

1. When `previous_response_id` is set, the engine loads the stored response
2. Conversation messages from the previous response are reconstructed
3. Previous output items (messages, function_calls, function_call_output) are appended as context
4. Instructions are prepended as a system message (if not already present)
5. Current input is appended
6. All messages are stored with the response for the next turn in the chain

**Code Location:** `pkg/core/engine/engine.go` (`buildConversationMessages()`)

---

### ❌ Not Implemented

| Feature | Status | Notes |
|---------|--------|-------|
| RAG / Vector Store integration | ❌ | Endpoints exist but return empty/stub data |
| File attachments in input | ❌ | Schema accepts but not processed |
| `file_search` / `web_search` tools | ❌ | Only `function` type tools are supported |
| Background/async processing | ❌ | `background` param echoed but not used |

---

## Endpoint Implementation Status

### Responses API (6/6 endpoints)

| Endpoint | Status | Notes |
|----------|--------|-------|
| POST /v1/responses | ✅ | Non-streaming & streaming (24 SSE events) |
| GET /v1/responses | ✅ | List with pagination (after, before, limit, order, model) |
| GET /v1/responses/{id} | ✅ | Retrieve stored response |
| DELETE /v1/responses/{id} | ✅ | Delete response |
| GET /v1/responses/{id}/input_items | ✅ | Retrieve input items |
| POST /responses | ✅ | Alias for /v1/responses (Open Responses spec) |

**Functional Status:**
- Request validation: ✅ OpenAPI schema enforced
- Response format: ✅ 100% spec compliant
- LLM integration: ✅ Full parameter passthrough
- Tool calling: ✅ Real tool calls via agentic loop
- Multi-turn: ✅ Conversation history loaded from previous responses
- Streaming: ✅ Including tool call deltas and incremental persistence

### Conversations API (6/6 endpoints)

| Endpoint | Status | Notes |
|----------|--------|-------|
| POST /v1/conversations | ✅ | Create conversation |
| GET /v1/conversations | ✅ | List with pagination |
| GET /v1/conversations/{id} | ✅ | Get conversation |
| DELETE /v1/conversations/{id} | ✅ | Delete conversation |
| POST /v1/conversations/{id}/items | ✅ | Add conversation items |
| GET /v1/conversations/{id}/items | ✅ | List conversation items |

### Models API (2/2 endpoints)

| Endpoint | Status | Notes |
|----------|--------|-------|
| GET /v1/models | ✅ | Returns available models |
| GET /v1/models/{id} | ✅ | Get specific model details |

### Prompts API (7/7 endpoints) — versioned, llama-stack pattern

| Endpoint | Status | Notes |
|----------|--------|-------|
| POST /v1/prompts | ✅ | Create prompt template (version 1) |
| GET /v1/prompts | ✅ | List prompts (default version of each) |
| GET /v1/prompts/{id} | ✅ | Get prompt (default or `?version=N`) |
| PUT /v1/prompts/{id} | ✅ | Update prompt (creates new version; `version` field required) |
| DELETE /v1/prompts/{id} | ✅ | Delete prompt (all versions) |
| GET /v1/prompts/{id}/versions | ✅ | List all versions of a prompt |
| POST /v1/prompts/{id}/default_version | ✅ | Set the default version |

### Files API (5/5 endpoints)

| Endpoint | Status | Notes |
|----------|--------|-------|
| POST /v1/files | ✅ | Upload works (multipart) |
| GET /v1/files | ✅ | List with pagination |
| GET /v1/files/{id} | ✅ | Metadata retrieval works |
| GET /v1/files/{id}/content | ✅ | Download works |
| DELETE /v1/files/{id} | ✅ | Deletion works |

**Limitation:** Files uploaded but not used in responses (no multimodal support yet).

### Vector Stores API (14/14 endpoints)

| Endpoint | Status | Notes |
|----------|--------|-------|
| POST /v1/vector_stores | ✅ | Create works |
| GET /v1/vector_stores | ✅ | List works |
| GET /v1/vector_stores/{id} | ✅ | Get works |
| PUT /v1/vector_stores/{id} | ✅ | Update works |
| DELETE /v1/vector_stores/{id} | ✅ | Delete works |
| POST /v1/vector_stores/{id}/files | ✅ | Add file works |
| GET /v1/vector_stores/{id}/files | ✅ | List files works |
| GET /v1/vector_stores/{id}/files/{file_id} | ✅ | Get file works |
| DELETE /v1/vector_stores/{id}/files/{file_id} | ✅ | Delete file works |
| GET /v1/vector_stores/{id}/files/{file_id}/content | ✅ | Get content works |
| POST /v1/vector_stores/{id}/search | 🔄 | Endpoint works but returns stub data |
| POST /v1/vector_stores/{id}/file_batches | ✅ | Create batch works |
| GET /v1/vector_stores/{id}/file_batches/{batch_id} | ✅ | Get batch works |
| GET /v1/vector_stores/{id}/file_batches/{batch_id}/files | ✅ | List batch files works |
| POST /v1/vector_stores/{id}/file_batches/{batch_id}/cancel | ✅ | Cancel batch works |

**Limitations:**
- Search functionality: ❌ No actual vector embeddings or similarity search
- RAG integration: ❌ Not connected to responses API

---

## Testing Coverage

| Test Type | Status | Coverage |
|-----------|--------|----------|
| **OpenAPI Schema** | ✅ | 99.5% conformant |
| **Smoke Tests** | ✅ | 9 test suites pass |
| **Unit Tests** | ⚠️ | Limited coverage |
| **Integration Tests** | ⚠️ | Basic scenarios only |

---

## Known Gaps vs OpenAI

| Feature | OpenAI | This Gateway | Gap |
|---------|--------|--------------|-----|
| Parameter support | ~40 params | 16 functional | Non-LLM params remaining |
| Tool calling | ✅ Real | ✅ Real | ✅ Parity for function tools |
| Multi-turn | ✅ Real | ✅ Real | ✅ Parity |
| RAG/Search | ✅ Real | ❌ Stub | Not implemented |
| Vision | ✅ Real | ❌ None | Not implemented |
| Streaming | ✅ Real | ✅ Real | ✅ Works with tool calls |

---

## Architecture

```
┌────────────────────────────────────────────────────┐
│ Responses API Request                              │
│ (18+ parameters accepted via OpenAPI schema)       │
└─────────────────┬──────────────────────────────────┘
                  ↓
┌────────────────────────────────────────────────────┐
│ Engine.ProcessRequest()                            │
│ • Echoes all params to response ✅                 │
│ • Builds conversation from previous_response_id ✅ │
│ • Parses input items (message, function_call,      │
│   function_call_output) ✅                         │
│ • Agentic loop with token budget ✅                │
└─────────────────┬──────────────────────────────────┘
                  ↓
┌────────────────────────────────────────────────────┐
│ ChatCompletionRequest (full passthrough)           │
│ • model, messages, temperature, top_p              │
│ • frequency_penalty, presence_penalty              │
│ • max_completion_tokens, tools, tool_choice        │
│ • parallel_tool_calls, reasoning_effort            │
│ • prompt_cache_key, safety_identifier              │
└─────────────────┬──────────────────────────────────┘
                  ↓
┌────────────────────────────────────────────────────┐
│ OpenAI Client (openai-go SDK v1.12.0)              │
│ • Full parameter passthrough                       │
│ • Tool call extraction from responses              │
│ • Tool call delta handling in streaming             │
└─────────────────┬──────────────────────────────────┘
                  ↓
            [LLM Backend]
```

---

## Version History

- **2026-02-10**: Major functional upgrade
  - Full parameter passthrough (16/18 params functional, up from 5)
  - Real tool calling with agentic loop (removed mock)
  - Multi-turn conversations via previous_response_id
  - Streaming tool call support (delta/done events)
  - Incremental persistence during streaming
  - Input array parsing (message, function_call, function_call_output)

- **2026-02-10**: Updated endpoint coverage
  - Added 3 missing Responses API endpoints (list, delete, input_items)
  - All 41 endpoints now schema-complete (100%)

- **2026-02-09**: Initial functional conformance audit
  - OpenAPI schema: 99.5% ✅
  - Functional implementation: ~35% ⚠️
  - Gap identified and documented

---

## See Also

- [CONFORMANCE.md](./CONFORMANCE.md) - Open Responses spec conformance (100%)
- [CONFORMANCE_STATUS.md](./CONFORMANCE_STATUS.md) - OpenAPI conformance vs OpenAI
- [TESTING.md](./TESTING.md) - Testing guide
- [PROJECT_PLAN.md](./PROJECT_PLAN.md) - Implementation roadmap

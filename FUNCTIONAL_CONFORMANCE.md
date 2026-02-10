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
| **Functional Conformance** | ~35% | Many params accepted but ignored |
| **Endpoint Coverage** | 100% | All 41 endpoints schema-complete |

---

## Responses API - Parameter Implementation

### ✅ Fully Implemented (5 parameters)

| Parameter | Status | Implementation |
|-----------|--------|----------------|
| `model` | ✅ | Passed to LLM backend |
| `input` | ✅ | Converted to messages, passed to LLM |
| `instructions` | ✅ | Converted to system message |
| `temperature` | ✅ | Passed to LLM as-is |
| `max_output_tokens` | ✅ | Passed to LLM as `max_tokens` |

**Code Location:** `pkg/core/engine/engine.go:148-156`

```go
llmReq := &api.ChatCompletionRequest{
    Model:       model,           // ✅ Used
    Messages:    messages,        // ✅ Used
    Temperature: req.Temperature, // ✅ Used
    MaxTokens:   req.MaxOutputTokens, // ✅ Used
    Stream:      false,
}
```

---

### ⚠️ Schema Only - NOT Passed to LLM (13 parameters)

These are **accepted, validated, and echoed** in the response, but **NOT used** in LLM calls:

| Parameter | Echoed? | Why Not Used |
|-----------|---------|--------------|
| `top_p` | ✅ Line 94 | Not in ChatCompletionRequest struct |
| `frequency_penalty` | ✅ Line 106 | Not in ChatCompletionRequest struct |
| `presence_penalty` | ✅ Line 109 | Not in ChatCompletionRequest struct |
| `truncation` | ✅ Line 112 | No direct chat completions equivalent |
| `top_logprobs` | ✅ Line 122 | Not in ChatCompletionRequest struct |
| `service_tier` | ✅ Line 125 | OpenAI-specific billing, not applicable |
| `background` | ✅ Line 128 | Not in ChatCompletionRequest struct |
| `parallel_tool_calls` | ✅ Line 100 | Not in ChatCompletionRequest struct |
| `store` | ✅ Line 103 | Session storage only, not LLM param |
| `prompt_cache_key` | ✅ Line 131 | Not in ChatCompletionRequest struct |
| `safety_identifier` | ✅ Line 132 | Not in ChatCompletionRequest struct |
| `metadata` | ✅ Line 133 | Stored locally, not sent to LLM |
| `include` | ✅ | Response filtering only, not LLM param |

**Impact:** Users can set these parameters, get them echoed back, but they have **no effect** on LLM behavior.

---

### 🔄 Mocked/Simulated (2 features)

| Feature | Status | Implementation |
|---------|--------|----------------|
| `tools` | 🔄 Mocked | **Fake tool calls generated** (line 174-189)<br/>Does NOT actually call LLM with tools |
| `tool_choice` | 🔄 Echoed | Accepted but no real tool calling |

**Code Location:** `pkg/core/engine/engine.go:174-189`

```go
// If tools are provided, simulate a function call (for testing)
if len(req.Tools) > 0 {
    // Generate a function call output for the first tool
    tool := req.Tools[0]
    funcArgs := `{"location":"San Francisco, CA"}` // 🔄 HARDCODED!
    resp.Output = []schema.ItemField{
        {
            Type:      "function_call",
            Name:      &tool.Name,
            Arguments: &funcArgs, // 🔄 NOT FROM LLM!
        },
    }
}
```

**Impact:** Tool calling appears to work, but returns **fake data** without consulting the LLM.

---

### ❌ Not Implemented

| Feature | Status | Notes |
|---------|--------|-------|
| `previous_response_id` | ❌ | Stored but conversation history not loaded (line 137-144) |
| Multi-turn conversations | ❌ | Each request is independent |
| RAG / Vector Store integration | ❌ | Endpoints exist but return empty/stub data |
| File attachments in input | ❌ | Schema accepts but not processed |

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
- LLM integration: ✅ Translates to chat completions
- Parameter passthrough: ⚠️ Only 5/18 params actually used in LLM calls

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

### Prompts API (5/5 endpoints)

| Endpoint | Status | Notes |
|----------|--------|-------|
| POST /v1/prompts | ✅ | Create prompt template |
| GET /v1/prompts | ✅ | List prompts |
| GET /v1/prompts/{id} | ✅ | Get prompt |
| PUT /v1/prompts/{id} | ✅ | Update prompt |
| DELETE /v1/prompts/{id} | ✅ | Delete prompt |

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
| **Parameter Tests** | ❌ | No tests for ignored params |

---

## Recommendations

### High Priority Fixes

1. **Implement Core Parameters** (affects all users)
   - `top_p`, `frequency_penalty`, `presence_penalty`
   - Add to `ChatCompletionRequest` struct
   - Pass through to OpenAI SDK

2. **Fix Tool Calling** (currently broken)
   - Remove mock at line 174-189
   - Actually pass tools to LLM
   - Return real tool call results

3. **Document Limitations** (user expectations)
   - Add warnings to API docs
   - Return errors for unsupported features?
   - Or silently ignore (current behavior)

### Medium Priority

4. **Multi-turn Conversations**
   - Implement `previous_response_id` loading
   - Build conversation history from stored responses

5. **Add Parameter Tests**
   - Verify each param actually affects LLM output
   - Test that ignored params are documented

### Low Priority

6. **Advanced Features**
   - Response format (JSON mode)
   - Seed for reproducibility
   - Stop sequences
   - Log probabilities

---

## Known Gaps vs OpenAI

| Feature | OpenAI | This Gateway | Gap |
|---------|--------|--------------|-----|
| Parameter support | ~40 params | 5 functional | 87% ignored |
| Tool calling | ✅ Real | 🔄 Mocked | Not functional |
| Multi-turn | ✅ Real | ❌ Stub | Not implemented |
| RAG/Search | ✅ Real | ❌ Stub | Not implemented |
| Vision | ✅ Real | ❌ None | Not implemented |
| Streaming | ✅ Real | ✅ Real | ✅ Works! |

---

## Architecture Clarity

```
┌────────────────────────────────────────────────────┐
│ Responses API Request                              │
│ (18+ parameters accepted via OpenAPI schema)       │
└─────────────────┬──────────────────────────────────┘
                  ↓
┌────────────────────────────────────────────────────┐
│ Engine.ProcessRequest()                            │
│ • Echoes all params to response ✅                 │
│ • Only uses 5 params for LLM ⚠️                    │
└─────────────────┬──────────────────────────────────┘
                  ↓
┌────────────────────────────────────────────────────┐
│ ChatCompletionRequest                              │
│ • model, messages, temperature,                    │
│   max_tokens, stream                               │
│ • Missing: top_p, penalties, tools, etc.           │
└─────────────────┬──────────────────────────────────┘
                  ↓
┌────────────────────────────────────────────────────┐
│ OpenAI Client (openai-go SDK)                      │
│ • Could support 40+ params                         │
│ • We only pass 5                                   │
└─────────────────┬──────────────────────────────────┘
                  ↓
            [LLM Backend]
```

**The Gap:** We accept everything, echo everything, but only **use 5 parameters**.

---

## Version History

- **2026-02-10**: Updated endpoint coverage
  - Added 3 missing Responses API endpoints (list, delete, input_items)
  - All 41 endpoints now schema-complete (100%)
  - Functional implementation still ~35% (parameter limitations remain)

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

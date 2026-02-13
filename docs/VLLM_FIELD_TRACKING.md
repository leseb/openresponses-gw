# vLLM vs Gateway: Responses API Field Tracking

This document tracks which `/v1/responses` fields are handled by vLLM (inference/stateless)
versus the gateway (stateful/orchestration), and which are not yet implemented.

## Response Fields

### Implemented: Inference (handled by vLLM, forwarded by gateway)

These fields are accepted in the request, forwarded to vLLM, and echoed in the response.
vLLM does the actual work.

| Field | Request | Response | Default | Notes |
|-------|---------|----------|---------|-------|
| `model` | required | echoed | -- | Model identifier |
| `input` | required | -- | -- | Prompt or conversation items |
| `instructions` | optional | echoed | null | System message |
| `temperature` | optional | echoed | 0 | Sampling temperature |
| `top_p` | optional | echoed | 0 | Nucleus sampling |
| `max_output_tokens` | optional | echoed | null | Token limit |
| `frequency_penalty` | optional | echoed | 0 | Frequency penalty |
| `presence_penalty` | optional | echoed | 0 | Presence penalty |
| `tools` | optional | echoed | [] | Function/MCP/file_search tool definitions |
| `tool_choice` | optional | echoed | "none" | Tool selection strategy |
| `reasoning` | optional | echoed | null | Reasoning config for o-series models |
| `truncation` | optional | echoed | "disabled" | vLLM truncates prompt when set to "auto" |
| `parallel_tool_calls` | optional | echoed | true | vLLM controls multi-tool output |
| `text` | optional | echoed | `{format:{type:"text"}}` | vLLM enforces structured output (json_schema) |
| `top_logprobs` | optional | echoed | 0 | vLLM computes log probabilities |

### Implemented: Gateway-managed (stateful/orchestration)

These fields are handled entirely by the gateway.

| Field | Request | Response | Default | Notes |
|-------|---------|----------|---------|-------|
| `store` | optional | echoed | true | Gateway persists responses for retrieval and chaining |
| `previous_response_id` | optional | echoed | null | Gateway loads conversation history |
| `conversation` | optional | echoed | null | Gateway manages conversation state |
| `metadata` | optional | echoed | null | Gateway stores and echoes key-value pairs |
| `include` | optional | -- | -- | Controls response content (e.g. logprobs) |
| `max_tool_calls` | optional | echoed | null | Gateway enforces agentic loop iteration limit |

### Not Yet Implemented

These fields are defined in the OpenAI Responses API spec but are **not accepted or echoed**
by this gateway. They are intentionally omitted to avoid giving a false impression of support.

| Field | Owner | Why Not Implemented | Spec Default |
|-------|-------|---------------------|--------------|
| `background` | Gateway | Requires async job queue, polling via GET, routing | false |
| `service_tier` | Gateway | Routing/scheduling concern; no multi-tier backend support | "auto" |
| `safety_identifier` | Gateway | No safety/moderation enforcement layer | null |
| `prompt_cache_key` | Gateway | Cross-request cache coordination not built | null |

## Content Part Fields (`output_text`)

| Field | Source | Status | Notes |
|-------|--------|--------|-------|
| `text` | vLLM | Implemented | The generated text content |
| `annotations` | Gateway | Implemented (empty) | Populated from tool results (web search citations, file citations); currently `[]` |
| `logprobs` | vLLM | Implemented (empty) | vLLM populates when `include` contains `"message.output_text.logprobs"` and `top_logprobs > 0`; currently `[]` because gateway does not yet forward `include` |

## vLLM `/v1/responses` Capabilities

Based on vLLM source code analysis
([protocol](https://github.com/vllm-project/vllm/blob/main/vllm/entrypoints/openai/responses/protocol.py),
[serving](https://github.com/vllm-project/vllm/blob/main/vllm/entrypoints/openai/responses/serving.py)):

- **Prompt truncation** (`truncation`): Accepted. `build_tok_params()` sets
  `truncate_prompt_tokens=-1` when `truncation != "disabled"`. Actual truncation
  depends on the tokenizer path; behaviour may vary between vLLM versions.
- **Parallel tool calls** (`parallel_tool_calls`): Accepted and echoed. In the
  Responses API path, tool calls are processed via a state machine, so the
  parameter may not change behaviour today. In the Chat Completions API path it
  actively controls multi-tool output.
- **Structured output** (`text.format`): Active. Uses xgrammar/outlines for
  JSON schema enforcement via `StructuredOutputsParams`.
- **Log probabilities** (`top_logprobs`): Active. `_topk_logprobs()` and
  `_create_stream_response_logprobs()` produce per-token logprobs when
  `include` contains `"message.output_text.logprobs"`.
- **Store** (`store`): In-memory only, single-instance. Controlled by
  `VLLM_ENABLE_RESPONSES_API_STORE` env var. Gateway should own persistence.
- **Background** (`background`): Single-instance async via
  `_run_background_request_stream()`. Not suitable for multi-instance routing.
- **Service tier** (`service_tier`): Pass-through, no behavioural effect.
- **Safety identifier** (`safety_identifier`): Not present in vLLM's
  `ResponsesRequest` / `ResponsesResponse` models.
- **Prompt cache key** (`prompt_cache_key`): Accepted but explicitly ignored by
  vLLM. vLLM has its own automatic prefix-caching mechanism.

## What the Gateway Forwards to vLLM

The gateway constructs an `api.ResponsesAPIRequest` and forwards these fields to vLLM:

- `model`, `input`, `instructions`, `tools`, `tool_choice`
- `temperature`, `top_p`, `frequency_penalty`, `presence_penalty`, `max_output_tokens`
- `truncation`, `parallel_tool_calls`, `text`
- `reasoning`
- `store` (always set to `false` -- gateway owns persistence)
- `stream`

### Not Yet Forwarded

| Field | Why | Impact |
|-------|-----|--------|
| `top_logprobs` | `api.ResponsesAPIRequest` struct missing the field | Logprobs not computed by vLLM |
| `include` | `api.ResponsesAPIRequest` struct missing the field | vLLM doesn't know to include logprobs |

## Sources

### vLLM source code

- [responses/protocol.py](https://github.com/vllm-project/vllm/blob/main/vllm/entrypoints/openai/responses/protocol.py) — `ResponsesRequest` and `ResponsesResponse` Pydantic models
- [responses/serving.py](https://github.com/vllm-project/vllm/blob/main/vllm/entrypoints/openai/responses/serving.py) — `OpenAIServingResponses` class (truncation, text, logprobs, store, background handling)
- [responses/api_router.py](https://github.com/vllm-project/vllm/blob/main/vllm/entrypoints/openai/responses/api_router.py) — FastAPI routes (`POST /v1/responses`, `GET /v1/responses/{id}`, `POST /v1/responses/{id}/cancel`)

### vLLM documentation

- [OpenAI Responses Client example](https://docs.vllm.ai/en/latest/examples/online_serving/openai_responses_client.html)
- [OpenAI-compatible server overview](https://docs.vllm.ai/en/latest/serving/openai_compatible_server/)

### vLLM issues / RFCs

- [RFC #24603: Responses API full functionality without stores](https://github.com/vllm-project/vllm/issues/24603) — proposes stateless operation by returning messages in responses
- [Issue #14721: Support OpenAI Responses API](https://github.com/vllm-project/vllm/issues/14721) — original feature request

### OpenAI spec

- [Responses API reference — Create response](https://platform.openai.com/docs/api-reference/responses/create)
- [Prompt caching guide](https://platform.openai.com/docs/guides/prompt-caching)
- [Safety checks guide](https://platform.openai.com/docs/guides/safety-checks)

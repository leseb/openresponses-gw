# Open Responses Conformance Testing

This project achieves **100% compliance** with the [Open Responses Specification](https://github.com/openresponses/openresponses) through automated conformance testing.

## What is Open Responses?

Open Responses is a unified API specification for AI inference that aims to standardize how applications interact with Large Language Models (LLMs). It provides:

- **Single endpoint**: `POST /v1/responses` for all inference requests
- **Provider-agnostic**: Works with OpenAI, Anthropic, local models, etc.
- **Streaming support**: 24 granular event types via Server-Sent Events
- **Request echo pattern**: All request parameters returned in responses
- **Multimodal input**: Text, images, files, video
- **Tool calling**: Function/tool invocation support
- **Reasoning models**: Support for o1/o3 style reasoning

## Conformance Test Suite

The official conformance test suite validates 6 critical API behaviors:

### 1. Basic Text Response
**Test ID:** `basic-response`

Validates fundamental request-response functionality:
- POST request to `/v1/responses`
- Response includes `id`, `object`, `created_at`, `model`, `status`
- Status progresses to `completed`
- Output array contains assistant message

**Example:**
```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","input":"What is 2+2?"}'
```

### 2. Streaming Response
**Test ID:** `streaming-response`

Ensures SSE streaming works correctly:
- Set `stream: true` in request
- Response uses `text/event-stream` content type
- Events follow Open Responses schema
- All 24 event types properly structured

**Event sequence:**
```
event: response.created
event: response.in_progress
event: response.output_item.added
event: response.output_text.delta (multiple)
event: response.output_text.done
event: response.output_item.done
event: response.completed
```

### 3. System Prompt
**Test ID:** `system-prompt`

Tests instruction-following:
- `instructions` field sets system context
- Model respects behavioral directives
- Output reflects system instructions

**Example:**
```json
{
  "model": "gpt-4",
  "input": "Explain quantum computing",
  "instructions": "You are a physics professor. Be concise."
}
```

### 4. Tool Calling
**Test ID:** `tool-calling`

Validates function invocation:
- `tools` array defines available functions
- `tool_choice` controls which tools to use
- Output includes `function_call` items
- Function arguments properly formatted

**Example:**
```json
{
  "model": "gpt-4",
  "input": "What's the weather in Paris?",
  "tools": [{
    "type": "function",
    "function": {
      "name": "get_weather",
      "parameters": {"type": "object", "properties": {"location": {"type": "string"}}}
    }
  }]
}
```

### 5. Image Input
**Test ID:** `image-input`

Tests multimodal capabilities:
- Input as array with image items
- Base64-encoded or URL-based images
- Image detail levels (auto, low, high)

**Example:**
```json
{
  "model": "gpt-4-vision",
  "input": [
    {"type": "message", "role": "user", "content": "What's in this image?"},
    {"type": "image", "image_url": {"url": "data:image/png;base64,..."}}
  ]
}
```

### 6. Multi-turn Conversation
**Test ID:** `multi-turn`

Validates conversation state:
- `previous_response_id` links responses
- Conversation history maintained
- Context preserved across turns

**Example:**
```json
{
  "model": "gpt-4",
  "input": "Tell me more",
  "previous_response_id": "resp_abc123"
}
```

## Running Conformance Tests

### Prerequisites

Install required tools:

```bash
# Bun (recommended)
curl -fsSL https://bun.sh/install | bash

# Or Node.js (alternative)
# https://nodejs.org/

# Pre-commit (for automated testing)
pip install pre-commit
```

### Manual Testing

Run conformance tests directly:

```bash
# Build server
make build-server

# Run tests
make test-conformance

# Or with custom settings
SERVER_PORT=9000 OPENRESPONSES_MODEL=gpt-4 ./tests/scripts/run-conformance-tests.sh
```

### Automated Testing via Pre-commit

Install pre-commit hooks:

```bash
make pre-commit-install
```

Tests run automatically on commit:

```bash
git commit -m "feat: new feature"
# → Conformance tests run automatically
```

Skip for WIP commits:

```bash
SKIP=openresponses-conformance git commit -m "WIP: in progress"
```

### CI/CD Integration

Example GitHub Actions workflow:

```yaml
name: Conformance

on: [push, pull_request]

jobs:
  conformance:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Set up Bun
        uses: oven-sh/setup-bun@v1

      - name: Run conformance tests
        run: make test-conformance
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
```

## Interpreting Results

### Successful Run

```
==== Open Responses Conformance Tests ====
Cloning conformance test repository...
Installing conformance test dependencies...
Starting server on port 8080...
Server is ready!
Running conformance tests...

✓ basic-response (245ms)
✓ streaming-response (1.2s)
✓ system-prompt (312ms)
✓ tool-calling (489ms)
✓ image-input (723ms)
✓ multi-turn (156ms)

6/6 tests passed
✓ All conformance tests passed!
```

### Failed Test Example

```
✗ streaming-response (1.1s)
  Error: Missing required event type 'response.output_text.delta'

  Request:
  POST /v1/responses
  {"model":"gpt-4","input":"Hello","stream":true}

  Response:
  event: response.created
  event: response.completed

  Expected events: response.created, response.in_progress,
                   response.output_text.delta, response.completed
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | 8080 | Port for test server |
| `OPENRESPONSES_MODEL` | gpt-4 | Model ID for tests |
| `OPENAI_API_KEY` | test-key | API key for backend LLM |

### Test Filtering

Run specific tests only:

```bash
cd .conformance-tests
bun run bin/compliance-test.ts \
  --base-url http://localhost:8080 \
  --api-key test-key \
  --filter basic-response,streaming-response
```

### Verbose Output

See full request/response details:

```bash
./tests/scripts/run-conformance-tests.sh
# Tests run with --verbose flag by default
```

## Troubleshooting

### Tests timeout

Increase timeout in conformance test config:
```bash
# Edit .conformance-tests/bin/compliance-test.ts
# Increase timeout value
```

### Server fails to start

Check logs:
```bash
cat .server-test.log
```

Common issues:
- Port 8080 already in use: `lsof -i :8080`
- Config file missing: Ensure `config/config.yaml` exists
- Binary not built: Run `make build-server`

### Image input test fails

Ensure model supports vision:
```bash
OPENRESPONSES_MODEL=gpt-4-vision-preview make test-conformance
```

### Tool calling test fails

Check that:
1. Model supports function calling
2. Tools are properly defined in schema
3. Backend LLM is responding with function calls

## Compliance Badge

Display compliance status in README:

```markdown
![Open Responses Compliant](https://img.shields.io/badge/Open%20Responses-100%25%20Compliant-brightgreen)
```

## Resources

- [Open Responses Specification](https://github.com/openresponses/openresponses)
- [Conformance Test Suite](https://github.com/openresponses/openresponses/pull/17)
- [OpenAPI Specification](./openapi.yaml)
- [Implementation Guide](./docs/implementation.md)

## Contributing

To maintain compliance:

1. Run conformance tests before committing
2. Keep `openapi.yaml` in sync with implementation
3. Add tests for new features
4. Update this documentation when adding capabilities

## License

Conformance tests are maintained by the Open Responses project under MIT license.

# Testing Guide

## Quick Start

### Option 1: Server Already Running

If your server is already running (e.g., you started it with `make run`):

```bash
# Test with default model (ollama/gpt-oss:20b)
./scripts/test-conformance.sh

# Test with specific model
./scripts/test-conformance.sh "gpt-4"

# Test with custom URL and model
./scripts/test-conformance.sh "claude-3-opus" "http://localhost:9000" "sk-key"
```

### Option 2: Auto-Start Server

Let the script start and stop the server automatically:

```bash
# Start server and run tests with default model
./scripts/test-conformance-with-server.sh

# Custom model
./scripts/test-conformance-with-server.sh "ollama/gpt-oss:20b"

# Custom model and port
./scripts/test-conformance-with-server.sh "gpt-4" "9000"

# Full control
./scripts/test-conformance-with-server.sh "gpt-4" "8080" "sk-test-key"
```

## Using Make Targets

```bash
# Start server automatically and run tests
make test-conformance-auto

# Run tests against already-running server
make test-conformance

# Run with custom model via environment variable
MODEL="gpt-4" PORT=8080 API_KEY="none" make test-conformance-custom
```

## Example: Testing with Ollama

```bash
# Start Ollama in another terminal
ollama serve

# Pull the model
ollama pull gpt-oss:20b

# Run conformance tests
./scripts/test-conformance-with-server.sh "ollama/gpt-oss:20b"
```

## Example: Testing with OpenAI

```bash
# Set your API key
export OPENAI_API_KEY="sk-..."

# Run with OpenAI model
./scripts/test-conformance-with-server.sh "gpt-4" "8080" "$OPENAI_API_KEY"
```

## Example: Testing with Custom Backend

```bash
# Start your server with custom backend configuration
./bin/responses-gateway-server -config config/my-backend.yaml &

# Run tests against it
./scripts/test-conformance.sh "my-custom-model" "http://localhost:8080" "my-key"
```

## What Gets Tested

The conformance test suite validates **6 critical behaviors**:

### 1. Basic Text Response ✓
Simple request-response with text input/output.

**Example Request:**
```json
{
  "model": "gpt-4",
  "input": "What is 2+2?"
}
```

**Validates:**
- Response structure (id, object, created_at, model, status)
- Status transitions to "completed"
- Output contains assistant message

### 2. Streaming Response ✓
Server-Sent Events with all 24 event types.

**Example Request:**
```json
{
  "model": "gpt-4",
  "input": "Tell me a story",
  "stream": true
}
```

**Validates:**
- SSE event stream format
- Event types: response.created, response.in_progress, response.output_text.delta, etc.
- Proper event sequencing

### 3. System Prompt ✓
Instruction-following with system messages.

**Example Request:**
```json
{
  "model": "gpt-4",
  "input": "Explain quantum computing",
  "instructions": "You are a physics professor. Be concise."
}
```

**Validates:**
- System instructions are respected
- Output reflects system context

### 4. Tool Calling ✓
Function invocation capabilities.

**Example Request:**
```json
{
  "model": "gpt-4",
  "input": "What's the weather in Paris?",
  "tools": [{
    "type": "function",
    "function": {
      "name": "get_weather",
      "parameters": {
        "type": "object",
        "properties": {
          "location": {"type": "string"}
        }
      }
    }
  }]
}
```

**Validates:**
- Tool definitions are recognized
- Function calls are generated
- Arguments are properly formatted

### 5. Image Input ✓
Multimodal input handling.

**Example Request:**
```json
{
  "model": "gpt-4-vision",
  "input": [
    {
      "type": "message",
      "role": "user",
      "content": "What's in this image?"
    },
    {
      "type": "image",
      "image_url": {"url": "data:image/png;base64,..."}
    }
  ]
}
```

**Validates:**
- Image input processing
- Base64-encoded images
- Image detail levels

### 6. Multi-turn Conversation ✓
Conversation history with previous_response_id.

**Example Request:**
```json
{
  "model": "gpt-4",
  "input": "What did I just tell you?",
  "previous_response_id": "resp_abc123"
}
```

**Validates:**
- Conversation state management
- Context preservation across turns
- Response linking

## Interpreting Results

### Success ✅
```
==== Running Conformance Tests ====

✓ Basic Text Response (245ms)
✓ Streaming Response (1.2s)
✓ System Prompt (312ms)
✓ Tool Calling (489ms)
✓ Image Input (723ms)
✓ Multi-turn Conversation (156ms)

Results: 6 passed, 0 failed, 6 total
✓ All conformance tests passed!
```

### Failure ❌
```
✗ Streaming Response (1.1s)
  ✗ Missing required event type 'response.output_text.delta'

  Request:
  {"model":"gpt-4","input":"Hello","stream":true}

  Response:
  event: response.created
  event: response.completed

  Expected: response.created, response.in_progress,
            response.output_text.delta, response.completed
```

## Troubleshooting

### "Server not accessible"
```bash
# Check if server is running
curl http://localhost:8080/health

# Start server manually
make run

# Or build and run
make build-server
./bin/responses-gateway-server
```

### "Port already in use"
```bash
# Find what's using the port
lsof -i :8080

# Kill the process
lsof -ti:8080 | xargs kill -9

# Or use a different port
./scripts/test-conformance-with-server.sh "gpt-4" "9000"
```

### "bun not found"
```bash
# Install Bun (recommended)
curl -fsSL https://bun.sh/install | bash

# Or use npx (slower)
# Tests will automatically use npx if bun is not available
```

### Tests timeout
- Increase model inference timeout in server config
- Check backend LLM is accessible
- Verify API key is valid
- Check network connectivity

### "Model not found"
- Verify model name is correct
- Check model is available in your backend
- For Ollama: `ollama list` to see available models
- For OpenAI: Verify model exists and you have access

## CI/CD Integration

### GitHub Actions

```yaml
name: Conformance Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - uses: oven-sh/setup-bun@v1

      - name: Run conformance tests
        run: make test-conformance-auto
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
```

### GitLab CI

```yaml
conformance:
  image: golang:1.21
  before_script:
    - curl -fsSL https://bun.sh/install | bash
    - export PATH="$HOME/.bun/bin:$PATH"
  script:
    - make build-server
    - ./scripts/test-conformance-with-server.sh
  variables:
    MODEL: "gpt-4"
    API_KEY: $OPENAI_API_KEY
```

## Advanced Usage

### Running Specific Tests

```bash
# Clone the conformance repo first
git clone https://github.com/openresponses/openresponses .conformance-tests
cd .conformance-tests
bun install

# Run specific tests
bun run test:compliance \
  --base-url http://localhost:8080 \
  --api-key none \
  --model "ollama/gpt-oss:20b" \
  --filter "basic-response,streaming-response"

# Available test IDs:
# - basic-response
# - streaming-response
# - system-prompt
# - tool-calling
# - image-input
# - multi-turn
```

### JSON Output for CI

```bash
cd .conformance-tests
bun run test:compliance \
  --base-url http://localhost:8080 \
  --api-key none \
  --model "gpt-4" \
  --json > results.json

# Parse results
cat results.json | jq '.summary'
```

### Verbose Output

```bash
# Tests run with --verbose by default in our scripts
# To see full request/response details:
./scripts/test-conformance.sh "gpt-4" 2>&1 | tee test-output.log
```

## Next Steps

After passing conformance tests:

1. ✅ **100% Open Responses compliant** - Your implementation is verified!
2. 📝 Add compliance badge to README
3. 🚀 Deploy with confidence
4. 🔄 Run tests in CI/CD on every commit
5. 📊 Monitor test results over time

---

## Architecture: Single Source of Truth with Multiple Adapters

The gateway follows a clean architecture pattern where the **core business logic is implemented once** and multiple **protocol adapters** translate between different protocols and the core:

```
┌─────────────────────────────────────────────────────────┐
│                    ARCHITECTURE                          │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────┐           ┌──────────────┐           │
│  │   HTTP API   │           │ Envoy Proxy  │           │
│  │   (REST)     │           │  (ExtProc)   │           │
│  └──────┬───────┘           └──────┬───────┘           │
│         │                          │                    │
│         ▼                          ▼                    │
│  ┌──────────────────────────────────────────┐          │
│  │         ADAPTER LAYER                     │          │
│  ├──────────────────┬───────────────────────┤          │
│  │  HTTP Adapter    │   Envoy Adapter       │          │
│  │  - handler.go    │   - processor.go      │          │
│  │  - models.go     │   - translator.go     │          │
│  │  - prompts.go    │                       │          │
│  └────────┬─────────┴───────────┬───────────┘          │
│           │                     │                       │
│           └─────────┬───────────┘                       │
│                     ▼                                   │
│           ┌─────────────────────┐                       │
│           │   CORE ENGINE       │  ◄── Single Source   │
│           │   engine.go         │      of Truth        │
│           │                     │                       │
│           │  - ProcessRequest() │                       │
│           │  - ProcessStream()  │                       │
│           └──────────┬──────────┘                       │
│                      │                                  │
│                      ▼                                  │
│           ┌─────────────────────┐                       │
│           │   STATE LAYER       │                       │
│           │   SessionStore      │                       │
│           │   (in-memory)       │                       │
│           └─────────────────────┘                       │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Key Benefits

✅ **Single Implementation**: All Open Responses logic lives in `pkg/core/engine/`
✅ **Protocol Agnostic**: Adapters only translate protocols, not business logic
✅ **Extensible**: Easy to add new protocols (gRPC, WebSocket, etc.)
✅ **Testable**: Can test core logic independently of protocol

### What's in Each Layer?

#### Core Engine (`pkg/core/engine/`)
- ✅ Request validation
- ✅ Response generation
- ✅ Streaming logic
- ✅ Tool calling
- ✅ Conversation state management

#### HTTP Adapter (`pkg/adapters/http/`)
- Translates HTTP requests → `ResponseRequest`
- Translates `Response` → HTTP JSON
- Handles SSE streaming
- Routes `/v1/responses`, `/v1/models`, etc.

#### Envoy ExtProc Adapter (`pkg/adapters/envoy/`)
- Translates Envoy ExtProc messages → `ResponseRequest`
- Translates `Response` → Envoy `ImmediateResponse`
- Implements gRPC ExtProc protocol
- Handles Envoy lifecycle phases

---

## Testing the Envoy ExtProc Adapter

The Envoy adapter can be tested at multiple levels:

### 1. Unit Tests (Protocol Translation)

Test the translator logic that converts between Envoy messages and core types:

```bash
# Run Envoy adapter unit tests
go test ./pkg/adapters/envoy/... -v

# Run with coverage
go test ./pkg/adapters/envoy/... -cover
```

**What's tested:**
- ✅ Request extraction from ExtProc messages
- ✅ Response formatting to ExtProc messages
- ✅ Error response generation
- ✅ Header manipulation
- ✅ Status code translation

**Example test:**
```go
func TestExtractResponseRequest(t *testing.T) {
    req := &extproc.ProcessingRequest{
        Request: &extproc.ProcessingRequest_RequestBody{
            RequestBody: &extproc.HttpBody{
                Body: []byte(`{"model":"llama3.2:3b","input":"Hello"}`),
            },
        },
    }

    got, err := ExtractResponseRequest(req)
    // Verify extraction logic
}
```

### 2. Integration Tests (End-to-End with Docker)

Test the full stack: Client → Envoy → ExtProc → Core Engine:

```bash
# Run integration tests
./scripts/test-envoy-extproc.sh

# Run with custom model
MODEL=gpt-4 ./scripts/test-envoy-extproc.sh
```

**What's tested:**
- ✅ Request flows through Envoy to ExtProc
- ✅ ExtProc processes request via core engine
- ✅ Response returns through Envoy to client
- ✅ Error handling at each boundary
- ✅ Envoy filter statistics

**Test scenarios:**
1. Basic non-streaming request
2. Request with system instructions
3. Request with tools
4. Invalid request handling
5. Response structure validation
6. ExtProc filter metrics

### 3. Debugging Integration Tests

```bash
# Start the stack manually
cd examples/envoy
docker-compose up

# In another terminal, make requests
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3.2:3b","input":"Hello"}'

# Check ExtProc logs
docker-compose logs -f envoy-extproc

# Check Envoy logs
docker-compose logs -f envoy

# Check Envoy stats
curl http://localhost:9901/stats | grep ext_proc

# Stop the stack
docker-compose down
```

---

## Test Matrix

| Test Type | HTTP Adapter | Envoy Adapter | What's Tested |
|-----------|--------------|---------------|---------------|
| **Conformance** | ✅ | ⚠️ Via HTTP | Spec compliance |
| **Unit** | ❌ | ✅ | Protocol translation |
| **Integration** | ✅ | ✅ | Full E2E flow |

**Note**: Conformance tests currently run against the HTTP adapter. The Envoy adapter reuses the same core engine, so it inherits spec compliance.

---

## OpenAPI Conformance Testing

The gateway includes an OpenAPI conformance checker that compares our API spec against OpenAI's official spec to ensure compatibility.

### Running OpenAPI Conformance Tests

```bash
# Check conformance (uses cached OpenAI spec)
./scripts/openapi_conformance.py

# Force re-download of OpenAI spec
rm openai-spec.yaml && ./scripts/openapi_conformance.py

# Save results to JSON
./scripts/openapi_conformance.py --output conformance-results.json

# Verbose output
./scripts/openapi_conformance.py --verbose
```

### What It Checks

The conformance test compares three API categories:

1. **Files API** - File upload, retrieval, deletion
2. **Vector Stores API** - Vector store management and search
3. **Responses API** - Response creation and retrieval

For each category, it identifies:
- ❌ Missing endpoints (in OpenAI but not in gateway)
- ⚠️ Schema differences (different request/response structures)
- ✅ Implemented endpoints

### Interpreting Results

```
✅ 90-100%: Excellent - High OpenAI compatibility
⚠️  70-89%: Good - Moderate gaps
❌ 0-69%: Needs Work - Significant gaps
```

**Current Baseline (as of initial implementation):**
- Files API: 0% (missing CRUD endpoints)
- Vector Stores API: 0% (missing all endpoints)
- Responses API: 25% (missing GET endpoint)
- **Overall: 8.3%**

See [OPENAPI_CONFORMANCE.md](./OPENAPI_CONFORMANCE.md) for detailed gap analysis and implementation roadmap.

### Adding to CI/CD

```yaml
# .github/workflows/conformance.yml
- name: Check OpenAPI Conformance
  run: |
    uv run --with pyyaml ./scripts/openapi_conformance.py --output conformance.json

    # Fail if below 70% threshold
    python3 -c "
    import json
    with open('conformance.json') as f:
        results = json.load(f)
    scores = [r['score'] for r in results.values()]
    avg = sum(scores) / len(scores)
    if avg < 70.0:
        raise SystemExit(f'Conformance {avg:.1f}% below 70% threshold')
    "
```

### Pre-Commit Hook

The OpenAPI conformance check is integrated with pre-commit hooks:

**Automatic Check** (when `openapi.yaml` changes):
```bash
# Pre-commit will automatically run conformance check when you modify openapi.yaml
git add openapi.yaml
git commit -m "Update API spec"
# Hook runs and shows conformance summary (non-blocking)
```

**Manual Check** (anytime):
```bash
# Run conformance check explicitly
pre-commit run openapi-conformance --all-files

# Or use Make target for full output
make test-openapi-conformance
```

**Important Notes:**
- The pre-commit hook is **non-blocking** - it won't prevent commits
- It shows a conformance summary to inform you of schema differences
- For detailed analysis, run `make test-openapi-conformance`
- The hook caches the OpenAI spec locally for faster runs

---

## Running All Tests

```bash
# 1. Unit tests (all adapters)
go test ./...

# 2. Conformance tests (HTTP adapter)
./scripts/test-conformance.sh llama3.2:3b

# 3. Smoke tests (critical path validation)
./scripts/test-smoke.sh

# 4. OpenAPI conformance (spec compatibility)
./scripts/openapi_conformance.py

# 5. Integration tests (Envoy adapter)
./scripts/test-envoy-extproc.sh

# 6. Pre-commit hooks
pre-commit run --all-files
```

---

## Resources

- [Open Responses Specification](https://github.com/openresponses/openresponses)
- [Conformance Test Suite](https://github.com/openresponses/openresponses/pull/17)
- [CONFORMANCE.md](./CONFORMANCE.md) - Detailed conformance documentation
- [scripts/README.md](./scripts/README.md) - Script documentation

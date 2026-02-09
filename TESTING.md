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

## Resources

- [Open Responses Specification](https://github.com/openresponses/openresponses)
- [Conformance Test Suite](https://github.com/openresponses/openresponses/pull/17)
- [CONFORMANCE.md](./CONFORMANCE.md) - Detailed conformance documentation
- [scripts/README.md](./scripts/README.md) - Script documentation

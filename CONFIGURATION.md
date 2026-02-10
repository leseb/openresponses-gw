# Configuration Guide

This guide explains how to configure the Open Responses Gateway to connect to real inference backends.

## Quick Start: Testing with OpenAI

### 1. Get an OpenAI API Key

Visit https://platform.openai.com/api-keys and create an API key.

### 2. Set Environment Variable

```bash
export OPENAI_API_KEY=sk-your-api-key-here
```

### 3. Run the Server

```bash
# The server will automatically detect the API key and use real OpenAI
make run

# Or with explicit config
./bin/openresponses-gw-server --config examples/standalone/config-openai.yaml
```

### 4. Test It!

```bash
# Simple test
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "What is 2+2?"
  }' | jq .

# Or use the comprehensive test script
OPENAI_API_KEY=sk-... ./examples/standalone/test-with-openai.sh
```

## Configuration Methods

The gateway supports **3 ways** to configure the inference backend (in order of precedence):

### Method 1: Environment Variables (Recommended)

```bash
# Required
export OPENAI_API_KEY=sk-your-key-here

# Optional (defaults to https://api.openai.com/v1)
export OPENAI_API_ENDPOINT=https://api.openai.com/v1

# Run
./bin/openresponses-gw-server
```

**Pros:**
- Most flexible
- Works with Docker/Kubernetes
- No secrets in config files

---

### Method 2: Configuration File

Create `config.yaml`:

```yaml
server:
  host: 0.0.0.0
  port: 8080
  timeout: 60s

engine:
  model_endpoint: https://api.openai.com/v1
  api_key: sk-your-key-here  # Not recommended - use env vars instead
  max_tokens: 4096
  timeout: 60s
```

Run:

```bash
./bin/openresponses-gw-server --config config.yaml
```

**Pros:**
- Explicit configuration
- Easy to version control (without api_key)

---

### Method 3: Command-Line Flags

```bash
./bin/openresponses-gw-server --port 9090
```

**Available flags:**
- `--config <path>` - Path to config file
- `--port <port>` - HTTP port (default: 8080)
- `--version` - Print version

---

## Backend Modes

The gateway automatically selects the backend mode based on configuration:

### Mode 1: Real OpenAI (Production)

**Triggers when:**
- `OPENAI_API_KEY` is set **AND**
- `OPENAI_API_ENDPOINT` is set (or defaults to OpenAI)

**Behavior:**
- Makes real API calls to OpenAI
- Returns actual LLM responses
- Charges your OpenAI account
- Full token usage tracking

**Example:**
```bash
export OPENAI_API_KEY=sk-proj-...
export OPENAI_API_ENDPOINT=https://api.openai.com/v1
./bin/openresponses-gw-server
```

---

### Mode 2: Mock LLM (Development/Testing)

**Triggers when:**
- `OPENAI_API_KEY` is **NOT** set **OR**
- `OPENAI_API_ENDPOINT` is **NOT** set

**Behavior:**
- Returns mock responses
- No external API calls
- Free to use
- Useful for testing gateway features

**Example:**
```bash
# No API key set
./bin/openresponses-gw-server
```

Mock response format:
```json
{
  "output": [{
    "content": {
      "text": "Mock response to: <your input>"
    }
  }]
}
```

---

## OpenAI-Compatible Backends

The gateway works with **any OpenAI-compatible API**:

### Groq

```bash
export OPENAI_API_KEY=gsk_your_groq_key
export OPENAI_API_ENDPOINT=https://api.groq.com/openai/v1
./bin/openresponses-gw-server
```

Models: `llama3-70b-8192`, `mixtral-8x7b-32768`, etc.

---

### Together AI

```bash
export OPENAI_API_KEY=your_together_key
export OPENAI_API_ENDPOINT=https://api.together.xyz/v1
./bin/openresponses-gw-server
```

---

### Ollama (Local)

```bash
# Start Ollama
ollama serve

# Pull a model
ollama pull llama3.2

# Configure gateway
export OPENAI_API_KEY=unused
export OPENAI_API_ENDPOINT=http://localhost:11434/v1
./bin/openresponses-gw-server
```

**Note:** Ollama doesn't require an API key, but you must set it to any non-empty value.

---

### Azure OpenAI

```bash
export OPENAI_API_KEY=your_azure_key
export OPENAI_API_ENDPOINT=https://your-resource.openai.azure.com/openai/deployments/your-deployment
./bin/openresponses-gw-server
```

---

### LM Studio (Local)

```bash
# Start LM Studio server on port 1234
export OPENAI_API_KEY=unused
export OPENAI_API_ENDPOINT=http://localhost:1234/v1
./bin/openresponses-gw-server
```

---

## Configuration Examples

### Example 1: Production OpenAI

```yaml
# config-production.yaml
server:
  host: 0.0.0.0
  port: 8080
  timeout: 60s

engine:
  model_endpoint: https://api.openai.com/v1
  # api_key loaded from OPENAI_API_KEY env var
  max_tokens: 4096
  timeout: 60s
```

```bash
export OPENAI_API_KEY=sk-proj-...
./bin/openresponses-gw-server --config config-production.yaml
```

---

### Example 2: Development with Ollama

```yaml
# config-dev.yaml
server:
  host: 127.0.0.1
  port: 8080
  timeout: 30s

engine:
  model_endpoint: http://localhost:11434/v1
  # api_key not needed for Ollama, but must be non-empty
  max_tokens: 2048
  timeout: 30s
```

```bash
export OPENAI_API_KEY=unused
ollama serve
./bin/openresponses-gw-server --config config-dev.yaml
```

---

### Example 3: Groq for Fast Inference

```yaml
# config-groq.yaml
server:
  host: 0.0.0.0
  port: 8080
  timeout: 60s

engine:
  model_endpoint: https://api.groq.com/openai/v1
  max_tokens: 8192
  timeout: 60s
```

```bash
export OPENAI_API_KEY=gsk_...
./bin/openresponses-gw-server --config config-groq.yaml
```

---

## Request Parameters

When making requests, you can control model behavior:

```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "Tell me a joke",

    # Optional parameters
    "instructions": "You are a comedian",
    "temperature": 1.5,
    "max_output_tokens": 500,
    "stream": true
  }'
```

### Available Parameters

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `model` | string | Model to use (required) | - |
| `input` | string | User input (required) | - |
| `instructions` | string | System message | - |
| `temperature` | float | Creativity (0-2) | 1.0 |
| `max_output_tokens` | int | Max response tokens | 4096 |
| `stream` | bool | Enable streaming | false |
| `previous_response_id` | string | For multi-turn | - |

---

## Validation

### How to Verify Backend Connection

1. **Check server logs:**
```bash
./bin/openresponses-gw-server

# Look for:
# INFO Initialized engine mode=openai  (real backend)
# INFO Initialized engine mode=mock    (mock backend)
```

2. **Make a test request:**
```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","input":"ping"}' | jq .
```

3. **Check response format:**

**Real backend** returns actual model output:
```json
{
  "output": [{
    "content": {
      "text": "Pong! How can I help you today?"
    }
  }],
  "usage": {
    "total_tokens": 23  # Real token count
  }
}
```

**Mock backend** returns predictable format:
```json
{
  "output": [{
    "content": {
      "text": "Mock response to: ping"
    }
  }],
  "usage": {
    "total_tokens": 3  # Approximate count
  }
}
```

---

## Troubleshooting

### "Failed to call LLM: API returned status 401"

**Problem:** Invalid API key

**Solution:**
```bash
# Check your API key
echo $OPENAI_API_KEY

# Verify it's correct
curl https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY"
```

---

### "Failed to call LLM: connection refused"

**Problem:** Backend endpoint not reachable

**Solution:**
```bash
# For Ollama, ensure it's running
ollama serve

# For other backends, check endpoint
curl https://api.openai.com/v1/models
```

---

### "Mock response to: ..." (when expecting real responses)

**Problem:** Gateway is in mock mode

**Solution:**
```bash
# Ensure API key is set
export OPENAI_API_KEY=sk-...

# Ensure endpoint is set
export OPENAI_API_ENDPOINT=https://api.openai.com/v1

# Restart server
make run
```

---

### Streaming doesn't work

**Problem:** Some backends don't support streaming

**Solution:**
```bash
# Test without streaming first
curl -X POST http://localhost:8080/v1/responses \
  -d '{"model":"gpt-4o-mini","input":"hi","stream":false}'

# If that works, backend doesn't support streaming
# Use non-streaming mode
```

---

## Docker Configuration

### Using Environment Variables

```bash
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=sk-... \
  -e OPENAI_API_ENDPOINT=https://api.openai.com/v1 \
  openresponses-gw:latest
```

### Using Config File

```bash
docker run -p 8080:8080 \
  -v $(pwd)/config.yaml:/config.yaml \
  openresponses-gw:latest --config /config.yaml
```

### Docker Compose

```yaml
version: '3.8'
services:
  gateway:
    image: openresponses-gw:latest
    ports:
      - "8080:8080"
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OPENAI_API_ENDPOINT=https://api.openai.com/v1
```

Run:
```bash
OPENAI_API_KEY=sk-... docker-compose up
```

---

## Security Best Practices

1. **Never commit API keys** to version control
2. **Use environment variables** instead of config files
3. **Rotate keys** regularly
4. **Use separate keys** for dev/staging/prod
5. **Monitor usage** in your provider dashboard

---

## Next Steps

- See [QUICKSTART.md](./QUICKSTART.md) for testing examples
- See [PROJECT_PLAN.md](./PROJECT_PLAN.md) for upcoming features
- See [CONTRIBUTING.md](./CONTRIBUTING.md) to add new backends

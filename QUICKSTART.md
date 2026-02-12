# Quick Start Guide

Welcome to the Open Responses Gateway! This guide will get you up and running in 5 minutes.

## Project Location

```
/Users/leseb/go/src/github.com/leseb/openresponses-gw
```

## What's Been Implemented (Phase 1)

✅ **Core Architecture**
- Gateway-agnostic core engine
- HTTP adapter with streaming support
- In-memory storage (development mode) and SQLite (persistent)
- Request/response handling
- Configuration management
- Structured logging

✅ **API Endpoints**
- `GET /health` - Health check
- `POST /v1/responses` - Create response (streaming and non-streaming)

✅ **Features**
- Non-streaming responses
- SSE streaming responses
- Multi-turn conversations (via `previous_response_id`)
- Mock LLM responses (for testing without OpenAI API key)

## Quick Start

### 1. Build and Run

```bash
# Navigate to project
cd /Users/leseb/go/src/github.com/leseb/openresponses-gw

# Build the server
make build

# Run the server
./bin/openresponses-gw-server
```

You should see:
```
INFO Server listening address=0.0.0.0:8080
```

### 2. Test It!

In another terminal:

```bash
# Health check
curl http://localhost:8080/health

# Simple request
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "What is 2+2?"
  }'

# Streaming request
curl -N -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "Write a poem",
    "stream": true
  }'
```

Or use the test script:

```bash
# Run all tests
./examples/standalone/test-requests.sh
```

### 3. With Persistent Storage (SQLite)

```bash
# Run server with SQLite session store
SESSION_STORE_TYPE=sqlite SESSION_STORE_DSN=data/responses.db make run
```

## Configuration

### Default Configuration

The server works out of the box with defaults. To customize:

```bash
# Create config file
cp examples/standalone/config.yaml my-config.yaml

# Edit as needed
vim my-config.yaml

# Run with custom config
./bin/openresponses-gw-server --config my-config.yaml
```

### Environment Variables

```bash
# OpenAI API
export OPENAI_API_KEY="sk-..."
export OPENAI_API_ENDPOINT="https://api.openai.com/v1"

# Persistent session store (optional, default: in-memory)
export SESSION_STORE_TYPE=sqlite
export SESSION_STORE_DSN=data/responses.db

# Custom port
./bin/openresponses-gw-server --port 9090
```

## Example Requests

### 1. Simple Non-Streaming

```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "Hello, how are you?"
  }' | jq .
```

Response:
```json
{
  "id": "resp_1707242400000000000",
  "object": "response",
  "created_at": 1707242400,
  "completed_at": 1707242401,
  "model": "gpt-4",
  "status": "completed",
  "output": [
    {
      "type": "message",
      "id": "msg_1707242400000000001",
      "role": "assistant",
      "content": {
        "type": "text",
        "text": "Mock response to: Hello, how are you?"
      }
    }
  ],
  "usage": {
    "input_tokens": 5,
    "output_tokens": 9,
    "total_tokens": 14
  }
}
```

### 2. Streaming Response

```bash
curl -N -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "Count to 5",
    "stream": true
  }'
```

Response (SSE):
```
event: response.created
data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_...","status":"in_progress"}}

event: response.output_item.added
data: {"type":"response.output_item.added","sequence_number":1,"output_index":0}

event: response.output_text.delta
data: {"type":"response.output_text.delta","sequence_number":2,"delta":"Mock "}

event: response.output_text.delta
data: {"type":"response.output_text.delta","sequence_number":3,"delta":"streaming "}

event: response.completed
data: {"type":"response.completed","response":{"status":"completed"}}
```

### 3. Multi-Turn Conversation

```bash
# First request
RESP1=$(curl -s -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","input":"What is Paris?"}' | jq -r .id)

echo "First response ID: $RESP1"

# Follow-up request
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"gpt-4\",
    \"input\": \"What about London?\",
    \"previous_response_id\": \"$RESP1\"
  }" | jq .
```

## Development

### Run with Auto-Reload

```bash
# Install air (if not already)
go install github.com/air-verse/air@latest

# Run with auto-reload
make run-dev
```

### Run Tests

```bash
# All tests
make test

# With coverage
make test-coverage
open coverage.html
```

### Format and Lint

```bash
# Format code
make fmt

# Run linter
make lint
```

## Project Structure

```
.
├── cmd/server/              # HTTP server entry point
├── pkg/
│   ├── core/               # Gateway-agnostic core
│   │   ├── engine/         # Main orchestration
│   │   ├── schema/         # API schemas
│   │   ├── state/          # State interfaces
│   │   └── config/         # Configuration
│   ├── adapters/http/      # HTTP adapter
│   ├── storage/memory/     # In-memory storage
│   ├── storage/sqlite/     # SQLite persistent storage
│   └── observability/      # Logging, metrics
├── examples/               # Usage examples
├── tests/                  # Tests
└── deployments/           # Docker, Helm
```

## Current Limitations

The following are areas with ongoing development:

- ❌ Tool execution (file search, web search, etc.) — partially implemented
- ❌ Advanced RAG pipelines

See [PROJECT_PLAN.md](./PROJECT_PLAN.md) for the roadmap.

## What Works Now

✅ HTTP server with graceful shutdown
✅ Request parsing and validation
✅ Non-streaming responses
✅ SSE streaming responses
✅ Multi-turn conversation tracking (previous_response_id)
✅ Health checks
✅ Structured logging
✅ Configuration from YAML and env vars
✅ In-memory and SQLite session storage

## Next Steps

### For Users

1. **Test the API** - Use the examples above or `test-requests.sh`
2. **Read the docs** - See [README.md](./README.md) and [PROJECT_PLAN.md](./PROJECT_PLAN.md)
3. **Star on GitHub** - If you find it useful!

### For Contributors

1. **Read** [CONTRIBUTING.md](./CONTRIBUTING.md)
2. **Pick a Phase 2 task** - See PROJECT_PLAN.md Phase 2 milestones
3. **Open an issue** - Discuss your contribution
4. **Submit a PR** - We welcome contributions!

### Upcoming

- Additional storage backends
- Advanced session lifecycle management

## Troubleshooting

### Server won't start

```bash
# Check if port 8080 is in use
lsof -i :8080

# Use a different port
./bin/openresponses-gw-server --port 9090
```

### Build errors

```bash
# Clean and rebuild
make clean
go mod tidy
make build
```

### Tests failing

```bash
# Update dependencies
go mod tidy
go mod download

# Run tests with verbose output
go test -v ./...
```

## Getting Help

- **Questions**: Open a GitHub Discussion
- **Bugs**: Open a GitHub Issue
- **Contributing**: See [CONTRIBUTING.md](./CONTRIBUTING.md)

## Useful Commands

```bash
# Build
make build              # Build all binaries
make build-server       # Build server only

# Run
make run                # Build and run server
make run-dev            # Run with auto-reload

# Test
make test               # Run tests
make test-coverage      # Generate coverage report

# Quality
make lint               # Run linter
make fmt                # Format code

# Docker
make docker-build       # Build Docker image
docker-compose up -d    # Start dependencies

# Clean
make clean              # Remove build artifacts
```

## What's Next?

Check out the [PROJECT_PLAN.md](./PROJECT_PLAN.md) for the full 14-week implementation roadmap!

---

**Storage:** In-memory (default) or SQLite (persistent)
**Contributors Welcome:** See open issues and [CONTRIBUTING.md](./CONTRIBUTING.md)

# Envoy External Processor Integration

This example demonstrates running the OpenAI Responses Gateway as an Envoy External Processor (ExtProc). This enables the gateway to intercept and process requests through Envoy's ExtProc protocol.

## Architecture

```
Client → Envoy:8080 → ExtProc:10000 (gRPC)
                           ↓
                    Core Engine (reused)
                           ↓
                    SessionStore (in-memory)
                           ↓
                    Backend (Ollama:11434)
```

**Key Design:**
- ExtProc uses `ImmediateResponse` to return responses directly to clients
- The backend (Ollama/OpenAI) is called by the ExtProc service, not by Envoy
- Envoy's role is limited to proxying and invoking ExtProc
- All business logic remains in the reusable core engine

## Prerequisites

- Docker and Docker Compose
- Go 1.21+ (for building locally)

## Quick Start

### 1. Start the Stack

```bash
cd examples/envoy
docker-compose up -d
```

This will start three services:
- **Ollama** (port 11434): Local LLM backend
- **ExtProc** (port 10000): gRPC service processing requests
- **Envoy** (ports 8080, 9901): Proxy and admin interface

### 2. Pull Ollama Model

```bash
docker-compose exec ollama ollama pull llama3.2:3b
```

### 3. Test the Integration

#### Automated Test

```bash
./test-via-envoy.sh
```

#### Manual Test

```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:3b",
    "input": "What is 2+2? Answer with just the number."
  }'
```

Expected response:
```json
{
  "id": "resp_...",
  "object": "response",
  "created": 1234567890,
  "status": "completed",
  "output": [
    {
      "type": "message",
      "role": "assistant",
      "content": "4"
    }
  ],
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 1,
    "total_tokens": 16
  }
}
```

## Configuration

### Envoy Configuration (envoy.yaml)

Key settings:
- **Listener**: Port 8080 for HTTP traffic
- **ExtProc Filter**:
  - Buffered body mode (Phase 1)
  - 30s timeout for processing
  - Failure mode: fail closed
- **Backend Cluster**: Routes to Ollama at port 11434

### ExtProc Configuration (config.yaml)

```yaml
engine:
  backend:
    url: "http://ollama:11434"
  session:
    ttl: 3600
  request:
    max_input_length: 10000
    default_temperature: 0.7
```

## Monitoring

### Envoy Admin Interface

```bash
# General stats
curl http://localhost:9901/stats

# ExtProc specific stats
curl http://localhost:9901/stats | grep ext_proc

# Config dump
curl http://localhost:9901/config_dump
```

### Service Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f envoy-extproc
docker-compose logs -f envoy
docker-compose logs -f ollama
```

## Features

✅ **Streaming Support**: Full support for both streaming and non-streaming responses
✅ **Buffered Mode**: Request bodies buffered for processing
✅ **Immediate Response**: ExtProc returns responses directly via ImmediateResponse
✅ **Structured Logging**: JSON-formatted logs with slog
✅ **Proper Error Handling**: OpenAI-compatible error responses with appropriate status codes
✅ **Content Decompression**: Automatic handling of gzip and brotli compression
✅ **Health Checks**: gRPC health protocol support

## Troubleshooting

### ExtProc Not Connecting

Check health status:
```bash
docker-compose ps
docker-compose logs envoy-extproc
```

Verify gRPC connectivity:
```bash
docker-compose exec envoy-extproc grpc_health_probe -addr=:10000
```

### Requests Timing Out

Increase timeouts in `envoy.yaml`:
```yaml
message_timeout: 60s
max_message_timeout: 600s
```

### Ollama Model Not Found

Pull the model:
```bash
docker-compose exec ollama ollama pull llama3.2:3b
```

List available models:
```bash
docker-compose exec ollama ollama list
```

## Advanced Usage

### Using Different Backend

Modify `config.yaml` to point to OpenAI or another provider:

```yaml
engine:
  backend:
    url: "https://api.openai.com/v1"
    api_key: "sk-..."
```

### Custom Models

Update the request to use different models:

```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "input": "Hello!"
  }'
```

## Cleanup

```bash
# Stop services
docker-compose down

# Remove volumes (includes Ollama models)
docker-compose down -v
```

## Advanced Features

### Error Handling

The ExtProc service returns OpenAI-compatible error responses:

```json
{
  "error": {
    "message": "model field is required",
    "type": "invalid_request_error",
    "code": 422
  }
}
```

Error types:
- `400` - Bad request (malformed JSON)
- `422` - Unprocessable entity (validation errors)
- `500` - Internal server error (backend failures)

### Logging

Structured JSON logging with configurable levels:

```bash
# Set log level via command flag
docker-compose run envoy-extproc -log-level debug

# Or in docker-compose.yaml
command: ["-config", "/app/config.yaml", "-log-level", "info"]
```

Log levels: `debug`, `info`, `warn`, `error`

### Monitoring Endpoints

ExtProc metrics via Envoy admin:
```bash
# ExtProc-specific stats
curl http://localhost:9901/stats | grep ext_proc

# Connection stats
curl http://localhost:9901/stats | grep extproc_cluster
```

## Future Enhancements

- [ ] Request/response header inspection and modification
- [ ] Metrics collection (Prometheus)
- [ ] Distributed tracing (OpenTelemetry)
- [ ] Rate limiting integration
- [ ] Authentication/authorization hooks
- [ ] Integration tests with Testcontainers

## References

- [Envoy ExtProc Documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_proc_filter)
- [ExtProc Protocol](https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/ext_proc/v3/external_processor.proto)
- [OpenAI Responses API](https://platform.openai.com/docs/api-reference/responses)

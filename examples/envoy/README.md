# Envoy External Processor Integration

This example demonstrates running the OpenAI Responses Gateway with Envoy as the single entrypoint. The gateway runs as a single binary serving both HTTP (CRUD APIs) and gRPC (ExtProc for inference).

## Architecture

```
Client → Envoy:8080 ─┬─ POST /v1/responses ──→ ExtProc:10000 ──→ Ollama
                      │                              ↑
                      │                    (process response)
                      │
                      └─ Everything else ──→ HTTP Server:8080
                         /v1/files              (CRUD APIs)
                         /v1/vector_stores       same process
                         /v1/prompts
                         /v1/conversations
                         /v1/connectors
                         GET/DELETE /v1/responses/*
                         /health
```

**Key Design:**
- Single binary runs both HTTP and gRPC listeners
- ExtProc and HTTP server share the same engine, stores, and state
- Envoy routes POST /v1/responses through ExtProc for inference
- All other routes go directly to the HTTP server (ExtProc disabled per-route)
- The backend (Ollama/OpenAI) is called by the ExtProc service, not by Envoy

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
- **Gateway** (ports 8080, 10000): HTTP + gRPC in one process
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
- **Route splitting**: POST /v1/responses → backend (through ExtProc), everything else → gateway (ExtProc disabled)
- **ExtProc Filter**: Buffered body mode, 120s timeout, fail closed
- **Backend Cluster**: Routes to Ollama at port 11434
- **Gateway Cluster**: Routes to HTTP server at port 8080

### Gateway Configuration (extproc-config.yaml)

```yaml
extproc:
  port: 10000

engine:
  model_endpoint: http://ollama:11434/v1
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
docker-compose logs -f gateway
docker-compose logs -f envoy
docker-compose logs -f ollama
```

## Troubleshooting

### Gateway Not Connecting

Check health status:
```bash
docker-compose ps
docker-compose logs gateway
```

Verify gRPC connectivity:
```bash
docker-compose exec gateway grpc_health_probe -addr=:10000
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

## Cleanup

```bash
# Stop services
docker-compose down

# Remove volumes (includes Ollama models)
docker-compose down -v
```

## References

- [Envoy ExtProc Documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_proc_filter)
- [ExtProc Protocol](https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/ext_proc/v3/external_processor.proto)
- [OpenAI Responses API](https://platform.openai.com/docs/api-reference/responses)

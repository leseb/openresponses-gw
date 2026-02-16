# Deployment

## Standalone HTTP Server

Simple Go binary, no external dependencies (except storage).

```bash
export OPENAI_API_ENDPOINT="http://localhost:11434/v1"
export OPENAI_API_KEY="your-api-key"
./bin/openresponses-gw-server
```

Or with a config file:

```bash
./bin/openresponses-gw-server --config config.yaml
```

## Envoy External Processor

Works as an Envoy ExtProc filter. See `examples/envoy/` for configuration examples.

```yaml
# envoy.yaml
http_filters:
- name: envoy.filters.http.ext_proc
  typed_config:
    grpc_service:
      envoy_grpc:
        cluster_name: openresponses_gw
```

Quick start with Envoy:

```bash
# Build and start the ExtProc server
make build-extproc
OPENAI_API_ENDPOINT="http://localhost:8000/v1" OPENAI_API_KEY="unused" \
  ./bin/openresponses-gw-extproc -port 10000 &

# Start Envoy with the example config
envoy -c examples/envoy/envoy.yaml &

# Make requests through Envoy
curl -X POST http://localhost:8081/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","input":"Hello"}'
```

## Docker

A Dockerfile for the ExtProc binary is available at `deployments/docker/Dockerfile.envoy-extproc`.

## Extensibility

The adapter design is extensible — new gateway adapters can be added under `pkg/adapters/`. Each adapter translates between the gateway's protocol and the core engine, which is fully gateway-agnostic.

## Backend Configuration

Connect to any OpenAI-compatible backend via environment variables:

| Backend | `OPENAI_API_ENDPOINT` | `OPENAI_API_KEY` |
|---------|----------------------|-------------------|
| OpenAI | `https://api.openai.com/v1` | Required |
| Ollama | `http://localhost:11434/v1` | Not needed |
| vLLM | `http://your-server:8000/v1` | Optional |

## Session Store Configuration

By default, sessions, conversations, and responses are stored in memory. For persistence across restarts, use the SQLite backend:

```bash
export SESSION_STORE_TYPE=sqlite
export SESSION_STORE_DSN=data/responses.db
```

Or in `config.yaml`:

```yaml
session_store:
  type: sqlite
  dsn: data/responses.db
```

| Backend | Persistence | Use Case |
|---------|-------------|----------|
| `memory` (default) | None — data lost on restart | Development, testing |
| `sqlite` | Local disk (pure Go, no CGO) | Production, single-node deployments |

When using Docker, mount a volume for the SQLite database:

```bash
docker run -p 8080:8080 \
  -e SESSION_STORE_TYPE=sqlite \
  -e SESSION_STORE_DSN=/data/responses.db \
  -v $(pwd)/data:/data \
  openresponses-gw:latest
```

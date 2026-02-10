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

Works as an Envoy ExtProc filter. See `examples/envoy/` for a complete docker-compose setup.

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
cd examples/envoy
docker-compose up -d

curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","input":"Hello"}'
```

## Docker

```bash
make docker-build       # Build Docker image
make docker-run         # Run in Docker
```

## Extensibility

The adapter design is extensible — new gateway adapters can be added under `pkg/adapters/`. Each adapter translates between the gateway's protocol and the core engine, which is fully gateway-agnostic.

## Backend Configuration

Connect to any OpenAI-compatible backend via environment variables:

| Backend | `OPENAI_API_ENDPOINT` | `OPENAI_API_KEY` |
|---------|----------------------|-------------------|
| OpenAI | `https://api.openai.com/v1` | Required |
| Ollama | `http://localhost:11434/v1` | Not needed |
| vLLM | `http://your-server:8000/v1` | Optional |

# Deployment

## HTTP Server

Simple Go binary, no external dependencies (except storage).

```bash
export OPENAI_API_ENDPOINT="http://localhost:11434/v1"
export OPENAI_API_KEY="your-api-key"
./bin/openresponses-gw
```

Or with a config file:

```bash
./bin/openresponses-gw --config config.yaml
```

## Behind a Reverse Proxy

The gateway can be deployed behind any reverse proxy (Envoy, nginx, HAProxy) as a regular upstream service for TLS termination, load balancing, rate limiting, and observability. For inference-aware routing with Envoy, consider [Gateway API Inference Extension (GIE)](https://gateway-api-inference-extension.sigs.k8s.io/).

## Backend Configuration

Connect to any OpenAI-compatible backend via environment variables:

| Backend | `OPENAI_API_ENDPOINT` | `OPENAI_API_KEY` |
|---------|----------------------|-------------------|
| OpenAI | `https://api.openai.com/v1` | Required |
| Ollama | `http://localhost:11434/v1` | Not needed |
| vLLM | `http://your-server:8000/v1` | Optional |

## Session Store Configuration

By default, sessions, conversations, and responses are stored in memory. For persistence across restarts, use SQLite or PostgreSQL:

```bash
# SQLite
export SESSION_STORE_TYPE=sqlite
export SESSION_STORE_DSN=data/responses.db

# PostgreSQL
export SESSION_STORE_TYPE=postgres
export SESSION_STORE_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

Or in `config.yaml`:

```yaml
# SQLite
session_store:
  type: sqlite
  dsn: data/responses.db

# PostgreSQL
session_store:
  type: postgres
  dsn: postgres://user:pass@host:5432/dbname?sslmode=disable
```

| Backend | Persistence | Use Case |
|---------|-------------|----------|
| `memory` (default) | None — data lost on restart | Development, testing |
| `sqlite` | Local disk (pure Go, no CGO) | Single-node deployments |
| `postgres` | PostgreSQL database (via `pgx/v5`) | Production, multi-node deployments |

When using Docker with SQLite, mount a volume for the database:

```bash
docker run -p 8080:8080 \
  -e SESSION_STORE_TYPE=sqlite \
  -e SESSION_STORE_DSN=/data/responses.db \
  -v $(pwd)/data:/data \
  openresponses-gw:latest
```

When using Docker with PostgreSQL:

```bash
docker run -p 8080:8080 \
  -e SESSION_STORE_TYPE=postgres \
  -e SESSION_STORE_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable" \
  openresponses-gw:latest
```

# Open Responses Gateway

![CI](https://github.com/leseb/openresponses-gw/actions/workflows/ci.yml/badge.svg)
![Open Responses Compliant](https://img.shields.io/badge/Open%20Responses-100%25%20Compliant-brightgreen)
![OpenAI Compatible](https://img.shields.io/badge/OpenAI%20API-99.5%25%20Schema%20Compatible-blue)

The **stateful layer** for the [Open Responses API](https://github.com/openresponses/openresponses) — adds persistence, conversations, file search, MCP tools, and prompts on top of any `/v1/responses`-compatible inference backend.

## Why

Inference servers like vLLM now expose the
[Open Responses API](https://github.com/openresponses/openresponses)
natively via `/v1/responses` — but they focus on LLM generation. A
production deployment also needs:

- **Persistent storage** — responses and conversations survive restarts
- **Files & Vector Stores** — upload documents, chunk, embed, and search (RAG)
- **Server-side tool execution** — file_search over vector stores, MCP tool
  calling via registered connectors
- **Conversations API** — multi-turn state management across requests
- **Prompts API** — versioned prompt templates

This gateway sits in front of any `/v1/responses`-compatible backend and
adds everything above. The inference backend does what it does best (LLM
generation), and the gateway handles the rest.

```
                                    ┌──────────────┐
                                    │  Inference    │
    Client ──> openresponses-gw ──> │  Backend      │
               (stateful tier)      │  (vLLM, etc)  │
               - storage            │               │
               - conversations      │ /v1/responses  │
               - files & vectors    └──────────────┘
               - MCP connectors
               - file_search
               - prompts
```

## Quick Start

**Prerequisites:** Go 1.24+, Make, and a `/v1/responses`-compatible backend (e.g. [vLLM](https://docs.vllm.ai))

```bash
# Start vLLM with a model
python -m vllm.entrypoints.openai.api_server --model <model>

# Clone and run the gateway
git clone https://github.com/leseb/openresponses-gw && cd openresponses-gw
export OPENAI_API_ENDPOINT="http://localhost:8000/v1"
export OPENAI_API_KEY="unused"
make run

# In another terminal
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model": "<model>", "input": "Hello, world!"}'
```

To use OpenAI instead, set `OPENAI_API_ENDPOINT="https://api.openai.com/v1"` and `OPENAI_API_KEY` to your API key.

For Envoy deployment, see [docs/DEPLOYMENT.md](./docs/DEPLOYMENT.md).

## API Status

| API | Endpoints | Status |
|-----|-----------|--------|
| **Responses** | 5 | Working |
| **Conversations** | 6 | Working |
| **Prompts** | 5 | Working |
| **Files** | 5 | Working |
| **Vector Stores** | 15 | Working (search via Milvus) |

**Vector search / RAG:** File ingestion (chunk, embed, insert) and vector search are supported via a pluggable backend. Milvus is the default vector store. The `file_search` tool in the Responses API is executed server-side when an embedding service and vector backend are configured. See [CONFIGURATION.md](./CONFIGURATION.md#vector-store--embedding-configuration) for setup.

**Not yet implemented:** web_search tool execution, vision/multimodal.

See [FUNCTIONAL_CONFORMANCE.md](./FUNCTIONAL_CONFORMANCE.md) for details.

## Development

```bash
make build                       # Build all binaries
make test                        # Unit tests
make lint                        # golangci-lint
make fmt                         # Format code
make test-conformance            # Open Responses spec conformance
make test-integration-python     # Python integration tests (requires uv)
make test-openapi-conformance    # OpenAI API schema comparison
make pre-commit-install          # Install pre-commit hooks
```

## Project Structure

```
cmd/
  server/              # Standalone HTTP server
  envoy-extproc/       # Envoy External Processor
pkg/
  core/                # Gateway-agnostic core (engine, config, schema, Responses API client)
  adapters/
    http/              # HTTP server adapter
    envoy/             # Envoy ExtProc adapter
  storage/
    memory/            # In-memory store (sessions, files, vectors)
  vectorstore/         # Vector store backends (Milvus, memory)
scripts/               # Utility & validation scripts
tests/
  scripts/             # Shell-based test scripts
  integration/         # Python integration tests
examples/
  envoy/               # Envoy docker-compose deployment
```

## Documentation

- [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) — System design and request flow
- [docs/DEPLOYMENT.md](./docs/DEPLOYMENT.md) — Deployment modes and backend configuration
- [FUNCTIONAL_CONFORMANCE.md](./FUNCTIONAL_CONFORMANCE.md) — What works and what doesn't
- [TESTING.md](./TESTING.md) — Test infrastructure guide
- [CONFORMANCE_STATUS.md](./CONFORMANCE_STATUS.md) — OpenAPI conformance journey
- [CONTRIBUTING.md](./CONTRIBUTING.md) — Development guidelines
- [PROJECT_PLAN.md](./PROJECT_PLAN.md) — Roadmap
- [openapi.yaml](./openapi.yaml) — Full API specification

## License

Apache 2.0 — See [LICENSE](./LICENSE)

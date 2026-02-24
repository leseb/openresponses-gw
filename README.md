# Open Responses Gateway

![CI](https://github.com/leseb/openresponses-gw/actions/workflows/ci.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/leseb/openresponses-gw)](https://goreportcard.com/report/github.com/leseb/openresponses-gw)

The **stateful layer** for the [Open Responses API](https://github.com/openresponses/openresponses) — adds persistence, conversations, file search, web search, MCP tools, content extraction, and prompts on top of any `/v1/responses`-compatible inference backend.

## Why

Inference servers like vLLM now expose the
[Open Responses API](https://github.com/openresponses/openresponses)
natively via `/v1/responses` — but they focus on LLM generation. A
production deployment also needs:

- **Persistent storage** — responses and conversations survive restarts
- **Files & Vector Stores** — upload documents, chunk, embed, and search (RAG)
- **Content extraction** — PDF, HTML, CSV, JSON/JSONL files extracted to text for vector ingestion
- **Server-side tool execution** — file_search over vector stores, web_search
  via Brave or Tavily, MCP tool calling via registered connectors
- **Citations** — url_citation and file_citation annotations on output text
- **Conversations API** — multi-turn state management across requests
- **Prompts API** — versioned prompt templates

This gateway sits in front of any `/v1/responses`-compatible backend and
adds everything above. The inference backend does what it does best (LLM
generation), and the gateway handles the rest. For a detailed breakdown of
which API fields are handled by the inference backend versus the gateway,
run `make vllm-field-tracking` to see the full report.

```
                                    ┌────────────────┐
                                    │  Inference     │
    Client ──> openresponses-gw ──> │  Backend       │
               (stateful tier)      │  (vLLM, etc)   │
               - storage            │                │
               - conversations      │ /v1/responses  │
               - files & vectors    └────────────────┘
               - MCP connectors
               - file_search
               - web_search
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

## Documentation

- [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) — System design and request flow
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md) — Configuration guide
- [docs/DEPLOYMENT.md](./docs/DEPLOYMENT.md) — Deployment modes and backend configuration
- [docs/TESTING.md](./docs/TESTING.md) — Test infrastructure guide
- `make vllm-field-tracking` — vLLM vs gateway field tracking report
- [docs/openapi.yaml](./docs/openapi.yaml) — Full API specification
- [CONTRIBUTING.md](./CONTRIBUTING.md) — Development guidelines

## License

Apache 2.0 — See [LICENSE](./LICENSE)

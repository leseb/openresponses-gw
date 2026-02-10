# Open Responses Gateway

![CI](https://github.com/leseb/openresponses-gw/actions/workflows/ci.yml/badge.svg)
![Open Responses Compliant](https://img.shields.io/badge/Open%20Responses-100%25%20Compliant-brightgreen)
![OpenAI Compatible](https://img.shields.io/badge/OpenAI%20API-99.5%25%20Schema%20Compatible-blue)

A gateway-agnostic implementation of the [Open Responses API](https://github.com/openresponses/openresponses) — translates the Responses API to Chat Completions against any OpenAI-compatible backend (OpenAI, Ollama, vLLM, etc.).

## Quick Start

**Prerequisites:** Go 1.24+, Make, and an OpenAI-compatible backend (e.g. [Ollama](https://ollama.com))

```bash
# Install and start Ollama, then pull a model
ollama pull llama3.2:3b

# Clone and run the gateway
git clone https://github.com/leseb/openresponses-gw && cd openresponses-gw
export OPENAI_API_ENDPOINT="http://localhost:11434/v1"
export OPENAI_API_KEY="unused"
make run

# In another terminal
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model": "llama3.2:3b", "input": "Hello, world!"}'
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
| **Vector Stores** | 15 | Working (search is stub) |
| **Models** | 2 | Working |

**Not yet implemented:** RAG integration, file_search/web_search tools, vision/multimodal.

See [FUNCTIONAL_CONFORMANCE.md](./FUNCTIONAL_CONFORMANCE.md) for details.

## Development

```bash
make build                       # Build all binaries
make test                        # Unit tests
make lint                        # golangci-lint
make fmt                         # Format code
make test-conformance            # Open Responses spec conformance
make test-openapi-conformance    # OpenAI API schema comparison
make pre-commit-install          # Install pre-commit hooks
```

## Project Structure

```
cmd/
  server/              # Standalone HTTP server
  envoy-extproc/       # Envoy External Processor
pkg/
  core/                # Gateway-agnostic core (engine, config, schema, API client)
  adapters/
    http/              # HTTP server adapter
    envoy/             # Envoy ExtProc adapter
  storage/
    memory/            # In-memory store (sessions, files, vectors)
scripts/               # Testing & conformance scripts
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

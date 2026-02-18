# CLAUDE.md — Open Responses Gateway

## Important workflow rules

- **Always run `pre-commit run --all-files` before pushing code.** Fix any failures, amend the commit, then push.

## Quick reference

```bash
make build                    # Build single gateway binary (HTTP + ExtProc)
make run                      # Build and run (HTTP :8080 + ExtProc :10000)
make test                     # Unit tests (go test -v -race ./...)
make lint                     # golangci-lint
make gen-openapi              # Regenerate docs/openapi.yaml (needs swag on PATH)
make test-openapi-conformance # Check OpenAPI conformance vs OpenAI spec
make vllm-field-tracking      # Show forwarded/accepted/missing vLLM fields
make pre-commit               # Run all pre-commit hooks
```

## Python

Always use `uv run` to execute Python — never bare `python` or `pip`.

```bash
uv run --with pyyaml python scripts/fix-openapi-nullable.py docs/openapi.yaml
uv run pytest tests/integration/ -v
make test-integration-python  # wraps uv run pytest
```

## Tools that must be on PATH

| Tool | Install | Notes |
|------|---------|-------|
| `swag` (v2) | `make install-swag` | Installs to `~/go/bin`; may need `export PATH="$HOME/go/bin:$PATH"` |
| `uv` | `brew install uv` | Python package manager — used for all Python execution |
| `oasdiff` | `brew install oasdiff` | OpenAPI diff tool for conformance checks |
| `golangci-lint` | `brew install golangci-lint` | Go linter |
| `pre-commit` | `brew install pre-commit` | Git hook manager |

## OpenAPI spec pipeline

The spec is **auto-generated** — never edit `docs/openapi.yaml` by hand.

1. `swag init --v3.1` generates from Go swagger annotations → `docs/openapi.yaml`
2. `scripts/fix-openapi-nullable.py` post-processes to fix swag limitations:
   - Nullable fields (Go pointer types → `anyOf` with `{type: "null"}`)
   - Files API multipart schema
   - Removes `type: object` from File schema
   - Unwraps bogus `oneOf` wrappers on requestBody schemas
   - Chunking strategy union variants (request: auto/static, response: static/other)
   - Search filter union variants (ComparisonFilter/CompoundFilter)
   - Search request default values
3. Pre-commit hooks verify the spec is up to date and conformant

When adding new fields to schemas, add proper `enums:"..."` tags so swag generates
enum constraints. For union types that swag can't express, add post-processing in
`fix-openapi-nullable.py`.

## Architecture

```
cmd/
  server/          → Single binary: HTTP + optional gRPC ExtProc (--extproc-port)
pkg/
  adapters/
    http/          → HTTP handlers, SSE streaming, OpenAPI serving
    envoy/         → gRPC ExtProc processor (shares engine with HTTP)
  core/
    engine/        → Main orchestrator: LLM calls, agentic tool loops, streaming
    schema/        → API type definitions (add swagger tags here)
    api/           → Backend client interfaces (vLLM/OpenAI)
    config/        → YAML + env var configuration
    services/      → Vector store ingestion/search coordination
  storage/         → Persistence backends (memory, SQLite)
  filestore/       → File storage backends (memory, filesystem, S3)
  vectorstore/     → Vector search backends (memory, Milvus)
  mcp/             → Model Context Protocol client
scripts/
  fix-openapi-nullable.py  → OpenAPI post-processing
  conformance/             → OpenAI spec + conformance checker
  vllm/                    → vLLM field tracking
```

## Key patterns

- **Request flow**: HTTP handler → `engine.ProcessRequest()` → `api.ResponsesOpenAIClient` → vLLM/OpenAI
- **Streaming flow**: HTTP handler → `engine.ProcessRequestStream()` → SSE events channel → `handleStreamingResponse()` flushes to client
- **Agentic loop**: Engine iterates up to 10 times, executing server-side tools (MCP, file_search) between LLM calls
- **ExtProc adapter**: Shares engine/stores with HTTP adapter in a single process. Does NOT support streaming — uses `httptest.NewRecorder()` which buffers full response.

## Conventions

- Go 1.24+, formatted with `gofmt`
- Conventional commits: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`
- Always run `pre-commit run --all-files` before pushing
- Table-driven tests preferred
- `omitempty` on optional JSON fields; pointer types for nullable fields
- Swagger annotations on handler functions for OpenAPI generation

## Gotchas

- `swag` installs to `~/go/bin` which may not be on PATH in pre-commit hooks — the `gen-openapi` hook will fail with "swag not installed" if so
- The `SearchVectorStoreRequest.Query` field is `interface{}` (accepts string or `[]string`) — swag generates it as "any" type; the post-processing script fixes it
- vLLM normalizations happen in `engine.go`: content_index remapping, lifecycle event management, output_item/content_part announcement tracking
- `VectorStoreFile.ChunkingStrategy` uses one Go type but needs different OpenAI union variants for request vs response contexts — handled in post-processing

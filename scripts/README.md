# Scripts Directory

This directory contains utility scripts for development, testing, and CI/CD.

## Pre-commit Hooks

### Setup

Install pre-commit hooks:

```bash
# Install pre-commit (if not already installed)
pip install pre-commit
# or
brew install pre-commit

# Install the git hook scripts
pre-commit install
```

### Available Hooks

The pre-commit configuration (`.pre-commit-config.yaml`) includes:

1. **Code Quality**
   - Go formatting (`go fmt`)
   - Go imports organization (`go imports`)
   - Go linting (`go vet`, `golangci-lint`)
   - Dependency cleanup (`go mod tidy`)

2. **General Checks**
   - Trailing whitespace removal
   - End-of-file fixing
   - Large file detection
   - Merge conflict detection
   - YAML validation

3. **OpenAPI Validation**
   - OpenAPI spec schema validation
   - Consistency check between `openapi.yaml` and `openapi.go`

4. **Conformance Testing**
   - Build server binary
   - Run Open Responses conformance tests

### Manual Execution

Run all hooks:
```bash
pre-commit run --all-files
```

Run specific hook:
```bash
pre-commit run go-fmt --all-files
pre-commit run openresponses-conformance --all-files
```

## Conformance Testing

### run-conformance-tests.sh

Runs the official Open Responses conformance test suite against the local server.

**Requirements:**
- Node.js with `npx` OR Bun runtime
- Server binary built (`make build-server`)

**Usage:**
```bash
# Run with defaults
./scripts/run-conformance-tests.sh

# Run with custom port
SERVER_PORT=9000 ./scripts/run-conformance-tests.sh

# Run with specific model
OPENRESPONSES_MODEL=gpt-4 ./scripts/run-conformance-tests.sh
```

**Environment Variables:**
- `SERVER_PORT` - Port for test server (default: 8080)
- `OPENRESPONSES_MODEL` - Model to use in tests (default: gpt-4)
- `OPENAI_API_KEY` - API key for backend LLM (default: test-key)

**What it does:**
1. Clones/updates the openresponses conformance test repository
2. Installs test dependencies (Bun or npm)
3. Builds and starts the server
4. Runs 6 conformance tests:
   - Basic text response
   - Streaming response
   - System prompt
   - Tool calling
   - Image input
   - Multi-turn conversation
5. Reports results with colored output
6. Cleans up server process

**Exit codes:**
- `0` - All tests passed
- `1` - One or more tests failed

## OpenAPI Validation

### validate-openapi-sync.sh

Validates that `openapi.yaml` and `pkg/adapters/http/openapi.go` are in sync.

**Requirements:**
- `yq` (optional, for detailed validation)

**Usage:**
```bash
./scripts/validate-openapi-sync.sh
```

**What it checks:**
- Both files exist
- Key endpoints match (/v1/responses, /v1/chat/completions)
- Files modified together (warning if >1 hour apart)

**Exit codes:**
- `0` - Files are consistent
- `1` - Files are out of sync

## Integration with Make

These scripts are also available as Makefile targets:

```bash
# Run conformance tests
make test-conformance

# Validate OpenAPI sync
make validate-openapi

# Run all pre-commit checks
make pre-commit
```

## CI/CD Usage

### GitHub Actions Example

```yaml
name: Conformance Tests

on: [push, pull_request]

jobs:
  conformance:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Set up Bun
        uses: oven-sh/setup-bun@v1

      - name: Build server
        run: make build-server

      - name: Run conformance tests
        run: ./scripts/run-conformance-tests.sh
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
```

## Troubleshooting

### Conformance tests fail to start

Check that:
1. Server binary exists: `ls -l bin/responses-gateway-server`
2. Config file exists: `ls -l config/config.yaml`
3. Port 8080 is available: `lsof -i :8080`

### OpenAPI validation warnings

If you see timestamp warnings:
1. Update both files together when making changes
2. Commit both files in the same commit
3. Run `./scripts/validate-openapi-sync.sh` before committing

### Pre-commit hooks too slow

Skip expensive hooks during rapid development:
```bash
SKIP=openresponses-conformance git commit -m "WIP: quick fix"
```

Re-enable for final commits:
```bash
git commit -m "feat: implement new feature"
```

## Adding New Checks

To add a new pre-commit hook:

1. Edit `.pre-commit-config.yaml`
2. Add hook under appropriate repo or create local hook
3. Test with `pre-commit run <hook-id> --all-files`
4. Document in this README

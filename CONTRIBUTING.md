# Contributing to Open Responses Gateway

Thank you for your interest in contributing! This document provides guidelines and instructions for contributing to the project.

## Development Setup

### Prerequisites

- Go 1.24 or later
- Make

### Initial Setup

```bash
# 1. Clone the repository
git clone https://github.com/leseb/openresponses-gw
cd openresponses-gw

# 2. Download dependencies
make init

# 3. Run tests
make test
```

## Development Workflow

### Building

```bash
# Build the gateway binary (HTTP + ExtProc)
make build
```

### Running Locally

```bash
# Run with auto-reload (requires air)
make run-dev

# Or build and run
make run
```

### Testing

```bash
# Run unit tests
make test

# Run with coverage
make test-coverage

# Run Python integration tests (requires uv and a running server)
make test-integration-python

# Run integration tests through Envoy ExtProc
make test-integration-envoy
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Run vet
make vet
```

## Code Style

### Go Style Guide

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting
- Write clear, descriptive variable names
- Add comments for exported functions and types
- Keep functions small and focused

### Example

```go
// ProcessRequest processes a Responses API request and returns a response.
// It validates the request, executes tools if needed, and calls the LLM.
func (e *Engine) ProcessRequest(ctx context.Context, req *schema.ResponseRequest) (*schema.Response, error) {
    // Implementation
}
```

## Testing Guidelines

### Unit Tests

- Test file name: `*_test.go`
- Use table-driven tests where appropriate
- Mock external dependencies
- Aim for >80% coverage

Example:

```go
func TestEngine_ProcessRequest(t *testing.T) {
    tests := []struct {
        name    string
        request *schema.ResponseRequest
        want    *schema.Response
        wantErr bool
    }{
        {
            name: "valid request",
            request: &schema.ResponseRequest{
                Model: "gpt-4",
                Input: "Hello",
            },
            wantErr: false,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## Commit Message Format

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

### Examples

```
feat(engine): add support for streaming responses

Implement SSE streaming for the Responses API. Events are
sent as server-sent events with proper error handling.

Closes #123
```

```
fix(storage): prevent race condition in session store

Add mutex to protect concurrent access to session map.

Fixes #456
```

## Pull Request Process

### Before Submitting

1. **Create an issue** describing the change
2. **Fork the repository** and create a feature branch
3. **Write tests** for your changes
4. **Run the test suite**: `make test`
5. **Run the linter**: `make lint`
6. **Update documentation** if needed

### Review Process

1. Automated checks must pass (CI/CD)
2. At least one maintainer approval required
3. Address review comments
4. Squash commits before merge (if requested)

## Project Structure

See [ARCHITECTURE.md](./docs/ARCHITECTURE.md) for detailed architecture.

```
pkg/
├── core/              # Gateway-agnostic core (NO gateway dependencies)
│   ├── engine/        # Main orchestration
│   ├── schema/        # API schemas
│   ├── config/        # Configuration loading
│   ├── api/           # Backend API clients
│   ├── services/      # Vector store service
│   └── state/         # State management interfaces
├── adapters/          # Gateway-specific adapters
│   ├── http/          # Standard HTTP server
│   └── envoy/         # Envoy ExtProc (delegates to HTTP handler)
├── storage/           # Storage implementations
│   ├── sqlite/        # SQLite session store
│   └── memory/        # In-memory stores (prompts, connectors, vector stores)
├── filestore/         # File storage backends
│   ├── memory/        # In-memory file store
│   ├── filesystem/    # Local filesystem
│   └── s3/            # AWS S3
└── vectorstore/       # Vector store backends
    └── milvus/        # Milvus backend
```

### Adding a New Adapter

1. Create package in `pkg/adapters/`
2. Implement `http.Handler` interface
3. Add tests
4. Add example in `examples/`
5. Update documentation

### Adding a New Storage Backend

1. Create package in `pkg/storage/`
2. Implement `SessionStore` interface
3. Add migrations (if applicable)
4. Add tests
5. Update configuration

## Getting Help

- **Questions**: Open a GitHub Discussion
- **Bugs**: Open a GitHub Issue

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

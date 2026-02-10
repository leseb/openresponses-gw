# Contributing to Open Responses Gateway

Thank you for your interest in contributing! This document provides guidelines and instructions for contributing to the project.

## Development Setup

### Prerequisites

- Go 1.23 or later
- Docker and Docker Compose
- Make
- PostgreSQL 16+ (optional, for local development)
- Redis 7+ (optional, for local development)

### Initial Setup

```bash
# 1. Clone the repository
git clone https://github.com/leseb/openresponses-gw
cd openai-openresponses-gw

# 2. Install dependencies
make deps

# 3. Install development tools
make tools

# 4. Start development dependencies
docker-compose up -d postgres redis

# 5. Run tests
make test
```

## Development Workflow

### Building

```bash
# Build all binaries
make build

# Build specific binary
make build-server
make build-extproc
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

# Run integration tests
make test-integration

# Run end-to-end tests
make test-e2e
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

### Integration Tests

- Use `testcontainers-go` for database/redis
- Tag with `//go:build integration`
- Clean up resources after tests

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

### PR Template

```markdown
## Description
Brief description of changes

## Related Issue
Closes #123

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests added/updated
- [ ] Manual testing performed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Comments added for complex logic
- [ ] Documentation updated
- [ ] No new warnings generated
- [ ] Tests pass locally
```

### Review Process

1. Automated checks must pass (CI/CD)
2. At least one maintainer approval required
3. Address review comments
4. Squash commits before merge (if requested)

## Project Structure

See [PROJECT_PLAN.md](./PROJECT_PLAN.md) for detailed architecture.

```
pkg/
├── core/              # Gateway-agnostic core (NO gateway dependencies)
│   ├── engine/        # Main orchestration
│   ├── schema/        # API schemas
│   ├── state/         # State management interfaces
│   └── tools/         # Tool execution framework
├── adapters/          # Gateway-specific adapters
│   ├── http/          # Standard HTTP
│   └── envoy/         # Envoy ExtProc
└── storage/           # Storage implementations
    ├── postgres/      # PostgreSQL
    ├── redis/         # Redis
    └── memory/        # In-memory (dev/test)
```

### Adding a New Adapter

1. Create package in `pkg/adapters/`
2. Implement `Adapter` interface
3. Add tests
4. Add example in `examples/`
5. Update documentation

### Adding a New Storage Backend

1. Create package in `pkg/storage/`
2. Implement `SessionStore` interface
3. Add migrations (if applicable)
4. Add tests
5. Update configuration

### Adding a New Tool

1. Create file in `pkg/core/tools/builtin/` or `pkg/core/tools/custom/`
2. Implement `Tool` interface
3. Register in tool registry
4. Add tests
5. Update documentation

## Release Process

1. Update version in code
2. Update CHANGELOG.md
3. Create release PR
4. Tag release: `git tag -a v0.x.0 -m "Release v0.x.0"`
5. Push tag: `git push origin v0.x.0`
6. GitHub Actions will build and publish

## Getting Help

- **Questions**: Open a GitHub Discussion
- **Bugs**: Open a GitHub Issue
- **Security**: Email security@example.com

## Code of Conduct

- Be respectful and inclusive
- Welcome newcomers
- Focus on constructive feedback
- Follow the [Contributor Covenant](https://www.contributor-covenant.org/)

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

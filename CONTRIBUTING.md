# Contributing to NestGo Gin Adapter

Thank you for your interest in contributing!

## Getting Started

1. Fork and clone the repository
2. Install dependencies: `go mod download`
3. Run tests: `go test ./...`
4. Make your changes
5. Submit a pull request

## Development

### Prerequisites

- Go 1.23 or later
- Gin v1.12+

### Project Structure

```
nestgo-gin-adapter/
  context.go       # GinContext — implements core.Context
  server.go        # GinServer/GinRouter — implements core.Server/Router
  doc.go           # Package documentation
  example_test.go  # Testable examples for pkg.go.dev
```

### Key Design Principles

1. **Implement core interfaces only** — this adapter should never add methods beyond what `core.Server`, `core.Router`, and `core.Context` define.
2. **Zero allocation on hot path** — use `sync.Pool` for contexts, avoid allocations in per-request code.
3. **Body caching** — `Body()` reads once and caches. `Bind()` restores the body reader after a cached read.
4. **Error propagation** — handler errors flow through `core.ErrorHandler`, with `gc.Abort()` to stop Gin's chain.

### How to Add a New core.Context Method

If the core package adds a new method to the `Context` interface:

1. Add the implementation to `GinContext` in `context.go`
2. Verify the compile-time check still passes (`var _ core.Context = (*GinContext)(nil)`)
3. Add a test

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep comments minimal — code should be self-documenting
- Use Go doc format for exported symbols

## Submitting Changes

1. Create a feature branch from `main`
2. Write clear commit messages
3. Ensure `go build ./...` and `go vet ./...` pass
4. Open a PR with a description of what changed and why

<p align="center">
  <h1 align="center">NestGo Gin Adapter</h1>
  <p align="center">Gin adapter for the NestGo framework.</p>
</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/ashrafAli23/nestgo-gin-adapter"><img src="https://pkg.go.dev/badge/github.com/ashrafAli23/nestgo-gin-adapter.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/ashrafAli23/nestgo-gin-adapter"><img src="https://goreportcard.com/badge/github.com/ashrafAli23/nestgo-gin-adapter" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
</p>

---

This package implements NestGo's `core.Server`, `core.Router`, and `core.Context` interfaces on top of [Gin](https://gin-gonic.com), letting you use NestGo's Guards, Interceptors, Pipes, and Middleware ecosystem with Gin's battle-tested HTTP engine.

## Install

```bash
go get github.com/ashrafAli23/nestgo-gin-adapter
```

**Prerequisites:**

```bash
go get github.com/ashrafAli23/nestgo      # core framework
go get github.com/gin-gonic/gin           # gin
```

## Quick Start

```go
package main

import (
    "github.com/ashrafAli23/nestgo/core"
    gin "github.com/ashrafAli23/nestgo-gin-adapter"
    "github.com/ashrafAli23/nestgo/middleware"
)

func main() {
    server := gin.New(core.DefaultConfig())

    // NestGo middleware works out of the box
    server.Use(middleware.Recovery())
    server.Use(middleware.CORS())
    server.Use(middleware.RequestID())

    server.GET("/hello", func(c core.Context) error {
        return c.JSON(200, map[string]string{"message": "Hello from Gin!"})
    })

    server.Start(":3000")
}
```

## Swapping from Fiber

NestGo's adapter pattern means switching from Fiber to Gin is a one-line change:

```diff
  import (
      "github.com/ashrafAli23/nestgo/core"
-     adapter "github.com/ashrafAli23/nestgo-fiber-adapter"
+     adapter "github.com/ashrafAli23/nestgo-gin-adapter"
  )

  func main() {
      server := adapter.New(core.DefaultConfig())
      // ... all handlers, middleware, guards, etc. remain identical
  }
```

## Features

### Context Pooling

Contexts are managed with `sync.Pool` to achieve **zero allocation per request** for context structs. Each request acquires a `GinContext` from the pool and releases it back after the handler returns.

### Body Caching

Gin's `GetRawData()` drains the request body — calling it twice returns empty on the second call. `GinContext.Body()` reads once and caches the result. Subsequent calls to `Body()` or `Bind()` reuse the cached data and restore the body reader automatically.

```go
server.POST("/echo", func(c core.Context) error {
    body, _ := c.Body()     // reads and caches
    body2, _ := c.Body()    // returns cached copy
    _ = c.Bind(&myStruct)   // also works — body is restored

    return c.JSON(200, map[string]string{"received": string(body)})
})
```

### Safe Goroutine Usage with Clone

Use `Clone()` to create a goroutine-safe context copy:

```go
server.GET("/async", func(c core.Context) error {
    cloned := c.Clone()
    go func() {
        ip := cloned.ClientIP()   // safe
        method := cloned.Method() // safe
        _ = ip
        _ = method
    }()
    return c.JSON(202, map[string]string{"status": "accepted"})
})
```

`Clone()` delegates to `gin.Context.Copy()`, which copies the request and all context values into an independent struct.

### Route Groups

```go
api := server.Group("/api/v1")
api.GET("/users", listUsers)
api.POST("/users", createUser)

// Nested groups with middleware
admin := api.Group("/admin", middleware.RateLimit(middleware.RateLimitConfig{
    Max:    10,
    Window: time.Minute,
}))
admin.DELETE("/users/:id", deleteUser)
```

### Accessing Raw Gin APIs

For Gin-specific features not covered by NestGo's abstractions:

```go
// Access the *gin.Engine
engine := server.Underlying().(*gin.Engine)
engine.SetTrustedProxies([]string{"192.168.1.0/24"})
engine.LoadHTMLGlob("templates/*")
engine.Static("/public", "./static")

// Access *gin.Context inside a handler
server.GET("/raw", func(c core.Context) error {
    gc := c.Underlying().(*gin.Context)
    // Use Gin-specific APIs directly
    return c.JSON(200, nil)
})
```

### Debug vs Release Mode

```go
// Debug mode — Gin logs all routes and requests
server := gin.New(&core.Config{Debug: true})

// Release mode (default) — no debug logging
server := gin.New(&core.Config{Debug: false})

// Disable Gin's default logger entirely (bring your own)
server := gin.New(&core.Config{DisableLogger: true})
```

### Graceful Shutdown

`GinServer` wraps Gin in a standard `net/http.Server`, so shutdown is fully Go-native:

```go
go func() {
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    server.Shutdown(ctx)
}()

server.Start(":3000")
```

## Configuration

Pass a `*core.Config` to `New()`:

```go
server := gin.New(&core.Config{
    AppName:       "my-api",
    Addr:          ":8080",
    Debug:         false,
    DisableLogger: false,
    ReadTimeout:   30,
    WriteTimeout:  30,
    BodyLimit:     10 * 1024 * 1024,
    ErrorHandler:  customErrorHandler,
})
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `AppName` | `string` | `""` | Application name |
| `Addr` | `string` | `":3000"` | Default listen address |
| `Debug` | `bool` | `false` | Gin debug mode (logs routes) |
| `DisableLogger` | `bool` | `false` | Skip Gin's default logger middleware |
| `ReadTimeout` | `int` | `0` | Read timeout in seconds |
| `WriteTimeout` | `int` | `0` | Write timeout in seconds |
| `BodyLimit` | `int` | `0` | Max request body size in bytes |
| `ErrorHandler` | `core.ErrorHandler` | `nil` | Custom error handler (defaults to `core.DefaultErrorHandler`) |

## Performance

| Optimization | Technique | Impact |
|-------------|-----------|--------|
| Context pooling | `sync.Pool` | Zero alloc per request |
| Body caching | Slice reuse via `append([:0])` | No double-read allocation |
| Standard HTTP server | `net/http.Server` | Full timeout control |
| Middleware chaining | Compile-time composition | No per-request chain rebuild |

## Compatibility

| Dependency | Version |
|-----------|---------|
| Go | 1.23+ |
| Gin | v1.12+ |
| NestGo Core | v1.x |

## API Reference

Full documentation is available on [pkg.go.dev](https://pkg.go.dev/github.com/ashrafAli23/nestgo-gin-adapter).

### Exported Types

- **`GinServer`** — implements `core.Server`. Created via `New()`.
- **`GinRouter`** — implements `core.Router`. Created via `Group()`.
- **`GinContext`** — implements `core.Context`. Wraps `*gin.Context`.

### Exported Functions

- **`New(config *core.Config) core.Server`** — creates a new Gin-backed server.

## Related Packages

| Package | Description |
|---------|-------------|
| [nestgo](https://github.com/ashrafAli23/nestgo) | Core framework (interfaces, middleware, DI) |
| [nestgo-fiber-adapter](https://github.com/ashrafAli23/nestgo-fiber-adapter) | Fiber v3 adapter |
| [nestgo-validator](https://github.com/ashrafAli23/nestgo-validator) | Validation & transformation |

## License

[MIT](LICENSE)

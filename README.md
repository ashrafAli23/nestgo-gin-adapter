<h1 align="center">NestGo Gin Adapter</h1>

<p align="center"><strong>The official Gin web framework adapter for NestGo — bringing Gin's high performance to NestGo's enterprise-grade architectural patterns.</strong>
</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/ashrafAli23/nestgo-gin-adapter"><img src="https://pkg.go.dev/badge/github.com/ashrafAli23/nestgo-gin-adapter.svg" alt="NestGo Gin Adapter Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/ashrafAli23/nestgo-gin-adapter"><img src="https://goreportcard.com/badge/github.com/ashrafAli23/nestgo-gin-adapter" alt="NestGo Gin Adapter Go Report Card"></a>
  <a href="https://github.com/ashrafAli23/nestgo-gin-adapter/releases"><img src="https://img.shields.io/github/v/release/ashrafAli23/nestgo-gin-adapter?style=flat-square" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="MIT License"></a>
</p>

---

The **NestGo Gin Adapter** (`nestgo-gin-adapter`) seamlessly bridges the gap between the [Gin web framework](https://gin-gonic.com) and [NestGo](https://github.com/ashrafAli23/nestgo).

> **📚 Documentation:** <https://ashrafali23.github.io/nestgo/adapters.html>

By implementing NestGo's `core.Server`, `core.Router`, and `core.Context` interfaces, this adapter allows you to leverage NestGo's powerful Dependency Injection (DI), Guards, Interceptors, Pipes, and Middleware ecosystem while utilizing Gin's battle-tested, high-performance routing engine for building modern Go (Golang) REST APIs and microservices.

## 📦 Installation

```bash
go get github.com/ashrafAli23/nestgo-gin-adapter
```

**Prerequisites:**

You will also need the core framework and the Gin HTTP engine:

```bash
go get github.com/ashrafAli23/nestgo      # NestGo core framework
go get github.com/gin-gonic/gin           # Gin web framework
```

## 🚀 Quick Start: Building APIs with NestGo & Gin

```go
package main

import (
    "github.com/ashrafAli23/nestgo/core"
    gin "github.com/ashrafAli23/nestgo-gin-adapter"
    "github.com/ashrafAli23/nestgo/middleware"
)

func main() {
    // Initialize the Gin server adapter with default configuration
    server := gin.New(core.DefaultConfig())

    // NestGo middleware works out of the box with Gin
    server.Use(middleware.Recovery())
    server.Use(middleware.CORS())
    server.Use(middleware.RequestID())

    // Define a robust REST endpoint
    server.GET("/hello", func(c core.Context) error {
        return c.JSON(200, map[string]string{"message": "Hello from Gin and NestGo!"})
    })

    // Start the high-performance HTTP server
    server.Start(":3000")
}
```

## 🔄 Swapping from Fiber to Gin

NestGo's powerful adapter pattern means switching your HTTP engine from Fiber to Gin (or vice versa) is typically a one-line code change seamlessly supporting your entire API:

```diff
  import (
      "github.com/ashrafAli23/nestgo/core"
-     adapter "github.com/ashrafAli23/nestgo-fiber-adapter"
+     adapter "github.com/ashrafAli23/nestgo-gin-adapter"
  )

  func main() {
      server := adapter.New(core.DefaultConfig())
      // ... all your NestGo controllers, middleware, guards, and services remain identical
  }
```

## ✨ Key Features & Optimizations

### Context Pooling

Context allocations are tightly managed using standard `sync.Pool`. This achieves **zero allocation per request** for context structs. Each incoming HTTP request acquires a `GinContext` from the shared pool and reliably releases it immediately after the handler unrolls. As a safety net, every context method checks an `atomic.Bool` released flag — accidentally using a context after the handler returned panics with a clear use-after-release message (use `Clone()` for goroutines) instead of silently reading another request's data.

### Smart Body Caching

By design, Gin's `GetRawData()` permanently drains the request body stream. The adapter's `GinContext.Body()` elegantly buffers and securely caches the payload. Subsequent calls to `Body()` or `Bind()` directly reuse the cached data slice and restore the `io.ReadCloser` seamlessly for further downstream usage.

```go
server.POST("/echo", func(c core.Context) error {
    body, _ := c.Body()     // Reads once, automatically caches payload
    body2, _ := c.Body()    // Instantly returns ultra-fast cached copy
    _ = c.Bind(&myStruct)   // Fully operational — body reader is automatically restored

    return c.JSON(200, map[string]string{"received": string(body)})
})
```

### Goroutine Safe Contexts with Clone

Easily offload heavy workloads into background goroutines. Use `Clone()` to generate a fully untethered, safe context copy:

```go
server.GET("/async", func(c core.Context) error {
    cloned := c.Clone() // Goroutine-safe copy
    go func() {
        ip := cloned.ClientIP()   // 100% safe concurrent access
        method := cloned.Method() // 100% safe concurrent access
        _ = ip
        _ = method
    }()
    return c.JSON(202, map[string]string{"status": "accepted"})
})
```

`Clone()` produces a fully detached snapshot: the request body is pre-read and copied, context values and route params are duplicated via `gin.Context.Copy()`, and the request is detached with `context.WithoutCancel` so the clone's `RequestCtx()` keeps working after the original handler completes. Response methods on the clone succeed harmlessly but write nothing to the client.

### Advanced Route Groups

```go
api := server.Group("/api/v1")
api.GET("/users", listUsers)
api.POST("/users", createUser)

// Nested sub-groups with specialized NestGo middleware integration
admin := api.Group("/admin", middleware.RateLimit(middleware.RateLimitConfig{
    Max:    10,
    Window: time.Minute,
}))
admin.DELETE("/users/:id", deleteUser)
```

### Middleware Composition & Error Propagation

The full middleware chain (group chain outermost, then route middleware, then the handler) is composed into a **single native Gin handler per route** at registration time. Errors returned by a handler flow back through every middleware — NestGo exception filters and interceptors see them — and the configured error handler runs exactly once, only if nothing has been written to the response yet. Panics are recovered by the adapter into a logged 500 through the same error path.

Because composition happens at registration, middleware added via `Use()` **after** a route is registered does not apply to that route — register global middleware before your routes (NestGo's DI container already does this automatically).

### Buffered Responses & Real-Time Streaming

Each request writes into a single buffered response recorder that is flushed to the client exactly once — so interceptors can read `ResponseBody()`, mutate headers after the handler runs, and never double-write. `SendStream`, `SendFile`, and `Download` switch to direct streaming with a flush per chunk, which serves Server-Sent Events (SSE) in real time and keeps large downloads out of memory.

### Accessing Raw Gin Configurations & APIs

For specific scenarios where extending Gin requires bare-metal access not covered by NestGo constraints:

```go
// Direct access to the raw *gin.Engine instance
engine := server.Underlying().(*gin.Engine)
engine.SetTrustedProxies([]string{"192.168.1.0/24"})
engine.LoadHTMLGlob("templates/*")
engine.Static("/public", "./static")

// Direct access to the raw *gin.Context inside a dynamic handler
server.GET("/raw", func(c core.Context) error {
    gc := c.Underlying().(*gin.Context)
    // Execute niche Gin-specific APIs perfectly
    return c.JSON(200, nil)
})
```

### Debug vs Release Mode & Built-in Loggers

```go
// Debug mode — Gin logs all routes and requests
server := gin.New(&core.Config{Debug: true})

// Release mode (default) — no debug logging
server := gin.New(&core.Config{Debug: false})

// Disable Gin's default logger entirely (bring your own)
server := gin.New(&core.Config{DisableLogger: true})
```

### Graceful Server Shutdown Management

The wrapper natively embeds standard `net/http.Server`, offering pure Go-native graceful shutdown capabilities for your microservices:

```go
go func() {
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    server.Shutdown(ctx)
}()

server.Start(":3000") // Blocks until shutdown sequence executes
```

## ⚙️ Configuration Options

Instantiate with explicit `*core.Config` struct references via `New()`:

```go
server := gin.New(&core.Config{
    Addr:              ":8080",
    Debug:             false,
    DisableLogger:     false,
    ReadTimeout:       30,
    WriteTimeout:      30,
    ReadHeaderTimeout: 10,
    IdleTimeout:       60,
    MaxHeaderBytes:    1 << 20,
    BodyLimit:         10 * 1024 * 1024,
    ErrorHandler:      customErrorHandler,
})
```

| Field               | Type                | Default   | Description                                                                |
| ------------------- | ------------------- | --------- | -------------------------------------------------------------------------- |
| `Addr`              | `string`            | `":3000"` | TCP Network Addr and Port                                                  |
| `Debug`             | `bool`              | `false`   | Enable Gin Debug level logging (prints routing paths)                      |
| `DisableLogger`     | `bool`              | `false`   | Disables Gin's built-in global logger middleware                           |
| `ReadTimeout`       | `int`               | `0`       | Prevent large payload DOS - Reading timeout (seconds)                      |
| `WriteTimeout`      | `int`               | `0`       | HTTP Responding timeout limit (seconds)                                    |
| `ReadHeaderTimeout` | `int`               | `10`      | Header read timeout in seconds (Slowloris protection)                      |
| `IdleTimeout`       | `int`               | `60`      | Keep-alive idle connection timeout (seconds)                               |
| `MaxHeaderBytes`    | `int`               | `1 << 20` | Max request header size in bytes (1MB default)                             |
| `BodyLimit`         | `int`               | `0`       | Max request body size in bytes — enforced, oversized bodies get 413        |
| `ErrorHandler`      | `core.ErrorHandler` | `nil`     | Global exception catching interface defaults to `core.DefaultErrorHandler` |

> Note: `AppName` is currently not used by the Gin adapter (the Fiber adapter applies it to `fiber.Config.AppName`).

## ⚡ Performance Summary

| Architectural Optimization | Underlying Technique                   | Application Impact                                  |
| -------------------------- | -------------------------------------- | --------------------------------------------------- |
| **Context Memory Pooling** | Go stdlib `sync.Pool` + released flag  | Zero context-struct alloc, loud misuse panics       |
| **IO Body Caching**        | Single read cached per request         | Evades expensive double-read runtime allocations    |
| **Response Buffering**     | One shared recorder, flushed once      | Response introspection; direct streaming for SSE    |
| **Timeout Lifecycles**     | Standard HTTP standard library server  | Full `net/http` timeout + header-size protection    |
| **Middleware Composition** | One composed native handler per route  | Errors reach filters; error handler runs once       |

## 📌 Framework Compatibility

| Software Dependency     | Supported Version ranges |
| ----------------------- | ------------------------ |
| **Go (Golang)**         | `v1.25.14+`              |
| **Gin Gonic Framework** | `v1.12+`                 |
| **NestGo Core**         | `v1.x`                   |

## 📚 API Reference Navigation

For full programmatic documentation and code examples, automatically check our robust [pkg.go.dev Reference](https://pkg.go.dev/github.com/ashrafAli23/nestgo-gin-adapter).

### Core Exported Adapters

- **`GinServer`** — Instantiates NestGo's `core.Server`. Initalized via `New()`.
- **`GinRouter`** — Instantiates NestGo's `core.Router`. Extracted via `Group()`.
- **`GinContext`** — Instantiates NestGo's `core.Context` abstraction. Intelligently wrapping `*gin.Context`.

## 🌐 Related Ecosystem Packages

Supercharge your NestGo capabilities utilizing our wider integration ecosystem:

| Integration Package                                                             | Brief Description                                                                           |
| ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| [**nestgo**](https://github.com/ashrafAli23/nestgo)                             | The official core framework (dependency injection, metadata parsing, middleware interfaces) |
| [**nestgo-fiber-adapter**](https://github.com/ashrafAli23/nestgo-fiber-adapter) | Ultra-fast Fiber v3 high performance web server adapter                                     |
| [**nestgo-validator**](https://github.com/ashrafAli23/nestgo-validator)         | Powerful DTO input validation and dynamic struct transformations                            |

## 📄 Open Source License

This module runs freely available under the [MIT License](LICENSE).

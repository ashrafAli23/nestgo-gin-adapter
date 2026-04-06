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

Context allocations are tightly managed using standard `sync.Pool`. This achieves **zero allocation per request** for context structs. Each incoming HTTP request acquires a `GinContext` from the shared pool and reliably releases it immediately after the handler unrolls.

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

`Clone()` smartly delegates to `gin.Context.Copy()`, precisely duplicating the incoming HTTP request context values into an isolated struct.

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
    AppName:       "microservices-api",
    Addr:          ":8080",
    Debug:         false,
    DisableLogger: false,
    ReadTimeout:   30,
    WriteTimeout:  30,
    BodyLimit:     10 * 1024 * 1024,
    ErrorHandler:  customErrorHandler,
})
```

| Field           | Type                | Default   | Description                                                                |
| --------------- | ------------------- | --------- | -------------------------------------------------------------------------- |
| `AppName`       | `string`            | `""`      | Enterprise Application Identifier                                          |
| `Addr`          | `string`            | `":3000"` | TCP Network Addr and Port                                                  |
| `Debug`         | `bool`              | `false`   | Enable Gin Debug level logging (prints routing paths)                      |
| `DisableLogger` | `bool`              | `false`   | Disables Gin's built-in global logger middleware                           |
| `ReadTimeout`   | `int`               | `0`       | Prevent large payload DOS - Reading timeout (seconds)                      |
| `WriteTimeout`  | `int`               | `0`       | HTTP Responding timeout limit (seconds)                                    |
| `BodyLimit`     | `int`               | `0`       | Upper limit for HTTP JSON Body bytes (`Content-Length`)                    |
| `ErrorHandler`  | `core.ErrorHandler` | `nil`     | Global exception catching interface defaults to `core.DefaultErrorHandler` |

## ⚡ Performance Summary

| Architectural Optimization | Underlying Technique                   | Application Impact                                  |
| -------------------------- | -------------------------------------- | --------------------------------------------------- |
| **Context Memory Pooling** | Go stdlib `sync.Pool`                  | Guaranteed Zero allocation per concurrent request   |
| **IO Body Caching**        | Raw slice reuse through `append([:0])` | Evades expensive double-read runtime allocations    |
| **Timeout Lifecycles**     | Standard HTTP standard library server  | Native `net/http` timeouts against connection drops |
| **Middleware Composition** | Compile-time interface adaptation      | Seamless chaining parity with zero overhead         |

## 📌 Framework Compatibility

| Software Dependency     | Supported Version ranges |
| ----------------------- | ------------------------ |
| **Go (Golang)**         | `v1.23+`                 |
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

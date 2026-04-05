// Package ginadapter provides a [Gin] adapter for the NestGo framework.
//
// It implements [core.Server], [core.Router], and [core.Context] on top of
// [github.com/gin-gonic/gin], letting you use NestGo's Guards, Interceptors,
// Pipes, and Middleware ecosystem with Gin's battle-tested HTTP engine.
//
// # Install
//
//	go get github.com/ashrafAli23/nestgo-gin-adapter
//
// # Quick Start
//
//	package main
//
//	import (
//	    "github.com/ashrafAli23/nestgo/core"
//	    gin "github.com/ashrafAli23/nestgo-gin-adapter"
//	    "github.com/ashrafAli23/nestgo/middleware"
//	)
//
//	func main() {
//	    server := gin.New(core.DefaultConfig())
//
//	    server.Use(middleware.Recovery())
//	    server.Use(middleware.CORS())
//
//	    server.GET("/hello", func(c core.Context) error {
//	        return c.JSON(200, map[string]string{"message": "Hello from Gin!"})
//	    })
//
//	    server.Start(":3000")
//	}
//
// # Architecture
//
// This adapter bridges NestGo's zero-dep core interfaces to Gin:
//
//	┌──────────────────────┐       ┌───────────────────────────┐
//	│  core.Server         │──────▶│  GinServer                │
//	│  core.Router         │──────▶│  GinRouter                │
//	│  core.Context        │──────▶│  GinContext                │
//	└──────────────────────┘       └───────────────────────────┘
//
// Your handlers only import [core.Context]. The adapter translates every call
// to the underlying [gin.Context] — you never touch Gin APIs directly unless
// you choose to via [GinContext.Underlying].
//
// # Context Pooling
//
// Contexts are managed with [sync.Pool] to avoid allocation per request.
// Each request acquires a [GinContext] from the pool, and releases it back
// after the handler returns. This is transparent to the user.
//
// # Body Caching
//
// Gin's [gin.Context.GetRawData] drains the request body — calling it twice
// returns empty on the second call. [GinContext.Body] solves this by reading
// once and caching the result. Subsequent calls to [GinContext.Body] or
// [GinContext.Bind] reuse the cached data and restore the body reader.
//
// # Safe Goroutine Usage with Clone
//
// Use [GinContext.Clone] to create a goroutine-safe copy:
//
//	server.GET("/async", func(c core.Context) error {
//	    cloned := c.Clone()
//	    go func() {
//	        ip := cloned.ClientIP()   // safe
//	        method := cloned.Method() // safe
//	        _ = ip
//	        _ = method
//	    }()
//	    return c.JSON(202, map[string]string{"status": "accepted"})
//	})
//
// [Clone] delegates to [gin.Context.Copy], which copies the request and
// all context values into a new struct that is independent of the pool.
//
// # Route Groups
//
// Use [GinServer.Group] (or [GinRouter.Group]) to create prefixed sub-routers
// with their own middleware:
//
//	api := server.Group("/api/v1")
//	api.GET("/users", listUsers)
//	api.POST("/users", createUser)
//
// # Accessing the Raw Gin Engine
//
// For Gin-specific features (trusted proxies, HTML templates, static files):
//
//	engine := server.Underlying().(*gin.Engine)
//	engine.SetTrustedProxies([]string{"192.168.1.0/24"})
//	engine.LoadHTMLGlob("templates/*")
//
// Similarly, within a handler you can access the raw [gin.Context]:
//
//	server.GET("/raw", func(c core.Context) error {
//	    gc := c.Underlying().(*gin.Context)
//	    _ = gc // use Gin-specific APIs
//	    return c.JSON(200, nil)
//	})
//
// # Graceful Shutdown
//
// [GinServer] wraps Gin in a standard [net/http.Server], so shutdown uses
// Go's built-in [http.Server.Shutdown]:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//	server.Shutdown(ctx)
//
// # Performance Characteristics
//
//   - Context pooling via [sync.Pool] — zero allocation per request for context structs
//   - Body caching with slice reuse — second read reuses the existing byte slice
//   - Standard [net/http.Server] — full control over read/write timeouts
//
// [Gin]: https://gin-gonic.com
package ginadapter

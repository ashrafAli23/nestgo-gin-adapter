// Package ginadapter provides a [Gin] adapter for NestGo, the NestJS-style
// web framework for Go.
//
// Full documentation: https://ashrafali23.github.io/nestgo/adapters.html
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
// Middleware registered with Use/Group and route middleware are composed
// in-process into ONE native gin handler per route (group chain outermost).
// Errors returned by handlers therefore flow back through every middleware,
// so exception filters and interceptors observe them, and the configured
// error handler runs exactly once — only if nothing was written yet.
//
// # Response Buffering
//
// Responses are buffered (status, headers, body) and flushed to the client
// exactly once at the end of the request, so middleware may inspect
// [core.Context.ResponseBody] (which returns a copy of the buffered body),
// set headers after the handler ran, or replace the response via
// [core.ResponseResetter]. [core.Context.SendStream] switches to direct
// streaming: the status and headers are flushed to the wire before the first
// chunk is read (so SSE/EventSource clients that wait for headers never
// hang), and each chunk is flushed as it is written, so streamed responses
// are sent chunked. [core.Context.SendFile] and [core.Context.Download]
// likewise bypass the buffer. Once streamed, ResponseBody returns nil.
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
// [Clone] returns a detached snapshot: the request body is pre-read and
// copied, context values are copied, and the clone's request context is not
// canceled when the handler returns. Response calls on a clone succeed but
// write nowhere. Using the ORIGINAL context after the handler returns
// panics with a clear use-after-release message.
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

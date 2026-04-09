package ginadapter

import (
	"context"
	"net/http"
	"time"

	core "github.com/ashrafAli23/nestgo/core"
	"github.com/gin-gonic/gin"
)

var _ core.Server = (*GinServer)(nil)
var _ core.Router = (*GinRouter)(nil)

// ═══════════════════════════════════════════════════════════════════════════
// GinServer
// ═══════════════════════════════════════════════════════════════════════════

type GinServer struct {
	engine     *gin.Engine
	httpServer *http.Server
	config     *core.Config
	router     *GinRouter
}

// New creates a new Gin-backed core.Server.
func New(config *core.Config) core.Server {
	if config == nil {
		config = core.DefaultConfig()
	}
	if config.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	var engine *gin.Engine
	if config.DisableLogger {
		engine = gin.New()
		engine.Use(gin.Recovery())
	} else {
		engine = gin.Default()
	}

	s := &GinServer{engine: engine, config: config}
	s.router = &GinRouter{group: &engine.RouterGroup, errHandler: config.ErrorHandler}
	return s
}

func (s *GinServer) Start(addr string) error {
	if addr == "" {
		addr = s.config.Addr
	}
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.engine,
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
	}
	core.Log().Info("starting server", core.F("adapter", "gin"), core.F("addr", addr))
	return s.httpServer.ListenAndServe()
}

func (s *GinServer) StartTLS(addr, certFile, keyFile string) error {
	if addr == "" {
		addr = s.config.Addr
	}
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.engine,
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
	}
	core.Log().Info("starting TLS server", core.F("adapter", "gin"), core.F("addr", addr))
	return s.httpServer.ListenAndServeTLS(certFile, keyFile)
}

func (s *GinServer) Shutdown(ctx context.Context) error {
	core.Log().Info("shutting down server", core.F("adapter", "gin"))
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *GinServer) Name() string            { return "gin" }
func (s *GinServer) Underlying() interface{} { return s.engine }

// Router delegation
func (s *GinServer) GET(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	s.router.GET(p, h, m...)
}
func (s *GinServer) POST(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	s.router.POST(p, h, m...)
}
func (s *GinServer) PUT(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	s.router.PUT(p, h, m...)
}
func (s *GinServer) DELETE(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	s.router.DELETE(p, h, m...)
}
func (s *GinServer) PATCH(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	s.router.PATCH(p, h, m...)
}
func (s *GinServer) OPTIONS(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	s.router.OPTIONS(p, h, m...)
}
func (s *GinServer) HEAD(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	s.router.HEAD(p, h, m...)
}
func (s *GinServer) ANY(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	s.router.ANY(p, h, m...)
}
func (s *GinServer) Group(prefix string, m ...core.MiddlewareFunc) core.Router {
	return s.router.Group(prefix, m...)
}
func (s *GinServer) Use(m ...core.MiddlewareFunc) { s.router.Use(m...) }
func (s *GinServer) Static(path string, root string, m ...core.MiddlewareFunc) {
	s.router.Static(path, root, m...)
}
func (s *GinServer) StaticFile(path string, filePath string, m ...core.MiddlewareFunc) {
	s.router.StaticFile(path, filePath, m...)
}

// ═══════════════════════════════════════════════════════════════════════════
// GinRouter
// ═══════════════════════════════════════════════════════════════════════════

type GinRouter struct {
	group      *gin.RouterGroup
	errHandler core.ErrorHandler
}

func (r *GinRouter) GET(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.GET(p, wrapHandler(applyRouteMiddleware(h, m), r.errHandler))
}
func (r *GinRouter) POST(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.POST(p, wrapHandler(applyRouteMiddleware(h, m), r.errHandler))
}
func (r *GinRouter) PUT(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.PUT(p, wrapHandler(applyRouteMiddleware(h, m), r.errHandler))
}
func (r *GinRouter) DELETE(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.DELETE(p, wrapHandler(applyRouteMiddleware(h, m), r.errHandler))
}
func (r *GinRouter) PATCH(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.PATCH(p, wrapHandler(applyRouteMiddleware(h, m), r.errHandler))
}
func (r *GinRouter) OPTIONS(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.OPTIONS(p, wrapHandler(applyRouteMiddleware(h, m), r.errHandler))
}
func (r *GinRouter) HEAD(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.HEAD(p, wrapHandler(applyRouteMiddleware(h, m), r.errHandler))
}
func (r *GinRouter) ANY(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.Any(p, wrapHandler(applyRouteMiddleware(h, m), r.errHandler))
}

func (r *GinRouter) Group(prefix string, mw ...core.MiddlewareFunc) core.Router {
	g := r.group.Group(prefix)
	for _, m := range mw {
		g.Use(wrapMiddleware(m, r.errHandler))
	}
	return &GinRouter{group: g, errHandler: r.errHandler}
}

func (r *GinRouter) Use(mw ...core.MiddlewareFunc) {
	for _, m := range mw {
		r.group.Use(wrapMiddleware(m, r.errHandler))
	}
}

func (r *GinRouter) Static(path string, root string, mw ...core.MiddlewareFunc) {
	if len(mw) > 0 {
		g := r.group.Group(path)
		for _, m := range mw {
			g.Use(wrapMiddleware(m, r.errHandler))
		}
		g.Static("", root)
	} else {
		r.group.Static(path, root)
	}
}

func (r *GinRouter) StaticFile(path string, filePath string, mw ...core.MiddlewareFunc) {
	if len(mw) > 0 {
		g := r.group.Group(path)
		for _, m := range mw {
			g.Use(wrapMiddleware(m, r.errHandler))
		}
		g.StaticFile("", filePath)
	} else {
		r.group.StaticFile(path, filePath)
	}
}

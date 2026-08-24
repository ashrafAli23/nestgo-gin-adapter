package ginadapter

import (
	"context"
	"net/http"
	"sync"
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
	engine *gin.Engine
	config *core.Config
	router *GinRouter

	mu         sync.Mutex // guards httpServer and shutdown
	httpServer *http.Server
	shutdown   bool
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

// rootHandler wraps the gin engine so Config.BodyLimit is enforced with
// http.MaxBytesReader on every request body (oversized bodies surface as a
// 413 via mapBodyLimitErr in Body/Bind).
func (s *GinServer) rootHandler() http.Handler {
	if s.config.BodyLimit <= 0 {
		return s.engine
	}
	limit := int64(s.config.BodyLimit)
	engine := s.engine
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		engine.ServeHTTP(w, r)
	})
}

// prepare constructs the http.Server under the mutex. It fails with
// http.ErrServerClosed when Shutdown has already been requested, so an early
// shutdown never leaks a listener.
func (s *GinServer) prepare(addr string) (*http.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shutdown {
		return nil, http.ErrServerClosed
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.rootHandler(),
		ReadTimeout:       time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(s.config.WriteTimeout) * time.Second,
		ReadHeaderTimeout: time.Duration(s.config.ReadHeaderTimeout) * time.Second,
		IdleTimeout:       time.Duration(s.config.IdleTimeout) * time.Second,
		MaxHeaderBytes:    s.config.MaxHeaderBytes,
	}
	s.httpServer = srv
	return srv, nil
}

func (s *GinServer) Start(addr string) error {
	if addr == "" {
		addr = s.config.Addr
	}
	srv, err := s.prepare(addr)
	if err != nil {
		return err
	}
	core.Log().Info("starting server", core.F("adapter", "gin"), core.F("addr", addr))
	return srv.ListenAndServe()
}

func (s *GinServer) StartTLS(addr, certFile, keyFile string) error {
	if addr == "" {
		addr = s.config.Addr
	}
	srv, err := s.prepare(addr)
	if err != nil {
		return err
	}
	core.Log().Info("starting TLS server", core.F("adapter", "gin"), core.F("addr", addr))
	return srv.ListenAndServeTLS(certFile, keyFile)
}

func (s *GinServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.shutdown = true
	srv := s.httpServer
	s.mu.Unlock()
	core.Log().Info("shutting down server", core.F("adapter", "gin"))
	if srv != nil {
		return srv.Shutdown(ctx)
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

// GinRouter accumulates its core middleware chain in mws (inheriting the
// parent group's chain). Use() APPENDS to the slice instead of registering a
// native per-middleware wrapper; at route registration the whole chain is
// composed in-process — group chain outermost, then route middleware — and
// registered as ONE native handler per route (see wrapHandler). This lets
// handler errors flow back through every middleware (filters/interceptors
// see them) and eliminates double-written responses.
//
// Like gin itself, middleware added via Use() only applies to routes
// registered afterwards.
type GinRouter struct {
	group      *gin.RouterGroup
	errHandler core.ErrorHandler
	mws        []core.MiddlewareFunc
}

// compose builds the single native handler for a route: group chain
// outermost, then route middleware, then the handler.
func (r *GinRouter) compose(h core.HandlerFunc, m []core.MiddlewareFunc) gin.HandlerFunc {
	h = applyRouteMiddleware(h, m)
	h = applyRouteMiddleware(h, r.mws)
	return wrapHandler(h, r.errHandler)
}

// chainWith returns a fresh slice of the group chain plus extra middleware.
func (r *GinRouter) chainWith(mw []core.MiddlewareFunc) []core.MiddlewareFunc {
	chain := make([]core.MiddlewareFunc, 0, len(r.mws)+len(mw))
	chain = append(chain, r.mws...)
	chain = append(chain, mw...)
	return chain
}

func (r *GinRouter) GET(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.GET(p, r.compose(h, m))
}
func (r *GinRouter) POST(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.POST(p, r.compose(h, m))
}
func (r *GinRouter) PUT(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.PUT(p, r.compose(h, m))
}
func (r *GinRouter) DELETE(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.DELETE(p, r.compose(h, m))
}
func (r *GinRouter) PATCH(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.PATCH(p, r.compose(h, m))
}
func (r *GinRouter) OPTIONS(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.OPTIONS(p, r.compose(h, m))
}
func (r *GinRouter) HEAD(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.HEAD(p, r.compose(h, m))
}
func (r *GinRouter) ANY(p string, h core.HandlerFunc, m ...core.MiddlewareFunc) {
	r.group.Any(p, r.compose(h, m))
}

func (r *GinRouter) Group(prefix string, mw ...core.MiddlewareFunc) core.Router {
	return &GinRouter{
		group:      r.group.Group(prefix),
		errHandler: r.errHandler,
		mws:        r.chainWith(mw),
	}
}

// Use appends middleware to this router's chain. It applies to routes
// registered after the call (matching gin's own Use semantics).
func (r *GinRouter) Use(mw ...core.MiddlewareFunc) {
	r.mws = append(r.mws, mw...)
}

func (r *GinRouter) Static(path string, root string, mw ...core.MiddlewareFunc) {
	chain := r.chainWith(mw)
	if len(chain) == 0 {
		r.group.Static(path, root)
		return
	}
	g := r.group.Group(path)
	g.Use(wrapNativeChain(chain, r.errHandler))
	g.Static("", root)
}

func (r *GinRouter) StaticFile(path string, filePath string, mw ...core.MiddlewareFunc) {
	chain := r.chainWith(mw)
	if len(chain) == 0 {
		r.group.StaticFile(path, filePath)
		return
	}
	g := r.group.Group(path)
	g.Use(wrapNativeChain(chain, r.errHandler))
	g.StaticFile("", filePath)
}

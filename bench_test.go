package ginadapter_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	ginadapter "github.com/ashrafAli23/nestgo-gin-adapter"
	core "github.com/ashrafAli23/nestgo/core"
	"github.com/ashrafAli23/nestgo/middleware"
	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.ReleaseMode) }

// benchServe measures one in-process request per iteration (no network).
func benchServe(b *testing.B, h http.Handler, path string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("status %d", w.Code)
		}
	}
}

func rawGin() http.Handler {
	e := gin.New()
	e.GET("/hello", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return e
}

func nestGoGin() http.Handler {
	cfg := core.DefaultConfig()
	cfg.DisableLogger = true
	s := ginadapter.New(cfg)
	s.GET("/hello", func(c core.Context) error { return c.JSON(200, map[string]bool{"ok": true}) })
	return s.Underlying().(*gin.Engine)
}

// Middleware3 = recovery + request-id header + an allow-all guard on both sides.
func rawGinMiddleware3() http.Handler {
	e := gin.New()
	e.Use(gin.Recovery())
	e.Use(func(c *gin.Context) { c.Header("X-Request-ID", "bench"); c.Next() })
	e.Use(func(c *gin.Context) { c.Next() }) // allow-all "guard"
	e.GET("/hello", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return e
}

func nestGoGinMiddleware3() http.Handler {
	cfg := core.DefaultConfig()
	cfg.DisableLogger = true
	s := ginadapter.New(cfg)
	s.Use(middleware.Recovery(), middleware.RequestID())
	allow := core.GuardFunc(func(core.Context) (bool, error) { return true, nil })
	s.GET("/hello", func(c core.Context) error {
		return c.JSON(200, map[string]bool{"ok": true})
	}, core.UseGuards(allow))
	return s.Underlying().(*gin.Engine)
}

func BenchmarkRawGin_HelloJSON(b *testing.B)      { benchServe(b, rawGin(), "/hello") }
func BenchmarkNestGoGin_HelloJSON(b *testing.B)   { benchServe(b, nestGoGin(), "/hello") }
func BenchmarkRawGin_Middleware3(b *testing.B)    { benchServe(b, rawGinMiddleware3(), "/hello") }
func BenchmarkNestGoGin_Middleware3(b *testing.B) { benchServe(b, nestGoGinMiddleware3(), "/hello") }

package ginadapter_test

import (
	"crypto/rand"
	"encoding/hex"
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

// Both sides return the same map[string]bool payload so JSON encoding does
// identical work; gin.H (map[string]any) costs more per encode and would
// make "NestGo allocates less" an artifact of the raw handler's own payload
// type rather than of adapter overhead.
func rawGin() http.Handler {
	e := gin.New()
	e.GET("/hello", func(c *gin.Context) { c.JSON(http.StatusOK, map[string]bool{"ok": true}) })
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
//
// Recovery: ginadapter.New installs gin.Recovery() unconditionally (see
// server.go), so nestGoGinMiddleware3 below does not add a second one via
// s.Use — that would double-count recovery only on the NestGo side. The raw
// side installs its own gin.Recovery() here so both paths run exactly one
// recovery layer.
//
// Request-ID: both sides do equivalent real work — generate 16 random bytes
// and hex-encode them into the header — instead of the raw side writing a
// constant string, which would undercount its cost relative to NestGo's
// middleware.RequestID().
func rawGinMiddleware3() http.Handler {
	e := gin.New()
	e.Use(gin.Recovery())
	e.Use(func(c *gin.Context) {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		c.Header("X-Request-ID", hex.EncodeToString(b))
		c.Next()
	})
	e.Use(func(c *gin.Context) { c.Next() }) // allow-all "guard"
	e.GET("/hello", func(c *gin.Context) { c.JSON(http.StatusOK, map[string]bool{"ok": true}) })
	return e
}

func nestGoGinMiddleware3() http.Handler {
	cfg := core.DefaultConfig()
	cfg.DisableLogger = true
	s := ginadapter.New(cfg) // already installs gin.Recovery()
	s.Use(middleware.RequestID())
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

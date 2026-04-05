package ginadapter

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"strings"
	"sync"

	core "github.com/ashrafAli23/nestgo/core"
	"github.com/gin-gonic/gin"
)

var _ core.Context = (*GinContext)(nil)

var contextPool = sync.Pool{
	New: func() interface{} { return &GinContext{} },
}

func acquireContext(gc *gin.Context) *GinContext {
	ctx := contextPool.Get().(*GinContext)
	ctx.ginCtx = gc
	return ctx
}

func releaseContext(ctx *GinContext) {
	ctx.ginCtx = nil
	ctx.bodyRead = false
	ctx.bodyData = nil
	ctx.bodyErr = nil
	contextPool.Put(ctx)
}

// GinContext wraps gin.Context to implement core.Context.
type GinContext struct {
	ginCtx   *gin.Context
	bodyRead bool
	bodyData []byte
	bodyErr  error
}

// ─── Request ────────────────────────────────────────────────────────────────

func (c *GinContext) Method() string                      { return c.ginCtx.Request.Method }
func (c *GinContext) Path() string                        { return c.ginCtx.FullPath() }
func (c *GinContext) Param(key string) string             { return c.ginCtx.Param(key) }
func (c *GinContext) Query(key string) string             { return c.ginCtx.Query(key) }
func (c *GinContext) QueryDefault(key, def string) string { return c.ginCtx.DefaultQuery(key, def) }
func (c *GinContext) GetHeader(key string) string         { return c.ginCtx.GetHeader(key) }

func (c *GinContext) Cookie(name string) string {
	val, err := c.ginCtx.Cookie(name)
	if err != nil {
		return ""
	}
	return val
}

// Body reads and caches the request body. Safe to call multiple times.
// Gin's GetRawData() drains the body — without caching, the second call
// returns empty. This fixes the body-read-once problem.
// Uses a pooled byte slice to reduce allocations.
func (c *GinContext) Body() ([]byte, error) {
	if !c.bodyRead {
		raw, err := c.ginCtx.GetRawData()
		c.bodyErr = err
		c.bodyRead = true
		if err == nil {
			// Get a pooled buffer or reuse existing bodyData slice
			if c.bodyData != nil {
				c.bodyData = append(c.bodyData[:0], raw...)
			} else {
				c.bodyData = raw
			}
			// Restore the body so ShouldBind still works after Body()
			c.ginCtx.Request.Body = io.NopCloser(bytes.NewReader(c.bodyData))
		}
	}
	return c.bodyData, c.bodyErr
}

// Bind parses the request body into the given struct.
// If Body() was called first, the body is restored so Bind still works.
func (c *GinContext) Bind(v interface{}) error {
	if c.bodyRead && c.bodyErr == nil {
		// Body was already read — restore it for ShouldBind
		c.ginCtx.Request.Body = io.NopCloser(bytes.NewReader(c.bodyData))
	}
	return c.ginCtx.ShouldBind(v)
}
func (c *GinContext) FormValue(key string) string { return c.ginCtx.PostForm(key) }
func (c *GinContext) ContentType() string         { return c.ginCtx.ContentType() }

func (c *GinContext) FormFile(key string) (*multipart.FileHeader, error) {
	return c.ginCtx.FormFile(key)
}

func (c *GinContext) IsWebSocket() bool {
	return strings.EqualFold(c.ginCtx.GetHeader("Upgrade"), "websocket")
}

// ─── Response ───────────────────────────────────────────────────────────────

func (c *GinContext) Status(code int) core.Context { c.ginCtx.Status(code); return c }

func (c *GinContext) JSON(status int, data interface{}) error {
	c.ginCtx.JSON(status, data)
	return nil
}

func (c *GinContext) XML(status int, data interface{}) error {
	c.ginCtx.XML(status, data)
	return nil
}

func (c *GinContext) String(status int, format string, vals ...interface{}) error {
	c.ginCtx.String(status, format, vals...)
	return nil
}

func (c *GinContext) SendBytes(status int, data []byte) error {
	c.ginCtx.Data(status, "application/octet-stream", data)
	return nil
}

func (c *GinContext) SendStream(stream io.Reader) error {
	c.ginCtx.Stream(func(w io.Writer) bool {
		_, err := io.Copy(w, stream)
		return err == nil
	})
	return nil
}

func (c *GinContext) NoContent(status int) error { c.ginCtx.Status(status); return nil }
func (c *GinContext) SetHeader(k, v string)      { c.ginCtx.Header(k, v) }

func (c *GinContext) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	c.ginCtx.SetCookie(name, value, maxAge, path, domain, secure, httpOnly)
}

func (c *GinContext) Redirect(status int, url string) error {
	c.ginCtx.Redirect(status, url)
	return nil
}

// ─── Metadata ───────────────────────────────────────────────────────────────

func (c *GinContext) ClientIP() string { return c.ginCtx.ClientIP() }

func (c *GinContext) FullURL() string {
	r := c.ginCtx.Request
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)
}

// ─── Context Storage ────────────────────────────────────────────────────────

func (c *GinContext) Set(key string, value interface{})  { c.ginCtx.Set(key, value) }
func (c *GinContext) Get(key string) (interface{}, bool) { return c.ginCtx.Get(key) }

// ─── Flow Control ───────────────────────────────────────────────────────────

func (c *GinContext) Next() error             { c.ginCtx.Next(); return nil }
func (c *GinContext) Underlying() interface{} { return c.ginCtx }

// Clone returns a copy of GinContext that is safe to use in goroutines.
// Uses gin.Context.Copy() which copies the request and context values.
func (c *GinContext) Clone() core.Context {
	return &GinContext{ginCtx: c.ginCtx.Copy()}
}

// ─── Internal helpers ───────────────────────────────────────────────────────

func wrapHandler(handler core.HandlerFunc, errHandler core.ErrorHandler) gin.HandlerFunc {
	return func(gc *gin.Context) {
		ctx := acquireContext(gc)
		defer releaseContext(ctx)
		if err := handler(ctx); err != nil {
			if errHandler != nil {
				errHandler(ctx, err)
			} else {
				core.DefaultErrorHandler(ctx, err)
			}
			gc.Abort()
		}
	}
}

func wrapMiddleware(mw core.MiddlewareFunc, errHandler core.ErrorHandler) gin.HandlerFunc {
	return func(gc *gin.Context) {
		ctx := acquireContext(gc)
		defer releaseContext(ctx)
		next := func(c core.Context) error { gc.Next(); return nil }
		handler := mw(next)
		if err := handler(ctx); err != nil {
			if errHandler != nil {
				errHandler(ctx, err)
			} else {
				core.DefaultErrorHandler(ctx, err)
			}
			gc.Abort()
		}
	}
}

func applyRouteMiddleware(handler core.HandlerFunc, mws []core.MiddlewareFunc) core.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}

package ginadapter

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"

	core "github.com/ashrafAli23/nestgo/core"
	"github.com/gin-gonic/gin"
)

var _ core.Context = (*GinContext)(nil)
var _ core.ResponseResetter = (*GinContext)(nil)
var _ core.ResponseHeaderReader = (*GinContext)(nil)
var _ core.SameSiteCookieSetter = (*GinContext)(nil)

// maxRetainedBufferCap caps the response-buffer capacity retained by pooled
// contexts so one huge response does not pin memory in the pool forever.
const maxRetainedBufferCap = 64 << 10 // 64KB

// streamCopyBufSize is the chunk size used by SendStream.
const streamCopyBufSize = 32 << 10 // 32KB

var contextPool = sync.Pool{
	New: func() interface{} { return &GinContext{} },
}

func acquireContext(gc *gin.Context) *GinContext {
	ctx := contextPool.Get().(*GinContext)
	ctx.released.Store(false)
	ctx.ginCtx = gc
	if rec, ok := gc.Writer.(*responseRecorder); ok {
		// A recorder is already installed for this request — share it
		// instead of nesting another copy of the response.
		ctx.recorder = rec
		ctx.installedRecorder = false
	} else {
		ctx.originalWriter = gc.Writer
		ctx.rec.reset(gc.Writer)
		gc.Writer = &ctx.rec
		ctx.recorder = &ctx.rec
		ctx.installedRecorder = true
	}
	return ctx
}

func releaseContext(ctx *GinContext) {
	ctx.released.Store(true)
	if ctx.installedRecorder && ctx.ginCtx != nil && ctx.originalWriter != nil {
		ctx.ginCtx.Writer = ctx.originalWriter
	}
	ctx.ginCtx = nil
	ctx.recorder = nil
	ctx.originalWriter = nil
	ctx.installedRecorder = false
	ctx.bodyRead = false
	ctx.bodyData = nil
	ctx.bodyErr = nil
	// Do not retain very large response buffers in the pool.
	if ctx.rec.body.Cap() > maxRetainedBufferCap {
		ctx.rec.body = bytes.Buffer{}
	}
	ctx.rec.ResponseWriter = nil
	contextPool.Put(ctx)
}

// ─── Response recorder ──────────────────────────────────────────────────────

// responseRecorder wraps gin.ResponseWriter and buffers status and body.
// Exactly one recorder is installed per request; the single native handler
// flushes it to the client once at the end of the request. SendStream and
// SendFile switch the recorder to direct streaming mode, after which writes
// bypass the buffer entirely.
type responseRecorder struct {
	gin.ResponseWriter // the real (underlying) writer
	status             int
	body               bytes.Buffer
	streamed           bool // direct streaming engaged — buffer bypassed
	done               bool // final flush performed
}

func (r *responseRecorder) reset(w gin.ResponseWriter) {
	r.ResponseWriter = w
	r.status = 0
	r.body.Reset()
	r.streamed = false
	r.done = false
}

func (r *responseRecorder) passthrough() bool { return r.streamed || r.done }

func (r *responseRecorder) WriteHeader(code int) {
	if r.passthrough() {
		r.ResponseWriter.WriteHeader(code)
		return
	}
	if code > 0 {
		r.status = code
	}
}

func (r *responseRecorder) WriteHeaderNow() {
	if r.passthrough() {
		r.ResponseWriter.WriteHeaderNow()
	}
	// Buffered mode: deferred until the final flush.
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.passthrough() {
		return r.ResponseWriter.Write(b)
	}
	return r.body.Write(b)
}

func (r *responseRecorder) WriteString(s string) (int, error) {
	if r.passthrough() {
		return r.ResponseWriter.WriteString(s)
	}
	return r.body.WriteString(s)
}

func (r *responseRecorder) Status() int {
	if r.passthrough() {
		return r.ResponseWriter.Status()
	}
	if r.status != 0 {
		return r.status
	}
	return http.StatusOK
}

func (r *responseRecorder) Size() int {
	if r.passthrough() {
		return r.ResponseWriter.Size()
	}
	return r.body.Len()
}

func (r *responseRecorder) Written() bool {
	if r.passthrough() {
		return r.ResponseWriter.Written()
	}
	return r.status != 0 || r.body.Len() > 0
}

// Flush switches to direct streaming (the buffered bytes are written out
// first) and flushes the underlying writer.
func (r *responseRecorder) Flush() {
	r.startStreaming()
	r.ResponseWriter.Flush()
}

// written reports whether any part of a response has been produced —
// used to guard against double-writing an error response.
func (r *responseRecorder) written() bool {
	return r.streamed || r.done || r.status != 0 || r.body.Len() > 0
}

// startStreaming engages direct streaming mode: the recorded status and any
// buffered body are handed to the underlying writer and further writes
// bypass the buffer.
func (r *responseRecorder) startStreaming() {
	if r.passthrough() {
		return
	}
	r.streamed = true
	if r.status != 0 {
		r.ResponseWriter.WriteHeader(r.status)
	}
	if r.body.Len() > 0 {
		_, _ = r.ResponseWriter.Write(r.body.Bytes())
		r.body.Reset()
	}
}

// flushToClient writes the buffered response to the client exactly once.
// No-op if the response was already streamed.
func (r *responseRecorder) flushToClient() {
	if r.passthrough() {
		return
	}
	r.done = true
	status := r.status
	if status == 0 {
		status = http.StatusOK
	}
	r.ResponseWriter.WriteHeader(status)
	if r.body.Len() > 0 {
		_, _ = r.ResponseWriter.Write(r.body.Bytes())
	} else {
		r.ResponseWriter.WriteHeaderNow()
	}
}

// discardWriter is a gin.ResponseWriter for cloned contexts: response calls
// on a Clone() succeed but go nowhere (the real response belongs to the
// original request).
type discardWriter struct {
	header http.Header
	status int
	size   int
}

func (d *discardWriter) Header() http.Header { return d.header }
func (d *discardWriter) Write(b []byte) (int, error) {
	d.size += len(b)
	return len(b), nil
}
func (d *discardWriter) WriteString(s string) (int, error) {
	d.size += len(s)
	return len(s), nil
}
func (d *discardWriter) WriteHeader(code int) {
	if code > 0 {
		d.status = code
	}
}
func (d *discardWriter) WriteHeaderNow() {}
func (d *discardWriter) Status() int {
	if d.status == 0 {
		return http.StatusOK
	}
	return d.status
}
func (d *discardWriter) Size() int     { return d.size }
func (d *discardWriter) Written() bool { return d.status != 0 || d.size > 0 }
func (d *discardWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}
func (d *discardWriter) Flush()                   {}
func (d *discardWriter) CloseNotify() <-chan bool { return make(chan bool) }
func (d *discardWriter) Pusher() http.Pusher      { return nil }

// GinContext wraps gin.Context to implement core.Context.
type GinContext struct {
	ginCtx            *gin.Context
	recorder          *responseRecorder
	rec               responseRecorder // pooled recorder storage
	originalWriter    gin.ResponseWriter
	installedRecorder bool
	bodyRead          bool
	bodyData          []byte
	bodyErr           error
	released          atomic.Bool
}

// checkReleased panics with a clear message if the context is used after
// release. Uses atomic.Bool for data-race-free checks without mutex overhead.
func (c *GinContext) checkReleased() {
	if c.released.Load() {
		panic("[NestGo] use-after-release: GinContext used after handler returned. " +
			"Gin contexts are pooled and recycled. Use c.Clone() before passing to goroutines.")
	}
}

// ─── Request ────────────────────────────────────────────────────────────────

func (c *GinContext) Method() string          { c.checkReleased(); return c.ginCtx.Request.Method }
func (c *GinContext) Path() string            { c.checkReleased(); return c.ginCtx.FullPath() }
func (c *GinContext) Param(key string) string { c.checkReleased(); return c.ginCtx.Param(key) }
func (c *GinContext) Query(key string) string { c.checkReleased(); return c.ginCtx.Query(key) }

// QueryDefault returns the default ONLY when the key is absent; a
// present-but-empty parameter returns "".
func (c *GinContext) QueryDefault(key, def string) string {
	c.checkReleased()
	if val, ok := c.ginCtx.GetQuery(key); ok {
		return val
	}
	return def
}
func (c *GinContext) GetHeader(key string) string { c.checkReleased(); return c.ginCtx.GetHeader(key) }

func (c *GinContext) Cookie(name string) string {
	c.checkReleased()
	val, err := c.ginCtx.Cookie(name)
	if err != nil {
		return ""
	}
	return val
}

// mapBodyLimitErr converts net/http's MaxBytesError (raised when
// Config.BodyLimit is exceeded) into a 413 HTTPError.
func mapBodyLimitErr(err error) error {
	if err == nil {
		return nil
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return core.NewHTTPError(http.StatusRequestEntityTooLarge, "request body too large")
	}
	return err
}

// Body reads and caches the request body. Safe to call multiple times.
// Gin's GetRawData() drains the body — without caching, the second call
// returns empty. This fixes the body-read-once problem.
func (c *GinContext) Body() ([]byte, error) {
	c.checkReleased()
	if !c.bodyRead {
		raw, err := c.ginCtx.GetRawData()
		c.bodyErr = mapBodyLimitErr(err)
		c.bodyRead = true
		if err == nil {
			c.bodyData = raw
			// Restore the body so ShouldBind still works after Body()
			c.ginCtx.Request.Body = io.NopCloser(bytes.NewReader(c.bodyData))
		}
	}
	return c.bodyData, c.bodyErr
}

// Bind parses the request body into the given struct.
// If Body() was called first, the body is restored so Bind still works.
func (c *GinContext) Bind(v interface{}) error {
	c.checkReleased()
	if c.bodyRead && c.bodyErr == nil {
		// Body was already read — restore it for ShouldBind
		c.ginCtx.Request.Body = io.NopCloser(bytes.NewReader(c.bodyData))
	}
	return mapBodyLimitErr(c.ginCtx.ShouldBind(v))
}
func (c *GinContext) FormValue(key string) string { c.checkReleased(); return c.ginCtx.PostForm(key) }
func (c *GinContext) ContentType() string         { c.checkReleased(); return c.ginCtx.ContentType() }

func (c *GinContext) FormFile(key string) (*multipart.FileHeader, error) {
	c.checkReleased()
	return c.ginCtx.FormFile(key)
}

func (c *GinContext) IsWebSocket() bool {
	c.checkReleased()
	return strings.EqualFold(c.ginCtx.GetHeader("Upgrade"), "websocket")
}

// ─── Response ───────────────────────────────────────────────────────────────

func (c *GinContext) Status(code int) core.Context {
	c.checkReleased()
	c.ginCtx.Status(code)
	return c
}

func (c *GinContext) JSON(status int, data interface{}) error {
	c.checkReleased()
	c.ginCtx.JSON(status, data)
	return nil
}

func (c *GinContext) XML(status int, data interface{}) error {
	c.checkReleased()
	c.ginCtx.XML(status, data)
	return nil
}

// String writes format verbatim when no values are given (a literal %
// survives); otherwise it formats via Sprintf.
func (c *GinContext) String(status int, format string, vals ...interface{}) error {
	c.checkReleased()
	if len(vals) == 0 {
		c.ginCtx.Data(status, "text/plain; charset=utf-8", []byte(format))
		return nil
	}
	c.ginCtx.String(status, format, vals...)
	return nil
}

// SendBytes sends raw bytes. When no Content-Type has been set it defaults
// to application/octet-stream (gin only applies it when the header is unset).
func (c *GinContext) SendBytes(status int, data []byte) error {
	c.checkReleased()
	c.ginCtx.Data(status, "application/octet-stream", data)
	return nil
}

// SendStream streams the reader to the client in direct streaming mode:
// buffered headers/status are written first, then each chunk is flushed as
// it is copied (which also serves SSE). The copy stops on EOF.
func (c *GinContext) SendStream(stream io.Reader) error {
	c.checkReleased()
	rec := c.recorder
	if rec == nil {
		_, err := io.Copy(c.ginCtx.Writer, stream)
		return err
	}
	rec.startStreaming()
	// Send headers before the first chunk — SSE clients wait on them.
	rec.ResponseWriter.WriteHeaderNow()
	buf := make([]byte, streamCopyBufSize)
	for {
		n, rerr := stream.Read(buf)
		if n > 0 {
			if _, werr := rec.Write(buf[:n]); werr != nil {
				return werr
			}
			rec.Flush()
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// SendFile streams the file directly to the client (bypassing the buffer).
func (c *GinContext) SendFile(filePath string) error {
	c.checkReleased()
	if c.recorder != nil {
		c.recorder.startStreaming()
	}
	c.ginCtx.File(filePath)
	return nil
}

// Download streams the file as an attachment (bypassing the buffer).
func (c *GinContext) Download(filePath string, filename string) error {
	c.checkReleased()
	if c.recorder != nil {
		c.recorder.startStreaming()
	}
	c.ginCtx.FileAttachment(filePath, filename)
	return nil
}

func (c *GinContext) NoContent(status int) error {
	c.checkReleased()
	c.ginCtx.Status(status)
	return nil
}

func (c *GinContext) ResponseStatus() int {
	c.checkReleased()
	if rec := c.recorder; rec != nil {
		if rec.passthrough() {
			return rec.ResponseWriter.Status()
		}
		if rec.status != 0 {
			return rec.status
		}
		if rec.body.Len() > 0 {
			return http.StatusOK
		}
		return 0
	}
	return c.ginCtx.Writer.Status()
}

// ResponseBody returns the buffered response body, or nil once the response
// has been streamed to the client.
func (c *GinContext) ResponseBody() []byte {
	c.checkReleased()
	if c.recorder == nil || c.recorder.streamed {
		return nil
	}
	if c.recorder.body.Len() == 0 {
		return nil
	}
	return c.recorder.body.Bytes()
}

func (c *GinContext) SetHeader(k, v string) { c.checkReleased(); c.ginCtx.Header(k, v) }

// ResponseHeader returns the current value of a response header, or "" if
// unset. Implements core.ResponseHeaderReader.
func (c *GinContext) ResponseHeader(key string) string {
	c.checkReleased()
	return c.ginCtx.Writer.Header().Get(key)
}

// ResetResponse discards the buffered response body and status so a
// middleware can replace the response. It has no effect once the response
// has been streamed. Implements core.ResponseResetter.
func (c *GinContext) ResetResponse() {
	c.checkReleased()
	if c.recorder == nil || c.recorder.passthrough() {
		return
	}
	c.recorder.status = 0
	c.recorder.body.Reset()
}

func (c *GinContext) SetCookie(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	c.checkReleased()
	c.ginCtx.SetCookie(name, value, maxAge, path, domain, secure, httpOnly)
}

// SetCookieSameSite sets a cookie with an explicit SameSite attribute.
// Implements core.SameSiteCookieSetter.
func (c *GinContext) SetCookieSameSite(name, value string, maxAge int, path, domain string, secure, httpOnly bool, sameSite string) {
	c.checkReleased()
	var ss http.SameSite
	switch strings.ToLower(sameSite) {
	case "lax":
		ss = http.SameSiteLaxMode
	case "strict":
		ss = http.SameSiteStrictMode
	case "none":
		ss = http.SameSiteNoneMode
	default:
		ss = http.SameSiteDefaultMode
	}
	c.ginCtx.SetSameSite(ss)
	c.ginCtx.SetCookie(name, value, maxAge, path, domain, secure, httpOnly)
	c.ginCtx.SetSameSite(http.SameSiteDefaultMode)
}

func (c *GinContext) Redirect(status int, url string) error {
	c.checkReleased()
	c.ginCtx.Redirect(status, url)
	return nil
}

// ─── Metadata ───────────────────────────────────────────────────────────────

func (c *GinContext) ClientIP() string { c.checkReleased(); return c.ginCtx.ClientIP() }

// FullURL returns scheme://host/path?query.
func (c *GinContext) FullURL() string {
	c.checkReleased()
	r := c.ginCtx.Request
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	u := r.URL
	full := scheme + "://" + r.Host + u.EscapedPath()
	if u.RawQuery != "" {
		full += "?" + u.RawQuery
	}
	return full
}

// ─── Context Storage ────────────────────────────────────────────────────────

func (c *GinContext) Set(key string, value interface{}) { c.checkReleased(); c.ginCtx.Set(key, value) }
func (c *GinContext) Get(key string) interface{} {
	c.checkReleased()
	val, _ := c.ginCtx.Get(key)
	return val
}

// ─── Flow Control ───────────────────────────────────────────────────────────

func (c *GinContext) Next() error             { c.checkReleased(); c.ginCtx.Next(); return nil }
func (c *GinContext) Underlying() interface{} { c.checkReleased(); return c.ginCtx }
func (c *GinContext) RequestCtx() context.Context {
	c.checkReleased()
	return c.ginCtx.Request.Context()
}
func (c *GinContext) SetRequestCtx(ctx context.Context) {
	c.checkReleased()
	c.ginCtx.Request = c.ginCtx.Request.WithContext(ctx)
}

// Clone returns a safe snapshot of the context for use in goroutines.
// The request body is pre-read and copied, context values are copied
// (via gin.Context.Copy), the request is detached from the live one, and
// the clone's request context survives handler completion (WithoutCancel).
// Response methods on the clone succeed but write nowhere.
func (c *GinContext) Clone() core.Context {
	c.checkReleased()

	// Snapshot the body first so the clone never touches the live reader.
	data, bodyErr := c.Body()

	cp := c.ginCtx.Copy() // copies Keys, Params, fullPath
	// Detach the request from the live one so the clone never races with
	// the server recycling it after the handler returns.
	req := cp.Request.Clone(context.WithoutCancel(cp.Request.Context()))
	var bodyCopy []byte
	if bodyErr == nil {
		bodyCopy = append([]byte(nil), data...)
		req.Body = io.NopCloser(bytes.NewReader(bodyCopy))
	} else {
		req.Body = http.NoBody
	}
	cp.Request = req

	// gin.Context.Copy nils the underlying response writer; give the clone
	// a discarding recorder so response calls do not panic.
	rec := &responseRecorder{ResponseWriter: &discardWriter{header: http.Header{}}}
	cp.Writer = rec

	return &GinContext{
		ginCtx:   cp,
		recorder: rec,
		bodyRead: true,
		bodyData: bodyCopy,
		bodyErr:  bodyErr,
	}
}

// ─── Internal helpers ───────────────────────────────────────────────────────

// wrapHandler is the SINGLE native gin handler registered per route. The
// full core middleware chain (group chain outermost, then route middleware,
// then the handler) is composed in-process, so errors returned by the
// handler flow back through every middleware — filters and interceptors see
// them. The handler also:
//   - recovers panics into a 500 through the error path (adapter-level
//     safety net, independent of middleware.Recovery());
//   - invokes the error handler EXACTLY ONCE, and only if nothing has been
//     written to the response yet (otherwise the error is logged);
//   - flushes the buffered response to the client exactly once.
func wrapHandler(handler core.HandlerFunc, errHandler core.ErrorHandler) gin.HandlerFunc {
	return func(gc *gin.Context) {
		ctx := acquireContext(gc)
		defer releaseContext(ctx)
		rec := ctx.recorder

		err := runSafely(handler, ctx)

		if err != nil {
			if rec.written() {
				core.Log().Error("handler error after response was written",
					core.F("error", err.Error()),
					core.F("path", gc.FullPath()))
			} else {
				eh := errHandler
				if eh == nil {
					eh = core.DefaultErrorHandler
				}
				eh(ctx, err)
			}
			gc.Abort()
		}
		rec.flushToClient()
	}
}

// runSafely executes the composed chain, converting panics into a 500 error.
func runSafely(handler core.HandlerFunc, ctx *GinContext) (err error) {
	defer func() {
		if r := recover(); r != nil {
			core.Log().Error("panic recovered",
				core.F("panic", fmt.Sprint(r)),
				core.F("stack", string(debug.Stack())))
			err = core.ErrInternalServer("Internal Server Error")
		}
	}()
	return handler(ctx)
}

// wrapNativeChain runs a composed core middleware chain around gin's native
// downstream handlers (used for Static/StaticFile, whose terminal handler is
// gin's own file server). File responses stream directly.
func wrapNativeChain(mws []core.MiddlewareFunc, errHandler core.ErrorHandler) gin.HandlerFunc {
	terminal := func(c core.Context) error {
		if gcx, ok := c.(*GinContext); ok {
			gcx.checkReleased()
			if gcx.recorder != nil {
				gcx.recorder.startStreaming()
			}
			gcx.ginCtx.Next()
			return nil
		}
		if gc, ok := c.Underlying().(*gin.Context); ok {
			gc.Next()
		}
		return nil
	}
	return wrapHandler(applyRouteMiddleware(terminal, mws), errHandler)
}

// applyRouteMiddleware composes middleware around a handler; mws[0] ends up
// outermost: final = mws[0](mws[1](...(handler))).
func applyRouteMiddleware(handler core.HandlerFunc, mws []core.MiddlewareFunc) core.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}

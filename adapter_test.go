package ginadapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	core "github.com/ashrafAli23/nestgo/core"
)

func newTestServer(t *testing.T, mutate func(*core.Config)) *GinServer {
	t.Helper()
	cfg := core.DefaultConfig()
	cfg.DisableLogger = true
	if mutate != nil {
		mutate(cfg)
	}
	return New(cfg).(*GinServer)
}

func serve(s *GinServer, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.engine.ServeHTTP(w, req)
	return w
}

// A handler error returned from the route must be visible to Use()
// middleware (the mechanism global filters/interceptors rely on).
func TestUseMiddlewareSeesHandlerError(t *testing.T) {
	s := newTestServer(t, nil)
	var seen error
	s.Use(func(next core.HandlerFunc) core.HandlerFunc {
		return func(c core.Context) error {
			err := next(c)
			seen = err
			return err
		}
	})
	s.GET("/boom", func(c core.Context) error {
		return core.ErrNotFound("nope")
	})

	w := serve(s, httptest.NewRequest("GET", "/boom", nil))

	if seen == nil {
		t.Fatal("Use() middleware did not observe the handler error")
	}
	var httpErr *core.HTTPError
	if !errors.As(seen, &httpErr) || httpErr.Code != 404 {
		t.Fatalf("middleware saw wrong error: %v", seen)
	}
	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), "nope") {
		t.Fatalf("body = %q, want error message", w.Body.String())
	}
}

// The error handler must run exactly once even with several middleware
// layers propagating the same error.
func TestErrorHandlerInvokedExactlyOnce(t *testing.T) {
	var calls int
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.ErrorHandler = func(c core.Context, err error) {
			calls++
			core.DefaultErrorHandler(c, err)
		}
	})
	passthrough := func(next core.HandlerFunc) core.HandlerFunc {
		return func(c core.Context) error { return next(c) }
	}
	s.Use(passthrough, passthrough, passthrough)
	s.GET("/err", func(c core.Context) error {
		return core.ErrBadRequest("bad")
	})

	w := serve(s, httptest.NewRequest("GET", "/err", nil))

	if calls != 1 {
		t.Fatalf("error handler ran %d times, want 1", calls)
	}
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if n := strings.Count(w.Body.String(), `"code"`); n != 1 {
		t.Fatalf("expected exactly one error body, got %d (%q)", n, w.Body.String())
	}
}

// If a response was already produced, a later error must NOT be written on
// top of it (no superfluous WriteHeader, no second body).
func TestDoubleWriteGuard(t *testing.T) {
	var calls int
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.ErrorHandler = func(c core.Context, err error) {
			calls++
			core.DefaultErrorHandler(c, err)
		}
	})
	s.Use(func(next core.HandlerFunc) core.HandlerFunc {
		return func(c core.Context) error {
			if err := c.JSON(200, map[string]string{"ok": "yes"}); err != nil {
				return err
			}
			return core.ErrInternalServer("late error")
		}
	})
	s.GET("/late", func(c core.Context) error { return nil })

	w := serve(s, httptest.NewRequest("GET", "/late", nil))

	if calls != 0 {
		t.Fatalf("error handler ran %d times after response was written, want 0", calls)
	}
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); !strings.Contains(got, `"ok":"yes"`) || strings.Contains(got, "late error") {
		t.Fatalf("body = %q, want only the first response", got)
	}
}

// Headers set by middleware AFTER next() must still reach the client
// (buffered response model).
func TestPostHandlerSetHeader(t *testing.T) {
	s := newTestServer(t, nil)
	s.Use(func(next core.HandlerFunc) core.HandlerFunc {
		return func(c core.Context) error {
			err := next(c)
			c.SetHeader("X-After", "1")
			return err
		}
	})
	s.GET("/hdr", func(c core.Context) error {
		return c.JSON(200, map[string]string{"a": "b"})
	})

	w := serve(s, httptest.NewRequest("GET", "/hdr", nil))

	if w.Header().Get("X-After") != "1" {
		t.Fatalf("post-handler header missing; headers: %v", w.Header())
	}
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// Middleware ordering: group chain outermost, then route middleware.
func TestMiddlewareOrdering(t *testing.T) {
	s := newTestServer(t, nil)
	var order []string
	tag := func(name string) core.MiddlewareFunc {
		return func(next core.HandlerFunc) core.HandlerFunc {
			return func(c core.Context) error {
				order = append(order, name)
				return next(c)
			}
		}
	}
	s.Use(tag("use"))
	g := s.Group("/g", tag("group"))
	g.GET("/route", func(c core.Context) error {
		order = append(order, "handler")
		return c.NoContent(204)
	}, tag("route"))

	serve(s, httptest.NewRequest("GET", "/g/route", nil))

	want := []string{"use", "group", "route", "handler"}
	if fmt.Sprint(order) != fmt.Sprint(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

// Panics in handlers become a 500 through the error path.
func TestPanicRecoveredTo500(t *testing.T) {
	s := newTestServer(t, nil)
	s.GET("/panic", func(c core.Context) error {
		panic("kaboom")
	})

	w := serve(s, httptest.NewRequest("GET", "/panic", nil))

	if w.Code != 500 {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Internal Server Error") {
		t.Fatalf("body = %q, want generic 500 body", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "kaboom") {
		t.Fatalf("panic message leaked to client: %q", w.Body.String())
	}
}

// Config.BodyLimit must be enforced with a 413.
func TestBodyLimitReturns413(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.BodyLimit = 8
	})
	s.POST("/upload", func(c core.Context) error {
		if _, err := c.Body(); err != nil {
			return err
		}
		return c.NoContent(204)
	})
	h := s.rootHandler()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/upload", strings.NewReader(strings.Repeat("x", 100)))
	h.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}

	// A body within the limit passes.
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/upload", strings.NewReader("tiny"))
	h.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

// SendStream must terminate on EOF (no busy loop) and deliver the content.
func TestSendStreamTerminates(t *testing.T) {
	s := newTestServer(t, nil)
	s.GET("/stream", func(c core.Context) error {
		return c.SendStream(strings.NewReader("hello stream"))
	})

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- serve(s, httptest.NewRequest("GET", "/stream", nil))
	}()

	select {
	case w := <-done:
		if w.Body.String() != "hello stream" {
			t.Fatalf("body = %q, want %q", w.Body.String(), "hello stream")
		}
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if !w.Flushed {
			t.Fatal("stream was never flushed to the client")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SendStream did not terminate (busy loop)")
	}
}

// SSE via core.SSE goes through SendStream and must terminate when the
// channel closes; ResponseBody must be nil for streamed responses.
func TestSSEStreamTerminates(t *testing.T) {
	s := newTestServer(t, nil)
	var streamedBody []byte
	s.Use(func(next core.HandlerFunc) core.HandlerFunc {
		return func(c core.Context) error {
			err := next(c)
			streamedBody = c.ResponseBody()
			return err
		}
	})
	s.GET("/sse", func(c core.Context) error {
		stream := core.NewSSEStream(2)
		stream <- core.SSEEvent{Event: "message", Data: "one"}
		stream <- core.SSEEvent{Event: "message", Data: "two"}
		close(stream)
		return core.SSE(c, stream)
	})

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- serve(s, httptest.NewRequest("GET", "/sse", nil))
	}()

	select {
	case w := <-done:
		body := w.Body.String()
		if !strings.Contains(body, "data: one") || !strings.Contains(body, "data: two") {
			t.Fatalf("body = %q, want SSE events", body)
		}
		if got := w.Header().Get("Content-Type"); got != "text/event-stream" {
			t.Fatalf("Content-Type = %q, want text/event-stream", got)
		}
		if streamedBody != nil {
			t.Fatalf("ResponseBody() = %q for a streamed response, want nil", streamedBody)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SSE stream did not terminate")
	}
}

// Clone must be a safe snapshot: usable after the handler returns, with a
// copied body, copied context values, and a non-canceled request context.
func TestCloneIsSafeSnapshot(t *testing.T) {
	s := newTestServer(t, nil)
	var clone core.Context
	s.POST("/clone/:id", func(c core.Context) error {
		c.Set("tenant", "acme")
		clone = c.Clone()
		return c.JSON(200, map[string]string{"ok": "1"})
	})

	req := httptest.NewRequest("POST", "/clone/42", strings.NewReader("payload"))
	w := serve(s, req)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// The request is over; the live context has been released. The clone
	// must still work.
	b, err := clone.Body()
	if err != nil || string(b) != "payload" {
		t.Fatalf("clone.Body() = %q, %v; want %q, nil", b, err, "payload")
	}
	if clone.Method() != "POST" {
		t.Fatalf("clone.Method() = %q", clone.Method())
	}
	if clone.Param("id") != "42" {
		t.Fatalf("clone.Param(id) = %q, want 42", clone.Param("id"))
	}
	if clone.Get("tenant") != "acme" {
		t.Fatalf("clone.Get(tenant) = %v, want acme", clone.Get("tenant"))
	}
	if err := clone.RequestCtx().Err(); err != nil {
		t.Fatalf("clone.RequestCtx() canceled: %v", err)
	}
	// Response calls on a clone must not panic (they write nowhere).
	if err := clone.JSON(202, map[string]string{"async": "done"}); err != nil {
		t.Fatalf("clone.JSON returned %v", err)
	}
}

// Using the pooled context after the handler returned must panic loudly
// instead of silently touching another request's data.
func TestUseAfterReleasePanics(t *testing.T) {
	s := newTestServer(t, nil)
	var leaked core.Context
	s.GET("/leak", func(c core.Context) error {
		leaked = c
		return c.NoContent(204)
	})
	serve(s, httptest.NewRequest("GET", "/leak", nil))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected use-after-release panic")
		}
		if !strings.Contains(fmt.Sprint(r), "use-after-release") {
			t.Fatalf("panic message = %v, want use-after-release explanation", r)
		}
	}()
	_ = leaked.Method()
}

// Concurrent Start/Shutdown must not race, and Shutdown before Start must
// prevent the listener from ever coming up.
func TestShutdownBeforeStart(t *testing.T) {
	s := newTestServer(t, nil)
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := s.Start("127.0.0.1:0"); !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Start after Shutdown = %v, want http.ErrServerClosed", err)
	}
}

func TestStartThenShutdown(t *testing.T) {
	s := newTestServer(t, nil)
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start("127.0.0.1:0") }()

	// Give ListenAndServe a moment to bind.
	time.Sleep(100 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Start returned %v, want http.ErrServerClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after Shutdown")
	}
}

// Server timeouts and header limits from core.Config must be mapped.
func TestServerConfigMapping(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.ReadHeaderTimeout = 7
		cfg.IdleTimeout = 33
		cfg.MaxHeaderBytes = 4096
	})
	srv, err := s.prepare("127.0.0.1:0")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if srv.ReadHeaderTimeout != 7*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout != 33*time.Second {
		t.Fatalf("IdleTimeout = %v", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes != 4096 {
		t.Fatalf("MaxHeaderBytes = %d", srv.MaxHeaderBytes)
	}
}

// ─── Cross-adapter parity ───────────────────────────────────────────────────

func TestQueryDefaultParity(t *testing.T) {
	s := newTestServer(t, nil)
	var present, empty, absent string
	s.GET("/q", func(c core.Context) error {
		present = c.QueryDefault("a", "def")
		empty = c.QueryDefault("b", "def")
		absent = c.QueryDefault("z", "def")
		return c.NoContent(204)
	})
	serve(s, httptest.NewRequest("GET", "/q?a=1&b=", nil))

	if present != "1" {
		t.Fatalf("present = %q, want 1", present)
	}
	if empty != "" {
		t.Fatalf("present-but-empty = %q, want \"\"", empty)
	}
	if absent != "def" {
		t.Fatalf("absent = %q, want def", absent)
	}
}

func TestStringVerbatimWithoutArgs(t *testing.T) {
	s := newTestServer(t, nil)
	s.GET("/pct", func(c core.Context) error {
		return c.String(200, "100% sure")
	})
	s.GET("/fmt", func(c core.Context) error {
		return c.String(200, "n=%d", 7)
	})

	if w := serve(s, httptest.NewRequest("GET", "/pct", nil)); w.Body.String() != "100% sure" {
		t.Fatalf("verbatim body = %q", w.Body.String())
	}
	if w := serve(s, httptest.NewRequest("GET", "/fmt", nil)); w.Body.String() != "n=7" {
		t.Fatalf("formatted body = %q", w.Body.String())
	}
}

func TestSendBytesDefaultContentType(t *testing.T) {
	s := newTestServer(t, nil)
	s.GET("/bin", func(c core.Context) error {
		return c.SendBytes(200, []byte{1, 2, 3})
	})
	s.GET("/png", func(c core.Context) error {
		c.SetHeader("Content-Type", "image/png")
		return c.SendBytes(200, []byte{1, 2, 3})
	})

	if w := serve(s, httptest.NewRequest("GET", "/bin", nil)); w.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("default Content-Type = %q", w.Header().Get("Content-Type"))
	}
	if w := serve(s, httptest.NewRequest("GET", "/png", nil)); w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("explicit Content-Type = %q", w.Header().Get("Content-Type"))
	}
}

func TestFullURL(t *testing.T) {
	s := newTestServer(t, nil)
	var got string
	s.GET("/p", func(c core.Context) error {
		got = c.FullURL()
		return c.NoContent(204)
	})
	serve(s, httptest.NewRequest("GET", "http://example.com/p?x=1&y=2", nil))
	if got != "http://example.com/p?x=1&y=2" {
		t.Fatalf("FullURL = %q", got)
	}
}

// ─── Optional capability interfaces ─────────────────────────────────────────

func TestResetResponseAndResponseHeader(t *testing.T) {
	s := newTestServer(t, nil)
	s.Use(func(next core.HandlerFunc) core.HandlerFunc {
		return func(c core.Context) error {
			err := next(c)
			// ETag-style: replace the buffered 200 with a 304.
			if rr, ok := c.(core.ResponseResetter); ok && c.ResponseStatus() == 200 {
				rr.ResetResponse()
				c.Status(304)
			}
			return err
		}
	})
	var ctVisible string
	s.GET("/etag", func(c core.Context) error {
		if err := c.JSON(200, map[string]string{"big": "body"}); err != nil {
			return err
		}
		if hr, ok := c.(core.ResponseHeaderReader); ok {
			ctVisible = hr.ResponseHeader("Content-Type")
		}
		return nil
	})

	w := serve(s, httptest.NewRequest("GET", "/etag", nil))
	if w.Code != 304 {
		t.Fatalf("status = %d, want 304", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty after reset", w.Body.String())
	}
	if !strings.HasPrefix(ctVisible, "application/json") {
		t.Fatalf("ResponseHeader(Content-Type) = %q", ctVisible)
	}
}

func TestSetCookieSameSite(t *testing.T) {
	s := newTestServer(t, nil)
	s.GET("/cookie", func(c core.Context) error {
		c.(core.SameSiteCookieSetter).SetCookieSameSite("sid", "v", 60, "/", "", true, true, "Strict")
		return c.NoContent(204)
	})
	w := serve(s, httptest.NewRequest("GET", "/cookie", nil))
	sc := w.Header().Get("Set-Cookie")
	if !strings.Contains(sc, "sid=v") || !strings.Contains(sc, "SameSite=Strict") {
		t.Fatalf("Set-Cookie = %q, want SameSite=Strict cookie", sc)
	}
}

// Only ONE recorder is installed per request even with many middleware, and
// ResponseBody sees the handler's write from any layer.
func TestSingleRecorderSharedAcrossLayers(t *testing.T) {
	s := newTestServer(t, nil)
	var bodies []string
	inspect := func(next core.HandlerFunc) core.HandlerFunc {
		return func(c core.Context) error {
			err := next(c)
			bodies = append(bodies, string(c.ResponseBody()))
			return err
		}
	}
	s.Use(inspect, inspect, inspect)
	s.GET("/one", func(c core.Context) error {
		return c.String(200, "payload")
	})

	w := serve(s, httptest.NewRequest("GET", "/one", nil))

	if w.Body.String() != "payload" {
		t.Fatalf("body = %q, want single %q (no duplicated writes)", w.Body.String(), "payload")
	}
	for i, b := range bodies {
		if b != "payload" {
			t.Fatalf("layer %d saw ResponseBody %q, want %q", i, b, "payload")
		}
	}
}

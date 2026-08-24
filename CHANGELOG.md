# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- Bumped the `go` directive from `1.25.0` to `1.25.14`, pulling in Go standard-library security fixes. `govulncheck ./...` reports zero reachable vulnerabilities (the only remaining advisory is the unfixable "openpgp unmaintained" notice in `golang.org/x/crypto`, which this module never imports).

### Changed

- Upgraded all transitive dependencies to latest stable, including `bytedance/sonic` v1.15.2, `go-playground/validator/v10` v10.30.3, `quic-go` v0.61.0, `golang.org/x/crypto` v0.55.0, and `golang.org/x/net` v0.58.0. Gin itself remains at `v1.12.0` (latest stable).
- **Behavior change:** middleware added via `Use()` after a route is registered no longer applies to that route — middleware chains are now composed into the route's handler at registration time (converges with the Fiber adapter).

### Added

- The underlying `http.Server` now honors `core.Config.ReadHeaderTimeout`, `IdleTimeout`, and `MaxHeaderBytes` (defaults 10s / 60s / 1 MB) — Slowloris protection and idle-connection bounds
- `GinContext` implements the `core.ResponseResetter`, `core.ResponseHeaderReader`, and `core.SameSiteCookieSetter` capabilities (enables ETag 304 rewrites, Idempotency Content-Type replay, and CSRF SameSite cookies)

### Fixed

- Handler errors now propagate back through `Use()`/`Group()` middleware: each route is composed into a single native Gin handler, the configured `ErrorHandler` runs exactly once and only when nothing has been written, and panics are recovered into a generic 500
- `SendStream` no longer spins in an infinite busy-loop after the stream drains; streaming responses (including SSE) flush per chunk in real time and terminate on EOF
- Responses are buffered exactly once per request instead of once per middleware layer; `SendStream`/`SendFile`/`Download` switch to direct streaming so large or streaming responses never accumulate in memory
- `core.Config.BodyLimit` is now enforced via `http.MaxBytesReader`; oversized bodies return 413 (matching the Fiber adapter)
- Data race between `Start` and `Shutdown` on the `http.Server` fixed; a `Shutdown` that lands before `Start` no longer leaks a listener
- `Clone()` returns a safe snapshot (cached body, request detached from the request context, discard response writer) instead of sharing the live `*http.Request`
- Pooled contexts now panic with a clear use-after-release message instead of silently exposing another request's data
- Cross-adapter parity: `SendBytes` defaults Content-Type to `application/octet-stream` when unset, `FullURL` returns `scheme://host/path?query`, `QueryDefault` returns the default only when the key is absent, and `String` writes the format verbatim when no values are given

---

## [1.3.0] - 2026-04-09

### Changed

- Upgraded `github.com/ashrafAli23/nestgo` core dependency to `v1.3.0`.
- **Logger Integration:** Replaced internal `fmt.Printf` and `log` calls with `core.Log()` to support NestGo's pluggable logging system.
- **Debug Mode:** The `core.Config.Debug` flag now correctly toggles between `gin.DebugMode` and `gin.ReleaseMode`.

### Added

- Implemented `ANY()` method on `Router` to support all HTTP methods (delegates to Gin's `Any()`).
- Added `StartTLS(addr, certFile, keyFile)` support for HTTPS servers.

---

## [1.2.0] - 2026-04-06

### Changed

- Upgraded `github.com/ashrafAli23/nestgo` core dependency to `v1.2.0`.

---

## [1.1.0] - 2026-04-05

### Added

- Initial release of the NestGo Gin Adapter.
- Full implementation of `core.Server`, `core.Router`, and `core.Context` interfaces.
- Context pooling for zero-allocation requests.
- Body caching for multiple reads.
- Graceful shutdown support.

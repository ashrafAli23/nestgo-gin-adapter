# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

# Benchmarks

In-process request benchmarks (no network): the raw engine vs NestGo running on it.
They measure NestGo's overhead over the engine, not absolute throughput.

Run them yourself:

```bash
go test -run '^$' -bench . -benchmem -count 3 ./...
```

## Results

Machine: 12th Gen Intel(R) Core(TM) i7-12850HX, Go go1.26.7 linux/amd64, 2026-08-25

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| RawGin_HelloJSON | 2315 | 1443 | 15 |
| NestGoGin_HelloJSON | 2381 | 1349 | 15 |
| RawGin_Middleware3 | 2910 | 1491 | 17 |
| NestGoGin_Middleware3 | 3918 | 1816 | 23 |

Values are the median of 3 runs (`go test -count 3`); raw output in `/tmp/bench-gin.txt`.

`Middleware3` = recovery + request-id header + an allow-all guard on both sides, so the
comparison is like-for-like.

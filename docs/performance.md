# Performance Workflow

FastBrew includes two performance tracks:

- End-to-end CLI comparisons (`benchmark/benchmark.go`)
- Microbench regression gates (`go test -bench`)
- Daemon mutation-job execution for reduced command startup overhead

## End-to-End Benchmarks

Use Make targets:

- `make perf-bench`
- `make perf-bench-cold`
- `make perf-bench-warm`
- `make perf-compare`

These run multi-sample benchmarks and report p50/p95 for cold/warm paths.

## Microbenchmarks

Use:

- `make perf-profile`

or directly:

```bash
go test -run '^$' -bench . -benchmem ./internal/brew ./internal/services
```

Key suites:

- `BenchmarkResolveDeps` (`internal/brew`)
- `BenchmarkVersionCompare` (`internal/brew`)
- `BenchmarkPlistParser_Parse` (`internal/services`)

## CI Regression Budgets

Workflow: `.github/workflows/perf-regression.yml`

- Enforces `BenchmarkResolveDeps` budget.
- Enforces `BenchmarkPlistParser_Parse` budget.

If either benchmark exceeds budget, CI fails to prevent regressions.

## Validation Checklist

1. Run cold and warm benchmark targets locally.
2. Verify read-command p50/p95 trend against baseline.
3. Run microbench suite and inspect ns/op deltas.
4. Check daemon stats after workload:
   - cache hits increasing
   - request volume stable
   - no repeated fallback warnings

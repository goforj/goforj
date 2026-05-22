# Bench Commands

This package contains GoForj's maintainer-only benchmarking and profiling commands.

These commands exist to answer a specific class of questions:

- Did a framework change make the hot path faster or slower?
- How much overhead comes from one feature such as inspects, metrics, or logging?
- Where is allocation or CPU time going in a real rendered app?
- Are improvements real in the live request path, or only visible in synthetic microbenchmarks?

They are intentionally hidden from normal CLI help and are meant for framework development, release validation, and regression hunting.

## Command Naming

The commands are exposed at the root CLI as `category:action`:

- `bench:inspect-overhead`
- `bench:logger-overhead`
- `bench:http-live-profile`
- `bench:http-runtime-profile`
- `bench:metrics-overhead`

Run them with:

```sh
cd /workspace/code/goforj
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go run ./cmd/forj <command>
```

The explicit Go cache paths keep repeated runs isolated from a developer's normal cache state.

## Why These Exist

GoForj has a lot of generated runtime behavior:

- HTTP middleware
- inspect capture
- metrics
- structured logging
- renderer-driven app bootstrapping

Normal unit tests prove correctness, but they do not answer:

- how much one layer costs
- whether a refactor moved real request throughput
- whether a new abstraction added allocation churn

These commands fill that gap.

## Commands

### `bench:inspect-overhead`

Renders a fixed probe app and compares HTTP request overhead with inspects disabled versus enabled.

Typical use:

```sh
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go run ./cmd/forj bench:inspect-overhead --iterations=5000 --rounds=5
```

What it measures:

- `ns/op`
- `B/op`
- `allocs/op`
- median comparison across rounds

Expected output shape:

- a section header
- one comparison table for the inspect scenarios
- deltas between `off` and `on`

Use this when:

- inspect code changed
- request context propagation changed
- capture/body/header behavior changed

### `bench:logger-overhead`

Renders a fixed probe app and benchmarks the generated logger path against direct logging behavior.

Typical use:

```sh
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go run ./cmd/forj bench:logger-overhead --iterations=200000 --rounds=3
```

What it measures:

- repeated versus unique access-log-like shapes
- `ns/op`
- `B/op`
- `allocs/op`
- median comparison across rounds

Expected output shape:

- one table per logger scenario
- direct/baseline row
- generated logger rows
- relative delta columns

Use this when:

- logger internals changed
- sink behavior changed
- dedupe behavior changed
- access-log hot path changed

### `bench:metrics-overhead`

Renders a fixed probe app and compares telemetry surfaces with metrics disabled versus enabled.

Typical use:

```sh
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go run ./cmd/forj bench:metrics-overhead --iterations=5000 --auth-iterations=500 --rounds=5
```

What it measures:

- multiple generated telemetry surfaces
- auth and non-auth paths
- `ns/op`
- `B/op`
- `allocs/op`
- median comparison across rounds

Expected output shape:

- surface-by-surface comparison tables
- `off` versus `on`
- optional warnings if the generated probe app observed something unusual

Use this when:

- metrics middleware changed
- label lookup changed
- telemetry hooks changed

### `bench:http-runtime-profile`

Renders a fixed app, compiles benchmark tests for `internal/http`, and gathers pprof data for benchmark modes.

Typical use:

```sh
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go run ./cmd/forj bench:http-runtime-profile --bench-time=3s
```

Useful flags:

- `--modes=baseline,metrics_only,logging_only,inspect_only,full_stack`
- `--top=20`
- `--keep`

What it measures:

- benchmark-mode `ns/op`
- `B/op`
- `allocs/op`
- CPU profile top entries
- allocation-space profile top entries

Expected output shape:

- one result block per runtime mode
- profile file paths when `--keep` is used
- top pprof summaries in console output

Use this when:

- you want mode-by-mode internal HTTP benchmark breakdowns
- live server benchmarking is too coarse
- you need pprof output tied to named benchmark modes

### `bench:http-live-profile`

Builds and runs a real server process, drives live HTTP traffic against it, and captures pprof data from the running app.

Typical use:

```sh
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go run ./cmd/forj bench:http-live-profile \
  --server-stack=rendered \
  --duration-ms=15000 \
  --inspect-enabled=false \
  --metrics-enabled=false \
  --http-access-logs=false \
  --httpcors=false \
  --health-mode=nocontent
```

Available server stacks:

- `rendered`
- `rawnethttp`
- `rawdirect`
- `echonative`

Available health modes:

- `json`
- `text`
- `nocontent`

What it measures:

- real loopback request throughput
- total ops
- `ops/sec`
- error count
- `P50/P95/P99`
- CPU profile top entries
- heap profile top entries

Expected output shape:

- a run summary with throughput and latency
- profile paths when `--keep` is used
- top CPU and heap lines from pprof

Use this when:

- a request-path change needs real server validation
- you want feature toggle matrices
- you want cross-stack comparisons on the same harness

## Reading Results

These commands are most useful for relative comparisons, not single raw numbers in isolation.

Good questions:

- Did `inspect off` versus `inspect on` narrow or widen?
- Did `metrics enabled` stay within an acceptable tax?
- Did `nocontent` move while `json` stayed flat?
- Did `allocs/op` drop without breaking throughput?

Bad conclusions:

- assuming one noisy run proves a permanent gain
- assuming synthetic runtime benches equal live traffic behavior
- comparing runs done with different response shapes or different stacks and calling them equivalent

## Working Style

When using these commands during framework work:

1. Make one narrow change.
2. Re-run the smallest relevant bench command first.
3. Re-run the live profile if the small benchmark moved enough to matter.
4. Keep only changes that show a stable win and stay simple.

## Temporary Workspaces

Most commands render a temp app and remove it automatically.

Use `--keep` when you want to inspect:

- rendered source
- built binaries
- generated profiles
- probe code

This is especially useful for `bench:http-runtime-profile` and `bench:http-live-profile`.

## Scope

This package is intentionally separate from `internal/forj` command implementation because these are framework-maintainer tools, not normal user-facing commands.

The commands still integrate with the root CLI, but their code and helpers live here so:

- the namespace is explicit
- the maintenance surface is easier to find
- benchmark-specific helpers do not clutter unrelated command code

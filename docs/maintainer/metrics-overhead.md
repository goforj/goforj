# Metrics Overhead Test

This repo includes a hidden CLI command for measuring framework-owned metrics overhead across the observed surfaces with metrics disabled and enabled.

Run it from the repo root:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go run ./cmd/forj bench:metrics-overhead --iterations=5000 --auth-iterations=500 --rounds=3
```

What it does:

- renders a temporary app with metrics enabled
- drives fixed traffic through the instrumented surfaces
- runs the same workload with metrics toggled off and on
- prints ASCII comparison tables for latency and memory overhead

Observed surfaces:

- `http`
- `auth`
- `cache`
- `storage`
- `events`
- `mail`
- `database`
- `queue`
- `scheduler`

Useful flags:

- `--iterations`: iterations for non-auth surfaces
- `--auth-iterations`: separate auth workload size because auth is dominated by password hashing
- `--rounds`: number of comparison rounds; the command prints medians across rounds

Example smaller shakeout run:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go run ./cmd/forj bench:metrics-overhead --iterations=500 --auth-iterations=50 --rounds=3
```

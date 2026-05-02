<p align="center">
  <img src="https://raw.githubusercontent.com/goforj/docs/refs/heads/main/docs/public/assets/goforj-full.png" width="500" alt="GoForj Logo">
</p>

<p align="center">
<strong>This project is currently unreleased and under heavy active private development.</strong>
</p>

<p align="center">
If you are interested in being a part of our private beta, please reach out to us by opening an issue or contacting us at chris@milestech.co
</p>

## Metrics Overhead Test

The repo includes a hidden CLI command for measuring framework-owned metrics overhead across the observed surfaces with metrics disabled and enabled.

Run it from the repo root:

```bash
cd /workspace/code/goforj
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go run ./cmd/forj test:metrics-overhead --iterations=5000 --auth-iterations=500 --rounds=3
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
cd /workspace/code/goforj
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
go run ./cmd/forj test:metrics-overhead --iterations=500 --auth-iterations=50 --rounds=3
```

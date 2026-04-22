# Observability

This document describes the current logging and telemetry model across GoForj and `web`.

## Current Direction

GoForj observability work should currently be thought of in two layers:

- metric production inside generated apps
- standard observability-stack validation outside the app

That means:

- generated apps emit Prometheus-compatible metrics
- the metric contract is first proven with VictoriaMetrics and Grafana
- Lighthouse comes after the metric contract is already proven useful

This ordering is intentional.

Lighthouse should not be the first place where the framework discovers:

- awkward metric names
- missing labels
- noisy histograms
- dashboard-hostile aggregation choices

Those are easier to discover by trying to build standard dashboards against a Prometheus-compatible backend first.

## Logging Philosophy

Prefer:

- high-signal default logs
- low-noise startup/shutdown output
- explicit operational markers

Avoid:

- giant boot dumps
- per-hook trace spam by default
- leaking implementation details into user-facing logs

## Process Logs

These are generally good default-visible logs:

- HTTP server start/stop
- scheduler start/stop
- queue worker start/stop
- watcher start/stop
- dev ready markers
- clear degraded-runtime warnings when optional facilities are unavailable

## Primitive Chatter

Detailed managed primitive chatter should generally be:

- debug-level
- or explicitly enabled

Examples:

- per-hook lifecycle details
- DB/event bus start-stop chatter
- repeated backend driver connection failures when the higher-level degraded state is already known

## Route Visibility

Normal boot should not print the full route table.

Instead:

- boot logs a route count summary
- `route:list` is the explicit full route dump

Current intended message shape:

```text
Routes registered; use command route:list for full list
```

## Degraded Runtime Feedback

When optional subsystems degrade, prefer one clear structured warning over repeated low-level noise.

Examples:

- optional storage disk unavailable; skipping
- reconnecting to Lighthouse live runtime

Recent rules learned from storage work:

- log optional storage disk failures through the normal app logger
- do not print directly from generated managers to raw `stderr`
- avoid emitting the same warning once per bootstrap process if `forj run` starts multiple subprocesses
- if the UI hides an unavailable resource, show an explicit unavailable/degraded state instead of silent emptiness

## `APP_LOG_TIME`

Console timestamps are gated by:

- `APP_LOG_TIME`

If timestamps seem present but only show `.000`, the likely issue is emitted timestamp precision, not the renderer.

## `web` Telemetry Story

Current important pieces in `web`:

- `webmiddleware`
  - request logging
  - body dump
  - other request/response middleware
- `web.Response`
  - response status/size/committed visibility
- `webprometheus`
  - Prometheus metrics middleware and handler

## `webprometheus`

This package exists to give `web` a first-class Prometheus story.

Current capability includes:

- scrape handler
- request counter
- in-flight gauge
- request duration histogram
- request size histogram
- response size histogram
- route-aware labels

This is meant to make `web` feature-close to Echo’s Prometheus support without forcing apps to depend directly on Echo middleware.

## Metrics Posture

When adding new framework metrics, prefer:

- normalized route or resource identities
- bounded labels
- low-cardinality dimensions
- metrics that answer real operational questions

Avoid:

- raw path labels with IDs in them
- labels sourced from user input
- emitting framework-owned transport traffic as if it were app traffic
- adding metrics that look impressive in `/metrics` output but do not help operators

Good default framework-owned surfaces:

- HTTP
- queues
- scheduler

Potentially useful later surfaces:

- database health
- cache/store operations
- mail delivery
- event delivery

The current proving path is:

1. HTTP metrics
2. queue metrics
3. scheduler metrics
4. VictoriaMetrics + Grafana validation
5. Lighthouse adaptation

If in doubt, read [../designs/metrics-design.md](../designs/metrics-design.md) before expanding the metric set.

## Lighthouse / Benchmark Output

Benchmark UI output should avoid:

- stale queued states after completion
- ambiguous runtime states

When UI state looks wrong, check whether the status precedence is correct before assuming runtime behavior is wrong.

Additional Lighthouse UX guidance:

- when the live websocket connection is lost after boot, dim the app and show an explicit reconnecting state
- while reconnecting, freeze navigation so users do not walk the UI into inconsistent states
- use toast feedback for transient action failures
- reserve inline alerts for page-state failures that need to stay visible
- when a selected resource disappears during refresh, show an explicit unavailable message instead of a silent empty state

Recent benchmark-specific lesson:

- benchmark letter grades should be driver-class aware
- do not grade external caches like Redis against in-process memory thresholds
- absolute throughput alone is a misleading UX signal when the backend class is fundamentally different

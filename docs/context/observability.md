# Observability

This document describes the current logging and telemetry model across GoForj and `web`.

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

# Lighthouse Inspects

## Current State

The old `trace/traces` product naming was renamed to `inspect/inspects`.

Important distinction:
- keep `trace_id` as the execution correlation field in context and stored records
- use `inspect` / `inspects` for the Lighthouse feature, APIs, storage keys, and UI

The feature is now live enough to browse real request/job/scheduler activity in Lighthouse:
- inspect list in the left lane
- inspect detail in the right lane
- request-specific tabs:
  - `Timeline`
  - `Request`
  - `Response`
- cache preview JSON beautification/highlighting
- copy actions for inspect id, headers, bodies, and request replay (`Copy to Curl`)

## Main Files

Backend capture and store:
- `templates/internal/inspects/manager.go.tmpl`
- `templates/internal/http/server.go.tmpl`

Lighthouse UI:
- `templates/internal/lighthouse/ui/src/views/InspectsView.vue`
- `templates/internal/lighthouse/ui/src/lib/json-preview.ts`

Related generated/runtime glue:
- `templates/internal/http/lighthouse.go.tmpl`
- `templates/wire/inject_app_services.go.tmpl`

## Important Product Decisions

### Naming

- Product surface is `inspect`
- Correlation field remains `trace_id`
- Named cache accessor for inspect storage is `caches.Inspects()`
- Lighthouse runtime/dashboard state uses `caches.Lighthouse()`

Do not reintroduce env-driven cache indirection like:
- `LIGHTHOUSE_INSPECT_CACHE`

Named accessors are the right abstraction.

### Request/Response Capture

Requests capture an `http_exchange` inspect event containing:
- method
- scheme
- host
- uri
- request headers
- request body
- response status
- response headers
- response body

The Request tab uses that event as the source of truth.

### Copy Semantics

- `Copy raw` and `Copy pretty` copy from the captured body values, not from the rendered preview HTML
- if the backend stored body is truncated, copied content will also be truncated

### Memory Usage

Do not show fake request memory usage.

`runtime.MemStats.Alloc` is process-level memory at a moment in time, not request-consumed memory.

That field was intentionally removed.

## Request/Response Bodies

JSON bodies use shared formatting from:
- `templates/internal/lighthouse/ui/src/lib/json-preview.ts`

Behavior:
- detect JSON if body starts with `{` or `[`
- pretty-print with 2-space indentation
- apply lightweight syntax highlighting
- non-JSON falls back to escaped text

Cache preview now reuses the same helper.

## Request Header UX

Current state:
- headers live inside Request and Response tabs, not in a separate Headers tab
- each header value has a per-value copy button
- copy toast uses the header key, e.g. `Content-Type copied`
- header sections are collapsible

Notes:
- header cards use the same dark surface as body cards
- key column was changed to content-sized layout instead of a wide fixed column

## Inspect Detail Header

This has been heavily iterated and is still the roughest product area.

Current goals:
- top row should feel like compact identity + quick facts
- use source-specific icons that match the sidebar route icons
- use semantic pills where useful

Current intent:
- title row:
  - source icon
  - inspect display name
  - right-side source/status badges
- pill row:
  - full copyable inspect id
  - method
  - primary label like `path` or `job_name`
  - status
  - started
  - duration
  - events
  - host
  - ip
  - remaining labels

The current implementation is in `InspectsView.vue`, but this area still needs product cleanup.

## Semantic Pill Coloring

The top summary strip should not be a rainbow, but it should surface meaningful state.

Current semantic intent:
- `duration`
  - latency-based color
- `method`
  - verb-based color
- `status`
  - status-class color
- other pills have lighter semantic treatment so the strip feels intentional

This work is still in progress in `InspectsView.vue`.

## Inspect Tabs

Request-based inspects have:
- `Timeline`
- `Request`
- `Response`

Tab selection is deep-linked through the querystring:
- `?inspect=<id>&tab=timeline`
- `?inspect=<id>&tab=request`
- `?inspect=<id>&tab=response`

If an inspect has no `http_exchange`, it falls back to `timeline`.

## Copy to Curl

`Copy to Curl` is now a plain action button in the tab row, shown only when the `Request` tab is active.

Important UX decisions already made:
- not a refresh-style control
- no fake refresh animation
- toast says `Curl command copied`

It was moved there because placing it deep in the request panel made it feel detached from the request-oriented views.

## Cache Browser State

There was a long-running bug where the cache browser could bounce between `store=default` and `store=inspects`.

Important lessons:
- querystring must be authoritative
- route state must drive refresh
- backend-selected store must not overwrite a query-owned `store`

There were also hydration issues where the selected store was not yet in the store options and the select drifted.

Fix direction that was taken:
- route query applied first
- synthetic temporary option if query-selected store is not yet in loaded options
- refresh reads requested store from route first

If this regresses, inspect:
- `templates/internal/lighthouse/ui/src/views/CacheView.vue`

## Body Truncation

Request and response bodies are truncated server-side at capture time in:
- `templates/internal/http/server.go.tmpl`

The logic currently truncates to `maxBodyBytes` and appends:
- `...[truncated]`

This means:
- preview and copy both reflect the stored truncated value
- frontend does not further truncate the copied body

If product needs exact replay for large requests, the backend capture limit is the thing to revisit.

## Validation Notes

The local Vite/frontend build has been unreliable in this environment due unresolved local toolchain resolution around `vite` and its plugins.

Do not assume frontend build validation happened unless it was explicitly run successfully.

For generated app validation, prefer rendering against a temp clone of a real app rather than mutating a live working app directly.

## Remaining Rough Edges

The inspect feature works, but these areas still need attention:
- top inspect detail header still needs more product shaping
- top pill ordering/semantic emphasis may still need refinement
- request header/body layout can still be tightened further
- timeline event presentation can still be made more compact and more intentional
- request/response capture limits should be revisited if replay needs full fidelity

## Current Dirty Files

At the time of writing, the likely in-progress files are:
- `templates/internal/http/server.go.tmpl`
- `templates/internal/lighthouse/ui/src/views/InspectsView.vue`

Check `git status` before assuming the tree is clean.

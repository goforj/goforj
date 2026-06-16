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
- job-specific payload tab:
  - `Payload`
- source-specific list headers and row summaries for:
  - `http`
  - `jobs`
  - `scheduler`
  - `cli`
- cache preview JSON beautification/highlighting
- copy actions for inspect id, headers, bodies, and request replay (`Copy to Curl`)

Auth/session debugging now intentionally shows up inside request inspects too:
- when debug logging is enabled, auth request failures and recoveries emit request-scoped debug log events
- this is how access-cookie expiry, refresh fallback, refresh-hash mismatches, and resulting `401` failures can be diagnosed from the inspect timeline itself

## Retention Model

Current design direction:

- source runtimes capture inspects locally in memory
- source runtimes publish finished inspect batches to Lighthouse
- Lighthouse is the retained recent-view owner

The main runtime controls are now:
- `LIGHTHOUSE_INSPECT_MAX_TOTAL`
  - Lighthouse-side retained recent inspect window
- `LIGHTHOUSE_INSPECT_MAX_INFLIGHT`
  - source-runtime in-memory protection
- `LIGHTHOUSE_INSPECT_MAX_EVENTS`
  - per-inspect payload cap
- `LIGHTHOUSE_INSPECT_SAMPLE_RATE`
  - source-runtime capture probability
- `LIGHTHOUSE_INSPECT_BUFFER_SIZE`
  - bounded outbound publish queue
- `LIGHTHOUSE_INSPECT_FLUSH_INTERVAL`
  - async publish cadence
- `LIGHTHOUSE_INSPECT_FLUSH_BATCH_SIZE`
  - async publish batch cap

The important product distinction is:
- source runtimes are not the long-term retained source of truth anymore
- Lighthouse owns finished inspect aggregation and recent browsing

## Main Files

Backend capture and store:
- `templates/internal/inspects/manager.go.tmpl`
- `templates/internal/http/server.go.tmpl`

Lighthouse UI:
- `templates/internal/lighthouse/ui/src/views/InspectsView.vue`
- `templates/internal/lighthouse/ui/src/lib/json-preview.ts`
- `templates/internal/lighthouse/ui/src/components/ui/tooltip/TooltipContent.vue`

Related generated/runtime glue:
- `templates/internal/http/lighthouse.go.tmpl`
- `templates/internal/jobs/lighthouse.go.tmpl`
- `templates/internal/schedules/lighthouse.go.tmpl`
- `templates/internal/lighthouse/inspects.go.tmpl`
- `templates/wire/inject_services_app.go.tmpl`
- `internal/generate/queues.go`

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

### Lighthouse Delivery

Current design direction is finished-inspect fan-in through Lighthouse:

- source runtimes keep capture local while an inspect is running
- on `Finish`, completed records are queued into a bounded async buffer
- batches are flushed to Lighthouse over the agent websocket
- Lighthouse ingests those finished records and retains the recent browse window

Current defaults:
- `LIGHTHOUSE_INSPECT_BUFFER_SIZE=4096`
- `LIGHTHOUSE_INSPECT_FLUSH_INTERVAL=1s`
- `LIGHTHOUSE_INSPECT_FLUSH_BATCH_SIZE=100`

Current drop behavior:
- if the publish buffer is full, new finished inspects are dropped
- if Lighthouse is unavailable, new finished inspects are dropped
- drop counters and flush metrics are emitted for Grafana/alerts

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

Job-based inspects now have:
- `Timeline`
- `Payload`

Tab selection is deep-linked through the querystring:
- `?inspect=<id>&tab=timeline`
- `?inspect=<id>&tab=request`
- `?inspect=<id>&tab=response`
- `?inspect=<id>&tab=payload`

If an inspect has no `http_exchange`, it falls back to `timeline`.

Important semantic rule:
- `Payload` is for the singular input of the job inspect being viewed
- it is not a dumping ground for child jobs that happened to be queued during that inspect

Queued child job payloads are shown inline on the relevant timeline annotation event instead.

## Copy to Curl

`Copy Request to Curl` is now a plain action button in the tab row, shown whenever the inspect has a captured request exchange.

Important UX decisions already made:
- not a refresh-style control
- no fake refresh animation
- toast says `Request copied as curl command`

It was moved there because placing it deep in the request panel made it feel detached from the request-oriented views.

Non-HTTP inspects do not show this action.

## Job Payload Semantics

There are now two distinct payload concepts:

1. Root job input
   - captured as `job_payload`
   - shown in the dedicated `Payload` tab for `jobs` inspects

2. Child job payload queued during another inspect
   - captured as `queued_job_payload`
   - shown inline on the parent inspect timeline
   - this is the right behavior for scheduler and parent-job timelines where multiple downstream jobs may be queued

The generator-side capture lives in:
- `internal/generate/queues.go`

The main reason for the split is product clarity:
- a `jobs` inspect should expose the one concrete input that job ran with
- a scheduler or parent inspect should expose downstream queued payloads inline with the enqueue event, not in a global payload tab

## Source-Specific List UX

The left inspect list is no longer treated as HTTP-only.

Current source-specific headers:
- `http`
  - `Method | Path | Duration | Time`
- `jobs`
  - `Kind | Job | Duration | Time`
- `scheduler`
  - `Kind | Schedule | Duration | Time`
- `cli`
  - `Type | Command | Duration | Time`

This work is intentionally product-facing, not just cosmetic. Non-HTTP inspects should stop reading like broken request tables.

## List Performance and Browsing Behavior

The left inspect list was reworked for large recent windows.

Current behavior:
- request list is virtualized/windowed
- inspect search text is precomputed per record
- selected inspect detail fetches are cached by `trace_id`
- new-arrival highlight is transient and fades
- selected-row styling takes precedence over new-arrival styling

Important browsing ergonomics:
- refresh no longer forces the list to jump back to the selected row if the user scrolled elsewhere
- when the selected inspect is offscreen, a `Scroll to selected` pill appears and fades away once selection is back in view
- scroll-to-top bubbles exist for the list and detail panes

## Live Refresh and Filters

The list supports live refresh directly in the filter area.

Current behavior:
- inline `Live 5s` toggle
- refresh every 5 seconds when enabled
- polling pauses when the tab is hidden
- polling resumes when the tab becomes visible again
- `Clear inputs` is an inline action, not a full extra control band

The intent is to keep the control surface compact without wasting vertical space.

## Timeline Rendering Notes

Timeline rendering went through a lot of cleanup.

Current intent:
- timeline rail and node sit on the timestamp/content divider
- duration stays on the far right for compact event rows
- metadata summary truncates cleanly instead of wrapping into junk layout
- noisy annotation payloads should not dump raw storage-shaped key/value blobs into the timeline

Specific special cases:
- `job_payload` timeline entries summarize the payload and point the operator to the `Payload` tab
- `queued_job_payload` timeline entries render inline payload content because that payload belongs to the enqueue event itself

If this area regresses, inspect:
- `templates/internal/lighthouse/ui/src/views/InspectsView.vue`

## Tooltip Theming

Lighthouse tooltip theming should match the darker card-style treatment used in uptime gopher.

The shared tooltip primitive now uses:
- `bg-card`
- `text-foreground`
- `border border-border`
- `shadow-sm`

instead of the older inverted white-on-dark treatment that rendered poorly in dark mode.

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
- non-HTTP detail headers still need more source-specific shaping beyond the left-list treatment
- scheduler and CLI input detail still need clearer product treatment comparable to job payload handling
- request/response capture limits should be revisited if replay needs full fidelity

## Current Dirty Files

At the time of writing, the likely in-progress files are:
- `templates/internal/http/server.go.tmpl`
- `templates/internal/lighthouse/ui/src/views/InspectsView.vue`

Check `git status` before assuming the tree is clean.

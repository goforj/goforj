# Repo Boundaries And Ownership

This document explains which repo should own a change before work starts.

## Repo Roles

### `goforj`

`goforj` is the framework, generator, and developer-workflow repo.

It owns:

- project rendering
- generated app templates
- `forj build`, `forj run`, `forj render`, `forj dev`
- generated app runtime conventions
- app-level env policy
- app-level lifecycle and bootstrap wiring
- demo app templates and Lighthouse UI

It should not absorb reusable web, queue, storage, or cache implementation details when those belong in sibling repos.

### `web`

`web` is the web abstraction repo.

It owns:

- `web.Context`, `web.Response`, `web.Router`
- route registration and route-list concepts
- middleware abstractions
- framework adapter logic in `adapter/echoweb`
- support packages such as:
  - `webmiddleware`
  - `webindex`
  - `webprometheus`

If a change can stay inside `web`, prefer that over pushing framework details into rendered apps.

### `storage` / `filesystem`

`storage` owns:

- storage abstraction shape
- driver implementations
- cross-driver contract behavior
- integration coverage for real backends
- docs/examples for storage capabilities

If the question is "should Lighthouse or the app be able to do this generically across storage drivers?", the answer often belongs in `storage` first.

### `cache`

`cache` owns:

- cache abstraction shape
- driver implementations
- cross-driver inspector/introspection behavior
- contract and integration coverage
- docs/examples for cache APIs

If the question is "should Lighthouse or the app be able to do this generically across cache drivers?", the answer often belongs in `cache` first.

### `queue`

`queue` owns:

- queue interfaces
- driver behavior
- Redis/Asynq pass-through behavior
- worker shutdown semantics inside the queue layer

If a bug is really about queue-driver consistency or backend-specific worker behavior, it probably belongs in `queue`, not `goforj`.

## Where Changes Belong

### Put the change in `goforj` when:

- it is app policy
- it affects generated app structure
- it is CLI/build/dev workflow behavior
- it is about templates or generators
- it is project-level env policy

Examples:

- `.env` conventions
- generated app lifecycle wiring
- `forj run` UX
- `forj dev` watcher/TUI behavior
- render-time local module replaces
- Lighthouse UX/state handling

### Put the change in `web` when:

- it is generic web runtime behavior
- it should be hidden behind the web abstraction
- it is route/middleware/telemetry functionality
- it is framework-adapter behavior

### Put the change in `queue` when:

- the queue driver itself is inconsistent
- shutdown semantics are wrong inside the driver
- backend-specific timeout/config behavior is incomplete

## Lighthouse Explorer Backlog

Likely next high-value explorer surfaces:

- config explorer
  - inspect effective config by component with safe redaction and source hints
- event explorer
  - browse recent emitted events and payloads
- mail explorer
  - inspect outbound mail messages and metadata when mail is enabled
- job payload explorer
  - inspect recent jobs, args, retries, and failures beyond simple queue health
- rate limit explorer
  - inspect buckets/keys if rate limiting becomes a first-class capability
- session explorer
  - inspect and revoke sessions if sessions become a first-class capability

Priority guidance:

- the strongest next additions after storage and cache are probably config, event, and mail
- do not add explorer surfaces just for parity; they should correspond to real operator debugging workflows

## Architectural Conclusions

### The `web` boundary is paying off

The strongest proof so far:

- Echo v5 was a major upstream break
- most of the migration stayed inside `web/adapter/echoweb`
- generated app shapes mostly did not need to change

That is exactly what the abstraction is for.

### `web` should own real primitives

`web` is not just a thin facade anymore.

It legitimately owns:

- request/response abstractions
- middleware surface
- telemetry packages
- route/indexing behavior

### Root runtime policy belongs near the root

App timeout and lifecycle policy should not be rediscovered by each primitive from env.

Resolve once, pass down as dependency, and keep runtime policy centralized.

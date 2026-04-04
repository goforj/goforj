# Primitive Design Suggestions

This note captures candidate first-class primitives that fit the current GoForj model:

- env-scoped configuration
- generated managers and typed accessors
- swappable local vs distributed drivers
- compile-time `*_SUPPORTED_DRIVERS` plus runtime `*_DRIVER`

## Strongest Candidate

### Lock

Why it fits:

- foundational infrastructure primitive
- clear local vs distributed split
- useful across jobs, scheduler, leader election, singleton work, dedupe, and coordination
- benefits from the same generated manager/accessor pattern as cache, storage, queue, and events

Why it should be separate from cache:

- cache is value storage
- lock is coordination and exclusivity
- cache correctness is usually best-effort
- lock correctness has much stricter semantics
- lock APIs are acquire/renew/release oriented, not read/write/delete

Suggested first backends:

- `memory`
- `redis`
- `postgres`

Why those first:

- `memory` for local development, tests, and single-process apps
- `redis` for common distributed lock use cases
- `postgres` for apps that already depend on a DB and want fewer external moving parts

Likely shape:

- `LOCK_SUPPORTED_DRIVERS`
- `LOCK_DRIVER`
- `LOCK_<NAME>_DRIVER`
- `app.Locks().Default()`
- `app.Locks().Scheduler()`

Possible API shape:

- `Acquire(ctx, key, ttl)`
- `Release(ctx, token)`
- `Refresh(ctx, token, ttl)`
- maybe `Run(ctx, key, ttl, func(...) error)`

Important caution:

- distributed lock guarantees need to be defined carefully
- this primitive is only worth shipping if the semantics are explicit and trustworthy

This is the most natural next primitive.

## Other Candidates

### Mail / Notifications

Why it could fit:

- apps often need swappable providers
- local/dev transport vs production provider split is common
- could support provider-specific backends while keeping app code stable

Recommended shape:

- a first-class `notifications` primitive
- one canonical notification message type for app code
- per-driver formatters/renderers that turn the canonical message into provider-specific payloads
- named instances for different delivery concerns such as `Transactional`, `Alerts`, or `Ops`

Core idea:

- app code should emit structured notification intent
- drivers should own transport and final formatting
- the primitive should feel closer to structured logging than handwritten provider payload assembly

Possible message shape:

```go
type Message struct {
	Title  string
	Body   string
	Fields map[string]any
	Level  string
	Tags   []string
	Meta   map[string]string
}
```

Possible usage:

```go
err := app.Notifications().Alerts().Send(ctx, notifications.Message{
	Title: "Monitor Down",
	Body:  "Primary API is not responding",
	Level: notifications.LevelError,
	Fields: map[string]any{
		"monitor": "api-primary",
		"region":  "us-east-1",
		"status":  503,
	},
})
```

Formatter model:

- Slack formatter can render fields as blocks
- email formatter can render HTML/text layouts
- SMS formatter can collapse the message into a short title/body form
- webhook formatter can emit the structured payload as JSON
- log formatter can emit the message as structured key/value output

Why this is the right model:

- gives app code one stable API
- centralizes presentation logic instead of scattering it through the app
- keeps driver-specific payload details out of business code
- makes local `log` or `null` drivers natural for development
- preserves room for branded or team-specific rendering rules

Likely configuration shape:

- `NOTIFY_SUPPORTED_DRIVERS`
- `NOTIFY_DRIVER`
- `NOTIFY_<NAME>_DRIVER`

Possible drivers:

- `log`
- `null`
- `smtp`
- `resend`
- `postmark`
- `ses`
- `twilio`
- `slack`
- `discord`
- `webhook`

Important caution:

- notifications are less uniform than cache, storage, queue, or lock
- email, SMS, chat, and webhook transports do not have identical capabilities
- the primitive should avoid pretending that every provider has the same payload model
- the canonical message should stay small and broadly portable
- provider-specific escape hatches should be limited and deliberate

Why it is lower priority:

- less foundational than `lock`
- feels more like an integration family than a core runtime primitive

### Search

Why it could fit:

- some apps may want local/sql-backed search in development and external engines in production
- driver model could work well if search becomes a first-class concern

Why it is lower priority:

- more domain-specific
- likely higher complexity than the current primitives

## Lower Confidence Ideas

### Blob / Object Store

Probably not needed yet because `storage` may already cover the important use cases.

Only worth introducing if:

- `storage` becomes too broad
- object/blob workflows need a materially different API shape

### KV / Config State

Probably not worth adding.

Reason:

- overlaps too much with cache, database, and storage
- likely muddies the primitive model instead of clarifying it

### Secrets

Interesting, but not a strong primitive candidate yet.

Reason:

- may belong closer to config/provider integration than runtime app infrastructure

## Current Recommendation

Priority order:

1. `lock`
2. `mail` / `notifications`
3. `search`

If GoForj adds one more foundational primitive next, `lock` is the best fit.

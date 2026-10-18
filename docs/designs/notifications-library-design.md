# Notifications Library And GoForj Integration Design

## Status

- Design status: proposed
- Planning date: 2026-09-06
- Target repositories: a new `github.com/goforj/notifications` sibling repository, optional driver modules, and `goforj`
- Primary sibling-library scope: notification intent, recipients, policy planning, route fanout, rendering, delivery results, idempotency, observations, and test support
- Primary GoForj scope: an optional Notifications component, generated routes and drivers, Queue and Mail integration, application delivery state, demo provider extraction, commands, documentation, and render coverage
- Replaces: `docs/designs/notifications-design-sketch.md`
- Cross-repository source of truth: this design is normative until the Notifications repository contains an accepted design or implementation plan that references it

## Summary

GoForj should add a reusable Notifications library and an optional generated component for delivering human-facing application messages across email, SMS, chat, webhook, push, and in-app channels.

Application code should express notification intent without constructing Slack blocks, SMTP messages, Twilio payloads, or webhook JSON:

```go
submission, err := notifier.Submit(ctx, notifications.Request{
	ID:         operationID,
	Route:      "ticket.updated",
	Recipients: []notifications.RecipientRef{{Type: "user", ID: userID}},
	Content: notifications.Content{
		Key: "tickets.assignment.changed",
		Values: []notifications.Value{
			notifications.String("ticket", ticketNumber),
			notifications.String("assignee", assigneeName),
		},
	},
})
```

The library should resolve a deliberate delivery plan, render once per recipient locale and channel, invoke channel drivers, and return structured outcomes. GoForj should compose application recipient resolution, preferences, Localization, Mail, Queue, persistence, metrics, and Testkit around that core.

Notifications are not Events with prettier payloads. Events announce domain facts to software subscribers. Notifications communicate selected facts to people or external destinations under recipient preferences, channel policy, localization, and delivery constraints. The application owns the decision that an event or command should produce a notification.

The current GoForj demo already contains substantial operational notification behavior and many provider adapters. That code is implementation evidence, not the reusable contract. The new library should extract portable mechanisms while leaving monitor-specific messages, database tables, secrets, and UI inside the demo application.

## Decision

Create a root `github.com/goforj/notifications` module with small optional driver or adapter modules where dependencies justify separate release surfaces. Integrate it into GoForj as the optional `Notifications` component.

Adopt these decisions:

1. A notification is human-facing delivery intent, not a domain event, queue job, audit record, or log entry.
2. The root library is GoForj-neutral and knows nothing about Auth, GORM, Web, Queue, Mail, Localization, Lighthouse, or generated applications.
3. Application code sends a bounded semantic request containing a stable route, recipients, content key, and typed values.
4. Stable notification IDs support retry-safe request identity. Reusing an ID with different canonical intent returns a conflict.
5. Notification identity is scoped by application and optional tenant at the GoForj integration boundary.
6. Recipient references are opaque application identities. The root library does not query an Auth user table.
7. Direct endpoints are accepted only through a trusted application resolver or an explicitly direct system route. Untrusted request data cannot select arbitrary webhook, email, or SMS destinations.
8. A policy planner resolves recipient preferences, allowed channels, locale, quiet-hours decision, and route behavior before drivers run.
9. The root library defines the planner contract but does not own application preference persistence.
10. Routes are semantic names such as `ticket.updated`, `billing.receipt`, or `ops.monitor_down`.
11. A route may use fanout, first-success fallback, or required-channel policy. The policy is explicit and directly tested.
12. Channel drivers receive channel-specific rendered payloads, not the application's semantic request.
13. Renderers own channel adaptation, bounds, and safe fallback. Drivers own transport and provider response classification.
14. The canonical request does not contain provider-specific payload objects or arbitrary `map[string]any` values.
15. The root API supports synchronous dispatch. Asynchronous execution is a GoForj Queue integration, not a hidden library goroutine.
16. A successful queue dispatch proves notification work was accepted, not that a provider delivered it.
17. Delivery is at-least-once unless a provider and configured idempotency strategy offer a stronger documented guarantee.
18. The library never claims exactly-once delivery across a database, queue, network provider, and callback.
19. Each delivery has a stable versioned attempt identity derived from application and tenant scope, notification ID, recipient, route, channel, selected driver, and destination identity.
20. Drivers propagate provider idempotency keys when supported and report whether that capability was used.
21. Results distinguish planned, suppressed, deferred, attempted, accepted, delivered when synchronously known, failed, and unknown outcomes.
22. Provider acceptance is not recipient delivery. The API and UI use those terms precisely.
23. Retryability is a typed driver classification. The library does not retry automatically inside `Send`.
24. Queue workers or application workflows own retry timing, backoff, and maximum attempts.
25. Quiet hours may suppress, allow, or defer. Deferral requires the application or Queue integration to persist and schedule work.
26. Email delivery delegates to `github.com/goforj/mail`. Notifications does not implement SMTP or email-provider APIs again.
27. Localization integrates through a narrow renderer seam and an explicit canonical recipient locale. The root module does not require a concrete Localization package.
28. In-app inbox delivery is an optional driver and store contract. Authorization of inbox reads remains application-owned.
29. Push delivery begins with a generic device-endpoint contract only after endpoint ownership and invalidation semantics are proven.
30. Webhook and chat endpoints are trusted configuration. V1 does not send to arbitrary URLs supplied in a notification request.
31. Delivery state may be recorded through an optional journal, but journaling does not strengthen network delivery guarantees dishonestly.
32. Secrets, message bodies, recipient endpoints, template values, and provider responses never enter ordinary errors, logs, metrics, or Lighthouse.
33. Observations contain bounded route, channel, driver, status, duration, attempt count, and safe error class only.
34. Fakes apply production validation and capture immutable request, plan, render, and result snapshots.
35. GoForj generated routes use typed accessors. Component-off applications contain no Notifications code or configuration.
36. Runtime route configuration may choose among drivers compiled into the application, but it cannot load arbitrary provider code.
37. The demo provider catalog should be extracted in reviewed groups rather than copied wholesale into v1.
38. V1 starts with log, webhook, chat-webhook, Mail, and fake drivers. SMS and provider-specific incident systems follow through provider packs.
39. Notification templates, recipient policy, and preferences are application data or code. Provider drivers do not own business wording.
40. Digests, campaigns, marketing automation, analytics, and inbound provider webhooks are outside v1.

## Why A Separate Library

### The reusable mechanism

Applications repeatedly need the same machinery:

- validate semantic message intent;
- resolve recipients and preferences;
- choose channels;
- render per locale and channel;
- fan out with explicit success policy;
- preserve idempotency identity;
- classify provider failures;
- expose safe delivery results; and
- capture deterministic tests.

These concerns are not specific to GoForj generation or to the monitoring demo. A sibling library creates one stable contract for API applications, workers, scheduled tasks, and other Go services.

### Why not use Mail for everything

Mail owns email composition and email transport. SMS, Slack, Discord, Teams, PagerDuty, generic webhooks, and in-app inboxes have different endpoint, payload, limit, and delivery behavior. Notifications coordinates those channels and delegates email execution to Mail.

### Why not publish Events directly

An event such as `TicketAssigned` is a durable or transient fact for code. It does not identify who should be contacted, whether they opted out, which locale to use, whether quiet hours apply, or how success is judged across channels.

An event handler may construct a notification request, but that mapping remains explicit application behavior. Notifications does not subscribe to every event automatically.

### Why not make Queue own it

Queue moves executable work through asynchronous infrastructure. It does not define human recipients, channel preference, rendering, or provider semantics. GoForj can enqueue a stable notification job whose handler invokes this library.

### Why not keep the demo implementation

The demo manager is specialized for monitoring channels and monitor alert fields. It dynamically constructs many providers from application tables and currently mixes route selection, provider construction, logging, and monitor-specific formatting.

The demo remains valuable evidence for provider details and operational behavior. Extraction must remove monitor-specific assumptions and adopt the root contracts, not move demo packages unchanged.

## Goals

1. Give application services one small, stable notification API.
2. Keep provider SDKs and payloads out of domain code.
3. Make recipient and channel policy explicit.
4. Support both application-user messages and operational destinations without conflating their resolution.
5. Provide honest partial-success and delivery semantics.
6. Support synchronous local behavior and deliberate queued execution.
7. Integrate cleanly with Mail, Localization, Queue, Auth, Audit, Metrics, Lighthouse, and Testkit.
8. Make provider failures classifiable without leaking responses or credentials.
9. Preserve retry-safe identities and expose provider idempotency support.
10. Extract proven demo providers through bounded compatibility-tested packs.

## Non-goals

1. A domain event bus.
2. A queue or workflow engine.
3. A marketing campaign platform.
4. Contact discovery or customer data management.
5. A complete user preference product.
6. A drag-and-drop template editor.
7. Analytics, open tracking, click tracking, or engagement scoring.
8. Guaranteed exactly-once delivery.
9. Guaranteed recipient receipt or attention.
10. Arbitrary user-provided webhooks.
11. Automatic notification generation from logs, database changes, or every event.
12. A replacement for Mail.
13. Provider-specific payload escape hatches in the core request.
14. Unlimited attachments or embedded file bytes.
15. Inbound SMS, email, or provider callback processing in v1.
16. Mobile device registration in v1.
17. Cross-tenant recipient lookup.
18. Legal consent management or jurisdiction-specific compliance claims.

## Terminology

### Notification request

Validated application intent to communicate semantic content to one or more recipients through a named route.

### Route

A stable semantic delivery policy selected by application code. It resolves to allowed channels and a success rule.

### Recipient reference

An opaque application-owned identity such as a user, team, account, or operational destination.

### Endpoint

A trusted resolved destination for one channel, such as a canonical email address, phone identity, inbox identity, or configured chat webhook.

### Plan

The immutable result of recipient and policy resolution, containing deliveries, suppressions, deferrals, locales, and a success rule.

### Delivery

One channel-specific attempt for one resolved endpoint.

### Renderer

An adapter that converts semantic content into a bounded channel-specific payload.

### Driver

A transport implementation that sends one channel-specific payload and returns a classified provider result.

### Journal

An optional record of request, plan, and attempt state containing only application-approved persisted fields.

## Repository Ownership

### `github.com/goforj/notifications`

The root sibling library owns:

- request, content, route, recipient, endpoint, plan, and result contracts;
- canonical validation and fingerprints;
- synchronous orchestration and route success policies;
- renderer, driver, planner, and journal interfaces;
- delivery and error classifications;
- context enrichment contracts;
- observer events and redaction boundaries;
- memory planner, log driver, and fake;
- conformance suites for planners, renderers, drivers, and journals; and
- package documentation and executable examples.

### Optional adapter and driver modules

Separate modules may own:

- Mail delivery through `github.com/goforj/mail`;
- generic webhook and chat-webhook transports through `httpx`;
- SMS provider packs;
- incident-management provider packs;
- in-app inbox persistence; and
- optional GoForj Localization rendering.

Module boundaries should follow dependency weight and release independence. A provider pack must not force every application to compile every provider SDK.

### `goforj`

The framework owns:

- optional Notifications component selection;
- generated route and compiled-driver configuration;
- App managers and named route accessors;
- trusted application and tenant scoping;
- recipient resolvers using Auth or application repositories;
- preference and quiet-hours adapters;
- Queue job and worker integration;
- Mail and Localization adapters;
- optional database journal migrations and repositories;
- metrics, Lighthouse, Audit summaries, and readiness;
- demo provider extraction and compatibility tests;
- Testkit fake composition;
- commands and generated documentation;
- render coverage; and
- dependency pins across all generated surfaces.

### Generated application

The application owns:

- which actions produce notifications;
- route names and success policies;
- recipient identity and authorization;
- preference, consent, and quiet-hours data;
- content keys, templates, and translations;
- application links and domain fields;
- direct versus queued execution choice;
- retry and retention policy;
- notification history authorization; and
- provider credentials and operational configuration.

## Core Invariants

1. Every request has exactly one application scope and at most one tenant scope.
2. Request IDs are unique only within that exact scope.
3. Reusing an ID with identical canonical intent returns the existing request classification.
4. Reusing an ID with different canonical intent returns `ErrConflict`.
5. All caller fields are validated and bounded before planner, renderer, driver, or journal invocation.
6. The canonical fingerprint excludes attempt time, provider response, and other execution-assigned fields.
7. A request contains at least one recipient and never expands past configured recipient and delivery limits.
8. Recipient references do not contain raw endpoints unless the route explicitly accepts a trusted direct endpoint type.
9. The planner cannot add a different application or tenant scope.
10. Every planned endpoint declares its trusted source and stable destination identity.
11. Raw endpoint values are available only to the selected driver and are absent from observations and ordinary errors.
12. Every delivery uses an immutable snapshot of plan and content.
13. A renderer is selected by channel and declared content capability before it receives values.
14. Rendered payloads have per-channel hard limits before driver invocation.
15. Drivers never receive unsupported payload types through runtime type assertions.
16. Route completion uses its declared success policy, not a generic `errors.Join` interpretation.
17. A partial result returns every bounded delivery outcome even when the route fails.
18. Cancellation stops new deliveries at the next safe boundary and marks unattempted deliveries honestly.
19. Cancellation never rewrites a provider-accepted result as definitely undelivered.
20. Retryable, permanent, throttled, unauthorized, and unknown outcomes remain distinct.
21. `Retry-After` is bounded and represented as a duration or time, never trusted as an arbitrary sleep command.
22. The core library starts no background lifecycle goroutines and performs no hidden retries. Synchronous fanout may use bounded, joined per-call goroutines under explicit limits.
23. Concurrent `Send` calls are safe when supplied collaborators satisfy their documented concurrency contracts.
24. Observers cannot change route outcomes and receive no content, endpoint, recipient, or provider body.
25. Driver errors contain stable safe classifications and preserve safe cancellation identities.
26. Raw provider bodies, URLs with credentials, tokens, phone numbers, and email addresses do not enter public unwrap chains.
27. A journal failure follows an explicit before-send or after-send policy and never creates a false delivery claim.
28. The fake and production dispatcher share request and plan validation.
29. Named routes and drivers remain distinct and deterministic.
30. Component-off GoForj applications contain no Notifications types, env keys, migrations, routes, or provider dependencies.

## Public API Shape

The exact names may change, but the capability and result boundaries should remain.

```go
package notifications

type Sender interface {
	Send(context.Context, Request) (Result, error)
}

type Submitter interface {
	Submit(context.Context, Request) (Submission, error)
}

type ClaimExecutor interface {
	ExecuteClaim(context.Context, Claim) (Result, error)
}

type BoundSender interface {
	Send(Request) (Result, error)
}

type Notifier interface {
	Sender
	WithContext(context.Context) BoundSender
}

type Submission struct {
	RequestID string
	State     SubmissionState
	Result    *Result
}

type Request struct {
	ID         string
	Route      RouteName
	Recipients []RecipientRef
	Content    Content
	Priority   Priority
	Tags       []string
}

type Content struct {
	Key      ContentKey
	Values   []Value
	Fallback *LiteralContent
}

type RecipientRef struct {
	Type string
	ID   string
}

type Scope struct {
	Application string
	Tenant      string
}
```

The root request does not expose App or tenant fields to ordinary callers. Every notifier is constructed with one immutable non-empty application scope and an explicit normalized tenant key. Scope enters canonical requests, planning inputs, fingerprints, and journal keys without relying on caller context.

### Construction

```go
notifier, err := notifications.NewScoped(
	notifications.Scope{Application: "support-api", Tenant: tenantID},
	planner,
	renderers,
	drivers,
	notifications.WithClock(clock),
	notifications.WithObserver(observer),
	notifications.WithJournal(journal),
)
```

Required collaborators fail during construction. The observer is optional and receives an explicit no-op implementation when omitted. A journal is optional only for non-durable synchronous operation. Queued construction and synchronous routes that claim durable convergence require a functional journal and fail during construction without one. Injected non-optional dependencies are never hidden behind nil guards.

`Sender.Send` always means synchronous planning and driver execution. `Submitter.Submit` is the application-facing mode-aware boundary: `SubmissionQueued` means Queue accepted versioned work, while `SubmissionCompleted` includes a synchronous `Result`. Neither state implies more provider or recipient evidence than it contains. Runtime configuration never changes the meaning of `Send`.

`ClaimExecutor.ExecuteClaim` is the narrow worker capability. It consumes a fenced journal claim, loads its validated replay material, resumes only the claim's authoritative nonterminal deliveries, records outcomes through that fence, and completes aggregate policy. GoForj Queue handlers do not orchestrate renderers or drivers themselves.

### Content

`Content.Key` is a bounded lowercase dotted identifier such as:

```text
tickets.assignment.changed
billing.receipt.ready
auth.password_reset.requested
ops.monitor.down
```

Values use a closed scalar set:

```go
notifications.String("ticket", ticketNumber)
notifications.Integer("attempt", attempt)
notifications.Decimal("amount", amount)
notifications.Boolean("urgent", urgent)
notifications.Time("occurred_at", occurredAt)
notifications.URL("ticket_url", trustedTicketURL)
```

There is no `map[string]any`, arbitrary struct serialization, raw HTML, or provider payload in the root request.

`Fallback` is optional reviewed literal text for routes that must remain usable without a template renderer. It is not generated from a content key and is subject to the same bounds. Routes may forbid literal content.

### Priority and tags

Priority is a closed portable hint such as `low`, `normal`, `high`, or `urgent`. A driver maps it only when the target supports an equivalent feature. It does not override preferences, quiet hours, or route policy unless the planner explicitly says so.

Tags are bounded stable application classifications. They are not arbitrary user labels and must not become unbounded metric dimensions.

## Planning And Recipient Resolution

### Planner contract

```go
type Planner interface {
	Plan(context.Context, PlanningRequest) (Plan, error)
}

type PlanningRequest struct {
	Scope   Scope
	Request CanonicalRequest
}

type Plan struct {
	Deliveries   []PlannedDelivery
	Suppressions []Suppression
	Deferrals    []Deferral
	Policy       SuccessPolicy
}
```

The planner resolves:

- recipient existence in the exact scope;
- endpoint ownership and verification;
- enabled channel preferences;
- route-level allowed and required channels;
- canonical recipient locale;
- quiet hours and urgent override policy;
- duplicate endpoints shared by multiple references; and
- delivery success policy.

The planner returns immutable values and never invokes a driver. Returned plans carry the same opaque scope identity as the input; a plan that changes or omits it is rejected before rendering.

### Recipient classes

V1 supports two deliberate classes:

1. Application recipients, resolved from opaque scoped references through application-owned repositories.
2. System destinations, resolved from trusted named configuration such as `on_call` or `billing_ops`.

The same request may not silently reinterpret a missing application recipient as a similarly named system destination.

### Endpoints

Illustrative internal endpoint shape:

```go
type Endpoint struct {
	Channel       Channel
	Driver        DriverName
	Identity      string
	Address       SecretAddress
	Verified      bool
	Locale        string
	Source        EndpointSource
}
```

`Driver` is selected and validated by route or destination configuration against the compiled driver manifest. `Identity` is a stable opaque fingerprint or application key suitable for idempotency. `Address` exposes the actual destination only through a narrow driver-facing accessor and redacts itself from formatting and errors.

Email and phone destinations must be verified when route policy requires it. Chat and webhook endpoints come from trusted system configuration or an application-owned verified integration record.

### Preferences and consent

The library models planner outcomes but does not prescribe database tables. A GoForj adapter may read:

- route opt-in or opt-out;
- allowed channels;
- locale;
- quiet-hours window and time zone;
- verified endpoints; and
- mandatory service-message policy.

Applications own the legal and product meaning of consent. The root library must not label a route legally transactional or marketing based only on its name.

### Quiet hours

Quiet hours require an explicit time zone. Locale is never used as a time-zone substitute. The planner returns:

- allowed now;
- suppressed;
- deferred until a canonical instant; or
- configuration error.

The synchronous dispatcher returns deferral without sleeping. Queue integration schedules the later attempt.

## Route Success Policies

V1 supports a closed set:

- `AllRequired`: every required delivery must reach `accepted` or `delivered`; optional failures remain in the result.
- `Any`: at least one delivery must reach `accepted` or `delivered`.
- `FirstSuccess`: ordered fallback stops after the first `accepted` or `delivered` outcome.
- `BestEffort`: attempts every delivery and returns a successful route result when planning succeeded, while preserving failures in the structured result.

Routes declare one policy during construction. A request cannot weaken it.

For `FirstSuccess`, permanent and retryable failure behavior is explicit. A retryable failure does not automatically advance to a less preferred channel unless route configuration permits that tradeoff. This prevents a transient email outage from unexpectedly sending SMS at significant cost.

An empty plan is not automatically successful. The planner must classify every recipient as suppressed, deferred, missing, or invalid, and the route declares whether complete suppression is an acceptable outcome.

Policy evaluation uses fixed outcome rules. `Accepted` and `Delivered` satisfy a delivery because some transports, including the Mail compatibility adapter, cannot synchronously prove delivery beyond acceptance. `Unknown`, retryable failure, and permanent failure do not satisfy it. Suppressed work is not a successful delivery and is acceptable only under the route's explicit complete-suppression rule. A deferral keeps the aggregate pending rather than converting it to success or failure. `BestEffort` is the deliberate exception: after all non-deferred attempts become terminal, planning success satisfies the route while each delivery outcome remains visible. Driver error values cannot redefine this matrix.

## Rendering Model

### Renderer contract

```go
type Renderer interface {
	Channel() Channel
	Render(context.Context, RenderRequest) (Payload, error)
}

type RendererFor[P Payload] interface {
	Channel() Channel
	Render(context.Context, RenderRequest) (P, error)
}

type RenderRequest struct {
	Content Content
	Context RenderContext
	Target  TargetCapabilities
}

type RenderContext struct {
	Locale         string
	TimeZone       string
	RouteRevision  string
	RenderRevision string
}
```

`RenderContext` carries the canonical recipient locale, a validated IANA time-zone name, and the route and renderer revisions pinned by the reservation. Locale never substitutes for time zone. Invalid or unavailable zones fail planning before a renderer sees the request.

Concrete payload types remain channel-owned and are registered with drivers through construction-time typed adapters. The root dispatcher does not cast arbitrary `any` values at delivery time.

Renderers own:

- content-key lookup;
- recipient-locale selection through an injected adapter;
- title and body structure;
- field labels and ordering;
- channel-specific truncation;
- safe link presentation;
- plain-text fallback; and
- capability negotiation.

Drivers own:

- authentication;
- endpoint construction;
- HTTP, SMTP, or SDK calls;
- provider timeout and response parsing;
- provider idempotency headers or keys;
- safe provider error classification; and
- no presentation policy beyond unavoidable transport encoding.

### Channel capabilities

Capabilities are explicit, for example:

- title;
- plain text;
- HTML;
- structured fields;
- actions;
- media references;
- maximum bytes;
- maximum action count; and
- collapse or thread key.

A renderer either produces a valid payload for the target or returns a stable unsupported-content error. It does not silently drop required content.

### Localization

The root `Renderer` accepts a canonical locale string and content contract. A separate adapter may use `github.com/goforj/localize`, application templates, or another engine. This avoids a hard root-module dependency while keeping per-recipient localization explicit.

One request may render differently for two recipients. Rendered content is never shared across recipients unless locale, channel, target capabilities, and every non-secret semantic value are identical under a safe bounded cache key.

### Attachments and media

V1 content may carry bounded application-owned media references only if a renderer and route explicitly support them. The root library never accepts file bytes or opens paths. Mail attachment behavior remains Mail-owned, and other drivers resolve approved Storage references through application adapters.

## Driver Contract

```go
type Driver[P Payload] interface {
	Name() DriverName
	Channel() Channel
	Capabilities() Capabilities
	Deliver(context.Context, Delivery[P]) (DriverResult, error)
}

type ChannelAdapter[P Payload] struct {
	Renderer RendererFor[P]
	Driver   Driver[P]
}
```

`ChannelAdapter` is the only registration unit. Its shared payload type proves at construction that the renderer output is accepted by the driver. The registry may erase that type behind a private closure after registration, but neither runtime dispatch nor the public application request casts arbitrary `any` values. Incompatible pairs fail to compile rather than surfacing after a notification has been planned.

`DriverResult` includes:

- provider acceptance identifier when safe to retain;
- outcome classification;
- whether provider idempotency was used;
- bounded retry-after guidance;
- safe provider class; and
- provider timestamp when trustworthy.

Provider response bodies and endpoint addresses never appear in the public result.

### Driver conformance

Every driver passes a common suite covering:

- context cancellation and deadlines;
- timeout behavior;
- idempotency propagation;
- retryable, throttled, unauthorized, permanent, and unknown errors;
- response-body bounds;
- redirect policy;
- credential and endpoint redaction;
- malformed success responses;
- concurrent use; and
- cancellation while the generated owner is preparing shutdown.

## Idempotency And Delivery Guarantees

### Request identity

A caller-supplied request ID must accompany stable semantic content. The canonical fingerprint includes the route name and revision, content definition and renderer revision, recipients, content key, typed values, priority, and stable tags. It excludes execution times, plan order where the policy declares order irrelevant, provider results, and attempt counters.

The same ID and fingerprint converge. A changed fingerprint returns `ErrConflict` before a new driver attempt when the configured journal can prove prior identity. Without a journal, the process-local dispatcher can validate only concurrent in-memory duplicates and must not claim durable convergence.

Every reservation pins a versioned route snapshot and renderer revision before planning. Retry, deferred recovery, and replay use those pinned revisions even if later configuration changes channel order, drivers, templates, or policy. Implementations may retain immutable compiled snapshots or reconstruct them from a versioned application reference, but they must fail explicitly when the pinned revision is unavailable.

### Attempt identity

Each planned delivery receives a deterministic derivation-versioned attempt base identity containing normalized application and tenant scope, request ID, recipient identity, route, channel, selected driver or provider class, and destination identity. Retries add a bounded attempt number while retaining the base identity. Drivers that support idempotency receive the base or attempt key according to their documented provider semantics. The derivation version is persisted so algorithm changes cannot collide with existing provider keys.

### Honest guarantees

There is an unavoidable uncertainty window when a provider accepts a request but the process fails before recording the result. The next worker may retry. The library reports an unknown prior outcome and uses provider idempotency where available.

Documentation must distinguish:

- queued;
- attempted;
- accepted by provider;
- synchronously confirmed delivered;
- later callback-confirmed delivered; and
- read or acknowledged by a recipient.

V1 does not implement callback confirmation.

## Journal And Persistence

The optional `Journal` records bounded request and delivery state and owns the reservation and fencing protocol used by synchronous and queued execution:

```go
type Journal interface {
	ReserveSend(context.Context, CanonicalRequest, ReplayMaterial) (Reservation, error)
	ReserveEnqueue(context.Context, CanonicalRequest, ReplayMaterial) (Reservation, error)
	MarkEnqueued(context.Context, EnqueueLease, EnqueueReceipt) error
	CancelEnqueue(context.Context, EnqueueLease) error
	Claim(context.Context, ReservationToken, LeaseRequest) (Claim, error)
	Renew(context.Context, ClaimToken) (ClaimToken, error)
	Defer(context.Context, ClaimToken, DeferralRecord) (DeferralToken, error)
	ClaimDueDeferrals(context.Context, DueRequest) ([]DeferralClaim, error)
	MarkDeferralEnqueued(context.Context, DeferralClaim, EnqueueReceipt) error
	RecordPlan(context.Context, ClaimToken, PlanSummary) error
	RecordAttempt(context.Context, ClaimToken, AttemptRecord) error
	Complete(context.Context, ClaimToken, CompletionRecord) error
}
```

`ReserveSend` returns a stable opaque `ReservationToken` without creating an enqueue lease. `ReserveEnqueue` returns the same stable identity plus a short-lived opaque `EnqueueLease`. The versioned job carries the stable token before enqueue status is marked. An enqueue lease expires and can be reacquired by an identical submission. If Queue accepted work but the process died before `MarkEnqueued`, duplicate jobs still carry the same reservation token and worker claims fence execution. `EnqueueReceipt` is bounded transport-neutral proof of backend acceptance, not merely a generated correlation or dispatch ID. A claim contains a monotonically fenced generation, lease expiry, and the authoritative list of nonterminal deliveries. Every mutating journal operation verifies that generation so an expired worker cannot record or complete over its successor.

`Defer` atomically records the canonical wake time and releases the active execution claim. Due deferrals are reacquired through fenced bounded claims. The generated recovery runner dispatches the stable reservation token through the same classified enqueue protocol and marks it enqueued only after `Accepted`. `Rejected` releases the due claim for bounded retry, while `Indeterminate` preserves it for reconciliation. A crash before or after delayed enqueue is recovered without creating a second delivery identity. Concurrent recovery runners cannot own the same deferral fence, and repeated quiet-hours evaluation may create a later versioned deferral without losing attempt state.

Queued submission uses an adapter outcome with exactly three states: `Accepted` includes backend acceptance proof, `Rejected` proves the backend did not accept work, and `Indeterminate` means acceptance cannot be established either way. A correlation ID alone is not proof. Notifications requires an updated Queue contract that exposes this classification as a prerequisite for generated queued mode. The currently pinned Queue API cannot provide it reliably and must be upgraded across every module and fixture pin before this integration ships.

Snapshot-at-enqueue routes follow this protocol:

1. validate and canonicalize the scoped request;
2. pin route and renderer revisions and plan against current endpoints, locale, time zone, preferences, and quiet-hours policy;
3. atomically reserve durable identity, the immutable plan snapshot, and a bounded enqueue lease;
4. enqueue only the opaque reservation token in a versioned Queue job;
5. mark the reservation enqueued idempotently only for `Accepted`, even if inline execution already claimed or completed it;
6. cancel only the enqueue lease for `Rejected`, and preserve the reservation for reconciliation after `Indeterminate`;
7. let the worker claim the reservation before rendering or delivery;
8. renew bounded leases only while actively working;
9. record each terminal delivery independently; and
10. complete or defer the aggregate only after route policy can be evaluated.

Delivery-time preference routes instead reserve the canonical request, pinned revisions, and versioned authoritative references before enqueue. Their worker claims first, rehydrates and fingerprint-checks those references, then plans against delivery-time state before rendering. Both modes use the same classified enqueue outcome rules.

An identical already-completed reservation returns its prior bounded completion without enqueueing or delivering again. An active enqueued reservation returns the existing submission state. A pending enqueue whose lease expires may be reacquired and enqueued by an identical submission, recovering a crash or failed cancellation. A recoverable expired worker claim may be taken by a new worker at a higher fence generation.

Queue retries and process restarts resume only nonterminal deliveries. Accepted or otherwise terminal delivery A is never invoked again merely because delivery B failed retryably. Unknown outcomes follow driver idempotency and route policy explicitly; the journal cannot promote them to unsent.

Durable synchronous sending follows a parallel protocol: canonicalize, `ReserveSend`, claim the stable reservation, execute under the fence, record each outcome, and complete. An identical completed request returns its prior bounded completion. An active identical claim returns a stable in-progress outcome rather than sending concurrently. A conflicting fingerprint fails before a driver call. Synchronous and queued calls arbitrate through the same scoped reservation identity, so only the winning claim can execute. A journal failure before a driver call fails closed; a failure after a driver call follows the unknown-recording policy below.

The memory journal supports tests and explicitly process-local development only. It may back queued execution only when the Queue also cannot outlive that process and configuration acknowledges ephemeral loss. A production Queue or any Queue with durable jobs requires a durable journal. A GoForj SQL journal is generated when queued delivery, delivery history, or durable idempotency is selected.

Every queued route declares one durable replay strategy at construction:

- an encrypted canonical snapshot supplied through a configured codec and key policy; or
- a versioned application reference whose rehydrator reconstructs and revalidates semantic values, recipient timing policy, and scope.

Queued route construction fails without one. For reference replay, the claim executor canonicalizes the reconstructed intent and compares it with the reserved fingerprint before planning or rendering. Any difference is replay drift or corruption and fails closed without a driver call. Snapshot-at-enqueue endpoint, locale, time-zone, preference, and quiet-hours decisions are part of the reserved immutable plan. Delivery-time preference routes store recipient references and re-resolve authoritative state. The opaque Queue job token alone is never treated as sufficient render input.

Persisted content policy is explicit:

- request fingerprint and content key are retained;
- recipient and endpoint identities are opaque scoped identifiers;
- raw endpoint addresses are omitted or separately encrypted only by application policy;
- rendered bodies and values are omitted from ordinary journal columns, while required replay material is encrypted or represented by a versioned resolver reference;
- provider IDs are retained only when safe and useful; and
- retention is configured independently from Audit.

Journal writes before delivery may fail closed for routes requiring durable identity. Writes after provider acceptance cannot make the provider action atomic. A post-send journal failure returns a structured unknown-recording state while preserving the known provider outcome in memory. Recovery treats that delivery as unknown, not safely retryable, unless provider idempotency proves replay safety.

Generated migrations never imply legal delivery proof. Audit may record that notification dispatch was requested or classified, but it must not copy message bodies or claim recipient receipt.

## Error Model

Stable sentinels should include:

- `ErrInvalidRequest`
- `ErrInvalidRecipient`
- `ErrInvalidContent`
- `ErrUnknownRoute`
- `ErrConflict`
- `ErrNoDelivery`
- `ErrSuppressed`
- `ErrDeferred`
- `ErrRender`
- `ErrUnsupportedContent`
- `ErrDriverUnavailable`
- `ErrUnauthorizedProvider`
- `ErrThrottled`
- `ErrPermanent`
- `ErrOutcomeUnknown`
- `ErrJournalUnavailable`
- `ErrClosed`

Typed errors carry safe route, channel, driver, status, and retry classification. They do not contain recipient IDs, endpoints, content values, rendered payloads, provider URLs, credentials, or raw provider responses.

Errors preserve `context.Canceled` and `context.DeadlineExceeded` through `errors.Is`. Raw driver causes are delivered only to an explicitly private diagnostic hook when safe operational debugging requires them.

`Result` remains available for partial outcomes. The error answers whether the declared route policy succeeded, while the result explains every bounded planned delivery.

## Observation Model

```go
type Observation struct {
	Operation  Operation
	Route      RouteName
	Channel    Channel
	Driver     DriverName
	Status     Status
	Duration   time.Duration
	Count      int
	ErrorClass string
}
```

Observers receive no recipient reference, endpoint, locale input, content key when the application classifies it as sensitive, content value, rendered body, provider ID, or response body. Route, channel, and driver labels come from bounded generated catalogs.

The observer is non-blocking by contract. GoForj adapters use bounded buffers and report drops. Observer panics are recovered and cannot change delivery results.

## Fake And Test Support

The root repository provides:

- `notifications.NewMemoryPlanner` for explicit routes and endpoints;
- `notifications.NewMemoryJournal` for local behavior and conformance;
- `notifications.NewLogDriver` with redacted structured output; and
- `notificationsfake.New` for controlled plans, driver outcomes, and immutable capture.

Suggested assertions:

```go
notificationsfake.RequireRequest(t, fake, "ticket.updated")
notificationsfake.RequireDelivery(t, fake, notifications.ChannelEmail)
notificationsfake.RequireSuppressed(t, fake, "ticket.updated")
```

The fake applies normal validation and success policies. It distinguishes capture from execution and never reports a captured request as provider-delivered.

Failure output includes route, channel, safe status, and counts. It omits recipient IDs and values by default. Tests may inspect immutable snapshots explicitly when their data is known safe.

## GoForj Component Model

### Selection and dependencies

`Notifications` is an optional App component.

Rules:

- Notifications requires no database for synchronous configured system routes.
- a durable journal requires Database.
- queued delivery requires Queue and a functional journal; durable Queue requires a durable journal.
- email channel support requires Mail.
- localized templates require a renderer, commonly the optional Localization component.
- Auth can provide application-recipient resolution but is not required for system destinations.
- Notifications does not require Events, Cache, Storage, Audit, Metrics, Lighthouse, Web API, or Web UI.
- each App selects Notifications independently.

GoForj component validation reports missing dependency edges before generation. It does not inject nil drivers or silently disable selected channels.

### Generated configuration

Illustrative project configuration:

```yaml
apps:
  api:
    components: [notifications, jobs, mail, localization]
    notifications:
      execution: queued
      journal: database
      routes:
        ticket.updated:
          policy: all_required
          channels:
            - {name: in_app, driver: inbox}
            - {name: email, driver: mail}
        ops.monitor_down:
          policy: any
          destinations: [on_call]
          channels:
            - {name: slack, driver: slack}
            - {name: pagerduty, driver: pagerduty}
```

Driver compilation is explicit:

```yaml
notifications:
  compiled_drivers: [log, webhook, slack, pagerduty, inbox, mail]
```

The generated project manifest is the build-time source of truth for compiled drivers. Environment variables configure credentials and endpoints only; they cannot change the binary's driver set. Provider credentials use driver-scoped environment contracts. Generated `.env.example` files contain names and safe placeholders, never live secrets.

Runtime configuration chooses only drivers compiled into the binary. `channels` is an ordered list of logical route channels, and each entry maps its name to one explicit compatible driver. There is no ambiguous global driver default for a multi-channel route. Two drivers for the same transport class require distinct channel names and deterministic list order. Route changes that add channels, change drivers, weaken required delivery, or change recipient classes are operational behavior changes and should appear in configuration plans.

### Generated manager

Generated `internal/notifications.Manager` owns:

- scoped root notifier construction;
- named route accessors;
- compiled renderer and driver registries;
- application recipient and system destination resolution;
- preference and quiet-hours adapters;
- Queue handler registration when selected;
- bounded deferral recovery runner when queued routes can defer;
- Mail and Localization adapters;
- optional SQL journal;
- metrics and Lighthouse observers;
- readiness and close behavior; and
- maintenance access kept outside the ordinary sender.

Generated App access is typed:

```go
func (a *App) Notifications() *notifications.Manager
```

For a multi-tenant App, the generated manager exposes an application-domain boundary such as `ForTenant(tenants.ID) (*TenantNotifications, error)`. The tenant identifier comes from server-authoritative application state and is normalized before the manager constructs lightweight scoped route submitters over shared manager-owned resources. A tenant is never inferred from request content or an unverified context value. Single-tenant Apps expose routes directly through their fixed generated scope.

Application services should normally depend on `notifications.Submitter` or an application-local narrow route interface rather than the generated manager. Code that explicitly requires synchronous provider execution may depend on `notifications.Sender` and cannot be wired to a queued route.

Named route access may look like:

```go
tenantNotifications, err := app.Notifications().ForTenant(tenantID)
if err != nil {
	return err
}
tenantNotifications.TicketUpdated().Submit(ctx, request)
app.Notifications().OpsMonitorDown().Submit(ctx, request)
```

The exact route accessor generation should avoid APIs that require callers to repeat the route inside the request.

### Execution modes

Synchronous submission invokes the root dispatcher before returning and yields `SubmissionCompleted` with a result. It is useful for local development, tests, and explicitly synchronous system routes. The lower-level `Sender.Send` is always synchronous regardless of route deployment configuration.

Queued mode:

1. validates and canonicalizes the scoped request;
2. plans and reserves an immutable snapshot for snapshot routes, or reserves versioned references for delivery-time routes;
3. dispatches a versioned job containing only the opaque reservation token;
4. marks enqueue only after classified `Accepted`, cancels only the enqueue lease after classified `Rejected`, and preserves ambiguous acceptance for reconciliation;
5. returns `SubmissionQueued` only after classified Queue acceptance;
6. claims and fences the reservation in the worker;
7. uses the pinned snapshot or resolves delivery-time policy according to route configuration; and
8. resumes only nonterminal deliveries before aggregate completion.

Preference timing is explicit. Transactional messages may snapshot verified endpoints and locale at enqueue time. Preference-sensitive messages may resolve at delivery time. Queue payloads never contain raw credentials or provider secrets.

The queue job has bounded attempts and uses Queue's retry facilities. Notifications does not create a second retry loop. A job retry reloads journal state and never repeats terminal channel deliveries.

Before each driver call, the claim executor renews the lease and verifies that the remaining lease exceeds the configured driver timeout plus a safety margin. Driver timeouts are bounded below the maximum lease duration. The root starts no background renewal goroutine. If a driver violates cancellation or crosses lease expiry, the stale worker cannot record through its old fence; the outcome is unknown and replay follows provider idempotency policy.

### Database journal

When selected, GoForj generates tables for notification requests, plans, and attempts. Schema design must resolve:

- application and tenant scope;
- scope-local request identity;
- opaque recipient and endpoint identities;
- canonical request fingerprint;
- state transitions;
- attempt numbering;
- provider idempotency use;
- unknown outcomes;
- retention; and
- concurrent worker claims.

The journal is not an inbox. An in-app channel uses separate inbox records and authorization.

### In-app inbox

An optional inbox driver may persist:

- scoped recipient identity;
- content key and approved render data or a rendered safe projection;
- created and expiry times;
- read or acknowledged time;
- action links approved by the application; and
- notification request identity.

The root inbox contract supports write and recipient-scoped query behavior. GoForj does not generate a generic public inbox endpoint without application-owned authorization and DTO projection.

Read state is not provider delivery state. A user may receive the same semantic request through inbox and email with independent outcomes.

## Framework Integrations

### Mail

The Notifications email adapter constructs an ordinary Mail message from a rendered email payload and calls the selected Mail manager. It reuses Mail's sender policy, header validation, HTML/text behavior, preview, fake, metrics, and providers.

Notifications does not expose SMTP or email provider settings. The current Mail contract returns only an error. A nil error therefore maps conservatively to provider acceptance with provider ID absent and idempotency capability unknown; it never maps to delivered. Typed Mail failures map only when their stable classification proves the mapping. Optional richer Mail result and capability interfaces may be added compatibly later, while legacy error-only mailers remain supported.

### Localization

The generated renderer resolves one canonical locale per planned recipient and uses stable content keys plus typed values. Localization fallback diagnostics remain available without copying text into Notifications observations.

Locale does not imply time zone. Quiet hours and localized time formatting receive an explicit recipient time zone from policy.

### Queue

Queued execution uses a versioned application job and the normal generated Queue manager. The worker records notification attempt results and respects cancellation and shutdown.

Queue uniqueness or retry configuration does not replace notification request identity. Both layers retain distinct contracts and tests.

### Events

Applications may subscribe to reviewed domain events and create notification requests. The integration is generated only for explicit application mappings. There is no catch-all event subscriber.

Notification results may emit application events only through an explicit adapter after result persistence. The root library does not publish events automatically.

### Auth and tenancy

Auth integration resolves canonical active users and verified endpoints inside the exact application and tenant scope. A user ID from another tenant returns a non-enumerating missing-recipient outcome.

Account deactivation, endpoint verification, locale, and preferences use server-authoritative state. Request payloads cannot assert them.

### Audit

Applications may audit that a sensitive notification was requested, suppressed, or accepted by a provider. Audit records use categorical route and outcome values and omit recipient endpoints, bodies, template values, and provider responses.

Audit failure policy is application-owned. Notifications does not silently turn an Audit failure into delivery success or vice versa.

### Testkit

Application Testkit injects a fresh canonical Notifications fake and exposes typed route and delivery assertions. Default mode captures planned notification intent without contacting providers. An explicit execution mode runs renderers and selected local drivers.

Captured request does not imply provider acceptance. Testkit diagnostics follow the root fake's redaction rules.

## Demo Provider Extraction

The current demo includes a wide provider catalog. Extraction should happen in groups:

### Phase A: foundational

- log;
- generic webhook;
- Mail adapter;
- fake; and
- Slack-compatible webhook rendering.

### Phase B: common chat

- Slack;
- Discord;
- Teams;
- Google Chat;
- Telegram; and
- generic webhook-compatible chat providers.

### Phase C: incident systems

- PagerDuty;
- Opsgenie;
- Jira Service Management;
- ntfy; and
- Gotify.

### Phase D: SMS and regional packs

- Twilio and other SMS providers;
- WeCom;
- DingTalk;
- Feishu;
- LINE; and
- WhatsApp-compatible providers.

Each extraction must identify the provider's real authentication, payload, timeout, rate-limit, idempotency, redirect, and error semantics. Provider names in the demo catalog do not prove tested support.

The demo migrates to the public library only after parity tests prove its monitor alert behavior and configuration migration. Application database configuration remains demo-owned unless a reusable provider-config contract is deliberately added.

### Demo compatibility projection

The first demo migration preserves its current operational behavior deliberately:

- its default monitor-alert route remains best effort;
- a nil legacy provider error continues to display the legacy `delivered` label through a UI projection, while new journal state records the more accurate `accepted` classification;
- historical `alert_dispatch_events` rows remain readable through dual-read projection or an explicit backfill that never invents provider delivery evidence;
- raw historical error text is retained under existing database access policy but new writes store a safe error class and a separately access-controlled redacted diagnostic;
- current database-owned channel configuration remains the source for demo system destinations during migration; and
- the Queue handler continues its legacy no-retry behavior until a separately reviewed route-policy change enables typed retries.

Changing the demo to retry provider failures is a distinct operational rollout. It requires per-delivery journal resumption, cost analysis for SMS and incident providers, and a migration note. Library adoption alone does not enable it.

## Security Model

1. Every request, plan, journal operation, and inbox query is application and tenant scoped.
2. Recipient references are bounded and resolved server-side.
3. Direct endpoints require trusted configuration or a verified application resolver.
4. Request input cannot select arbitrary network destinations.
5. Generic webhook drivers default to HTTPS and a strict redirect policy.
6. Webhook transports resolve and validate every A and AAAA address at dial time, reject mixed public and disallowed private answers, pin the validated address for that connection while preserving TLS server name, and repeat policy for every redirect.
7. Loopback, link-local, metadata, multicast, unspecified, and private ranges are denied by default for IPv4 and IPv6. Applications that permit private-network webhooks use explicit destination and CIDR allowlists.
8. Proxy use is disabled by default for restricted webhook delivery or validated under an equally explicit proxy destination policy. DNS cache and connection reuse cannot bypass destination revalidation after policy expiry.
9. Driver configuration separates secret and non-secret values.
10. URLs redact user information, query values, fragments, and token-shaped paths in diagnostics.
11. HTTP response bodies are bounded before reading and never exposed publicly.
12. Email addresses and phone numbers are treated as secrets in errors and observations.
13. Rendered payloads and semantic values are untrusted for their destination context.
14. Channel renderers use provider-safe encoding rather than concatenating payload JSON.
15. HTML email still passes through Mail's HTML and header policy.
16. Action URLs allow only application-approved schemes and origins unless a route explicitly permits external links.
17. Journal and inbox retention are explicit and independently authorized.
18. Notification history and inbox routes require application-owned authorization.
19. Provider callbacks, when later added, require signature verification and replay protection.
20. Metrics labels come only from bounded generated route, channel, driver, and status catalogs.
21. Test failures redact recipients, endpoints, values, content, credentials, and provider bodies.
22. Logs and Lighthouse receive classifications, not message content.

## Concurrency And Performance

- Notifiers, immutable plans, renderers, and drivers document concurrent-use safety.
- Fanout concurrency is bounded per request and globally by application configuration.
- Sequential execution remains available for ordered fallback routes.
- No API creates one goroutine per unbounded recipient.
- Recipient and delivery counts have hard limits.
- Rendered payload size is bounded before network calls.
- Provider response reads are bounded.
- Queued bulk delivery uses application batches with explicit continuation, not one enormous request.
- Shared render caching never retains secret endpoint data or unbounded user values.
- Journal indexes match scoped request identity, pending attempts, recipient history, and retention.

Benchmarks should measure validation, plan evaluation, rendering, bounded fanout, fake capture, and journal transitions. Network benchmarks remain driver-specific and do not become hard CI gates.

## Readiness And Lifecycle

The root notifier owns no background lifecycle and borrows constructed planners, renderers, drivers, and journals. It exposes no `Close`. The generated manager owns those resources, registers exactly one lifecycle hook for each distinct instance even when several routes share it, and closes them after in-flight sends drain.

Queued deferral enables a manager-owned bounded recovery runner. Startup waits for journal schema readiness, Queue readiness, and handler registration before starting the runner. Each manager instance may poll, while fenced `ClaimDueDeferrals` provides distributed single-owner coordination for each due item without assuming one process. The runner has bounded batches and concurrency, reports its last successful scan and oldest due age, and stops producing jobs before Queue shutdown begins.

GoForj readiness distinguishes:

- configuration ready;
- journal schema ready;
- Queue worker registered;
- deferral recovery runner started and scanning when queued deferral is enabled;
- Mail dependency ready;
- driver locally constructible; and
- remote provider reachable.

Remote provider health is not probed on every readiness request. Many providers lack safe health endpoints, and sending a test notification is not a readiness check. Operational status reports last attempt classifications without bodies or endpoints.

Shutdown stops accepting new notification work, stops and joins the deferral recovery runner, stops and drains the Queue worker through Queue lifecycle ownership, drains bounded in-flight synchronous sends, then closes manager-owned distinct resources in reverse construction order. A runner claim racing shutdown is released or left recoverable by lease expiry and never enqueues after Queue shutdown starts. Constructor rollback uses the same ownership stack. No root notifier or route wrapper closes a borrowed driver.

## Commands

Potential generated commands:

- `notifications:routes` lists route names, policies, channels, and execution mode without endpoints or secrets.
- `notifications:status` reports local composition and bounded recent classifications.
- `notifications:test --route ops.monitor_down --destination on_call` sends only to a named trusted system destination and requires explicit confirmation outside local environments.
- `notifications:prune` applies journal or inbox retention through a separate maintenance capability.

There is no generic command that accepts an arbitrary URL, email address, phone number, provider payload, or template values from an untrusted shell invocation.

## Error And Failure Presentation

User-facing application errors should state whether notification work was queued, suppressed by preference, deferred, partially attempted, or unavailable. They should not expose which private endpoints a recipient owns.

Operational diagnostics may include safe route, channel, provider class, request correlation, attempt number, duration, and retry classification. Private diagnostic hooks remain access-controlled and still redact credentials and message bodies.

Provider outages should not automatically fail the domain mutation after it commits. Applications decide whether notification acceptance is required inside their transaction, normally through an outbox or same-database queued-intent pattern. The root synchronous API cannot create cross-system atomicity.

## Compatibility

### Root API

Public request, plan, result, error, renderer, and driver contracts follow semantic versioning. Adding a new driver does not itself break consumers. Changing route policy or result meaning can change runtime behavior even without a Go API change.

### Configuration

Route channel additions, fallback order changes, mandatory-channel changes, execution-mode changes, and provider substitutions are operational behavior changes. Generated plans must show them.

### Persisted data

Request fingerprints, job payload versions, journal states, inbox records, and attempt identities are persisted contracts. Migrations require explicit backward-reading or rollout rules.

### Demo migration

Replacing demo-owned notification code must preserve existing channel configuration, monitor route selection, and supported provider behavior or provide a concrete migration. The generic library does not justify silently renaming environment keys or database fields.

### Minimum Go version

Each sibling module chooses the minimum Go version required by its implementation and dependencies. GoForj does not raise its minimum merely for API ergonomics that can be expressed compatibly.

## Testing Strategy

### Root contract tests

Cover:

- request validation and bounds;
- zero-scope rejection, scope-local identity across applications and tenants, and fingerprint conflicts;
- planner attempts to omit or change scope;
- recipient and delivery limits;
- trusted endpoint resolution;
- preference suppression and quiet-hours deferral;
- route success policy matrix across accepted, delivered, unknown, failed, suppressed, and deferred outcomes independently from driver errors;
- empty plans;
- cancellation before, during, and after provider acceptance;
- partial results;
- retry and throttle classification;
- observer drop, panic, and redaction;
- journal before-send and after-send failures;
- process-restart duplicate convergence with a durable journal;
- reservation, first claim, renewal, fencing, lease expiry, crash recovery, and enqueue-failure cancellation;
- process death between reservation and enqueue, cancellation failure, and concurrent resubmission after enqueue-lease expiry;
- concurrent durable synchronous sends and synchronous-versus-queued submissions with the same scoped request identity;
- duplicate Queue delivery and concurrent worker claims;
- claimed execution with a driver crossing lease expiry and a competing fenced worker;
- queued restart using encrypted snapshot and versioned-reference replay strategies with non-empty values;
- replay after configuration changes using pinned route and renderer revisions, including explicit failure when a pinned revision is unavailable;
- reference replay whose backing values or recipients drift from the reserved fingerprint without invoking a driver;
- deferred delivery creation, concurrent due claims, crash recovery before and after enqueue acceptance, and repeated quiet-hours deferral;
- classified Queue adapter outcomes for shutdown, backend rejection, duplicate acceptance, proven acceptance with a diagnostic, and indeterminate legacy responses;
- snapshot and delivery-time routes whose endpoint, locale, time zone, and preferences change between submission and execution;
- deferral recovery startup, distributed fenced claims, readiness, bounded scans, stop-before-Queue ordering, and shutdown races;
- partial fanout retry where terminal delivery A is not repeated after delivery B fails;
- concurrent sends and bounded joined per-call fanout without background lifecycle goroutines;
- defensive copies; and
- fake parity.

### Renderer tests

Cover each channel's required capabilities, output limits, Unicode behavior, action URL policy, locale and validated time-zone selection, fallback, and destination-safe encoding. Construction tests prove compatible typed renderer-driver adapters register and incompatible payload pairings do not compile. Golden files are reviewed application fixtures, not opaque snapshots updated automatically.

### Driver tests

Use local test servers and provider-owned official sandboxes only in explicit integration lanes. Cover authentication placement, idempotency, redirects, timeouts, rate limits, malformed responses, bounded bodies, and redaction.

### GoForj render tests

Render under `/tmp` and cover:

- component off;
- synchronous log-only App;
- queued delivery;
- SQL journal;
- Mail email delivery;
- Localization per-recipient rendering;
- Auth recipient resolution;
- in-app inbox;
- Audit integration;
- default and named Apps;
- each selected provider pack;
- demo legacy status projection, historical rows, database channel configuration, partial failures, and unchanged initial Queue retry counts;
- rerender stability; and
- largest supported composition.

Configuration coverage includes historical mapping input migration, canonical component-sequence output using `jobs`, dependency validation, marshal and rerender stability, and component-off removal.

### Behavioral tests

One representative application proves:

- a domain event maps explicitly to one notification request;
- two recipients with different preferences and locales receive different plans;
- quiet hours defer without sleeping;
- queued acceptance differs from provider acceptance;
- email uses Mail rather than a duplicate transport;
- partial route success is represented honestly;
- retryable failures use Queue policy only once;
- duplicate request IDs converge after restart;
- cross-tenant recipient references reveal no endpoint;
- two tenants may concurrently use the same request and recipient IDs without identity collision;
- Audit and observations omit message content; and
- Testkit capture never claims delivery.

API contract tests prove `Send` always means synchronous execution, `Submit` reports queued versus completed mode explicitly, and queued acceptance cannot satisfy assertions about provider acceptance.

Mail adapter tests cover every supported Mail driver and fake, including nil-error acceptance, absent provider IDs, unknown idempotency capability, typed and legacy errors, throttling where exposed, cancellation, and malformed provider success handled inside Mail.

### Security tests

Verify:

- arbitrary webhook URLs cannot enter through requests;
- DNS rebinding, mixed public/private answers, IPv6 special ranges, redirect-to-private, and proxy behavior obey destination policy;
- redirects cannot leak credentials;
- provider bodies and endpoint values do not reach errors;
- template values cannot inject provider payload structure;
- action URLs reject dangerous schemes;
- journal and inbox queries remain scope-bound;
- metrics have bounded labels; and
- fakes and failure diffs redact sensitive data.

Configuration tests cover simultaneous channels, two named drivers for one channel, per-App mappings, incompatible driver/channel selection, runtime configuration against the generated compiled-driver manifest, build-time inclusion of Mail when the `mail` driver is selected, queued execution without a journal failing before render, durable Queue rejecting a memory journal, and generated queued mode rejecting a Queue version without classified acceptance.

## Documentation

The sibling README should explain:

- notifications versus events, queue, mail, logs, and audit;
- semantic requests and routes;
- recipients, endpoints, preferences, and quiet hours;
- route success policies;
- rendering and drivers;
- synchronous and queued execution;
- idempotency and honest delivery guarantees;
- errors, observations, and redaction;
- journals and inboxes; and
- fakes and contract suites.

Generated documentation should explain:

- how to add a route;
- how to select execution and journal modes;
- how to configure trusted destinations;
- how application recipients resolve;
- how Mail and Localization integrate;
- how retries and deferrals work;
- how to inspect safe outcomes;
- how retention works;
- what provider acceptance proves; and
- how to test without contacting providers.

## Delivery Plan

### Phase 0: extraction inventory

1. Inventory the existing Notifications sketch and every demo provider.
2. Classify monitor-specific, reusable, secret, persistence, and UI behavior.
3. Inventory Mail, Queue, Auth, HTTP client, metrics, Lighthouse, and lifecycle seams.
4. Capture demo behavior and configuration compatibility tests before extraction.
5. Specify and release Queue's accepted, rejected, and indeterminate dispatch contract, then update every GoForj module and fixture pin.
6. Decide exact module boundaries and release order.

### Phase 1: root contracts

1. Create the sibling repository.
2. Implement request, route, recipient, endpoint, plan, result, error, and observer types.
3. Implement strict validation, canonical fingerprints, and route success policies.
4. Implement memory planner, memory journal, log driver, and fake.
5. Publish planner, renderer, driver, and journal conformance suites.
6. Add executable examples and generated API documentation.

### Phase 2: foundational adapters

1. Add generic webhook with strict URL, redirect, timeout, and response policies.
2. Add Slack-compatible webhook rendering.
3. Add the Mail adapter without duplicating transport.
4. Add the optional Localization renderer adapter.
5. Prove idempotency and error classification against local servers.

### Phase 3: GoForj component

1. Add component selection, dependency validation, and route configuration.
2. Add generated manager, typed accessors, and scoped recipient resolution.
3. Add synchronous and Queue execution modes.
4. Add optional SQL journal and retention maintenance.
5. Add metrics, Lighthouse, Audit, and Testkit integration.

### Phase 4: inbox and provider packs

1. Add the optional in-app inbox contract and generated persistence.
2. Extract common chat providers with parity tests.
3. Extract incident providers with provider-specific conformance.
4. Add SMS and regional packs only with maintained integration coverage.
5. Migrate the demo through an explicit compatibility plan.

### Phase 5: release

1. Inventory every root, adapter, driver, fixture, integration, and generated dependency pin.
2. Release sibling modules independently using repository scripts.
3. Verify every module tag and checksum availability.
4. Integrate published versions into GoForj.
5. Run `GOWORK=off` validation so local replacements cannot hide missing releases.

## Acceptance Criteria

The design is implemented only when:

- application code expresses bounded semantic notification intent without provider payloads;
- every notifier has immutable non-empty application scope and explicit tenant scope;
- recipient endpoints are resolved from trusted scoped state;
- preferences, allowed channels, locale, and quiet hours produce an immutable explicit plan;
- reservations pin route and renderer revisions, and render context carries a validated recipient time zone independently from locale;
- route fanout and success policy are deterministic and directly tested;
- renderers and drivers have separate contracts, compile-time typed adapter pairing, and conformance suites;
- Mail owns email transport and Localization remains an optional renderer adapter;
- synchronous and queued execution do not create duplicate retry loops;
- `Send` remains synchronous while `Submit` distinguishes queued acceptance from completed dispatch;
- generated queued mode depends on explicit accepted, rejected, and indeterminate Queue outcomes rather than treating a dispatch ID as acceptance proof;
- multi-tenant generated access constructs tenant-scoped route submitters from server-authoritative application identifiers;
- request identity and conflicts survive restart when durable journaling is selected;
- reservation, fenced claims, enqueue receipt semantics, deferred recovery, lease recovery, and per-delivery terminal state prevent duplicate completed-channel sends;
- queued, attempted, accepted, delivered, unknown, suppressed, and deferred states remain distinct;
- partial results are returned without leaking recipients, endpoints, or message content;
- raw provider errors and responses do not enter public errors or observations;
- arbitrary request-supplied network destinations are impossible;
- journal and inbox persistence are scope-bound and have explicit retention;
- Testkit capture is deterministic and never claims provider delivery;
- demo provider extraction preserves supported behavior through parity tests;
- initial demo adoption preserves historical status projection and Queue retry behavior explicitly;
- component-off renders contain no Notifications surface;
- default, named, minimal, and largest compositions render under `/tmp`;
- all module and dependency pin surfaces are independently released and validated; and
- documentation states delivery guarantees and cross-system atomicity honestly.

## Risks And Mitigations

### Risk: one message model becomes the lowest common denominator

Mitigation: keep semantic content small, use typed values, declare channel capabilities, and let renderers create channel-specific payloads.

### Risk: Notifications duplicates Mail

Mitigation: require the email adapter to invoke Mail and keep all SMTP, provider, header, preview, and attachment behavior there.

### Risk: Notifications duplicates Queue

Mitigation: provide synchronous dispatch only in the root and use Queue for scheduling, retry, and workers.

### Risk: provider acceptance is presented as delivery

Mitigation: use precise result states and reserve delivered for evidence that supports it.

### Risk: endpoint resolution creates an SSRF or privacy boundary failure

Mitigation: resolve trusted endpoints server-side, forbid request URLs, constrain redirects, and redact every diagnostic surface.

### Risk: preferences become an inflexible framework schema

Mitigation: define planner contracts and outcomes while leaving application preference persistence and legal meaning application-owned.

### Risk: the demo provider set creates an unmaintainable matrix

Mitigation: extract in provider packs only with conformance, integration ownership, and real maintenance demand.

### Risk: durable state implies exactly-once delivery

Mitigation: document the provider acceptance uncertainty window and preserve unknown outcomes.

### Risk: notifications leak sensitive content through observability

Mitigation: keep observations categorical and test errors, logs, metrics, Lighthouse, Audit, and fake diffs adversarially.

## Deferred Questions

1. Whether mobile push endpoint registration belongs in a future Notifications adapter or a separate device library.
2. Whether digest aggregation is a route policy or an application workflow.
3. Whether callback-confirmed delivery deserves a generic receipt contract.
4. Whether provider configuration should remain generated environment state or support a reusable encrypted repository.
5. Whether a rich portable action and media model can avoid lowest-common-denominator behavior.
6. Whether large campaign-style fanout belongs in a separate bulk delivery library.
7. Whether inbox read-state events should integrate with Events by default or remain application-owned.

The v1 intent, planning, rendering, driver, result, idempotency, and framework integration contracts should not wait on these questions.

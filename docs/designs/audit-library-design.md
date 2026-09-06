# Audit Library And GoForj Integration Design

## Status

- Design status: proposed
- Planning date: 2026-09-05
- Target repositories: a new `github.com/goforj/audit` sibling repository and `goforj`
- Primary sibling-library scope: validated audit records, append and query contracts, transaction binding, retention primitives, observers, an in-memory fake, and a GORM-backed driver
- Primary GoForj scope: the optional Audit component, generated database persistence, App composition, actor and correlation enrichment, lifecycle, metrics, Lighthouse inspection, commands, documentation, and render coverage
- Cross-repository source of truth: this design is normative until the Audit repository contains an accepted design or implementation plan that references it

## Summary

GoForj should add a reusable application audit trail library backed by an optional generated Audit component.

Audit records answer durable product and security questions:

- who changed a ticket's assignee;
- what administrator disabled an account;
- when a share link was revoked;
- whether an export succeeded or failed;
- which application and request initiated a sensitive change; and
- what bounded, deliberately selected fields changed.

This is not another event bus and not another logger. Events notify code that something happened. Logs help operators diagnose software. Audit records are durable, application-owned evidence intended to be queried and shown to authorized people.

The reusable library should provide a small record model, strict validation, explicit context enrichment, append-only application APIs, stable query semantics, transaction-aware store binding, safe retention operations, observer hooks, and test support. It should not infer audit records from database changes, subscribe to every application event, serialize arbitrary objects, or claim compliance and non-repudiation guarantees it cannot prove.

GoForj should integrate Audit with its existing database rather than introduce another service-shaped driver manager. The generated App should expose a default recorder and a deliberate same-database transaction binding for mutations that must commit atomically with their audit record.

## Decision

Create a single root module, `github.com/goforj/audit`, plus an optional GORM driver module only if dependency isolation requires it. Integrate it into GoForj as the optional `Audit` App component.

Adopt these decisions:

1. Audit records are durable application data, not transient messages or diagnostic log lines.
2. The public write API is append-only. It exposes no update operation.
3. The root library owns contracts and validation but knows nothing about GoForj, auth, HTTP, GORM, Wire, metrics, or Lighthouse.
4. An entry names an action, outcome, actor, resource, timestamps, correlation identifiers, and deliberately selected details. It never accepts an arbitrary application object for automatic serialization.
5. Audit values are bounded JSON scalar values. Nested arbitrary payloads, request bodies, response bodies, credentials, and model snapshots are outside the default contract.
6. The application explicitly chooses what to record. The library never observes all events, ORM hooks, or HTTP requests automatically.
7. Audit writes return errors. The caller explicitly decides whether a write failure aborts the business operation; the library never silently downgrades a required record to best effort.
8. Operations requiring atomic evidence bind the recorder to the same database transaction as the business mutation.
9. A record describes a committed transition only when it is appended inside that transaction. A post-commit write must not present itself as atomic.
10. Record identifiers are immutable and unique within `(application, tenant)` scope. Callers may supply a stable scope-local identifier for retry-safe append behavior; otherwise the recorder creates one using injected cryptographic entropy and time.
11. A caller-supplied identifier requires an explicit occurred time so a retry cannot acquire different semantic content from a later clock read.
12. Reusing a record identifier with byte-equivalent canonical intent returns the existing record. Reusing it with different content returns a conflict. Recorder-assigned append-attempt time is not part of the intent fingerprint.
13. Query order is stable by `(recorded_at, id)`, with opaque cursors bound to a normalized filter fingerprint.
14. Authorization is not part of the root query API. GoForj does not generate a public audit endpoint because application policy must scope every query.
15. Tenant and application scope are ordinary indexed record fields, not implicit database session state.
16. The root library provides redaction primitives and strict bounds, but it cannot infer which domain fields are sensitive. Applications remain responsible for selecting safe data.
17. Retention is explicit and separately authorized through a maintenance interface that is not present on the normal recorder or reader.
18. V1 is append-only at the application API and can detect conflicting duplicate identifiers. It does not claim storage immutability, tamper-proofing, legal non-repudiation, or compliance certification.
19. Optional integrity sealing may be added only after its concurrency, partitioning, key rotation, deletion, backup, and verification semantics are designed and tested across supported databases.
20. Audit observer events contain operation summaries and classifications, never full audit details or change values.
21. GoForj's Audit component requires a database and reuses a named generated database connection. It does not add `AUDIT_DRIVER` or provision separate infrastructure in v1.
22. Auth, starter applications, and generated resources may record selected audit actions when Audit is enabled, but Audit does not require Auth, Events, Queue, Mail, Storage, Cache, Metrics, or Lighthouse.
23. The generated default is fail-closed for explicitly invoked `Record` calls: errors reach the caller. There is no global environment switch that makes mandatory writes disappear.
24. Tenant-aware recorders fail before persistence when tenant scope is absent. Missing tenant context never falls back to the tenantless partition.
25. Every retention batch appends one bounded maintenance summary in the same transaction as its deletions.

## Why This Is A Separate Library

### Audit versus events

`github.com/goforj/events` publishes typed Go events to local or distributed subscribers. Its contract is fan-out. Most drivers are intentionally non-durable or expose only partial durability, and the library does not provide product-facing history queries.

Audit instead provides:

- durable append semantics;
- stable record identity;
- actor and affected-resource identity;
- explicit success, failure, or denial outcomes;
- bounded change descriptions;
- chronological queries;
- retention controls; and
- authorization-sensitive presentation data.

An application may publish an event and append an audit record for the same domain operation, but one is not a substitute for the other. The audit record should normally be written at the mutation boundary. Publishing an event and later deriving an audit record from a subscriber creates a consistency gap and loses the application context best known at the mutation site.

### Audit versus logs

Application logs are operational telemetry. They may be sampled, rotated, reformatted, shipped to an external backend, or accessible only to operators. Audit history is product data with explicit access rules and retention policy.

OWASP explicitly notes that process monitoring, audit trails, and transaction trails are often collected for different purposes and should be kept separate. Its application logging guidance also emphasizes recording when, where, who, and what while excluding or masking secrets and sensitive personal data. The Audit design follows those principles without presenting OWASP guidance as a compliance certification:

- [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)

OpenTelemetry's stable log data model informs correlation fields such as event time, observed time, trace ID, span ID, and event name. Audit retains its own product contract rather than pretending an OTLP log backend is a queryable application audit store:

- [OpenTelemetry Logs Data Model](https://opentelemetry.io/docs/specs/otel/logs/data-model/)

### Audit versus database history tables

Database triggers and temporal tables can capture row changes, but they commonly lack the authenticated application actor, request intent, denial outcome, and domain-level action. They can also record implementation details that are unsafe or meaningless to product users.

Audit operates at the domain mutation boundary. Applications may still use database-native history for separate forensic or recovery needs.

### Audit versus Lighthouse inspection

Lighthouse inspection is bounded operational diagnostics with its own retention and redaction posture. It is not authoritative product history. An audit record may carry the same request or trace identifier, allowing authorized operators to correlate the two systems without copying the audit body into Lighthouse.

## Motivation

### Application activity history is already recurring

The current sample applications independently ask for append-only history:

- PhotoDrop records uploads, tagging, sharing, expiration, restoration, and deletion.
- PocketDesk records ticket creation, assignment, status changes, comments, and administrative actions.
- Gather needs organizer actions around publication, cancellation, waitlists, and roster exports.

Those requirements currently encourage each application to invent a table, actor representation, pagination scheme, redaction policy, and test fake.

### Security-sensitive behavior needs application context

Infrastructure access logs cannot reliably reconstruct:

- the domain action the user intended;
- the resource that was affected;
- whether authorization denied the action;
- which fields the application deliberately changed;
- whether an administrator acted on behalf of another user; or
- whether a background process, API token, or human initiated the operation.

The domain service knows these facts and should record them explicitly.

### A shared contract improves tests and presentation

A reusable record model allows:

- application services to use one recorder interface;
- Testkit to capture and assert entries;
- generated metrics to classify append outcomes consistently;
- product UIs to paginate stable records;
- Lighthouse to link an audit append to an execution without storing its content; and
- retention commands to behave consistently across generated database dialects.

## Goals

1. Make durable application audit history straightforward to record correctly.
2. Preserve clear separation from events, logs, metrics, and database change capture.
3. Capture actor, action, resource, outcome, time, and correlation through a stable transport-neutral contract.
4. Make sensitive-data selection explicit and place conservative bounds on every caller-controlled field.
5. Support atomic business mutation plus audit append when both use the same database transaction.
6. Provide deterministic ordering and cursor pagination across supported stores.
7. Make duplicate retries visible and safe through immutable caller-supplied record identifiers.
8. Support authorized retention without exposing deletion on the ordinary application API.
9. Provide an in-memory fake and reusable store conformance suite.
10. Fit default and named GoForj Apps without provisioning a new infrastructure service.
11. Preserve context cancellation, request correlation, trace correlation, metrics, and inspection.
12. Keep the root package useful outside GoForj.

## Non-goals

1. A general logging facade or OpenTelemetry Logs SDK replacement.
2. An event bus, outbox, queue, scheduler, webhook system, or change-data-capture pipeline.
3. Automatic auditing of every route, ORM write, SQL statement, event, or command.
4. Reflection-based before-and-after model diffing.
5. Storing arbitrary request bodies, response bodies, model snapshots, headers, or stack traces.
6. A SIEM, anomaly detector, compliance reporting product, or legal hold system.
7. A guarantee of legal non-repudiation, WORM storage, or protection from a database administrator.
8. Transparent cross-database atomicity.
9. Generic row-level authorization or a generated public audit browser.
10. A distributed global ordering guarantee.
11. Blockchain storage or a cryptographic ledger in v1.
12. Automatic event publication after append.
13. A domain activity feed with comments, reactions, visibility groups, or notification preferences.
14. Full object versioning or restoration.
15. Automatic retention enforcement without an operator-installed command or schedule.
16. User analytics or behavioral tracking.

## Terminology

### Entry

Caller-supplied intent to append an audit record. It contains no recorder-assigned `RecordedAt` value.

### Record

The immutable, validated representation returned after a successful append. It includes its identifier and recorded time.

### Actor

The human, service, API client, scheduled task, or system process responsible for an action.

### Resource

The primary domain object affected by the action. A record may include bounded related-resource references as metadata, but it has exactly one primary resource.

### Action

A stable application-defined identifier such as `ticket.assigned`, `user.disabled`, or `album.shared`.

### Outcome

Whether the attempted action succeeded, failed, was denied, or was cancelled.

### Change

A deliberately selected scalar before-and-after value associated with a stable field name.

### Occurred time

When the domain action occurred. For ordinary in-process recording it defaults to the recorder's current time.

### Recorded time

When the recorder began the canonical append attempt. It is assigned immediately before store invocation and cannot be supplied as authoritative by an untrusted client. It is not a database commit timestamp.

### Scope

Application and optional tenant identity used for isolation and querying.

### Maintainer

A separately constructed policy-bearing capability for retention deletion or integrity verification. It carries scope, tenant, actor, clock, and validation rules and is not reachable through the normal recorder or reader interfaces.

## Repository Ownership

### `github.com/goforj/audit`

The sibling library owns:

- entry, record, actor, resource, change, outcome, query, cursor, and page contracts;
- validation and normalization;
- identifier, time, and canonical record rules;
- recorder, reader, store, transaction-binding, maintainer, and lower-level retention-store interfaces;
- context enrichment contracts;
- redaction helpers and sensitivity markers;
- observer events;
- an in-memory store and capture fake;
- reusable store conformance tests;
- library documentation and executable examples; and
- compatibility guarantees for persisted canonical values.

### Optional Audit driver module

If keeping GORM out of the root module requires a nested module, `github.com/goforj/audit/driver/gormaudit` owns:

- GORM-backed append and query behavior;
- SQLite, MySQL, and PostgreSQL dialect compatibility;
- binding to an existing `*gorm.DB`, including transaction handles;
- duplicate-ID conflict detection;
- stable cursor queries;
- retention deletion primitives; and
- real-database conformance coverage.

The driver does not run schema migrations automatically.

### `goforj`

The framework owns:

- the optional Audit component and its database dependency rule;
- generated audit table migrations for every supported database;
- generated `internal/audits` composition and named database selection;
- trusted actor, App, tenant, request, execution, trace, and span enrichment adapters;
- a transaction-bound recorder seam for generated GORM connections;
- metrics and Lighthouse observer adapters;
- lifecycle and readiness integration;
- retention and verification command presentation where supported;
- Auth and demo integration examples;
- Testkit capture integration;
- environment contracts and generated README material;
- render, migration, integration, and largest-composition coverage; and
- released dependency pins for every relevant module surface.

### Generated application

The application owns:

- action names and constants;
- the exact domain operations that require audit history;
- which actor and resource identifiers are safe to retain;
- which changes and metadata fields are safe and useful;
- authorization for audit queries and exports;
- tenant scoping policy;
- whether an audit failure aborts a particular business operation;
- retention periods and legal requirements;
- transaction placement for atomic records;
- UI projection and display labels; and
- direct tests for required audit paths and redaction.

## Core Invariants

1. A successful append produces exactly one immutable record identity.
2. Within one application/tenant scope, a record identifier never refers to different canonical content. The same identifier in another scope is independent and reveals no conflict oracle.
3. `Record` never returns success before the selected store has accepted the append.
4. A cancelled context stops work at the next safe boundary and never yields a successful record without a committed append.
5. The normal API cannot update or delete an existing record.
6. All caller-controlled fields are validated and bounded before store invocation.
7. Unknown outcomes and malformed actor, resource, action, scope, or change fields fail before persistence.
8. The library never serializes arbitrary structs into details or changes.
9. Caller inputs and returned records are defensively copied where mutable containers exist.
10. Recorded timestamps are UTC and retain sufficient precision for the selected database.
11. Stable pagination uses recorded time plus record ID as a total order.
12. A cursor is accepted only with the normalized query filter for which it was created.
13. Query results cannot escape the requested App and tenant scope through a cursor.
14. A transaction-bound recorder uses the supplied transactional store for every operation; it never falls back to the root connection.
15. A store error is returned with a stable sanitized classification and only library-owned public unwrap values. Raw driver causes go solely to an explicitly private diagnostic hook.
16. Observer failure never changes append or query behavior.
17. Observer payloads contain no audit change values, display names, summaries, or arbitrary metadata.
18. Retention deletion requires the maintenance capability, one exact scope, an authenticated maintenance actor, and an explicit exclusive cutoff.
19. Each retention batch deletes rows and appends its maintenance summary atomically.
20. In-memory and SQL stores satisfy the same append, conflict, ordering, cursor, cancellation, and retention contract.
21. GoForj never starts a second database solely for Audit.

## Public API Shape

The exact names may change during implementation, but the capability split should remain.

```go
package audit

type API interface {
	Recorder
	Reader
	WithContext(context.Context) BoundAPI
}

type Recorder interface {
	Record(context.Context, Entry) (AppendResult, error)
}

type Reader interface {
	Find(context.Context, string) (Record, error)
	Query(context.Context, Query) (Page, error)
}

type BoundAPI interface {
	Record(Entry) (AppendResult, error)
	Find(id string) (Record, error)
	Query(Query) (Page, error)
}

type AppendResult struct {
	Record      Record
	Disposition AppendDisposition
}

type AppendDisposition string

const (
	AppendInserted  AppendDisposition = "inserted"
	AppendDuplicate AppendDisposition = "duplicate"
)

type Observer interface {
	TryObserve(Observation) bool
}

type Store interface {
	Append(context.Context, Record) (AppendResult, error)
	Find(context.Context, Scope, string) (Record, error)
	Query(context.Context, Query) (Page, error)
}

type Maintainer interface {
	Prune(context.Context, PruneRequest) (PruneResult, error)
}

type RetentionStore interface {
	PruneBatch(context.Context, PrunePlan, Record) (PruneResult, error)
}
```

`Recorder` and `Reader` keep context on each operation so a narrow injected capability remains narrow. The full `API` also offers the established GoForj `WithContext` convenience through a separate `BoundAPI` wrapper; calling it does not widen a recorder-only dependency. Implementations normalize a nil context to `context.Background()` but do not add nil guards for required injected stores, clocks, entropy readers, or observers.

`AppendResult` reports whether this call inserted the canonical record or converged on an identical record already present in the same scope. A conflict never returns a successful result. The disposition is part of the store conformance contract so duplicate metrics and observations do not infer outcomes from driver-specific errors.

The normal `API` intentionally omits maintenance operations. `NewMaintainer` separately constructs a `Maintainer` with the same application, tenant-required policy, validation limits, and clock policy as the recorder. It validates the authenticated actor and request, fixes one operation timestamp, constructs the canonical maintenance summary, and delegates an atomic delete-plus-summary operation to `RetentionStore`. `RetentionStore` is a driver-facing implementation contract, not a capability handed to application services.

### Construction

```go
recorder, err := audit.New(store,
	audit.WithApplication("support-api"),
	audit.WithClock(clock),
	audit.WithEntropy(rand.Reader),
	audit.WithObserver(observer),
)
```

Required collaborators fail during construction. There is no package-global default recorder.

### Recording

```go
result, err := recorder.Record(ctx, audit.Entry{
	ID:      operationID,
	Action:  "ticket.assigned",
	Outcome: audit.OutcomeSucceeded,
	Actor: audit.Actor{
		Type: "user",
		ID:   actorID,
	},
	Resource: audit.Resource{
		Type: "ticket",
		ID:   ticketID,
	},
	Changes: []audit.Change{
		audit.StringChange("assignee_id", oldAssigneeID, newAssigneeID),
	},
})
```

Expected output should be consumed structurally rather than printed from examples containing identifiers. `result.Record` includes the canonical ID, `OccurredAt`, and `RecordedAt`; `result.Disposition` distinguishes a new insert from an identical duplicate.

### Failure and denial records

Failures and denials are legitimate outcomes, but recording one is an explicit application choice:

```go
_, err := recorder.Record(ctx, audit.Entry{
	Action:   "report.exported",
	Outcome:  audit.OutcomeDenied,
	Actor:    audit.Actor{Type: "user", ID: actorID},
	Resource: audit.Resource{Type: "report", ID: reportID},
	Reason:   "insufficient_scope",
})
```

`Reason` is a stable machine-readable code, not a raw error string. Raw provider errors, SQL text, stack traces, and authorization internals do not belong in the record.

### Querying

```go
page, err := reader.Query(ctx, audit.Query{
	Scope: audit.Scope{
		Application: "support-api",
		Tenant:      tenantID,
	},
	Resource: &audit.ResourceRef{Type: "ticket", ID: ticketID},
	Limit:    50,
	Cursor:   cursor,
})
```

The query surface supports only indexed, portable filters in v1:

- scope;
- actor type and ID;
- resource type and ID;
- exact action or bounded action set;
- outcome;
- recorded-time range; and
- forward or reverse chronology.

V1 does not expose arbitrary metadata predicates, substring search, JSON-path filters, or unbounded exports.

## Record Model

Illustrative shape:

```go
type Entry struct {
	ID         string
	Action     string
	Outcome    Outcome
	Actor      Actor
	Resource   Resource
	OccurredAt time.Time
	Reason     string
	Summary    string
	Changes    []Change
	Attributes []Attribute
}

type Record struct {
	ID          string
	Scope       Scope
	Action      string
	Outcome     Outcome
	Actor       Actor
	Resource    Resource
	OccurredAt  time.Time
	RecordedAt  time.Time
	Reason      string
	Summary     string
	Changes     []Change
	Attributes  []Attribute
	RequestID   string
	ExecutionID string
	TraceID     string
	SpanID      string
}

type Scope struct {
	Application string
	Tenant      string
}

type Actor struct {
	Type        string
	ID          string
	DisplayName string
	Impersonator *ActorRef
}

type Resource struct {
	Type        string
	ID          string
	DisplayName string
}
```

`DisplayName` values are optional denormalized presentation hints. They are not identity, are bounded more strictly than IDs, and may become stale. Applications that consider names sensitive should omit them and resolve current labels during presentation.

### Actions

Actions use lowercase dotted identifiers:

```text
ticket.created
ticket.assigned
user.disabled
album.share_revoked
report.exported
```

The grammar should be ASCII, bounded, and stable. It must reject whitespace, control characters, path separators, empty segments, and values derived directly from user input.

Libraries should encourage constants:

```go
const AuditActionTicketAssigned = "ticket.assigned"
```

No global registry is required in v1. Metrics must not use arbitrary action names as labels unless GoForj can prove a bounded application-owned catalog.

### Outcomes

V1 outcomes are closed:

- `succeeded`
- `failed`
- `denied`
- `cancelled`

The outcome describes the attempted domain action, not whether the audit append succeeded. An append failure is returned as an error and observed separately.

### Actors

Actor types are application-defined stable identifiers, with recommended values:

- `user`
- `service`
- `api_client`
- `job`
- `scheduler`
- `system`

Anonymous activity uses an explicit `anonymous` actor type and an empty ID only where the application chooses to retain that activity. An empty actor is otherwise invalid.

Impersonation records both the effective actor and the initiating actor. It must never overwrite the effective user with the administrator or discard the administrator identity.

### Changes and attributes

The library provides scalar constructors:

```go
audit.String("status", "open")
audit.Integer("attempt", 2)
audit.Boolean("enabled", true)
audit.Time("expires_at", expiresAt)
audit.Redacted("email")
```

Changes use optional before and after scalar values. Absence and explicit null remain distinct.

Values do not accept arbitrary `any`, maps, arrays, or structs in v1. This restriction reduces accidental data capture, unstable encodings, oversized records, and cross-dialect JSON differences.

Keys are unique after normalization. Duplicate keys fail validation rather than applying last-write-wins behavior.

### Time

The recorder owns a clock. If `OccurredAt` is zero, it uses the current clock value. A caller-supplied occurred time is allowed for importing an externally observed domain action but is validated against configurable future and historical bounds.

`RecordedAt` always comes from the recorder immediately before it invokes `Append`. Stores preserve the supplied canonical UTC value rather than applying an unrelated database default. It represents append-attempt time, not the later instant at which a lock wait or commit completed. Database timestamps and the recorder clock are tested for the supported precision.

### Identity and duplicate retries

The default identifier is a sortable, text-safe 128-bit identifier derived from the recorder clock and cryptographic entropy. The exact encoding becomes a persisted compatibility contract and must be specified before implementation.

Callers may provide an identifier when the business operation already has a stable operation ID. IDs are scope-local, so the same operation ID may exist independently in two tenants or Apps. A caller-supplied ID requires a non-zero explicit `OccurredAt`; otherwise a later retry would resolve a different time and could not be compared honestly. On conflict within the same exact scope:

- identical canonical intent returns the existing record with a duplicate result classification;
- different canonical content returns `ErrConflict`; and
- concurrent identical attempts converge on one record.

The intent fingerprint includes every caller-controlled canonical field and excludes only recorder-assigned `RecordedAt`. This behavior requires the store to compare the intent fingerprint after a uniqueness conflict. It must never treat every duplicate ID as success.

SQL stores must detect duplicates without poisoning a caller-owned transaction. PostgreSQL and SQLite use conflict-tolerant insert semantics such as `ON CONFLICT DO NOTHING`; MySQL uses an equivalently narrow duplicate-key strategy that does not suppress unrelated validation or storage errors. The store then reads only the same scoped identity and compares its fingerprint. An unhandled uniqueness error followed by a read is invalid because PostgreSQL leaves the transaction aborted.

## Context Enrichment

The root library supports explicit immutable context values for:

- scope;
- actor;
- request ID;
- execution ID;
- trace ID and span ID; and
- trusted source classification.

Example:

```go
ctx = audit.WithActor(ctx, audit.Actor{Type: "user", ID: userID})
ctx = audit.WithScope(ctx, audit.Scope{Application: "admin", Tenant: tenantID})
```

Context enrichment follows deterministic precedence:

1. recorder construction supplies required application scope;
2. trusted context may supply tenant, actor, and correlation;
3. the explicit entry may refine actor display information but cannot silently replace a non-empty trusted actor identity; and
4. conflicts fail with a validation error.

This prevents a service from accidentally recording a different actor than the authenticated middleware established. Non-GoForj applications can omit trusted context enrichment and provide the actor directly.

Construction supports an explicit tenant policy such as `audit.WithTenantRequired()`. A tenant-aware API rejects record, find, and query operations lacking a tenant before any store call; its separately constructed matching maintainer applies the same rule to retention. GoForj enables this policy explicitly for an App that declares tenant-scoped Audit; it never infers a tenantless/global fallback after middleware fails to enrich context.

GoForj middleware should derive audit context only from authenticated server-side state. It must never trust an actor, tenant, request ID, or trace ID solely because a client submitted it in a header or body.

## Validation And Limits

The root library validates the complete canonical record before calling the store.

Required configurable hard limits include:

- record ID bytes;
- action bytes and segments;
- actor/resource type and ID bytes;
- display-name bytes;
- reason-code bytes;
- summary bytes;
- number of changes;
- number of attributes;
- attribute key bytes;
- scalar string bytes;
- total canonical record bytes;
- query action count;
- query page size; and
- cursor bytes.

Defaults should be conservative and compiled into the library. Applications may lower limits but cannot raise them beyond library hard ceilings without constructing an explicitly unsafe custom policy.

Validation rejects:

- invalid UTF-8;
- NUL and control characters;
- CR/LF injection in single-line identifiers and summaries;
- duplicate normalized keys;
- non-finite numbers;
- timestamps outside accepted bounds;
- empty required identity fields;
- malformed trace/span identifiers;
- unknown outcomes;
- unsupported scalar kinds;
- oversized canonical records; and
- scope conflicts between entry, context, query, and cursor.

Validation errors identify the field and stable reason without echoing its sensitive value.

## Sensitive Data And Redaction

The library cannot infer application sensitivity. Its safe posture is therefore omission-first.

Applications should record stable identifiers and categorical changes rather than full values. The default documentation must explicitly prohibit:

- passwords;
- session IDs and cookies;
- access and refresh tokens;
- API keys and signing secrets;
- encryption keys;
- database DSNs;
- payment card or bank data;
- full request or response bodies;
- raw authorization failure internals;
- health and government identifiers; and
- unreviewed model serialization.

`audit.Redacted(key)` records that a field changed while retaining neither value. Hashing is not a universal anonymization mechanism and should not be the default helper for low-entropy values such as email addresses or phone numbers.

Observer, log, metric, and Lighthouse adapters receive only safe classifications:

- operation;
- store/connection name;
- outcome;
- duplicate/conflict classification;
- duration;
- result count; and
- sanitized error class.

## Store Contract

### Append

`Append` atomically performs one of three outcomes:

1. inserts and returns the record;
2. returns the existing canonically equal record as a duplicate; or
3. returns `ErrConflict` for the same ID with different canonical content.

Stores must not mutate the supplied record, assign a different scope, truncate fields, normalize values differently, or replace append-attempt time.

### Find

`Find` always requires scope plus ID. Identifier uniqueness and conflict resolution are scoped by application plus normalized tenant key. A same-ID record in another scope is neither a duplicate nor a conflict and must not be read during resolution.

### Query

Queries are stable snapshots only to the extent supported by the database transaction isolation selected by the caller. Ordinary cursor pagination guarantees deterministic continuation order, not an immutable multi-page snapshot while concurrent records arrive.

The default order is newest first. Cursor state includes:

- direction;
- last `recorded_at`;
- last record ID;
- normalized filter fingerprint; and
- cursor format version.

Cursors are opaque and integrity-protected with a process-configured key in GoForj HTTP presentation. The root store cursor need only be structurally opaque and filter-bound because authorization must still be applied independently. Applications must not treat possession of a cursor as authorization.

### Canonical encoding

The library defines one deterministic canonical representation used for:

- size enforcement;
- duplicate-content fingerprints;
- fake/driver parity; and
- possible future integrity sealing.

Canonical encoding is versioned. Map ordering, timestamp precision, absent versus null values, numeric representation, and Unicode handling are specified and covered by golden tests. The storage representation may differ, but decoding must reproduce the same logical record.

## Transactional Consistency

### Required atomic path

For a successful business mutation whose audit record is required, both writes must use the same database transaction:

```go
err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
	txAudit, err := auditManager.WithDB(tx)
	if err != nil {
		return err
	}

	if err := tickets.WithDB(tx).Assign(ctx, ticketID, assigneeID); err != nil {
		return err
	}

	_, err = txAudit.Record(ctx, entry)
	return err
})
```

The generated `internal/audits.Manager.WithDB` returns `(audit.Recorder, error)` backed only by the supplied `*gorm.DB`. Binding is the transaction callback's first operation and returns an error before the business mutation runs. It never falls back to the root database connection.

The transaction commits only after the audit append succeeds. A rollback removes both the business mutation and its audit record.

### Recording denied and failed attempts

Denials and failures often have no committed business mutation. They may be appended using the root recorder after the decision. Applications must avoid returning a misleading failure reason or sensitive authorization detail.

### Cross-database and external operations

The library does not promise atomicity across databases, object storage, queues, or external APIs. Applications choose among:

- recording intent before the operation and a result afterward;
- using an application-owned outbox in the same database transaction;
- accepting an explicitly documented consistency gap; or
- treating the external operation's provider identifier as a correlated record attribute.

Audit must not smuggle an outbox or workflow engine into its core API.

## Persistence Model

GoForj generates an `audit_records` table for SQLite, MySQL, and PostgreSQL.

Logical columns include:

- `id`, unique only with normalized application and tenant scope;
- `application`;
- `tenant_key`, non-null and set to the canonical tenantless key when no tenant applies;
- `action`;
- `outcome`;
- actor type, ID, and optional display name;
- optional impersonator type and ID;
- resource type, ID, and optional display name;
- `occurred_at`;
- `recorded_at`;
- optional reason and summary;
- bounded canonical changes;
- bounded canonical attributes;
- request, execution, trace, and span identifiers;
- canonical format version; and
- canonical content fingerprint.

Tenantless records use one explicit normalized non-null tenant key such as the empty string in indexed identity columns; nullable SQL uniqueness semantics must not decide record identity differently across dialects. The primary/unique identity is `(application, tenant_key, id)`.

Application, tenant, record ID, actor/resource identity, and chronological tie-break columns use bytewise/binary comparison consistently across every dialect. Caller-supplied record IDs use one lowercase ASCII grammar and reject non-canonical casing; they are not silently folded. MySQL migrations must not inherit a case-insensitive default collation for identity or ordering columns.

Required indexes should serve the supported query contract rather than every possible combination:

- `(application, tenant_key, recorded_at, id)`;
- `(application, tenant_key, resource_type, resource_id, recorded_at, id)`;
- `(application, tenant_key, actor_type, actor_id, recorded_at, id)`; and
- an action/time index only if representative query plans justify it.

The schema must avoid database-specific enum types so action and outcome compatibility remains portable. Outcomes still receive application validation and database check constraints where the supported dialect can preserve migration compatibility.

Changes and attributes may be stored in JSON-capable columns, but queries never depend on JSON-path behavior in v1. SQLite text JSON must be validated before persistence. Drivers return an error for malformed stored canonical data rather than silently skipping it.

Migrations are application source of truth. The driver never calls `AutoMigrate`.

## Append-only, Integrity, And Retention

### Honest append-only claim

V1 is append-only through its normal application API and generated database role guidance. It cannot prevent a database owner from changing or deleting rows.

Documentation must say:

- application append-only is not physical immutability;
- backups and replicas inherit their own access controls;
- a privileged database administrator remains capable of alteration; and
- compliance requirements may require external immutable storage or a specialized service.

### Integrity sealing

Hash chaining is deferred from v1. A correct design must resolve:

- concurrent append serialization;
- per-tenant or global chain partitioning;
- hot-partition performance;
- deletion and retention checkpoints;
- backup/restore continuity;
- key rotation;
- missing-record proofs;
- verification after partial restore; and
- behavior across all supported databases.

A naive `previous_hash` field would create an unjustified security claim and a global write bottleneck.

### Retention

Retention uses the separate maintenance capability:

```go
result, err := maintenance.Prune(ctx, audit.PruneRequest{
	Scope:  scope,
	Actor:  maintenanceActor,
	Before: cutoff,
	Limit:  1000,
})
```

Pruning is bounded and repeatable. Each request names exactly one application and one tenant key, including the explicit tenantless key; it returns counts and the next continuation boundary. It never accepts an empty scope as shorthand for every tenant or App. The maintainer fixes the operation start time once and rejects a cutoff later than that time, preventing the new maintenance summary from entering its own deletion window.

Each batch transaction deletes its selected rows and appends exactly one maintenance record in the same scope. That summary records the authenticated maintenance actor, exclusive cutoff, deleted count, continuation boundary, and outcome, but not the deleted contents. Its occurrence and recorded times are the maintenance execution time, so the summary is outside the historical cutoff that caused the deletion. If either the deletion or summary append fails, neither commits.

GoForj may generate an `audit:prune` command only when an application configures a retention policy. The command:

- defaults to a plan/count preview;
- requires an explicit execution flag;
- displays scope and cutoff;
- requires `--tenant` for tenant-aware applications, including an explicit value representing the tenantless scope;
- processes bounded batches;
- records operational metrics without recursively auditing every deleted row; and
- never claims legal-hold awareness.

Applications with legal hold or regulated retention need an application-specific policy layer outside v1.

`AUDIT_RETENTION` is operator-enforced policy input, not an automatic expiry guarantee. `audit:status` reports the configured duration, the last successful prune time, and overdue state. Deployments must install the command in their existing scheduler, platform cron, or operations workflow; selecting Audit does not imply GoForj Scheduler or create a hidden goroutine. Documentation must say plainly that records remain until that operation runs successfully.

## Error Model

Stable sentinels should include:

- `ErrInvalidEntry`
- `ErrInvalidQuery`
- `ErrNotFound`
- `ErrConflict`
- `ErrCorruptRecord`
- `ErrCursorMismatch`
- `ErrUnavailable`
- `ErrUnauthorizedMaintenance`
- `ErrClosed`

Typed errors may add field names, safe classifications, and retry hints. They must support `errors.Is` and `errors.As`.

Raw SQL, DSNs, credentials, full record content, and sensitive driver responses must not appear in public error strings or public unwrap chains. `errors.Is` and `errors.As` expose only library-owned classifications, typed safe details, and the safe standard-library cancellation identities `context.Canceled` and `context.DeadlineExceeded`. Raw driver causes go only to an explicitly configured private diagnostic hook whose contract forbids normal application logging.

The library does not label validation, conflict, or corruption errors as retryable. Availability and transaction errors retain conservative driver-informed classification.

## Observer Model

Observer callbacks cover:

- append started/completed;
- duplicate append;
- conflicting append;
- find/query completed;
- prune batch completed; and
- corruption detected.

Illustrative event:

```go
type Observation struct {
	Operation  Operation
	Store      string
	Status     Status
	Duration   time.Duration
	Count      int
	ErrorClass string
}
```

The observer contract is a non-blocking `TryObserve(Observation) bool`. Invocation is synchronous only after the store result and disposition are fixed; `false` means the observation was dropped. Panics are recovered and reported through a separately configured private diagnostic hook. A caller-supplied observer that violates the non-blocking contract can delay the API return, but it cannot change, roll back, or replace the committed store result. GoForj-provided metrics and Lighthouse adapters perform no network or disk I/O in the callback and use bounded downstream buffers where export is needed.

## In-memory Store And Fake

The root repository should provide:

- `audit.NewMemoryStore()` for local use and store contract tests; and
- `auditfake.New()` for capture and assertions.

The fake implements the same recorder API and validation path. It records immutable snapshots under a mutex and supports concurrent tests.

Suggested assertion functions avoid generic methods so the package remains compatible with the repository's minimum Go version:

```go
auditfake.RequireCount(t, fake, 1)
auditfake.RequireAction(t, fake, "ticket.assigned")
record := auditfake.FindByResource(t, fake, "ticket", ticketID)
```

Assertions call `t.Helper()`, present bounded human-readable diffs, and never dump redacted or sensitive values by default. Callers may inspect returned records explicitly.

`Reset` is a fake-only operation and never appears on the production recorder.

## GoForj Component Model

### Selection and dependencies

`Audit` is an optional App component.

Rules:

- Audit requires a database.
- Audit does not imply Auth.
- Auth may emit audit records when both are enabled.
- Audit does not require Web API, Web UI, Events, Jobs, Scheduler, Cache, Storage, Mail, Metrics, or Lighthouse.
- Each App independently selects Audit.
- Multiple Apps may share one audit table because application scope is mandatory.

The creation wizard should describe Audit as “durable actor/action history,” not “enhanced logging.”

### Configuration

V1 reuses the database manager:

```dotenv
AUDIT_DB_CONNECTION=default
AUDIT_RETENTION=2160h
AUDIT_MAX_PAGE_SIZE=100
```

Component selection is the enablement boundary. V1 has no runtime `AUDIT_ENABLED` switch because explicit recording calls must not disappear or change semantics underneath application code. Auth integration must declare whether each record is required or optional; it cannot ignore the result accidentally.

Retention policy should be explicit in `.env.example`, alongside the requirement to install and monitor its operator-run enforcement. Changing retention is an operational behavior change and must be documented.

There is no `AUDIT_SUPPORTED_DRIVERS` in v1. The component binds to an existing database connection and uses its dialect.

### Generated manager

Generated `internal/audits.Manager` owns:

- the root `audit.API`;
- the configured database connection name;
- GORM store construction;
- transaction binding;
- observer attachment;
- readiness checks; and
- construction of the policy-matched maintainer, kept private from normal App accessors.

The manager borrows the configured `*gorm.DB`; it never closes the shared database handle or registers a second owner for it. Audit readiness checks only Audit-specific schema compatibility, required table/index presence, and a harmless scoped read through that already-live handle. Connection-pool reachability and lifecycle remain owned by the database component.

Generated App accessors should be explicit:

```go
func (a *App) Audit() audit.API
```

The maintenance capability is injected only into generated audit commands and scheduled retention. It is not exposed as `App.AuditMaintenance()`.

Services needing atomic audit writes may depend on the generated application-local transaction binder or accept a recorder inside their transaction callback. The normal reusable domain surface should continue to use `audit.Recorder`.

### Actor and correlation integration

When Auth is enabled, trusted auth middleware enriches request context with:

- effective user ID;
- impersonator ID when present;
- tenant ID only after server-side authorization resolution; and
- actor type.

Runtime inspection already carries source and execution context. Audit adapters add:

- App name;
- request ID;
- execution ID;
- trace ID;
- span ID; and
- source classification such as HTTP, CLI, job, scheduler, or startup.

Background jobs must establish an actor deliberately. A job is not attributed to the user who originally caused it unless that attribution is persisted safely in the job payload or domain record and restored intentionally.

### Auth integration

When both components are enabled, generated Auth should record a small reviewed catalog such as:

- login succeeded or failed;
- logout and logout-all;
- password changed;
- password reset completed;
- email verified;
- session revoked;
- user disabled or re-enabled;
- provider identity linked or unlinked; and
- administrator impersonation started or stopped, if supported.

Authentication failure records need abuse-resistant bounds. Repeated unauthenticated failures can fill the audit table and capture attacker-controlled identifiers. The design should use categorical actor/resource information, rate-aware aggregation outside Audit where appropriate, and no raw passwords, tokens, cookies, or provider responses.

### Routes and UI

GoForj does not generate a generic public route. A starter example may demonstrate an authorized resource-scoped query whose controller:

1. authenticates the caller;
2. authorizes access to the resource or tenant;
3. constructs a scope-bound query server-side;
4. clamps the page size;
5. projects records into a public DTO; and
6. never returns internal correlation or attributes by default.

OpenAPI should describe the projected DTO and opaque cursor, not the entire internal record type.

### Commands

Potential generated commands:

- `audit:status` reports component readiness and configured scope without record contents.
- `audit:prune` performs retention planning and explicit bounded execution.
- `audit:verify` is deferred until a real integrity contract exists.
- `audit:export` is not generic in v1 because authorization, redaction, and legal scope are application-specific.

Named App routing follows existing conventions:

```bash
forj admin audit:status
forj admin audit:prune --before 2026-01-01T00:00:00Z --execute
```

## Metrics And Lighthouse

### Metrics

Candidate metrics:

- append attempts by store and status;
- append duration;
- query duration and returned count;
- duplicate/conflict counts;
- corruption detections;
- prune duration and count; and
- last successful append timestamp.

Do not label metrics with:

- actor IDs;
- resource IDs;
- tenant IDs;
- request or trace IDs;
- summaries;
- reason values derived from users; or
- arbitrary action names unless backed by a statically bounded catalog.

### Database logging and inspection

Audit persistence must not pass expanded SQL or bound audit values through a borrowed GORM handle's logger. The GORM driver creates a scoped session with an Audit-owned logger that never invokes GORM's SQL-rendering callback and emits only a bounded operation classification, table identifier, duration, row count, and sanitized error class to an optional private diagnostic sink. GoForj also marks the operation as sensitive so its database inspection adapter cannot create `raw_sql` as a fallback. This protection applies to successful statements, slow-query reporting, retries, and failures, and it must be verified against both a hostile caller-supplied GORM logger and the generated inspection path for every supported dialect.

### Lighthouse

Inspection events should show:

- “audit append”;
- store/connection;
- success, duplicate, conflict, or error;
- duration; and
- the execution correlation already present in Lighthouse.

They should not show actor display names, resource display names, changes, summaries, attributes, or failure reasons.

V1 emits no record ID or direct audit-record link into Lighthouse. An authorized operator may use the execution correlation already present in both systems through an application-owned audit UI, but Lighthouse must not become an alternate authorization bypass.

## Security Model

1. Every query is scope-bound before store execution.
2. Cursor contents never grant scope or authorization.
3. Audit UI and exports require application-owned authorization.
4. External actor and tenant identifiers are treated as untrusted until resolved by server-side code.
5. All text rejects control-character injection and invalid UTF-8.
6. Every collection and record has a hard size ceiling.
7. Secrets and sensitive fields are omitted or explicitly redacted before persistence.
8. Records are not copied into logs, metrics, errors, or Lighthouse.
9. Maintenance uses a separate capability and bounded scope.
10. SQL drivers use parameterized queries and never interpolate filters.
11. Database permissions should deny application updates to audit rows where deployments can support separate roles.
12. Restore, replication, backup, and administrator access remain outside the library's immutability guarantee.
13. Query endpoints protect against enumeration through authorization and stable not-found behavior.
14. Bulk failure traffic and attacker-controlled unauthenticated actions cannot create unbounded records without application-level abuse controls.
15. Test failure output redacts values unless explicitly requested by the test author.

## Concurrency And Performance

- Recorder handles are safe for concurrent use.
- Bound context handles are immutable views over shared recorder state.
- Memory stores use defensive copies and a mutex.
- SQL appends rely on primary-key uniqueness for duplicate convergence.
- Queries use composite indexes matching scope and chronology.
- Page sizes are bounded and no API returns all records.
- Changes and attributes are encoded once per append.
- Observer callbacks are non-blocking by contract; provided exporters use bounded buffers and report drops.
- V1 performs no global serialization or hash-chain locking.
- Retention deletes bounded batches to avoid long locks and transaction-log spikes.

Benchmarks should measure:

- validation and canonical encoding;
- memory append and query;
- duplicate append comparison;
- cursor encode/decode;
- representative SQL append latency; and
- indexed resource-history queries at realistic table sizes.

Benchmarks detect regressions but do not become noisy hard CI gates.

## Compatibility

### Source and API compatibility

The new component is additive. Existing generated Apps without Audit retain their source and dependency shape.

### Configuration compatibility

Audit introduces new `AUDIT_*` keys only when selected. Existing keys keep their meaning.

### Persisted-data compatibility

The canonical record format, outcome strings, timestamp precision, identifier encoding, and cursor version are persisted contracts. Changes require compatible reads and explicit migration planning.

Cursor compatibility may be intentionally shorter-lived than record compatibility, but decoding failures must return a stable error that lets clients restart pagination.

### Runtime behavior

Enabling Auth audit emission adds database writes to selected flows and can cause those flows to fail if records are configured as mandatory. This is a concrete runtime behavior change and must be documented per action.

### Operational migration

Adding the component requires applying the generated audit migration before enabling writes. Removing it does not delete the table or retained records automatically.

### Minimum Go version

The Audit library should initially match the current GoForj sibling baseline. No version increase is justified by this design alone.

## Testing Strategy

### Root unit tests

Directly cover every validation branch and failure mode:

- identifiers and grammar;
- actor/resource rules;
- scope conflicts;
- outcomes;
- scalar kinds;
- duplicates;
- size budgets;
- timestamps;
- canonical encoding;
- cursor/filter binding;
- context cancellation;
- defensive copies;
- observer panics; and
- redacted error strings.

Fuzz:

- entry validation;
- canonical decode;
- cursor decode;
- malformed UTF-8;
- scalar encodings; and
- duplicate-content comparison.

### Store contract

One reusable conformance suite runs against:

- memory;
- SQLite;
- MySQL; and
- PostgreSQL.

It verifies:

- append/find;
- identical duplicate convergence;
- conflicting duplicate rejection;
- PostgreSQL duplicate convergence inside an explicit transaction without poisoning that transaction;
- scope-local identity, explicit tenantless identity, and mixed-case ID rejection;
- concurrent duplicate attempts;
- total query ordering;
- forward and reverse cursors;
- cursor filter mismatch;
- scope isolation;
- malformed persisted data;
- cancellation;
- transaction commit and rollback; and
- bounded retention; and
- exact migration index columns on every dialect;
- cancellation identity through `errors.Is`;
- atomic retention deletion plus one same-scope maintenance summary, including rollback when the summary fails; and
- future-cutoff rejection and a no-op terminal retention batch.

### GoForj tests

Cover:

- component selection and database requirement;
- generated dependencies and Wire composition;
- each migration dialect;
- default and named Apps;
- named database selection;
- borrowed database ownership and Audit-specific schema readiness;
- trusted auth enrichment;
- transaction-bound recorder behavior;
- Auth actions enabled and absent when component-disabled;
- metrics cardinality;
- database log/inspection suppression of expanded Audit values;
- Lighthouse redaction and absence of record identifiers;
- observer drop, panic recovery, and post-commit blocking behavior;
- retention command preview and execution;
- retention overdue status without implying automatic enforcement;
- rerender stability; and
- removal preserving retained data.

All rendered validation occurs under `/tmp`, never inside the GoForj repository.

### Security tests

Verify:

- CR/LF and delimiter injection rejection;
- secret values never reach observations or errors;
- raw driver errors cannot be reached through the public unwrap chain;
- tenant cursor swapping fails;
- unauthorized query examples cannot broaden scope;
- malformed stored JSON fails visibly;
- oversized records fail before database access;
- high-volume denied actions remain bounded by application integration policy; and
- maintenance cannot accept an empty/global scope accidentally.

## Documentation

The Audit repository README should answer:

- what Audit is and is not;
- how it differs from events and logs;
- how to record success, denial, and failure;
- how to bind a database transaction;
- how to choose safe fields;
- how cursor pagination works;
- what append-only does and does not guarantee;
- how retention works; and
- how to use the fake.

Generated App documentation should include:

- component configuration;
- the exact table and indexes;
- action naming guidance;
- auth actions emitted by default;
- atomic mutation examples;
- authorization requirements for history views;
- retention operation; and
- a conspicuous sensitive-data checklist.

Examples must place readable expected output immediately after calls that produce output and must avoid printing realistic secrets or personal data.

## Delivery Plan

### Phase 1: root contracts

1. Create the sibling repository.
2. Implement record types, validation, canonical encoding, identifiers, clocks, and context enrichment.
3. Implement recorder/reader/store interfaces.
4. Implement memory store and fake.
5. Publish the store conformance suite.
6. Generate API docs and compile all examples.

### Phase 2: GORM persistence

1. Add the dependency-isolated GORM driver.
2. Support existing GORM handles and transaction binding.
3. Prove SQLite, MySQL, and PostgreSQL conformance with real databases.
4. Prove duplicate convergence and stable query plans.
5. Do not add automatic migrations.

### Phase 3: GoForj component

1. Add Audit component selection and database dependency validation.
2. Add generated migrations and environment contracts.
3. Add manager composition, App accessor, readiness, and lifecycle.
4. Add transaction binding and direct rollback tests.
5. Add metrics and Lighthouse adapters.

### Phase 4: framework integrations

1. Add the reviewed Auth action catalog.
2. Add one resource-scoped demo history surface.
3. Add Testkit capture and assertion support.
4. Add retention status/plan/execution behavior.
5. Validate default, named, minimal, and largest generated compositions.

### Phase 5: release

1. Inventory all root, generated, integration, fixture, and script dependency pins.
2. Release the root and any nested driver module independently using repository scripts.
3. Verify every required tag is available.
4. Integrate published versions into GoForj.
5. Run `GOWORK=off` validation so local replacements cannot hide missing releases.

## Acceptance Criteria

The design is implemented only when:

- Audit is demonstrably separate from events, logging, and Lighthouse storage;
- the root API has no GoForj, GORM, auth, HTTP, or observability dependency;
- every entry is validated and bounded before persistence;
- arbitrary structs and nested payloads cannot enter changes or attributes through the normal API;
- successful mutation plus required audit append can commit and roll back atomically on every supported database;
- transaction binding never falls back to a root connection;
- identical duplicate IDs converge and conflicting content fails;
- queries are scope-bound, stable, cursor-paginated, and index-supported;
- there is no generic generated public audit endpoint;
- maintenance is inaccessible from the normal application recorder;
- errors, metrics, logs, inspections, and test diffs do not leak audit values;
- append-only and integrity guarantees are documented honestly;
- memory, SQLite, MySQL, and PostgreSQL pass one store contract;
- component-off renders contain no Audit code, migration, env keys, or dependency;
- default and named Apps preserve App scope correctly;
- generated migration and runtime behavior survive rerender;
- all relevant modules and tag surfaces are independently released and validated; and
- generated examples and documentation remain authoritative and reproducible.

## Risks And Mitigations

### Risk: Audit becomes a second event system

Mitigation: expose append and query, not subscribe or publish. Do not derive records from every event.

### Risk: Audit becomes a second logging system

Mitigation: keep durable product records separate from diagnostic messages and OTLP export. Observations carry only summaries.

### Risk: Sensitive data accumulates permanently

Mitigation: scalar-only values, hard bounds, omission-first documentation, redaction helpers, explicit retention, and security tests.

### Risk: Records claim changes that rolled back

Mitigation: provide and document the same-transaction path as the required model for committed mutations.

### Risk: Append-only language overstates integrity

Mitigation: distinguish API append-only behavior from physical immutability and defer cryptographic claims.

### Risk: Generic audit UI leaks cross-tenant history

Mitigation: generate no generic route and require server-constructed scope after authorization.

### Risk: Authentication attacks fill the table

Mitigation: keep unauthenticated data categorical, bound record size, and require application-level abuse controls or aggregation.

### Risk: Audit failures unexpectedly break user flows

Mitigation: make error policy explicit at each integration point and document which framework actions are mandatory.

### Risk: JSON behavior diverges across databases

Mitigation: define canonical encoding in the root, avoid JSON predicates, and run one real-driver contract across all dialects.

## Deferred Questions

The following require evidence after v1 and are not invitations to leave core semantics ambiguous:

1. Whether a real use case requires a specialized immutable external sink.
2. Whether integrity sealing can avoid hot partitions while supporting retention and key rotation.
3. Whether audit records should support a second related resource as a typed indexed field.
4. Whether large authorized exports justify an application-facing streaming reader.
5. Whether a statically registered action catalog provides enough value for action-labelled metrics.
6. Whether denial/failure aggregation belongs in a separate security-events facility.

The v1 record, scope, atomicity, validation, query, and retention contracts should not wait on those questions.

# Search Primitive And GoForj Integration Design

## Status

- Design status: proposed
- Planning date: 2026-08-30
- Independent review: eight independent passes completed 2026-08-30; review findings are incorporated into this revision
- Target repositories: a new `github.com/goforj/search` sibling repository, `goforj`, and `goforj-docs`
- Primary sibling-library scope: portable search contracts, index definitions, query and filter types, document mutations, driver capabilities, observers, fakes, conformance tests, and backend drivers
- Primary GoForj scope: the Search component, ordinary named-index lookup, environment policy, App composition, maker commands, reindex workflows, Compose services, Lighthouse integration, metrics, API-index compatibility, render coverage, and Atlas guidance
- Related context:
  - [`../context/repo-boundaries-and-ownership.md`](../context/repo-boundaries-and-ownership.md)
  - [`../context/generated-app-extension-points.md`](../context/generated-app-extension-points.md)
  - [`completed/app-primitive-component-gating-plan.md`](completed/app-primitive-component-gating-plan.md)
  - [`primitive-design-suggestions.md`](primitive-design-suggestions.md)
  - [`completed/forj-api-index-design.md`](completed/forj-api-index-design.md)

## Summary

GoForj should add Search as a first-class optional App component backed by a new reusable `github.com/goforj/search` sibling library.

The library should provide one stable application-facing contract for the most common lexical-search workflow:

- declare an index and its fields;
- create or validate that index;
- upsert and delete documents;
- perform text queries;
- apply structured filters and deterministic sorting;
- paginate results;
- return term facets, while deferring range facets;
- return safe structured highlights;
- report exact, estimated, or lower-bound totals honestly;
- expose asynchronous mutation state through concrete receipts;
- bulk-index with item-level failure reporting;
- observe calls without leaking indexed content; and
- document optional concrete-driver behavior without inflating the portable API.

The portability promise is intentionally narrower than identical search behavior:

> Switching Search drivers preserves the application workflow and data contract. It does not guarantee identical tokenization, relevance ordering, typo tolerance, highlight fragments, facet precision, or latency.

That is an honest and useful boundary. Search engines share a large application-facing core, but relevance is application- and engine-specific. The library should make the common path portable while leaving advanced behavior explicit.

The recommended first production release supports:

1. Bleve as the embedded, zero-service default;
2. Meilisearch as the first service-backed driver; and
3. Typesense as the second service-backed driver and the first proof that the contract is not shaped around one server.

PostgreSQL, SQLite FTS5, Redis Search, OpenSearch, Elasticsearch, MySQL, and Algolia should follow in deliberate tiers. They should not block the initial release, and no driver should ship until it passes the shared semantic contract plus its own real-backend integration suite.

## Decision

Build Search as a sibling primitive with one small consumer-owned backend
interface and a concrete higher-level `Index`.

Adopt these decisions:

1. The database or another application-owned source remains authoritative. A search index is rebuildable derived data.
2. Index definitions are explicit Go values. Reflection or struct tags may help construct definitions, but they are not the only source of truth.
3. The portable core is lexical search. Vector, hybrid, geo, suggestion, synonym, and engine-ranking APIs are optional capabilities.
4. Search requests use a typed expression tree. The portable API never accepts a raw backend query string as its normal filter surface.
5. Index writes return a concrete `Receipt`. Callers wait on the index for visibility without learning a backend task API.
6. Bulk operations report every item outcome the backend can prove and label outcome precision explicitly. A successful transport response cannot hide known partial document failures.
7. Highlight output is structured into text and matched segments. The core does not return trusted HTML.
8. Total and facet precision are represented explicitly.
9. Relevance scores are useful only within one response from one driver. The contract does not normalize scores across drivers.
10. Startup validation fails closed on schema drift. Construction never silently drops, rebuilds, or destructively mutates an index.
11. A disabled Search component renders no Search packages, env, dependencies, commands, Compose activation, or runtime wiring.
12. Search does not imply Database, Queue, Events, or Cache. GoForj integrates with those components when selected but retains a complete direct path without them.
13. A null driver that silently returns no matches is not a production driver. Tests use `searchfake`; unavailable production search returns a classified error.
14. Plain query text is literal untrusted text. Drivers compile it without enabling backend query-string operators.
15. V1 rebuilds are offline. Online promotion and fencing require a separate control-plane design.
16. GoForj validates required definitions during runtime start/readiness. V1 does not claim to detect out-of-band drift after successful startup.
17. Partial or timed-out search responses fail; partial search is not a v1 mode.
18. Every text query uses all-terms membership. Relaxed matching is deferred.
19. Runtime construction never alters schema; provisioning is a separate GoForj workflow.
20. Fresh-index creation is idempotent and distinct from schema mutation.
21. Portable v1 facets are term distributions; range facets are deferred.
22. Public methods take `context.Context`, constructors return concrete types, and interfaces live at the consuming boundary.
23. Deployment names, managers, registries, lifecycle, and provisioning orchestration belong to GoForj, not the reusable domain API.

## Motivation

Search is a recurring product requirement rather than a niche infrastructure feature.

The existing sample applications already need it:

- PhotoDrop searches photos by title, original filename, people, albums, and dates.
- PocketDesk searches and filters tickets.
- Gather searches published events and organizer data.
- Atlas searches framework and project documentation.
- Lighthouse increasingly needs bounded search across operator-facing records.

Without a primitive, each generated application must decide independently:

- whether to query SQL with `LIKE`, use database full-text search, or run a search service;
- how to describe indexes and searchable fields;
- how to synchronize source records into an index;
- how to represent filters, facets, totals, highlights, and partial failures;
- how to rebuild indexes safely;
- how to test search behavior without coupling every unit test to a service; and
- how to observe indexing lag and backend failures.

Search also fits GoForj's established local-to-production model. An App can begin with an embedded index and later select a distributed or hosted driver through configuration and generated support, without rewriting domain services or controllers.

## Goals

- Make ordinary application search portable across materially different engines.
- Provide a useful embedded default that requires no developer service.
- Preserve access to advanced engine capabilities without polluting the portable API.
- Keep backend clients and query syntax out of domain code.
- Make index definitions, drift, mutation visibility, and rebuild behavior explicit.
- Support multiple named indexes and multiple configured engines in one App.
- Make fakes and contract tests first-class sibling packages.
- Follow existing GoForj sibling conventions for modules, docs, examples, releases, quality gates, and integration testing.
- Integrate cleanly with generated App composition, API indexing, Lighthouse, metrics, queues, events, and developer workflows.
- Make unsafe multi-tenant and arbitrary-filter patterns difficult to introduce accidentally.

## Non-Goals

- Do not produce identical result ordering across engines.
- Do not expose every backend query DSL through the root package.
- Do not build another general database or ORM.
- Do not make search indexes authoritative application storage.
- Do not automatically inspect or mirror arbitrary database tables.
- Do not make vector databases, retrieval-augmented generation, or embeddings part of v1.
- Do not generate browser-visible administration credentials.
- Do not promise zero-downtime rebuilds for drivers or applications that cannot provide the required promotion and change-capture guarantees.
- Do not ship a driver based only on mocked HTTP tests.
- Do not force Search into every generated App or into the normal new-project wizard defaults.

## Design Principles

### Application portability, not result identity

The portable contract covers what the application asks for and the shape it receives. The following may legitimately differ after a driver switch:

- tokenization and stemming;
- stop words;
- typo and prefix matching;
- relevance scoring;
- tie behavior before the library's stable ID tie-break;
- chosen highlight fragments;
- exact versus estimated counts; and
- update visibility latency.

Tests should assert portable invariants, not identical backend snapshots.

### Derived data stays derived

Applications must be able to recreate every index from an authoritative source. A Search backup is an optimization, not the only recovery path. Standard GoForj backup behavior should preserve definitions and source data; it should not claim an external index backup is portable across engines.

### Explicit definitions beat inference

An index definition should be reviewable in source and produce a deterministic fingerprint. Struct-tag inference may reduce repetition, but hidden reflection must not decide production mappings without a generated or explicit definition that tests can inspect.

### Capabilities are honest contracts

Optional behavior is documented and tested per concrete driver. Driver-specific
methods fail with `ErrUnsupported` when the connected backend/version cannot
provide them; the portable root does not advertise a universal capability bag.

### Safe defaults are bounded defaults

The root package should impose configurable domain limits for document bytes,
JSON nesting depth, object members, array length, definition fields and pointer
depth, query bytes and terms, filter nodes and depth, result size, facets,
highlights, batch documents, and batch bytes. Concrete drivers own transport
limits such as HTTP body size, connections, and timeouts.

## Terminology

- **Engine:** one configured backend connection or embedded runtime, such as a Meilisearch server or Bleve root.
- **Driver:** the implementation that translates the portable contract to an engine.
- **Logical index:** the stable App-facing index name, such as `photos` or `tickets`.
- **Physical index:** a driver-owned generation used to implement rebuild and promotion.
- **Definition:** the declared searchable schema and portable index behavior.
- **Document:** one encoded, uniquely identified search record.
- **Hit:** one search result with its document, score, sort values, and highlights.
- **Receipt:** an immutable token for accepted mutation work whose visibility may be synchronous or asynchronous.
- **Promotion:** atomically or operationally switching a logical index to a rebuilt physical generation.
- **Capability:** optional behavior with a precise conformance contract.

## Repository Ownership

### `github.com/goforj/search`

The sibling library owns:

- public search types and semantics;
- index definitions and canonical fingerprints;
- query, filter, sort, facet, pagination, and highlight types;
- raw document encoding and typed helpers;
- one concrete `Index` and the small backend interface it consumes;
- the small backend contract and documented capabilities;
- error classification;
- receipt and partial-bulk semantics;
- observer events;
- fake and conformance packages;
- driver implementations;
- driver compatibility documentation;
- executable GoDoc examples and generated README API index; and
- unit, race, fuzz, benchmark, and real-backend integration coverage.

### `goforj`

The framework owns:

- the optional Search component;
- compile-time supported-driver selection and runtime driver selection;
- generated App composition and ordinary named-index lookup;
- driver construction, named engines, namespaces, and the
  runtime registry;
- generated environment inventory and safe defaults;
- maker commands and App registration points;
- lifecycle ownership and lazy engine initialization;
- queue/event integration for application indexing;
- reindex commands that call application-owned source providers;
- Compose catalog services and profile activation;
- build manifests and dependency generation;
- the core dependency catalog, render-warm version resolver and generated
  module, root/integration pins, and published-module generator fixtures;
- Lighthouse index, receipt, and health presentation;
- framework metrics and inspect annotations;
- render, smoke, integration, and API-index compatibility tests; and
- Atlas docs, guidance, tools, and live-agent evaluation scenarios.

### `goforj-docs`

The public documentation repository owns Search library ingestion and catalog
registration, the framework guide, environment and CLI reference, generated
file ownership documentation, navigation, ingestion tests, and the production
site build. Search is not complete merely because in-repository design docs
exist.

### `web`

The web sibling may own optional HTTP adapters or middleware that are generic outside GoForj, but it must not own search semantics. Application authorization, allowed filters, tenant policy, and response DTO choices remain App concerns.

## Proposed Multi-Module Repository Layout

```text
search/
  go.mod                         # github.com/goforj/search
  search.go
  index.go
  backend.go
  definition.go
  value.go
  mutation.go
  query.go
  filter.go
  result.go
  receipt.go
  observer.go
  errors.go
  searchtest/
  searchfake/
  driver/
    blevesearch/go.mod
    meilisearch/go.mod
    typesensesearch/go.mod
  dev/
    go.mod                       # tooling module; not published
    docs/
      readme/
      examplegen/
    examples/
    bench/
    integration/
      all/
      root/
      scenario/
      testenv/
  scripts/
  .github/workflows/
```

`searchtest` and `searchfake` are packages in the root module and version with
the contract they test. There is no `searchcore` module: the root consumes the
small `Backend` interface, root never imports concrete drivers, and driver
packages return concrete implementations that satisfy it. A driver receives a
separate module only when its dependency weight justifies isolation.
Roadmap driver directories and module manifests are created only when their
feasibility work begins; empty future release surfaces are not checked in.

The repository must check in a module manifest before its first release. The
root and isolated driver modules are published. `dev/go.mod` owns docs tooling,
examples, integration, and benchmarks as one untagged module. This keeps release ordering to
root first and then only the drivers that depend on that root version.

Published `go.mod` files must not contain committed relative sibling replaces.
Local development uses `go.work` or generated temporary modfiles. Release
checks validate every published module with `GOWORK=off` against downloaded
published siblings, not merely the repository workspace.

Do not create a shared SQL core module until PostgreSQL, SQLite, and MySQL implementations prove that they share more than connection plumbing. Their full-text semantics and schema operations are substantially different.

## Core Data Model

### Names and identifiers

Use validated domain-neutral types:

```go
type IndexName string
type FieldName string
type DocumentID string
```

Validation should reject empty names, control bytes, path traversal, backend delimiters that cannot be encoded safely, and the reserved `_forj` field namespace.

The sibling sees one logical `IndexName`. GoForj constructs one namespaced
manager per App and configures each concrete driver with the App's physical
prefix, so two Apps may both use `products` without adding `OwnerName`,
`IndexKey`, or engine plumbing to the search-domain API. Deployment identity is
recorded in GoForj diagnostics and backend ownership metadata.

Physical encoding is specified per driver, including case folding, maximum
length, reserved words, Unicode handling, truncation, and hash suffix. Any
truncation retains a collision-resistant hash of the complete logical identity.
The reverse mapping and ownership token are persisted as index metadata so
administrative commands display both logical and exact physical targets and do
not reconstruct destructive targets from names alone.

### Encoded document

The raw SPI should exchange JSON so drivers do not depend on application types:

```go
type Document struct {
    ID   DocumentID
    Body json.RawMessage
}
```

`Body` must contain exactly one JSON object. The root package clones caller
bytes, rejects duplicate object keys and trailing values, uses `json.Number`
while validating numbers, and validates every declared field before a driver
call. Unknown JSON fields are retained but are not queryable. Reserved `_forj`
keys are rejected at every object level where a declared path could collide.

The root package supplies generic free functions compatible with the sibling repositories' current minimum Go policy:

```go
func Encode[T any](id DocumentID, value T) (Document, error)
func Decode[T any](document Document) (T, error)
func Upsert[T any](ctx context.Context, index *Index, id DocumentID, value T) (Receipt, error)
func SearchAs[T any](ctx context.Context, index *Index, query Query) (Page[T], error)

type Page[T any] struct {
    Hits   []HitOf[T]
    Total  Total
    Facets []FacetResult
    Took   time.Duration
}

type HitOf[T any] struct {
    ID         DocumentID
    Value      T
    Score      *float64
    SortValues []Value
    Highlights map[FieldName][]Fragment
}
```

The typed hit preserves `Document.ID` separately because the decoded value need
not contain an ID property. Decode failure
identifies the hit ID and stops the typed operation; it is never converted into
a missing hit.

Do not raise the minimum Go version solely to use generic methods. If the sibling ecosystem later adopts a Go version with generic methods, ergonomic method forms can be evaluated as additive API.

Drivers may inject reserved metadata into their physical representation. The
portable round-trip contract is semantic JSON equivalence, not byte identity:
object member ordering, insignificant whitespace, escape spelling, and number
spelling may change. The root package canonicalizes accepted JSON before driver
submission, and returned documents must decode to that canonical value. A
future byte-preserving capability would need to store the original bytes as a
separate opaque value and account for its size explicitly.

### Portable values and request atoms

Essential values are concrete and cannot represent invalid unions:

```go
type Value struct { /* private */ }

func Keyword(string) (Value, error)
func Boolean(bool) Value
func Integer(int64) (Value, error)
func (v Value) Kind() ValueKind
func (v Value) Keyword() (string, bool)
func (v Value) Boolean() (bool, bool)
func (v Value) Integer() (int64, bool)

type Mutation struct { /* private */ }

func UpsertMutation(Document) (Mutation, error)
func DeleteMutation(DocumentID) (Mutation, error)
func (m Mutation) Kind() MutationKind
func (m Mutation) Document() (Document, bool)
func (m Mutation) ID() DocumentID

type Sort struct {
    Field     FieldName
    Direction Direction
}

type Highlight struct {
    Field FieldName
}
```

`ValueKind` is `keyword`, `boolean`, or `integer`; `MutationKind` is `upsert` or
`delete`; and `Direction` is `ascending` or `descending`. Unknown and zero
values are invalid at the public call boundary. Accessors return owned data,
and a mutation is exactly one complete-document upsert or one ID delete. An
empty `Sort` or `Highlight` is invalid. V1 deliberately has no `any` value,
generic map, partial-update union, or highlight tuning surface.

### Index definition

```go
type Definition struct {
    Name              IndexName
    Fields            []Field
    DefaultSearchable []FieldName
    DefaultSort       []Sort
}

type Field struct {
    Name       FieldName
    Path       JSONPointer
    Type       FieldType
    Required   bool
    Searchable bool
    Filterable bool
    Sortable   bool
    Facetable  bool
}
```

Portable v1 deliberately has five field shapes: `text`, `keyword`,
`boolean`, signed integer, and keyword arrays. Only text is searchable.
Keywords use `Equal`/`In`, sorting, and term facets rather than pretending
service engines expose one common untokenized full-text analyzer. Booleans and
integers may be filtered and sorted; booleans may be
faceted. Keyword arrays use membership filters and term facets but are not
sortable. Applications use scaled integers for money and canonical keyword
strings for dates until broader scalar semantics have passed real-driver
fixtures.

Analyzed text is not filterable, sortable, or facetable. An application that
needs both full-text and exact behavior declares text and keyword fields over
the same JSON path. Multiple fields may share a path, but a document must
validate against every declared mapping. The definition fingerprint retains
each mapping.

`Path` is an RFC 6901 JSON Pointer and defaults to the escaped top-level field
matching `Name`. Drivers never reinterpret dotted names as native nested
paths. Nested objects may be stored and returned, but portable queries address
only explicitly mapped values. The submitted JSON object remains the returned
document; driver metadata is removed before exposure.

Definitions preserve declaration order for documentation, reject duplicate or
conflicting fields, verify every requested operation against the type matrix,
require every sortable field to be `Required`, require searchable fields to be
text, validate default searchable fields and sorts, and produce a canonical,
versioned fingerprint independent of map iteration. The semantic definition
version is separate from the library version.

Portable document rules are intentionally small:

- fields are optional unless `Required`;
- optional null is indexed as absent but retained in the stored document;
- required null or absence is invalid;
- keyword arrays contain only non-null strings;
- array order and duplicates are retained in the document, while filters and
  facets use set-membership semantics;
- integers use the exact common range `[-(2^53)+1, (2^53)-1]`; and
- strings are valid UTF-8 and are not silently normalized.

Document identity is `Document.ID`, never inferred from the body. A public
`id` property is ordinary data. Drivers keep identity in a reserved physical
field and reject any backend response whose physical identity does not equal
the decoded document identity.

Dates, floating point, locale-specific analyzers, field weights, text arrays,
numeric/date ranges, geo, vectors, and engine-native types remain candidate
capabilities. Phase 0 must prove their cross-driver encoding and observable
semantics before any enters the portable API. The mandatory fixtures also prove
the specified UTF-8 keyword order and required-field sort behavior. Future
fixtures must settle analyzer/version drift, date precision, and binary64
equality rather than relying on prose alone.

### Provisioning and drift

`NewIndex` is side-effect free. It validates a definition but does not create
or alter backend state. Each concrete driver exposes its own administrative
methods because schema updates, tasks, aliases, files, and revisions are not a
portable request-path concern.

GoForj's provisioning command defines the small interface it consumes in its
own package. For v1 it needs only create-if-absent and exact validation of an
existing index. Creation is idempotent for the same complete fingerprint;
mismatch fails closed with `ErrSchemaDrift`. Ordinary application startup
validates required indexes and never mutates them. Each concrete driver derives
a versioned materialized fingerprint from the portable definition plus every
owned physical field and backend setting—searchable/filterable/sortable/facet
configuration, ranking order, pagination window, analyzers, and storage
metadata—and compares it with actual backend state. Persisted portable metadata
alone is not proof of an unchanged index.

Planning, compatible in-place updates, destructive changes, promotion, and
rollback remain driver administration features until two implementations prove
a common workflow. Definitions still carry stable fingerprints and drivers
persist ownership metadata so future tools can diagnose drift without guessing.
Field removal, type changes, and identity changes require rebuild. Adding a
field also requires rebuild unless a future driver can validate every retained
document atomically; sampling does not qualify.

## Public API

### Small construction boundary

The sibling library has no named-engine manager, global driver registry,
engine state machine, or deployment namespace. Those belong to GoForj,
which constructs concrete drivers and stores named `*search.Index` values.

The complete consumer-side boundary is deliberately small:

```go
type Backend interface {
    Search(context.Context, Definition, Query) (Result, error)
    Upsert(context.Context, Definition, Document) (Receipt, error)
    Delete(context.Context, Definition, DocumentID) (Receipt, error)
    Bulk(context.Context, Definition, []Mutation) (BulkResult, error)
    Wait(context.Context, Receipt) (Outcome, error)
}

func NewIndex(backend Backend, definition Definition, opts ...Option) (*Index, error)

func (i *Index) Search(context.Context, Query) (Result, error)
func (i *Index) Upsert(context.Context, Document) (Receipt, error)
func (i *Index) Delete(context.Context, DocumentID) (Receipt, error)
func (i *Index) Bulk(context.Context, []Mutation) (BulkResult, error)
func (i *Index) Wait(context.Context, Receipt) (Outcome, error)
```

`NewIndex` validates and owns a normalized definition and accepts the backend
interface at the point of use. It returns a concrete concurrent-safe `*Index`.
Contexts are explicit first parameters and are never retained in a handle.
`Filter` is also a concrete value with private representation, created only by
constructors such as `Equal`, `And`, and `ContainsAny`; applications and
drivers cannot provide arbitrary implementations. Drivers inspect it through a
read-only view:

```go
type FilterView struct {
    Kind     FilterKind
    Field    FieldName
    Values   []Value
    Children []Filter
}

func (f Filter) View() FilterView
```

Only fields meaningful for `Kind` are populated. Returned slices are clones.
This keeps construction valid while giving external driver packages a complete
tree to compile without type switches on private nodes.

Concrete driver packages return concrete types:

```go
driver, err := blevesearch.Open(ctx, cfg)
index, err := search.NewIndex(driver, definition)
```

A service driver constructor may be lazy when no I/O is needed, while an
embedded `Open` accepts context because it acquires files and locks. Driver
configuration owns transport details and secrets. GoForj owns construction
retry, named engines, App namespaces, runtime activation, close order, and
lookup. Every registered v1 index is required; optional degradation is deferred.
The sibling only requires
that backend calls honor context, are concurrency-safe as documented, and return
`ErrClosed` after concrete driver closure.

Root options are few. `WithObserver` attaches bounded events and `WithLimits`
applies explicit overrides to conservative defaults:

```go
type Limits struct {
    NameBytes          int
    DefinitionBytes    int
    DocumentBytes      int
    JSONDepth          int
    ObjectMembers      int
    ArrayLength        int
    DefinitionFields   int
    JSONPointerDepth   int
    TextBytes          int
    QueryBytes         int
    QueryTerms         int
    QueryFields        int
    FilterNodes        int
    FilterDepth        int
    FilterValues       int
    FilterValueBytes   int
    FacetNameBytes     int
    ResultLimit        uint64
    ResultWindow       uint64
    Facets             int
    HighlightFields    int
    BulkDocuments      int
    BulkBytes          int
}
```

Zero means inherit the documented default; nonzero values override it up to a
documented hard ceiling. This keeps existing option literals forward-compatible
when later releases add a safety limit. V1 defaults `Limit` to 20, caps one page
at 100 hits, and requires `Offset + Limit <= 1000` with checked arithmetic.
Initial driver provisioning configures at least that 1000-hit window; a driver
that cannot do so fails validation rather than silently truncating.

Validation occurs before a backend call. Aggregate definition and normalized
query budgets include names, pointers, filter values, facet names, and collection
overhead; collection counts are checked before cloning. Structural JSON parsing is streaming
or otherwise budget-aware so a deeply nested or member-heavy small document
cannot exhaust memory first. Driver configurations separately bound HTTP
bodies, decoded responses, headers, concurrency, retry policy, and timeouts.

### Mutations and receipts

```go
receipt, err := index.Upsert(ctx, document)
receipt, err := index.Delete(ctx, id)
result, err := index.Bulk(ctx, mutations)
```

Delete is idempotent for a missing document. Upsert replaces the complete
logical document. Portable partial update is deferred because backend merge,
null, array, and field-removal behavior differs.

`Receipt` is a concrete immutable value identifying accepted work:

```go
type Receipt struct {
    // opaque, process-local backend token and bounded metadata
}

func NewReceipt(token []byte) (Receipt, error)
func (r Receipt) Token() []byte

type Outcome struct {
    State      OutcomeState
    VisibleAt  time.Time
    Bulk       *BulkOutcome
    Error      error
}
```

`NewReceipt` is the driver construction boundary. It rejects empty or oversized
tokens and clones them; `Token` also returns a clone. Tokens must be opaque,
bounded, free of credentials and user content, and meaningful only to the
backend that issued them. `Index` records an unexported per-handle identity in
the receipt and rejects a forged receipt or one from another Index before
invoking `Wait`.

A synchronous backend returns an already-complete receipt. `Index.Wait(ctx,
receipt)` is idempotent and concurrency-safe. Terminal state is monotonic.
A successful outcome means the mutation is visible to searches submitted
after `Wait` returns against the same logical index revision. A backend that
cannot prove visibility does not return success; it returns an unknown or
failed outcome. Context cancellation stops waiting but does not imply that the
backend canceled accepted work.

`Wait` returns a non-nil method error only when it cannot produce a trustworthy
terminal outcome now: invalid receipt, context cancellation, or transport/
inspection failure. Once a terminal outcome is known, the method error is nil;
`Outcome.State` is `succeeded`, `failed`, or `unknown`, and `Outcome.Error`
contains the sanitized classification for failed or terminal-unknown work.
`ErrMutationFailed` and `ErrReceiptExpired` therefore appear in
`Outcome.Error`, not as the method error. This preserves `BulkOutcome` even
when accepted work ultimately fails. `VisibleAt` is the root-observed completion
time and is zero for failed or unknown outcomes; it is not a backend clock.

Bulk input is an ordered `[]Mutation` of upserts and deletes. Duplicate
document IDs, including mixed delete/upsert entries, are invalid in v1 because
cross-driver ordering is not portable. The result preserves input order:

```go
type BulkResult struct {
    Receipt   Receipt
    Items     []ItemResult
    Precision ItemOutcomePrecision
}

type ItemResult struct {
    Position int
    ID       DocumentID
    Status   ItemStatus
    Error    error
}

type BulkOutcome struct {
    Items     []ItemResult
    Precision ItemOutcomePrecision
}
```

A non-nil method error means no trustworthy `BulkResult` can be returned:
validation failed before submission, acceptance is wholly unknown, or transport
failed without a recoverable receipt. Known per-item rejection or failure is
data in `Items`, not `ErrPartialFailure`, so ordinary Go error handling does
not discard useful partial results. Initial item states are `rejected`,
`accepted`, or `unknown`; `Wait` may return terminal `succeeded`,
`failed`, or `unknown` states.

`ItemOutcomePrecision` is `per_item` or `batch_only`. A batch-only backend
never fabricates per-item success from aggregate task success. Meilisearch
therefore reports aggregate terminal precision unless its supported API proves
individual outcomes, while Typesense parses every import response line. Drivers
inspect item responses even after HTTP 200, and validation bounds batch count
and bytes before sending.

For a bulk receipt, `Outcome.Bulk` is always non-nil and preserves input
positions. With `per_item` precision it carries terminal item states. With
`batch_only` precision, accepted items remain `unknown` even when the aggregate
outcome succeeds or fails; only submission-time rejections remain individually
known. For a single-document receipt, `Outcome.Bulk` is nil.

Transport loss after possible submission returns `ErrOutcomeUnknown`; when a
backend supplied a trustworthy receipt, it is returned through a typed
`UnknownOutcomeError` accessor rather than a second partially valid result.
Retrying is safe only under complete-replacement/idempotent delete semantics or
an App-owned version policy. `Receipt` contains no document or query content
and has no portable serialization. V1 waits in the process that submitted the
work; after process loss callers replay idempotent work or reconcile from the
authoritative source rather than depending on a portable task-inspection API.

Transport retries belong to concrete drivers because only they know protocol
idempotence and retry guidance. They remain within the caller deadline and do
not retry validation or authorization failures. No driver automatically
replays a mutation after a write could have reached the backend. Drivers use
idempotency keys when available; otherwise transport loss is
`ErrOutcomeUnknown` and callers retain any recoverable receipt or reconcile
from the authoritative source. Retry attempt count and terminal classification are
observable without logging request content.

Backend-native snapshots are outside the portable v1 design. The recovery path
is rebuilding from the authoritative source; GoForj backup documentation must
not describe a backend snapshot as portable source data.

### Query API

```go
type Query struct {
    Text       string
    Fields     []FieldName
    Filter     Filter
    Sort       []Sort
    Facets     []TermFacet
    Highlight []Highlight
    Offset     uint64
    Limit      uint64
}
```

An empty `Text` is a match-all query subject to filters. `Text` is literal
untrusted user text, never a backend query-string program. Drivers escape or
construct native typed queries so punctuation cannot activate boolean, field,
range, wildcard, regular-expression, or scripting syntax. Every compiled term
must be present; phrase, relaxed/OR-like, prefix, fuzzy, and advanced syntax are
optional typed capabilities. Requested fields must be declared searchable. The
index applies definition defaults before backend invocation so every backend
receives a normalized query.

When `Fields` is empty, the normalized definition's `DefaultSearchable` set is
used; an explicitly empty effective set is invalid for non-empty text. Under
the portable rule, every compiled term must match at least one selected field, but
different terms may match different fields. Match-all queries have no portable
relevance score.

Concrete drivers may later expose a precisely named relaxed mode, but
Meilisearch's progressive term dropping is not mislabeled as set OR:
it can stop relaxing once `limit` is filled, making totals, facets, and later
pages differ from the complete union. Every driver also publishes its maximum
compiled query terms. Root byte/term bounds and driver validation reject an
over-limit query before search; no driver may silently ignore trailing terms.
Meilisearch therefore rejects queries compiling to more than ten backend words
for the supported version. Literal-query conformance includes quotes, minus
signs, reversed terms, documents containing only each individual term, facets,
totals, offsets near the portable result-window boundary, and the backend term
boundary.

The v1 filter expression tree supports:

- `And`
- `Or`
- `Not`
- `Equal`
- `In`
- `ContainsAny`
- `ContainsAll`
- `LessThan`, `LessThanOrEqual`
- `GreaterThan`, `GreaterThanOrEqual`
- `Between`
- `Exists`

Values are typed scalars matching the field definition. Invalid comparisons fail before a driver call. Arbitrary raw expressions are absent from the root `Query` type.

`In` means that one scalar field equals one of the supplied values.
`ContainsAny` and `ContainsAll` are the corresponding portable array-field
operations. Their distinct names prevent drivers from inventing inconsistent
array semantics for `Equal` or `In`.

Empty `And` matches all, empty `Or` matches none, `Not(nil)` is invalid, and
empty `In`, `ContainsAny`, or `ContainsAll` is invalid. `Between` is inclusive
on both bounds; callers use the explicit comparison constructors for open
bounds.

Filters use ordinary two-valued Boolean logic. A comparison or membership leaf
on an absent or null field evaluates false. `Exists` is true only for a present,
non-null value, so `Not(Exists(field))` matches both missing and null fields and
`Not(Equal(field, value))` also matches them. `Not`, `And`, and `Or` then apply
normal Boolean truth tables and De Morgan transformations are sound. An empty
array is present and non-null (`Exists` true), but contains no members, so its
membership leaves are false. The normative truth table covers nested
expressions over missing, null, empty arrays, and populated arrays; driver
compilers may not substitute SQL three-valued logic without explicitly
coalescing it to these results.

Portable `Sort` contains only a declared field and direction; relevance is not
a public sort key. At most two field criteria are accepted because the library
always appends normalized document ID as the third key. When `Sort` is empty,
a text query uses backend relevance followed by document ID, while a match-all
query uses `Definition.DefaultSort` followed by document ID, or ID alone when
the default is empty. `DefaultSort` is field-only and has the same two-field
limit. A non-empty `Sort` is field-primary in declared order; each initial
driver must prove its native ranking configuration preserves that order.

Portable ascending order is keyword UTF-8 byte order, `false < true`, and
integer numeric order; descending reverses it. Requiring sortable fields
prevents backend-specific missing/null placement from entering v1. The library appends a
reserved normalized document-ID field as the final tie-break when it is not
already present. Portable ID order is ascending comparison of the original
UTF-8 bytes; physical encoding must preserve that order rather than relying on
backend collation. Driver tests must prove stable pagination under tied primary
sorts, including non-ASCII and mixed-case IDs.

`Hit.SortValues` contains one value per caller-visible field criterion in the
same order. It excludes the hidden document-ID tie-break and is empty for
relevance-only or ID-only ordering.

Offset pagination is the portable baseline. `Limit == 0` selects the configured
default rather than requesting zero hits. Page and result-window validation is
performed after applying that default. Its stable-order guarantee applies
only while the observed index is unchanged; mutations between page requests
may cause duplicates or omissions. Cursor/search-after pagination is an
optional capability because cursor stability, point-in-time views, and encoded
sort state differ materially across engines.

### Results

```go
type Result struct {
    Hits   []Hit
    Total  Total
    Facets []FacetResult
    Took   time.Duration
}

type Hit struct {
    Document   Document
    Score      *float64
    SortValues []Value
    Highlights map[FieldName][]Fragment
}

type Total struct {
    Value    uint64
    Relation TotalRelation
}
```

`TotalRelation` is `exact`, `estimated`, or `lower_bound`. A missing score is distinct from zero. Scores are not normalized across drivers and must not be persisted as business state.

`Took` is end-to-end driver-call duration measured by the root package;
driver-reported execution time, when available, is separate diagnostic
metadata. Timeout, canceled shard, and partial-backend responses return an
error. V1 does not expose partial search results because one global flag cannot
describe independently incomplete hits, totals, and facets safely. A future
capability needs per-surface completeness backed by two real drivers.

`Document.ID` is the hit identity; duplicating it on `Hit` would create an
avoidable disagreement state. Nil and empty collections are semantically
equivalent. HTTP examples or generated DTOs may normalize them at their own
serialization boundary when a stable JSON shape is required.

### Facets

Portable v1 facets are term distributions for keyword, boolean, and keyword
array fields. Requests and results are explicit:

```go
type TermFacet struct {
    Name  string
    Field FieldName
    Limit uint64
}

type FacetResult struct {
    Name      string
    Field     FieldName
    Buckets   []Bucket
    Precision CountPrecision
    Truncated bool
}

type Bucket struct {
    Value Value
    Count uint64
}
```

Names are unique per query. Buckets are ordered by descending count then the
field's portable scalar order. `Limit` uses the root bound when zero, and
truncation is reported separately from count precision.

Facet counts are computed over the complete query/filter result set before hit
pagination. Missing and null values do not form a bucket in v1. A multivalue
document contributes at most once to each matching bucket even when its stored
array contains duplicates. Scalar comparison and Unicode keyword equality are
the same as filter semantics.
Drivers may return exact or estimated counts, but may not change these counting
rules. A checked-in facet truth table covers null/missing, duplicate array
values, ties, truncation, and filter interaction.

Caller-defined numeric/date range buckets are deferred. Any eventual API must
return hits, totals, term facets, and all inclusive/open/overlapping range
buckets from one backend snapshot. A driver may not emulate them with
uncoordinated searches. Request/result types are not published until at least
two drivers pass adjacent-boundary, overlap, missing/null, pagination, and
concurrent-mutation fixtures.

Disjunctive-facet UI behavior is not implied by one query: every requested
facet sees the query's complete filter, including a filter on its own field.
Applications or a future multi-search helper issue separate filter variants
when they want self-excluding/disjunctive counts.

### Highlights

The public result never treats backend markup as trusted HTML:

```go
type Fragment struct {
    Segments []Segment
}

type Segment struct {
    Text    string
    Matched bool
}
```

Drivers request collision-resistant internal markers, validate returned markup, decode it into segments, and reject malformed output. UIs escape every segment's text and apply their own markup to matched segments. This prevents stored content from becoming an HTML injection path.

### Errors

Stable sentinels and typed wrappers should cover:

- `ErrNotFound`
- `ErrIndexNotFound`
- `ErrUnsupported`
- `ErrInvalidDefinition`
- `ErrInvalidDocument`
- `ErrInvalidQuery`
- `ErrSchemaDrift`
- `ErrConflict`
- `ErrOutcomeUnknown`
- `ErrUnavailable`
- `ErrUnauthorized`
- `ErrRateLimited`
- `ErrMutationFailed`
- `ErrReceiptExpired`
- `ErrClosed`

Context cancellation and deadline errors preserve `errors.Is` behavior. Public
unwrap chains contain only library sentinels and sanitized typed errors; a raw
driver/backend error is never exposed merely for diagnostics. Errors identify
the logical index, operation kind, receipt when safe, and allowlisted backend codes without
including credentials, full documents, raw query content, request/response
bodies, URLs containing secrets, or backend prose that has not been proven
safe. `ItemResult.Error`, joined errors, formatting with `%+v`, and receipt
outcomes obey the same rule.

Drivers may retain a native cause only in an internal ephemeral diagnostic
record guarded by an explicit privileged debug hook whose default is disabled
and whose sink applies independent redaction; it is not reachable through
`errors.Unwrap`. Bulk parsers construct safe item errors from allowlisted codes,
positions, and document IDs only when ID disclosure policy permits. Tests inject
credentials, documents, query text, and hostile backend messages into every
error path and recursively inspect formatting and unwrap trees.

## Backend contract and optional capabilities

The `Backend` interface shown above is the entire portable driver SPI. It
belongs to the consumer package and concrete driver constructors return
concrete values. This follows normal Go interface ownership: an implementation
does not register itself globally or implement lifecycle and administration
methods that a request-path consumer does not need.

The root `Index` performs domain validation, normalization, limits,
observation, and safe error mapping. The backend translates already validated
definitions, documents, mutations, and queries. It must be concurrency-safe,
honor context, never mutate inputs, sanitize returned errors, and accurately
report receipt and bulk precision. A closed concrete backend returns
`ErrClosed`.

Optional behavior starts in a concrete driver package. A small interface moves
to the root only after at least two drivers and a real consumer need the same
semantics. V1 does not publish guessed interfaces for readiness, diagnostics,
native access, operation listing, conditional mutation, schema planning,
generation administration, cursor search, suggestions, synonyms, geo, vector,
or hybrid search. V1 exposes none of that behavior through `*Index`. Code that
intentionally couples to a driver constructs and retains the concrete driver
separately; ordinary GoForj accessors remain portable. There is no generic
backend escape hatch.

Provisioning follows the same rule. GoForj owns its command-facing interface
and adapts concrete driver administration methods. A service driver can expose
tasks and an embedded driver can expose file operations without forcing either
model into the portable `Backend`.

Capabilities are generated documentation and test metadata, not one giant
runtime interface. Each advertised cell names its exact behavior, minimum
backend version, fixture, and limitations. Native access remains
driver-package-specific; the root never accepts raw query JSON or
`Native(any) any`.

## Driver Plan

### Tier 1: initial supported drivers

#### Bleve

Package: `driver/blevesearch`

Role:

- default embedded driver;
- zero external services;
- persistent indexes below a configured runtime root;
- optional in-memory mode for tests and ephemeral tools.

Bleve supports the initial lexical contract, sorting, highlighting, and facets.
Its broader numeric, date, geo, and vector behavior remains driver-specific
until shared capability fixtures exist. The driver must serialize destructive
index-open/create operations per physical path and define multi-process
behavior clearly: an embedded index is process-local deployment topology, not
a shared network service.

#### Meilisearch

Package: `driver/meilisearch`

Role:

- first service-backed developer and production option;
- primary proof for asynchronous operations;
- atomic index-swap support for rebuild promotion.

Every settings or document mutation returns a Meilisearch task. The driver maps
task state to `Receipt` and reports aggregate rather than fabricated per-item
terminal precision when the task API lacks item outcomes. Filterable, sortable,
facetable, and searchable attributes derive from the definition.

Meilisearch index swap is atomic but asynchronously enqueued and has no
expected-current fencing precondition. V1 exposes it only to the offline admin
workflow and does not claim online-safe promotion.

#### Typesense

Package: `driver/typesensesearch`

Role:

- second service-backed option;
- independent validation of schema, filtering, faceting, highlighting, aliases, and bulk partial-failure semantics.

Typesense bulk import can return HTTP success while individual lines fail. The driver must parse every result line. Collection aliases can support promotion where the deployed version provides the required atomicity; the capability contract must reflect the tested behavior, not an assumption.

### Driver roadmap, not v1 design

Later drivers are ordered by likely value, but their schemas, clients, and
capabilities are deliberately not designed here. Each begins with a feasibility
corpus and its own short design after real demand:

| Order | Candidate | Why evaluate |
| --- | --- | --- |
| 1 | PostgreSQL | common no-new-service production topology |
| 2 | SQLite FTS5 | embedded option for Apps already using SQLite |
| 3 | Redis Search | reuses a common service while remaining distinct from Cache |
| 4 | OpenSearch | widely deployed scalable search |
| 5 | Elasticsearch | similar domain but independent client/version contract |
| 6 | MySQL | valuable after the SQL-driver contract has evidence |
| 7 | Algolia | hosted option requiring protected live validation |

MongoDB Atlas Search, Azure AI Search, Solr, Manticore, Sonic, Vespa, and
cloud-specific products remain evaluated candidates. Generic SQL LIKE is not
a search driver. This list is comprehensive planning, not a promise to freeze
unspiked implementation details into the root API.

## Capability And Guarantee Matrix

The repository must generate a checked-in driver matrix from machine-readable
scenario results. Driver declarations select expected capabilities, but cannot
mark one supported without a matching executed conformance result for every
advertised backend version. At minimum the matrix records:

- supported backend versions;
- lexical search;
- filters and expression limits;
- stable sort;
- page and result-window limits;
- exact/estimated totals;
- term facets;
- highlights;
- synchronous/asynchronous mutations;
- bulk partial results;
- create-only bootstrap;
- actual-state materialized fingerprint validation;
- maximum negotiated query, facet, document, and batch limits;
- readiness semantics;
- concurrent client safety;
- multi-process support; and
- integration scenario names proving each claim.

Range facets, cursor/search-after, multi-search, swaps/promotion, conditional
mutation, and other deferred areas are added to the generated matrix only when
their public capability ships. The design may track research results separately
without presenting a column of permanently false promises as API inventory.

A missing expected scenario result is a failure. Capability-gated skips must name the matrix entry that permits the skip.

## Index Synchronization

### Normal application writes

The search library does not subscribe to databases or event buses. App code
maps authoritative records into complete replacement documents. It may write
inline or enqueue only an entity ID, then reload current state before indexing.
A transactional outbox is required when losing the transaction-to-event handoff
would be unacceptable.

Reloading alone does not order concurrent jobs: an older job may finish last.
Portable v1 therefore promises idempotence, not last-write-wins convergence.
An App that needs immediate convergence must serialize work per entity through
receipt visibility or use a concrete driver's proven version-precondition
feature. Otherwise it schedules reconciliation from the authoritative source.
Generated documentation states this tradeoff and never implies that per-process
queue concurrency is a global lock.

### Offline rebuilds

Reindexing is a GoForj workflow, not part of the search request API. Each App
registers an authoritative source for a logical index:

```go
type ReindexSource interface {
    Begin(context.Context) (Snapshot, error)
}

type Snapshot interface {
    Fingerprint() SourceFingerprint
    Next(context.Context, Checkpoint, int) (SourceBatch, Checkpoint, bool, error)
    Close() error
}
```

`Next` is pull-based so the coordinator can submit one bounded batch, wait for
its receipt, persist the returned checkpoint, and only then request more. A
callback iterator cannot safely express that acknowledgement boundary. A
checkpoint is opaque, bounded, serializable, scoped to the source fingerprint
and rebuild identity, and resumes after the last durably accepted batch. The
source guarantees deterministic traversal for one fingerprint. On restart,
`Begin` must reproduce that fingerprint; a changed fingerprint refuses resume
and starts a clean target rather than mixing snapshots. Replaying the last batch is
safe because upserts replace complete documents and deletes are idempotent.
`done` is true only when the returned batch is the final batch; an empty,
non-final batch is invalid.

V1 supports offline rebuild only. Bleve proves maintenance with its
single-process ownership lock. A service-backed rebuild requires an explicit
operator-attested maintenance procedure; the command must not claim it fenced
other application replicas. The workflow is:

1. validate the source and definition;
2. create a new owned physical index through the concrete driver's admin API;
3. pull, validate, submit, wait, and checkpoint bounded batches;
4. compare counts where meaningful and run App-owned verification queries;
5. promote only through a driver operation whose atomicity has been integration
   tested, otherwise print the manual cutover action; and
6. retain or remove the previous index according to explicit policy.

Durable progress records the driver, physical target, definition and source
fingerprints, checkpoint, and terminal state outside the checkout. Receipts are
never checkpointed: progress advances only after `Wait`, and a crash in between
replays the idempotent batch.
Resume revalidates all identity before writing. The command never derives a
destructive target from a user-supplied physical name; it uses persisted
ownership metadata and refuses an active or unknown target.

Online rebuild, catch-up feeds, dual-write, fenced leases, promotion CAS,
rollback freshness, and a distributed maintenance coordinator are a separate
control-plane design. V1 publishes no interfaces for them. They may be designed
only when a real deployable coordinator and at least two drivers can pass crash,
stale-holder, credential-isolation, and multi-replica tests.

## GoForj Component Model

### Selection and dependency closure

Search is optional and initially unselected. It does not require Database, Cache, Events, Queue, or Storage.

The default selected driver is Bleve. Service-backed driver support is compiled in only when selected in the project configuration. Adding a supported driver is additive; removing support requires the same explicit transition and preflight discipline as other primitives.

Bleve is a single-process topology driver. The combined `app run` host may
share one in-process engine among HTTP, jobs, and scheduler. Separate
`http:serve`, `queue:work`, `schedule:run`, multiple App binaries, or horizontal
replicas must not open the same Bleve physical root concurrently. The driver
uses an OS-level ownership lock and fails readiness on a second writer. GoForj
preflight warns or fails when selected process topology is known to require a
service-backed driver. In particular, queue-based indexing in one process and
HTTP querying in another requires Meilisearch, Typesense, or another shared
driver rather than Bleve.

Search should appear as one component choice, not as a new normal wizard stage. Advanced driver selection can be offered after project creation through a resource maker or configuration command.

### Runtime activation

Search is not registered as an unconditional App lifecycle startup
hook. Current generated commands invoke App startup globally, so doing that
would contact Search for unrelated CLI commands and would prevent validation
commands from inspecting unhealthy state.

Generated runtime entry points activate Search explicitly and narrowly:

- the long-running combined App and HTTP server call `Search.Start(ctx)` only
  when their selected App has registered Search indexes;
- queue workers activate it only when the rendered worker composition contains
  indexing/search jobs;
- scheduler and other runtimes activate it only for registered tasks that
  declare the dependency;
- `search:status`, `search:validate`, `search:bootstrap`, and other Search commands
  call purpose-specific readiness/administration methods that can inspect
  missing or drifted state without first requiring healthy indexes; and
- every unrelated command receives at most inert lookup wiring and never calls
  driver readiness.

Each activating entry point owns matching bounded shutdown, while a never-used
driver is never opened. Render tests instrument driver factories and
assert exact initialization counts per command. A future general command-scoped
lifecycle facility may remove this special composition, but the Search design
does not assume it already exists.

The GoForj runtime owns one small state machine: `new`, `starting`, `ready`,
`stopping`, `closed`. Concurrent `Start` calls share one result. If any required
engine fails, already-opened engines close in reverse order so Bleve locks are
released. `Shutdown` prevents new calls, waits for or cancels in-progress start
within its context, drains admitted calls, and closes once. Restart after
shutdown is forbidden. Tests race Start/Shutdown, inject partial-start failure,
and prove all paths release files, goroutines, and service clients.

### Generated environment

Illustrative owner environment:

```env
SEARCH_SUPPORTED_DRIVERS=bleve,meilisearch
SEARCH_DRIVER=bleve
SEARCH_PREFIX=app
SEARCH_BLEVE_ROOT=_data/search

SEARCH_MEILISEARCH_URL=http://127.0.0.1:7700
SEARCH_MEILISEARCH_API_KEY=
```

Rules:

- `SEARCH_SUPPORTED_DRIVERS` is a generated compile-time support inventory.
- `SEARCH_DRIVER` selects the runtime default engine.
- named engine overrides follow existing scoped env conventions.
- every generated App receives a stable owner identifier and owner-scoped
  physical prefix; named Apps never share the default App's Bleve directory or
  service index namespace even when logical index names match.
- `.env.example` contains safe inventory without secrets.
- `.env.testing` uses isolated temporary Bleve roots unless a rendered integration profile explicitly selects another driver.
- API keys and cloud credentials never appear in safe committed defaults.
- production guidance separates runtime search/write credentials from
  provisioning administration credentials. V1 passes the admin credential only
  to the explicit bootstrap/manual maintenance job, never ordinary App replicas.
- engine URLs use host-accessible defaults, not Docker-only hostnames.

### Generated App surface

Generated composition stays ordinary Go:

- `app/searches.go` owns typed index-name constants, definitions, and a
  function returning GoForj-local registrations;
- named Apps use `app/<name>/searches.go`;
- Wire constructs the concrete drivers and one runtime registry;
- runtime discovery and commands consume that same registry; and
- Lighthouse adapters are conditional on Lighthouse selection.

```go
const ProductsIndex search.IndexName = "products"

products, err := app.Search().Index(ProductsIndex)
```

Every registered v1 index is required. Duplicate logical names, unknown engines,
and definition/name mismatch fail during construction. There is no stable
registration ID, machine manifest, per-index generated accessor, or AST-enforced
configuration language. Applications may assemble registrations through normal
Go helpers and shared values.

### Later maker command

The optional maker starts small:

```text
forj make:search-index products
forj admin make:search-index audit-events
```

It appends a conventional typed name constant, definition, registration, and
focused test to the selected App-owned file. It does not infer a production
schema from a database table, generate a document projection, or create
event/job/reindex scaffolding in v1. Re-running is additive and detects duplicate
logical names, but ordinary manual Go edits and helper functions remain valid;
runtime construction is the final validator.

Disabling Search or removing a driver is preflighted. It refuses while
registrations or imports still depend on the component/driver and removes only
marker-proven generated wiring and environment entries. It never deletes
App-owned definitions, local index data, or external physical indexes.

### Commands

The initial generated surface is deliberately small:

- `search:list` reads generated definitions without initializing a backend;
- `search:status [index]` reports connection and exact fingerprint state;
- `search:validate [index]` fails on missing or drifted state;
- `search:bootstrap [index]` creates only a missing index and refuses mismatch;
  and
- `search:reindex [index]` appears only with the offline-rebuild phase.

Commands initialize only the selected driver and never pass through global App
readiness. Build and render do not contact search infrastructure. Bootstrap is
an explicit provisioning job: it uses a durable idempotency identity for
asynchronous services, waits for settings, rereads the complete fingerprint,
and never races from ordinary application startup. In-place apply, rollback,
cleanup, and task-browser commands are deferred until their workflows exist.

### Docker Compose catalog

The always-rendered optional developer-service catalog should eventually include exact profiles for:

- `meilisearch`
- `typesense`
- `redis-stack`
- `opensearch`
- `elasticsearch`

Only the selected local driver seeds its profile in the initial owner environment. Catalog availability is not component selection. Services use pinned, tested versions, health checks, persistent named volumes, and host-accessible ports. Heavy OpenSearch and Elasticsearch profiles should not start merely because Search is enabled with Bleve.

## HTTP API Index Integration

Search must not create a parallel HTTP contract system.

No Search-specific API-index generator or analyzer extension is planned unless
an executable typed-controller fixture reproduces a defect in `webindex`.
Ordinary typed DTO compatibility is the requirement, not a new integration
surface.

Generated search controllers should use ordinary `web.Context`, registered routes, explicit request DTOs, and explicit response DTOs. The Forj API Index must be able to analyze representative endpoints containing:

- query text;
- allowed filter fields;
- pagination;
- typed hits;
- total relation;
- facets; and
- structured highlights.

Do not expose `map[string]any` as the default HTTP response solely because the underlying search document is JSON. App controllers should project indexed documents into typed public DTOs so OpenAPI remains useful and private indexed fields cannot leak.

Add a rendered integration fixture whose route returns a generic or projected Search result and verify:

- Manifest v2 contains the route and selected App ownership;
- OpenAPI contains the hit document schema, facets, total relation, and highlight segments;
- diagnostics remain deterministic;
- another App's Search routes do not leak into the selected App index; and
- Search-disabled Apps publish no Search routes or schemas.

If `webindex` cannot resolve the chosen generic result form, prefer a typed App DTO or improve `webindex`; do not weaken the public HTTP schema to bypass analysis.

## Sibling README API Index And Examples

The Search sibling README should follow the current source-generated documentation pattern:

- exported APIs have complete GoDoc comments;
- public functions and methods use stable `@group` annotations;
- examples live beside authoritative GoDoc or in executable example sources;
- expected output appears immediately after the producing call with one `//` line per output line;
- `docs/readme` regenerates the marked README API index;
- `docs/examplegen` emits standalone compile-tested examples;
- test-count, coverage, benchmark, and driver-matrix embeds are generated from authoritative inputs; and
- running every generator twice produces no diff.

Initial API index groups should include:

- Construction
- Backend
- Definitions
- Documents
- Values
- Mutations
- Queries
- Filters
- Results
- Receipts
- Observability
- Testing

Driver constructors and configuration examples should be indexed with qualified package names. Testing-only internals remain excluded from the primary consumer index while `searchfake` and `searchtest` have their own package documentation.

Examples should include at least:

- create and validate a definition;
- upsert and wait for visibility;
- bulk upsert with partial-failure inspection;
- text search;
- match-all with filters;
- compound typed filters;
- stable sorting and pagination;
- term facets and a separately gated range-facet example when implemented;
- rendering structured highlights safely;
- fake assertions;
- observer usage;
- Bleve construction;
- Meilisearch construction; and
- Typesense construction.

## Authorization, Tenancy, And Security

Search indexes often contain denormalized private data. The framework must not treat search as an authorization boundary.

Rules:

- Authorization constraints participate in the backend query before hits, totals, highlights, or facets are computed; post-filtering a page is not authorization.
- Tenant/owner filters are injected by trusted service code and cannot be overridden by client filters.
- Generated HTTP endpoints expose an allowlist of searchable, filterable, sortable, and facetable fields.
- Arbitrary backend query strings are not accepted from HTTP request parameters.
- Search administration credentials stay server-side.
- Direct browser search keys are not generated by default, even when a provider supports scoped keys.
- Query logs and inspect events omit full document bodies and redact or hash sensitive terms by policy.
- Highlight text is untrusted and escaped by renderers.
- Driver HTTP clients validate TLS by default; insecure TLS requires an explicit development-only setting.
- Index prefixes isolate projects and tests. Tests use unique run-scoped suffixes.
- Destructive index operations resolve and display the exact physical targets before execution.
- Tenant-local IDs are encoded with an immutable tenant namespace, or the App proves IDs are globally unique.
- App-owned upsert and delete services validate mandatory tenant identity; a caller cannot mutate another tenant by guessing its ID.

V1 keeps authorization decoration in the App. The root provides immutable,
validated filter values and safe `And` composition, but no alternate Index or
Searcher wrapper. App-owned typed services inject the mandatory predicate
before calling `Index.Search`; generated HTTP controllers expose only those
services. This preserves one concrete public handle and avoids a wrapper that
silently loses optional behavior. Direct tests prove caller filters cannot
replace or negate the mandatory predicate and that the combined tree is
revalidated against complexity limits.

Tenant identity and mutation authorization remain App responsibilities, as
they depend on domain principals and ownership rules. Generated examples show
an App service that constructs a collision-safe tenant-qualified `DocumentID`
from trusted tenant and local IDs, validates the immutable tenant field on
upsert, and tenant-qualifies delete rather than accepting an unscoped ID. A
document cannot change tenant in place; moving it is an authorized delete plus
create. Globally unique IDs prevent collisions but do not grant access. App
integration tests attempt forged prefixes, mismatched document tenants, and
cross-tenant deletes. The sibling library supplies safe ID-encoding helpers and
mandatory filter composition, not a domain-specific `MutationPolicy` API.

V1 creates no portable snapshot or rollback promise. Applications own source
retention and erasure policy; active-index deletion is waited to visibility,
and offline rebuild sources must exclude erased records. Any future retained
generation or snapshot feature requires a separate enumeration and verified
erasure design before GoForj may claim deletion is complete.

## Observability

### Sibling observer contract

Expose an observer event with bounded metadata:

```go
type Event struct {
    Kind          string
    ReceiptID     string
    Phase         EventPhase
    Index         IndexName
    Duration      time.Duration
    DocumentCount int
    HitCount      int
    TotalRelation TotalRelation
    ErrorCode     string
    Outcome       string
}
```

Observers receive context for correlation. They do not receive raw backend
errors, API keys, full documents, full queries, or backend responses. Query
shape metadata may include field names, filter node count, requested facets,
offset, and limit. Submission and terminal visibility are separate events
correlated by a bounded one-way digest of the receipt token when available, so
the token itself is never observable and request duration is not
confused with time until visible.

Observer invocation is synchronous after state is known but outside internal
driver locks. Registration order is delivery order. Observer panics are
recovered and reported through an optional safe hook; they never change search
outcomes. `EventPhase` distinguishes `submission`, `terminal`, and
`search_complete`; mutation phases carry the same bounded receipt ID when
one exists. A `MultiObserver` invokes every observer and reports joined observer
failures through that hook.

The synchronous default is intentionally honest: a blocking observer can delay
the API return after the backend has accepted work, although it cannot undo or
hold the internal driver operation. The library documents observer duration
separately from request/visibility duration. Exporters that cannot tolerate
this use a provided bounded asynchronous adapter with configurable capacity,
non-blocking enqueue, dropped-event metrics, FIFO order for retained events,
panic isolation, and caller-owned `Shutdown(ctx)` flushing. It never uses an
unbounded goroutine or queue. The Index does not own the adapter's lifecycle.

### GoForj metrics

Framework integration should instrument:

- search request count and duration;
- result count;
- mutation and bulk document count;
- mutation receipt latency until visible;
- partial failure count;
- schema validation and bootstrap outcomes;
- reindex documents, bytes, duration, failures, and cutover outcome;
- engine readiness.

Labels must remain bounded: App, logical runtime, driver, engine, logical index,
call kind, and outcome. Never label by query, receipt, document ID, tenant,
error string, or physical generation.

### Lighthouse and Inspects

Lighthouse should eventually expose:

- configured engines and logical indexes;
- definition fingerprints and drift state;
- readiness and backend version;
- document counts where supported;
- safe links to driver-local developer services.

Request, job, scheduler, and CLI inspects should annotate search calls with
bounded metadata. Root indexing jobs show index, document count, a sanitized
receipt ID when available, wait duration, and result, but not document bodies.

## Testing Strategy

### Contract and fake packages

`searchtest` is a package in the root module. Its reusable suite covers
definition and document validation, semantic JSON round-trip, replacement
upsert, idempotent delete, receipt visibility, ordered bulk outcomes, literal
all-terms search, filters, deterministic sort, pagination, totals, term facets,
structured highlights, context cancellation, concurrent use, and safe errors.
Every driver runs the applicable suite unchanged; capability-specific suites
are additive and named in the generated matrix.

Administration stays out of the production interface. Driver tests supply a
test-only provisioner:

```go
type Provision func(context.Context, Definition) (*Index, func(context.Context) error, error)

func RunBackend(t *testing.T, provision Provision)
```

The driver-owned callback creates a uniquely named, already provisioned Index
and cleanup function using its concrete admin API, temporary directory, or
Testcontainer. The runner registers cleanup before executing scenarios and
reports cleanup failure. `searchtest` knows nothing about backend creation or
deletion.

Fixtures cover duplicate JSON keys, structural limits, missing/null values,
integer boundaries, invalid and zero Value/Mutation/Sort/Highlight shapes,
empty expressions, array membership,
Unicode, literal operator characters, query-term limits, tied sorts, malformed
highlights, and backend identity mismatch. They also cover two field sorts plus
the hidden ID key, relevance versus field-primary ordering, zero/default page
size, offset overflow, the result-window edge, oversized filter-value lists,
and aggregate name/query/definition budgets. Facet fixtures cover pagination
independence, multivalue deduplication, ordering, ties, precision, and
truncation.

`searchfake` is also a root package. It records calls, supports queued results
and failures, respects context, and offers deterministic token matching for
service tests. It does not pretend to reproduce production ranking or backend
administration.

### Unit, race, fuzz, and benchmarks

Root tests directly cover every validation branch and error classification.
Race tests stress concurrent calls, waits, observation, filter composition, and
backend close. Fuzz targets cover names, JSON validation and budgets,
fingerprints, filter normalization, query limits, highlight parsing, bulk
responses, receipt identity checks, and error redaction. Benchmarks measure
normalization, encode/decode, validation, batching, observation, and the Bleve
vertical slice with corpus and allocations reported.

### Real-driver integration

A tooling-only `integration` module owns Testcontainers scenarios. Initial CI
runs pinned Meilisearch and Typesense versions; later driver modules add their
own pinned PostgreSQL, Redis Stack, OpenSearch, Elasticsearch, or MySQL lanes.
Bleve and SQLite use unique temporary directories. No render or test data is
created inside a GoForj checkout.

Fixtures wait for real readiness, use random ports and run-scoped prefixes,
capture redacted logs on failure, and clean up through test lifecycle hooks.
Tests are not internally retried. The initial matrix covers the shared
contract, async receipt success/failure/expiry, known and unknown bulk outcomes,
create-if-absent and materialized-setting drift, restart, transport loss before
and after possible write acceptance, deadlines, concurrent operations, size
boundaries, stable pagination, and isolation between two Apps using the same
logical name.

Service HTTP tests additionally cover TLS and authentication failure, redirect
credential isolation, compressed and decoded size limits, hostile error bodies,
oversized headers, slow or partial responses, rate-limit classification, and
secret/document/query redaction. The supported-version table is generated from
the exact versions exercised in CI; an untested version range is not advertised.

### CI quality gates

For every relevant module CI runs the minimum supported and stable Go versions,
`go mod tidy -diff`, vet, unit tests, race tests, and `GOWORK=off` compilation.
It also verifies generated API docs and examples are unchanged, compiles and
runs examples, compiles integrations without containers, runs per-driver
Testcontainers lanes, checks vulnerability/license policy, verifies the module
manifest, and regenerates the capability matrix. README installation is added
only when a driver's module, tag, docs, contract suite, and integration lane all
exist.

## GoForj Validation

Framework integration requires more than sibling unit tests.

Add focused render profiles for:

- Search disabled;
- Bleve only;
- Meilisearch only;
- Typesense only;
- multiple supported drivers with one runtime selection;
- Search with Database but without Queue/Events;
- Search with Queue and Events;
- Search in one App but absent from another App;
- CLI-only App with Search commands;
- Web API/UI App with a typed search endpoint; and
- the largest supported generated composition.

For each relevant nested `go.mod`, run tests and vet independently. Rendered integration tests use `/tmp`, exercise Compose/Testcontainers where needed, and include a `GOWORK=off` pass proving published modules rather than local replacements are selected.

Validation should prove:

- `internal/coredeps/modules.go`, `scripts/generate-renderwarm.sh`,
  `tools/renderwarm/go.mod`, root/integration modules, and published-module
  generator fixtures all use the intended released Search versions;
- generated env files remain synchronized and secret-safe;
- rerender is idempotent;
- disabling Search produces truthful absence;
- additive driver enablement preserves App-owned definitions;
- generated Wire compiles;
- `forj build` succeeds;
- API index and OpenAPI artifacts are correct;
- bare runtime launch starts only required engines;
- unrelated CLI commands do not contact Search;
- `search:status`, `search:bootstrap`, `search:validate`, and `search:reindex`
  initialize only their purpose-specific dependencies and do not pass through
  global runtime readiness first;
- Lighthouse reports every required engine independently while readiness still
  fails if any registered index is unavailable; and
- frontend starter kits can consume typed results with their documented JSON shapes.

The `goforj-docs` pass adds Search to the library ingestion registry and
library catalog, publishes the framework guide plus env/CLI/generated-file
references, updates navigation, runs ingestion/generator tests, and completes a
site build with working generated API links. It also updates `docs/drivers.md`,
`bin/collect-proof-stats.mjs`, generated proof data, and their checks so the
primitive/library totals and the three initial Search drivers cannot remain
self-consistently stale.

The API-index fixture instantiates a concrete type such as
`Page[ProductSearchDTO]`; it does not expose raw `Document` or the internal
`Filter` tree. OpenAPI documents the route's public allowlisted filter and
sort vocabulary explicitly.

## Compatibility

### Deployments and persisted indexes

Runtime startup validates the exact expected definition fingerprint and fails
closed on drift. V1 does not coordinate mixed-schema rolling deployments.
Compatible application-only releases may roll normally; a definition change
requires an explicit offline provisioning/rebuild procedure before runtimes
using the new fingerprint serve traffic.

Changing drivers always rebuilds from the authoritative source. There is no
portable cross-backend atomic cutover. Embedded Bleve format compatibility is
documented per supported version; an incompatible library upgrade reports that
a rebuild is required rather than rewriting derived data during construction.

Compatibility is classified separately for source/API, GoForj configuration,
persisted derived indexes, runtime semantics, operational migration, and
minimum Go version. A dependency version change is not itself a breaking
change; the release notes identify the concrete supported behavior that changes.

## Release Model

The root is one published module containing `search`, `searchtest`, and
`searchfake`. A driver becomes a nested published module only when its
dependency weight justifies isolation. Docs, examples, benchmarks, integration,
and generation tools share one unpublished tooling module.

The checked-in module manifest and release script inventory every `go.mod`,
verify dependency direction, run generation and quality gates, publish the root
before dependent driver modules, wait for proxy/checksum availability, and test
downloaded modules with `GOWORK=off`. GoForj consumes a release only after all
drivers it selects are independently resolvable.

## Implementation Phases

### Phase 0: executable feasibility

- Build one small corpus for the proposed v1 semantics and hostile boundaries.
- Spike Bleve and Meilisearch end to end: literal all-terms text, keyword
  equality, boolean/integer filters, keyword arrays, stable ID tie-break,
  stored-body round-trip, structured highlights, totals, term facets, receipts,
  and bulk precision.
- Demote a driver if it cannot prove mandatory ordering or encoding; keep
  candidate features optional rather than compensating with a larger abstraction.
- Validate the API against the existing GoForj sample applications.

### Phase 1: smallest useful vertical slice

- Create the root module, docs, examples, and tooling module; extract
  `searchtest` and `searchfake` only after the Bleve slice proves the boundary.
- Implement `Index`, `Backend`, definitions, documents, query/filter/result
  values, receipts, limits, safe errors, and observation.
- Implement Bleve and pass unit, race, fuzz, benchmark, and conformance gates.
- Generate the README API index from GoDoc/examples and verify a second
  generation has no diff.

### Phase 2: service semantics

- Add Meilisearch with a pinned Testcontainer.
- Prove async receipt lifecycle, unknown write outcomes, definition bootstrap
  and drift, transport security, and aggregate bulk precision.
- Adjust the root only when the second architecture demonstrates a portable
  need.

### Phase 3: independent validation

- Add Typesense with its pinned Testcontainer.
- Prove item-level bulk failures, schema handling, filtering, facets,
  highlights, and deterministic pagination.
- Freeze v1 only after all three drivers pass the same core suite.

### Phase 4: thin GoForj integration

- Add optional component selection, namespaced concrete driver construction,
  lifecycle, App-owned definitions, typed name constants plus lookup, and minimal status,
  bootstrap, and validation commands.
- Preserve sibling conventions for generated environment inventory, executable
  examples, API-index/OpenAPI output, Compose profiles, focused renders, and
  largest-composition validation.
- Defer makers, broad dashboard surfaces, and clever capability forwarding
  until repeated use justifies them.

### Phase 5: offline rebuild

- Add the pull-based App source, durable checkpoints, bounded batching,
  verification, active-index erasure guidance, and truthful maintenance UX.
- Keep online cutover unavailable pending a separate control-plane design.

### Later phases

Add PostgreSQL and SQLite FTS5, then Redis Search, then OpenSearch and
Elasticsearch in heavier lanes. Evaluate MySQL, Algolia, date/float/locale,
range facets, cursor pagination, suggestions, and vector/hybrid behavior from
demonstrated demand and executable fixtures.

## Acceptance Criteria

The initial sibling is complete when:

- the root and initial driver modules are independently publishable and
  resolvable with `GOWORK=off`;
- Bleve, Meilisearch, and Typesense pass the same portable contract;
- their exact supported versions and differences appear in the generated
  capability matrix;
- receipts and bulk precision cannot turn unknown or known item failure into
  success;
- create-if-absent is safe and actual owned backend-setting drift fails closed;
- domain and structural limits reject oversized work before backend calls;
- term facets, structured highlights, literal queries, bounded stable pagination, and
  error redaction pass real-driver fixtures;
- GoDoc examples compile and run with human-readable output, and README/API
  index generation is idempotent;
- every relevant module passes tidy, vet, test, race, minimum/stable Go, and
  published-module checks; and
- an application can change among the three backends without changing its
  domain search code.

The GoForj integration is complete when disabled projects have no Search
residue; selected drivers follow normal component and environment policy;
definitions and typed name constants survive rerender; unrelated commands
do not initialize Search; status/bootstrap/validation are useful and safe;
typed endpoints retain API-index and OpenAPI output; two Apps can reuse a
logical name without physical collision; and focused plus largest-composition
renders pass from published modules. The core dependency catalog, render-warm
surfaces, root/integration pins, and generator fixtures resolve those published
modules, and `goforj-docs` ingestion plus its production site build pass.

The offline-rebuild phase is complete when pull checkpoints survive process
loss, every accepted batch is waited before checkpoint advancement,
verification gates cutover, erased records cannot reenter from the source, Bleve
maintenance is enforced, and service maintenance is described honestly.
Online reindex is explicitly unavailable.

## Risks And Mitigations

### The contract becomes a lowest common denominator

Mitigation: keep a strong lexical core, add typed optional capabilities, and provide driver-specific extension packages instead of raw escape hatches.

### Driver differences surprise users

Mitigation: publish the capability/guarantee matrix, expose precision and async state in types, and test migration scenarios with representative corpora.

### Search schema changes destroy availability or data

Mitigation: fail closed on drift, treat indexes as derived, stage rebuilds in new physical generations, and promote only through explicit verified commands.

### Background indexing loses or reorders changes

Mitigation: carry source IDs, reload current authoritative state, make writes idempotent, expose lag/failures, and use a transactional outbox where the application requires durable transaction-to-index delivery.

### Multi-tenancy leaks records

Mitigation: server-owned mandatory filters, typed public DTOs, field allowlists, no default browser keys, and direct authorization tests for every search endpoint.

### Driver modules become release-heavy

Mitigation: separate modules intentionally, automate inventory/version/tag validation, and ship drivers in tiers rather than blocking every release on all candidates.

### Heavy integration CI becomes slow or flaky

Mitigation: per-driver jobs, pinned images, meaningful readiness, compile-only lanes, deterministic scenario contracts, classified bootstrap retries, and separate OpenSearch/Elasticsearch jobs.

## Sources

Primary backend references used to validate the proposed contract and driver tiers:

- [Go contexts and structs](https://go.dev/blog/context-and-structs)
- [Go Code Review Comments: interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)
- [Bleve repository and supported search features](https://github.com/blevesearch/bleve)
- [Bleve index mappings](https://blevesearch.com/docs/Index-Mapping/)
- [Meilisearch search API](https://www.meilisearch.com/docs/reference/api/search/search-with-post)
- [Meilisearch term facet distributions and numeric statistics](https://www.meilisearch.com/docs/capabilities/filtering_sorting_faceting/how_to/build_faceted_navigation)
- [Meilisearch asynchronous tasks](https://www.meilisearch.com/docs/capabilities/indexing/tasks_and_batches/monitor_tasks)
- [Meilisearch atomic index swaps](https://www.meilisearch.com/docs/reference/api/indexes/swap-indexes)
- [Typesense current API reference](https://typesense.org/docs/latest/api/)
- [Typesense collection schema administration](https://typesense.org/docs/29.0/api/collections.html)
- [OpenSearch Go client](https://docs.opensearch.org/latest/clients/go/)
- [Elasticsearch Go client](https://www.elastic.co/docs/reference/elasticsearch/clients/go)
- [Elasticsearch bulk indexing](https://www.elastic.co/docs/reference/elasticsearch/clients/go/using-the-api/bulk-indexing)
- [PostgreSQL full-text search](https://www.postgresql.org/docs/17/textsearch.html)
- [PostgreSQL ranking and highlighting](https://www.postgresql.org/docs/17/textsearch-controls.html)
- [SQLite FTS5](https://www.sqlite.org/fts5.html)
- [MySQL full-text search](https://dev.mysql.com/doc/refman/8.4/en/fulltext-search.html)
- [Redis Search](https://redis.io/docs/latest/develop/ai/search-and-query/)
- [Redis Search with the Go client](https://redis.io/docs/latest/develop/clients/go/queryjson/)
- [Algolia API clients](https://www.algolia.com/doc/libraries/sdk)

## Recommendation

Proceed with Search as the next major GoForj sibling primitive.

Start with the portable contract, shared corpus, documentation generator, and a
Bleve vertical slice. Extract the reusable contract suite and fake from that
working boundary, then implement Meilisearch and Typesense. Three distinct
drivers are the minimum convincing proof that the public API represents
application search rather than one engine with renamed types.

Do not wait for every planned driver before integrating GoForj. Integrate the first proven three, keep later drivers independently releasable, and make the capability matrix and conformance suite the gate for expanding support.

# Localization Library And GoForj Integration Design

## Status

- Design status: proposed
- Planning date: 2026-09-06
- Target repositories: a new `github.com/goforj/localize` sibling repository and `goforj`
- Primary sibling-library scope: locale tags, matching, immutable catalogs, message rendering, typed values, fallback, diagnostics, and test support
- Primary GoForj scope: an optional Localization component, generated catalog layout, HTTP and Auth locale resolution, Mail and Notifications integration, frontend parity checks, commands, documentation, and render coverage
- Cross-repository source of truth: this design is normative until the Localization repository contains an accepted design or implementation plan that references it

## Summary

GoForj should add a reusable localization library backed by an optional generated Localization component.

The library should let application code render stable message identifiers for a resolved locale without knowing where catalogs came from or how language fallback works:

```go
message, err := localizer.Text(ctx, "tickets.assignment.changed",
	localize.String("ticket", ticketNumber),
	localize.String("assignee", assigneeName),
)
```

Localization is broader than translating frontend labels. Server applications need one consistent locale contract for validation errors, transactional mail, notifications, CLI output intended for end users, exported documents, and server-rendered pages. GoForj already stores an Auth user locale and ships a frontend locale registry, but it does not provide a server-side catalog, matching, fallback, or validation model.

The reusable library should build on established Unicode and Go language-tag behavior rather than inventing locale semantics. It should provide a small stable application API, strict catalog validation, deterministic fallback, typed interpolation values, plural selection, immutable concurrent bundles, privacy-safe observations, and strong test tooling. GoForj should own generated project layout, middleware integration, catalog checking, and component composition.

## Decision

Create `github.com/goforj/localize` as a domain-neutral sibling library and integrate it into GoForj as the optional `Localization` App component.

Adopt these decisions:

1. Locale identifiers are canonical BCP 47 language tags, not free-form strings.
2. The root API exposes GoForj-neutral `Localizer`, `Catalog`, `Bundle`, and locale-resolution contracts.
3. Application code uses stable message IDs. English source text is not the lookup key.
4. Catalog loading and validation happen before the bundle becomes visible to callers.
5. Bundles are immutable and safe for concurrent use.
6. A reload builds a complete replacement and publishes it atomically. Callers never observe a partially loaded catalog.
7. The application default locale must have a complete catalog for every required message.
8. Required messages come from a separate versioned definition manifest, so deleting a key from every catalog cannot make an incomplete application appear valid.
9. A missing translation in another supported locale follows an explicit fallback chain and emits a bounded diagnostic.
10. A missing message in the default locale is a build, startup, or test failure, not a runtime fallback to the message ID.
11. Interpolation accepts a closed set of named scalar values. It does not reflect over arbitrary structs or call arbitrary `Stringer` implementations.
12. Plural selection uses CLDR-compatible locale rules supplied by an established engine.
13. V1 supports cardinal plural forms. Ordinal rules and grammatical gender require explicit later design.
14. V1 messages render plain text. HTML-safe fragments, Markdown, and transport-specific rich content are outside the root trust contract.
15. Rendering never performs HTML, shell, SQL, URL, or header escaping. The destination owns contextual escaping.
16. Locale matching uses established `golang.org/x/text/language` behavior.
17. The initial implementation may use `go-i18n/v2` internally for catalog and plural behavior, but no public type exposes that dependency.
18. Catalog syntax is deliberately constrained and versioned by this library even when an internal engine supports more formats.
19. JSON is the canonical server catalog format in v1. YAML and TOML are not accepted merely because an underlying package supports them.
20. Catalog files are application source. Embedded catalogs are the production default.
21. The root library does not watch files, contact translation services, or mutate catalogs remotely.
22. Context may carry an already resolved locale, but untrusted request values never become trusted merely by entering a context.
23. GoForj HTTP resolution follows a documented precedence and only selects from the generated supported-locale set.
24. An authenticated user preference outranks `Accept-Language` after server-side account resolution.
25. Missing or malformed locale preferences fall back safely and never block authentication.
26. Localization does not own user profile persistence, preference UI, geography inference, or authorization.
27. Mail and Notifications integrate through a narrow renderer interface. Localization does not send anything.
28. Frontend and backend catalogs may use different runtime engines, but generated checks require locale and message-key parity where messages are declared shared.
29. Metrics and observations use bounded locale classes and message catalogs. They never include rendered messages or interpolation values.
30. Component selection is compile-time generation. There is no runtime switch that silently changes a localized application back to raw message IDs.
31. The library ships a memory builder, conformance tests, and a fake suitable for deterministic application tests.

## Why A Separate Library

### The reusable problem

The hard parts are independent of GoForj:

- canonical language tags;
- supported-locale matching;
- fallback chains;
- plural categories;
- catalog validation;
- placeholder compatibility;
- immutable publication;
- missing-message behavior;
- typed interpolation; and
- deterministic tests.

A sibling library makes those contracts useful in workers, command applications, mail renderers, exporters, and other Go programs without importing GoForj generation or HTTP packages.

### Why not expose `go-i18n` directly

An established engine can provide correct plural and language behavior, but GoForj still needs a smaller compatibility surface with stricter rules. Direct exposure would make catalog syntax, error types, mutable bundle behavior, and dependency-specific message structures part of every generated application's API.

`localize` adds value by defining:

- one canonical catalog format;
- deterministic validation before publication;
- stable message and error contracts;
- typed values instead of arbitrary template data;
- explicit fallback and missing-message policy;
- safe observation boundaries; and
- framework-neutral test support.

The implementation should prefer an established engine internally rather than reimplement Unicode plural rules.

### Why not put this in Web

Locale resolution also applies to background jobs, notifications, mail, CLI commands, reports, and exports. HTTP middleware is one adapter, not the owner of localization.

### Why not put this in Notifications or Mail

Those libraries deliver or format channel-specific content. The same localized messages may appear in validation responses, UI, documents, or commands. Localization must remain independently usable.

## Standards And Prior Art

Implementation should align with:

- [BCP 47 language tags, RFC 5646](https://www.rfc-editor.org/rfc/rfc5646)
- [Language tag matching, RFC 4647](https://www.rfc-editor.org/rfc/rfc4647)
- [HTTP Accept-Language semantics, RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html#name-accept-language)
- [`golang.org/x/text/language`](https://pkg.go.dev/golang.org/x/text/language)
- [`golang.org/x/text/message`](https://pkg.go.dev/golang.org/x/text/message)
- [`go-i18n/v2`](https://github.com/nicksnyder/go-i18n)

These sources inform parsing, matching, and plural behavior. This design remains the application-facing contract.

## Goals

1. Give application code a small, stable rendering API.
2. Make locale selection explicit and testable across HTTP and background execution.
3. Detect missing messages, invalid placeholders, and broken plural forms before production traffic.
4. Support deterministic fallback without hiding incomplete catalogs.
5. Keep user-supplied values out of message syntax and diagnostics.
6. Preserve a clear boundary between translation and contextual output escaping.
7. Support local development without external services.
8. Integrate cleanly with Auth, Mail, Notifications, Web, Testkit, and generated frontends.
9. Keep runtime reads lock-free or close to lock-free after bundle construction.
10. Avoid making an internal localization engine part of the public API.

## Non-goals

1. A translation management system.
2. Machine translation.
3. Remote catalog editing or deployment.
4. A frontend i18n runtime replacement.
5. A rich-text sanitizer.
6. HTML template ownership.
7. Locale-aware database collation or search analysis.
8. Time zone resolution or calendar scheduling.
9. Currency conversion.
10. User preference persistence.
11. Geographic locale inference.
12. Automatic extraction from arbitrary Go string literals.
13. Runtime acceptance of unknown message IDs.
14. Arbitrary executable templates or functions.
15. A promise that every Unicode display concern is solved in v1.

## Terminology

### Locale

A canonical BCP 47 language tag such as `en`, `en-GB`, `fr`, or `zh-Hant`.

### Supported locale

A locale intentionally shipped by the application and accepted by its resolver.

### Default locale

The application's complete fallback locale. Every required server message must exist in it.

### Message ID

A stable dotted identifier such as `auth.password_reset.subject` or `tickets.validation.subject_required`.

### Catalog

The validated messages for one locale and one catalog version.

### Bundle

An immutable set of catalogs, supported locales, default locale, fallback policy, and renderer state.

### Values

Named typed scalar inputs supplied to a message at render time.

### Resolver

An adapter that selects one supported locale from trusted preferences and accepted request hints.

## Repository Ownership

### `github.com/goforj/localize`

The sibling library owns:

- locale parsing, canonicalization, and matching;
- message ID and placeholder grammar;
- catalog decoding and validation;
- immutable bundle construction and atomic replacement;
- rendering and cardinal plural selection;
- typed values;
- fallback and missing-message contracts;
- stable errors;
- privacy-safe observations;
- memory builders and fakes;
- catalog conformance and parity helpers; and
- package documentation and executable examples.

### `goforj`

The framework owns:

- optional Localization component selection;
- generated catalog directories and embedding;
- App composition and accessors;
- HTTP locale middleware and precedence;
- Auth preference adaptation;
- Mail and Notifications renderer adapters;
- CLI checks and extraction from explicit declarations;
- frontend locale registry and shared-key parity generation;
- local reload integration where explicitly enabled;
- metrics and Lighthouse summaries;
- Testkit integration;
- render coverage and authoritative templates; and
- dependency pins across every generated surface.

### Generated application

The application owns:

- supported and default locales;
- message IDs and source descriptions;
- translated catalog content;
- which request override mechanisms are allowed;
- user preference UI and persistence beyond Auth's existing field;
- domain-specific date, number, and currency presentation policy;
- translator workflow and review; and
- tests for business-critical wording and fallback.

## Core Invariants

1. A published bundle is complete for its default locale.
2. Completeness is measured against the independent definition manifest, not the union of loaded catalog keys.
3. Bundle construction is deterministic for identical definition, catalog, profile, and option bytes.
4. A failed load or reload leaves the previously published bundle unchanged.
5. Every locale is canonicalized once and compared by language-tag semantics.
6. Unsupported locales never create an unbounded catalog or metric label.
7. Locale fallback cannot escape the configured supported-locale set.
8. Message IDs use a bounded lowercase ASCII dotted grammar.
9. Every message declares one stable set of placeholders across all plural variants.
10. Every translated message uses placeholders compatible with the default message.
11. Unknown, duplicate, or malformed placeholders fail catalog validation.
12. Runtime values are validated and bounded before rendering.
13. Missing, duplicate, and unexpected runtime values fail visibly.
14. Rendered output has a configurable hard byte limit.
15. Catalog templates cannot invoke filesystem, network, environment, reflection, or application functions.
16. Rendering does not mark output as trusted HTML or another trusted destination type.
17. Caller-owned value slices and byte data are defensively copied where necessary.
18. A context-bound localizer is an immutable view.
19. Nil contexts normalize to `context.Background()`.
20. Required injected collaborators fail during construction rather than being hidden by nil guards.
21. Observers receive message ID, selected locale, fallback classification, status, and duration only.
22. Rendered messages and interpolation values never enter metrics, logs, or Lighthouse by default.
23. The fake and production renderer apply the same message ID and value validation.
24. Locale resolution never treats `Accept-Language` as authorization or identity.
25. Authentication remains successful when a stored locale is malformed; the locale falls back and emits a safe diagnostic.
26. Component-off renders contain no Localization code, catalog embedding, environment keys, or dependency.

## Public API Shape

The exact names may change during implementation, but the narrow capabilities should remain.

```go
package localize

type Localizer interface {
	Text(context.Context, MessageID, ...Value) (string, error)
	Render(context.Context, MessageID, FormatContext, ...Value) (RenderResult, error)
	For(Locale) (BoundLocalizer, error)
	Locale(context.Context) Locale
}

type BoundLocalizer interface {
	Text(MessageID, ...Value) (string, error)
	Render(MessageID, FormatContext, ...Value) (RenderResult, error)
	Locale() Locale
}

type Bundle interface {
	Localizer() Localizer
	Supported() []Locale
	Default() Locale
}

type MessageID string

type Locale struct {
	// contains unexported canonical language-tag state
}

type Zone struct {
	// contains an unexported validated IANA time-zone identity
}

type FormatContext struct {
	Zone Zone
}

type RenderResult struct {
	Text            string
	RequestedLocale Locale
	RenderedLocale  Locale
	Fallback        FallbackClass
}

type Source struct {
	// contains an unexported catalog source policy
}

func ParseLocale(string) (Locale, error)
func ParseZone(string) (Zone, error)
func NewFSSource(fs.FS) Source
func NewDirSource(string) (Source, error)
type Resolution struct {
	// contains unexported locale, source, trust, and match state
}

func ContextWithResolution(context.Context, Resolution) context.Context
func ResolutionFromContext(context.Context) (Resolution, bool)
```

`Locale` and `Zone` are opaque validated values with invalid zero values. `Resolution` is a separate opaque value produced only by a configured resolver and retains selected locale, source, trust, fallback class, and resolver identity. A bundle rejects a resolution minted by another resolver or App. `Text` is a convenience wrapper returning only rendered text with the default format context. `Render` returns call-local fallback evidence suitable for concurrent previews and accepts an immutable per-render zone. `For` rejects zero or unsupported locales and is useful for background work, Mail, and Notifications. Context helpers bind only a `Resolution`, use a private collision-safe key, and ignore raw same-named context values. A parsed locale is a candidate, not trusted precedence state.

### Typed values

```go
localize.String("name", displayName)
localize.Integer("count", count)
localize.Decimal("amount", amount)
localize.Boolean("active", active)
localize.Time("expires_at", expiresAt)
localize.Date("billing_date", billingDate)
```

V1 does not accept `map[string]any`. Decimal values use a documented decimal string or small domain-neutral decimal contract rather than binary floating point for user-visible money. Time values retain an instant and require an explicit display zone policy from the application adapter.

### Construction

```go
bundle, err := localize.NewBundle(
	localize.WithDefault("en"),
	localize.WithSupported("en", "en-GB", "fr"),
	localize.WithDefinitions(localize.NewFSSource(definitionFS), "messages.json"),
	localize.WithCatalogs(localize.NewFSSource(catalogFS), "catalogs"),
	localize.WithFormattingProfiles(profiles),
	localize.WithObserver(observer),
)
```

Construction parses and validates every configured catalog before returning. It does not lazily discover syntax errors on the first request.

An opaque `Source` value is accepted consistently by construction and reload. `NewFSSource` provides portable embedded and test loading. The library validates names lexically but cannot prove how an arbitrary `fs.FS` implementation resolves symlinks, so the caller owns backend confinement. Production GoForj uses `embed.FS`. `NewDirSource` is the development operating-system directory source and opens entries root-relative with platform-appropriate no-follow confinement rather than accepting `os.DirFS` as a security boundary.

## Message And Catalog Model

### Definition manifest

Message existence and schema are declared independently from translations. A versioned definition manifest contains every required message ID, description, placeholder type, formatting profile, and optional plural operand. Bundle construction validates every catalog against this manifest.

Removing a definition is an explicit compatibility operation. Removing the same key from every catalog without updating the manifest fails. Catalog keys absent from the manifest are stale and fail strict validation rather than becoming callable runtime IDs.

### Message IDs

IDs are bounded lowercase ASCII segments separated by dots:

```text
auth.login.failed
auth.password_reset.subject
tickets.assignment.changed
tickets.comments.count
```

IDs are semantic compatibility identifiers. Renaming one requires updating every catalog and caller. Display text may change without changing the ID.

### Canonical JSON

Illustrative catalog:

```json
{
  "version": 1,
  "locale": "en",
  "messages": {
    "tickets.comments.count": {
      "description": "Comment count shown on a ticket",
      "plural_operand": "count",
      "one": "{{count}} comment",
      "other": "{{count}} comments",
      "placeholders": {
        "count": "integer"
      }
    },
    "tickets.assignment.changed": {
      "description": "Confirmation after changing an assignee",
      "other": "Ticket {{ticket}} is now assigned to {{assignee}}.",
      "placeholders": {
        "assignee": "string",
        "ticket": "string"
      }
    }
  }
}
```

The example combines definition and translation details only for readability. The authoritative format stores definitions separately from per-locale translated forms. The precise on-disk schemas must be versioned before release. Object keys are sorted for generated output and duplicate keys are rejected by a decoder that does not silently apply last-write-wins behavior.

Descriptions are translator context and never render. Catalogs contain no secrets.

### Placeholder grammar

Placeholders are explicit and named. Positional placeholders are rejected because they are fragile under translation. Names use lowercase ASCII snake case and must match their declaration exactly.

The template language permits only:

- literal text;
- named value insertion;
- engine-owned cardinal plural selection; and
- escaped literal delimiters.

It permits no conditionals beyond declared plural selection, loops, includes, filesystem access, function calls, or nested template evaluation.

### Plural forms

The default catalog must provide `other`. It provides every additional category required by its locale and message use. Translated catalogs are checked against categories required by their own locale rather than copied mechanically from English.

Each pluralized message definition names exactly one `plural_operand`. It must reference a declared integer placeholder. Plural categories on a definition without an operand, a non-integer operand, or operand drift in translated metadata fail catalog validation. Messages may contain other integer placeholders without creating ambiguity. V1 does not infer plurality from arbitrary values.

### Catalog layering

V1 supports one application catalog per locale plus library-provided namespaces registered explicitly during construction. Precedence is deterministic:

1. application override for an explicitly overridable library message;
2. application catalog;
3. owning library catalog.

Duplicate application IDs and undeclared overrides fail. Filesystem order never decides precedence.

## Locale Resolution

### General resolver

```go
type Candidate struct {
	// contains unexported locale, source, and resolver-policy identity
}

resolution, err := resolver.Resolve(candidates...)
```

`Candidate` fields are opaque. Source-specific resolver methods mint candidates bound to that resolver's policy, so callers cannot substitute an authenticated source or reuse a candidate across Apps. The resolver returns an opaque `Resolution` containing one supported locale plus a bounded explanation such as `exact`, `parent`, `matched`, or `default`. It never returns an unsupported input verbatim. Trust is granted by configured source policy, not by caller fields.

### HTTP precedence

Generated HTTP integration uses two stages because current Auth also protects individual routes inside the otherwise public Auth group. A bounded public resolver runs outside routing and binds the initial resolution for every request, including failed-authentication responses. After any successful Auth middleware, an authenticated override middleware resolves again with the server-authoritative user preference and replaces only the locale resolution. Generated route composition applies this override to every protected application route and every route-local protected Auth endpoint.

Within the applicable path, resolution uses this order:

1. an application-set trusted `Resolution` already bound to the request;
2. the authenticated user's server-resolved locale preference;
3. a signed application locale cookie, when generated UI enables it;
4. an explicit route or query preference only when that route opts in;
5. `Accept-Language` with bounded input and entry count; and
6. the application default locale.

The selected opaque resolution is added through `ContextWithResolution` after matching. A plain parsed locale bound by application code is only a rendering hint and cannot become precedence-1 state. Raw header, cookie, query, and user-profile values are never copied into metric labels. Failed authentication retains the public resolution and does not reveal whether a stored user preference exists.

Generated APIs should set `Content-Language` from `RenderResult.RenderedLocale`, never merely from the requested locale, when a response body is intentionally localized. Parent and default fallback therefore describe the language actually used in the representation. They should add `Vary: Accept-Language, Cookie` only when those inputs actually affect cacheable output. Authenticated responses should follow their existing private-cache policy rather than relying on `Vary` as an authorization control.

### Background work

Jobs, events, notifications, and scheduled work do not inherit a request context accidentally. A locale needed later must be persisted as a canonical supported locale or resolved again from authoritative recipient state.

A job payload should carry the intended locale only when delayed delivery is expected to preserve the locale selected at enqueue time. Applications that want current preferences should persist the recipient identity and resolve during execution instead.

## Fallback Policy

Fallback is explicit and deterministic:

1. exact selected locale;
2. configured supported parent or application-declared fallback;
3. application default locale.

For example, `fr-CA` may fall back to `fr` only if `fr` is supported or declared as its parent. The library does not load arbitrary inferred catalogs.

Every fallback returns a safe classification to the observer. Production may render the fallback message, but CI and catalog checks still report incomplete non-default locales according to application policy.

Applications choose whether a supported locale must be complete or may intentionally fall back. The default generated policy requires completeness for release builds and permits explicit incomplete locales only in local development with conspicuous diagnostics.

## Formatting Boundaries

### Text and escaping

The root library returns ordinary UTF-8 text. It validates UTF-8 and output size but does not escape for HTML, JSON, URLs, CSV, terminals, or message headers.

Callers must use the destination's normal safe APIs:

- HTML templates escape localized text in HTML contexts;
- JSON encoders escape JSON strings;
- Mail validates and encodes headers;
- shell and SQL construction never interpolates localized strings;
- terminal adapters handle control characters according to their output policy.

Catalog validation rejects forbidden control characters except documented whitespace. Interpolated user values retain their text semantics and are subject to the destination's escaping.

### Numbers, dates, and currency

V1 provides immutable named formatting profiles as bundle construction input. Each decimal, date, time, or currency placeholder definition names one compatible profile. Profiles own fixed style, precision, and rounding. `FormatContext` supplies the validated per-render IANA time zone for time-bearing placeholders. Values carry domain data, not ad hoc format strings. The application supplies through profiles and render context:

- currency code for monetary display;
- time zone for local time display;
- precision and rounding policy for decimals; and
- style such as short date or full date.

Localization never performs currency conversion or infers a user's time zone from locale.

Formatting profiles are named, bounded application configuration. Unknown or type-incompatible profiles fail bundle construction. A required time zone with a zero `Zone` fails rendering rather than falling back to process-local time. Safe render caches include locale, message, profile generation, and zone identity, and never include unbounded raw zone strings. Profile changes can change rendered output and are reported as configuration compatibility changes. Arbitrary format strings from request data are rejected.

## Errors

Stable sentinels should include:

- `ErrInvalidLocale`
- `ErrUnsupportedLocale`
- `ErrForeignResolution`
- `ErrInvalidMessageID`
- `ErrCatalogInvalid`
- `ErrCatalogIncomplete`
- `ErrMessageNotFound`
- `ErrValueMissing`
- `ErrValueDuplicate`
- `ErrValueUnexpected`
- `ErrValueType`
- `ErrOutputTooLarge`
- `ErrClosed`

Typed errors may include safe locale and message identifiers, catalog file locations relative to the declared root, and placeholder names. They must not include rendered output, interpolation values, absolute developer paths, or catalog contents.

Errors preserve `context.Canceled` and `context.DeadlineExceeded` through `errors.Is`. Internal engine errors are mapped to stable library classifications rather than exposed as a public dependency contract.

## Observation Model

```go
type Observation struct {
	Operation    Operation
	MessageID    MessageID
	Locale       Locale
	Fallback     FallbackClass
	Status       Status
	Duration     time.Duration
	ErrorClass   string
}
```

The observer is non-blocking by contract and receives only supported canonical locale values and bounded message IDs. It never receives message text, values, user identity, raw locale inputs, headers, or catalog bytes. Panics are recovered and cannot change rendering results.

Metrics may count rendering, fallback, and missing translation by a generated bounded catalog of message IDs. Applications with large message sets should aggregate by namespace rather than create unbounded series.

## Reload And Lifecycle

Production embeds catalogs and normally constructs one immutable bundle at startup. Reloadable local development uses a separate owning `Store` capability:

```go
type Store interface {
	Localizer() Localizer
	Snapshot() Bundle
	Reload(context.Context, Source) error
	Close() error
}
```

`Store.Localizer` is a live view that selects one bundle snapshot at the start of each render. `Store.Snapshot` and every `BoundLocalizer` remain pinned to an immutable bundle. `Close` rejects future live renders and reloads with `ErrClosed` but does not invalidate prior snapshots. Reload is serialized through one gate. A reload that enters first completes or fails before a later reload can construct and publish, so an older slow load cannot overwrite newer input. Shutdown acquires the same gate and marks the store closed. Reload:

1. reads a bounded catalog set from a declared root;
2. constructs and validates a complete candidate bundle;
3. verifies the bundle is still open and atomically swaps it only after success; and
4. retains the previous bundle on any failure.

The root library does not own a watcher or goroutine. GoForj local development may connect its existing watcher to an explicit reload callback. Shutdown prevents new reloads but does not invalidate already bound immutable localizers.

## Fake And Test Support

The sibling repository provides:

- `localize.NewMemoryBundle` for ordinary catalogs;
- `localizefake.New` for captured calls and controlled results;
- catalog conformance helpers;
- completeness and placeholder parity assertions; and
- locale resolution test cases.

The fake applies production validation, captures immutable snapshots, is concurrency-safe, and does not expose a generic `map[string]any` assertion surface.

Suggested helpers:

```go
localizefake.RequireRendered(t, fake, "tickets.assignment.changed")
localizefake.RequireLocale(t, fake, "fr")
localize.RequireComplete(t, bundle, "en", "fr")
```

Failure output includes IDs, locale, and value names, but not value contents unless a test explicitly opts into a value-safe comparator.

## GoForj Component Model

### Selection

`Localization` is an optional project testing and runtime component.

Rules:

- Localization requires no database or external service.
- Localization does not imply Auth, Web, Mail, Notifications, Events, Queue, Storage, Cache, Audit, Metrics, or Lighthouse.
- Auth may provide a stored user locale when both are enabled.
- Mail and Notifications may require Localization only when their application templates declare localized content.
- each App selects its own default and supported locales.

Illustrative project configuration:

```yaml
apps:
  api:
    components: [localization]
    localization:
      default: en
      supported: [en, en-GB, fr]
      require_complete: true
```

Supported locales are build-time project metadata, not a comma-separated runtime driver list. Changing the default or removing a supported locale is a configuration and user-experience migration that should be explicit in generated plans.

### Generated layout

```text
internal/localization/
    manager.go
    catalogs.go
    resolver.go
    messages.go
resources/locales/
    messages.json
    en.json
    en-GB.json
    fr.json
```

Catalog files are application-owned once created. Generators may add missing framework-owned messages through reconciliation metadata, but must never overwrite translations silently.

Generated `messages.go` may contain constants for declared IDs:

```go
const (
	MessageAuthLoginFailed localize.MessageID = "auth.login.failed"
	MessageTicketAssigned  localize.MessageID = "tickets.assignment.changed"
)
```

Generation should not create one Go function per message by default. Typed helper generation may be considered only after evidence shows placeholder mistakes remain common despite catalog checks.

### App access

```go
func (a *App) Localization() localize.Localizer
```

Component-off Apps contain no accessor or imports. Services should usually depend on the narrow `localize.Localizer` interface rather than the generated manager.

### Environment

Production catalog identity is embedded at build time. Small runtime policy may include:

```dotenv
LOCALIZATION_STRICT_FALLBACK=true
LOCALIZATION_MAX_OUTPUT_BYTES=65536
```

The default and supported locale list remain generated configuration so deployments cannot select a catalog that was not built and validated.

## Framework Integrations

### Auth

The current generated Auth user model already has a locale field, but the current profile update flow and frontend language switch do not synchronize it with server resolution. Integration must add an explicit authenticated locale-preference endpoint and an optional signed-cookie path for public UI selection. It must:

- parse and canonicalize a submitted preference;
- accept only the App's supported set;
- persist the canonical form;
- avoid blocking authentication when legacy stored data is malformed;
- bind the trusted resolved preference after user resolution; and
- provide a migration path when a supported locale is removed.

The preference endpoint uses a dedicated `UpdateLocale(userID, locale)` column-scoped repository operation. It must not call the full-row `Save` path or rewrite password, activity, lockout, verification, or login-state fields from a stale snapshot. It refreshes only the cached profile projection that includes locale, leaves the security projection unchanged, refreshes the signed locale cookie when used, and returns the canonical selected locale. Server-rendered bootstrap data hydrates the frontend selection. The frontend may still use local storage for immediate UI choice, but local storage alone is never a server preference.

Removing a locale does not rewrite users automatically. The application must choose a replacement mapping or allow those users to fall back until migrated.

### Web API and server-rendered UI

Public middleware resolves once, and successful Auth may replace that resolution once through the authenticated override stage. Validation and application errors should carry stable codes; the response adapter localizes the public message at the boundary. Domain services should not return already localized errors when callers need machine-readable behavior.

Public APIs should not vary error codes or JSON field names by locale. Only human-facing message fields change.

### Mail

Mail remains responsible for recipients, headers, HTML/text templates, attachments, and transport. A generated Mail renderer may ask Localization for subject and body fragments using an explicit recipient locale.

Localized text does not bypass Mail's header validation or HTML escaping. Preview tooling selects a supported locale and uses the call's `RenderResult` to display requested locale, rendered locale, and fallback classification without relying on a global observer.

### Notifications

Notifications pass a recipient locale and semantic content to a renderer interface. A Localization adapter can render channel-neutral text before the channel formatter adapts it. Notifications retains routing, preferences, delivery, and attempt semantics.

The dependency remains optional through an adapter package or GoForj composition. The Notifications root API must not require `localize` concrete types.

### CLI and jobs

Framework developer commands remain English in v1 unless separately selected for localization. Application commands producing end-user documents may bind an explicit locale.

Jobs never derive locale from process environment. Their handler receives a canonical persisted locale or resolves authoritative recipient state.

### Frontend parity

GoForj does not force Vue, React, and server Go code to share one runtime message syntax. It generates a manifest for explicitly shared message IDs and placeholder schemas, then validates:

- supported locale registry parity;
- default locale parity;
- presence of shared IDs;
- placeholder name and type parity; and
- absence of stale generated framework keys.

Frontend-only and server-only namespaces remain valid. The parity tool reports exact keys and files without copying translations between incompatible engines.

## Commands

Potential generated commands:

- `localization:check` validates every catalog and parity manifest.
- `localization:missing --locale fr` lists missing IDs and descriptions without values.
- `localization:unused` reports candidate unused application IDs but never deletes them automatically.
- `localization:extract` reads explicit message declarations and generated manifests, not arbitrary string literals.

Commands are deterministic and suitable for CI. Machine-readable output is versioned if provided.

## Security Model

1. Locale inputs are bounded before parsing.
2. `Accept-Language` entry count and byte size are bounded.
3. Only configured supported locales can reach catalogs, metrics, or paths.
4. Every catalog source rejects lexical traversal. The operating-system directory source additionally rejects symlink escape through root-relative no-follow access; generic `fs.FS` confinement remains the caller's contract.
5. Production catalogs are embedded and cannot be replaced by request input.
6. Catalog syntax has no arbitrary functions, includes, network reads, or environment reads.
7. Interpolation uses typed bounded values and never reflection over application objects.
8. Rendered text is untrusted for every destination context.
9. Diagnostics omit message text and value contents by default.
10. Missing-message behavior never reveals filesystem paths or hidden catalog content.
11. Locale selection does not alter authorization, tenancy, or resource visibility.
12. A localized error preserves its stable machine code.
13. Bidirectional text control characters require explicit catalog policy and review; untrusted interpolation never gains trusted markup status.
14. Catalog reload retains the last valid bundle on malformed or oversized input.
15. Translation files contain no credentials, API keys, or recipient data.

## Concurrency And Performance

- Rendering and locale matching are safe for concurrent use.
- Published bundles and bound localizers are immutable.
- Catalog maps are built once and read without per-call mutation.
- Reload uses atomic whole-bundle publication.
- Output builders preallocate conservatively and enforce hard limits.
- `Accept-Language` parsing is bounded before matching.
- Message templates may be compiled during bundle construction.
- Shared caches retain only compiled templates and immutable formatter metadata, never rendered output or user values.
- Zone-dependent formatter metadata keys include the supported locale, profile generation, and canonical zone identity.

Benchmarks should measure bundle construction, exact and fallback render, plural render, locale matching, and concurrent reads. Benchmarks detect regressions but do not become noisy hard CI gates.

## Compatibility

### Public API

The root public API follows semantic versioning. Internal engine replacement must not change message lookup, placeholder validation, fallback, plural selection, or stable errors without an explicit compatibility plan.

### Catalog data

Catalog schema version is persisted source compatibility. A new schema requires a validator that explains the migration and a generator update that does not rewrite application translations silently.

Changing a message's required placeholders is a source and catalog compatibility change. Generated checks must identify every affected locale.

### Configuration

Adding Localization is additive. Changing default locale, strictness, supported locales, or fallback chains changes runtime presentation and may require user preference migration.

### Minimum Go version

The sibling library should choose the lowest Go version supported by its required `x/text` and localization engine versions. GoForj should not raise its minimum Go version unless a concrete required dependency demands it.

## Testing Strategy

### Root library tests

Cover:

- BCP 47 parsing and canonicalization;
- forged, malformed, zero, nested, and unsupported opaque locale bindings;
- source substitution and cross-App candidate or resolution reuse;
- exact, parent, weighted, malformed, and default matching;
- bounded `Accept-Language` parsing;
- message ID and placeholder grammar;
- duplicate JSON keys;
- required plural categories;
- default catalog completeness;
- required-definition deletion from every catalog, undeclared runtime IDs, stale catalog IDs, and manifest drift;
- zero, ambiguous, non-integer, and drifting plural operands;
- strict and permitted fallback;
- missing, unexpected, and mistyped values;
- duplicate runtime value names in production and fake renderers;
- output bounds and invalid UTF-8;
- detailed concurrent render results with requested locale, rendered locale, and fallback kept call-local;
- date and time profiles for the same locale and instant across zones and DST boundaries, decimal rounding, zero zones, and unknown profiles;
- cancellation identity;
- immutable defensive copies;
- concurrent rendering and reload;
- deliberately reordered overlapping `Store.Reload` requests and reload crossing `Store.Close`;
- live localizer versus immutable snapshot and bound-localizer behavior after reload and close;
- failed reload retaining the previous bundle;
- observer panic and drop behavior;
- diagnostic redaction; and
- fake parity with production validation.

### Engine conformance

Pin representative CLDR cases for languages with materially different cardinal rules, including English, French, Arabic, Polish, Russian, Japanese, and Chinese. These tests protect behavior when the internal engine or Unicode data version changes.

### GoForj render tests

Render under `/tmp` and cover:

- component off;
- minimal localized CLI App;
- Web API;
- server-rendered UI;
- Auth with valid, malformed, and removed user locales;
- protected-route ordering where Auth locale conflicts with cookie and `Accept-Language`, every route-local protected Auth endpoint, plus public and localized failed-auth behavior;
- authenticated preference updates, cache invalidation, signed-cookie refresh, and frontend hydration;
- locale preference updates racing password and active-state changes without unrelated column regression;
- Mail previews across locales;
- Notifications rendering across recipients;
- default and named Apps with different supported sets;
- each frontend starter parity manifest;
- rerender preservation of application translations; and
- largest supported composition.

Configuration tests cover legacy mapping input migration, canonical component-sequence output, marshal and rerender stability, and component-off removal.

### Security tests

Verify:

- lexical traversal fails for every source, and symlink construction plus symlink-swap attempts fail during reload through the secure operating-system directory source;
- concurrent renders of the same message and profile with different zones and values cannot reuse rendered output across calls;
- oversized headers, catalogs, values, and output fail safely;
- template syntax cannot call functions or include files;
- interpolation does not become trusted HTML;
- control characters do not enter headers or terminals through framework adapters;
- raw locale inputs and values do not reach errors or observations; and
- locale changes cannot broaden authorization or tenant scope.

## Documentation

The sibling README should explain:

- message IDs and catalogs;
- locale parsing and matching;
- plural forms;
- typed values;
- fallback policy;
- destination escaping;
- immutable bundles and reload;
- error handling; and
- fakes and completeness tests.

Generated application documentation should explain:

- how to add a supported locale;
- how to add and translate a message;
- how Auth and HTTP resolution interact;
- how background jobs retain or resolve locale;
- how Mail and Notifications render per recipient;
- how frontend parity works;
- what catalog checks run in CI; and
- why locale does not imply time zone or currency.

## Delivery Plan

### Phase 1: root contracts

1. Create the sibling repository.
2. Pin the internal engine and Unicode data versions.
3. Implement locale, message ID, typed value, error, and observer contracts.
4. Implement immutable bundle construction and memory catalogs.
5. Publish catalog and renderer conformance helpers.
6. Add executable examples and generated API documentation.

### Phase 2: catalog and formatting

1. Finalize canonical JSON schema version 1.
2. Add duplicate-key, completeness, placeholder, plural, and output validation.
3. Add integer, decimal, date, time, and currency-code-aware formatting profiles.
4. Prove representative plural behavior across pinned locales.
5. Add atomic replacement without a library-owned watcher.

### Phase 3: GoForj component

1. Add component selection and generated catalog layout.
2. Add embedding, manager composition, and App accessor.
3. Add HTTP resolution and `Content-Language` behavior.
4. Add Auth locale adaptation and removal migration guidance.
5. Add checks, missing reports, and explicit extraction.

### Phase 4: ecosystem integrations

1. Add Mail renderer and preview integration.
2. Add Notifications renderer integration.
3. Add Testkit locale and fake access.
4. Add frontend shared-key manifests for each starter.
5. Validate default, named, minimal, and largest compositions.

### Phase 5: release

1. Inventory every module, generated fixture, integration module, catalog schema, and dependency pin surface.
2. Release the sibling module with repository scripts.
3. Verify the module tag and checksum availability independently.
4. Integrate the published version into GoForj.
5. Run `GOWORK=off` validation so local replacements cannot hide missing releases.

## Acceptance Criteria

The design is implemented only when:

- application code renders stable message IDs through a GoForj-neutral interface;
- an independent versioned definition manifest makes deletion from every catalog detectable;
- locale parsing, matching, and fallback use established language-tag behavior;
- locale and time-zone values are opaque and validated, while context carries only resolver-minted resolution state bound to one resolver and App;
- the default locale is complete before a bundle is published;
- every catalog, explicit plural operand, and formatting profile passes deterministic validation;
- placeholder names and types remain compatible across translations;
- runtime rendering accepts only typed bounded values;
- output is plain untrusted text with explicit destination escaping;
- bundle snapshots and bound localizers are immutable, while a live Store serializes reload and shutdown so failed or stale reloads cannot replace the last valid version;
- detailed render results report requested locale, rendered locale, and fallback per call without shared mutable diagnostic state;
- missing and fallback behavior is observable without exposing content or values;
- Auth locale preferences are canonical, synchronized through an explicit endpoint or signed cookie, cache-invalidated, and safe for legacy malformed data;
- protected HTTP routes resolve Auth before Localization, while public and failed-auth paths use bounded public resolution;
- localized HTTP responses use the rendered locale for `Content-Language`, with exact, parent-fallback, default-fallback, and corresponding `Vary` behavior directly tested;
- jobs, Mail, and Notifications use an explicit recipient or persisted locale;
- frontend shared-key and locale parity is checked without forcing one runtime syntax;
- component-off renders contain no Localization surface;
- application catalogs survive rerender unchanged;
- default, named, and largest compositions pass renders under `/tmp`;
- all relevant module and dependency pin surfaces are released and validated; and
- documentation explains fallback, escaping, plural behavior, and preference migration honestly.

## Risks And Mitigations

### Risk: the library becomes a thin engine wrapper

Mitigation: own a deliberately smaller catalog schema, stable typed API, validation contract, fallback policy, observations, and test suite while keeping the engine private.

### Risk: incomplete catalogs fail in production

Mitigation: validate every catalog before publication and require CI completeness by default.

### Risk: translated text becomes trusted markup

Mitigation: return ordinary text and require destination-specific escaping in every adapter.

### Risk: locale and time zone are conflated

Mitigation: require explicit time zone and currency information for relevant formatting.

### Risk: generated catalogs overwrite translations

Mitigation: treat catalogs as application-owned and use reconciliation metadata plus check commands instead of replacement.

### Risk: backend and frontend translations drift

Mitigation: validate declared shared IDs, locales, and placeholders through a generated parity manifest without pretending the engines share syntax.

### Risk: fallback hides poor translation coverage

Mitigation: distinguish runtime availability from release completeness and report every fallback with bounded diagnostics.

## Deferred Questions

1. Whether the public module name should remain `localize` or use `i18n` before repository creation.
2. Whether ordinal plural rules belong in v2.
3. Whether a standardized rich-text token model can remain portable and safe.
4. Whether generated typed functions provide enough value over constants and catalog checks.
5. Whether XLIFF import and export should be provided as an offline tool.
6. Whether application commands should support localized terminal output broadly.
7. Whether Unicode data upgrades require an application-visible compatibility report.

The v1 locale, catalog, matching, rendering, validation, fallback, and framework integration contracts should not wait on these questions.

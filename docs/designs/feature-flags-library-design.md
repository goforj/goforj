# Feature Flags Library And GoForj Integration Design

## Status

- Status: proposed, for discussion; no implementation or public API commitment.
- Planning date: 2026-11-11.
- Proposed repositories: `github.com/goforj/feature`, `goforj`, and `goforj-docs`.
- Scope: server-side boolean feature decisions with live refresh.

## Decision

Build a small standalone `feature` library if we agree that owning the narrow evaluation contract is worth maintaining it. Integrate it as an optional GoForj component after the standalone contract is validated.

Use explicit constructor injection, local evaluation, complete immutable snapshots, and a small versioned rule format. Start with memory and JSON file sources. Do not mutate the process environment to update flags.

This addresses behavior changes such as enabling a new checkout for selected tenants. HTTP listeners, database credentials, timezones, and infrastructure drivers retain their existing startup configuration contract.

## Why A Separate Primitive

The current generated App resolves many environment settings when it constructs services. `env.Reload()` updates environment variables but cannot replace existing services or make their configuration agree.

A flag evaluator instead owns a deliberately dynamic value. Application code evaluates that value at a defined decision point. It can change a feature decision without changing the dependency graph.

The sibling library owns evaluation, snapshot validation, refresh, sources, and testing helpers. GoForj owns component selection, environment configuration, generated providers, App lifecycle, commands, and Lighthouse integration.

Flags are application behavior controls. Authorization, billing entitlements, and emergency infrastructure shutdown need their own authoritative mechanisms. A stale flag snapshot cannot guarantee an immediate fleet-wide stop.

## Application API

Proposed fragments, not currently available APIs:

```go
subject := feature.Subject{
	Key: "tenant:acme",
	Attributes: map[string]string{
		"plan": "pro",
	},
}

enabled := flags.Enabled(ctx, "checkout-v2", false, subject)
```

`flags` is an injected `*feature.Client`. The subject is explicit rather than inferred from authentication or hidden in `context.Context`. Prefixing identities with `tenant:` or `user:` makes the intended rollout unit visible.

The small public surface is:

| API | Contract |
| --- | --- |
| `New(ctx, source, options...) (*Client, error)` | Load and validate the initial snapshot synchronously; start no background work. |
| `Enabled(ctx, key, fallback, subject) bool` | Evaluate locally and return the caller's fallback on an evaluation failure. |
| `Evaluate(ctx, key, fallback, subject) Decision` | Return the same value plus reason, snapshot revision, and a classified evaluation error. |
| `Snapshot() *View` | Capture one immutable revision for multiple decisions in a request or job. |
| `Refresh(ctx) error` | Load, validate, and atomically publish one complete document. |
| `Run(ctx, interval) error` | Run the single refresh loop until cancellation; reject concurrent loops. |
| `Status() Status` | Report active revision, refresh times, and refresh failure classification. |

`View` provides `Enabled` and `Evaluate` with the same arguments. It never changes after capture. Services decide when to capture a view; no HTTP middleware is required.

```go
view := flags.Snapshot()
newCheckout := view.Enabled(ctx, "checkout-v2", false, subject)
newReceipts := view.Enabled(ctx, "receipts-v2", false, subject)
```

`Decision` contains `Value`, `Reason`, `Revision`, and `Err`. Reasons distinguish a configured default, matching rule, disabled flag, missing flag, and evaluation failure. Unknown keys return the explicit fallback with a missing-key error available through `Evaluate`. Convenience calls remain observable through bounded counters or an explicitly configured observer.

Context cancellation returns the fallback and a cancellation error. There is no network access during evaluation. Subject attributes are read only during a call; callers must not mutate them concurrently. Construction deep-copies source data so later source mutations cannot change a published snapshot.

## First Rule Format

Use one versioned JSON document:

```json
{
  "schema": 1,
  "flags": {
    "checkout-v2": {
      "default": false,
      "disabled": false,
      "rules": [
        {
          "subjects": ["tenant:internal"],
          "value": true
        },
        {
          "attributes": {"plan": "pro"},
          "rollout": {"basis_points": 1000, "salt": "checkout-v2-2026"},
          "value": true
        }
      ]
    }
  }
}
```

This enables internal tenants, then approximately 10 percent of pro tenants. All others receive the configured false default. A missing flag receives the caller fallback.

Evaluation order is fixed:

1. A disabled flag returns false, regardless of rules, configured default, or caller fallback.
2. Evaluate rules in document order. Every condition in a rule must match.
3. Return the first matching rule's boolean value.
4. If no rule matches, return the flag's configured default.

`subjects` means membership in an exact, case-sensitive identity list. `attributes` means every listed string attribute must exist and match exactly. Missing attributes do not match. No regex, expression language, implicit coercion, nesting, segments, or cross-flag dependencies in v1.

A rollout condition requires a nonempty subject key. Missing identity makes that rule ineligible, including a 100 percent rollout. An empty subject is otherwise valid for unconditional and attribute-only rules. Rules must have at least one condition; unconditional behavior belongs in the flag default.

Reject unknown fields, duplicate JSON keys, missing required values, invalid keys, empty targeting conditions, unsupported schema versions, and rollout percentages outside 0 through 10000. Distinguish an omitted boolean from an explicit false during decoding. Validate the entire document before publication. Apply bounded document, flag, rule, and attribute sizes, with exact limits fixed before implementation.

## Stable Rollouts

Define the v1 bucket as the first unsigned 64 bits, in big-endian order, of SHA-256 over a version marker and length-prefixed UTF-8 flag key, salt, and subject key, modulo 10000. A subject matches when its bucket is less than `basis_points`.

The version marker is the UTF-8 string `goforj-feature-rollout-v1`. Each following field has an unsigned 32-bit big-endian byte length followed by its bytes. Do not normalize case or Unicode. Publish golden vectors before freezing this contract.

Changing the percentage preserves existing members when the percentage increases, assuming the same flag key, salt, subject, and preceding rules. Changing the salt deliberately reshuffles membership. A changed hash algorithm or encoding can change customer assignments and must be treated as a behavior compatibility change.

Rollout salt is not a credential. Deterministic buckets are not an authorization boundary.

## Sources And Refresh

The consuming boundary is a small `Source` interface with `Load(context.Context) (Document, error)`. A `Document` contains the full flag set, never a partial update. The evaluator validates and compiles it into private immutable state.

- Memory source: deterministic construction and explicit complete-document replacement for tests and embedded applications.
- File source: bounded JSON reads from a configured path, including regular-file symlinks. Periodic content checks tolerate editor rename-and-replace saves. File writers should use atomic replacement.

Source loading and compilation happen away from readers. A single atomic pointer swap publishes the new snapshot. Readers observe one complete revision. A `View` keeps its captured revision even if a refresh completes mid-request.

Serialize refresh operations so a slow older load cannot overwrite a newer publication. Unchanged validated content preserves the active revision. Revision is a canonical content hash, not a timestamp; all instances with equivalent definitions should report the same revision.

Initial load failure returns an error and prevents normal construction. An intentionally empty memory document is valid and serves caller defaults. Runtime refresh failure retains the last valid snapshot. Invalid or deleted source files do not clear flags; explicitly publishing an empty valid document does.

`Run` retries on later ticks after source errors and records failure status. Cancellation stops polling promptly. Unexpected loop termination is returned to its lifecycle owner. Refresh errors must not include document contents or subject attributes. No detached goroutines or hidden startup polling.

V1 retains the last valid snapshot indefinitely while the process runs, exposing its age. It does not persist snapshots across restarts. Automatic expiration would need an explicit policy because replacing stale decisions with defaults can itself change application behavior.

## Distribution Contract

V1 provides per-process atomicity and eventual convergence across processes. Instances can disagree while refreshing or when a source is unavailable. With successful source access, convergence depends on the configured polling interval and load time. There is no hard upper bound during an outage.

Memory is process-local. A local file is only shared when deployment infrastructure distributes it. A shared-source adapter can follow later; it must deliver the same complete document contract. Using a cache alone as the authoritative production store would require explicit durability and eviction semantics.

Queued jobs evaluate at execution unless application code deliberately persists an earlier decision. Snapshot consistency ends at the process boundary. V1 offers no distributed transaction or fleet-wide atomic switch.

## GoForj Integration

Introduce an optional Feature component after the standalone library is stable:

- Disabled component emits no dependencies, packages, environment keys, commands, or runtime wiring.
- Generated provider constructs one client per App process and injects it into consumers.
- Generated runtime lifecycle owns the refresh loop for HTTP, worker, scheduler, and aggregate execution. Aggregate execution starts only one loop for its shared client.
- Short-lived commands load once and exit without polling.
- App configuration selects memory or file source, source path, and refresh interval at startup. The flag document supplies dynamic behavior.
- Each App can select its own document. No ambient promotion of App-prefixed flag values into the process environment.
- Existing installations receive integration through the normal render/upgrade mechanism, with ordinary user-owned composition files preserved.

Exact component identifiers, environment names, generated paths, and command signatures must be checked against renderer conventions during implementation. This proposal does not reserve them yet.

`forj dev` must exclude the selected runtime flag document from rebuild triggers while preserving source-code and `.env` rebuild behavior. Verify this through a rendered App test, rather than assuming the runtime poller prevents supervisor restarts.

Initially expose read-only status: active revision, last successful refresh, snapshot age, and refresh failure class. Flag refresh failures after startup report degraded freshness; they do not automatically make the whole App unready.

Future Lighthouse inspection must read the running App's state. A new CLI process reading the file cannot prove which revision a running worker has adopted. Mutation commands, UI writes, audit persistence, and authorization for administration are a separate design.

Do not expose full rules, tenant lists, or raw targeting attributes to browser clients. No per-subject metric labels. Flag names are allowed only within a bounded registered inventory; unknown lookup keys must not create unbounded telemetry.

## Build Versus Integrate

GO Feature Flag already supports polling, multiple retrievers, targeting, rollouts, instance clients, and optional persisted configuration. Those existing capabilities substantially reduce the case for another general-purpose engine. See its [configuration documentation](https://gofeatureflag.org/docs/go_module/configuration) and [rollout overview](https://gofeatureflag.org/product/rollouts).

OpenFeature provides a vendor-neutral evaluation API with provider integration. Its [Go SDK documentation](https://openfeature.dev/docs/reference/technologies/server/go/) is the reference for a possible interoperability path.

Recommended boundary: own the compact native contract above if a dependency-light, explicitly injected GoForj primitive is the product goal. Integrate GO Feature Flag instead if rich rules, remote providers, experimentation, or broad OpenFeature compatibility are near-term requirements. A wrapper that hides those differences would not eliminate their complexity.

Before creating the sibling repository, prototype the two core paths: native snapshot evaluation and an instance-based GO Feature Flag integration. Compare dependency footprint, rule semantics, lifecycle ownership, failure behavior, and request snapshot support. Record the outcome here. No performance or dependency-size claims are established by this proposal.

Do not promise that changing an external provider preserves targeting assignments. Hosted evaluation providers and snapshot sources have different contracts. An OpenFeature adapter, if added, needs its own conformance and lifecycle work; v1 does not claim OpenFeature compliance.

## Delivery Plan

1. Resolve build-versus-integrate with the bounded prototype and freeze rule semantics, limits, and rollout vectors.
2. Ship the standalone library: memory source, boolean evaluation, targeting, immutable views, file source, explicit refresh lifecycle, status, executable examples, and contract tests.
3. Release the sibling library, then add GoForj component wiring and update every applicable dependency pin. Preserve existing module Go versions unless a concrete dependency constraint requires a change.
4. Validate fresh rendered compositions and upgrade behavior, then update authoritative framework docs and generated library documentation.
5. Consider shared sources and read-only Lighthouse inspection based on actual usage. Administration and experimentation remain separate work.

Each stage should produce a focused PR. The native implementation and the integration alternative should not both become supported engines by accident.

## Acceptance Criteria

- Table-driven evaluation tests cover every validation branch, decision reason, rule ordering, empty identities, exact attribute matching, missing flags, and caller defaults.
- Golden rollout vectors prove reproducibility, boundary percentages, stable membership, and deliberate salt changes.
- Race tests prove immutable views, concurrent refresh/evaluation, caller-data isolation, loop ownership, and cancellation.
- Source tests cover malformed and duplicate-key JSON, unknown fields, oversized data, file replacement, deletion, read failure, and recovery.
- Refresh tests prove last-valid retention, unchanged revisions, serialized publication, and no partial snapshots.
- A rendered App changes a flag decision without changing PID, while `.env` changes retain their documented behavior.
- Generated HTTP, worker, scheduler, aggregate, CLI-only, additional-App, component-disabled, and largest supported compositions have appropriate lifecycle coverage.
- Discover and validate every affected Go module. Validate published dependencies with `GOWORK=off`, and perform all GoForj test renders under `/tmp`.
- Existing App behavior is unchanged until the component is explicitly selected and application code uses it.

## Deferred

Multivariate values, experimentation analytics, exposure tracking, scheduled rollouts, percentage overrides by environment, recursive rule languages, browser SDKs, persistent last-valid snapshots, write APIs, and distributed atomic promotion are outside v1.

The proposal is useful only if the first version stays small enough to explain, test, and operate without a feature-management service.

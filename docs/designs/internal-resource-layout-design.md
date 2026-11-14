# Internal Resource Package Layout

## Status

- Design status: proposed
- Planning date: 2026-11-14
- Target repository: `goforj`
- Decision scope: package layout and responsibility boundaries in generated Projects
- Implementation scope: a separate framework layout change; this design does not relocate code

## Decision

Standardize framework resource families under `./internal/resources/<family>` in generated Projects. Preserve the existing leaf package names and keep application domains flat under `internal/`.

The governing rule is:

> Resources are configured dependencies whose instances and lifetime the App manages. Application packages implement the behavior that uses them.

This is a long-term architectural decision. Its justification is the distinction between acquiring configured dependencies and implementing application behavior, independently of migration effort or the scope of current enhancement PRs.

`internal` establishes the application's implementation boundary through Go's import rules. `resources` organizes one coherent category within that boundary. There is no Go package at `internal/resources`, common resource interface, or generic service locator. Each family remains an independent package composed through ordinary typed providers.

The root-level `./resources` alternative would expose ordinary importable packages for managers that depend on a particular Project's configuration and composition. Reusable library contracts and drivers continue to live in their standalone GoForj modules.

## Package Layout

```text
internal/
  resources/
    caches/
    database/
    events/
    features/
    mail/
    notifications/
    queues/
    searches/
    storages/
  auth/
  billing/
  catalog/
  checkout/
  http/
  logger/
  metrics/
  runtime/
app/
  search.go
  wire/
```

The tree illustrates responsibilities, rather than a mandatory component selection. Disabled components emit no corresponding resource package. Feature, Search, and Notification are separately proposed integrations; their inclusion here defines their destination when those components are available.

| Family | Resource responsibility | Application responsibility |
| --- | --- | --- |
| `caches` | Configured stores and reusable cache handles | Keys, invalidation, and application cache policy |
| `database` | Configured connections, pools, and named connection access | Models, repositories, and transaction workflows |
| `events` | Configured buses, transport integration, and subscription mechanisms | Domain event types, subscribers, and reactions |
| `features` | Configured evaluators, source loading, and refresh | Flag definitions, targeting inputs, and rollout decisions |
| `mail` | Configured mailers and transport integration | Templates, recipients, and decisions about sending mail |
| `notifications` | Configured senders and transport lifetime | Audience selection, preferences, escalation, and fallback workflows |
| `queues` | Configured queue backends and dispatch handles | Job payloads, handlers, and processing workflows |
| `searches` | Configured index handles and backend lifetime | Schemas, document mapping, provisioning, and rebuild workflows |
| `storages` | Configured disks and object-store handles | Object naming, access policy, and business retention rules |

A resource name identifies a configured instance, such as the `checkout` evaluator or `catalog` index. It does not create another package directory. Additional Apps use the same family packages and receive their own configured managers through their Wire graphs. Package placement does not change App isolation or root-resource inheritance.

## Admission Rule

A new resource family must satisfy all four conditions:

1. It binds one or more independently configured logical instances, such as a connection, disk, index, evaluator, sender, or client.
2. Its application-facing result is a reusable, domain-neutral handle for application work. Hosting, coordinating, or instrumenting the App's own execution belongs to runtime and support packages.
3. App composition owns construction, initialization, and release where applicable. Consumers use the resulting handles without constructing a separate backend per operation.
4. Domain entities, business decisions, schemas, and execution orchestration remain outside the resource implementation.

An instance can be in-process, file-backed, or remote. A network connection, external service, or nontrivial close operation is not a prerequisite. A configuration knob, injected constructor, or generated source file is not sufficient by itself.

The [resource catalog](../../project/resource_catalog.go) already distinguishes configured resource families from backing services such as Redis and Postgres. The catalog and the family layout must agree, but catalog membership must follow this admission rule. Adding a catalog entry does not establish an architectural justification on its own.

## Boundaries as the Framework Grows

Placement follows responsibility rather than the name of a technology.

| Capability | Boundary |
| --- | --- |
| Secret stores | Configured secret-provider bindings can form a resource family. Secret values, application configuration, and authorization policy stay outside it. |
| Outbound HTTP | Framework-managed client bindings can qualify. A billing-specific API adapter belongs with billing, and a generic HTTP helper needs no resource wrapper. |
| Model providers | Configured model-provider clients can qualify. Prompts, tools, agents, and business workflows remain application behavior. |
| Metrics and logging | App instrumentation, collection, output policy, and export runtime remain in their support packages. A separately configured client for querying an external metrics service can qualify as a resource. |
| HTTP serving | Servers, routes, controllers, and ingress assembly remain runtime or application behavior. |
| Scheduling | The execution host and scheduled work remain runtime and application behavior. A configured client for an external scheduling service can qualify separately. |
| Auth | Account, session, and security policy remain under auth. Configured identity-provider client bindings can qualify as resources. |

These examples establish a test for future proposals rather than reserving new packages now. They do not require creating a manager for every imported library.

The namespace must not accumulate generic `common`, `services`, `clients`, or `helpers` packages. Private helpers belong to the family that needs them. Cross-cutting runtime support retains its own package. Resource families may use support such as logging and instrumentation, but must not import application domain packages to acquire their dependencies.

The framework also uses resource terminology for discoverable development URLs. The exploratory [Resource Registry Design](resource-registry-design.md) describes App, OpenAPI, Mailpit, and dashboard links. That model is separate from dependency bindings. Its implementation must use a discovery or links namespace rather than taking ownership of `internal/resources`.

## Application and Resource Composition

Services continue to receive typed dependencies. Changing an import from `example.com/shop/internal/caches` to `example.com/shop/internal/resources/caches` does not change the leaf package name `caches` or the consumer's `*caches.Manager` parameter type spelling.

App accessors retain their existing names and signatures apart from the qualified package paths of relocated framework types. Examples include `App.Caches()`, `App.Storage()`, `App.Mail()`, `App.Events()`, and `App.Queues()`. Proposed Feature, Search, and Notification accessors follow the same rule.

Wire constructs each App's manager graph and retains its existing cleanup responsibilities. Application providers remain under `app/wire/` or `app/<name>/wire/`. The namespace introduces no global resource registry, universal lookup API, hidden lifecycle, or implicit cross-App sharing.

Search demonstrates the intended direction: App-owned `SearchDefinitions()` supplies domain schemas to the resource manager. The manager does not import the owning App or choose its schema migration policy. Provider code at the composition boundary connects those responsibilities.

## Separate Existing Mixed Responsibilities

Adopting the namespace requires a clean responsibility boundary within the packages themselves.

### Events

The current `internal/events` package contains bus support alongside application event types. Ungrouped `make:event` and `make:subscriber` output also defaults there.

Move framework bus support to `internal/resources/events`. Keep application event payloads and subscribers under their owning domains. Preserve `internal/events` as an application-owned fallback for ungrouped event and subscriber makers; grouped commands continue to select domain packages such as `internal/billing`.

Generated application code imports the resource bus package where needed. It must not be moved wholesale into the resource namespace merely because its current package also contains bus support.

An ungrouped subscriber's Wire set can import both `internal/events` and `internal/resources/events`, whose declared package names are both `events`. Generators must assign distinct, stable import references, for example `events` for application constructors and `eventbus` for the resource manager. Import insertion, constructor references, Wire generation, and removal must agree on those references and identify packages by import path.

Existing source can use a single `internal/events` import for both framework bus symbols and application payload symbols. Migration must distinguish those references and split the import where needed. Application files already inside `internal/events` may also reference framework bus types without an import qualifier. Those references need explicit reconciliation when bus support moves. A blanket package-path replacement cannot perform this separation safely.

### Mail and Auth

The current mail package contains `AuthDelivery`, which implements the auth delivery contract and imports auth-owned payloads.

Place that integration at the auth or App composition boundary. The generic `internal/resources/mail` package owns mailer configuration and transport mechanics without importing auth. Message decisions and auth-specific rendering remain outside the resource family.

### File Ownership

The resource directory describes responsibility, not a blanket overwrite policy. Framework-generated files, preserved extension files, and application-owned adapters retain explicit file-level ownership rules. Directory ancestry alone cannot authorize regeneration or deletion.

Application behavior makers must not implicitly select `internal/resources` from a grouped name or output default. Reserve that namespace in the namespaced layout and reject application-behavior maker destinations beneath it with a diagnostic pointing to domain placement. Resource generators and documented resource implementation extension points retain their own ownership contracts.

## One Path Contract Across Framework Surfaces

Rendering, focused generation, maker output, inspection, cleanup, and upgrade snapshots must consume one Project layout decision. No surface may independently infer a destination from the presence of a resource directory.

The implementation must update these surfaces together:

- Generator output paths, module/import publication, and legacy cleanup contracts.
- Resource template mappings, App assembly, Wire providers, observers, runtime discovery, and shell commands.
- Application makers that import resource types, including jobs, repositories, events, and subscribers.
- Starter kits, demos, Gopher Eats, executable scenarios, integration fixtures, and largest supported compositions.
- Component removal, disabled-component residue checks, and framework upgrade artifact ownership.
- Public documentation, generated examples, and Atlas ownership, guidance, and evaluation paths where they reference the layout.

The [generation output contract](../../internal/generate/output_paths.go) is one existing source of authoritative paths. The implementation should extend the existing contracts rather than introduce a second independent path catalog. Template sources and generators remain authoritative; checked-in projections must be regenerated.

CLI implementation packages in the GoForj repository are a different scope. This design does not relocate `goforj/internal/forj/resources`, the Project catalog, or the framework's own tooling packages simply because their names contain resources.

## Compatibility and Rollout

The destination architecture is one consistent nine-family namespace. Compatibility handling must preserve that decision without silently relocating application source during ordinary rendering.

| Concern | Contract |
| --- | --- |
| Source and API | Moving package import paths changes application source, including preserved Wire providers and resource types exposed through App methods. Reconcile these explicitly. |
| Configuration | Resource names, component flags, environment keys, provider selection, and App inheritance retain their meanings. Record layout identity independently of which components happen to be enabled. |
| Persisted data | Database schemas, queue identities, cache keys, object paths, and search index locations retain their meanings. A Go source path move does not justify changing them. |
| Runtime behavior | Preserve construction, lazy initialization where supported, readiness, refresh, shutdown, and cleanup behavior for each family. |
| Operations | Existing Projects need a reviewable source migration and a recoverable failure path. |
| Minimum Go version | This layout change introduces no minimum Go version increase. |

New Projects use the namespaced layout once implementation is complete. Existing Projects retain their established layout until an explicit migration reconciles framework output and application-owned imports. Supporting existing Projects is a transition contract, not a second recommended architecture.

The implementation must record one Project-wide layout identity durably, including Projects with every resource component disabled. Apps and resource families within one Project share that layout. Empty directories cannot distinguish a legacy Project from a namespaced one. The implementation PR must specify storage in Project/render provenance and the explicit migration command contract before enabling the new default; those details are not introduced by this documentation PR.

Every mutating surface must apply the same interpretation contract:

- New Project creation records the namespaced identity before publishing its resource output.
- An existing Project with absent identity retains legacy interpretation. Absence is not permission to rewrite source or overwrite conflicting namespaced artifacts.
- A supported explicit identity selects its recorded layout independently of enabled components.
- Malformed, unsupported, or conflicting identities fail before Project filesystem or configuration mutation. If multiple provenance records carry identity, disagreement blocks publication.
- Existing artifacts that contradict the selected identity require explicit reconciliation. Their presence may reveal a conflict, but must not silently select a different layout.

Configuration loading, frozen upgrade inputs, render input digests, and artifact snapshots must preserve and account for layout identity. Keeping an unknown field during one config read is insufficient if a later projection discards it.

Migration must plan framework-owned moves, import rewrites in preserved App/domain source, ownership conflicts, application event retention, and old artifact retirement before publication. It must validate the staged result and advance recorded layout only after success. Existing read-only upgrade inspection is not an automatic import migration engine. Follow the transaction and preservation requirements in [Render Ownership And Upgrade Reconciliation](render-upgrade-reconciliation-design.md).

Compatibility forwarding packages may be used only when they refer to one canonical implementation and preserve type identity. They must not construct parallel managers or duplicate clients, refresh workers, subscriptions, or cleanup ownership. They cannot be assumed to preserve arbitrary same-package application extensions.

## Implementation Acceptance Criteria

Implementation is complete when all of the following hold:

1. Every enabled resource family in a namespaced Project follows the namespace contract, with disabled families absent and no empty umbrella package emitted. Legacy Projects retain their selected layout until explicit migration.
2. Domain packages remain flat; application events, schemas, job handlers, and auth delivery policy stay outside resource implementations.
3. All path-producing and path-inspecting surfaces agree for the recorded Project layout. Repeated rendering and generation produce no further diff.
4. Default and additional Apps retain independent manager graphs, existing named-resource inheritance, and correct lifecycle behavior.
5. Fresh maximum-component and mixed-App compositions compile, build, and execute relevant resource behavior tests.
6. Application makers create and remove domain code in the intended packages while importing the correct resource types. Ungrouped event/subscriber tests exercise the two `events` packages through creation, Wire generation, compilation, and removal without losing the bus import or confusing constructors.
7. Customized legacy Projects retain working imports until migration. Migration preserves owner files, separates mixed framework/domain references, handles conflicts without partial publication, and is repeatable after success.
8. Disable/re-enable transitions and generated-file retirement preserve application-owned neighbors.
9. Backup, local-provider switching, inspection, and shell commands retain resource identity and storage semantics.
10. Root and relevant nested Go modules are validated, intentional sibling replacements are preserved, and released-module resolution is checked with `GOWORK=off` where applicable.
11. Authoritative templates, generated examples, public documentation, and Atlas path policies agree with the implemented layout.
12. Layout identity tests cover absence, supported identities, malformed values, unsupported values, conflicting provenance, contradictory artifacts, and all-components-disabled Projects. Failure leaves Project state unchanged, and frozen upgrade inputs and snapshots retain the selected identity.

Tests must exercise actual generated code, retained custom providers, and resource behavior. A search-and-replace assertion over template strings is insufficient evidence for the namespace contract.

## Alternatives Considered

| Alternative | Architectural assessment |
| --- | --- |
| Flat resource families under `internal` | Simple and legitimate, but configured dependency managers compete with application domains for names and become harder to distinguish as the ecosystem grows. |
| Root `resources` | Organizes the category but omits the application implementation boundary supplied by `internal`. |
| `internal/infrastructure` | Broad enough to absorb servers, telemetry, auth, deployment, and application adapters without a precise admission rule. |
| `internal/forj` | Groups by framework provenance rather than responsibility and can become a bucket for unrelated support mechanisms. |
| Namespace only the three new families | Gives equivalent resource families two locations and weakens the convention. |
| A universal resource abstraction | Adds coupling between families with different APIs and lifecycle needs. Directory organization requires no such abstraction. |

The chosen layout gives one recognizable dependency category a stable home while preserving independent packages and prominent application domains.

## Related Sources

- [App Structure](../context/app-structure.md)
- [Runtime Architecture](../context/runtime-architecture.md)
- [Generated App Extension Points](../context/generated-app-extension-points.md)
- [Resource catalog](../../project/resource_catalog.go)
- [Generation output paths](../../internal/generate/output_paths.go)
- [App assembly template](../../templates/wire/app.go.tmpl)
- [Render Ownership And Upgrade Reconciliation](render-upgrade-reconciliation-design.md)
- Proposed framework resource integrations: [Feature #274](https://github.com/goforj/goforj/pull/274), [Notification #273](https://github.com/goforj/goforj/pull/273), and [Search #276](https://github.com/goforj/goforj/pull/276).

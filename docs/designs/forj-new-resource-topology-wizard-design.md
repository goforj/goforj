# `forj new` Resource Topology Wizard Design

## Status

- Design status: accepted
- Research date: 2026-07-13
- Implementation date: 2026-07-14
- Scope: `forj new`, initial resource-driver selection, generated environment
  defaults, and local service planning
- Target repository: the permanent `goforj` repository
- Implementation status: implemented; the release checks in the final section
  remain intentionally open

## Decision Summary

`forj new` should add one compact **App Resources** stage after components,
starter-kit selection, and extras have finalized the project's capabilities.
The normal path asks for only two related decisions:

- a starting resource shape:
  - **Standalone resources**
  - **Shared through Redis**
- a database implementation, independently:
  - MySQL
  - Postgres
  - SQLite

The stage is primarily a review screen. **Continue** is focused initially, so a
user who accepts the defaults advances with one Enter press. **Advanced resource
setup** exposes per-resource active and built-in driver choices without making
the normal path exhaustive.

The two normal shapes coordinate only the primitives for which GoForj has a
clear process-local and Redis-backed transition:

| Resource | Standalone resources | Shared through Redis | Built in by default |
| --- | --- | --- | --- |
| Cache | `memory` | `redis` | `memory,redis` |
| Queue, when Jobs is enabled | `workerpool` | `redis` | `workerpool,redis` |
| Events | `inproc` | `redis` | `inproc,redis` |

Database selection remains orthogonal. A project using standalone resources
with MySQL or Postgres is a first-class outcome, not an Advanced-mode exception.
Storage remains local and mail retains its own development policy in both
shapes.

The common local and Redis drivers are compiled into normal generated Apps.
Changing between them later therefore changes deployment configuration, not
application code or the generated binary. As an additional independent
portability decision, Mail-enabled normal projects build both `log` and `smtp`.
The generated Compose file also contains an optional Redis service behind a
profile, while the environment activates that profile when Redis is locally
managed. This makes the common Redis transition possible without rerendering
the project.

The shape is a one-time wizard macro. It is expanded into concrete active and
supported driver environment values and is never persisted as an architectural
identity. Active drivers and supported drivers do not belong in `.goforj.yml`,
and no shape or runtime-mode identity is persisted.

## Product Promise

The concise promise shown by the wizard is:

> Choose a starting setup. Most resources can switch drivers later without
> changing application code.

The fuller contract documented in the generated project is:

> Common app-local and Redis drivers are already built in. Switch an active
> driver through environment configuration, make its required service
> available, then restart or redeploy the App. Adding a driver that was not
> built in requires regeneration and a new build. Stateful resources may also
> require draining or data migration.

This distinction matters. GoForj can make driver changes unsurprising without
claiming that cache contents, queued jobs, event delivery semantics, database
data, or stored files move automatically.

## Why This Change Is Needed

GoForj has a broad driver surface, but asking a new user to choose a driver for
every primitive would make the wizard teach infrastructure before it creates an
App. Hiding every decision is also insufficient because the starting defaults
determine whether state can be shared across App processes.

The current project model contains the pieces of the right abstraction:

- Apps depend on cache, queue, events, storage, mail, and database resources.
- Sibling repositories own the implementations behind those resource APIs.
- `*_DRIVER` selects the active runtime implementation.
- `*_SUPPORTED_DRIVERS` controls which implementations are generated and built
  into the App.
- generated environment files own deployment-specific selections.
- GoForj owns the integration policy that maps those selections to generated
  source, local services, and documentation.

What is missing is a coordinated starting policy. In particular, selecting
Redis for cache but not queue or events fragments the simplest shared-service
deployment for no useful reason. Conversely, selecting workerpool jobs should
not start an unused Redis container merely because Jobs is enabled.

The design therefore treats the normal choice as a small resource-topology
macro, then derives every concrete driver and service decision from it.

## Terminology

### Starting resource shape

A **starting resource shape** is the transient wizard choice that expands into
initial active-driver values. It is not a durable project profile.

The UI labels are deliberately concrete:

- **Standalone resources** means the root cache, enabled queues, and events use
  implementations inside the App process.
- **Shared through Redis** means those resources use one Redis service and can
  coordinate across App processes.

The second option may be described conversationally as the shared-services
shape, but **Shared through Redis** is the user-facing selection. It says what
will actually be introduced and avoids implying that every primitive becomes
distributed.

### Starting driver and built-in drivers

The wizard uses these labels instead of exposing environment terminology first:

- **Starts with** maps to `*_DRIVER`.
- **Built into App** maps to `*_SUPPORTED_DRIVERS`.

Generated documentation continues to teach the exact environment variables.

### Standalone runtime versus standalone resources

GoForj already uses **standalone** for the combined `./bin/app run` host. That is
a process-launch topology. This design uses the qualified term **Standalone
resources** and does not change how the App launches.

A project may:

- run one combined App process while using shared Redis resources;
- run separate API, worker, or scheduler processes while using the shared
  resource shape;
- use MySQL or Postgres while cache, queue, and events remain process-local.

The resource shape does not select binaries or claim that separate runtime
orchestration is operational. Runtime topology is expressed by the command a
deployment launches (`app run` or an explicit leaf runtime), not by a
`RUNTIME_MODE` environment variable.

## Design Grounding

This design follows existing GoForj boundaries:

- [App resource shell commands](app-resource-shell-commands-design.md) treats
  database engines and similar technologies as drivers behind App-owned
  resources.
- [App composition layout](app-composition-layout-design.md) keeps generated App
  composition separate from reusable sibling packages.
- [Primitive suggestions](primitive-design-suggestions.md) anticipates further
  driver-backed resources, such as locks.
- [Lazy infrastructure initialization](cli-lazy-infrastructure-initialization-design.md)
  requires supported but inactive infrastructure not to be initialized.
- [Environment contract generation](env-contract-generation-design.md) defines
  the safe committed `.env.example` needed to reproduce build inputs without
  committing local secrets.
- [Runtime architecture](../context/runtime-architecture.md) defines standalone
  process hosting independently from the resource decisions here.
- [Starter kits](completed/starter-kits-design.md) establishes that conditional
  questions belong after starter and extra selections have finalized the
  capabilities they depend on.

## Goals

- Keep the normal `forj new` path short and obvious.
- Let one Enter press accept the visible resource defaults.
- Make standalone resources with SQLite, MySQL, or Postgres equally valid.
- Consolidate the simple shared-resource shape onto one Redis service.
- Compile both sides of the common app-local-to-Redis transition by default.
- Make later switching honest and easy for both externally managed and local
  Compose environments.
- Keep runtime deployment state out of `.goforj.yml`.
- Give Advanced users precise per-resource control without duplicating the
  resource model.
- Derive environment, generated source, service planning, confirmation, and
  documentation from one canonical resource catalog.
- Preserve existing environment values during rerenders and migrate the legacy
  queue-driver key without losing user configuration.

## Non-Goals

- Do not ask one normal-path question per primitive.
- Do not label either shape Development, Production, Simple, or Enterprise.
  Standalone resources can be correct in production, and shared Redis is not a
  complete production architecture.
- Do not make storage shared automatically.
- Do not migrate database rows, stored objects, cache contents, queued jobs, or
  event history.
- Do not provision cloud accounts or managed infrastructure.
- Do not redesign sibling-repository driver APIs.
- Do not introduce a runtime-mode environment switch; launched commands own
  process topology.
- Do not add topology selection to `make:app` in the first slice.
- Do not persist a shape name that could later drift from concrete environment
  values.
- Do not expose a Lean-build toggle in the first slice. Portability is the
  normal default; Advanced per-resource support is sufficient initially.

## Wizard Placement

The stage must run after starter kits and extras because those choices can add
Jobs, Auth, Mail, Demo, or database requirements. It must run before Atlas,
path, and confirmation because rendering and confirmation need the resolved
resource plan.

```text
Project Name
  → Module
  → Components
  → Help
  → Starter Kit, when Web UI is enabled
  → Extras
  → App Resources
  → Atlas
  → Path
  → read-only target reconciliation
  → Confirm
```

Target reconciliation is an internal transition, not another question. It is a
no-op for an empty destination.

The old queue-only Runtime stage is replaced rather than supplemented. There
must not be a second queue-driver question.

Back navigation follows the same conditional path in reverse and retains the
resolved resource plan and any Advanced edits.

When the user backs up and changes an earlier capability:

- preserve selections for resources that are still applicable;
- remove disabled resources from the effective plan and service summary;
- initialize newly applicable resources from the current base shape;
- retain the user's independent database choice unless a new capability
  constraint locks it;
- restore the prior unlocked database choice when a temporary lock, such as
  Demo, is removed;
- surface any new conflict on App Resources instead of silently repairing it.

Choosing a different value in **Starting resources** is a deliberate preset
operation. It reapplies active and normal supported values to shape-managed
root and generated named resources. It does not reset Database, Storage, Mail,
or unrelated Advanced configuration without saying so.

### Existing target reconciliation

`forj new --allow-non-empty` can select a directory that already contains an
owner-controlled `.env`, but the wizard does not know the target path when App
Resources is first shown. After Path is accepted and before Confirm or any
write, the wizard performs a read-only reconciliation:

1. Load the target's existing `.env`, safe `.env.example`, App scopes, and any
   legacy `queue_driver` migration source.
2. Apply the ownership precedence in this design: existing concrete values win,
   and the selected resource plan fills only missing initialization keys.
3. Validate active/supported pairs, capability constraints, service placement,
   and database compatibility.
4. Recompute the effective shape/custom label, named-resource requirements, and
   project-default service plan.
5. Show those effective values on Confirm, including a concise **Existing target
   overrides** notice when they differ from the earlier selection.

An invalid existing contract blocks confirmation with an actionable error and
does not rewrite either file. A valid owner override is preserved rather than
silently replaced by the wizard choice. Changing Path reruns reconciliation,
and backing up retains the user's proposed plan for use with another target.

## Normal-Path Experience

The normal screen is a compact summary with two editable rows:

```text
App Resources

Choose a starting setup. Most resources can switch drivers later
without changing application code.

  Starting resources   Standalone resources
  Database             MySQL

> Continue
  Advanced resource setup
```

Behavior:

- **Continue** has initial focus.
- Enter on Continue accepts the visible defaults.
- Moving to **Starting resources** opens the two-choice shape picker.
- Moving to **Database** opens MySQL, Postgres, and SQLite.
- The Database row is hidden when the Database capability is disabled.
- A capability constraint may require the row or lock its available drivers;
  the row always explains the feature that owns that constraint.
- `a` is an optional shortcut for Advanced resource setup.
- Escape or Back returns without discarding edits.
- The screen fits the existing 90-column layout without requiring scrolling.

### Starting-resource choices

```text
Standalone resources
Memory cache · Workerpool jobs · In-process events
These resources stay inside the App process. Choose any database separately.

Shared through Redis
Redis cache · Redis jobs · Redis events
One Redis service lets these resources work across App processes.
```

When Jobs is disabled, job wording is omitted from both descriptions rather
than showing an irrelevant choice.

The default is **Standalone resources**. This avoids making Redis a mandatory
dependency for a new App while the portable build still leaves the common
transition available.

### Database choice

Database is independent of the resource shape:

| Database | Standalone resources | Shared through Redis |
| --- | --- | --- |
| SQLite | Allowed | Allowed with a scope warning |
| MySQL | Allowed | Allowed |
| Postgres | Allowed | Allowed |

MySQL remains the initial default so this design does not smuggle in an
unrelated default change. SQLite stays one selection away for a self-contained
App, while MySQL and Postgres support the common case of one real infrastructure
component with otherwise process-local resources.

MySQL and Postgres are database-driver identities, not placement identities.
The wizard must not add `MySQL Local`, `MySQL External`, `Postgres Local`, or
`Postgres External` as separate database choices. For a new Docker-enabled
project, choosing MySQL or Postgres includes the conventional local Compose
service. Without Docker the service is external, and a production deployment
uses the same driver with its managed endpoint while simply not launching the
development Compose service. Placement is deployment and service-planning
context; encoding it in the driver name would duplicate choices and make later
movement look like an application architecture change.

The component stage should present Database as one capability rather than three
peer capabilities. The App Resources stage owns its implementation choice.
Internally, the first implementation can continue mapping that choice onto the
existing mutually exclusive MySQL, Postgres, and SQLite component flags. This
preserves render compatibility without duplicating the choice in the UI.

Toggling the Database capability off and back on preserves the last independent
implementation choice. Auth or OAuth prevents it from being disabled and shows
the dependency immediately; the user must not discover that requirement only at
late render validation. Demo both requires Database and applies the driver lock
below.

Components, starter kits, and extras resolve capability constraints before the
App Resources stage. Auth and OAuth require the Database capability but allow
its normal implementations. The Demo extra currently forces MySQL, and its
migration set does not support Postgres. The first slice therefore shows MySQL
as locked for Demo and explains that Demo owns the constraint. Adding SQLite or
Postgres as selectable Demo databases requires a separate compatibility audit
and complete dialect coverage; the new screen must not imply those choices work
merely because ordinary Apps support them.

No earlier choice may silently reset a resource selection after the App
Resources stage. If a future stage introduces a new constraint, the wizard must
return to App Resources with the conflict visible rather than silently changing
the plan.

**Shared through Redis + SQLite** does not mean the entire App is replica-ready.
The wizard and confirmation say that cache, jobs, and events are shared while
SQLite and local storage remain tied to a filesystem.

## Standard Resource Plan

The following table is the authoritative normal-path policy:

| Resource | Applies when | Standalone active | Shared active | Built in by default | Service effect |
| --- | --- | --- | --- | --- | --- |
| Database | Database enabled | selected database | selected database | selected database, plus starter-required fallbacks | MySQL or Postgres service when locally managed; none for SQLite |
| Root cache | Always | `memory` | `redis` | `memory,redis` | Redis only when active |
| Queue | Jobs enabled | `workerpool` | `redis` | `workerpool,redis` | Redis only when active |
| Events | Always | `inproc` | `redis` | `inproc,redis` | Redis only when active |
| Storage | Always | `local` | `local` | `local` | none |
| Mail | Mail enabled, Docker enabled | `smtp` | `smtp` | `log,smtp` | Mailpit as a development tool |
| Mail | Mail enabled, Docker disabled | `log` | `log` | `log,smtp` | none; SMTP can be configured externally later |

Ordering is part of the generated contract. Built-in lists are deduplicated and
use the local implementation before the shared implementation, as shown above.

Starter kits may extend the database build contract when they genuinely require
another implementation. In particular, the current Demo behavior that builds
SQLite alongside its MySQL default should be preserved until the starter itself
no longer depends on that portability. This exception does not make database
part of the resource-shape macro.

Queue variables are omitted when Jobs is disabled. The absence of a queue
capability must not add queue dependencies, driver configuration, or Redis
service planning.

### Why storage remains local

There is no single low-cost shared storage choice analogous to Redis for cache,
queue, and events. S3, GCS, FTP, SFTP, Dropbox, and rclone introduce different
credentials and operational assumptions, and existing files require migration.
The normal shape therefore leaves storage local. Advanced mode can select a
different starting and built-in driver deliberately.

### Why mail remains independent

Mail delivery does not become process-shared in the same sense as cache, queue,
or events. Its normal active driver continues to follow the local development
experience: SMTP through Mailpit when Docker is enabled, otherwise log. Building
both `log` and `smtp` keeps that common transition inexpensive without tying it
to the resource shape.

### Future primitives

New primitives join a normal shape only when they have a similarly coherent
pair. A future lock primitive is the obvious example:

| Resource | Standalone active | Shared active | Built in by default |
| --- | --- | --- | --- |
| Lock | `memory` | `redis` | `memory,redis` |

This should be a catalog addition, not another wizard conditional.

## Named Resource Policy

Root driver values do not automatically govern every named resource. Generated
environment variables such as `CACHE_SESSIONS_DRIVER` override `CACHE_DRIVER`,
so the preset resolver must classify generated named resources explicitly.

Initial policy:

| Named resource | Standalone resources | Shared through Redis | Reason |
| --- | --- | --- | --- |
| Auth sessions | `memory` | `redis` | Sessions must be visible across App processes in the shared shape. |
| Inspection cache | `memory` | `memory` | Diagnostic data is intentionally process-local. |
| Lighthouse cache | `memory` | `memory` | Lighthouse inspection state is intentionally local to its process. |
| Demo settings cache | `memory` | `memory` initially | It is a database-backed optimization; changing its consistency policy is separate work. |
| Future rate-limit cache | `memory` | `redis` | Cross-process limits need shared counters. |
| Generated named queues | inherit root | inherit root | Queue generation already falls back to the root driver. |
| Generated named event buses | explicit `inproc` | explicit `redis` | Event children currently default independently to `inproc`, so the preset must seed the intended value. |
| Named storage disks | `local` | `local` | Storage is outside the normal shape. |

The catalog records this policy so environment rendering and service discovery
cannot disagree.

User-created named resources are not edited by `forj new`, and later rerenders
must never silently rewrite their explicit environment overrides. Advanced mode
in the first slice controls the generated root resources, not an unbounded list
of named instances.

Generated named-resource requirements remain visible even though they are not
editable rows in the first Advanced slice:

- they are resolved from the most recently applied base shape;
- resources marked **inherit root** follow an explicit Advanced root-driver
  change, while shape-specific resources such as Auth sessions and generated
  event buses retain their resolved shape policy;
- every named active driver is locked into the root supported-driver list;
- attempting to remove a required built-in driver explains the owner, for
  example `Redis is required by Auth sessions`;
- they participate in shape classification, service planning, and confirmation.

Selecting a different normal shape deliberately recomputes all shape-derived
named values. Disabling the owning capability removes its generated requirement.
User-authored named overrides remain owner-controlled and are discovered during
rerender rather than rewritten from the original shape.

Do not change the global named-event fallback as an accidental side effect of
this wizard. Existing user-authored event children with no explicit driver
currently default to `inproc`; changing that behavior requires its own migration
decision. The wizard writes explicit values only for named resources it
generates and owns.

## Portable Build Contract

Normal generated Apps contain both sides of the common transition:

```env
CACHE_SUPPORTED_DRIVERS=memory,redis
QUEUE_SUPPORTED_DRIVERS=workerpool,redis
EVENTS_SUPPORTED_DRIVERS=inproc,redis
```

Standalone resources start with:

```env
CACHE_DRIVER=memory
QUEUE_DRIVER=workerpool
EVENTS_DRIVER=inproc
```

Shared through Redis starts with:

```env
CACHE_DRIVER=redis
QUEUE_DRIVER=redis
EVENTS_DRIVER=redis
```

The contract has four invariants:

1. Every active driver is present in the corresponding supported list.
2. Supported but inactive drivers are generated and compiled, but they do not
   connect, register health checks, run migrations, or start lifecycle hooks.
3. Changing to an already built-in driver requires environment changes and an
   App restart or redeploy, not application-code changes or a rebuild.
4. Adding a driver absent from the supported list requires updating the build
   contract, regenerating source, and building a new artifact.

The generated source and README files must use **built in** and **active**
consistently. “Supported” must never be presented as “currently running.”

Some current generators unconditionally retain a small native fallback, such
as workerpool for queues, log for mail, local for storage, or SQLite database
support. Those implementation baselines may remain, but they do not weaken the
public contract: the supported list is the authoritative set the user may
select, and validation rejects an active driver outside it.

That authority has two phases:

- during generation, `*_SUPPORTED_DRIVERS` selects the intended driver manifest;
- inside an existing artifact, an embedded compiled-driver manifest is the
  authority and runtime startup validates every effective active driver against
  it.

The runtime does not reinterpret a changed `*_SUPPORTED_DRIVERS` string as if it
could alter an existing binary. A native fallback may remain compiled as an
implementation detail, but it is not selectable unless the generated manifest
includes it. The generated error names the unsupported active driver, the
compiled choices, and the regeneration command.

### Switching caveats

The wizard can promise a stable App API, not transparent state transfer:

- Cache changes begin with a cold cache unless data is migrated separately.
- Queue changes require producers to switch coherently and workers to drain the
  old backend; outstanding jobs are not copied.
- Event drivers have different delivery, ordering, and durability semantics.
  Redis events being cross-process does not imply durable event storage.
- Database changes require schema and data migration.
- Storage changes require object migration and URL/visibility review.
- Mail changes require delivery credentials and may change retry behavior.

These caveats appear in generated resource documentation and in Advanced driver
descriptions where space permits. They do not need to burden the normal screen.

## Local Service Planning

The current component-to-service mapping is too coarse for driver portability.
Redis must be derived from effective resource selections, not from whether Jobs
is enabled.

Introduce a pure service planner that consumes:

- enabled project capabilities;
- effective active and supported resource drivers across the default App and
  every generated named App scope;
- named generated resource policy;
- whether Docker support is enabled;
- explicit local-service intent for each locally provisionable service.

It returns deduplicated service requirements with one of four states:

- **active local**: GoForj starts the generated local service;
- **available local**: an optional local definition exists for a built-in
  transition but is not started;
- **local requested, unused**: owner-controlled local-service intent starts the
  service even though no discovered active resource consumes it;
- **external required**: the active driver needs a service that GoForj is not
  managing locally.

Docker availability and an active driver are not enough to infer local
management. A Docker-enabled project may intentionally use managed Redis. For a
new normal project, the preset seeds local Redis intent when Shared through
Redis and Docker are selected. Advanced can choose **Local Compose** or
**External** when the catalog marks an active service placement-selectable;
Redis is the first-slice case. On rerender, the owner-controlled environment and
current Compose-profile state win.

For Redis, the concrete local-management signal is the exact `redis` token in
`COMPOSE_PROFILES`:

| Effective state | Service result |
| --- | --- |
| Redis active, Docker enabled, `redis` profile active | active local |
| Redis active, profile absent or Docker disabled | external required |
| Redis inactive, supported, Docker enabled, profile absent | available local |
| Redis inactive, supported, Docker enabled, `redis` profile active | local requested, unused; preserve and warn |
| Redis inactive and Docker disabled, or Redis unsupported | absent local definition; preserve any unrelated/stale token and warn when useful |

An existing exact `redis` token is owner intent, not something rerender removes
merely because resource drivers changed. “Supported does not start services”
means support alone never adds or activates the profile. An explicitly retained
profile can still start an unused Redis container, which confirmation reports
instead of hiding or silently removing it.

MySQL and Postgres keep their existing Docker-component local-management policy
in the first slice. Their active service is derived from the effective database
driver rather than from every compatibility flag. Generalizing database
services to optional local/external profiles is separate work.

Redis cache, queue, events, sessions, and future locks share one service key.
Multiple compatible consumers therefore produce exactly one Redis requirement.
Today all three normal drivers reuse global `REDIS_HOST`, `REDIS_PORT`,
`REDIS_PASSWORD`, and `REDIS_DB` through their generated connection contracts.
Equivalent cross-driver TLS configuration remains future work.

Shared through Redis may ship only after its external Redis exercise confirms
the generated authenticated client contract across cache, queue, and events,
and generated validation must reject a configuration that one primitive cannot
use. TLS is advertised only after all three drivers can express and verify the
same TLS endpoint policy. The unauthenticated local Compose Redis path remains
the baseline, not evidence that managed Redis is portable.

App-scoped overrides are still included in discovery. `forj new` initializes
only the default App, and this design does not add a topology question to
`make:app`, but a later named App that activates Redis must still affect the
project-level service plan. Consumers deduplicate only when their effective
service key and endpoint mapping match; an App-scoped external endpoint is
reported separately rather than silently pointed at the local Redis container.

Service identity includes kind, placement, and endpoint affinity rather than
only a driver name. Redis-backed Storage selected in Advanced participates just
like cache, queue, and events. Resource-specific Redis addresses, SMTP hosts,
SQL DSNs, and App-prefixed overrides can make otherwise identical driver names
refer to different services. Only consumers explicitly mapped to the generated
local service deduplicate onto it; external endpoints remain separate
requirements.

### Optional Redis Compose profile

For Docker-enabled projects, the normal portable build renders a Redis service
definition whenever Redis is built into at least one relevant resource. The
service is guarded by the Compose profile `redis`:

```yaml
services:
  redis:
    profiles: [redis]
    image: redis:7.4
```

The project-root `.env` file controls the project-default Compose profile
activation. The generated environment activates that profile only when an
effective active resource uses Redis **and** local Redis management was
selected:

```env
COMPOSE_PROFILES=redis
```

Standalone resources leave the profile unset. `docker-compose up -d` therefore
does not start Redis even though the portable local definition is available.
This follows the standard
[Compose profiles contract](https://docs.docker.com/compose/how-tos/profiles/).
It is how the design keeps both promises true:

- Redis can be selected later without rerendering Compose.
- Support alone does not activate or start Redis; the generated Standalone
  default leaves the profile off.

Shell `COMPOSE_PROFILES` and explicit `--profile` flags can override the project
default for a particular Compose invocation. Static wizard confirmation cannot
predict those invocation-time overrides and labels its result as the
project-default service plan.

When switching a local project later, the generated README gives the complete
operation:

1. Change the relevant active `*_DRIVER` values.
2. Add or remove `redis` from the comma-separated `COMPOSE_PROFILES` value
   without discarding any unrelated profiles.
3. Restart the App and reconcile Compose services.

GoForj modifies `COMPOSE_PROFILES` token by token. Existing tokens retain their
relative order, `redis` is appended when newly enabled, and removal deletes only
the exact `redis` token while normalizing empty separators. Rerender does not
replace an existing profile list merely because the project originally used a
different shape.

Moving from shared Redis back to standalone resources must stop the old Redis
container explicitly. While the profile is still addressable, run a command
such as `docker-compose --profile redis down`; then remove the profile and start
the remaining services again. GoForj must not imply that editing an environment
file automatically stops an already running service.

Top-level Redis volume configuration may remain in the Compose file while the
profile is inactive. No container uses it, and preserving the volume makes the
switch reversible.

If the derived service plan contains no active local service, GoForj omits the
`docker-compose up -d` pre-development task. A Compose file containing only
inactive profiled definitions must not fail project creation or `forj dev` with
“no service selected.”

### Docker-disabled projects

Docker controls local service emission, not driver availability. When Docker is
disabled:

- active MySQL, Postgres, Redis, NATS, and similar drivers are reported as
  externally required;
- supported inactive drivers remain valid build choices;
- GoForj does not silently enable Docker;
- cloud and external-service credentials remain placeholders for the user to
  configure after generation.

### Other generated services

Database services, Mailpit, VictoriaMetrics, and Grafana remain distinct in the
service plan:

- the selected MySQL or Postgres database is an App service;
- Mailpit, VictoriaMetrics, and Grafana are development tools;
- Redis is an App service only when an active resource consumes it;
- SQLite, memory, workerpool, in-process events, and local storage do not create
  service requirements.

The distinction lets confirmation answer the user's real question: “What must
run for this App?” without describing Mailpit or Grafana as part of its resource
topology.

## Advanced Resource Setup

Advanced mode edits the same `ResourcePlan` produced by a normal preset. It is
not a second configuration path.

The first screen shows only resources that apply to the selected capabilities:

```text
Advanced Resource Setup

Resource       Starts with       Built into App
Database       MySQL             MySQL
Cache          Memory            Memory, Redis
Queue          Workerpool        Workerpool, Redis
Events         In-process        In-process, Redis
Storage        Local             Local
Mail           SMTP             Log, SMTP
```

Interaction rules:

- Enter on a row edits its starting driver.
- A secondary action edits its built-in drivers.
- A driver explicitly marked placement-selectable exposes **Local Compose** or
  **External**; Redis is the placement-selectable service in the first slice.
- Selecting a starting driver automatically selects and locks it as built in.
- Removing a built-in driver that is active is rejected with a concise reason.
- Removing a driver required by a generated named resource is also rejected and
  names that resource.
- Back preserves edits.
- Reset restores the currently selected normal shape, not global defaults.
- Exact normal active mappings across shape-managed root and generated named
  resources retain their shape label.
- A changed built-in list with the same active mappings displays, for example,
  `Standalone resources · custom support`.
- A divergent shape-managed active mapping displays `Custom`.
- Database never changes the shape label. Advanced Mail or Storage changes add
  `· customized` without erasing an otherwise accurate resource-shape label.
- Advanced uses a grouped, viewport-backed list so the 90-column terminal does
  not grow indefinitely.

Driver descriptions focus on operational consequences:

- runs inside the App process;
- writes to the local filesystem;
- can reuse the selected database when the SQL driver family matches and a
  compatible resource-specific connection mapping is configured;
- adds or reuses Redis;
- requires an external service;
- requires cloud credentials;
- changes delivery or durability behavior.

Selecting an external or cloud driver does not contact infrastructure during
the wizard. GoForj emits the applicable environment placeholders and calls out
the remaining configuration on confirmation.

### Driver inventory

The initial catalog mirrors the generator capabilities already present in
GoForj:

| Resource | Available drivers |
| --- | --- |
| Database | `mysql`, `postgres`, `sqlite` |
| Cache | `memory`, `file`, `null`, `redis`, `memcached`, `dynamodb`, `sqlite`, `postgres`, `mysql`, `nats` |
| Queue | `null`, `sync`, `workerpool`, `redis`, `nats`, `sqs`, `rabbitmq`, `sqlite`, `postgres`, `mysql` |
| Events | `inproc`, `null`, `redis`, `nats`, `natsjetstream`, `kafka`, `gcppubsub`, `sns` |
| Storage | `local`, `memory`, `redis`, `ftp`, `sftp`, `s3`, `gcs`, `dropbox`, `rclone` |
| Mail | `log`, `smtp`, `resend`, `postmark`, `mailgun`, `sendgrid`, `ses` |

The UI groups these by operational shape rather than presenting one alphabetic
wall:

1. process-local and filesystem drivers;
2. SQL-backed drivers that may reuse the selected database with an explicit
   compatible DSN mapping;
3. Redis and other shared infrastructure;
4. managed cloud and external-service drivers;
5. null or synchronous testing/development choices where applicable.

The catalog, not this Markdown table, becomes the executable source of truth.
Tests assert that generator inventories and wizard inventories cannot drift.

Cache and queue SQL drivers currently own resource-specific DSN configuration;
they do not automatically inherit `DB_*`. The service planner therefore
distinguishes a matching SQL engine with an explicit reuse mapping, a different
SQL engine that needs another service, and an external DSN. Advanced must never
silently enable another database capability or assume reuse from a matching
driver name alone. Only catalog entries explicitly marked locally provisionable
receive a generated local service; every other active infrastructure driver is
reported as external.

Advanced database support uses the transitional compatibility contract:

- every selected built-in database enables its project render-capability flag;
- `DB_DRIVER` alone determines the active database;
- only that active MySQL or Postgres driver receives a local Compose service;
- inactive built-in databases do not start services and do not receive the
  no-rerender profile bridge promised specifically for Redis;
- changing an active local database later may require rerendering Compose even
  when the binary already contains its driver, as well as explicit schema and
  data migration.

## Confirmation Experience

The normal confirmation remains compact but separates resource topology from
tooling:

```text
Resource shape     Standalone resources
Database           MySQL
Active resources   Memory cache · Workerpool jobs · In-process events · Local storage
Built-in bridge    Redis for cache, jobs, and events
App services       MySQL
Development tools  Mailpit · VictoriaMetrics · Grafana
```

For Shared through Redis:

```text
Resource shape     Shared through Redis
Database           Postgres
Active resources   Redis cache · Redis jobs · Redis events · Local storage
App services       Postgres · Redis
Development tools  Mailpit · VictoriaMetrics · Grafana
```

When Docker is disabled, **App services** becomes **External services required**
for services GoForj cannot start locally.

Shared through Redis with SQLite adds one concise notice:

```text
Redis resources can cross App processes; SQLite and local storage remain filesystem-local.
```

Custom plans show the expanded resource table. Confirmation is derived from the
same plan and service planner as rendering; it must not reconstruct labels with
wizard-only conditionals.

## Configuration Ownership

### `.goforj.yml`

Project YAML owns durable generation capabilities and policy such as enabled
components, starter-kit selection, render dependencies, and framework version.
It does not own deployment-time active drivers.

Do not add or restore any of these fields:

```yaml
resource_shape:
app_shape:
runtime_mode:
queue_driver:
cache_driver:
events_driver:
storage_driver:
mail_driver:
```

The current database component flags are acknowledged transitional technical
debt. During the transition they mean **render capability**, not active runtime
selection:

- `DB_DRIVER` is the active database.
- `DB_SUPPORTED_DRIVERS` is the database build contract.
- every built-in database driver enables its project-level compatibility flag
  so required templates and generated imports remain available;
- existing environment values win and compatibility flags must never overwrite
  `DB_DRIVER` during rerender;
- when a supported database lacks its compatibility flag, rerender may promote
  the flag additively after reporting the migration;
- extra legacy flags do not expand `DB_SUPPORTED_DRIVERS` or start services;
- an active database absent from `DB_SUPPORTED_DRIVERS` is a validation error.

The one-Database-capability presentation initially applies to `forj new`.
`make:app` keeps its app-exclusive database picker and its existing promotion of
project-level render capabilities. Eventually, one Database capability plus
supported-driver metadata should replace the driver-named project flags. This
temporary exception does not justify persisting other active drivers in YAML.

### Environment

The project-root `.env` owns deployment state:

- active root and named-resource drivers;
- connection endpoints and credentials;
- `COMPOSE_PROFILES` for optional local services;
- other deployment-specific resource settings.

Wizard choices are initialization seeds. Once the project exists, a user's
environment values win over rerender defaults.

The safe, committed `.env.example` owns reproducible default and
`*_SUPPORTED_DRIVERS` build inputs for a clean checkout. It contains no secrets.
If the broader environment-contract generator has not landed by implementation
time, this wizard feature must deliver the minimal safe-contract behavior it
needs rather than relying on an uncommitted secret-bearing `.env`.

Full project rendering uses this deterministic precedence:

1. owner-controlled project-root `.env` values;
2. explicit new-project plan values for keys not yet owned by `.env`;
3. committed `.env.example` for missing generation/build defaults.

A direct `forj generate` invocation uses the same controlled project snapshot.
If GoForj adds an explicit command-scoped override above `.env`, that is a new
documented interface, not existing behavior: the current env loader overloads
process values with layered env files. Any such override must apply to that run
only and be passed explicitly to every selected generator; unrelated ambient
variables must not leak through cached global state.

`.env.example` is a fallback for generation, not a runtime deployment override.
Creating a local environment from it is explicit. Rerender updates the safe
contract only through the environment-contract rules and never copies secret
values into it.

The three current local files have distinct ownership:

- `.env` is the owner-controlled base for resource drivers, supported lists,
  endpoints, and Compose profile intent.
- `.env.local` is currently a framework-regenerated local runtime overlay. This
  design does not change that ownership and must not use it as the durable
  driver build contract.
- `.env.host` carries host-connectivity overrides for commands running outside
  Compose. It does not select built-in drivers or local service placement.

Compose reads the project-root `.env`; full-render service planning therefore
derives from that same snapshot. A runtime process-environment override can
still select an external service at deploy time, but static local Compose
planning cannot infer that deployment and must not mutate the project in
response to ambient shell state.

Resource generation must consume one immutable effective-value snapshot. The
command assembles the documented precedence once, after environment seeding,
and passes that source to cache, queue, events, storage, mail, database, and
service generators. Those generators must not independently reuse a cached
process-global `env.Load()` result. This keeps generated imports, validation,
Compose, and confirmation on the same values.

The build contract has three distinct representations:

- `*_SUPPORTED_DRIVERS` is the source-generation input;
- committed generated manager source and its driver manifest record what a
  clean checkout will build;
- the compiled manifest is the final truth inside an existing artifact.

Changing a supported list beside an already-built binary does not mutate that
artifact. The user must regenerate and rebuild. `*_DRIVER` remains runtime input
and must select a driver present in the compiled manifest.

### No persisted shape identity

Do not write `RESOURCE_SHAPE=standalone` or similar. The concrete values are the
truth after generation, and users are expected to change them. Persisting a
profile name would create ambiguous states such as a “Standalone” profile with
Redis active for two resources.

The UI can compute a display label from the effective active and supported
values when it needs one.

## Implementation Model

Create one canonical resource catalog in a package that both the wizard and
generators can consume. The top-level `project` package is a reasonable home;
`internal/forj/resources` already represents generated runtime resource
discovery and should not be overloaded with wizard policy.

A conceptual transient model is:

```go
type ResourcePlan struct {
	Shape           StartingResourceShape
	Selections      map[ResourceKey]DriverSelection
	NamedSelections map[string]string
}

type DriverSelection struct {
	Active    string
	Supported []string
}

type LocalServiceIntent struct {
	Modes map[ServiceKey]LocalServiceMode
}
```

The actual Go definitions must follow project comment conventions. The model is
not serialized to YAML or JSON.

Each catalog resource records:

- stable key, label, and description;
- applicability to project capabilities;
- ordered driver inventory;
- normal-shape active and supported mappings;
- named generated-resource policy;
- driver-specific service key and local-provisioning support;
- process scope and storage behavior;
- required environment placeholders;
- switching caveat.

Capability resolution also returns constraints such as **required resource**,
**allowed drivers**, and a user-facing reason. These constraints are consumed
by both the normal and Advanced editors; Advanced is not an escape hatch from a
starter or extra whose generated code cannot support a driver.

Derived functions resolve:

- a normal shape into a `ResourcePlan`;
- Advanced edits into a validated plan;
- generated named-resource requirements into effective selections;
- a plan and enabled components into `.env` and safe `.env.example` seeds;
- a plan, all App scopes, Docker capability, and explicit local-service intent
  into a deduplicated service plan;
- a plan into a normal or Custom summary label.

`ServicePlan` is derived rather than stored inside `ResourcePlan`, preventing
driver and service state from drifting. `LocalServiceIntent` is a separate
input because an active external Redis deployment and an active generated Redis
container have the same driver but different operational requirements. For
existing projects, the intent is reconstructed from owner-controlled concrete
configuration such as `COMPOSE_PROFILES`, not from the original shape.

The catalog must drive:

- wizard labels and descriptions;
- normal-preset resolution;
- Advanced driver lists;
- environment initialization;
- supported-driver validation and ordering;
- generator driver inclusion;
- Compose service/profile rendering;
- confirmation summaries;
- generated switching documentation.

The implementation should isolate this work from the existing large wizard
file, for example:

```text
project/resource_catalog.go
project/resource_plan.go
internal/forj/new_project_resources.go
internal/forj/new_project_resources_test.go
```

Exact filenames may change, but resource-policy logic must not accumulate as
scattered conditionals in `new_project_cmd.go` or templates.

## Validation Invariants

The resolver validates the complete plan before project configuration or files
are written:

1. Every active driver is non-empty and included in its supported list.
2. Every driver is known and applicable to the enabled capability.
3. Driver lists are normalized, deduplicated, and deterministically ordered.
4. Disabled capabilities contribute no driver or service requirements.
5. Support alone contributes build dependencies but no runtime initialization,
   health checks, migrations, or active local services; separate explicit
   service intent may still start an unused local service.
6. Effective generated and user-authored named resources participate in
   supported-driver validation and service discovery.
7. A generated named-resource driver cannot be removed from the root supported
   set while that named requirement applies.
8. Effective drivers across every generated App scope participate in service
   discovery.
9. Consumers deduplicate only when their service key and endpoint mapping are
   compatible.
10. Local versus external management is explicit; Docker changes available
    provisioning but does not itself select it or remove driver support.
11. Existing environment and unrelated Compose-profile tokens are not
    overwritten during rerender.
12. Database compatibility flags never select or overwrite the active database.
13. No active-local-service plan emits a Compose startup task with no selectable
    service.
14. Confirmation and rendered files consume the same validated plan.

Invalid Advanced combinations are rejected before rendering, so a bad plan
cannot leave a partially initialized project directory.

## Legacy `queue_driver` Migration

The durable `render.queue_driver` field must be removed without discarding the
active choice in existing projects.

The YAML loader may accept the legacy key for migration, but normal marshaling
must never write it. Migration is ordered as a small transaction:

1. Read the existing environment, any explicit initialization plan, and the
   legacy YAML value before rewriting either file.
2. Resolve active values using this precedence:
   1. existing environment value;
   2. explicit new-project resource plan;
   3. legacy `queue_driver`;
   4. compatibility fallback.
3. Fill only missing environment keys. When the supported list is also missing,
   seed it with at least the resolved active driver.
4. Write the environment to a same-directory temporary file, preserve the
   intended mode, flush and close it, then atomically rename it into place.
5. Only then atomically rewrite `.goforj.yml` without the legacy key.

An existing supported-driver list is preserved. If it excludes the effective
active driver, validation fails with an actionable error rather than silently
rewriting either value. Migration does not silently broaden an existing
project’s build contract to the new portable pair; that pair is the default for
new projects.

When Jobs is disabled, the legacy queue choice is non-applicable. Migration does
not create `QUEUE_*` environment keys merely to preserve dormant intent. It
reports that the obsolete value was ignored, then removes the YAML key after a
successful atomic YAML rewrite. If Jobs is enabled later, queue values initialize
from the then-current resource shape. The failure-safe “do not lose the only
copy” rule applies when Jobs is enabled and the value is operational.

If environment writing fails, the legacy YAML key remains so the source value
is recoverable. Atlas discovery, generated Lighthouse project configuration,
test-render helpers, and benchmarks must all stop treating the YAML field as
authoritative.

## Rendering Rules

The renderer receives a typed transient resource plan, not queue-specific loose
arguments and not new fields on `project.RenderConfig`.

For a new project:

1. resolve and validate the plan;
2. map every built-in database driver to current project compatibility flags;
3. initialize missing `.env` values and the safe `.env.example` contract from
   the plan;
4. construct one immutable effective environment snapshot;
5. generate managers from supported-driver values in that snapshot;
6. derive the service plan from the same active values, all App scopes, local
   capabilities, and explicit service intent;
7. render Compose and documentation from those derived values.

For an existing project rerender:

1. load owner-controlled environment values;
2. use migration-only sources for missing legacy values;
3. validate effective active/supported pairs and add missing database render
   capabilities without changing `DB_DRIVER`;
4. construct and pass one effective snapshot rather than relying on a stale
   process-global environment cache;
5. derive source and services from the effective environment and all App
   scopes;
6. never replay the original starting shape.

This preserves environment ownership while keeping generation deterministic.

## Implementation Sequence

Implementation mechanics across all phases below landed together on
2026-07-14. The sequence is retained to document dependency order and to make
later changes easier to review; the final release proofs are tracked
separately below.

### Phase 0: transient-state baseline

- Make `queue_driver` load-only legacy state.
- Fix migration ordering so environment values are written before YAML cleanup.
- Remove production reads of `RenderConfig.QueueDriver`.
- Move Atlas, Lighthouse's config mirror, render helpers, and benchmarks to
  environment-backed discovery.
- Remove the unreachable queue-only Runtime UI and stale confirmation row.

### Phase 1: pure resource catalog and plan

- Add resource, driver, shape, named-resource, and service metadata.
- Implement Standalone resources and Shared through Redis resolvers.
- Add active-subset, applicability, ordering, and deduplication validation.
- Map every built-in database choice to existing project capability flags while
  keeping the active database environment-owned.
- Add pure unit tests before connecting the wizard or templates.

### Phase 2: environment and generator contract

- Seed the paired cache, queue, and events supported-driver lists.
- Seed `log,smtp` for Mail when enabled.
- Omit queue values when Jobs is disabled.
- Establish the safe committed `.env.example` fallback required for clean
  checkout generation.
- Build and pass one controlled effective environment snapshot instead of
  letting generators independently read cached global environment state.
- Update cache, queue, events, mail, and environment validation generation to
  consume the canonical catalog or assert exact parity with it.
- Embed the compiled driver manifest and validate effective runtime active
  drivers against that manifest.
- Make one generated binary capable of changing between built-in local and
  Redis drivers using environment values; retain the end-to-end proof as a
  release gate.

### Phase 3: service planner and Compose bridge

- Replace `Components.Jobs` Redis gating with the derived service plan.
- Deduplicate Redis across root and named consumers.
- Add explicit local-versus-external Redis service intent.
- Render the optional Redis Compose profile for portable Docker projects.
- Seed `COMPOSE_PROFILES=redis` only when Redis is active locally.
- Preserve unrelated Compose profile tokens and omit empty service startup
  tasks.
- Align Redis authentication across cache, queue, and events before advertising
  external Shared through Redis compatibility.
- Distinguish App services, external requirements, and development tools.
- Update local-switching documentation.

### Phase 4: brainless wizard stage

- Add App Resources after Extras and before Atlas.
- Make Continue the initial focus.
- Add the two starting-resource choices and independent database row.
- Present Database as one capability in the component UI while retaining the
  current render flags internally.
- Reconcile Auth/OAuth database requirements immediately, preserve the last
  database choice across capability toggles, and enforce Demo's MySQL lock.
- Update forward/back transitions, progress, summaries, and confirmation.
- Keep resource UI and resolution logic out of the monolithic wizard file where
  practical.

### Phase 5: Advanced editor

- Add per-resource active and built-in driver editing.
- Group drivers by operational effect.
- Add viewport behavior and selection retention.
- Emit external/cloud configuration placeholders and warnings.
- Derive normal, custom-support, and Custom summary labels.

### Phase 6: generated guidance and integration coverage

- Update generated resource READMEs with the switching contract and caveats.
- Add the complete render matrix below.
- Exercise Compose profiles and service discovery.
- Verify no legacy or transient topology keys appear in project YAML.

Phases may land in separate commits, but phases 4 and 5 are one user-facing
release. The normal stage must not expose an **Advanced resource setup** action
until that editor works, and the wizard feature must not ship before the
portable environment and service-planning behavior it promises is working.

## Verification Strategy

All test renders must use directories under `/tmp`, never the GoForj repository
directory. Go build and test commands use the repository's required temporary
cache locations.

### Pure plan tests

- Standalone resources resolve exact active and supported values.
- Shared through Redis resolves exact active and supported values.
- Jobs disabled removes queue selection entirely.
- Active drivers are automatically included in supported lists.
- Duplicates normalize into stable catalog order.
- Exact preset mappings retain their labels.
- Support-only edits produce the custom-support label.
- Divergent shape-managed active edits produce Custom.
- Database changes preserve the shape label; Advanced Storage or Mail changes
  add a customization suffix.
- Database changes do not reset cache, queue, events, storage, or mail choices.
- Backtracking capability changes preserve applicable edits, drop disabled
  effective resources, and initialize newly applicable resources from the base
  shape.
- Generated named-resource requirements lock their driver into root support.
- Back and reset behavior preserve the documented state.

### Render matrix

- Standalone resources + SQLite
- Standalone resources + MySQL
- Standalone resources + Postgres
- Shared through Redis + SQLite, including the scope warning
- Shared through Redis + MySQL
- Shared through Redis + Postgres
- both shapes with Jobs disabled
- both shapes with Docker disabled
- both shapes with Mail disabled
- Auth enabled in both shapes, verifying the sessions policy
- Demo enabled in both shapes, verifying named-cache policy and the MySQL lock
- at least one Advanced custom plan with an external driver
- a project with a named App-scoped Redis override
- a clean checkout with no `.env`, regenerated from safe committed defaults
- `--allow-non-empty` with valid owner overrides and missing keys to initialize

For each render, assert exact active and supported environment values, generated
driver imports, project YAML ownership, confirmation output, and service plan.

### Compose tests

- Standalone resources include the optional Redis definition when Redis is
  built in but do not activate its profile.
- Workerpool Jobs do not start Redis.
- Redis cache without Jobs activates exactly one Redis service.
- Redis events without Jobs activates exactly one Redis service.
- Redis cache, queue, events, and sessions together still activate one service.
- Shared through Redis seeds the Redis profile.
- Docker-enabled active Redis with the profile absent is reported as external.
- Adding and removing `redis` preserves unrelated Compose profile tokens and
  their relative order.
- An owner-retained Redis profile with no active Redis consumer is preserved and
  reported as a locally requested unused service.
- A project with only inactive, non-activated profiled service definitions emits
  no empty Compose startup pretask.
- Docker-disabled Redis is reported as external and emits no Compose service.
- MySQL and Postgres services remain independent of the resource shape.
- Mailpit and observability tools remain separate from App service reporting.

### Migration tests

- An existing environment active driver wins over legacy YAML.
- A missing environment value is filled from legacy YAML.
- An existing supported list is preserved.
- An existing non-empty supported list that excludes the active driver fails
  without rewriting the environment or legacy YAML.
- A missing supported list includes the resolved active driver.
- Environment replacement is atomic, and YAML cleanup happens only after that
  successful replacement.
- A simulated environment-write failure retains the legacy YAML key.
- Jobs-disabled migration removes the obsolete YAML key without creating queue
  environment values.
- Saving generated Lighthouse configuration cannot recreate `queue_driver`.
- Rerender never restores the original shape over edited environment values.
- Non-empty-target reconciliation previews owner overrides before any write and
  blocks invalid active/supported pairs without mutation.

### Runtime behavior tests

- A generated App builds with both normal cache drivers.
- A generated Jobs App builds with both normal queue drivers.
- A generated App builds with both normal event drivers.
- Authenticated external Redis works through cache, queue, and events before the
  shared external path is considered complete.
- The same generated artifact starts once with process-local drivers and once
  with Redis drivers using environment changes only.
- Supported inactive Redis does not connect, register a health check, run
  lifecycle hooks, or fail when Redis is unavailable.
- A native fallback omitted from the compiled manifest cannot be activated only
  by changing runtime environment values.
- Changing `*_SUPPORTED_DRIVERS` beside an existing binary does not change its
  compiled manifest.
- Queue-switch documentation requires draining outstanding work.
- A controlled generation snapshot produces the same driver manifest regardless
  of stale process-global environment cache state.
- Rebuilding a clean checkout from `.env.example` reproduces the committed
  normal driver manifest.

Compose and driver integration tests that require network access should use the
normal repository elevation path rather than replacing the integration with a
mocked workaround.

## Acceptance Criteria

- The normal App Resources stage exposes only Starting resources and Database.
- Continue is initially focused and accepts visible defaults with one action.
- The default is Standalone resources with the existing default database.
- Standalone resources works with SQLite, MySQL, and Postgres.
- Shared through Redis uses Redis for root cache, enabled queues, events, and
  generated shared-session resources.
- Demo keeps its documented MySQL constraint until additional dialect support
  is explicitly completed.
- Storage remains local in both normal shapes.
- Normal renders build both sides of the cache, queue, and event transition.
- Active drivers always validate as built in.
- Supported inactive Redis with no explicit `redis` profile intent does not
  initialize or start Redis.
- Standalone resources with MySQL or Postgres starts no Redis service.
- Shared through Redis produces exactly one Redis service regardless of the
  number of Redis-backed consumers.
- A Docker-enabled standalone project can activate the Redis bridge later
  without rerendering Compose.
- Local Redis activation changes only the exact `redis` Compose-profile token;
  unrelated profiles survive.
- A retained Redis profile with no active consumer is preserved and reported as
  running by explicit local intent, not mistaken for driver activity.
- Docker-enabled active Redis can be marked external rather than being inferred
  local solely from Docker availability.
- A Docker-disabled project clearly reports external service requirements.
- Advanced mode edits the same resource plan as the normal presets.
- The normal stage and Advanced editor ship together.
- Existing environment values survive rerender.
- A clean checkout reproduces the supported-driver build contract without a
  secret-bearing local `.env`.
- Full rendering uses one controlled effective environment snapshot for
  generators, service planning, and confirmation.
- No new or rewritten project YAML contains `queue_driver`, another active
  driver, runtime mode, or a starting-shape key.
- Legacy queue-driver migration cannot lose the only copy of the user's value.
- Back navigation preserves normal and Advanced choices.
- Confirmation accurately separates active resources, App services, external
  requirements, and development tools.
- Non-empty targets are reconciled after Path, and confirmation shows the
  effective owner-overridden plan before any file is written.
- Generated documentation explains environment-only switching, rebuild
  requirements, and stateful migration caveats.

## Risks and Mitigations

### “Shared” is mistaken for fully distributed

Mitigation: use **Shared through Redis**, name cache/jobs/events explicitly, and
warn that SQLite and local storage remain filesystem-local.

### Portable builds increase dependency size

Mitigation: keep the normal bridge deliberately bounded to the selected pairs.
Database and storage alternatives remain opt-in, and Advanced can narrow
supported lists without adding a separate Lean concept to the normal path.
Record binary-size, clean-build-time, and module/dependency deltas for the
standalone-only baseline versus the portable normal build before release.

### Supported drivers accidentally initialize infrastructure

Mitigation: enforce the inactive-driver invariant in generator and lifecycle
tests, not only in documentation.

### Compose becomes stale after a later switch

Mitigation: render the optional Redis definition up front, activate it with a
standard Compose profile, and document service reconciliation when disabling
the profile.

### Wizard, templates, and generators drift

Mitigation: use one catalog and add exact parity tests for any generator-owned
inventory that cannot consume it directly.

### Existing projects lose their queue choice

Mitigation: make legacy loading one-way and failure-safe, with environment
writing committed before YAML cleanup.

### The wizard file becomes harder to maintain

Mitigation: isolate resource policy, view state, and tests in focused files and
keep the main wizard responsible only for stage orchestration.

## Implementation Revalidation

The implementation fact refresh was completed on 2026-07-14:

1. The current `forj new` stage order was verified and App Resources now sits
   after Extras and before Atlas. The obsolete queue-only Runtime question was
   removed rather than supplemented.
2. Legacy `queue_driver` is load-only migration input. Environment publication
   succeeds before YAML cleanup, and current config writers cannot recreate the
   key.
3. Cache, queue, events, storage, mail, and database inventories were refreshed
   from their generators. The canonical project catalog and generator parity
   tests now guard those inventories against drift.
4. Existing `.env` values retain ownership during rerender. Missing resource
   keys are initialized atomically, `.env.example` is generated through the
   secret-redaction contract, and direct generation runs from one controlled
   project environment snapshot.
5. Compose profile behavior was verified. Portable Docker renders include an
   inactive Redis profile, exact profile-token edits preserve unrelated tokens,
   and service startup tasks are derived from actual local requirements.
6. Redis cache, queue, and events retain the shared global endpoint fallback;
   Redis events now use a configured client so password and database settings
   follow the same connection contract.
7. Generated named resources, arbitrary named scopes, and App-prefixed overlays
   participate in accessor generation, compiled-driver validation, and
   endpoint-aware service planning. App-only and config-only named scopes gain
   accessors without promoting an App root overlay into a false resource name.
   Framework-owned public and Demo favicon disks retain explicit named storage
   requirements. Database compatibility aliases canonicalize before planning,
   different database engines cannot silently attach to the root Compose
   database, and external SMTP endpoints are not mistaken for Mailpit.
8. Runtime topology was revalidated as a command concern. `app run` remains the
   combined host, explicit leaf commands select split roles, and no
   `RUNTIME_MODE` environment abstraction is generated.
9. The complete Go test suite and `go vet ./...` passed with the required
   temporary Go caches. The ten-combination smoke render profile rendered,
   generated Wire code, and built every project under `/tmp`.

The following release gates remain open and do not change the implemented
product shape:

- measure binary size, clean-build time, and module/dependency deltas between a
  narrowed local-only build and the normal portable Redis bridge;
- run the network-backed authenticated external Redis exercise through cache,
  queue, and events;
- demonstrate the same built artifact starting once with process-local drivers
  and once with Redis drivers using environment changes only;
- run the broader release render profile in addition to the completed smoke
  profile.

If the work is picked up after sitting unreleased, rerun this section's tests
and release evidence against the then-current generators. That refresh should
not reopen the two-shape model unless a changed constraint makes the portable
bridge incorrect.

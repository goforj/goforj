# `forj new` Components and Resource Defaults Design

## Status

- Design status: accepted
- Research date: 2026-07-13
- Revised product model: 2026-07-14
- Scope: `forj new`, generated resource-driver defaults, build manifests,
  environment ownership, and local service planning
- Target repository: `goforj`

This revision supersedes the earlier proposal for a normal **App Resources**
stage. The resource planning and generation contracts remain useful, but the
wizard hierarchy was asking users to understand an internal model that GoForj
can resolve safely on their behalf.

## Decision Summary

`forj new` keeps component selection in one place.

- Database engines remain concrete, mutually exclusive choices in
  **Components**: MySQL, Postgres, and SQLite.
- There is no normal **App Resources** stage after Components.
- When selected, Cache, Jobs, and Events start on implementations that run
  inside the App.
- Their corresponding Redis implementations are included in the generated App
  by default, but Redis is inactive and does not start.
- When selected, File Storage remains local by default. Mail keeps its existing
  development defaults and includes the common log/SMTP pair.
- No App-wide resource shape, profile, or mode is introduced, and the wizard
  does not label an App Standalone, Shared, or Portable.
- Per-resource Advanced selection and a guided switch command are follow-up
  work, not another hierarchy in the new-project wizard.

The concise product promise is:

> Selected cache, jobs, and events start without Redis. Redis support is
> included so the same generated App can switch later.

This promise is intentionally scoped. A project may still require MySQL,
Postgres, mail delivery, object storage, or another service explicitly selected
by its components and deployment configuration. GoForj does not describe the
entire App as self-contained or infrastructure-free.

## Why This Is Simpler

The earlier design combined three different concepts in a second wizard stage:

1. what the App contains;
2. which implementation starts for each resource;
3. which alternative implementations are compiled into the artifact.

Only the first concept is a normal new-project decision. Components answers
that question, including the concrete database engine. GoForj can provide a
safe default for the other two:

- begin with the service-light implementations;
- include the common Redis alternatives;
- leave optional infrastructure inactive until deployment configuration asks
  for it.

This is the useful behavior users would not know to request. Presenting it as a
choice made the default look like an architecture commitment and made Database
appear to move out of Components into a second component system.

## Wizard Experience

### Components owns Database

Database implementations remain visible in the component list:

```text
Components

  ...
● Database (MySQL)     · Store app data in MySQL
○ Database (Postgres)  · Store app data in Postgres
○ Database (SQLite)    · Store app data in SQLite
  ...
```

The rows are one exclusive group. Selecting one clears the other database
engine. Clearing the selected database remains valid when no selected
capability requires persistence.

Auth and OAuth require a database but do not prescribe its engine. The Demo App
continues to require MySQL until its generated SQL and migrations support the
other engines. A temporary Demo constraint must not erase the user's prior
database choice when Demo is disabled again.

MySQL and Postgres identify database drivers, not placement. The wizard does
not invent `MySQLLocal`, `MySQLExternal`, `PostgresLocal`, or
`PostgresExternal` components:

- with Docker enabled, the selected engine receives its conventional local
  Compose service;
- without Docker, the same engine requires an externally managed connection;
- a production deployment can use that same generated driver with a managed
  endpoint without changing the component identity.

### No second resource stage

The normal route is:

```text
Project Name
  → Module
  → Components
  → Help
  → Starter Kit, when applicable
  → Extras
  → Atlas
  → Path
  → read-only target reconciliation
  → Confirm
```

There is no **Resources** progress item and no review screen that asks users to
choose between resource shapes. The obsolete queue-only Runtime question is
removed rather than replaced.

Resource defaults are derived after Components, Starter Kit, and Extras have
finalized the applicable capabilities. Back navigation recomputes missing
defaults without overwriting retained owner configuration.

### Confirmation

Confirmation remains component-focused rather than introducing a read-only
version of the resource hierarchy that was removed:

```text
Components          CLI · Docker · Database (MySQL) · Jobs · ...
Development tools   Mailpit · VictoriaMetrics · Grafana
```

The generated environment contract and resource documentation explain the
active defaults, included alternatives, and switching path after creation.
Project creation does not repeat those details as another decision or review
surface.

## Default Resource Contract

The following table is the normal new-project policy:

| Resource | Applies when | Active by default | Included in the App | Service effect at creation |
| --- | --- | --- | --- | --- |
| Database | A database component is selected | selected engine | selected engine | local MySQL/Postgres with Docker, external without Docker, none for SQLite |
| Cache | Cache is enabled | `memory` | `memory,redis` | Redis remains inactive |
| Queue | Jobs is enabled | `workerpool` | `workerpool,redis` | Redis remains inactive |
| Events | Events is enabled | `inproc` | `inproc,redis` | Redis remains inactive |
| Storage | File Storage is enabled | `local` | `local` | none |
| Mail | Mail and Docker are enabled | `smtp` | `log,smtp` | Mailpit is a development tool |
| Mail | Mail is enabled without Docker | `log` | `log,smtp` | none; external SMTP may be configured later |

When their corresponding components are enabled, the generated defaults are:

```env
CACHE_DRIVER=memory
CACHE_SUPPORTED_DRIVERS=memory,redis
QUEUE_DRIVER=workerpool
QUEUE_SUPPORTED_DRIVERS=workerpool,redis
EVENTS_DRIVER=inproc
EVENTS_SUPPORTED_DRIVERS=inproc,redis
```

Cache, Queue, Events, and Storage variables are omitted when their corresponding
components are disabled.

The Demo App remains a documented exception to the normal database build
contract. It starts with MySQL and includes SQLite when its generated fallback
requires it. That exception does not widen database support for ordinary Apps.

## Active Versus Included Drivers

`*_DRIVER` chooses the implementation used when the App starts.
`*_SUPPORTED_DRIVERS` chooses implementations generated and compiled into the
App.

The contract has four invariants:

1. Every active driver is included.
2. An included but inactive driver does not connect, register health checks,
   run migrations, initialize clients, or start lifecycle hooks.
3. Switching to an included driver requires deployment configuration and an
   App restart or redeploy, but not application-code changes or a rebuild.
4. Adding a driver that was not included requires updating the build contract,
   regenerating source, and building a new artifact.

Generated managers embed a compiled-driver manifest. Runtime validation uses
that manifest as the authority inside an existing artifact. Changing
`*_SUPPORTED_DRIVERS` beside an already-built binary cannot make omitted code
appear; startup fails with an actionable message naming the active driver, the
compiled choices, and the appropriate generator.

Generated documentation uses **active** and **included** consistently. It never
describes an included driver as running.

## Switching Is Operationally Explicit

Including both implementations removes a code-generation and build step. It
does not move state or make every change lossless:

- changing cache drivers normally begins with a cold cache;
- changing queue drivers requires producers and workers to switch coherently,
  and outstanding work must be drained or migrated;
- event drivers may differ in delivery, ordering, and durability semantics;
- changing databases requires schema and data migration, and may require
  Compose changes even when the target driver is compiled in;
- changing storage requires object migration and URL or visibility review;
- changing mail requires credentials and may change retry behavior.

Generated resource documentation records these caveats. A future guided switch
command should present the same checks before changing configuration.

## Named Resources and App Scopes

Root settings do not automatically govern every generated named resource.
Values such as `CACHE_SESSIONS_DRIVER` can override `CACHE_DRIVER`, so generated
requirements remain explicit.

The initial local-default policy includes:

| Named resource | Active by default | Reason |
| --- | --- | --- |
| Auth sessions | `memory` | new Apps must not require Redis to start |
| Demo settings cache | `memory` | it is a database-backed optimization |
| Generated queues | inherit root | queue generation already follows the root driver |
| Generated event buses | `inproc` | existing named-event behavior remains explicit |
| Named storage disks | `local` | storage is outside the Redis-ready default |

Every generated named active driver is included in the corresponding root
manifest. User-authored named resources and App-prefixed overrides remain
owner-controlled during rerender. They participate in compiled-driver
validation and service discovery without being rewritten from a remembered
wizard choice.

Consumers share a local service only when their service identity and endpoint
mapping are compatible. App-scoped external endpoints remain separate
requirements rather than being silently redirected to a generated local
container.

## Environment and Configuration Ownership

### Project YAML

`.goforj.yml` owns durable generation capabilities and policy, including
components, starter kits, render dependencies, and framework version. It does
not own deployment-time active drivers.

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

The existing database component flags remain transitional render-capability
flags. `DB_DRIVER` selects the active engine, while `DB_SUPPORTED_DRIVERS`
defines the generated build contract. Compatibility flags must never overwrite
owner environment values or start inactive database services.

### Environment files

The project-root `.env` owns deployment state:

- active root, named, and App-scoped drivers;
- service endpoints and credentials;
- `COMPOSE_PROFILES` for optional local services;
- other deployment-specific resource settings.

The committed `.env.example` owns safe, reproducible generation defaults and
supported-driver inputs for a clean checkout. It contains no secrets.

`.env.local` remains a framework-regenerated local runtime overlay.
`.env.host` carries host-connectivity overrides for commands running outside
Compose. Neither file owns the driver build contract.

Rendering uses one immutable effective snapshot with this precedence:

1. owner-controlled project-root `.env` values;
2. explicit new-project values for keys not yet owned by `.env`;
3. committed `.env.example` values for missing generation defaults.

Cache, queue, events, storage, mail, database, Compose, validation, and
generated documentation consume the same snapshot. The normal wizard does not
turn existing owner values into another preview or decision screen; it applies
them during read-only target preparation and reports only conflicts that block
a safe render. Unrelated ambient process variables must not widen a generated
artifact through stale global environment state.

For an existing target, `forj new --allow-non-empty` reconciles environment
state after Path is selected and before confirmation or any write. Existing
concrete values win. Invalid active/included pairs stop generation without
rewriting owner files.

## Service Planning

Service planning is derived from effective drivers, every generated App scope,
Docker capability, endpoint affinity, and explicit owner service intent. It is
not derived from Jobs, a remembered wizard profile, or a broad component name.

A service requirement has one of four states:

- **active local**: GoForj starts a generated local service;
- **available local**: an optional local definition exists for a later switch
  but is not started;
- **local requested, unused**: owner configuration retains a local service even
  though no active resource currently consumes it;
- **external required**: an active driver requires infrastructure GoForj is not
  managing locally.

Cache, queue, events, and compatible named consumers use one global Redis
connection contract by default. Compatible consumers produce one Redis service
requirement. Resource-specific or App-specific endpoints remain separate.

MySQL and Postgres keep their existing Docker-component policy. Only the active
selected engine receives the conventional local service. SQLite creates no
service requirement.

## Inactive Redis Compose Bridge

For Docker-enabled projects, including Redis support renders one Redis service
behind the exact Compose profile `redis`:

```yaml
services:
  redis:
    profiles: [redis]
    image: redis:7.4
```

The default environment does not activate that profile. Ordinary
`docker-compose up -d` therefore does not start Redis.

When a deployment later activates Redis-backed drivers, it must either:

- provide an external Redis endpoint; or
- add the exact `redis` token to `COMPOSE_PROFILES` and reconcile local Compose
  services.

GoForj edits profile values token by token. Unrelated profiles retain their
order, and support alone never inserts or activates `redis`. An existing exact
token is owner intent and survives rerender even if Redis becomes inactive;
confirmation may report the resulting unused local service rather than remove
it silently.

A Compose file containing only inactive profiled services does not create a
Compose startup task. Redis volumes may remain declared while the profile is
inactive so switching remains reversible.

Docker controls local service emission, not driver availability. Without
Docker, an active Redis driver is an external requirement while inactive Redis
support remains a valid compiled choice.

## Runtime Topology Is Separate

Driver defaults do not select how many processes run:

- `app run` remains the combined App host;
- explicit API, worker, jobs, or scheduler commands run leaf roles.

No `RUNTIME_MODE` environment variable or durable App mode is generated.
Process topology is expressed by the command a deployment launches.

## Legacy `queue_driver` Migration

The obsolete `render.queue_driver` field is accepted only as migration input.
Normal project YAML never writes it.

Migration follows this precedence for an enabled Jobs capability:

1. existing `QUEUE_DRIVER` environment value;
2. an explicit initialization plan;
3. legacy `queue_driver`;
4. the workerpool compatibility fallback.

The migration fills only missing environment keys. It preserves an existing
supported list and rejects an active driver excluded by that list. It does not
silently widen an existing project's build contract to the new Redis-ready
default.

Environment publication completes atomically before `.goforj.yml` is rewritten
without the legacy key. If the environment write fails, the YAML value remains
recoverable. When Jobs is disabled, migration does not create dormant
`QUEUE_*` keys merely to preserve obsolete intent.

## Advanced and Guided Switching Follow-Up

This design does not add a per-resource Advanced editor to `forj new`. The
normal wizard should not teach the complete driver catalog before creating an
App.

The generated environment contract and compiled manifest make manual switching
possible today. Follow-up work may add commands such as an interactive driver
configuration or switch flow. That work should:

- distinguish the active driver from drivers included in the artifact;
- show service and credential requirements before applying a change;
- reconcile the exact Redis Compose-profile token when local Redis is chosen;
- explain queue draining, cache cold starts, event semantics, and data
  migration;
- regenerate and rebuild only when the requested driver is absent from the
  compiled manifest;
- preserve owner-controlled named-resource and App-scoped overrides.

If a future Advanced surface is added, it edits the same typed resource plan
and service plan used by generation. It must not introduce a second persisted
configuration model or return a broad topology mode to the wizard.

## Implementation Model

A typed transient resource plan remains the source of truth for rendering:

```go
type ResourcePlan struct {
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

No resource-shape field is needed in the new-project model or persisted project
configuration. The concrete plan and service intent carry the information the
renderer needs.

The canonical resource catalog drives:

- applicability to selected components;
- ordered driver inventories;
- default active and included mappings;
- named generated-resource requirements;
- environment placeholders;
- compiled-driver validation;
- service identity and local provisioning;
- Compose rendering;
- generated switching documentation.

`ServicePlan` is derived rather than stored inside `ResourcePlan`.
`LocalServiceIntent` remains separate because the same active Redis driver may
use a generated local container or an external endpoint.

## Validation Invariants

Before any project file is written:

1. Every active driver is known, applicable, and included.
2. Included-driver lists are non-empty, deduplicated, and deterministically
   ordered.
3. Disabled capabilities contribute no driver or service requirement.
4. Included inactive drivers contribute code but no runtime initialization or
   active service.
5. Generated and owner-authored named resources participate in manifest
   validation and service discovery.
6. Consumers deduplicate only when service identity and endpoint affinity are
   compatible.
7. Local versus external management remains explicit.
8. Existing environment values and unrelated Compose-profile tokens survive
   rerender.
9. Database compatibility flags never select or overwrite the active database.
10. Rendered source, environment, Compose, and documentation use the same
    effective snapshot.

## Acceptance Criteria

- MySQL, Postgres, and SQLite remain visible exclusive choices in Components.
- The normal wizard contains no App Resources stage or Resources progress item.
- The queue-only Runtime question remains removed.
- Selected Cache, Jobs, and Events components activate memory cache, workerpool
  jobs, and in-process events.
- Selected Cache, Jobs, and Events components include the corresponding Redis
  drivers by default.
- Included inactive Redis does not initialize a client or start a service.
- Docker projects render one inactive profiled Redis definition that can be
  activated later without rerendering Compose.
- Ordinary MySQL and Postgres projects include only their selected database
  driver; changing database engines remains an explicit rebuild and migration
  concern unless support was deliberately widened.
- Selected File Storage remains local by default, and Mail retains its
  documented log/SMTP policy.
- Existing `.env` values remain authoritative during rerender.
- A clean checkout reproduces the supported-driver build contract from the
  committed safe environment contract.
- No project YAML persists active drivers, a resource shape, or runtime mode.
- Legacy `queue_driver` migration cannot lose the only operational copy of its
  value.
- Confirmation remains component-focused and introduces no secondary resource
  inventory.
- Advanced driver selection and guided switching remain explicit follow-up
  work.

## Verification and Release Evidence

The implementation should retain coverage for:

- exact local active and Redis-included default plans;
- active-driver membership in compiled manifests;
- inactive-driver lazy initialization;
- Database and Redis independence across MySQL, Postgres, and SQLite;
- Docker and Docker-disabled service planning;
- exact `COMPOSE_PROFILES` token preservation;
- existing environment reconciliation and atomic publication;
- legacy `queue_driver` migration failures;
- generated named resources and App-prefixed overrides;
- smoke renders and clean-checkout generation under `/tmp`.

Before release, record:

- binary-size, clean-build-time, and dependency deltas for the Redis-ready
  default versus a narrowed local-only build;
- an authenticated external Redis exercise across cache, queue, and events;
- one built artifact starting first with local drivers and then Redis drivers
  using environment changes only;
- the broader render profile in addition to smoke coverage.

# feat: make generated resource drivers ready to switch

## Summary

This PR lets new GoForj Apps start with simple local resource drivers while already containing the common Redis alternatives needed later.

The wizard stays Components-first. MySQL, Postgres, and SQLite remain concrete choices in Components, and there is no second App Resources stage that asks users to classify their App or learn every driver before generating it.

Cache starts in memory, Jobs uses workerpool when enabled, and events stay in-process. Redis support is generated and compiled alongside those defaults, but it remains inactive and no Redis service starts.

That gives a new App a low-service starting point without closing off the common move to Redis. Switching to an included driver changes deployment configuration and requires a restart or redeploy, not application-code changes or a rebuild.

## What users see

The queue-only Runtime question is removed and is not replaced by another normal wizard stage.

Database engines stay where users expect them: in Components. They remain one mutually exclusive group, so a project selects MySQL, Postgres, SQLite, or no database when no enabled capability requires one.

The normal wizard does not expose an App-wide resource shape, profile, or mode, and it does not label the App Standalone, Shared, or Portable. Those terms would describe internal planning rather than a durable App identity.

Per-resource Advanced selection and a guided switch command are intentionally left for follow-up work. The generated environment contract and documentation provide the underlying switching path without making project creation exhaustive.

## Default generated contract

| Resource | Active by default | Included in the App |
| --- | --- | --- |
| Cache | Memory | Memory and Redis |
| Queue, when Jobs is enabled | Workerpool | Workerpool and Redis |
| Events | In-process | In-process and Redis |
| Storage | Local | Local |
| Mail with Docker | SMTP through Mailpit | Log and SMTP |
| Mail without Docker | Log | Log and SMTP |
| Database | Selected Components engine | Selected Components engine |

The generated environment makes the distinction explicit:

```dotenv
CACHE_DRIVER=memory
CACHE_SUPPORTED_DRIVERS=memory,redis
QUEUE_DRIVER=workerpool
QUEUE_SUPPORTED_DRIVERS=workerpool,redis
EVENTS_DRIVER=inproc
EVENTS_SUPPORTED_DRIVERS=inproc,redis
```

`*_DRIVER` selects what starts. `*_SUPPORTED_DRIVERS` selects what generation builds into the App.

## Compiled driver manifests

Generated cache, queue, events, storage, mail, and database managers now embed an authoritative compiled-driver manifest.

The runtime validates every effective root, named, and App-scoped active driver against that manifest. Changing `*_SUPPORTED_DRIVERS` beside an existing binary cannot make omitted code appear, and startup fails with an actionable error when configuration selects a driver that was not built in.

Selecting a driver already in the manifest does not require regeneration or rebuilding. Adding a driver absent from the manifest still requires updating the supported list, running the relevant generator, and building a new artifact.

Inactive included drivers remain lazy. They do not initialize clients, connect to infrastructure, register health checks, run migrations, or start lifecycle hooks.

## Database remains a Component

MySQL, Postgres, and SQLite remain concrete mutually exclusive Component choices rather than moving into a secondary resource hierarchy.

MySQL and Postgres identify database implementations, not placement. A Docker project receives the conventional local Compose service for its selected engine; without Docker, the same driver requires an external connection. Production can use that same artifact with a managed endpoint without inventing `MySQLLocal` or `PostgresExternal` components.

Auth and OAuth require a database but do not prescribe its implementation. Demo temporarily requires MySQL and retains SQLite as its generated fallback until its complete dialect coverage changes.

Ordinary projects build only the selected database driver. Changing database engines can require regeneration or Compose changes as well as explicit schema and data migration; this PR does not imply that database state moves automatically.

## Redis is included but inactive

For Docker projects, the generated Compose file contains one Redis service behind the `redis` profile whenever Redis is included by a relevant resource.

```yaml
services:
  redis:
    profiles: [redis]
    image: redis:7.4
```

The default project environment does not activate that profile, so ordinary `docker-compose up -d` does not start Redis.

When a deployment later switches active drivers to Redis, it can provide an external endpoint or add the exact `redis` token to `COMPOSE_PROFILES` for the generated local service. Profile updates preserve unrelated tokens, and rerender does not remove an owner-retained Redis profile merely because no active resource currently consumes it.

Cache, queue, events, sessions, and other compatible consumers share the generated global Redis connection contract. Service planning deduplicates them only when their service identity and endpoint affinity match; App-scoped external endpoints remain separate requirements.

## Service planning

Resource and development services are derived from effective drivers rather than broad component guesses.

A service may be active and managed locally, available locally for a later switch, explicitly retained locally while unused, or required from an external provider.

Redis support alone produces only an available inactive service definition. The selected MySQL or Postgres engine remains an App service when locally managed, while Mailpit, VictoriaMetrics, and Grafana remain development tools. SQLite, memory cache, workerpool jobs, in-process events, and local storage create no service requirement.

Compose startup tasks are emitted only when an active local service or selected development tool needs them. A Compose file containing only inactive profiles does not cause an empty `docker-compose up` failure.

## Environment ownership and reproducible generation

Resource-driver choices are deployment and build configuration. They are not persisted as a resource shape, runtime mode, or active-driver fields in `.goforj.yml`.

The project-root `.env` remains owner-controlled and wins during rerender. Missing values are initialized without replacing existing choices, invalid active/included combinations stop before rendering, and environment publication is atomic.

The committed `.env.example` carries safe clean-checkout defaults and supported-driver build inputs without secrets. Direct generation uses one controlled effective environment snapshot so unrelated ambient process variables cannot widen the generated artifact.

Cache, queue, events, storage, mail, database, Compose, and generated documentation consume that same snapshot.

## Named resources and named Apps

Driver discovery covers root resources, framework-generated named resources, owner-defined named resources, and App-prefixed root and named overrides.

Generated named active drivers are locked into the applicable compiled manifest. App-only named resources receive accessors, and App root overrides do not create false named accessors.

The same effective scopes feed service planning. Consumers sharing one endpoint deduplicate correctly, while different external endpoints remain separate. A named database using a different engine cannot silently attach to the root Compose database.

## Stateful switching remains explicit

Including a driver removes the regeneration and rebuild step; it does not migrate state.

- Cache switches normally begin cold unless data is moved separately.
- Queue switches require coherent producer changes and draining or migrating outstanding jobs.
- Event drivers may have different delivery, ordering, and durability semantics.
- Database switches require schema and data migration.
- Storage switches require object migration and URL or visibility review.
- Mail switches require credentials and may change delivery and retry behavior.

Generated resource documentation records these operational caveats.

## Legacy `queue_driver` migration

The obsolete `.goforj.yml` `queue_driver` setting is accepted only as migration input and is never written by current project configuration.

Existing environment values remain authoritative. When Jobs is enabled and `QUEUE_DRIVER` is missing, migration can seed it from the legacy value, publish the environment atomically, and only then rewrite YAML without the obsolete key.

If the environment write fails, the legacy YAML value remains recoverable. Existing supported-driver lists are preserved, invalid active/included pairs fail visibly, and migration does not silently widen an existing project's build contract to the new Redis-ready default.

## Runtime topology is separate

This PR removes `RUNTIME_MODE` from generated runtime behavior.

`app run` remains the combined App host, while explicit API, worker, jobs, and scheduler commands select leaf roles. Resource drivers do not choose process layout and no runtime mode is persisted.

## Compatibility and behavioral changes

- New Apps activate local cache, queue, and event drivers while including Redis alternatives.
- The new-project wizard contains no App Resources stage and keeps concrete database engines in Components.
- The old queue-only Runtime question remains removed.
- Existing environment values survive rerender.
- Newly rendered projects do not persist active resource drivers or runtime mode in project YAML.
- Legacy `queue_driver` values migrate to environment state before YAML cleanup.
- Docker projects that include Redis render an inactive profiled Redis service.
- Supported inactive drivers increase generated dependencies and may increase binary size, but Advanced narrowing is not added to the normal wizard in this PR.
- This PR does not migrate database rows, cache contents, queued jobs, event history, stored objects, or running services.

## Review guide

1. `project/resource_catalog.go`, `project/resource_plan.go`, and `project/service_plan.go` define the resource, manifest, and service-planning contracts.
2. `internal/forj/new_project_cmd.go` keeps Components and the normal wizard route coherent.
3. `internal/forj/project_resources.go`, `internal/forj/project_service_consumers.go`, and `internal/forj/new_project_resource_reconcile.go` connect owner environment state to rendering and service planning.
4. `internal/generate/{caches,queues,events,storages,mail,db}.go` emit and enforce compiled driver manifests.
5. Environment, Compose, runtime, Wire, and resource README templates expose the generated contract.

The full rationale is documented in `docs/designs/forj-new-resource-topology-wizard-design.md`.

## Validation

The following validation passes with Go caches outside the repository:

```text
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./... -count=1
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
```

The smoke render profile renders, generates Wire code, and builds all ten component combinations under `/tmp`:

```text
PATH=/tmp/gobin:$PATH GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache FORJ_TEST_RENDERS_DIR=/tmp/forj-resource-defaults-renders FORJ_TEST_RENDERS_WORKERS=4 go run ./cmd/forj test:renders --profile=smoke
```

## Follow-up release evidence

- Measure binary size, clean-build time, and dependency deltas between a narrowed local-only build and the Redis-ready default.
- Run the authenticated external Redis exercise through cache, queue, and events.
- Demonstrate one built artifact starting with local drivers and then Redis drivers using environment changes only.
- Run the broader release render profile.
- Design a guided driver-switch command only after the generated contract and operational checks are proven.

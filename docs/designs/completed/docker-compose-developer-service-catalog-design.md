# Docker Compose Developer-Service Catalog Design

## Status

- Design decision: complete for the initial built-in catalog.
- Implemented scope: always-rendered Redis, RustFS, and OpenSearch Compose profiles; exact profile
  activation; Redis and S3 service-plan integration; invocation-time development lifecycle support;
  profile-aware test filtering; and generated developer documentation.
- Future scope: richer catalog recipes and endpoint metadata, resource-registry links, helper commands,
  a first-party Search resource, and broader live integration coverage.
- Target repository: `goforj`.

This document describes the behavior shipped by the initial catalog slice. The final section separates
possible catalog expansion from the completed acceptance criteria.

## Decision

Every Docker-enabled GoForj project receives the same built-in set of optional developer services in
its generated `docker-compose.yml`. Each service is inactive behind an exact Docker Compose profile.

Catalog availability is not another project choice. GoForj does not persist a developer-service list
in `.goforj.yml`, infer catalog inclusion from selected components, or require a rerender before an
already-rendered service can be enabled.

`COMPOSE_PROFILES` is the activation state:

```env
COMPOSE_PROFILES=
```

A developer can enable one or more services later by changing that owner-managed value:

```env
COMPOSE_PROFILES=redis,rustfs
```

The next direct Compose startup observes the new value without changing `.goforj.yml`, regenerating
Compose, or rebuilding an App. Projects with a normal `forj dev` watcher session also pick it up in
their pre-development lifecycle.

The contract separates three concerns:

```text
Built-in Compose catalog  decides which optional local services are available
COMPOSE_PROFILES          decides which catalog services start on this machine
App resource config      decides whether and how an App consumes a service
```

Starting a profile does not silently add an App driver, change the selected driver, create a bucket or
index, or redirect an explicitly external endpoint.

## Why The Catalog Is Separate From App Resources

Some developer services provide infrastructure used by App resources:

- Redis can back Cache, Queue, Events, or Storage.
- RustFS can provide an S3-compatible endpoint for Storage.

Other services are useful before GoForj has a matching App primitive. OpenSearch supports manual
development and extension work even though this slice does not introduce a first-party Search
resource.

The Compose catalog therefore cannot be derived only from `ResourceCatalog`, supported-driver lists,
or the current component set. Those inputs describe App capabilities, not every useful local tool.

The current canonical catalog metadata records a stable key, label, profile, ordering, and an optional
service requirement supplied by the entry. It connects Redis and RustFS to service planning without
controlling whether their Compose definitions are rendered. Endpoint metadata and resource discovery
are not yet part of this catalog definition.

## Shipped Catalog

The initial catalog establishes these stable profile identities:

| Profile | Generated services | Role | Current App integration |
| --- | --- | --- | --- |
| `redis` | Redis 7.4 | Resource provider | Existing shared Redis service contract |
| `rustfs` | RustFS `1.0.0-beta.10` | Resource provider and local admin tool | S3-compatible Storage service |
| `opensearch` | OpenSearch 3.7.0 and OpenSearch Dashboards 3.7.0 | Standalone service and local admin tool | None |

The profile is the user-facing lifecycle unit. One profile can contain multiple Compose services when
the containers form one useful stack, as OpenSearch and Dashboards do.

All three entries are rendered for every Docker project. Their Compose definitions use reviewed
version tags, named volumes where data should survive ordinary restarts, explicit published-port settings,
and the generated backend network. RustFS and OpenSearch include service health checks; Dashboards
waits for the OpenSearch service to become healthy.

Existing required services such as a selected local MySQL or Postgres database retain their current
component and service-plan behavior. This work does not move all existing Compose services behind
profiles.

## Activation Contract

Profile matching is exact and comma-delimited. `redis-debug` does not activate `redis`, and
`rustfs-tools` does not activate `rustfs`. Combinations activate the union of the exact profiles.

An absent or empty `COMPOSE_PROFILES` value leaves all optional catalog entries inactive. Services
without profiles continue to follow their existing Compose behavior.

Unknown profile tokens remain owner intent. GoForj preserves them so an owner-managed Compose
override can define additional profiled services. Built-in catalog profile names are a generated
configuration compatibility contract and must not be repurposed.

Docker Compose can also start an explicitly targeted profiled service. GoForj's normal development
lifecycle uses the effective `COMPOSE_PROFILES` value instead.

For a newly rendered project, GoForj may seed `COMPOSE_PROFILES` from an active local Redis or S3
service requirement. OpenSearch has no App provider binding and is never enabled from App resource
selection. After generation, the `.env` assignment is owner-managed and survives reconciliation.

## Generated Ownership

GoForj owns the generated `docker-compose.yml` and may refresh built-in catalog definitions during a
render. Project-specific Compose customization belongs in the normal owner-managed override surface,
not in edits to the generated catalog.

The ownership rules are:

- framework catalog definitions and image pins are generated;
- `.env` owns profile activation, ports, bootstrap credentials, and local connection values;
- unrelated profile tokens and existing owner environment values survive reconciliation;
- named-volume declarations are generated, while their stored data remains user state;
- disabling a profile, rerendering, and ordinary shutdown do not delete named volumes; and
- there is no `dev.services` or equivalent list in `.goforj.yml`.

Generated `containers/goforj-development-services.md` documents available profiles, endpoints,
credentials, resource binding examples, host-versus-container addresses, and data ownership. Its
GoForj-specific name and generated marker establish framework ownership without claiming an existing
generic `containers/README.md`. A catalog-list command is not part of the completed slice.

An existing project needs one render to receive a catalog introduced by a newer GoForj release. Once
the service exists in its Compose file, later activation changes require only `COMPOSE_PROFILES`.

## Resource Provider Integration

The current catalog can associate an entry with one existing service requirement:

- `redis` provides the shared Redis service requirement;
- `rustfs` provides the S3 storage service requirement; and
- `opensearch` has no provider association.

S3 is locally provisionable in the service catalog. A Docker project's S3 consumer is classified as
local only when its endpoint is the exact generated container address `http://rustfs:9000` (with an
optional trailing slash). An empty endpoint remains the SDK's external provider default, and
`http://rustfs` without port `9000` is not captured as local.

Endpoint affinity remains strict:

- an explicitly external endpoint remains external;
- a neighboring name such as `rustfs-tools` is not treated as the generated provider;
- Redis and S3 become local intent only through their exact provider profiles;
- compatible consumers share an existing provider only under the existing service-plan rules; and
- activating `rustfs` does not add `s3` to supported drivers, select it, create a bucket, or overwrite
  owner credentials.

The profile owns local service lifecycle. App configuration still owns whether the App consumes the
service.

## Development Lifecycle

The App resource service plan cannot be the only trigger for Compose startup because a standalone
`opensearch` profile has no App consumer.

At each invocation, `forj dev` uses the process `COMPOSE_PROFILES` value when it is present and falls
back to the project's `.env` value otherwise. For Docker projects, it adds the conventional Compose
startup task when at least one profile token is present and no task with that conventional name is
already configured. This allows a newly enabled built-in or owner-defined profile to start without
rerendering while preserving an owner's custom conventional task. Compose itself validates unknown
tokens and reports `no service selected` when a nonempty selection matches no service.

Existing generated startup tasks continue to cover unprofiled required services. A Docker project
with only inactive catalog entries does not add a profile-only Compose startup task, and a stale
generated task is suppressed after its last active profile is removed when neither the base Compose
file nor its conventional override contains an unprofiled service. Owner-customized conventional
tasks are preserved. A project without dev watches or configured dev Apps still exits before
pre-development tasks, so profile-only and CLI-only projects use direct Compose startup.

`forj dev` teardown and `forj down` ensure the conventional
`docker-compose --profile "*" down` task is present for Docker projects even when generation did not
persist one. Existing generated `docker-compose down` tasks are normalized to that command, while
owner-customized conventional tasks are preserved. Selecting all profiles ensures a service is still
included after its activation token is removed. Ordinary teardown does not pass `-v`, so it does not
request deletion of named volumes.

Removing a token changes later Compose selection; it does not stop a container that was already
created. Developers run `forj down` or the all-profile Compose teardown when disabling an active
profile before starting the new selection.

## Profile-Aware Test Rendering

The rendered-project test harness filters Compose services before allocating host ports or starting
testcontainers:

- services without profiles remain active;
- a profiled service is active when at least one of its declared profiles is an exact enabled token;
- inactive Redis, RustFS, OpenSearch, and Dashboards services are skipped; and
- unknown and neighboring tokens do not accidentally activate a built-in service.

This prevents the always-baked catalog from increasing ordinary render-test container counts or
network requirements. An active RustFS dependency receives an isolated API port and host-side
`STORAGE_ENDPOINT`. OpenSearch and Dashboards carry an explicit `x-forj-test: false` marker because the
generic App dependency harness does not reproduce their Compose network and dependency semantics;
they remain assigned to a future dedicated live lane.

## Security And Operational Defaults

An always-rendered catalog must be inert while disabled and explicit about local-development security
when enabled.

- RustFS, OpenSearch, and Dashboards bind their published ports to
  `DEV_SERVICE_IP_ADDRESS=127.0.0.1` by default.
- Redis retains its existing `IP_ADDRESS` bind contract for compatibility; projects that leave
  `IP_ADDRESS=0.0.0.0` should treat unauthenticated Redis as reachable on all host interfaces. Generated
  guidance tells developers to select loopback before enabling it on an untrusted network or supply an
  authenticated owner override and matching App configuration.
- RustFS authentication remains enabled with overridable development credentials.
- OpenSearch uses a bounded single-node configuration, an explicit heap limit, TLS, and an overridable
  initial administrator password. That value bootstraps a fresh persisted security index and does not
  rotate an existing password; generated guidance covers deliberate rotation and destructive local reset.
- The generated documentation distinguishes container endpoints from host endpoints and identifies
  the credentials as local-development defaults, including dotenv quoting for `$`, `#`, and spaces.
- The shipped recipes do not require privileged mode, host networking, a Docker socket mount, or a
  broad host-directory mount.

The catalog is a development convenience, not production orchestration. Production credentials, TLS
policy, clustering, backups, migration, and availability remain deployment responsibilities.

## Compatibility

The existing exact `redis` profile and resource behavior remain valid.

Adding an inactive entry changes generated Compose output but does not activate its containers, change
App source or API, alter the resource build contract, or raise the minimum Go version.

Compatibility risks are classified separately:

- **Generated configuration:** Docker projects gain inactive services, volumes, environment defaults,
  documentation, and stable profile names.
- **Runtime behavior:** enabling a profile starts its pinned local stack and exposes the documented
  ports.
- **Persisted data:** named-volume contents survive disablement and ordinary teardown.
- **Operational migration:** a future image or data-format change may require release notes and a
  service-specific migration path.
- **Owner configuration:** unrelated profile tokens and existing owner environment values remain
  intact during reconciliation.

## Non-Goals Of The Completed Slice

- Do not add a developer-service selection stage to `forj new`.
- Do not persist selected or available catalog entries in `.goforj.yml`.
- Do not activate every catalog profile by default.
- Do not use components or supported resource drivers as the catalog inclusion gate.
- Do not silently add App drivers, dependencies, resource instances, buckets, indexes, credentials,
  or endpoint overrides.
- Do not introduce a first-party Search resource for OpenSearch.
- Do not redirect external or opaque resource endpoints to local containers.
- Do not add active service links to the resource registry in this slice.
- Do not accept arbitrary Compose fragments, images, or install scripts in project YAML. Owners use
  the standard Compose override surface.
- Do not delete developer-service data during profile disablement, rerender, or ordinary shutdown.
- Do not present these Compose recipes as production deployment manifests.

## Completed Verification

The implementation has focused coverage for:

- stable catalog metadata, ordering, defensive copies, and exact lookups;
- exact and combined profile parsing, including neighboring and unknown tokens;
- Redis and RustFS local-service intent reconciliation;
- generated Compose and `.env` output for the built-in catalog;
- RustFS endpoint classification without redirecting external S3 endpoints;
- Redis and S3 service-plan states;
- invocation-time `forj dev` startup task selection, stale-task suppression, Compose override
  eligibility, and process-environment precedence;
- Docker project all-profile teardown availability and generated-command normalization; and
- profile-aware rendered-project test filtering before port allocation and container startup,
  including RustFS host projection and explicit standalone OpenSearch exclusion.

These unit and render tests verify the completed contract. They do not claim a live Docker integration
matrix for every catalog entry.

## Completed Acceptance Criteria

- Every Docker-enabled render contains the Redis, RustFS, and OpenSearch catalog; Docker-disabled
  renders contain no generated Compose catalog.
- All catalog containers are inactive when `COMPOSE_PROFILES` is absent or empty, while existing
  unprofiled required services retain their behavior.
- Exact `redis`, `rustfs`, and `opensearch` tokens select only their intended stacks; comma-delimited
  combinations select the union.
- Neighboring tokens such as `redis-debug` and `rustfs-tools` do not activate built-in entries.
- Changing only `COMPOSE_PROFILES` is sufficient for the next direct Compose startup after the catalog
  has been rendered; normal watcher-backed `forj dev` sessions pick up the same change.
- `forj dev` can start a standalone active profile in a normal watcher-backed project even when no App
  resource requires it and avoids a profile-only startup when none is active.
- Removing the last active profile suppresses a stale generated profile-only startup task without
  removing owner-customized tasks or startup required by an unprofiled base or override service.
- Generated teardown selects all profiles so clearing activation state does not strand catalog
  containers, and ordinary teardown does not delete their named volumes.
- Redis retains its existing profile and shared-service compatibility.
- RustFS renders a pinned, authenticated S3-compatible service and console while leaving driver
  selection, compilation, bucket creation, and owner endpoint configuration explicit.
- OpenSearch renders a pinned, bounded single-node service and dashboard without requiring a Search
  component or resource.
- RustFS and OpenSearch host ports default to loopback, and the generated documentation explains local
  credentials and host-versus-container endpoints.
- Unrelated profile tokens and existing owner environment values survive reconciliation.
- Profile-aware render tests do not start the full catalog accidentally.

## Future Catalog Expansion

The following ideas are compatible with this design but are not completed acceptance criteria:

1. Enrich the canonical definition with Compose recipe ownership, service names, endpoints, ports,
   environment keys, descriptions, and resource-link metadata instead of keeping those details in the
   current templates and integration code.
2. Compose catalog entries from reusable generated fragments so adding a built-in service does not
   require editing one monolithic Compose template.
3. Publish active-only API and admin links through the existing resource registry for RustFS,
   OpenSearch, and future services, without exposing secrets.
4. Add a helper that lists profiles or edits `COMPOSE_PROFILES` token by token while preserving the
   same activation contract.
5. Introduce a first-party Search resource only when its App API and OpenSearch provider contract are
   designed independently.
6. Add live Docker validation for empty, individual, combined, and unknown profile selections,
   including dependency readiness and supported host architectures.
7. Add direct Compose lifecycle coverage for all-profile shutdown without making normal shutdown
   destructive.
8. Define image and persisted-data migration policy before changing an existing pinned service in a
   way that requires operator action.

For a future built-in entry, maintainers should choose a stable exact profile, check naming and port
collisions, pin and review the image, define safe port and volume behavior, document activation and
App binding, add exact-token render coverage, and add focused live coverage proportional to the
service's operational risk.

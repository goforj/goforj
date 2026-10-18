# Docker Compose Developer-Service Catalog Design

## Status

- Design decision: complete for the expanded built-in catalog.
- Implemented scope: always-rendered optional Compose profiles; modular catalog-owned recipes;
  multi-capability resource-provider bindings; exact profile activation; component-default and legacy
  profile migration; invocation-time lifecycle support; profile-aware test filtering; active component
  tool links; and generated developer documentation.
- Target repository: `goforj`.

This document records the completed catalog contract. It does not claim that every recipe has a live
Docker integration lane.

## Decision

Every Docker-enabled GoForj project receives the same built-in set of optional developer services in
its generated `docker-compose.yml`. Each catalog service is inactive behind an exact Docker Compose
profile unless a selected local resource or compatibility component seeds that profile in the initial
owner environment.

Catalog availability is not another project choice. GoForj does not persist a developer-service list
in `.goforj.yml`, infer which definitions should be rendered from the current components, or require a
rerender before an already-rendered service can be enabled.

`COMPOSE_PROFILES` is the activation state:

```env
COMPOSE_PROFILES=
```

A developer can enable any combination later by editing that owner-managed value:

```env
COMPOSE_PROFILES=nats,jaeger,adminer
```

The next direct Compose startup observes the change without modifying `.goforj.yml`, regenerating
Compose, or rebuilding the App. Normal watcher-backed `forj dev` sessions observe the same effective
profile value through their pre-development lifecycle.

The contract separates three concerns:

```text
Built-in Compose catalog  decides which optional local services are available
COMPOSE_PROFILES          decides which catalog services start on this machine
App resource config      decides whether and how an App consumes a service
```

Starting a profile does not silently add an App driver, change a selected driver, create a bucket,
queue, topic, realm, user, or index, or redirect an explicitly external endpoint.

## Catalog Architecture

The canonical `internal/devservices` catalog records each entry's stable key, label, exact profile,
ordered list of resource service keys it can provide, optional component defaults, owned template
partial, and presentation order. Catalog access returns defensive copies, and profile lookup and
enablement use exact tokens.

One entry may provide several App capabilities. This is necessary for NATS, whose one local service
can satisfy Cache, Queue, and Events requirements, and for shared Redis, which continues to serve the
existing cross-resource Redis contract. Provider metadata controls local service planning; it does
not gate whether a recipe is rendered.

Each catalog entry owns a Compose partial under
`templates/containers/developer-services`. A partial defines that entry's volume and service blocks.
The project renderer loads the partial named by every catalog definition and builds the aggregate
volume and service templates consumed by `docker-compose.yml`. The catalog order therefore determines
deterministic output without restoring one monolithic Compose template.

This arrangement makes the catalog definition, recipe ownership, profile advertisement, provider
selection, and rendered composition one connected contract. Adding an entry requires a catalog
definition and its partial rather than edits scattered through a hard-coded aggregate.

## Shipped Catalog

The complete catalog has these stable user-facing profiles:

| Profile | Generated services | Role and integration |
| --- | --- | --- |
| `redis` | Redis | Shared local provider for Redis-backed Cache, Queue, Events, and Storage |
| `rustfs` | RustFS API and console | S3-compatible Storage provider and admin tool |
| `opensearch` | OpenSearch and OpenSearch Dashboards | Standalone search and administration stack; no first-party Search resource |
| `nats` | NATS with JetStream and monitoring | One provider for NATS Cache, Queue, NATS Events, and NATS JetStream Events |
| `rabbitmq` | RabbitMQ and management UI | RabbitMQ Queue provider |
| `redpanda` | Redpanda and Redpanda Console | Kafka Events provider and administration stack |
| `dynamodb` | DynamoDB Local | DynamoDB Cache provider |
| `elasticmq` | ElasticMQ and ElasticMQ UI | SQS Queue provider and administration stack |
| `pubsub` | Google Cloud Pub/Sub emulator | Google Pub/Sub Events provider |
| `memcached` | Memcached | Memcached Cache provider |
| `sftpgo` | SFTPGo SFTP endpoint and web administration | Standalone file-transfer tool; deliberately not an automatic Storage provider |
| `adminer` | Adminer | Standalone database browser |
| `jaeger` | Jaeger UI and OTLP receivers | Standalone tracing service |
| `qdrant` | Qdrant HTTP and gRPC APIs | Standalone vector database |
| `temporal` | Temporal development server and UI | Standalone workflow service |
| `keycloak` | Keycloak development server | Standalone identity service |
| `mockserver` | MockServer | Standalone API test double |
| `toxiproxy` | Toxiproxy API and one proxy listener | Standalone network-failure tool; no upstream is selected automatically |
| `clickhouse` | ClickHouse HTTP and native APIs | Standalone analytics database |
| `meilisearch` | Meilisearch | Standalone application-search service |
| `mailpit` | Mailpit SMTP and web inbox | SMTP Mail provider and Mail-component compatibility default |
| `victoriametrics` | VictoriaMetrics and component-dependent vmagent | Observability-component compatibility default |
| `grafana` | Grafana and component-dependent dashboard seeding | Grafana-component compatibility default; also selects VictoriaMetrics |

The profile is the user-facing lifecycle unit. One profile may own several containers when they form
one useful stack, as with OpenSearch, Redpanda, and ElasticMQ. VictoriaMetrics declares both the
`victoriametrics` and `grafana` profiles so selecting Grafana also starts its datasource.

Catalog images and component build bases use explicit tags. Stateful services use named volumes where
restart persistence is useful, and services that need startup ordering use health or dependency
conditions. Existing required services such as a selected local MySQL or Postgres database retain
their unprofiled component and service-plan behavior.

## Activation Contract

Profile matching is exact and comma-delimited. `redis-debug` does not activate `redis`, and
`rustfs-tools` does not activate `rustfs`. Combinations activate the union of exact profiles.

An absent or empty `COMPOSE_PROFILES` value leaves optional entries inactive unless a new render seeds
profiles from selected local providers or component compatibility defaults. Unknown tokens remain
owner intent and survive reconciliation so an owner-managed Compose override can define additional
profiled services. Built-in profile names are a generated-configuration compatibility contract and
must not be repurposed.

Docker Compose can also start an explicitly targeted profiled service. GoForj's normal development
lifecycle uses the effective `COMPOSE_PROFILES` value instead.

An existing project needs one render to receive catalog definitions introduced by a newer GoForj
release. After the definitions exist in its Compose file, later activation changes require only an
edit to `COMPOSE_PROFILES`.

## Resource Provider Integration

Catalog definitions may provide zero, one, or several existing service requirements:

- `redis` provides the existing shared Redis requirement;
- `rustfs` provides S3 Storage;
- `nats` provides NATS Cache, NATS Queue, and NATS or NATS JetStream Events;
- `rabbitmq` provides RabbitMQ Queue;
- `redpanda` provides Kafka Events;
- `dynamodb` provides DynamoDB Cache;
- `elasticmq` provides SQS Queue;
- `pubsub` provides Google Pub/Sub Events;
- `memcached` provides Memcached Cache; and
- `mailpit` provides local SMTP Mail.

The corresponding drivers are locally provisionable. Initial profile selection is projected from
active-local or retained local service requirements. Resource-specific container values are generated
only for exact local endpoints, with matching host values in `.env.host`:

| Provider | Exact container value |
| --- | --- |
| RustFS | `STORAGE_ENDPOINT=http://rustfs:9000` |
| NATS | `CACHE_URL`, `QUEUE_URL`, or `EVENTS_URL` at `nats://goforj:goforj@nats:4222` |
| RabbitMQ | `QUEUE_URL=amqp://goforj:goforj@rabbitmq:5672/` |
| Redpanda | `EVENTS_BROKERS=redpanda:9092` |
| DynamoDB Local | `CACHE_ENDPOINT=http://dynamodb:8000` |
| ElasticMQ | `QUEUE_ENDPOINT=http://elasticmq:9324` with local region and placeholder credentials |
| Pub/Sub emulator | `EVENTS_URI=gcppubsub:8085` with a local project ID |
| Memcached | `CACHE_ADDRESSES=memcached:11211` |
| Mailpit | `MAIL_SMTP_HOST=mailpit` and `MAIL_SMTP_PORT=1025` |

Endpoint affinity remains strict:

- an explicitly external endpoint remains external;
- neighboring names such as `rustfs-tools`, `nats-debug`, and `rabbitmq-admin` are not captured;
- missing ports or a different port do not match the generated provider;
- compatible consumers share a provider only under existing service-plan rules; and
- activating a profile does not add a supported driver, select it, create provider objects, or
  overwrite owner credentials.

Redis retains its established service-intent bridge. The other providers use exact generated
container endpoints for local affinity and project corresponding localhost endpoints for host-run
commands.

SFTPGo is intentionally excluded from provider metadata. Its generated default administrator can
configure the control plane, but a usable FTP or SFTP App disk also needs a protocol user, home
permissions, and an explicit host-key trust decision. Those are owner-managed security and data
choices, so generation does not pretend the service is ready to bind automatically.

## Component Defaults And Migration

Mailpit, VictoriaMetrics, and Grafana previously followed component-specific unprofiled startup. They
now use catalog profiles without losing their established component behavior:

- the Mail component seeds `mailpit`, and local SMTP requirements can select the same profile;
- the Observability component seeds `victoriametrics` and adds its generated vmagent; and
- the Grafana component seeds `grafana`, uses generated provisioning and dashboard seeding, and also
  starts VictoriaMetrics through the shared profile declaration.

For an existing owner environment, reconciliation inspects the previous generated Compose file. An
exact unprofiled `mailpit`, `victoriametrics`, or `grafana` service causes its matching profile token to
be appended once. Existing owner and unknown tokens keep their order and are not removed. A profiled
service, neighboring service name, or volume with the same name does not trigger migration.

New renders derive these defaults through catalog `DefaultFor` metadata rather than special-case
Compose inclusion. After the initial seed or one-way migration, `.env` remains the owner-controlled
activation surface.

## Generated Ownership

GoForj owns the generated `docker-compose.yml`, the catalog partials used to produce it, the ElasticMQ
configuration, and `containers/goforj-development-services.md`. A rerender may refresh built-in
recipes and image pins. Project-specific customization belongs in the normal owner-managed
`docker-compose.override.yml`, not in edits to generated catalog definitions.

The ownership rules are:

- framework catalog definitions, recipes, companion configuration, and image pins are generated;
- `.env` owns profile activation, published ports, bootstrap credentials, and local connection values;
- `.env.host` owns host-process endpoint projection;
- unrelated profile tokens and existing owner values survive reconciliation;
- named-volume declarations are generated, while their stored data remains user state;
- disabling a profile, rerendering, and ordinary shutdown do not delete named volumes; and
- there is no `dev.services` or equivalent list in `.goforj.yml`.

The generated guide lists every profile, host and container endpoints, local credential defaults,
resource bindings, SFTPGo's manual bootstrap boundary, bind-address compatibility, and data ownership.

The dev summary and resource registry expose Mailpit, VictoriaMetrics, and Grafana links only when
their exact profiles are enabled. They do not expose passwords or infer activation from neighboring
tokens.

## Development Lifecycle

The App resource service plan cannot be the only Compose startup trigger because standalone profiles
such as `jaeger`, `keycloak`, and `opensearch` have no App consumer.

At each invocation, `forj dev` uses the process `COMPOSE_PROFILES` value when present and otherwise
falls back to the project's `.env` value. For Docker projects, it adds the conventional Compose startup
task when at least one profile token is present and no owner task with that conventional name already
exists. This lets any built-in or owner-defined profile start without rerendering while preserving an
owner's custom conventional task. Compose validates unknown tokens.

Existing generated startup tasks continue to cover unprofiled required services. A Docker project
with only inactive catalog entries does not add a profile-only startup task. A stale generated task is
suppressed after the last active profile is removed when neither the base Compose file nor its
conventional override contains an unprofiled service. Owner-customized tasks are preserved.

`forj dev` teardown and `forj down` ensure the conventional all-profile Compose down task is present
for Docker projects. Selecting all profiles ensures a container is still included after its activation
token is removed. Ordinary teardown does not request named-volume deletion.

Removing a token changes later Compose selection; it does not stop a container that was already
created. Developers use `forj down` or an all-profile Compose teardown when disabling a running
profile before starting the new selection.

## Profile-Aware Test Rendering

The rendered-project test harness filters Compose services before allocating host ports or starting
testcontainers:

- services without profiles remain active;
- a profiled service is active when at least one declared profile is an exact enabled token;
- inactive catalog services are skipped; and
- unknown and neighboring tokens do not accidentally activate a built-in service.

Recipes whose dependency, network, resource, or bootstrap semantics are not modeled by the generic
harness carry an explicit test exclusion. Structural, unit, and render tests cover the catalog and its
profile behavior; this design does not claim a live Docker matrix for every entry.

## Security And Operational Defaults

An always-rendered catalog must be inert while disabled and explicit about local-development security
when enabled.

- Catalog ports bind through `DEV_SERVICE_IP_ADDRESS=127.0.0.1` by default.
- Redis, Mailpit, VictoriaMetrics, Grafana, MySQL, and PostgreSQL continue to
  accept `IP_ADDRESS` as an explicit override, but no longer default it to a
  public interface.
- Services with bootstrap authentication use overridable local credentials. Services documented as
  unauthenticated rely on loopback isolation and must not be exposed casually.
- RustFS authentication remains enabled, but bucket creation and App driver selection remain explicit.
- OpenSearch keeps security and development TLS enabled. Its initial password bootstraps a fresh
  persisted security index and does not rotate an existing password.
- SFTPGo creates an administrator but not a protocol user or an App host-key policy.
- Toxiproxy creates no upstream mapping, and enabling it does not redirect App traffic.
- Generated documentation distinguishes container endpoints from host endpoints and explains dotenv
  quoting for local secrets.
- The recipes do not require privileged mode, host networking, a Docker socket mount, or a broad
  host-directory mount.

The catalog is a development convenience, not production orchestration. Production credentials, TLS
policy, clustering, backups, migration, and availability remain deployment responsibilities.

## Compatibility

The existing exact `redis`, `rustfs`, and `opensearch` profiles remain valid. Redis preserves its
shared-service and bind-address contracts. Mailpit, VictoriaMetrics, and Grafana keep their existing
component defaults through initial profile seeding and one-way migration from exact unprofiled
services.

Adding an inactive catalog entry changes generated Compose output but does not activate its container,
change an App source API, alter the resource build contract, redirect an external endpoint, or raise
the minimum Go version.

Compatibility risks are classified separately:

- **Generated configuration:** Docker projects gain optional services, volumes, environment defaults,
  companion configuration, documentation, and stable profile names.
- **Runtime behavior:** enabling a profile starts its pinned local stack and exposes documented ports.
- **Persisted data:** named-volume contents survive profile disablement and ordinary teardown.
- **Operational migration:** a future image or data-format change may require release notes and a
  service-specific migration path.
- **Owner configuration:** unknown profile tokens and existing owner environment values remain intact
  during reconciliation.

## Non-Goals

- Do not add a developer-service selection stage to `forj new`.
- Do not persist selected or available catalog entries in `.goforj.yml`.
- Do not activate every profile by default.
- Do not use components or supported resource drivers as the catalog inclusion gate.
- Do not silently add App drivers, dependencies, resource instances, buckets, queues, topics, realms,
  protocol users, credentials, or endpoint overrides.
- Do not introduce first-party Search, Vector, Workflow, or Identity resources merely because a local
  tool exists.
- Do not redirect external or opaque resource endpoints to local containers.
- Do not accept arbitrary Compose fragments, images, or install scripts in project YAML. Owners use
  the standard Compose override surface.
- Do not delete developer-service data during profile disablement, rerender, or ordinary shutdown.
- Do not present these recipes as production deployment manifests.

## Completed Verification

The implementation has focused structural, unit, and render coverage for:

- stable catalog metadata, ordering, defensive copies, exact lookups, and template ownership;
- aggregate rendering from every catalog partial;
- exact and combined profile parsing, including neighboring and unknown tokens;
- provider-to-profile projection, including NATS multi-capability sharing;
- strict generated endpoint affinity without redirecting external endpoints;
- generated Compose, `.env`, `.env.host`, companion configuration, and developer guide output;
- Mailpit, VictoriaMetrics, and Grafana component defaults and exact one-way migration;
- invocation-time `forj dev` task selection, stale-task suppression, override eligibility, and process
  environment precedence;
- all-profile teardown availability and generated-command normalization;
- profile-aware rendered-project filtering before port allocation and container startup; and
- exact active-profile gating for Mailpit, VictoriaMetrics, and Grafana links.

These checks verify the completed contract without claiming live integration coverage for every
catalog recipe.

## Completed Acceptance Criteria

- Every Docker-enabled render contains all catalog profiles; Docker-disabled renders contain no
  generated Compose catalog.
- Optional containers remain dormant unless an exact profile is enabled, while existing unprofiled
  required services retain their behavior.
- Every advertised token selects only its owned stack; neighboring tokens do not activate entries.
- Editing only `COMPOSE_PROFILES` changes the next startup selection after the catalog is rendered.
- Unknown owner profiles and existing owner environment values survive reconciliation.
- Catalog definitions own modular Compose partials, and deterministic aggregation includes every
  definition's volumes and services.
- Redis, RustFS, NATS, RabbitMQ, Redpanda, DynamoDB Local, ElasticMQ, Pub/Sub, Memcached, and Mailpit
  project exact local provider intent without capturing external endpoints.
- NATS can satisfy Cache, Queue, and Events requirements through one profile and one local service.
- SFTPGo remains standalone until an owner creates a protocol user and chooses host-key handling.
- Mailpit, VictoriaMetrics, and Grafana are profile-controlled, seed from their components, and migrate
  exact legacy unprofiled services without discarding owner tokens.
- Grafana activation also includes VictoriaMetrics; component-specific vmagent and dashboard seeding
  remain conditional on the corresponding generated components.
- All published development-service ports default to loopback, while the legacy
  `IP_ADDRESS` override remains documented for intentional external access.
- Profile-aware render tests do not start the complete catalog accidentally.
- Generated teardown can select every profile without requesting deletion of named volumes.

## Future Work

The completed catalog does not prevent later additions such as richer endpoint metadata, generalized
active-service links, resource-oriented helper commands, a first-party Search resource, or dedicated
live lanes for heavier stacks. Those additions must preserve exact profile activation, owner
environment ownership, strict endpoint affinity, and the modular partial contract.

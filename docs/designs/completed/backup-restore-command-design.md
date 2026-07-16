# Backup And Restore Command Design

Status:

- completed for v1; later backend and operator refinements are follow-up work
- intended for the framework operator command surface
- focused first on application-level database and storage backups

## Purpose

This document defines a framework-level model for backing up and restoring
resources configured by a generated GoForj App.

The goal is to make the common production operation feel simple for resources
owned by a generated GoForj App:

```bash
forj backup:create
forj backup:list
forj backup:verify
forj backup:restore
```

while preserving GoForj's normal model:

- App-owned resource configuration
- framework-owned operator commands
- explicit resource names
- driver-aware behavior
- safe defaults for destructive restore operations
- clear separation between application backups and deployment-node archives

## Problem

Production GoForj Apps can have more than one durable resource:

- a default MySQL or MariaDB database
- a reporting Postgres database
- a local SQLite database
- private local file storage
- public S3-compatible storage
- multiple named disks, queues, caches, or event transports

Without a framework command, each project has to rediscover:

- which resources are durable
- which resources are ephemeral
- which driver-specific backup tool is correct
- which environment keys define connection details
- how to produce a restorable backup set
- how to avoid leaking secrets in command output
- how to safely restore into the intended environment

Simple projects often start with shell scripts. That is acceptable, but it does
not scale well when a generated App uses multiple databases, multiple storage
drivers, or named resources. GoForj already knows the App resource shape and can
turn that knowledge into a consistent backup plan.

## Goals

- Provide standard backup and restore commands in the `forj` framework CLI for generated Apps.
- Discover configured durable resources from generated App configuration.
- Use driver-aware backup and restore strategies.
- Support multiple databases and multiple storage disks.
- Produce a portable manifest-backed backup set.
- Provide an explicit portable data format for cross-driver transfers.
- Prove portable transfers across every supported database pair.
- Keep restore operations explicit, inspectable, and hard to run accidentally.
- Support local backup directories first, then object-storage repositories.
- Keep secrets out of logs, manifests, and console output.
- Allow projects to retain deployment-level archive workflows separately.
- Make backup status visible enough for production operators and future
  Lighthouse surfaces.

## Non-Goals

- Do not make raw deployment-directory archives the core framework backup
  abstraction.
- Do not guarantee crash-consistent raw database file backups.
- Do not make GoForj a full replacement for enterprise backup products.
- Do not hide unsupported drivers behind fake success.
- Do not back up volatile in-memory resources by default.
- Do not make restore silently overwrite production resources.
- Do not require S3, Backblaze, restic, kopia, or any external backup tool for
  the first slice.
- Do not move reusable driver internals into GoForj when they belong in sibling
  resource packages.

## Core Model

GoForj should distinguish between two backup classes.

### Application Backup

Application backups are the framework-owned command surface.

They contain durable App resources:

- logical database dumps
- storage disk exports
- selected App metadata
- checksums
- a manifest describing how the backup was created and how it can be restored

Application backups are meant to be restored resource-by-resource or as a full
App data set.

### Deployment Backup

Deployment backups are node-level archives.

Examples:

- archive `/opt/deployments/myapp`
- preserve permissions, ownership, symlinks, and timestamps
- include `.env`, Compose files, local `_data/`, deploy scripts, and repo state
- keep a small number of weekly copies

That is a valid production pattern, but it is not the same as an application
backup. Raw database files inside a deployment archive are opportunistic and may
not be the authoritative restore source. A restore should prefer the
application-level logical database dump when one exists.

Deployment backup retention must be configured separately from application
backup retention. A project may reasonably keep four weekly deployment archives
while keeping 14 or 30 daily application backups. Those policies answer
different restore questions and should not share one `BACKUP_KEEP` value.

GoForj can provide templates or docs for deployment archives later, but
`backup:create` should first mean "backup the App's durable resources using
driver-aware strategies."

If GoForj later adds a deployment archive helper, it should use a distinct
command and env namespace, for example:

```bash
forj deploy:archive
```

```env
DEPLOYMENT_BACKUP_KEEP_WEEKLY=4
DEPLOYMENT_BACKUP_PATH=/var/backups/goforj/deployments
```

That helper would archive the deployment directory. It would not replace
application-level `backup:create`.

## Command Shape

Use resource-first command names on the framework operator CLI:

```bash
forj backup:create
forj backup:list
forj backup:verify
forj backup:restore
forj backup:prune
forj backup:plan
```

Short aliases may be added later only if they stay clear:

```bash
forj backup
```

The canonical shape should remain `backup:<action>` because it matches existing
GoForj command grammar:

```bash
route:list
queue:work
schedule:run
make:model
auth:create-user
```

These are framework operator commands. Generated Apps expose resource metadata
for the framework to consume, but do not embed backup behavior:

```bash
forj backup:create
```

The framework owns the backup implementation and command surface. For an
App-prefixed backup command, `forj` loads `.env`, promotes the selected App's
prefixed keys, and runs the framework command in-process. It does not delegate
to a generated binary that cannot import the framework's internal package.

## Backup Plan

`backup:plan` should print the driver-aware plan without creating a backup.

Example:

```text
Backup plan

Databases
  default      mysql       dump via mysqldump
  reporting    postgres    dump via pg_dump

Storage
  private      local       archive files
  public       s3          export object manifest

Skipped
  cache        memory      ephemeral
  queue        workerpool  ephemeral
```

The plan is useful before automation because it makes resource classification
visible.

Suggested flags:

- `--json` prints a machine-readable plan.
- `--resource <kind.name>` filters to one resource.
- `--include-ephemeral` shows resources that are normally skipped.
- `--env-file <path>` selects the environment file when supported by the App.

## Resource Classification

Each configured resource should be classified before backup.

Suggested classes:

| Class | Meaning |
| --- | --- |
| `backupable` | GoForj has a strategy to create a restorable artifact. |
| `restorable` | GoForj has a strategy to restore the artifact. |
| `ephemeral` | The resource is intentionally skipped by default. |
| `external-managed` | The resource should be backed up by its provider or platform. |
| `unsupported` | The resource is durable, but GoForj has no strategy yet. |

Examples:

| Resource | Driver | Classification |
| --- | --- | --- |
| `db.default` | `mysql` | `backupable`, `restorable` |
| `db.reporting` | `postgres` | `backupable`, `restorable` |
| `db.local` | `sqlite` | `backupable`, `restorable` |
| `storage.private` | `local` | `backupable`, `restorable` |
| `storage.public` | `s3` | `backupable`, maybe `restorable` |
| `cache.default` | `memory` | `ephemeral` |
| `queue.default` | `workerpool` | `ephemeral` |
| `cache.sessions` | `redis` | `ephemeral` by default, opt-in later |

If a durable resource is unsupported, `backup:create` should fail unless the
user explicitly excludes that resource.

## Backup Set Layout

Backups should be written as a directory first. A later step may archive or push
that directory to a remote repository.

Suggested layout:

```text
backup-2026-07-09T030000Z/
  manifest.json
  checksums.txt

  databases/
    default.mysql.sql.zst
    reporting.postgres.dump.zst

  storage/
    private.local.tar.zst
    public.s3.objects.json.zst

  metadata/
    app.json
    env-contract.json
```

The backup set can then be archived:

```text
backup-2026-07-09T030000Z.tar.zst
```

The directory form is easier to inspect, verify, and partially restore. The
archive form is easier to upload as one object.

## Manifest

Every backup must include a manifest.

Suggested fields:

```json
{
  "version": 1,
  "created_at": "2026-07-09T03:00:00Z",
  "app": {
    "name": "app",
    "binary": "app",
    "goforj_version": "0.20.0",
    "git_sha": "..."
  },
  "resources": [
    {
      "id": "db.default",
      "kind": "database",
      "name": "default",
      "driver": "mysql",
      "strategy": "mysqldump",
      "artifact": "databases/default.mysql.sql.zst",
      "checksum": "sha256:..."
    }
  ]
}
```

The manifest should not contain passwords, tokens, DSNs with credentials, access
keys, or raw environment values.

It should contain enough safe metadata to answer:

- what App created this backup
- which resources were included
- which resources were skipped
- which strategy created each artifact
- which tool versions were used when available
- which checksums prove artifact integrity

## Database Strategies

Database backups should use logical dumps by default.

### MySQL And MariaDB

Strategy:

```bash
mysqldump --single-transaction --quick --routines --triggers --events --hex-blob
```

Notes:

- use `mariadb-dump` when selected or when `mysqldump` is unavailable
- avoid printing passwords
- support host, port, username, database, and DSN-derived configuration
- preserve UTC/timezone-related connection policy where the generated App owns it

### Postgres

Strategy:

```bash
pg_dump --format=custom
```

Notes:

- custom format enables better restore behavior with `pg_restore`
- support plain SQL as an option if users need human-readable artifacts
- avoid printing connection URLs with embedded credentials

### SQLite

Strategy:

- use SQLite's online backup API when GoForj owns a Go-based implementation
- otherwise use `sqlite3 ".backup"` through the local client

Notes:

- avoid raw file copy when the database may be live
- include WAL-related behavior in the strategy documentation

## Portable Database Backups

Native database backups are the first-class production backup format. They
preserve the semantics and features of their database engine and should be the
default for disaster recovery:

```bash
forj backup:create
```

Portable backups are a separate, explicit format for moving application data
between supported database drivers:

```bash
forj backup:create --portable
forj backup:restore --portable --from ./portable-backup --target-driver sqlite
```

`--portable` must not convert a native dump after it has been created. It reads
rows from the source database and writes a versioned GoForj logical data
archive. This keeps native recovery artifacts faithful and makes portable
conversion behavior inspectable.

The portable workflow is:

1. Resolve the source database and its migration stream.
2. Read the source schema and migration fingerprint.
3. Extract rows into GoForj canonical values.
4. Write a portable manifest and table data artifacts.
5. Run the target driver's migrations before restore. The generated command
   validates the resulting migration fingerprint; migration execution remains
   the App's existing migration command so backup does not invent a second
   migration runner.
6. Validate target schema compatibility.
7. Restore rows in dependency order.
8. Restore identity or sequence state and validate constraints.

The target schema is always built from target-dialect migrations. Portable
backup must not recreate arbitrary discovered schema because migrations are the
application's schema contract.

Portable manifests carry the ordered SQL migration fingerprint when the project
has a `migrations/` directory. Restore compares that fingerprint before opening
the write transaction, then performs the independent target-table and column
compatibility checks.

### Canonical Value Model

Portable data should use a deliberately small type system rather than trying to
translate vendor-specific SQL types directly:

| Canonical type | Examples |
| --- | --- |
| `string` | text and character columns |
| `integer` | signed integer values |
| `decimal` | exact numeric values represented without floating point |
| `boolean` | normalized true or false |
| `timestamp` | normalized instants with an explicit timezone policy |
| `date` | calendar dates |
| `bytes` | binary and blob values |
| `json` | structured JSON values |
| `null` | SQL NULL |

Driver-specific values are normalized before they enter the portable archive.
For example, SQLite, MySQL, and Postgres boolean representations must all
become the same canonical boolean value. Decimal values must remain exact, and
binary values must remain byte-for-byte recoverable.

The portable manifest should include:

- portable format version
- source driver and database identity without credentials
- migration stream and schema fingerprint
- included tables and row counts
- canonical column types
- conversion warnings
- checksums for every data artifact

Portable mode should explicitly report or reject features that do not have a
safe common representation, including stored procedures, engine-specific
indexes, collations, generated columns, extensions, arrays, spatial types,
vendor-specific enums, and database permissions.

## Portable Transfer Test Matrix

Portable backup is not production-ready until it is tested as a transfer
matrix, not only as a source-database round trip.

For the initial SQLite, MySQL, and Postgres drivers, integration tests should
cover every source-target pair:

| Source | SQLite | MySQL | Postgres |
| --- | ---: | ---: | ---: |
| SQLite | yes | yes | yes |
| MySQL | yes | yes | yes |
| Postgres | yes | yes | yes |

Each test should:

1. Start a real source and target database using the supported test harness.
2. Apply the source dialect's migrations.
3. Load the shared compatibility fixture into the source.
4. Apply the target dialect's migrations independently.
5. Create a portable archive from the source.
6. Restore the archive into the target.
7. Compare source and target through canonical semantic values.
8. Verify constraints, identity state, row counts, and checksums.

The shared fixture should exercise the portable boundary deliberately:

- signed and large integer values
- exact decimal values, including trailing zeroes
- booleans and nullable booleans
- empty strings, Unicode, and long text
- dates and timestamps with timezone information
- UUIDs, JSON, and arbitrary binary data
- foreign keys, nullable foreign keys, and composite keys
- unique constraints and empty tables
- auto-increment and sequence continuation

Assertions must compare semantic values rather than raw driver return values.
For example, a MySQL `1`, a Postgres `true`, and a SQLite integer boolean are
equal only after canonicalization.

### Round-Trip And Negative Tests

The matrix should also include chained transfers:

```text
mysql -> sqlite -> postgres
postgres -> mysql -> sqlite
sqlite -> postgres -> mysql
```

The final canonical data must equal the original canonical data. After each
restore, the test should insert a new row to prove that identity and sequence
state continue correctly.

The suite must include negative compatibility tests. Portable restore should
fail clearly, or require an explicit override, when it encounters:

- a missing target table or column
- a non-null target column receiving source NULL
- precision that exceeds the target schema
- unsupported arrays, spatial values, or generated columns
- incompatible migration fingerprints
- foreign-key cycles that cannot be restored safely
- unsupported enum, collation, or extension behavior

Errors should identify the table, column, source type, target type, and
recommended action. Silent truncation or lossy coercion is not acceptable.

### Native Backup Tests

Native backup tests remain separate from portable transfer tests:

```text
mysql -> mysqldump -> mysql
postgres -> pg_dump -> postgres
sqlite -> native backup -> sqlite
```

These tests validate engine-specific fidelity and production recovery. They
should cover native features that portable mode intentionally does not promise,
such as triggers, indexes, sequences, and engine-specific metadata.

### Multiple Connections

Each configured connection should be backed up independently:

```text
db.default
db.audit
db.reporting
```

Restore should allow:

```bash
forj backup:restore --resource db.reporting --from ./backup-...
```

## Storage Strategies

Storage backups should use the storage resource name as the stable selector.

### Local Storage

Strategy:

- archive the configured root
- preserve permissions, ownership, symlinks, and timestamps where the platform
  allows it
- compress the archive

For application storage, a tar archive is acceptable because the files are
ordinary App assets, uploads, and generated artifacts rather than database data.

### S3-Compatible Storage

There are two possible strategies.

First slice:

- write an object manifest with keys, sizes, ETags, versions when available,
  timestamps, and checksums when available
- optionally mirror objects into the backup set when `--materialize-remote`
  is set

Later slice:

- support object-copy export into the backup repository
- support restore by replaying the manifest or copying exported objects back

S3-compatible storage can represent a large data set. The command should be
explicit about whether it is backing up object metadata only or the object
contents.

The first implementation records a checksummed object inventory for configured
S3 disks. The inventory is deliberately marked metadata-only; restore refuses
to treat it as a complete object backup until an explicit materialization mode
is implemented.

### Public And Private Disks

Generated Apps may have public and private disks. Treat both as normal named
storage resources:

```text
storage.private
storage.public
storage.invoices
```

Do not hardcode only one storage path.

## Destination Backends

The first implementation should write to a local path:

```bash
forj backup:create --to ./storage/backups
```

Later repository backends can include:

```text
local
s3
b2-s3
ssh
```

Suggested environment shape:

```env
APP_BACKUP_DRIVER=local
APP_BACKUP_PATH=./storage/backups
APP_BACKUP_KEEP_DAILY=14
APP_BACKUP_KEEP_WEEKLY=4

APP_BACKUP_S3_ENDPOINT=
APP_BACKUP_S3_BUCKET=
APP_BACKUP_S3_PREFIX=
APP_BACKUP_S3_REGION=
APP_BACKUP_S3_ACCESS_KEY_ID=
APP_BACKUP_S3_SECRET_ACCESS_KEY=
```

Backblaze B2 should not require a special core model. It can be represented as
S3-compatible storage with an endpoint and credentials. A `b2-s3` alias may be
useful for defaults and documentation.

## Retention And Pruning

Retention should be explicit and separate from backup creation:

```bash
forj backup:prune
forj backup:prune --dry-run
```

`backup:create` may optionally run prune after a successful verified upload:

```bash
forj backup:create --prune
```

Important rule:

> Never delete older backups until the new backup has completed and verification
> has passed.

Suggested retention keys:

```env
APP_BACKUP_KEEP_DAILY=14
APP_BACKUP_KEEP_WEEKLY=4
APP_BACKUP_KEEP_MONTHLY=6
```

Application backup retention should operate on completed manifest-backed
backups only. It must not prune deployment archives.

## Restore Safety

Restore is destructive and should be conservative.

Required defaults:

- `backup:restore` prints a plan first.
- `backup:restore` refuses to run against production without explicit
  confirmation.
- `backup:restore --dry-run` should be the documented first command.
- partial restore should be supported through `--resource`.
- scratch restore should be supported when practical.

Suggested examples:

```bash
forj backup:restore --from ./backup-2026-07-09T030000Z --dry-run
forj backup:restore --from ./backup-2026-07-09T030000Z --resource db.default
forj backup:restore --from ./backup-2026-07-09T030000Z --target ./restore-test
forj backup:restore --from ./backup-2026-07-09T030000Z --confirm restore-production
```

The restore plan should show:

- source backup identity
- target App/environment
- resources that will be overwritten
- resources that will be skipped
- driver-specific restore commands
- whether services should be stopped first

The command should not infer that a production restore is safe just because a
backup exists.

## Consistency Model

The framework should be honest about consistency.

Recommended first behavior:

1. Build the backup plan.
2. Create database dumps.
3. Create storage artifacts.
4. Write the manifest and checksums.
5. Verify local artifacts.
6. Push to the backup repository when configured.
7. Verify the uploaded backup when supported.
8. Prune only after successful verification.

Backing up the database before storage may leave extra unreferenced files in the
storage backup. That is usually acceptable. The worse default failure mode is a
database row referencing a file that was not backed up.

Future consistency tools may include:

- app-level maintenance mode
- upload write locks
- driver-level snapshots
- resource-specific pre-backup and post-backup hooks
- quiesce hooks for high-write applications

Those should be explicit; the first version should not pretend to create a
perfect distributed snapshot.

## Hooks

The framework CLI may expose optional lifecycle-style hooks for backup
operations. These hooks are registered and executed by `forj`; generated Apps
only expose resource metadata and do not contain backup execution code.

Conceptual shape:

```text
BeforeBackup
AfterBackup
BeforeRestore
AfterRestore
```

These hooks are for App-owned operational policy, not driver internals.

Examples:

- pause an import worker
- enable a short maintenance flag
- emit an audit event
- invalidate derived caches after restore

Hooks should be optional and explicit. Missing hooks should not require nil
guards in command code; the registry should provide an empty hook list by
default.

## Encryption And Secrets

GoForj should avoid leaking secrets even if backups are not encrypted.

Rules:

- never print raw passwords, tokens, access keys, or DSNs with credentials
- never write backup repository credentials into the manifest
- redact command previews
- keep environment contract metadata separate from raw `.env` contents

Encryption should be supported, but the first implementation can be scoped:

- local backup set creation may be unencrypted by default
- repository upload can use provider-side encryption when available
- client-side encryption can be added later through a backup repository wrapper

If GoForj eventually integrates a tool such as restic or kopia, it should do so
as a repository backend, not as the only backup model.

## Observability

Backup commands should write concise operator output:

```text
backup     planned 4 resources
backup     dumped db.default mysql 18 MB
backup     archived storage.private local 240 MB
backup     wrote manifest
backup     verified checksums
backup     complete backup-2026-07-09T030000Z
```

Failure output should identify the resource:

```text
backup failed: db.reporting postgres dump failed: pg_dump not found
```

Future Lighthouse integration can show:

- last successful backup
- last failed backup
- backup age by resource
- restore/verify command history
- manifest details with secrets redacted

Lighthouse should observe backup state. It should not become the only way to
perform backups.

## App Ownership And Delegation

Backup commands should remain framework operator commands. Apps should expose
only a stable, secret-free resource description contract.

Reasons:

- the App owns resource configuration while `forj` owns operator workflows;
- named Apps may have different resources;
- the backup implementation is absent from production App binaries;
- one framework command surface avoids version drift between App binaries.

For named Apps:

```bash
forj marketplace backup:create
forj marketplace backup:create
```

should back up the `marketplace` App's configured resources.

## Relation To Resource Shell Commands

This design should reuse the same resource-addressing model as App resource
shell commands:

```bash
forj db reporting
forj backup:create --resource db.reporting
```

The App-facing resource name is the selector. The backend driver is an
implementation detail used to choose the backup strategy.

## Relation To Resource Registry

The resource registry design is primarily about discoverable local resources
and links. Backup needs a stricter resource inventory:

- durable or ephemeral
- backupable or unsupported
- restore strategy
- safe display metadata

These concepts can share resource IDs and labels, but backup should not depend
on a UI-oriented registry if that would weaken restore safety.

## Package Boundary

Backup behavior should live in a framework-owned internal package:

```text
internal/backup/
  plan.go
  manifest.go
  checksum.go
  restore.go
  native/
    strategy.go
    sqlite.go
    mysql.go
    postgres.go
  portable/
    archive.go
    canonical.go
    schema.go
```

The package owns backup planning, artifact creation, manifest validation,
checksums, native strategies, portable conversion, and restore orchestration.
It should not own CLI parsing, generated App wiring, or project rendering.

The portable SQL layer should depend on Go's `database/sql` interfaces and a
small dialect adapter, not on GORM. GORM is an App-level connection
implementation, while backup must remain usable with native `database/sql`
connections and future driver integrations. The framework resolves connection
metadata from the App contract and environment without embedding an ORM in the
backup package.

Generated Apps should expose a stable, secret-free resource description
contract through templates. `forj` consumes that contract and injects the
selected App environment into the framework backup workflow:

The contract is versioned (`version: 1`) and contains resource IDs, kinds,
names, normalized drivers, default status, and configuration key names only.
Secret values, DSNs, passwords, and tokens are never serialized.

```text
App resource description
  -> forj backup command
      -> internal/backup service
      -> database connection resolver
      -> native or portable strategy
      -> manifest/artifact repository
```

The framework command is responsible for flags, confirmation, output, and
selecting the active App/resource scope. It may invoke the generated App only
for `resources:describe`; backup execution remains inside the framework.

```bash
forj backup:create
forj billing backup:create
```

The first package interfaces should be small and driver-neutral:

- `Planner` creates an inspectable backup plan.
- `Strategy` creates or restores one database artifact.
- `ArtifactStore` writes and reads local backup artifacts.
- `Manifest` describes verified backup contents.
- `PortableCodec` converts database rows to and from canonical values.

Driver-specific code belongs behind these interfaces. The package may use the
generated database connection manager through an injected resolver, but should
not depend on generated App packages or project template paths.

## Milestones

### Milestone 1: Backup Plan And Manifest

Objective:

- define resource classification, backup plan output, and manifest format

Tasks:

- add generated App resource description contract
- implement `backup:plan`
- define `manifest.json` schema
- define stable resource ID conventions
- add tests for plan ordering and manifest redaction

Exit criteria:

- `forj` can print a deterministic backup plan for a selected App without
  embedding backup behavior in that App

Status: implemented and covered by plan, manifest, redaction, and generated
render tests.

### Milestone 2: Local Database And Local Storage Backups

Objective:

- create useful local application backups for common single-node apps

Tasks:

- implement MySQL/MariaDB dump strategy
- implement Postgres dump strategy
- implement SQLite backup strategy
- implement local storage archive strategy
- write checksums
- implement `backup:verify`

Exit criteria:

- a generated App can create and verify a local backup set containing logical
  database dumps and local storage archives

Status: implemented. SQLite uses the native driver's `VACUUM INTO` path;
MySQL/MariaDB and Postgres use their native dump tools.

### Milestone 3: Restore

Objective:

- safely restore complete or partial backup sets

Tasks:

- implement restore planning
- support `--dry-run`
- require confirmation for destructive targets
- restore MySQL/MariaDB dumps
- restore Postgres dumps
- restore SQLite backups
- restore local storage archives

Exit criteria:

- a backup set can be restored into a scratch/local environment and verified

Status: implemented for native SQLite, MySQL/MariaDB, Postgres, and local
storage, with destructive confirmation and dry-run coverage.

### Milestone 4: Portable Database Transfers

Objective:

- support intentional cross-driver data movement with verified semantics

Tasks:

- define and version the portable archive format
- implement canonical value encoding and decoding
- implement source schema inspection and target migration validation
- implement dependency-aware row export and restore
- add the SQLite, MySQL, and Postgres integration test matrix
- add chained round-trip and negative compatibility tests
- document unsupported and lossy database features

Exit criteria:

- every supported source driver can restore into every supported target driver
- the shared compatibility fixture has canonical data equality after transfer
- identity and sequence values continue correctly after restore
- unsupported or lossy conversions fail explicitly
- portable archives can be verified independently through their manifest and
  checksums

Status: implemented. The integration suite exercises all 3x3 source/target
transfers, chained round trips, identity continuation, and negative contracts.

### Milestone 5: Retention And Repository Backends

Objective:

- support production automation and remote repositories

Tasks:

- implement `backup:list`
- implement `backup:prune --dry-run`
- implement local retention
- add S3-compatible repository upload
- document Backblaze B2 configuration through S3-compatible env keys

Exit criteria:

- production can run scheduled backups to local or S3-compatible repositories
  with deterministic retention

Status: implemented for local repositories and S3-compatible upload, download,
list, delete, checksummed manifests, and calendar-bucket local retention.
Remote object inventory restore is intentionally rejected until explicit
materialization is requested.

### Milestone 6: Operator Integration

Objective:

- make backup state inspectable

Tasks:

- record last backup metadata where appropriate
- expose safe backup status to Lighthouse
- add generated docs for cron/systemd timer examples
- add optional backup hooks

Exit criteria:

- operators can inspect backup freshness without reading raw logs

Status: implemented through `backup:status`, compact freshness output, and
explicit before/after create and restore hook registries in the framework CLI.
Generated Apps expose only the resource contract used to select backup inputs.

## Open Questions

- Should backup repository configuration live only in env, or should `.goforj.yml`
  also declare production backup policy?
- Should client-side encryption be built into GoForj or delegated to repository
  backends such as restic/kopia?
- S3 storage disks are represented by a checksummed object inventory by
  default. Native object materialization is an explicit future repository
  operation; restore rejects metadata-only inventories today.
- Should durable queues ever be backed up, or should they be treated as
  operational state outside the application backup contract?
- How should multi-App projects coordinate shared resources that are configured
  by more than one App?
- Should deployment archive helpers become a separate generated task such as
  `deploy:archive`, or remain documented project-specific operations?

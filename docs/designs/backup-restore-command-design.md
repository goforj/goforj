# Backup And Restore Command Design

Status:

- proposed
- intended for generated App command surfaces
- focused first on application-level database and storage backups

## Purpose

This document defines a framework-level model for backing up and restoring
resources configured by a generated GoForj App.

The goal is to make the common production operation feel simple:

```bash
forj backup:create
forj backup:list
forj backup:verify
forj backup:restore
```

while preserving GoForj's normal model:

- App-owned configuration
- generated App command surfaces
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

- Provide standard backup and restore commands for generated Apps.
- Discover configured durable resources from generated App configuration.
- Use driver-aware backup and restore strategies.
- Support multiple databases and multiple storage disks.
- Produce a portable manifest-backed backup set.
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

Use resource-first command names:

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

These should be generated App commands. The framework CLI may delegate to them
through the existing source-aware path:

```bash
forj backup:create
forj app backup:create
./bin/app backup:create
```

The generated App owns the backup command behavior because it owns the effective
resource configuration.

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

Generated Apps should have optional lifecycle-style hooks for backup operations.

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

Backup commands should be generated App commands.

Reasons:

- the App owns resource configuration
- named Apps may have different resources
- the built binary should work on a production node without source tooling
- source-aware `forj` delegation can preserve one command surface

Inside a generated App:

```bash
./bin/app backup:create
```

should use the same command implementation as:

```bash
forj backup:create
```

For named Apps:

```bash
forj marketplace backup:create
./bin/marketplace backup:create
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

## Milestones

### Milestone 1: Backup Plan And Manifest

Objective:

- define resource classification, backup plan output, and manifest format

Tasks:

- add generated App backup command skeletons
- implement `backup:plan`
- define `manifest.json` schema
- define stable resource ID conventions
- add tests for plan ordering and manifest redaction

Exit criteria:

- generated Apps can print a deterministic backup plan without touching
  unrelated infrastructure

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

### Milestone 4: Retention And Repository Backends

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

### Milestone 5: Operator Integration

Objective:

- make backup state inspectable

Tasks:

- record last backup metadata where appropriate
- expose safe backup status to Lighthouse
- add generated docs for cron/systemd timer examples
- add optional backup hooks

Exit criteria:

- operators can inspect backup freshness without reading raw logs

## Open Questions

- Should backup repository configuration live only in env, or should `.goforj.yml`
  also declare production backup policy?
- Should client-side encryption be built into GoForj or delegated to repository
  backends such as restic/kopia?
- Should S3 storage disks be materialized by default or represented by manifests
  unless explicitly requested?
- Should durable queues ever be backed up, or should they be treated as
  operational state outside the application backup contract?
- How should multi-App projects coordinate shared resources that are configured
  by more than one App?
- Should deployment archive helpers become a separate generated task such as
  `deploy:archive`, or remain documented project-specific operations?

# Database Backup and Restore

## Summary

This branch adds an opt-in framework-level backup and restore system to GoForj. It is
designed for live service applications where operators need a verified local
backup, a production repository, and a deliberate path for moving data between
SQLite, MySQL/MariaDB, and Postgres.

The feature is generated only when an App enables the `backup` component. The
App binary owns the production command surface while using the shared framework
implementation, so default Apps receive no backup commands or dependencies.

## Commands

Backup-enabled generated Apps expose:

Enable the component in `.goforj.yml` before rendering:

```yaml
render:
  components:
    backup: true
```

```text
backup:plan
backup:create
backup:list
backup:verify
backup:restore
backup:prune
backup:status
```

Native backup creation follows the configured database driver. Restore is
destructive and requires an explicit confirmation token:

```bash
./bin/app backup:create
./bin/app backup:verify .goforj/backups/backup-20260712T120000Z
./bin/app backup:restore --from .goforj/backups/backup-20260712T120000Z --dry-run
./bin/app backup:restore \
  --from .goforj/backups/backup-20260712T120000Z \
  --confirm restore-production
```

`backup:status` reports the newest completed local backup in a compact form so
operators do not need to inspect raw logs.

## Native Backups

Native backups preserve the production driver's own format and behavior:

- SQLite uses the native driver's `VACUUM INTO` mechanism.
- MySQL and MariaDB use the appropriate `mysqldump`/`mysql` tools.
- Postgres uses `pg_dump`/`pg_restore`.
- Local storage is archived as a checksummed compressed archive.

Every backup set contains a manifest, resource metadata, artifact checksums,
and sizes. Verification happens before a completed backup is uploaded to a
remote repository.

Restore validates the manifest and checksums before touching the target. It
also verifies that the configured target driver matches the native artifact;
cross-driver restore must use the portable format.

## Portable Transfers

`backup:create --portable` exports data through `database/sql` into a
database-neutral archive. The format contains:

- canonical values for integers, decimals, booleans, timestamps, JSON, bytes,
  strings, and nulls;
- table and column contracts;
- schema and migration fingerprints;
- dependency-aware row data;
- identity and sequence continuation metadata;
- a manifest and checksums.

The target schema remains the target database's migration contract. Operators
run the target migrations first, then restore the portable data. GoForj checks
the migration and schema contracts before opening the write transaction.

The integration suite exercises every source-to-target pair:

```text
SQLite  -> SQLite, MySQL, Postgres
MySQL   -> SQLite, MySQL, Postgres
Postgres -> SQLite, MySQL, Postgres
```

It also covers chained round trips, identity continuation, incompatible
schemas, unsupported values, and failed transfers.

## Repositories and Retention

Completed backup directories can be stored locally or uploaded to an
S3-compatible repository, including Backblaze B2 through its S3 interface.
Repository operations include upload, download, list, and delete. Downloads
use guarded path extraction so object names cannot escape the destination.

S3-backed application storage is currently recorded as a checksummed object
inventory. Because an inventory does not contain object contents, native
restore refuses it rather than claiming that the objects were recovered.
Materialization is an explicit repository operation for future work.

Local retention supports the documented policy keys:

```env
APP_BACKUP_KEEP_DAILY=14
APP_BACKUP_KEEP_WEEKLY=4
APP_BACKUP_KEEP_MONTHLY=6
```

The existing count-based `backup:prune --keep` behavior remains available for
simple deployments. Backups are only eligible for pruning after they have a
valid manifest.

## Hooks

The backup package provides an explicit hook registry for App-owned operational
policy:

- before and after create;
- before and after restore.

Hooks can pause workers, enable maintenance mode, emit audit events, or
invalidate derived state. The default registry is empty, so applications do
not need placeholder dependencies or hidden hook behavior.

## CI

Backup integration now has its own CI job in `.github/workflows/test.yml`.
The job runs in the nested `integration` module with Docker and executes:

1. the portable transfer matrix and compatibility tests;
2. native SQLite and MySQL recovery;
3. isolated native Postgres recovery.

This keeps backup failures visible as a feature-specific signal instead of
depending on the repository's general unit or generator integration jobs.

## Design Boundaries

- Native formats remain first-class for same-driver production recovery.
- Portable archives are for intentional cross-driver movement, not a
  replacement for native dumps.
- Migrations remain the application's schema authority; backup does not create
  a second migration runner.
- Restore is explicit and destructive by design.
- Secrets are read from runtime configuration and are never written into
  manifests or backup metadata.
- The generated public bridge must be included in the pinned GoForj framework
  release before generated Apps are rendered against that release. Local
  development renders use a temporary repository replacement for an
  unreleased checkout.

## Verification

Validated locally with:

```bash
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./internal/backup ./backup
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./internal/backup ./backup
GOFORJ_BACKUP_INTEGRATION=1 \
  GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache \
  sh -c 'cd integration && go test -tags=integration_backup ./...'
```

Wire generation, generated-App rendering, compile-only repository tests, and
`git diff --check` also pass. Full framework integration execution may require
exclusive Docker ports on the runner.

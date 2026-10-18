# Local Providers

Use `forj env:local` when a project configured for MySQL, Redis, or other external providers needs to run with local alternatives.

```sh
forj env:local
```

Choose **Use local providers**, select which resources to switch, review the changes, and confirm. The defaults are SQLite for databases, memory for caches, workerpool for queues, inproc for events, local storage, and log mail. Disabled components stay disabled. The picker includes named resources and App overrides. Exclude a resource to keep its current provider; inherited settings are pinned when needed to preserve its database target or driver.

For example, switching all three service-backed resources changes:

```dotenv
DB_DRIVER=mysql
DB_SUPPORTED_DRIVERS=mysql
DB_DSN=your-private-service-dsn
CACHE_DRIVER=redis
QUEUE_DRIVER=redis
COMPOSE_PROFILES=mysql,redis,grafana
```

To:

```dotenv
DB_DRIVER=sqlite
DB_SUPPORTED_DRIVERS=mysql,sqlite
DB_DSN=
DB_SQLITE_DATABASE=./_data/local/db.db
CACHE_DRIVER=memory
CACHE_SUPPORTED_DRIVERS=redis,memory
QUEUE_DRIVER=workerpool
QUEUE_SUPPORTED_DRIVERS=redis,workerpool
COMPOSE_PROFILES=grafana
```

The exact supported-driver order depends on the existing inventory. Existing providers remain compiled so service configuration can be restored. Only Compose profiles previously used by switched resources and no longer needed by any resource are removed. Observability, custom profiles, and services still used by an excluded resource or another App remain selected.

Run the existing build command afterward:

```sh
forj build
```

This regenerates driver support and builds the App. The switch itself does not build, restart Apps, or stop existing containers. A running `forj dev` session can detect `.env` changes and run its configured startup tasks. Stop it before preparing a database transition.

## Choose individual drivers

Run `forj env:local` and choose **Choose drivers** to switch back to distributed providers or pick another combination:

```text
Resource drivers
  Use local providers
  Choose drivers
  Restore previous settings
  Cancel
```

Choose a resource, pick a driver from its supported catalog choices, and repeat for any other resources. Choose **Review changes** when ready. For example, choose MySQL for `DB_DRIVER` and Redis for `CACHE_DRIVER` and `QUEUE_DRIVER`. **Keep current setting** leaves a resource alone, including inherited settings. Named resources and App overrides appear separately.

The preview shows driver selections, compiled support, and Compose profile changes. Newly required service profiles are enabled. Unselected resources keep their providers and database targets. Connection values stay private. Switching back to the original database driver recovers its saved DSN when the local switch changed it. A SQLite DSN is cleared when selecting a service database. If a named resource would still inherit an incompatible DSN from its parent, the switch stops before writing. Switch the parent too, or configure the resource's driver and dedicated DSN together in `.env`. Saved service DSNs are recovered even when they originally came from a parent that stays on SQLite. A change also stops if another App would inherit that DSN and its existing target cannot be preserved; select the affected named connections together or configure dedicated DSNs. Accepted driver aliases keep their existing DSN when selecting the equivalent catalog driver. Check host, port, database, credentials, and any required DSN before starting a newly selected service driver. The picker does not collect credentials or provision remote services.

You can make further selections without restoring first. One restore point still leads back to the settings before the first switch. Run `forj build` after applying your choices, then restart the App.

## Restore previous settings

Run `forj env:local` again and choose **Restore previous settings**. It previews the restoration and asks for confirmation. There are no named profiles or saved configuration collections.

One ignored, owner-only file, `.env.local-providers.backup`, records the changed settings before and after the switch. Unchanged credentials remain in `.env`; changed connection values are retained privately for restoration. A no-op does not replace the backup. Further switches update that same restore point without losing the original settings. There is no per-switch history.

Restoration changes only assignments touched by the switch. Later edits to unrelated settings survive. If a controlled setting changed after the switch, restoration stops and identifies the conflicting key without displaying its value. Review that edit before restoring. Successful restoration clears the private values from the backup.

Before publishing, the command reparses the proposed `.env` and checks every resolved value against the reviewed configuration. If a preserved reference such as `DB_REPORTS_DRIVER=${DB_DRIVER}` would change an excluded resource, or another dependent setting would change unexpectedly, the command stops without writing. Replace the affected reference with a literal value and retry. The same check protects restoration. Stable references remain untouched.

Whitespace-only database settings follow the existing runtime rules: a nonempty named value blocks root inheritance, and a selected DSN is trimmed afterward. A whitespace-only DSN therefore keeps using its configured SQLite path.

Writes preserve unrelated settings and comments, reject symlinks and tracked private files, check for concurrent edits, and roll back failed replacements. Git inspection failures stop the operation. Other runtime dotenv layers and process-environment overrides keep their existing precedence and are identified in the preview without revealing their values.

## Data and runtime behavior

Switching providers changes configuration. It does not translate migrations, create schemas, or copy database contents. Use compatible migrations and an explicit data-transfer operation when needed. See the [migration translation design](../designs/migration-translation-design.md) for the separate proposal.

Existing SQLite paths and DSNs are retained. Converted service databases receive distinct paths under `./_data/local/`. Shared environment keys with incompatible inherited database targets fail before writing; use distinct resource or App names, or exclude the affected parent resources.

Memory and inproc providers are process-local. Separate App processes do not share their state. Keep a distributed provider selected when cross-process coordination or persistence is required.

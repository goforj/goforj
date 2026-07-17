# Database Connections (database)

This package owns database connection configuration, lazy connection creation, and code-generated accessors. It is designed for multiple connections and multiple drivers, configured entirely from environment variables.

## Overview

- `database.Connections` provides access to the default connection and any named connections.
- Connections are created lazily on first use.
- Configuration is read from env vars with the `DB_` prefix.
- `forj generate --db` emits `connections_gen.go` with typed accessors.
- `forj build` also refreshes generated DB accessors before building the app.

## Env Layout

Default connection uses `DB_*` (driver can be `mysql` or `postgres`):

```
DB_SUPPORTED_DRIVERS=mysql,postgres
DB_DRIVER=mysql
DB_HOST=mysql
DB_DATABASE=db
DB_USERNAME=user
DB_PASSWORD=password
DB_PORT=3306
DB_QUERY_LOGGING=false
DB_MAX_IDLE_CONNECTIONS=5
DB_MAX_OPEN_CONNECTIONS=20
DB_CONN_MAX_LIFETIME_MINUTES=5
```

Named connections use `DB_<NAME>_*`:

```
DB_ANALYTICS_DRIVER=mysql
DB_ANALYTICS_HOST=mysql
DB_ANALYTICS_DATABASE=analytics
DB_ANALYTICS_USERNAME=user
DB_ANALYTICS_PASSWORD=password
DB_ANALYTICS_PORT=3306
DB_ANALYTICS_QUERY_LOGGING=false
DB_ANALYTICS_MAX_IDLE_CONNECTIONS=5
DB_ANALYTICS_MAX_OPEN_CONNECTIONS=20
DB_ANALYTICS_CONN_MAX_LIFETIME_MINUTES=5
```

SQLite uses a file path or DSN:

```
DB_SUPPORTED_DRIVERS=sqlite
DB_DRIVER=sqlite
DB_DATABASE=./_data/sqlite/app.db
```

Supported keys:

```
DRIVER
DSN
HOST
DATABASE
USERNAME
PASSWORD
PORT
QUERY_LOGGING
MAX_IDLE_CONNECTIONS
MAX_OPEN_CONNECTIONS
CONN_MAX_LIFETIME_MINUTES
```

### Driver Selection

`DB_SUPPORTED_DRIVERS` controls which database drivers are generated into the app at compile time. `DB_DRIVER` and `DB_<NAME>_DRIVER` still choose which enabled driver each connection uses at runtime.

This is useful when an app owner wants to build and distribute one app binary that can run in different environments. For example, you may want SQLite in local development, but MySQL or Postgres in production. Enabling multiple drivers in `DB_SUPPORTED_DRIVERS` lets the generated app compile with those database backends available, while deployment env still decides which one is active.

Example:

```
DB_SUPPORTED_DRIVERS=sqlite,mysql
DB_DRIVER=mysql
DB_ANALYTICS_DRIVER=sqlite
```

That generates support for both MySQL and SQLite, uses MySQL for the default connection, and SQLite for the `analytics` connection.

When `DSN` is provided, it is used directly; otherwise DSN is built from host/database credentials.

## Time zones

Generated app runtimes and Compose services default to `TZ=UTC`. Override `TZ` explicitly when application calendar behavior or log presentation should use another location. Use a loadable IANA location, with the optional Unix `:` prefix, or a readable absolute TZif file; invalid values fail startup. Database sessions and automatic persistence remain UTC so stored timestamps do not depend on that runtime choice.

Apply a user's profile time zone when presenting stored timestamps or when a business rule explicitly depends on local calendar time.

Generated database DSNs include the driver's UTC parse location and session time zone settings. A custom `DB_DSN` or `DB_<NAME>_DSN` is used verbatim, so it must provide equivalent UTC settings itself.

### Existing apps

Adopting the UTC runtime, database, and session defaults is an operational migration for existing deployments. Audit scheduled or calendar-based behavior and log timestamp expectations; set `TZ` explicitly to the previous application location if those concerns need a staged transition.

A MySQL or MariaDB session time zone change affects how `TIMESTAMP` values are presented, while historical `DATETIME` values carry no time zone information and may reflect a previous local-time convention.

PostgreSQL does not rewrite stored `TIMESTAMPTZ` instants, but UTC sessions can change their display, casts, `CURRENT_DATE`, `date_trunc`, and conversions involving timestamps without time zones.

Audit those columns and application assumptions before rerendering or redeploying, then choose an app-specific conversion where needed.

### Switching Drivers

`DB_SUPPORTED_DRIVERS` is the set of drivers built into the app; `DB_DRIVER` and the named `DB_<NAME>_DRIVER` values select which of them are active. To switch an existing connection to a driver already built in, provision the destination, migrate its schema and data, configure the connection, then restart or redeploy the App. No source regeneration or rebuild is needed, although changing a locally managed MySQL or Postgres service may require rerendering the Compose setup.

To use a driver that is not built in, add it to `DB_SUPPORTED_DRIVERS`, run `forj generate --db`, and build a new artifact before selecting it. GoForj does not translate schemas or copy database data between drivers.

## Generated Accessors

`forj generate --db` creates typed accessors in `connections_gen.go`:

```go
db, err := conns.Default()
analytics, err := conns.Analytics()
```

Accessors are generated from env var prefixes (`DB_ANALYTICS_*` -> `Analytics()`).

## Generation

### Generate connections only

```
forj generate --db
```

### Generate all (recommended)

```
forj generate
```
`forj generate` runs all generators by default. Use `--db` to run only the database accessor generator.

`forj build` also runs the same DB accessor generation step automatically before building.

## Lazy Connections

`Connections` resolves DB connections lazily, so non‑DB commands don’t require a live database. Each call resolves a cached connection instance.

## Database Shell

Database-enabled Apps include `db:shell`, with the short alias `db`, for opening a shell against configured connections.

Open the default connection:

```
forj db
```

Use the canonical command name when you want the explicit form:

```
forj db:shell
```

Open a named connection:

```
forj db analytics
forj db --connection analytics
```

Choose the launch method:

```
forj db --method local
forj db --method compose
```

By default, `forj db` tries the local client first (`mysql`, `psql`, or `sqlite3`). If the client is missing, it falls back to the generated Docker Compose service when one exists.

Print the selected command without running it:

```
forj db --print
forj db analytics --method local --print
```

Secrets are masked in printed output.

Run one SQL statement:

```
forj db --exec "select count(*) from users"
forj db analytics --exec "select count(*) from events"
```

Pass client-native arguments after `--`:

```
forj db -- --batch -e "select count(*) from users"
forj db analytics -- -c "select count(*) from events"
forj db --connection analytics -- -c "select now()"
```

GoForj adds the configured connection arguments first, then passes your client arguments through.

## Migrations & Multiple Connections

Migrations can target multiple connections based on directory structure:

```
migrations/2026_01_01_000001_create_users.sqlite.up.sql        -> default (sqlite)
migrations/2026_01_01_000001_create_users.mysql.up.sql         -> default (mysql)
migrations/analytics/2026_01_01_000001_add_events.postgres.up.sql -> analytics (postgres)
```

Rules:

- Root migration directory targets the default connection.
- Subfolders map to named connections (e.g. `analytics` -> `DB_ANALYTICS_*`).
- Driver-specific files are generated from `DB_SUPPORTED_DRIVERS`. If that env var is unset, generation falls back to the currently active `DB_DRIVER` and `DB_<NAME>_DRIVER` values.
- If a runtime connection selects a driver that is not listed in `DB_SUPPORTED_DRIVERS`, generation fails fast with a validation error.
- Each connection maintains its own `migrations` table within that database.

## Testing Notes

`connections.go` contains the shared plumbing; `connections_gen.go` is generated. Tests live alongside the package and verify env parsing, accessors, and config behavior.

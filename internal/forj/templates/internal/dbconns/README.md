# Database Connections (dbconns)

This package owns database connection configuration, lazy connection creation, and code-generated accessors. It is designed for multiple connections and multiple drivers, configured entirely from environment variables.

## Overview

- `dbconns.Connections` provides access to the default connection and any named connections.
- Connections are created lazily on first use.
- Configuration is read from env vars with the `DB_` prefix.
- A generator (`generate:dbconns`) emits `connections_gen.go` with typed accessors.

## Env Layout

Default connection uses `DB_*` (driver can be `mysql` or `postgres`):

```
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

`DB_DRIVER` (and `DB_<NAME>_DRIVER`) selects the driver. When `DSN` is provided, it is used directly; otherwise DSN is built from host/database credentials.

## Generated Accessors

`generate:dbconns` creates typed accessors in `connections_gen.go`:

```go
db, err := conns.Default()
analytics, err := conns.Analytics()
```

Accessors are generated from env var prefixes (`DB_ANALYTICS_*` -> `Analytics()`).

## Generation

### Generate connections only

```
forj generate:dbconns
```

### Generate all (recommended)

```
forj generate:all
```

`generate:all` runs all generators and can be filtered:

```
forj generate:all --only dbconns,model
```

## Lazy Connections

`Connections` resolves DB connections lazily, so non‑DB commands don’t require a live database. Each call resolves a cached connection instance.

## Migrations & Multiple Connections

Migrations can target multiple connections based on directory structure:

```
internal/migrations/2026_01_01_000001_create_users.up.sql       -> default
internal/migrations/analytics/2026_01_01_000001_add_events.up.sql -> analytics
```

Rules:

- Root migration directory targets the default connection.
- Subfolders map to named connections (e.g. `analytics` -> `DB_ANALYTICS_*`).

## Testing Notes

`connections.go` contains the shared plumbing; `connections_gen.go` is generated. Tests live alongside the package and verify env parsing, accessors, and config behavior.

# App Resource Shell Commands Design

Status:

- completed and implemented
- intended for generated App command surfaces
- first cut covers database and cache resources

## Purpose

This document defines a command model for opening local shell clients against
resources configured by a generated GoForj App.

The goal is to make common infrastructure access feel simple:

```bash
forj db
forj cache
```

while preserving GoForj's normal model:

- App-owned configuration
- generated App command surfaces
- explicit resource names
- driver-aware behavior
- local-first development
- clear failure boundaries

## Problem

Developers frequently need to inspect common App resources:

- a MySQL, MariaDB, Postgres, or SQLite database
- a Redis-backed cache store

Without a framework command, users need to remember:

- which driver the App is using
- which env keys define host, port, username, password, database, or DSN
- whether the service is reachable from the host machine or only through Docker
  Compose
- the exact client syntax for `mysql`, `psql`, `sqlite3`, or `redis-cli`

That makes a common local-development task feel more backend-specific than it
needs to be.

## Goals

- Provide short, memorable commands for common resource shells.
- Keep the canonical command shape aligned with existing GoForj command grammar.
- Resolve connection details from the App owner's configuration.
- Prefer host-machine client tools when available.
- Fall back to Docker Compose only when the local client executable is missing
  and the selected endpoint maps to a generated local service.
- Keep fallback behavior deterministic and inspectable.
- Support named App resources where the generated configuration supports them.
- Preserve the same command surface through `forj` delegation and built App
  binaries.
- Fail with actionable messages when configuration, client tools, or Compose
  services are unavailable.

## Non-Goals

- Do not create a generic hidden service locator.
- Do not make Docker Compose the default execution path.
- Do not silently hide connection failures by retrying through another method.
- Do not expose backend-specific commands as the canonical App surface.
- Do not require distributed infrastructure for local development.
- Do not implement full database administration tooling.
- Do not make these commands depend on unrelated App infrastructure.

## Command Shape

Use resource-first command names.

Canonical commands:

```bash
forj db:shell
forj cache:shell
```

Short aliases:

```bash
forj db
forj cache
```

The first cut does not expose `shell:db`, `shell:cache`, or other `shell:*`
aliases. Help and generated documentation use only the canonical
`resource:shell` names and their short resource aliases.

The canonical shape should remain `resource:shell` because existing GoForj
commands generally use resource-first grammar:

```bash
route:list
queue:work
schedule:run
make:model
auth:create-user
```

Avoid making backend names the canonical command:

```bash
forj mysql
forj postgres
forj redis
forj shell:mysql
```

The App has database and cache resources. MySQL, Postgres, SQLite, and Redis are
selected drivers behind those resources. If an App changes a resource driver,
the resource command name stays stable and dispatches to the new driver's shell
behavior when one is available.

## Resource Addressing

The command model must support Apps with more than one configured resource.

The default command chooses the default App resource:

```bash
forj db
forj cache
```

If multiple shellable resources exist and the user does not provide a resource
name, interactive commands may show a selection list:

```text
Select database connection

  default    mysql     mysql:3306/db
  reporting  postgres  postgres:5432/reports
```

Selection only runs when stdin and stdout are attached to a TTY. In
non-interactive contexts, the command always chooses the default resource. It
does not infer a lone shellable named resource. If the default is not shellable,
the command fails and requires an explicit resource name.

When a resource name is provided, the command should resolve that App-facing
resource name before choosing the backend client:

```bash
forj db reporting
forj db --connection reporting
forj db default
forj cache:shell sessions
forj cache:shell rate-limits
forj cache:shell default
```

`default` is an accepted explicit selector for both database connections and
cache stores.

Use the App resource name as the user-facing selector, not the backend service
name. For example, `sessions` is the cache resource even if it happens to use
Redis.

Resource names follow the generated named-resource model:

- databases: `default`, named database connections
- caches: `default`, named cache accessors

Queue and event resource selectors are deferred beyond the first cut.

The command fails clearly when the selected resource is not shellable:

```text
cache store sessions uses driver "memory"; no external shell is available
```

This keeps shell commands aligned with App resources while avoiding fake shells
for in-process drivers.

## Interactive Selection

Interactive selection is available when no resource name was provided and both
stdin and stdout are terminals.

Implemented behavior:

1. Build the resource list from generated App configuration.
2. Mark the default resource.
3. Filter or annotate resources that are not shellable.
4. Honor an explicit resource name, including `default`, without prompting.
5. In an interactive terminal, use the only shellable resource or show a compact
   selector when more than one is available.
6. Outside an interactive terminal, use only the default resource. If it is not
   shellable, fail instead of silently switching to a named resource.

The selection list should show enough context to choose quickly:

- resource name
- driver
- host or local path when safe to show
- whether the resource is default
- short reason when a resource is not shellable

Example:

```text
Select cache store

  sessions      redis    redis:6379/0
  rate-limits   redis    redis:6379/1
```

If the user selects a non-shellable resource, fail clearly:

```text
cache store default uses driver "memory"; no external shell is available
```

Selection flags:

- `--select` requests the interactive selector when a TTY is available.
- `--no-select` disables prompting and uses the default resource when no name is
  provided.

Do not prompt in CI, when stdin is not a TTY, or when stdout is not a TTY.
That safety rule also applies to `--select`; non-interactive execution remains
default-only.

## Ownership

These commands should be generated App commands, not native framework commands.

Inside a generated App:

```bash
forj db
```

should reach the App command through the existing source-aware delegation path.

For built binaries:

```bash
./bin/app db
```

should use the same command implementation and App environment.

This keeps command behavior tied to the App's actual generated configuration and
component selection.

## Implemented Commands

### `db:shell`

`db:shell` opens a shell client for the configured database.

Examples:

```bash
forj db
forj db reporting
forj db --connection reporting
forj db --method local
forj db --method compose
forj db --print
forj db --exec "select count(*) from users"
forj db -- --batch -e "select count(*) from users"
forj db reporting -- -c "select count(*) from users"
```

Implemented flags:

- optional `[connection]` selects a named database connection.
- `--connection <name>` selects a named database connection when an explicit
  flag reads better in scripts.
- `--select` opens an interactive connection selector when available.
- `--no-select` disables prompting and uses the default connection when no
  connection is provided.
- `--method auto|local|compose` chooses the launch method.
- `--print` prints the selected command with secrets masked and does not run it.
- `--exec <sql>` executes one SQL string instead of opening an interactive shell.
- arguments after `--` are passed directly to the selected database client after
  GoForj adds the configured connection arguments.
- `--verbose` prints method-selection details before launching.
- `--no-color` disables ANSI output in GoForj messages.

Connection names should map to generated database env scopes:

- default connection uses `DB_*`
- named connection uses `DB_<NAME>_*` with root fallback rules where the
  database configuration already supports them

`default` is accepted explicitly as either the positional connection or the
value of `--connection`.

Supported driver mappings:

| Driver | Local Client | Compose Service | Notes |
| --- | --- | --- | --- |
| `mysql` | `mysql` | `mysql` | Also acceptable for MariaDB-backed local services. |
| `mariadb` | `mysql` | `mysql` | Normalized to the MySQL client contract. |
| `postgres` | `psql` | `postgres` | `postgresql` should normalize to `postgres`. |
| `postgresql` | `psql` | `postgres` | Alias of `postgres`. |
| `sqlite` | `sqlite3` | none | Uses the configured database path or DSN. |
| `sqlite3` | `sqlite3` | none | Alias of `sqlite`. |

Database discovery and connection precedence match the generated runtime. A
selected `DB_DSN` or `DB_<NAME>_DSN` takes precedence over split connection
fields. MySQL DSNs are translated into the equivalent TCP or Unix-socket client
arguments, Postgres URL and keyword DSNs retain their target and non-secret
options, and SQLite uses the same DSN, configured path, and generated default
path order as the App runtime. An unusable DSN fails before launch rather than
silently selecting a different target. MySQL DSN options without a portable
equivalent across the supported MySQL and MariaDB clients, including TLS
profiles, fail rather than silently downgrading the connection.

### Database Shell Examples

Default interactive shell:

```bash
forj db
```

Named connection:

```bash
forj db reporting
forj db --connection reporting
```

Debug method selection:

```bash
forj db --print
forj db --method compose --print
forj db reporting --method local --print
```

Single SQL statement through the framework flag:

```bash
forj db --exec "select count(*) from users"
forj db reporting --exec "select count(*) from events"
```

Client-native passthrough after `--`:

```bash
forj db -- --batch -e "select count(*) from users"
forj db reporting -- -c "select count(*) from events"
forj db --connection reporting -- -c "select now()"
```

The wrapper adds configured connection arguments first and then appends
passthrough client arguments. This keeps configuration App-owned while still
allowing native `mysql`, `psql`, and `sqlite3` flags.

### `cache:shell`

`cache:shell` opens a shell client for a shellable cache resource.

Examples:

```bash
forj cache:shell
forj cache:shell --select
forj cache:shell sessions
forj cache:shell rate-limits --print
```

The command resolves the selected cache resource and supports Redis-backed
stores with `redis-cli`.

First-cut mappings:

| Driver | Shell Behavior |
| --- | --- |
| `redis` | Open `redis-cli` using the selected cache resource connection details. |
| all other drivers | Report that no external shell is available. |

Without an explicit store name, non-interactive execution uses only the default
cache. It fails if that cache is not Redis-backed, even when exactly one named
Redis cache exists. Interactive selection may choose a shellable named cache;
scripts must name it explicitly.

### Deferred: `queue:shell`

`queue:shell` is not implemented in the first cut. Queue resources can use
backends whose meaningful inspection surfaces differ substantially, so queue
inspection remains in Lighthouse or backend-specific tools until that behavior
has a separate design.

Examples:

```bash
forj queue:shell
forj queue:shell critical
```

Potential mappings:

| Driver | Shell Behavior |
| --- | --- |
| `redis` | Open `redis-cli` for the selected queue backend. |
| `sqlite`, `mysql`, `postgres` | Open the matching database client for database-backed queues. |
| `sync`, `workerpool`, `null` | Report that no external shell is available. |
| `sqs`, `nats`, `rabbitmq` | Requires driver-specific design before exposing a shell command. |

These mappings remain possible future work; the implemented command surface does
not advertise `queue:shell`.

## Method Selection

The default method is `auto`.

For `auto`, use this order:

1. Try a local client executable on `PATH`.
2. If the executable is missing and the selected endpoint maps to a generated
   local service, try that Docker Compose service.
3. If neither method is available, print an actionable error.

Important rule:

If the local client executable exists but the connection fails, do not
automatically retry through Docker Compose.

A connection failure can indicate:

- wrong host or port
- service not running
- wrong credentials
- wrong database name
- network or TLS configuration mismatch

Falling through to another method would hide the real configuration problem.

Fallback should only happen for method availability problems, such as:

- local client executable not found
- Docker Compose unavailable when `--method compose` is selected
- expected Compose service missing

Both automatic fallback and explicit `--method compose` refuse external or
opaque endpoints. A Compose launch must preserve the selected resource identity;
it cannot redirect an external connection to a similarly typed local service.

## Output Model

Default successful interactive mode should be quiet.

This should open the client without extra chatter:

```bash
forj db
```

When a selector is shown, the selector is the only pre-launch UI. After
selection, the command should either launch the client directly or print one
concise verbose line if `--verbose` is set.

Verbose mode may print one concise selection line:

```text
Opening database default via docker compose service mysql.
```

`--print` should show a runnable command with secrets masked:

```text
MYSQL_PWD='***' mysql -h mysql -P 3306 -u user db
```

Errors should name the failing boundary:

```text
mysql client not found; trying docker compose service mysql
```

Fallback attempts are printed only with `--verbose`.

Final failures should be specific:

```text
mysql client not found; docker compose fallback unavailable: docker compose service mysql is not configured
```

```text
missing DB_USERNAME
```

```text
psql exited with code 2
```

## Security

Do not print secrets by default.

Rules:

- `--print` must mask passwords, tokens, and DSN secrets.
- verbose output must not include raw passwords.
- client passwords are passed through client-specific environment variables:
  `MYSQL_PWD`, `PGPASSWORD`, `PGSSLPASSWORD`, and `REDISCLI_AUTH`.
- generated values replace inherited variables of the same name so stale
  credentials cannot win by environment ordering.
- Compose uses `docker compose exec -e NAME` and supplies the value through the
  child environment, keeping the secret itself out of Docker's argument list.
- Postgres passwords are removed from URL and keyword DSNs before the sanitized
  DSN is placed in arguments.

Environment variables are not a universal secret store; operators must still
apply the process and container visibility controls appropriate to their
platform. The first-cut guarantee is that GoForj does not echo raw credentials
or deliberately place them in process arguments when a supported client
environment variable exists.

## Environment Resolution

The command should use the same env-loading behavior as other generated App
commands.

Database resolution mirrors generated database connection behavior:

- read the selected connection's scoped env values
- fall back to root `DB_*` values where the database configuration already does
  that
- normalize driver names before choosing the client
- honor DSN precedence and translate the same target into client-native arguments
- report missing required keys before launching a client

`cache:shell` resolves a selected cache from generated `CACHE_*` and
`CACHE_<NAME>_*` configuration, normalizes its active driver, and then chooses
the matching client behavior. A Redis cache may inherit shared `REDIS_*`
connection defaults under the normal Cache runtime rules, but those keys never
create a separate infrastructure command.

Do not open full App infrastructure just to build a shell command. These
commands should inspect configuration and launch an external process.

## Docker Compose Behavior

Compose fallback is only for generated local development services and uses
`docker compose exec`. The first cut does not use `docker compose run --rm` and
does not start stopped services on behalf of the user.

Compose commands use these shapes:

```bash
docker compose exec mysql mysql ...
docker compose exec postgres psql ...
docker compose exec redis redis-cli ...
```

Every resource-shell command preflights Docker Compose availability with:

```bash
docker compose version
```

Service names come from generated service conventions:

- `mysql`
- `postgres`
- `redis`

If the service is not present in `docker-compose.yml`, fail clearly.

Before Compose execution, validate that the effective endpoint from the
configured fields or DSN is the generated service name, `localhost`, or a
loopback address. Reject empty or external hosts, Unix sockets, service-file
targets, and other opaque DSN targets. This guard applies to both automatic
fallback and explicit `--method compose`.

Do not assume Compose exists for SQLite.

## Interactive Process Handling

Interactive shells preserve the terminal:

- attach stdin
- attach stdout
- attach stderr
- return the exact non-zero child exit status through the command error
- avoid wrapping the client in log output

The built App entrypoint recognizes that status-bearing error and exits with the
same code, so direct binaries and `forj`-delegated commands preserve client
failure status. Wrapper/configuration failures continue through the normal
GoForj fatal-error path.

## Generated Component Selection

Generate `db:shell` only when the App has a database component.

Generate `cache:shell` only when the App has Cache participation.

Do not generate commands for infrastructure providers. Redis may back a cache,
queue, event bus, storage resource, or local service, but that does not create a
`redis:shell` command. The resource owns its command and its configured driver
selects the client. `queue:shell` and other resource shells remain deferred.

## Relationship To `about`

`forj about` already exposes configured primitives.

That makes it the discovery command:

```bash
forj about
```

Resource shell commands should not duplicate the full `about` report. They
should focus on opening the requested client or explaining why they cannot.

## Relationship To Resource Registry

If the resource registry design is implemented, these commands should consume
the registry for resource discovery where it improves consistency.

Until then, command-specific env resolution is acceptable as long as it matches
generated component behavior.

Do not block this feature on a full resource registry.

## Testing Strategy

Unit tests should cover:

- driver-to-client mapping
- env resolution for default and named database connections
- env resolution for default and named cache resources
- resource selector behavior for one resource, many resources, and
  non-interactive mode
- command argument construction
- password masking
- method selection when local client exists
- method selection when local client is missing
- Compose service detection
- rejection of external and opaque Compose targets
- DSN target parity and credential extraction
- exact child exit-code propagation
- failure messages for missing config

Rendered App tests should cover:

- MySQL render includes `db:shell`
- Postgres render includes `db:shell`
- SQLite render includes `db:shell`
- Redis-backed cache render includes `cache:shell`
- Redis-backed resources do not expose `redis:shell`
- Apps without databases do not expose `db:shell`

Integration tests may cover:

- `db --print` for each database driver
- `cache --print` for each shellable cache driver
- Compose fallback command construction

Full interactive client sessions do not need broad integration coverage. The
important framework behavior is resolution, selection, command construction, and
exit-code forwarding.

## Implementation Notes

Implemented package placement:

- `templates/internal/cmd/db_shell_cmd.go.tmpl`
- `templates/internal/cmd/cache_shell_cmd.go.tmpl`
- `templates/internal/cmd/command_exit_code.go.tmpl` for the shared App boundary

Keep this in `internal/cmd` unless a resource-specific package already owns the
workflow strongly enough to justify placement there.

Command constructors should be cheap and should not dial infrastructure.

Use external process execution with terminal streams attached. Avoid shelling
through `bash -c` when direct `exec.Command` argument construction is practical.

## Resolved Decisions

- Do not add `shell:*` aliases. Use `db:shell` and `cache:shell`, plus their
  short resource aliases.
- Accept `default` as an explicit database or cache selector.
- Keep provider names behind resource commands. A Redis-backed cache is opened
  through `cache:shell <name>`, not an infrastructure-specific command.
- Use only `docker compose exec`, and only when the selected endpoint maps to
  the generated local service.
- Defer `queue:shell` until queue inspection has a driver-specific contract.

## Implemented First Cut

The completed first cut provides:

```bash
forj db
forj db:shell
forj cache
forj cache:shell
```

with:

- `--method auto|local|compose`
- `--print`
- `--verbose`
- `--exec`
- passthrough client args after `--`
- `--select`
- `--no-select`
- database `--connection`
- cache optional `[name]`

It uses the local client first, falls back to Compose only when the client
executable is missing and the endpoint is local-mapped, preserves exact child
status, and reports real connection/configuration failures without retrying
through another method.

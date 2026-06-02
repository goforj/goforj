# App Resource Shell Commands Design

Status:

- proposed
- intended for generated App command surfaces
- focused first on database and Redis-backed resources

## Purpose

This document defines a command model for opening local shell clients against
resources configured by a generated GoForj App.

The goal is to make common infrastructure access feel simple:

```bash
forj db
forj redis
```

while preserving GoForj's normal model:

- App-owned configuration
- generated App command surfaces
- explicit resource names
- driver-aware behavior
- local-first development
- clear failure boundaries

## Problem

Developers frequently need to inspect common local services:

- a MySQL, MariaDB, Postgres, or SQLite database
- Redis used by queues, cache, events, or local development services

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
- Fall back to Docker Compose only when the local client executable is missing.
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
forj redis:shell
```

Short aliases:

```bash
forj db
forj redis
```

Optional discoverability aliases may be supported:

```bash
forj shell:db
forj shell:redis
```

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
forj shell:mysql
```

The App has a database resource. MySQL, Postgres, and SQLite are selected
drivers behind that resource. If the App changes drivers, `forj db` should keep
working.

## Resource Addressing

The command model must support Apps with more than one configured resource.

The default command should choose the default App resource:

```bash
forj db
forj redis
```

If multiple shellable resources exist and the user does not provide a resource
name, interactive commands may show a selection list:

```text
Select database connection

  default    mysql     mysql:3306/db
  reporting  postgres  postgres:5432/reports
```

Selection should only run when stdin and stdout are attached to a TTY. In
non-interactive contexts, the command should choose the default resource unless
the user passes a flag that requires explicit selection.

When a resource name is provided, the command should resolve that App-facing
resource name before choosing the backend client:

```bash
forj db reporting
forj db --connection reporting
forj cache:shell sessions
forj cache:shell rate-limits
forj queue:shell critical
```

Use the App resource name as the user-facing selector, not the backend service
name. For example, `sessions` is the cache resource even if it happens to use
Redis, and `critical` is the queue resource even if its backend queue name is
`production-critical-jobs`.

Resource names should follow the generated named-resource model:

- databases: `default`, named database connections
- caches: `default`, named cache accessors
- queues: `default`, named queue resources
- events: `default`, named event buses if generated support exists

The command should fail clearly when the selected resource is not shellable:

```text
cache sessions uses memory driver; no external shell is available
```

This keeps shell commands aligned with App resources while avoiding fake shells
for in-process drivers.

## Interactive Selection

Interactive selection is allowed when a command has more than one possible
resource and no resource name was provided.

Suggested behavior:

1. Build the resource list from generated App configuration.
2. Mark the default resource.
3. Filter or annotate resources that are not shellable.
4. If exactly one shellable resource exists, use it without prompting.
5. If multiple shellable resources exist and the terminal is interactive, show a
   compact selection list.
6. If multiple shellable resources exist and the terminal is not interactive,
   use the default resource.

The selection list should show enough context to choose quickly:

- resource name
- driver
- host or local path when safe to show
- whether the resource is default
- short reason when a resource is not shellable

Example:

```text
Select cache resource

  default       memory   no external shell
  sessions      redis    redis:6379/0
  rate-limits   redis    redis:6379/1
```

If the user selects a non-shellable resource, fail clearly:

```text
cache default uses memory driver; no external shell is available
```

Suggested flags:

- `--select` always opens the interactive selector when available.
- `--no-select` disables prompting and uses the default resource when no name is
  provided.
- `--json` may be added later for machine-readable resource discovery, but
  `forj about --json` is the current discovery path.

Do not prompt in CI, when stdin is not a TTY, or when stdout is not a TTY.

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

## First Commands

## `db:shell`

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

Suggested flags:

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

Supported driver mappings:

| Driver | Local Client | Compose Service | Notes |
| --- | --- | --- | --- |
| `mysql` | `mysql` | `mysql` | Also acceptable for MariaDB-backed local services. |
| `mariadb` | `mysql` or `mariadb` | `mysql` | Prefer an available compatible client. |
| `postgres` | `psql` | `postgres` | `postgresql` should normalize to `postgres`. |
| `postgresql` | `psql` | `postgres` | Alias of `postgres`. |
| `sqlite` | `sqlite3` | none | Uses the configured database path or DSN. |
| `sqlite3` | `sqlite3` | none | Alias of `sqlite`. |

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

## `redis:shell`

`redis:shell` opens `redis-cli` for the configured Redis endpoint.

Examples:

```bash
forj redis
forj redis --method local
forj redis --method compose
forj redis --db 1
forj redis --print
forj redis --exec PING
```

Suggested flags:

- `--method auto|local|compose` chooses the launch method.
- `--db <n>` overrides the selected Redis logical database.
- `--print` prints the selected command with secrets masked and does not run it.
- `--exec <command...>` runs one Redis command instead of opening an interactive
  shell.
- `--verbose` prints method-selection details before launching.
- `--no-color` disables ANSI output in GoForj messages.

The initial Redis command may resolve the shared Redis env keys:

- `REDIS_HOST`
- `REDIS_PORT`
- `REDIS_PASSWORD`
- `REDIS_DB`

Later versions may support selecting Redis-backed App resources, such as a
specific cache, queue, or events transport, when those resources expose distinct
Redis endpoints.

## `cache:shell`

`cache:shell` opens a shell client for a shellable cache resource.

Examples:

```bash
forj cache:shell
forj cache:shell --select
forj cache:shell sessions
forj cache:shell rate-limits --print
```

The command should resolve the selected cache resource and then choose the
backend client based on its driver.

Initial useful mappings:

| Driver | Shell Behavior |
| --- | --- |
| `redis` | Open `redis-cli` using the selected cache resource connection details. |
| `memcached` | Open a practical Memcached client if one is available, or fail with guidance. |
| `sqlite`, `mysql`, `postgres` | Open the matching database client when the cache driver uses a database backend and exposes enough connection details. |
| `memory`, `file`, `null` | Report that no external shell is available. |

This command should not pretend that every cache backend has a meaningful
interactive shell. It should be useful when a resource maps to external
infrastructure and explicit when it does not.

## `queue:shell`

`queue:shell` may be useful for queue resources backed by shellable
infrastructure, but it should not be part of the first implementation unless the
driver-specific behavior is clear.

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

The first cut should avoid presenting queue shelling as universal. Queue
inspection may be better served by Lighthouse or queue-specific admin commands.

## Method Selection

The default method is `auto`.

For `auto`, use this order:

1. Try a local client executable on `PATH`.
2. If the executable is missing, try a generated Docker Compose service.
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
mysql -h mysql -P 3306 -u user -p'***' db
```

Errors should name the failing boundary:

```text
mysql client not found; trying docker compose service mysql
```

Only print fallback attempts in default mode when the first method is
unavailable and the user would otherwise see a pause or surprising behavior.

Final failures should be specific:

```text
mysql client not found and docker compose service mysql is not available
```

```text
missing DB_PASSWORD for database connection default
```

```text
psql exited with code 2
```

## Security

Do not print secrets by default.

Rules:

- `--print` must mask passwords, tokens, and DSN secrets.
- verbose output must not include raw passwords.
- client commands may pass secrets through environment variables when the client
  supports it, such as `PGPASSWORD`.
- avoid placing passwords in process arguments when a practical client-specific
  environment option exists.

For MySQL-compatible clients, fully hiding the password from process listings is
harder because `mysql` commonly accepts `-p...`. Prefer the most practical
client option available, but never echo the raw value in GoForj output.

## Environment Resolution

The command should use the same env-loading behavior as other generated App
commands.

Database resolution should mirror generated database connection behavior:

- read the selected connection's scoped env values
- fall back to root `DB_*` values where the database configuration already does
  that
- normalize driver names before choosing the client
- report missing required keys before launching a client

Redis resolution should start with shared `REDIS_*` keys.

Resource-specific commands should use generated named-resource configuration
before falling back to shared connection settings. For example, a named cache
resource should resolve `CACHE_<NAME>_*` driver and connection settings before
using root cache or shared Redis configuration.

Do not open full App infrastructure just to build a shell command. These
commands should inspect configuration and launch an external process.

## Docker Compose Behavior

Compose fallback is only for generated local development services.

Suggested Compose commands:

```bash
docker compose exec mysql mysql ...
docker compose exec postgres psql ...
docker compose exec redis redis-cli ...
```

Implementation should detect Docker Compose availability with:

```bash
docker compose version
```

Service names should come from generated service conventions:

- `mysql`
- `postgres`
- `redis`

If the service is not present in `docker-compose.yml`, fail clearly.

Do not assume Compose exists for SQLite.

## Interactive Process Handling

Interactive shells should preserve the terminal:

- attach stdin
- attach stdout
- attach stderr
- forward exit status from the child process
- avoid wrapping the client in log output

This is similar to the behavior expected from delegated App commands that own
the terminal.

## Generated Component Selection

Generate `db:shell` only when the App has a database component.

Generate `redis:shell` when the App has a Redis-relevant component or when the
rendered env includes Redis configuration. Initial candidates:

- jobs with Redis queue support
- Redis cache support
- Redis events support
- explicit Docker Redis service

If Redis env keys are present but no component currently uses Redis, the command
may still be useful for local development, but the first implementation should
avoid over-generating commands with no App-owned resource.

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
- failure messages for missing config

Rendered App tests should cover:

- MySQL render includes `db:shell`
- Postgres render includes `db:shell`
- SQLite render includes `db:shell`
- Redis-backed render includes `redis:shell`
- Redis-backed cache render includes `cache:shell`
- Apps without databases do not expose `db:shell`

Integration tests may cover:

- `db --print` for each database driver
- `redis --print`
- Compose fallback command construction

Full interactive client sessions do not need broad integration coverage. The
important framework behavior is resolution, selection, command construction, and
exit-code forwarding.

## Implementation Notes

Suggested package placement:

- `templates/internal/cmd/db_shell_cmd.go.tmpl`
- `templates/internal/cmd/redis_shell_cmd.go.tmpl`
- `templates/internal/cmd/cache_shell_cmd.go.tmpl`
- small shared helpers under `templates/internal/cmd/resource_shell.go.tmpl`

Keep this in `internal/cmd` unless a resource-specific package already owns the
workflow strongly enough to justify placement there.

Command constructors should be cheap and should not dial infrastructure.

Use external process execution with terminal streams attached. Avoid shelling
through `bash -c` when direct `exec.Command` argument construction is practical.

## Open Questions

- Should `shell:db` and `shell:redis` be supported as aliases, or should help
  text and docs rely only on `db` and `redis`?
- Should `db:shell --connection` use `default` as an explicit accepted value?
- Should `redis:shell` accept resource references such as
  `--resource cache:sessions`, or should `cache:shell sessions` remain the only
  resource-specific path?
- Should Compose fallback use `docker compose exec` only, or should it support
  `docker compose run --rm` for stopped services?
- Should `queue:shell` ship in the first implementation, or should queue
  inspection remain in Lighthouse until driver-specific admin behavior is
  clearer?

## Recommended First Cut

Implement:

```bash
forj db
forj db:shell
forj redis
forj redis:shell
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
- Redis `--db`
- cache optional `[name]`

Use local client first, Compose only when the client executable is missing, and
clear errors for real connection failures.

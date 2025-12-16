# Migrations - Versioned Database Changes

GoForj includes first-class support for versioned database migrations. The goal is predictable, auditable changes you can run in dev, CI, and production.

## What migrations are

- Ordered, versioned database changes with up/down semantics.
- Stored alongside your project; tracked so reruns are idempotent.
- Designed to be explicit: no hidden schema drift or runtime magic.

## Generating migrations

Use the CLI generator (example name shown; adjust to your convention):

```bash
forj make:migration add_users_table
```

This creates a versioned migration file (SQL or Go, depending on your templates) in your migrations directory (commonly `internal/migrations`).

## Where migrations live (and how they ship)

- Default location: `internal/migrations` in your project.
- GoForj embeds migrations into the built binary, so you can run `forj migrate` and `forj migrate:rollback` anywhere the binary is deployed (no extra files needed).

## Applying migrations

```bash
forj migrate           # apply all pending migrations
forj migrate --step 2  # apply only the next 2 migrations
```

- Runs pending migrations in order; `--step N` limits how many apply in this run.
- Up/down scripts are respected; rerunning is safe (already-applied migrations are skipped).
- Errors are surfaced immediately; fix and rerun.

## Rollbacks

If supported by your templates, you can roll back the last batch (or a limited number):

```bash
forj migrate:rollback           # roll back one batch
forj migrate:rollback --step 1  # roll back one migration
```

Use carefully in production; prefer forward fixes unless you are confident in reversibility.

## Environment and connections

- Use `.env` / `.env.host` to configure DB connection settings.
- If Docker + Database were selected, dev hooks include `docker-compose up -d` and a DB wait loop; ensure Docker is running before migrate.

## Best practices

- Keep migrations small and reversible when possible.
- Treat migration files as code: review, test, and commit them.
- Align application code changes with migrations in the same PR.
- Never edit applied migrations; create new ones for changes.

## Troubleshooting

- DB not reachable: check Docker is running (if using Docker), verify DSN in `.env`.
- Migration failed midway: fix the script and rerun; already-applied steps are skipped.
- Rollback missing: ensure down scripts exist if you plan to roll back; otherwise prefer forward fixes.

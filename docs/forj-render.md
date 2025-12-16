# `forj render` - Project Scaffolding

Render generates and refreshes your project scaffolds based on `.goforj.yml`. It is opinionated, explicit, and safe to rerun: existing files are skipped and reported.

## What it does

- Reads `.goforj.yml` in the project root.
- Generates component scaffolds (core, Docker, API/UI, Database, Scheduler, Jobs) into the correct packages.
- Creates env files (`.env`, `.env.host`) if missing.
- Runs `go mod tidy` and prints how many modules were resolved.
- Installs Wire and runs `wire` so `go run main.go` works immediately.
- Prints aligned counts: created vs skipped, only shows skipped when non-zero.

## Prerequisites

- Run `forj new` first to create `.goforj.yml`.
- Run from the project root (where `.goforj.yml` lives).
- Docker running if you selected Docker/Database.

## Basic usage

```bash
forj render
```

## Output you should see

- Section lines like `▸ Core Components Rendering    ✔ 11 files` with skips only when present.
- `go mod tidy` line with module count (e.g., `✔ 44 modules`).
- Wire install + generate step so `main.go` is runnable immediately.
- Final summary with created/skipped totals and next steps.

## Idempotency and skips

- If a file already exists, render reports it as skipped and leaves it untouched.
- Env files are created once; reruns keep your existing values.
- Re-running render after adding/removing components in `.goforj.yml` will create the new scaffolds while respecting existing files.

## What gets generated (by component)

- Core: `main.go`, root command wiring, logger, wire injectors, env files.
- Docker: `docker-compose.yml`; database-specific files when Database is selected.
- Web API: HTTP handlers and supporting files.
- Web UI: frontend scaffold if selected.
- Database: database setup and supporting files.
- Scheduler: scheduler stubs and wiring.
- Jobs: job stubs and wiring.

## Why Wire runs here

Running Wire during render ensures `go run main.go` works without requiring `forj dev` first. Wire install uses `go install github.com/google/wire/cmd/wire@latest` and executes `wire` in the `wire` directory.

## Troubleshooting

- Missing `.goforj.yml`: run `forj new` or ensure you are in the project root.
- Skipped everything: likely re-rendering an existing project; check totals and `.goforj.yml` components.
- Docker-related errors: ensure Docker is running; rerun render afterward.
- Wire errors: ensure `go` is on PATH; rerun render to reinstall and regenerate.

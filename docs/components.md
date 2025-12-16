# Components - What Gets Generated and Why

GoForj components are explicit, idempotent, and recorded in `.goforj.yml`. This page summarizes what each component produces, the dependencies it assumes, and how it affects dev hooks and render output.

## Core (always on)
- Generates `main.go`, CLI root commands, logger setup, wire injectors, env files (`.env`, `.env.host`).
- Runs `go mod tidy`, installs Wire, and generates wire code so `go run main.go` works.
- Dev watches: App (http:serve) and Wire regeneration.

## Docker
- Generates `docker-compose.yml`.
- Adds `pre_dev` task: `docker-compose up -d`.
- Adds `dev_down` task: `docker-compose down`.
- If Database is also selected, adds DB wait loop in `pre_dev`.
- Expect Docker daemon running for render/dev/down.

## Web API
- Generates HTTP handlers and supporting wiring.
- Dev watch: App (http:serve) with file exclusions (wgo flags) in `.goforj.yml`.

## Web UI
- Generates frontend scaffold (if templates exist).
- Dev watch: NPM dev (`npm run dev`) when `frontend/package.json` has a `dev` script.
- You must run `cd frontend && npm install` before editing UI.

## Database
- Generates database-related scaffolds (and Docker DB assets when paired with Docker).
- If paired with Docker, render adds DB wait loop to `pre_dev` to ensure DB is ready before app starts.

## Scheduler
- Generates scheduler stubs and wiring.
- Dev watch: scheduler runner (`go run main.go schedule:run`).

## Jobs
- Generates job stubs and wiring.
- Dev watch: job worker (`go run main.go queue:work`).

## Selection rules
- CLI is always selected and cannot be toggled off.
- Components can be toggled in the wizard (or edited later in `.goforj.yml`) and rerun with `forj render`; existing files are skipped and reported.

## Render output expectations
- Each component section reports created vs skipped counts; skips are shown only when non-zero.
- `go mod tidy` reports module count.
- Wire install + generate runs as part of render to keep `main.go` runnable.

## Dev loop impact
- `pre_dev`, `dev_watches`, and `dev_down` in `.goforj.yml` are derived from selected components.
- Docker selection adds up/down hooks; Database + Docker adds DB wait.
- Web UI selection plus a dev script adds an NPM watch.

## Safe reruns
- Rerun `forj render` after changing components; new files are added, existing files are left intact and reported as skipped.
- Keep `.goforj.yml` under version control to document component choices and hooks.

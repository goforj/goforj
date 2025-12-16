# Getting Started with GoForj

This guide is the happy path: install the CLI, create a project with the wizard, render scaffolds, and start the dev loop. Everything is explicit, production-minded, and ready to run.

## Prerequisites

- Go 1.23+ in PATH
- Optional but common: Docker running (for Docker/database components)
- The wizard installs Wire and wgo for you during render/dev; no manual setup needed.

## 1) Install the CLI

```bash
go install github.com/goforj/goforj/cmd/forj@latest
forj --version
```

## 2) Create a project with the wizard

```bash
forj new
```

Wizard stages:

- Project: human-readable name (required)
- Module: Go module import path (required)
- Components: CLI (always on), plus Docker, Web API, Web UI, Database, Scheduler, Jobs; shortcuts: a (all), c (clear), space (toggle)
- Path: must be empty or will be created; live validation and status; default is cwd if empty, otherwise cwd/slug
- Confirm: review and create `.goforj.yml` at the chosen path

Cancel anytime with Esc/Ctrl+C. CLI is always selected and cannot be toggled off.

## 3) Render scaffolds

```bash
cd <your-project-path>
forj render
```

What happens:

- Generates scaffolds for the selected components (core, Docker, API/UI, DB, scheduler, jobs)
- Writes env files if missing (`.env`, `.env.host`)
- Runs `go mod tidy` and prints module count
- Installs Wire and runs `wire` so `go run main.go` works immediately
- Prints created vs skipped counts and next steps

## 4) Start the dev loop

```bash
forj dev
```

Pre-dev tasks (recorded in `.goforj.yml`):

- Wire install + generate
- wgo install
- Docker compose up (if Docker component selected)
- Optional DB wait if Database + Docker

Dev watches:

- App (http serve)
- Wire (regenerate)
- Scheduler (if selected)
- Jobs (if selected)
- NPM dev (if Web UI selected and package.json has a dev script)

To tear down Docker when you are done:

```bash
forj down
```

## Project layout (after render)

```
.
├── .goforj.yml           # recorded components, hooks, watches
├── .env / .env.host      # environment defaults
├── cmd/forj/main.go      # CLI entry
├── internal/             # app code, commands, logger, etc.
├── wire/                 # Wire injectors + wire_gen.go
├── docker-compose.yml    # if Docker selected
└── frontend/             # if Web UI selected
```

## Guarantees and expectations

- `.goforj.yml` is always written to the chosen target path.
- Render is idempotent: existing files are skipped and reported.
- Wire is installed and generated during render so `go run main.go` works without extra steps.
- dev_down commands (docker-compose down) are only included when Docker is selected.

## Next steps

- Open `.goforj.yml` to see recorded hooks and watches.
- Customize components and re-run `forj render` to regenerate scaffolds safely.
- If you selected Web UI, run `cd frontend && npm install` before UI edits.
- Keep `.env` and `.env.host` up to date for your environment defaults.

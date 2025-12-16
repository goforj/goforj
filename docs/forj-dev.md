# `forj dev` and `forj down` - Development Loop

The dev loop is designed to be predictable and production-minded: pre-flight tasks run first, watches attach, and teardown is explicit when you are done.

## What `forj dev` does

- Runs pre-dev tasks from `.goforj.yml`:
  - Wire install + generate
  - wgo install
  - `docker-compose up -d` if Docker is selected
  - Database wait loop if Database + Docker are selected
- Starts file watches from `.goforj.yml`:
  - App (http serve)
  - Wire (regenerate on changes)
  - Scheduler (if selected)
  - Jobs (if selected)
  - NPM dev (if Web UI selected and package.json has a dev script)
- Streams logs so you can see each watcher’s output.

## What `forj down` does

- Runs `dev_down` tasks from `.goforj.yml`, typically `docker-compose down`.
- Only present when the Docker component was selected.

## Prerequisites

- `.goforj.yml` present in the project root.
- Docker running if Docker/Database components are enabled.

## Basic usage

```bash
forj dev   # start pre-dev tasks, watchers, and your app
forj down  # optional teardown, e.g., docker-compose down
```

## Where the tasks come from

`forj new` writes the default hooks into `.goforj.yml`:

- `pre_dev`: wire install/generate, wgo install, Docker up, DB wait.
- `dev_watches`: App, Wire, Scheduler, Jobs, NPM (based on selected components).
- `dev_down`: Docker down (only when Docker is selected).

You can adjust commands in `.goforj.yml` if needed; they are explicit shell commands.

## Behavior and guarantees

- Pre-dev tasks run before watches start; failures are surfaced in the console.
- Watchers rely on `wgo` and your local Go toolchain; ensure both are on PATH.
- `forj down` is safe to run after `forj dev` to stop Docker dependencies.

## Troubleshooting

- “No dev watches defined”: ensure `.goforj.yml` was written by `forj new` and you are in the project root.
- Docker errors: start Docker, then rerun `forj dev` (and `forj down` if needed).
- Wire errors: ensure `go` is on PATH; rerun `forj render` or `forj dev` to reinstall/regenerate.

# `forj new` - Project Wizard

This is the guided entry point to create a GoForj project. It reflects GoForj’s principles: explicit configuration, opinionated defaults, production-ready scaffolds, and no hidden magic.

## What it does

- Collects project name, Go module path, component selections, and target path.
- Validates required fields and ensures the target path is empty (or will be created).
- Writes `.goforj.yml` into the chosen path with components, hooks, and watches.
- Prepares render-ready state so `forj render` can run immediately.

## Prerequisites

- Go 1.23+ installed and in PATH.
- Docker running if you plan to select Docker/Database.
- A target path that is empty or can be created.

## Wizard stages (in order)

1) Project  
   - Human-friendly name (required).  
   - Preview: slug, suggested path.

2) Module  
   - Go module import path (required).  
   - Preview: full import path.

3) Components  
   - CLI (always selected, cannot be toggled).  
   - Optional: Docker, Web API, Web UI, Database, Scheduler, Jobs.  
   - Shortcuts: `a` select all, `c` clear (CLI stays on), space toggles current, arrows move.  
   - Footer shows shortcuts on this screen only.

4) Path  
   - Choose where to write the project.  
   - Live validation: must be a directory that is empty or does not yet exist.  
   - Defaults: current dir if empty, otherwise current dir plus slug.  
   - Shows resolved absolute path and status (exists/empty, will create, not empty).

5) Confirm  
   - Review name, module, components, and resolved path.  
   - Validation runs again before completion.

## Success output

- Writes `.goforj.yml` into the selected path.  
- Moves you to StageDone view, then you can run `forj render` from that path.

## Cancel behavior

- Esc or Ctrl+C at any stage quits cleanly without writing files.

## Path rules (explicit)

- Absolute or relative paths are accepted.  
- Relative paths resolve from the current working directory when you run `forj new`.  
- The target directory must be empty; if it does not exist, GoForj will create it.  
- The wizard blocks confirmation if the directory is non-empty or not a directory.

## What gets recorded in `.goforj.yml`

- `project_name`, `module_name`, `updated_at`
- `components` flags for CLI, Docker, Web API, Web UI, Database, Scheduler, Jobs
- `pre_dev` hooks: wire install/generate, wgo install, docker-compose up (if Docker), DB wait (if Docker + Database)
- `dev_down` hooks: docker-compose down (only if Docker selected)
- `dev_watches`: App (http serve), Wire, Scheduler, Jobs, NPM dev (if Web UI with a dev script)

## After the wizard

Run the standard flow from the project path:

```bash
forj render
forj dev
```

Render is idempotent: it reports created vs skipped files, runs go mod tidy with module counts, installs Wire, and generates wire code so `go run main.go` works immediately.

## Troubleshooting

- “Target path is not empty”: pick a new path or clear the directory; the wizard will not proceed otherwise.
- “.goforj.yml missing” after wizard: ensure you ran `forj new` in a directory you can write to and completed the Confirm step.
- Docker selected but daemon not running: the wizard completes, but pre_dev hooks in `forj dev` will need Docker up.

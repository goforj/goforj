# Context Index

Start here.

Do not read every file in `docs/context` by default. Pick the smallest set that matches the work.

## How To Use This Folder

When the prompt is "Let's work on X", read:

- 1 primary doc for the topic
- 1 adjacent doc if the change crosses a boundary
- 1 workflow doc if you expect render, smoke, or release validation

That should usually keep context to 2-3 files, not the whole folder.

## Topic Map

### App layout, `cmd/app`, `app/`, app composition, or multi-app routing

Read:

- [App Structure](app-structure.md)
- [Generated App Extension Points](generated-app-extension-points.md)

Add if the task touches runtime ownership, lifecycle, scheduler, jobs, or process startup:

- [Runtime Architecture](runtime-architecture.md)

Add if the task is about the full design history or unresolved architecture tradeoffs:

- [../designs/completed/app-composition-layout-design.md](../designs/completed/app-composition-layout-design.md)

### Rendering, templates, smoke apps, or `forj render`

Read:

- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)
- [Generated App Extension Points](generated-app-extension-points.md)

Add if sibling repos are involved:

- [Releasing Sibling Repos](releasing-sibling-repos.md)

Add if the issue mentions temp renders, `/tmp`, emitted app compile failures, or "works in GoForj but not after render":

- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)

### Runtime wiring, lifecycle, scheduler, jobs, or process ownership

Read:

- [Runtime Architecture](runtime-architecture.md)
- [Generated App Extension Points](generated-app-extension-points.md)

### Database connections, DSNs, migrations, or time zones

Read:

- [Generated Database README](../../templates/internal/database/README.md)
- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md) when changing generated behavior

### Auth, login, sessions, cookies, JWT, or future provider support

Read:

- [Auth](auth.md)
- [Generated App Extension Points](generated-app-extension-points.md)
- [Runtime Architecture](runtime-architecture.md)

### Web adapters, middleware, HTTP abstractions, or "should this live in `web`?"

Read:

- [Web Boundary](web-boundary.md)
- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)

### Sibling repo work: `console`, `web`, `queue`, `storage`, `cache`, `scheduler`

Read:

- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)
- [Releasing Sibling Repos](releasing-sibling-repos.md)

### CLI output, semantic messages, loaders, progress, prompts, or console layout

Read:

- [Console Output](console.md)
- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)

Add if the task changes the interactive `forj dev` experience:

- [../designs/completed/forj-dev-tui-design.md](../designs/completed/forj-dev-tui-design.md)

### Integration tests, rendered dependency boot, compose-driven testcontainers

Read:

- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)
- [Practical Workflows](practical-workflows.md)

### Logging, metrics, boot output, Lighthouse/runtime visibility

Read:

- [Observability](observability.md)
- [Runtime Architecture](runtime-architecture.md)
- [../designs/completed/metrics-design.md](../designs/completed/metrics-design.md)

Add if the issue mentions:

- per-surface metrics toggles
- Grafana panels or dashboard drift
- query fingerprints / slow queries
- vmagent scrape targets or metrics target generation
- noisy HTTP warm-up / `HEAD` probe behavior

Then prioritize:

- [Observability](observability.md)

### Starter kits, frontend app shells, or new-project kit selection

Read:

- [../designs/completed/starter-kits-design.md](../designs/completed/starter-kits-design.md)
- [Generated App Extension Points](generated-app-extension-points.md)
- [Auth](auth.md)

Add if the task is about React:

- [../designs/completed/react-starter-kit-design.md](../designs/completed/react-starter-kit-design.md)

Add if the task is about server-rendered templates, Blade-like rendering, htmx, or templ:

- [../designs/completed/templ-htmx-starter-kit-design.md](../designs/completed/templ-htmx-starter-kit-design.md)

### Queue shutdown behavior or worker-stop semantics

Read:

- [Queue Shutdown Behavior](queue-shutdown-behavior.md)
- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)

### Local development loop, rendered-app validation, or "what should I run?"

Read:

- [Practical Workflows](practical-workflows.md)
- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)

Add if the issue mentions dirty repo state, embedded assets, or inconsistent smoke results:

- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)

Add if the issue mentions `forj dev`, Ctrl+C, watcher shutdown, Docker Compose shutdown, or helper containers:

- [Practical Workflows](practical-workflows.md)
- [Observability](observability.md)

## File Guide

- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)
  - which repo should own a change
  - sibling repo roles
  - Lighthouse explorer backlog
- [Runtime Architecture](runtime-architecture.md)
  - generated app ownership lines
  - scheduler/runtime boundaries
- [App Structure](app-structure.md)
  - current `cmd/app`, `app/`, `app/wire`, and `internal/` ownership
  - default app vs additional app shape
  - command prefix routing for additional apps
- [Generated App Extension Points](generated-app-extension-points.md)
  - where app-level customizations should go
- [Generated Database README](../../templates/internal/database/README.md)
  - database environment, DSN, time zone, and migration behavior emitted into generated apps
- [Auth](auth.md)
  - current auth model
  - security invariants
  - future provider direction
  - user-facing package overview: [../../auth/README.md](../../auth/README.md)
- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)
  - render/smoke/integration workflow
  - compose-driven rendered dependency model
  - temp render smoke via `test:render`
  - repo cleanliness and embedded asset pitfalls
- [Releasing Sibling Repos](releasing-sibling-repos.md)
  - release and consume sibling repos cleanly
- [Console Output](console.md)
  - sibling package surface and global/instance API stance
  - console, logging, generated-app, and Bubble Tea ownership boundaries
- [Observability](observability.md)
  - log/metrics/boot-output model
  - per-surface metrics toggles
  - database fingerprinting/query-shape guidance
  - metrics target generation direction
  - polling HTTP warm-up behavior
- [../designs/completed/metrics-design.md](../designs/completed/metrics-design.md)
  - metrics primitive design
  - observability boundary and instrumentation coverage
- [../designs/completed/starter-kits-design.md](../designs/completed/starter-kits-design.md)
  - starter-kit model and wizard step
  - Vue-first official kit direction
  - ownership and render layering
- [Practical Workflows](practical-workflows.md)
  - day-to-day loops
  - common commands and pitfalls
- [Web Boundary](web-boundary.md)
  - what should move into `web`
- [Queue Shutdown Behavior](queue-shutdown-behavior.md)
  - queue timeout and shutdown ownership

## Anti-Pattern

Do not load the entire folder because a task is vaguely related to GoForj.

If the task is narrow, stay narrow.

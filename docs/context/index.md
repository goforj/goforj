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

### Rendering, templates, smoke apps, or `forj render`

Read:

- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)
- [Generated App Extension Points](generated-app-extension-points.md)

Add if sibling repos are involved:

- [Releasing Sibling Repos](releasing-sibling-repos.md)

### Runtime wiring, lifecycle, scheduler, jobs, or process ownership

Read:

- [Runtime Architecture](runtime-architecture.md)
- [Generated App Extension Points](generated-app-extension-points.md)

### Auth, login, sessions, cookies, JWT, or future provider support

Read:

- [Auth](auth.md)
- [Generated App Extension Points](generated-app-extension-points.md)
- [Runtime Architecture](runtime-architecture.md)

### Web adapters, middleware, HTTP abstractions, or "should this live in `web`?"

Read:

- [Web Boundary](web-boundary.md)
- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)

### Sibling repo work: `web`, `queue`, `storage`, `cache`, `scheduler`

Read:

- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)
- [Releasing Sibling Repos](releasing-sibling-repos.md)

### Integration tests, rendered dependency boot, compose-driven testcontainers

Read:

- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)
- [Practical Workflows](practical-workflows.md)

### Logging, metrics, boot output, Lighthouse/runtime visibility

Read:

- [Observability](observability.md)
- [Runtime Architecture](runtime-architecture.md)
- [../designs/metrics-design.md](../designs/metrics-design.md)

### Starter kits, frontend app shells, or new-project kit selection

Read:

- [../designs/starter-kits-design.md](../designs/starter-kits-design.md)
- [Generated App Extension Points](generated-app-extension-points.md)
- [Auth](auth.md)

### Queue shutdown behavior or worker-stop semantics

Read:

- [Queue Shutdown Behavior](queue-shutdown-behavior.md)
- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)

### Local development loop, rendered-app validation, or "what should I run?"

Read:

- [Practical Workflows](practical-workflows.md)
- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)

## File Guide

- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)
  - which repo should own a change
  - sibling repo roles
  - Lighthouse explorer backlog
- [Runtime Architecture](runtime-architecture.md)
  - generated app ownership lines
  - scheduler/runtime boundaries
- [Generated App Extension Points](generated-app-extension-points.md)
  - where app-level customizations should go
- [Auth](auth.md)
  - current auth model
  - security invariants
  - future provider direction
  - user-facing package overview: [../../auth/README.md](../../auth/README.md)
- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)
  - render/smoke/integration workflow
  - compose-driven rendered dependency model
- [Releasing Sibling Repos](releasing-sibling-repos.md)
  - release and consume sibling repos cleanly
- [Observability](observability.md)
  - log/metrics/boot-output model
- [../designs/metrics-design.md](../designs/metrics-design.md)
  - metrics primitive design
  - observability boundary and instrumentation coverage
- [../designs/starter-kits-design.md](../designs/starter-kits-design.md)
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

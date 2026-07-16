# Working In GoForj And Related Repos

This document is now a short orientation layer.

Use [Context Index](index.md) first and then load the smallest number of topic docs that match the work.

## Read This Instead

For most work, start with one of these:

- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)
- [Runtime Architecture](runtime-architecture.md)
- [Generated App Extension Points](generated-app-extension-points.md)
- [Rendering And Smoke Workflow](rendering-and-smoke-workflow.md)
- [Practical Workflows](practical-workflows.md)
- [Releasing Sibling Repos](releasing-sibling-repos.md)
- [Console Output](console.md)
- [Observability](observability.md)
- [Web Boundary](web-boundary.md)
- [Queue Shutdown Behavior](queue-shutdown-behavior.md)

## Durable Summary

Keep these high-level rules in mind:

- the rendered app is a smoke target, not the source of truth
- `goforj` owns app policy, generation, templates, and developer workflow
- sibling repos should own reusable primitives instead of pushing everything back into GoForj
- `console` owns reusable line-oriented presentation while GoForj owns command semantics and interactive dev TUI state
- for future observability work, keep `metrics` as the concrete primitive and treat `observability` as the broader subsystem concept
- `internal/runtime` is the generated root runtime package
- scheduler runtime/bootstrap, schedule registration, and Lighthouse/operator glue are separate concerns and should stay separate
- when a fix should survive rerender, it belongs in GoForj source or a sibling repo, not only in the rendered app
- use semantic commit messages for GoForj changes
- do not add defensive nil checks for services or commands that are explicitly wired through DI; fix the wiring instead

## Why This File Is Short Now

The previous version of this file had grown into a catch-all document.

That made retrieval worse because a narrow task could drag in repo roles, rendering rules, runtime boundaries, release workflow, and stale status notes all at once.

The folder is now split so agents can read only the files that match the work.

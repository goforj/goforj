# Working In GoForj And Related Repos

This document is now a short orientation layer.

Use [Context Index](/workspace/code/goforj/docs/context/index.md) first and then load the smallest number of topic docs that match the work.

## Read This Instead

For most work, start with one of these:

- [Repo Boundaries And Ownership](/workspace/code/goforj/docs/context/repo-boundaries-and-ownership.md)
- [Runtime Architecture](/workspace/code/goforj/docs/context/runtime-architecture.md)
- [Generated App Extension Points](/workspace/code/goforj/docs/context/generated-app-extension-points.md)
- [Rendering And Smoke Workflow](/workspace/code/goforj/docs/context/rendering-and-smoke-workflow.md)
- [Practical Workflows](/workspace/code/goforj/docs/context/practical-workflows.md)
- [Releasing Sibling Repos](/workspace/code/goforj/docs/context/releasing-sibling-repos.md)
- [Observability](/workspace/code/goforj/docs/context/observability.md)
- [Web Boundary](/workspace/code/goforj/docs/context/web-boundary.md)
- [Queue Shutdown Behavior](/workspace/code/goforj/docs/context/queue-shutdown-behavior.md)

## Durable Summary

Keep these high-level rules in mind:

- the rendered app is a smoke target, not the source of truth
- `goforj` owns app policy, generation, templates, and developer workflow
- sibling repos should own reusable primitives instead of pushing everything back into GoForj
- `internal/app` is the generated root runtime package
- scheduler runtime/bootstrap, schedule registration, and Lighthouse/operator glue are separate concerns and should stay separate
- when a fix should survive rerender, it belongs in GoForj source or a sibling repo, not only in the rendered app

## Why This File Is Short Now

The previous version of this file had grown into a catch-all document.

That made retrieval worse because a narrow task could drag in repo roles, rendering rules, runtime boundaries, release workflow, and stale status notes all at once.

The folder is now split so agents can read only the files that match the work.

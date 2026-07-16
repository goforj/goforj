# Console Output

This document describes the boundary between GoForj and
`github.com/goforj/console`.

## Current Package

`console` is the reusable, line-oriented output toolkit used by the GoForj CLI.
It owns:

- semantic action, info, success, warning, error, fatal, and debug output
- coordinated stdout and stderr writers
- ANSI, Unicode, terminal-width, wrapping, padding, and truncation policy
- sections, rules, key/value output, lists, boxes, tables, and trees
- line-oriented prompts, confirmation, choices, and secret input
- loaders and determinate progress with redirect-safe output

It deliberately does not own:

- structured application logging
- command parsing or command semantics
- subprocess execution
- full-screen TUI state or event loops
- GoForj lifecycle and build orchestration

The package stays small enough to use for ordinary command output without
requiring an application to adopt Bubble Tea or another TUI framework.

## Global And Instance APIs

Most operations are available in both forms:

- package helpers such as `console.Success`, `console.RenderTable`, and
  `console.Confirm` use the process-wide default console
- methods on `*console.Console` provide the same behavior with isolated writers,
  input, terminal hooks, environment policy, and exit behavior

Prefer package helpers for normal GoForj CLI output. Construct an instance when
one operation needs isolated streams or policy, especially in tests and build
progress. Loader and progress construction use `console.NewLoader` and
`console.NewProgress` globally, and `Console.Loader` and `Console.Progress` on an
instance.

## GoForj Ownership

GoForj uses the sibling package for reusable presentation behavior, but keeps
framework policy local.

`console` owns:

- marks, colors, text measurement, layout, prompts, loaders, and progress
- coordination between durable writes and a transient console line
- TTY, redirect, CI, ANSI, and Unicode presentation behavior

`goforj` owns:

- what each command means and which messages it emits
- build, render, test, and project-creation orchestration
- the private `__FORJ_BUILD_PROGRESS__` protocol consumed by `forj dev`
- Bubble Tea model state, key handling, filtering, and viewport behavior
- structured application and Lighthouse logging

The private build-progress protocol is not a general console feature. It is an
integration boundary between GoForj's build pipeline and dev TUI.

## Current Integration

Framework commands and supporting packages import `github.com/goforj/console`
directly. Important examples include:

- the root `forj` CLI and command diagnostics
- project creation and its loader
- interactive build progress
- render-check and scenario output
- dev transcript marks, colors, and debug messages

The build pipeline constructs a dedicated `Console` whose stdout and stderr both
target the status stream. That keeps transient build progress isolated from
machine-readable command output.

The `forj dev` Bubble Tea implementation may consume console marks, colors, and
text utilities, but Bubble Tea remains the owner of the interactive terminal
experience.

## Generated App Boundary

Generated applications currently retain their generated `internal/console`
package. That source belongs to the rendered application and is separate from
GoForj's former framework-internal console package.

Do not assume a new sibling-package feature automatically exists in rendered
apps. Moving generated applications onto `github.com/goforj/console` would be a
separate dependency and template migration requiring render and compatibility
coverage.

## Package Work

For package ownership and release validation, also read:

- [Repo Boundaries And Ownership](repo-boundaries-and-ownership.md)
- [Releasing Sibling Repos](releasing-sibling-repos.md)
- [Console Package Handoff](../designs/completed/console-package-handoff.md)

The [sibling package README](https://github.com/goforj/console#readme) is
generated from source examples. API changes are not complete until
implementation, examples, generated README output, and global/instance parity
are reconciled.

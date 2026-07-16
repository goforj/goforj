# forj Dev TUI Design

## Purpose

This document captures the intended terminal model for `forj dev`.

The goal is to keep `forj dev` feeling like a normal streaming terminal command
while still supporting a small amount of stable operator UI:

- a persistent footer for hotkeys and runtime state
- a transient build-status line
- clear restart/build separators in the transcript

This is deliberately not a full-screen TUI.

## Design Goals

- Preserve ordinary watcher and app logs as normal transcript output.
- Keep hotkey and runtime hints visible without polluting the transcript.
- Show build progress without permanently spamming the log stream.
- Leave the terminal in a clean state on exit, restart, or failure.
- Keep the implementation understandable enough to maintain.

## Non-Goals

- Turning `forj dev` into a full-screen terminal application.
- Replacing ordinary logs with TUI widgets.
- Hiding runtime output behind panes, tabs, or scrollback abstractions.
- Making every transient state part of the persistent footer.

## Current Terminal Model

`forj dev` has three distinct output surfaces:

1. Transcript output
2. Sticky footer
3. Transient build-status line

These surfaces have different responsibilities and should stay separate.

### Transcript Output

Transcript output is the normal scrolling log stream.

It includes:

- watcher output
- app stdout/stderr
- restart separators
- final build completion lines

Transcript output must remain readable when copied, saved, or reviewed after the
fact.

### Sticky Footer

The sticky footer is a persistent bottom-of-terminal UI surface managed by the
dev TUI layer.

It is intended for:

- hotkey hints
- debug level state
- high-level runtime state

It is not intended for:

- build progress animation
- restart separators
- watcher log lines

### Transient Build-Status Line

Build status is intentionally separate from the footer.

It appears above the footer while a build is in progress and may be redrawn
transiently. On completion, the final status can be emitted into the transcript.

This gives us live feedback during builds without overloading the persistent
footer with short-lived state.

## Architectural Boundaries

The key implementation rule is to keep ownership explicit.

### `internal/forj/dev_tui.go`

This layer should own:

- footer lifecycle
- footer redraw logic
- transient build-status redraw logic
- terminal cursor/state restoration

It should not own:

- parsing watcher semantics
- build pipeline step orchestration

### `internal/forj/devwatch_streamer.go`

This layer should own:

- watcher output handling
- restart separator emission
- interception of internal build progress markers
- translation of those markers into dev TUI build-status updates

It should not collapse all output into footer rendering.

### `internal/build/pipeline.go`

This layer should own:

- build step execution
- direct standalone build progress rendering for `forj build`
- emission of machine-readable progress markers when running under `forj dev`

It should not need to understand footer rendering.

## Direct Build vs Dev Loop

The build pipeline behaves differently depending on the command context.

### `forj build`

Direct build mode may render its own animated progress line when attached to a
TTY.

Desired behavior:

- animated progress while steps run
- compact success/failure result
- minimal transcript noise

### `forj dev`

In dev mode, the build pipeline should emit internal progress markers instead of
drawing directly to the terminal.

The dev streamer then translates those markers into a transient build-status
surface controlled by the dev TUI layer.

Desired behavior:

- no raw internal progress markers in transcript output
- no direct build renderer fighting the footer renderer
- one coherent terminal owner for dev-mode transient UI

## Restart Semantics

Runtime restarts should remain transcript events, not footer events.

Important restart moments:

- render/build start
- runtime shutdown
- runtime start
- watcher-triggered rebuild/restart cycles

These should be visible as ordinary labeled transcript separators so the operator
has an audit trail of what happened and when.

## State Model

The implementation should treat these as separate state channels:

- transcript state
- footer state
- build-status state

Each channel should have a narrow API.

For example:

- transcript writes append lines
- footer writes redraw the reserved footer region
- build-status writes redraw a transient line above the footer

The more these channels are mixed together, the more fragile the terminal
behavior becomes.

## Failure and Exit Behavior

A correct TUI implementation must restore the terminal cleanly.

Required behaviors:

- stop redraw loops on shutdown
- restore cursor visibility/state if modified
- avoid leaving partial transient lines in the prompt area
- avoid corrupting shell prompt placement after Ctrl+C

Failure-path cleanliness matters as much as success-path polish here.

## Practical Rules For Future Changes

If this area is touched again, preserve these rules:

1. Do not route build progress into the sticky footer.
2. Do not emit internal build markers into the transcript.
3. Keep watcher and app logs as ordinary transcript lines.
4. Keep restart markers in the transcript.
5. Test in a real terminal, not only through unit tests.
6. Prefer explicit helper methods over clever cursor tricks.

## Known Complexity Hotspot

`devFooterWriter` is currently the main complexity hotspot because it spans both:

- sticky footer behavior
- transient build-status behavior

That coupling is manageable for now, but it is the first place that should be
split if the terminal behavior grows any further.

## Next Cleanup Direction

If we continue investing in this area, the next cleanup should be:

1. Separate sticky footer rendering from transient build-status rendering.
2. Tighten the contract between the build pipeline and the dev streamer.
3. Keep transcript emission paths fully independent from footer redraw paths.
4. Add a small set of manual verification scenarios for terminal behavior.

## Manual Verification Checklist

Changes in this area should be checked in a real terminal for:

- initial `forj dev` startup
- first build
- watcher-triggered rebuild
- runtime restart
- build failure
- Ctrl+C shutdown
- terminal resize if relevant to the change

## Summary

The correct mental model for `forj dev` is:

- a normal transcript-first CLI command
- with a small persistent footer
- and a separate transient build-status surface

That separation is what keeps the implementation understandable and the terminal
output usable.

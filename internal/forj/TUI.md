# forj Dev TUI Notes

This file documents the current `forj dev` terminal model and the main constraints
behind it. It is intentionally colocated with the TUI code in this package.

## Current Model

`forj dev` is not a full-screen TUI. It has three terminal behaviors layered together:

1. Normal transcript output
2. A sticky bottom footer for hotkeys/runtime state
3. A transient inline build status line above the footer

The relevant code lives mainly in:

- `internal/forj/dev_tui.go`
- `internal/forj/devwatch_streamer.go`
- `internal/build/pipeline.go`

## Footer

The footer is managed by `devFooterWriter`.

Responsibilities:

- keep the hotkey footer pinned at the bottom of the transcript
- redraw the footer after normal output lines
- update footer content when runtime state changes

Important constraint:

- the footer is a persistent UI surface
- normal app/watcher logs must remain part of the transcript

## Build Status

Build progress is handled differently in direct build mode vs `forj dev`.

### Direct `forj build`

`internal/build/pipeline.go` renders a one-line animated loader directly to the terminal
when stderr is a TTY and `FORJ_BUILD_PROGRESS` is not set.

Behavior:

- animated green braille spinner during build steps
- final green check on success
- no extra transcript spam

### `forj dev`

`Build App` sets `FORJ_BUILD_PROGRESS=1`, so the build pipeline emits internal progress
markers instead of drawing directly to the terminal.

`internal/forj/devwatch_streamer.go` intercepts those markers and drives a transient
inline build-status line through `devFooterWriter`.

Behavior:

- animated build status line during active build steps
- final green check is left in the transcript on completion
- this line is not part of the sticky footer

Important constraint:

- build status must not be rendered through the footer itself
- the footer should remain reserved for hotkeys/runtime state

## Runtime Restart Sections

Runtime restarts are organized around the single `Run App` watcher.

`devwatch_streamer.go` emits labeled separators when the runtime supervisor restarts:

- `Shutdown`
- `Start`

These are transcript separators, not footer UI.

## Why This Is Delicate

This code is sensitive because terminal state is shared across:

- watcher output
- footer redraws
- transient build-status redraws
- restart separators

The easiest way to break things is to mix these responsibilities without being explicit
about which output is:

- persistent footer UI
- transient inline UI
- ordinary transcript output

## Practical Rules

If you change this area:

1. Do not route build status back into the sticky footer.
2. Do not print internal build progress markers into the transcript.
3. Keep watcher/app logs as ordinary transcript lines.
4. Be careful with cursor movement; verify behavior in a real terminal.
5. Prefer small, explicit helper methods over clever terminal rewrites.

## Future Cleanup

The main complexity hotspot is `devFooterWriter`, because it currently owns both:

- sticky footer behavior
- transient inline build-status behavior

If this area grows further, the next cleanup should likely split those two concerns.

# Dev TUI Footer Plan

## Current Problem

The current `forj dev` footer is implemented by writing real separator/footer lines into stdout and then rewriting them on the next output line.

That means:

- the footer can appear inline in the transcript at least once
- startup/restart timing affects when the footer materializes
- trying to "fix" this with cursor-position hacks without a real reserved region breaks terminal layout

## What We Want

- normal watcher/app logs should stay in the scrollable transcript
- the hotkey footer should stay pinned to the bottom
- the footer should never be emitted as ordinary log output
- the terminal should remain clean on exit, restart, render, and Ctrl+C

## Proper Solution

Implement a real reserved footer region instead of line-rewrite tricks.

Core pieces:

1. Reserve the bottom 2 terminal lines for:
   - separator
   - footer
2. Set the terminal scroll region to the area above the footer.
3. Write watcher/app output only into that scrollable region.
4. Redraw the footer independently without affecting transcript lines.
5. Restore the terminal scroll region and cursor state on shutdown.

## Required Behaviors

### Startup

- initialize the footer region before watcher output begins
- draw separator + hotkey footer once in the reserved bottom region
- do not print the footer into the transcript

### Normal Output

- watcher/app output scrolls above the footer
- footer remains fixed
- footer updates in place when hotkey state changes

### Restart / Render

- keep footer region intact if possible
- if full reset is needed, reinitialize region cleanly before resumed output

### Exit / Failure

- restore normal terminal scrolling
- clear footer region or leave terminal in a sane plain state
- avoid corrupted shell prompt placement

### Resize

- detect terminal resize
- recompute footer rows and scroll region
- redraw footer cleanly

## Risks

- PTY subprocess output may emit cursor movement / ANSI sequences that fight region control
- terminal emulator differences may affect scroll-region behavior
- resize handling is easy to get subtly wrong
- failure paths must restore terminal state or the shell is left broken

## Implementation Notes

- keep the logic isolated in `internal/forj/dev_tui.go`
- avoid more partial hacks that still write footer text into the transcript
- prefer a single explicit model:
  - transcript region
  - footer region
- test interactively in a real terminal, not just unit tests

## Suggested Build Order

1. Add explicit footer-region lifecycle helpers:
   - init
   - redraw
   - teardown
2. Add terminal state restoration guarantees.
3. Add resize handling.
4. Wire transcript writes through the scroll region.
5. Test:
   - startup
   - restart
   - render
   - Ctrl+C
   - watcher error / recovery
   - terminal resize

## Non-Goals

- do not turn `forj dev` into a full-screen TUI
- do not special-case watcher output formatting further until the footer region is correct
- do not keep mixing transcript output and footer rendering

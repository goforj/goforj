# Web Boundary

This document explains what belongs in `web` versus what should stay in GoForj.

## Put It In `web` When

- it is reusable web runtime behavior
- it should hide framework-specific details from generated apps
- it is middleware, routing, or telemetry functionality
- it improves web abstraction parity with the underlying engine

Examples:

- request/response abstractions
- route listing/sorting
- middleware adapters
- Prometheus support
- Echo version migrations

## Keep It In GoForj When

- it is app policy
- it is generation policy
- it depends on project-local env or build artifacts
- it is demo-app-specific behavior
- it is local DX/workflow behavior

Examples:

- generated env conventions
- render-time module replace support
- dev TUI output policy
- demo app command behavior
- project-local swagger wiring tied to local build artifacts

## Gray Areas

Some things are reusable but not urgent to move.

For example:

- SPA helpers may be good future `web` candidates
- some project-local helpers may stay in GoForj temporarily if moving them now does not buy much

The rule is not “move everything possible into `web` immediately.”

The rule is:

- move it when doing so meaningfully improves abstraction and reduces framework leakage

## Success Condition

The best sign the boundary is working:

- upstream framework churn stays inside `web`
- generated app shapes do not need to know or care

The Echo v5 migration was strong evidence that this boundary is now paying off.

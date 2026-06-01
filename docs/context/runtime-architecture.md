# Runtime Architecture

This document explains the generated app runtime structure.

## Root Package

The root runtime package is:

- `internal/app`

This is where app-level runtime policy belongs.

## Main Concepts

### `Lifecycle`

Coordinates startup and shutdown phases for long-lived runtime components.

Use it for:

- startup hooks
- shutdown hooks
- ordering root runtime behavior

### `Timeouts`

Holds app-level timeout policy resolved once.

This exists so leaf primitives do not rediscover env values independently.

### `LifecycleRegistry`

The intended user extension point for custom lifecycle hooks.

This is where docs should point users when they need to run their own startup/shutdown logic.

## Ownership Lines

### `internal/app`

Owns:

- app timeout policy
- app lifecycle coordination
- root runtime wiring

### `internal/http`

Owns:

- HTTP server composition
- API/web runtime glue

### `internal/jobs`

Owns:

- long-lived job worker process behavior
- queue worker runtime wiring

### `internal/schedules`

Owns:

- long-lived scheduler process behavior
- scheduler runtime wiring
- declarative schedule registration in `internal/schedules/scheduler_registry.go`

Important boundary:

- `internal/schedules/scheduler.go` should stay focused on scheduler runtime/bootstrap concerns
- domain-owned scheduled work should live on the owning types, not as thin `Scheduler.runX` wrappers
- Lighthouse/operator translation for schedules belongs in `internal/schedules/lighthouse.go`, not in scheduler bootstrap code

Recent concrete pattern:

- schedule registry entries should point directly at domain-owned methods such as:
  - `retentionService.RunScheduled`
  - `monitorCheckJob.RunScheduledPoll`
  - `monitorCheckJob.RunScheduledPushTrigger`

This keeps the registry declarative and avoids growing `Scheduler` into a business-logic bucket.

## Process Boundary Principle

Process bootstrap concerns should live at the process boundary.

Examples:

- process-specific Lighthouse runtime startup
- process-owned lifecycle logging
- process-owned operator adapters such as scheduler Lighthouse command registration

They should not be hidden in generic low-level helpers if the ownership is really process-specific.

## Timeout Principle

Resolve timeout policy once near the root, then pass it down.

Do not repeat env lookups in:

- HTTP server
- worker
- scheduler
- other long-lived primitives

That was a major cleanup direction in recent work.

## Logging Principle

Keep a distinction between:

- top-level process lifecycle logs
- primitive-level detailed chatter

Top-level lifecycle logs should stay visible.
Primitive chatter should usually be debug-level or explicitly scoped.

## Extension Point Guidance

If a user wants custom lifecycle logic, the intended place is:

- `internal/app/lifecycle_registry.go`

Not ad hoc edits scattered through generated runtime files.

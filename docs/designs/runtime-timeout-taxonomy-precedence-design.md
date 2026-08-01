# Runtime Timeout Taxonomy and Precedence Design

## Status

- Design status: implemented
- Planning date: 2026-07-31
- Implemented: 2026-08-01
- Scope: App runtime shutdown, queues, scheduler commands, subprocesses, development supervision, and health probes

## Summary

Timeouts should be classified by purpose and constrained from the outside in. `APP_SHUTDOWN_TIMEOUT` is the total graceful-shutdown budget for one App process. Component and subprocess shutdown settings may shorten work inside that budget, but may never extend it. Normal command execution, probes, and external supervisor grace periods are separate clocks.

## Prior Behavior and Conflict

Framework-managed `internal/runtime.Timeouts` resolves `APP_SHUTDOWN_TIMEOUT` (30s), `QUEUE_SHUTDOWN_TIMEOUT` (10s), and `SCHEDULER_SUBPROCESS_SHUTDOWN_TIMEOUT` (falling back to the App value). The rendered `.env` currently sets the scheduler subprocess value to 90s. Queue workers use their queue value, scheduler shutdown uses the App value, HTTP receives the App value, and scheduled command execution independently reads `SCHEDULER_COMMAND_TIMEOUT` (10m).

This allows contradictory budgets: a scheduler-owned subprocess may plan to spend 90 seconds shutting down while its parent scheduler and App have only 30 seconds. The native `forj dev` supervisor has its own 5-second default before forced process-tree termination. Readiness checks currently use a 2-second per-check timeout. These are useful mechanisms, but they do not yet form one explicit precedence model.

## Taxonomy

| Setting or budget | Purpose | Starts | Relationship |
| --- | --- | --- | --- |
| `SCHEDULER_COMMAND_TIMEOUT` | Maximum normal runtime of one scheduled command | When the command starts | Execution deadline, not shutdown grace |
| `APP_SHUTDOWN_TIMEOUT` | Total graceful shutdown of one App process | On cancellation or termination | Outer in-process deadline |
| `QUEUE_SHUTDOWN_TIMEOUT` | Queue polling stop and in-flight job drain | When queue shutdown starts | Child of remaining App budget |
| `SCHEDULER_SUBPROCESS_SHUTDOWN_TIMEOUT` | Graceful cleanup inside a scheduler-originated command process | When that subprocess is canceled | Child of remaining command and process budgets |
| component shutdown budget | HTTP, scheduler, lifecycle hook, or sink flush allowance | When that component begins stopping | Child of remaining App budget |
| supervisor grace | Time an external owner waits before force-kill | When it signals the process tree | Must exceed the advertised App budget plus margin |
| probe timeout | Time a health/readiness caller or dependency check waits | Per probe request/check | Independent operational budget |

## Precedence

The effective deadline is always the earliest applicable deadline already in the context or configuration:

```text
effective leaf deadline = min(parent context deadline, App deadline, component override)
```

Rules:

1. The first shutdown signal creates one App deadline. All logical runtimes and lifecycle hooks receive contexts derived from its remaining time.
2. Runtime components stop concurrently where dependencies allow; they do not each receive a fresh full App timeout in sequence.
3. `QUEUE_SHUTDOWN_TIMEOUT` is `min(configured queue value, remaining App budget)`. A missing, invalid, or non-positive value inherits the App budget.
4. Scheduler engine shutdown is bounded by the remaining App budget.
5. `SCHEDULER_COMMAND_TIMEOUT` bounds healthy execution. Cancellation or its expiry begins subprocess termination; it does not grant a new shutdown window.
6. `SCHEDULER_SUBPROCESS_SHUTDOWN_TIMEOUT` is `min(configured value, remaining parent command deadline, remaining App/supervisor-visible budget)`. Missing, invalid, or non-positive values inherit the remaining parent budget.
7. Logger/sink flush and other cleanup consume the remaining App budget rather than extending it.
8. A second termination signal may request immediate exit; it does not reset any deadline.

The root `Timeouts` policy should resolve all four environment settings once and expose effective-duration helpers. Scheduler code should stop reading env directly.

## Supervisor Budgets

An external supervisor owns escalation, not graceful cleanup. Its grace period must be greater than the App's advertised shutdown timeout by a small scheduling and signal-delivery margin. For example, a 30-second App budget needs a supervisor grace greater than 30 seconds in production.

`forj dev` is optimized for replacement speed and currently defaults to 5 seconds. It may intentionally cap the effective development shutdown budget by passing or deriving a shorter child budget before launch. It should make that cap visible rather than silently promising the App 30 seconds and force-killing it after 5. Production manifests and release tooling must not inherit the development default.

## Probe Budgets

Liveness remains cheap and dependency-free. Readiness dependency checks retain their own short per-check deadline, but the request also needs an overall deadline so a growing check list cannot multiply latency without bound. Each check uses the earlier of its per-check deadline and the request deadline.

Probe budgets do not borrow from shutdown cleanup. On shutdown, readiness should fail promptly while liveness may remain successful until the process is exiting, allowing the orchestrator to remove traffic before the supervisor grace expires.

## Defaults and Compatibility

Keep the existing environment names and initially keep `30s`, `10s`, and `10m` as the App, queue, and scheduler command defaults. Stop generating a contradictory independent 90-second scheduler subprocess promise; its default should inherit the App/parent budget.

Existing explicit values continue to parse, but values larger than an outer budget are capped and produce one structured startup warning. This is a runtime-behavior compatibility change, not a source or configuration-format break: shutdown may finish or escalate earlier where the old configuration could never be honored by the parent.

No minimum Go version, public domain API, or persisted data changes are required. About/config diagnostics should show configured and effective values when they differ.

## Validation Plan

Implementation should cover value parsing and inheritance, queue/app inversion, the current 90s-versus-30s scheduler conflict, command timeout followed by bounded cleanup, concurrent component shutdown under one deadline, second-signal escalation, async log flush using remaining time, development supervisor margin, and readiness per-check plus overall deadlines.

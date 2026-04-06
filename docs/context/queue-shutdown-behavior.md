# Queue Shutdown Behavior

This document explains the intended queue shutdown model.

## Goal

Shutdown should feel snappy and predictable.

That does not mean queue workers always stop instantly. It means:

- they honor the configured shutdown budget
- they do not hang unpredictably
- the shutdown path is consistent through the stack

## Timeouts

App/runtime timeout policy should be resolved once near the root.

Queue shutdown can use a queue-specific timeout, but it should align with the root runtime model rather than rediscover env values ad hoc.

## Driver-Level Responsibility

If Redis/Asynq shutdown behavior is inconsistent, the real fix may belong in `queue`, not `goforj`.

Examples:

- pass `QUEUE_SHUTDOWN_TIMEOUT` through to driver config
- ensure worker shutdown honors context
- keep backend shutdown behavior aligned with GoForj expectations

## What “Slow” Often Means

A queue worker shutting down after the API/scheduler does not necessarily mean a bug.

Often it means:

- active jobs are being drained
- Redis/Asynq is finishing in-flight work
- backend round trips are still happening

That is normal as long as it remains bounded and predictable.

## Useful Diagnostics

During shutdown it is useful to know whether the worker is waiting for active jobs.

That is higher-value than generic vague shutdown chatter.

## Design Principle

Make shutdown behavior:

- bounded
- explicit
- consistent across layers

Avoid:

- duplicated timeout resolution
- leaf components silently recreating default policy
- blaming GoForj for backend-driver behavior that really belongs in `queue`

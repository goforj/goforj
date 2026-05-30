# Job Queue Production DX Plan

This plan tracks the work needed for generated GoForj job queues to feel production-grade while keeping the App model simple.

## Current Direction

Named queues are the primary App-level model. One generated queue resource represents one queue.

- `worker` without flags starts every configured generated queue.
- `worker --queue <name>` starts only the selected named queue.
- Repeated `--queue` flags select a subset.
- Named queues inherit the root queue driver unless they override it.
- The generated queue name defaults to the resource name; `QUEUE_<NAME>_NAME` is only for rare backend name overrides.
- Queue priority should usually come from worker allocation and process sizing, for example `QUEUE_EMAILS_WORKERS=6` and `QUEUE_REPORTS_WORKERS=2`.
- Backend-specific queue weighting should stay secondary and driver-specific.

Per-job execution policy belongs on jobs, not queue configuration.

- `Timeout` is per job.
- `Retry` is per job.
- `Backoff` is per job.
- `OnQueue` is per job dispatch.
- Uniqueness is per job dispatch.

## High-Leverage Work

### 1. Generated Job Dispatch Policy

Make `forj make:job` able to generate visible dispatch policy:

```bash
forj make:job reports:generate --queue reports --timeout 3m --retries 3
```

The generated job should keep policy in one obvious place near dispatch:

```go
queue.NewJob(GenerateJobTypeName).
	Payload(payload).
	OnQueue("reports").
	Timeout(3 * time.Minute).
	Retry(3)
```

Do not introduce queue-level timeout config such as `QUEUE_REPORTS_TIMEOUT`. Timeout is about a unit of work, not the transport lane.

### 2. Worker Selection Coverage

Add behavioral coverage for generated worker selection:

- render a jobs App with named queues
- verify `./bin/app worker --queue reports` parses and starts the selected queue
- verify repeated flags parse
- verify an unknown queue fails with a clear error
- verify no flag selects all configured generated queues

Template text tests are useful, but this should be backed by a rendered App test.

### 3. Queue Inventory

Expose queue configuration in an operator-readable surface.

Possible command shapes:

```bash
./bin/app queue:list
./bin/app about
```

Show:

- queue name
- driver
- backend queue name
- worker count
- whether it is default
- supported admin capabilities when known

This should help operators answer: "What queues will this process work?"

### 4. Job Policy Inventory

Make job policy inspectable.

Show job type, queue, timeout, retry, backoff, and uniqueness where possible:

- `./bin/app about`
- Lighthouse job views
- inspect records
- queue/job documentation generated from App metadata

This matters more than adding another config knob.

### 5. Timeout Contract Audit

Document and test the timeout semantics precisely.

Current intended contract:

- timeout supplies a deadline/cancelable context to the handler
- handlers must honor `ctx.Done()`
- timeout is not a hard goroutine kill

Audit details:

- Redis currently applies a backend default timeout when a job does not specify one.
- SQL-backed queues persist timeout in seconds, so sub-second values are rounded.
- NATS, SQS, and RabbitMQ serialize timeout in milliseconds.
- Local drivers apply timeout directly with `context.WithTimeout`.

Decide whether Redis's default timeout and SQL rounding should remain as-is, be documented, or be adjusted.

### 6. Queue Operations Runbooks

Add runbooks for common production incidents:

- backlog growth
- stuck workers
- handler timeout or cancellation
- retry storm
- poison jobs
- broker outage
- queue admin unsupported by selected backend

Runbooks should explain what to check in logs, metrics, inspects, Lighthouse, and backend state.

## Suggested Order

1. Add generated job dispatch policy flags.
2. Add rendered App worker selection tests.
3. Add queue inventory.
4. Add timeout contract audit tests/docs.
5. Add job policy inventory.
6. Add queue operations runbooks.

This order improves day-to-day developer experience first, then improves operational visibility.

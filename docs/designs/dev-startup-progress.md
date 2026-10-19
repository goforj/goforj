# Development startup progress, schema v1

`forj dev` optionally emits bounded startup diagnostics independently of stdout and stderr. The CLI advertises `dev-startup-progress.v1` in `project:describe --json`. This is an additive observation contract, not a managed-session handshake, readiness signal, command channel, or resource inventory.

The caller creates an empty regular file in its private runtime directory, with mode `0600` on Unix and an owner-private parent ACL on Windows. It supplies the canonical absolute path as `FORJ_DEV_EVENTS_FILE` and a non-secret ASCII correlation ID (letters, digits, hyphens, underscores, at most 128 bytes) as `FORJ_DEV_EVENTS_SESSION`. The command remains ordinary `forj dev`, preserving process ownership receipts and standalone behavior. Older CLIs ignore the optional environment and continue producing human output. Consumers must retain their raw-output fallback.

GoForj validates the existing file without creating, truncating, or replacing it. It removes the two selectors from the live process environment and launcher inheritance before running child commands. A missing or invalid explicit destination is a launch error. A stream that becomes unwritable after admission disables diagnostics without failing development work. The caller owns retention and cleanup.

The file contains UTF-8 JSON Lines, at most 256 records and 32 KiB for one startup. Every complete record has exactly these fields:

```json
{"schema_version":1,"session":"session-123","sequence":1,"phase":"task","label":"Run Docker Compose","status":"started","elapsed_ms":0}
```

`sequence` increases from one without gaps. `phase` is `startup`, `task`, `app.prepare`, or `frontend.build`. `label` is a display label of at most 256 bytes, with controls removed; configured task names are display metadata and must not contain credentials. Commands, environment values, error strings, and subprocess output are never included. `status` is `started`, `completed`, `failed`, or `cancelled`. Completion records carry the elapsed milliseconds for that span. Match a terminal record to the most recent open span with the same phase and label; repeated task labels remain distinct occurrences.

The outer `startup` span surrounds initial configuration, tasks, builds, database setup, and watcher reconciliation. It completes when the watcher graph starts successfully, fails when startup returns an error, or cancels when the command exits before completion without an error. It does not assert HTTP readiness. There are no later rebuild or shutdown events in v1. Abrupt termination or a stream bound can leave incomplete spans; consumers must display that honestly and continue to use independent process and readiness observations.

A reader ignores an incomplete final line while the producer is writing. Unknown schema, phase, status, fields, mixed sessions, invalid labels, out-of-order sequences, oversized streams, and unsafe paths make the optional diagnostics unavailable. These errors must not stop a project, repair a listener, change durable lifecycle state, or hide raw output. JSON Lines must never be injected into the terminal log stream or recovered by matching human CLI text.

## Embedded presentation

A host that already owns resource navigation can additionally set `FORJ_DEV_PRESENTATION=embedded`. This independent opt-in omits the legacy `Dev ready` URL inventory from the plain stream; it does not remove subprocess output, failures, lifecycle events, or readiness checks. The selector is consumed before child commands and is not forwarded through launcher inheritance. Without that exact value, ordinary console and redirected output retain their existing summaries. Older CLIs ignore this selector, so a host must tolerate their raw output rather than filter terminal prose.

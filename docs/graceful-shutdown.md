# Graceful Shutdown

This project now includes graceful termination behavior for generated runtime components so in-flight work can finish before process exit.

## Signal handling

The following generated commands listen for `SIGINT` and `SIGTERM` using `signal.NotifyContext`:

- `http:serve`
- `schedule:run`
- `queue:work`

When a termination signal is received, each component transitions into shutdown flow instead of hard-exiting.

## HTTP server (`http:serve`)

Behavior:

- Starts the server with `http.Server.ListenAndServe`.
- On shutdown signal, calls `http.Server.Shutdown(...)` with a 30-second timeout.
- Stops accepting new connections.
- Allows active requests to complete before returning.

Effect:

- In-flight web requests are drained gracefully.

## Scheduler (`schedule:run`)

Behavior:

- Starts scheduled jobs normally.
- On shutdown signal, calls scheduler `Shutdown()`.
- Scheduler stops new triggers and waits for running jobs to complete.

Effect:

- Already-running schedule executions are allowed to finish.

## Worker (`queue:work`)

Behavior:

- Starts queue worker with `srv.Start(mux)`.
- Waits for shutdown signal.
- Calls `srv.Shutdown()` to stop polling and drain active tasks.

Effect:

- In-progress jobs continue to completion during shutdown.

## Lighthouse agent lifecycle

Each process now passes a cancellable runtime context to its lighthouse agent startup. On termination, the same context is cancelled so agent goroutines can stop cleanly with the process shutdown path.

## Source templates

The behavior above is generated from:

- `templates/internal/http/serve_cmd.go.tmpl`
- `templates/internal/http/server.go.tmpl`
- `templates/internal/scheduler/scheduler.go.tmpl`
- `templates/internal/jobs/worker.go.tmpl`

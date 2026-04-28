# GoForj Systray Design Sketch

This document sketches a possible systray companion for GoForj.

Status:

- exploratory
- intended to clarify the architecture before implementation
- intended to serve as a staged implementation plan

Important implementation constraint:

- the main `forj` CLI should remain CGO-free by default
- the tray UI should therefore live in a separate binary

External library reference:

- `github.com/getlantern/systray`

Key findings from inspecting that library:

- requires `CGO_ENABLED=1`
- supports Windows, macOS, and Linux
- Linux needs `gcc`, `gtk3`, and `libayatana-appindicator3` development headers
- Windows tray apps should be built with `-ldflags -H=windowsgui`
- macOS wants an app bundle for polished behavior and to avoid Dock presence
- menu items are managed in-process and interacted with through channels
- most menu mutations are safe from other goroutines

## Goal

Provide a lightweight multi-project system tray companion that gives developers fast access to:

- current project status
- local tool links
- dynamic resource links
- watcher health
- restart and recovery actions
- recent failures

The systray should behave like a local development control plane, not like a second app shell.

The tray should be session-aware across many concurrently running `forj dev` processes.

The tray should primarily act as a high-level jumping off point for local development utilities.

## Binary Boundary

The tray should not be built into the default `forj` binary.

Reason:

- common systray libraries such as `github.com/getlantern/systray` rely on CGO
- forcing CGO into the main CLI would complicate builds, releases, and local installs
- most `forj` users should not pay that cost unless they actually want tray support

Recommended layout:

- main CLI stays in `./cmd/forj`
- tray manager lives in a separate binary such as `./cmd/forj-systray/main.go`

Possible invocation model:

- `forj dev` detects whether the tray companion is available
- if enabled and not already running, `forj dev` launches `forj-systray`
- `forj dev` then registers its session over local HTTP

This keeps concerns clean:

- `forj` remains the portable core CLI
- `forj-systray` owns the CGO and desktop integration surface

Recommended package split:

- `./cmd/forj`
  - existing CLI
  - starts and updates tray companion when enabled
- `./cmd/forj-systray`
  - systray process entrypoint
  - CGO-only desktop host
- `./internal/trayapi`
  - request and response types shared by `forj` and `forj-systray`
- `./internal/trayclient`
  - client used by `forj dev`
- `./internal/trayserver`
  - in-memory session registry and local HTTP server
- `./internal/traymenu`
  - menu rendering and update logic

## Non-Goals

The systray should not try to become:

- a full log viewer
- a replacement for `forj dev`
- a replacement for Lighthouse
- a configuration editor for every project setting
- a product feature surface for generated applications

If an interaction needs a lot of space, text, or workflow depth, it should open the terminal, browser, or a dedicated window instead.

The tray should emphasize:

- persistent availability
- high-signal links
- one-click launch actions
- short health summaries

It should not try to absorb richer experiences that belong in:

- Lighthouse
- the browser app
- the terminal

## Core Idea

`forj dev` already knows the most important local state for one running project:

- which project is active
- which watchers are running
- which local tools are available
- which services are healthy
- which watcher failed most recently

The systray manager should collect that state from every active `forj dev` session and expose the most useful parts in a compact, low-friction menu.

The terminal remains the primary runtime surface.

The systray becomes:

- a quick launcher
- a durable utility shelf
- a health summary
- a notification source
- a small set of recovery controls
- a session switcher for multiple active projects

## Recommended Top-Level Menu

Suggested structure:

- overall status line
- active sessions summary
- current session submenu
- open links section
- watcher and service status section
- restart actions
- recent error section
- utilities
- quit

Example:

```text
GoForj: 3 sessions active
acme-api: healthy
wire: rebuilding
goforj: failed

Current: acme-api

Open App
Open API
Open Mailpit
Open Grafana
Open VictoriaMetrics
Open Lighthouse

API: healthy
Frontend: rebuilding
Jobs: healthy
Scheduler: healthy

Restart app
Restart frontend
Restart all

Recent error: frontend build failed
Open logs
Copy recent error

Run migrations
Open project folder
Copy app URL

Quit
```

Alternative submenu-oriented shape:

```text
GoForj: 3 sessions active

acme-api >
wire >
goforj >

Quit
```

Where each session submenu contains that project's links, statuses, and actions.

## What Belongs In The Tray

Good tray items are:

- frequent
- local-dev-specific
- low-risk
- instantly actionable

Best first-pass items:

- app status
- current project path or name
- active session count
- app and API URLs
- Mailpit URL
- observability URLs when enabled
- any generated utility URL that is only known at runtime
- per-watcher health
- restart actions
- recent failure summary

These give the tray immediate value without forcing too much product design into it.

This dynamic-links behavior is one of the strongest reasons for the tray to exist:

- the links are local
- the ports may vary
- the resources may be optional
- the tray is always available as a quick launch surface

## What Should Stay Out

Avoid putting these directly into the tray menu:

- long logs
- rich configuration forms
- migration diff review
- queue inspection details
- large notification history
- anything destructive without confirmation

Also avoid trying to mirror Lighthouse itself into the tray.

The tray may link to Lighthouse and summarize small pieces of session state, but it should not become a miniature observability dashboard.

For these cases, the tray should open:

- the terminal
- a browser page
- a dedicated debug window

## Status Model

The tray needs:

- one aggregate tray state across all sessions
- one aggregate state per session
- per-watcher detail within each session

Suggested aggregate states:

- healthy
- starting
- rebuilding
- degraded
- failed
- stopped

Suggested session aggregate derivation:

- `failed` if any critical watcher is failed
- `rebuilding` if no critical failures exist and any watcher is rebuilding
- `starting` if the session has not reached ready state yet
- `degraded` if non-critical checks are unhealthy
- `healthy` if all expected checks are healthy
- `stopped` only after unregister or timeout cleanup

Suggested watcher states:

- starting
- running
- rebuilding
- failed
- stopped

Suggested health source inputs:

- watcher lifecycle state from `forj dev`
- readiness probe results
- last known restart timestamp
- last known failure

## Notifications

The tray becomes much more valuable if it can emit passive notifications for important dev loop events.

Good candidates:

- app ready
- build failed
- app recovered
- migration required
- Mailpit received auth email
- queue worker stopped

Suggested notification policy:

- notify only on meaningful transitions
- avoid spam during hot reload churn
- debounce repeated identical failures

Examples:

- `Frontend rebuild failed`
- `API recovered`
- `Password reset email received in Mailpit`

## Actions

Recommended first-pass actions:

- open app
- open API
- open Mailpit
- open Grafana
- open VictoriaMetrics
- open Lighthouse
- open any other discovered project utility URL
- restart app
- restart frontend
- restart all
- open logs
- copy recent error
- copy app URL
- open project folder

Actions to consider later:

- run migrations
- seed database
- reset local database
- open latest Mailpit message

## Session Model

The tray should be multi-app aware from the start.

Recommended model:

- one tray manager process
- many concurrently registered `forj dev` sessions
- one in-memory session registry
- one current or selected session for detail-oriented actions

Each registered session should include enough metadata to render and control that project independently.

Suggested session identity fields:

- session id
- project name
- absolute project path
- process id
- start time
- current aggregate state
- local URLs
- watcher states
- recent error summary

Suggested stable session key:

- generated session id at `forj dev` startup
- not derived from PID alone
- survives watcher restarts inside the same dev session

The tray manager should support:

- selecting the active session
- listing all sessions
- dropping stale sessions after heartbeat timeout
- removing sessions immediately on graceful unregister

## Tray Lifecycle

Recommended lifecycle:

- `forj dev` starts
- it checks whether the tray manager is running
- if not, it starts the tray manager
- it registers its session over local HTTP
- it sends state updates and heartbeats while running
- on Ctrl+C or normal shutdown, it unregisters
- when the last session disappears, the tray manager exits after a short grace period

This keeps the tray coupled to real active dev sessions without making each `forj dev` instance own tray UI state directly.

Suggested grace behaviors:

- if a session misses heartbeats, mark it stale
- if stale timeout elapses, remove it automatically
- if no sessions remain for a short idle window, tray exits

That protects against orphaned sessions after crashes.

## Integration Model

The cleanest implementation is a dedicated local tray manager process with an in-memory state machine.

Recommended transport:

- local HTTP on loopback
- JSON request bodies
- one writer per session for its own session state
- tray manager as the single reader and state owner

Recommended default binding:

- `127.0.0.1` only
- random or discoverable local port
- protected by a local session token or nonce

The tray should not scrape terminal output.

The tray manager should be the only component that owns:

- session registry
- aggregate tray state
- notification dedupe
- active session selection
- menu rendering state

`forj dev` should own only:

- its own watcher and health facts
- its own lifecycle
- its own action execution hooks

The tray companion binary should own:

- CGO-backed systray integration
- local HTTP server
- session registry
- menu state and notifications
- graceful process lifetime when sessions come and go

It should receive structured events such as:

- watcher started
- watcher ready
- watcher failed
- watcher restarted
- service URL available
- recent error updated

Suggested endpoints:

- `POST /sessions/register`
- `POST /sessions/{id}/heartbeat`
- `POST /sessions/{id}/state`
- `POST /sessions/{id}/events`
- `DELETE /sessions/{id}`
- `POST /sessions/{id}/actions/{name}`

Suggested minimal v1 endpoints:

- `POST /v1/sessions/register`
- `POST /v1/sessions/{id}/heartbeat`
- `PUT /v1/sessions/{id}/state`
- `DELETE /v1/sessions/{id}`

Suggested v2 endpoints:

- `POST /v1/sessions/{id}/events`
- `POST /v1/sessions/{id}/actions/{name}`
- `GET /v1/sessions`
- `GET /v1/status`

The action endpoint may be owned by the tray manager and proxied to the session, or the tray may call a per-session local callback target. Either is acceptable, but the tray manager should remain the coordination point.

## Build And Packaging Notes

Because tray support lives in a separate binary, build and packaging should treat it as optional.

Recommended behavior:

- `go install ./cmd/forj` should remain CGO-free
- `go install ./cmd/forj-systray` may require CGO and platform-native tooling
- release automation can choose whether to publish the tray companion per platform

Recommended `forj dev` behavior when the tray binary is unavailable:

- continue normally
- do not fail the dev loop
- optionally print a one-line note that tray support is unavailable on this machine

That makes the tray an enhancement, not a runtime dependency.

Suggested build flags and packaging notes:

- Linux:
  - document GTK/AppIndicator prerequisites clearly
- Windows:
  - build `forj-systray` with `-ldflags -H=windowsgui`
- macOS:
  - treat app bundle packaging as a real deliverable, not an afterthought
  - likely ship a tiny app wrapper for the tray binary in release workflows

## Why Not A Shared Manifest

A shared filesystem manifest is a possible bootstrap mechanism, but it should not be the primary state model.

Reasons:

- every `forj dev` process becomes a partial state manager
- locking correctness becomes critical
- stale session cleanup still needs heartbeats and expiry logic
- action routing back to the right process becomes awkward
- file-watch or polling based updates are less direct than local HTTP

Filesystem artifacts are still useful for:

- discovering the tray manager
- ensuring only one tray manager instance starts
- storing an optional socket or port hint

They should not be the main multi-writer source of truth.

## Suggested Event Shape

One possible envelope:

```go
type TrayEvent struct {
	Type      string            `json:"type"`
	Project   string            `json:"project"`
	Timestamp time.Time         `json:"timestamp"`
	Payload   map[string]any    `json:"payload"`
}
```

Examples:

```go
type DevReadyPayload struct {
	Project string            `json:"project"`
	URLs    map[string]string `json:"urls"`
}

type WatcherStatePayload struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type RecentErrorPayload struct {
	Source  string `json:"source"`
	Summary string `json:"summary"`
}
```

The tray UI should remain thin:

- it receives structured state
- it renders menu items
- it executes bounded actions

The tray manager should treat resource links as first-class session data.

Suggested resource shape:

```go
type SessionResourceLink struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Category string `json:"category,omitempty"`
	Priority int    `json:"priority,omitempty"`
}
```

Examples:

- `App`
- `API`
- `Mailpit`
- `Grafana`
- `VictoriaMetrics`
- `Lighthouse`
- `OpenAPI`
- `Admin UI`

The menu should prefer showing the highest-value links first and avoid dumping an unstructured long list into the main menu.

Suggested registration payload:

```go
type RegisterSessionRequest struct {
	SessionID   string            `json:"session_id"`
	ProjectName string            `json:"project_name"`
	ProjectPath string            `json:"project_path"`
	PID         int               `json:"pid"`
	URLs        map[string]string `json:"urls"`
	Capabilities []string         `json:"capabilities,omitempty"`
}
```

Suggested state update payload:

```go
type SessionStateUpdate struct {
	AggregateState string                 `json:"aggregate_state"`
	Watchers       []WatcherStateSnapshot `json:"watchers"`
	RecentError    *RecentErrorPayload    `json:"recent_error,omitempty"`
}

type WatcherStateSnapshot struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}
```

## UX Notes

The tray should feel quiet and operational.

Recommended presentation choices:

- short labels
- explicit statuses
- separators between groups
- only one recent error line
- no nested menu maze unless there is a clear need
- clear session labels when many projects are active

Good examples:

- `API: healthy`
- `Frontend: rebuilding`
- `Recent error: vite build failed`
- `wire: failed`
- `goforj: 2m ago rebuilt`
- `Open Mailpit`
- `Open Grafana`

Bad examples:

- huge log excerpts
- many stacked timestamps
- five levels of submenus

## Platform Notes

A tray app has platform-specific constraints:

- menu size is limited
- labels should be short
- icons should be simple and monochrome-friendly
- browser opening and folder opening differ across platforms

That means the design should optimize for:

- a compact menu
- portable shell-out behavior
- minimal custom windowing

## Rollout Plan

Reasonable phases:

### Phase 1

- tray icon
- separate `forj-systray` binary
- local HTTP tray manager
- multi-session registration and unregister
- aggregate tray status
- session list
- open app and tool links
- per-session watcher status
- restart actions
- recent error summary

### Phase 2

- passive notifications
- copy-to-clipboard helpers
- open logs action
- open project folder
- heartbeat timeout cleanup
- active session switching polish

### Phase 3

- latest Mailpit email shortcut
- migrations and seed actions
- richer observability shortcuts

## Recommendation

If GoForj gets a systray, it should be positioned as:

- companion to `forj dev`
- local development status and launcher
- fast recovery surface

It should not try to become:

- a second terminal
- a mini admin panel
- a substitute for the browser UI

The best first version is still small, but it should be multi-app aware:

- session registration
- health
- links
- restart controls
- recent error

That is enough to be meaningfully useful without creating a large maintenance surface.

## Dependency-Specific Notes

Based on `getlantern/systray`, the design should assume:

- tray menu state is local to the tray process
- menu item click handling is in-process and channel-driven
- dynamic menu updates should be debounced and applied from a state projection layer

That means the tray manager should not try to mirror the entire session graph directly into menu items on every event. Instead:

- keep canonical session state in memory
- compute a compact menu projection
- update only changed labels, visibility, and enabled states

This avoids churn and keeps the systray layer simple.

## Recommended Initial Scope

The first implementation should intentionally avoid:

- embedded logs window
- bidirectional watcher streaming
- deep nested submenus for every watcher
- tray-driven terminal process ownership
- advanced auth between local clients beyond a simple local token

The goal is to prove:

- multi-session registration works
- tray state stays correct under concurrent `forj dev` sessions
- the UI remains stable and useful
- `forj` stays CGO-free
- dynamic resource links are materially useful

## Milestones

### Milestone 1: Protocol And State Model

Objective:

- define the shared local API between `forj dev` and the tray companion

Tasks:

- add shared request and response structs under `internal/trayapi`
- define session registration payload
- define session state payload
- define aggregate session status derivation rules
- define heartbeat and stale-session timeout policy
- define local discovery mechanism for the tray process

Exit criteria:

- API types compile
- session model is documented
- unit tests cover aggregate status derivation and stale cleanup rules

### Milestone 2: Tray Manager Process

Objective:

- build a standalone `forj-systray` process with an in-memory session registry and local HTTP server

Tasks:

- add `cmd/forj-systray/main.go`
- start local HTTP server on loopback
- ensure single running tray manager instance
- implement register, heartbeat, state update, and unregister endpoints
- add in-memory session registry with timeout cleanup
- add internal tests for session add, replace, timeout, and removal

Exit criteria:

- tray manager starts independently
- multiple sessions can register concurrently
- stale sessions are removed
- last-session shutdown logic works

### Milestone 3: Menu Host

Objective:

- render a stable multi-session menu using `getlantern/systray`

Tasks:

- add systray initialization in `forj-systray`
- define menu projection model
- render session list and current-session actions
- render aggregate status in title and tooltip
- support dynamic open-link actions
- support quit behavior for the tray process

Exit criteria:

- tray icon appears on supported platforms
- menu updates when session state changes
- multiple sessions are visible and selectable
- menu remains usable under frequent updates
- runtime resource links render predictably and open correctly

### Milestone 4: `forj dev` Integration

Objective:

- make `forj dev` start and feed the tray companion without becoming dependent on it

Tasks:

- add tray opt-in or auto-detect behavior to `forj dev`
- launch `forj-systray` if needed
- register session on dev startup
- send state updates on watcher lifecycle changes
- send heartbeats periodically
- unregister on clean shutdown
- tolerate tray unavailability without failing `forj dev`

Exit criteria:

- starting one `forj dev` session registers in tray
- starting multiple projects shows multiple sessions
- Ctrl+C unregisters cleanly
- crashing a session is cleaned up by timeout

### Milestone 5: User Actions And Notifications

Objective:

- make the tray materially useful as a dev tool

Tasks:

- wire open-app and open-tool links
- wire runtime-provided dynamic utility links
- support recent error summary updates
- add restart actions for app or frontend where feasible
- add passive notifications for major state transitions
- debounce repetitive notifications

Exit criteria:

- tray can open local tools reliably
- tray can expose session-specific dynamic resource links reliably
- tray can show and clear recent error state
- notifications are useful without spamming

### Milestone 6: Packaging And Platform Hardening

Objective:

- make the tray companion practical across supported developer platforms

Tasks:

- document Linux system dependencies
- add Windows build flags to release or local build flows
- define macOS app bundle strategy
- verify tray startup and shutdown on macOS, Linux, and Windows
- define behavior when platform prerequisites are missing

Exit criteria:

- build instructions are documented
- unsupported or misconfigured environments fail gracefully
- the tray can be distributed separately from the core CLI

## Implementation Checklist

Short checklist for teeing this up:

- confirm binary name: `forj-systray` or `forj tray`
- choose loopback discovery strategy
- choose session token strategy for local HTTP
- define v1 menu shape
- define which `forj dev` events should trigger state updates
- decide whether restart actions are in v1 or deferred
- decide whether tray is enabled automatically or behind a flag

## Recommendation

This is now concrete enough to implement in phases.

The best path is:

- keep the main `forj` CLI CGO-free
- build `forj-systray` as a separate desktop host
- use local HTTP for registration and state updates
- keep session state in memory in the tray manager
- ship a small but useful multi-session utility launcher first

That approach matches the actual constraints of the `getlantern/systray` library and keeps the core GoForj CLI architecture clean.

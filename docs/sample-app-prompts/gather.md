# Gather Sample App Prompt

Build **Gather**, a small event registration and waitlist application, in the
stock GoForj project provided to you.

Do not modify the GoForj framework itself. Use the generated project's existing
components, maker commands, App composition points, and infrastructure
abstractions. If a requirement exposes a framework defect or missing capability,
document it instead of bypassing generated architecture or editing
framework-managed generated files.

## Product

Gather lets anyone organize a small event and manage registrations without
payments or ticketing infrastructure.

An authenticated user can create an event, publish it, and share its public
page. Other authenticated users can register while capacity remains or join the
waitlist after it fills. When a registered attendee cancels, the first eligible
waitlisted attendee is promoted exactly once and notified.

Do not add payments, assigned seating, event series, external calendar
synchronization, maps, chat, or a social feed.

## Data Model

Implement the smallest clear model that supports:

- events with organizer, public slug, title, description, optional cover image,
  venue, application time zone, start/end times, registration open/close times,
  capacity, status, and timestamps
- registrations with attendee, state, position or ordering data, and timestamps
- reminder delivery records
- append-only event activity for publish, registration, waitlist, promotion,
  cancellation, and cancellation of the event itself

Event statuses are `draft`, `published`, `cancelled`, and `completed`.
Registration states are `registered`, `waitlisted`, `cancelled`, and `declined`.

Use migrations and database constraints where they protect capacity,
uniqueness, ownership, or state invariants. Keep time storage and application
time-zone behavior explicit.

## Required Behavior

Implement:

1. An organizer creates and edits a draft event.
2. An organizer publishes an event with valid capacity and registration dates.
3. A visitor can view a published event page.
4. An authenticated attendee registers while capacity remains.
5. An attendee joins the ordered waitlist when the event is full.
6. An attendee views and cancels their own registration.
7. Cancellation promotes the earliest eligible waitlisted attendee exactly
   once.
8. An organizer views the roster and waitlist, cancels an event, and exports a
   roster.
9. Registration is rejected before opening, after closing, after the event is
   cancelled, or after it begins.

Concurrent registrations must never exceed capacity. Retried requests must not
create duplicate registrations, waitlist entries, promotions, activity, or
notification work.

## Framework Components

Exercise these components through their normal generated abstractions:

- **Auth:** protect creation and registration while enforcing organizer and
  attendee ownership rules.
- **Database:** persist events and registrations with concurrency-safe capacity
  and promotion behavior.
- **Storage:** store optional event cover images and generated calendar `.ics`
  artifacts outside the database.
- **Events:** emit event-published, attendee-registered, attendee-waitlisted,
  attendee-promoted, registration-cancelled, and event-cancelled events.
- **Queue and jobs:** deliver confirmations, waitlist notices, promotions,
  reminders, and event-cancellation notifications asynchronously.
- **Mail:** provide useful attendee messages through the configured development
  transport.
- **Scheduler:** send upcoming-event reminders and complete past events. It must
  not duplicate reminders across repeated runs.
- **Cache:** cache published event summaries and remaining-capacity information,
  with correct invalidation after registration, cancellation, promotion, or
  organizer changes.
- **CLI:** seed realistic demo events, print an upcoming-event summary, and
  export an event roster.
- **Metrics and Lighthouse:** preserve framework instrumentation and make
  registration requests, promotion work, notification jobs, and reminder
  schedules understandable through existing observability surfaces.

Do not use backend-specific clients when the generated application provides a
database, storage, cache, queue, event, or mail abstraction.

## HTTP And UI

Use the starter kit already selected in the project. Provide:

- sign-in
- published event discovery or listing
- public event detail with current availability
- organizer event create/edit form
- attendee registration, waitlist, and cancellation states
- personal upcoming-events view
- organizer roster and ordered waitlist view
- clear cancelled, closed, full, and completed states

Expose an authenticated JSON API for organizer and attendee workflows plus the
minimal public read endpoints required for published events. Follow the
project's routing and response conventions so route listing and API index
generation continue to work.

## Authorization And Consistency

- Only an event's organizer can edit, publish, cancel, or export it.
- Attendees can view and cancel only their own registration.
- Draft events are visible only to their organizer.
- Public reads must not expose attendee email addresses or private roster data.
- Registration capacity and waitlist order must be decided on the server inside
  a concurrency-safe operation.
- Promotion must be idempotent and preserve waitlist order.
- Cache entries must not allow registration against stale capacity.

## Tests

Add focused tests proving at least:

- anonymous and cross-organizer mutations are rejected
- invalid dates, time zones, capacity, and state transitions are rejected
- registration outside its window is rejected
- concurrent attempts never create more registered attendees than capacity
- duplicate registration requests produce one active entry
- cancellation promotes the earliest eligible waitlisted attendee exactly once
- retried promotion does not duplicate events or queued mail
- event cancellation notifies each active attendee once
- capacity cache invalidation returns current availability
- reminder scheduling is deterministic and idempotent
- calendar artifacts are stored through the Storage abstraction

Use real generated integration boundaries where practical rather than replacing
the entire application with mocks. Control time explicitly in tests instead of
depending on the current wall clock.

## Completion Criteria

The task is complete when:

- migrations apply to a fresh database
- the seed command creates published, full, waitlisted, upcoming, and completed
  examples with human-readable output
- event creation, discovery, registration, waitlisting, promotion, cancellation,
  and roster export operate end to end
- queued mail can be observed through the configured development transport
- reminders and event completion can be demonstrated without waiting for real
  time to pass
- relevant backend and frontend tests pass, including the concurrency case
- `forj build` succeeds
- rerunning the project's normal generation/render workflow does not remove
  application behavior
- the final handoff lists commands run, test results, App-owned files changed,
  and any framework friction encountered

Keep the application focused on small events. Correct capacity, waitlist, and
notification behavior matters more than adding event-management features.

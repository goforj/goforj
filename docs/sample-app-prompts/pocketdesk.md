# PocketDesk Sample App Prompt

Build **PocketDesk**, a small team support desk, in the stock GoForj project provided to you.

Do not modify the GoForj framework itself. Use the generated project's existing components, maker commands, App composition points, and infrastructure abstractions. If a requirement exposes a framework defect or missing capability, document it instead of hiding it with copied framework code or edits to framework-managed generated files.

## Product

PocketDesk lets customers submit support tickets and lets support agents manage them.

The application needs two roles:

- `customer`: can create tickets and interact with tickets they own
- `agent`: can see and manage every ticket

Add a documented demo bootstrap or seed command that creates one agent, two customers, and enough tickets to make every screen useful.

## Data Model

Implement the smallest clear model that supports:

- users with a `customer` or `agent` role
- tickets with a public ID, owner, subject, description, status, priority, optional assignee, and timestamps
- comments belonging to a ticket and author
- attachments belonging to a ticket and uploader
- an append-only ticket activity record for important changes

Ticket statuses are `open`, `in_progress`, `waiting`, and `closed`. Priorities are `low`, `normal`, `high`, and `urgent`.

Use database constraints where they protect real invariants. Use migrations and repositories following the generated project's conventions.

## Required Behavior

Implement authenticated workflows for:

1. A customer creates a ticket.
2. A customer views their tickets and adds comments or attachments.
3. An agent views all tickets, filters by status or priority, assigns a ticket, changes its status, and comments.
4. A customer cannot view or mutate another customer's ticket.
5. An agent can view the activity history for a ticket.
6. Closing an already closed ticket is idempotent and does not produce duplicate notifications.

Validate required fields, enum values, attachment size, and attachment content type. Return useful user-facing errors without exposing internal details.

## Framework Components

Exercise these components through their normal generated abstractions:

- **Auth:** protect all application screens and APIs; enforce ticket ownership and agent-only operations.
- **Database:** persist the complete domain model.
- **Storage:** store attachment content outside the database and retain attachment metadata in the database.
- **Events:** emit events for ticket creation, assignment, comment creation, and closure. Use event subscribers for secondary reactions where appropriate.
- **Queue and jobs:** send assignment, reply, and closure mail asynchronously. The HTTP request must not wait for mail delivery.
- **Mail:** provide useful messages viewable through the configured development mail transport.
- **Scheduler:** periodically find open urgent tickets that have not changed recently and dispatch one reminder per eligible ticket per reminder window.
- **Cache:** cache agent dashboard counts by status and invalidate or refresh them when ticket state changes.
- **CLI:** add commands to seed demo data and print an overdue-ticket report.
- **Metrics and Lighthouse:** preserve framework instrumentation and make request, job, and scheduled execution understandable through existing inspection surfaces.

Do not call a queue, cache, storage, mail, or event backend directly when the generated application provides a manager or resource abstraction.

## HTTP And UI

Provide a usable interface using the starter kit already selected in the project. It must include:

- sign-in
- customer ticket list
- agent ticket inbox with status and priority filters
- create-ticket form
- ticket detail with comments, attachments, activity, and permitted actions
- compact agent dashboard counts

Expose an authenticated JSON API for the same primary ticket workflows. Follow the project's routing and response conventions so route listing and API index generation continue to work.

The UI only needs to be clear, responsive, and complete. Do not add a large design system or unrelated administrative features.

## Tests

Add focused tests proving at least:

- anonymous requests are rejected
- customers cannot access another customer's ticket
- agents can access and assign any ticket
- invalid status, priority, and attachment inputs are rejected
- ticket creation emits the expected event
- notification work is queued rather than delivered in the request path
- repeated closure does not duplicate closure work
- dashboard cache invalidation produces current counts
- the overdue schedule does not dispatch duplicate reminders within its reminder window

Use real generated integration boundaries where practical rather than replacing the entire application with mocks.

## Completion Criteria

The task is complete when:

- migrations apply to a fresh database
- the demo seed command produces human-readable output and a usable dataset
- the required UI and API workflows operate end to end
- queued mail can be observed with the configured development transport
- the scheduled reminder can be demonstrated without waiting for wall-clock time
- relevant tests pass
- `forj build` succeeds
- rerunning the project's normal generation/render workflow does not remove the application behavior
- the final handoff lists commands run, test results, App-owned files changed, and any framework friction encountered

Keep the implementation deliberately small. Prefer a complete vertical slice over extra ticketing features.

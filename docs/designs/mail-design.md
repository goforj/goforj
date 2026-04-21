# Mail Design

## Purpose

This document defines the first-class mail subsystem GoForj should grow next.

Important ownership line:

- reusable mail primitives should live in the sibling repo `github.com/goforj/mail`
- `goforj` should own the `Mail` component, generator wiring, env policy, previews, and generated app integration on top of that repo

The immediate goal is not “support every provider feature.” The goal is:

- give the framework a real mail story
- let auth stop pretending log lines are email delivery
- support local/dev, preview, testing, and production delivery with one coherent model
- keep provider details out of business logic

## Why This Exists

Auth now has real password reset and email verification flows, but the default delivery layer only logs intent.

That is acceptable as a temporary development stub, but it exposes a real framework gap:

- no first-class mail component
- no adopted framework mail primitive wired through GoForj yet
- no mail preview/render story
- no testing fake dedicated to mail

Mail should be its own subsystem, not auth-specific glue.

That subsystem should be split cleanly:

- `github.com/goforj/mail` owns the reusable mail abstraction and transport behavior
- `goforj` owns generated app composition, templates, previews, component flags, and framework policy

## Component Model

Mail should ship as a separate optional component:

- `Mail`: framework-owned mail transport, rendering, preview, and testing support

Dependency rules:

- `Mail` should not require `Auth`
- `Auth` should be able to depend on `Mail` when both are enabled
- local auth flows may still fall back to local-only token echo when `Mail` is not enabled

This keeps mail reusable for:

- auth
- invites
- receipts
- notifications
- support flows
- any app-owned outbound email

## Design Goals

GoForj mail should be:

- explicit
- provider-agnostic at the app boundary
- fluent to compose and send
- previewable without sending
- testable without live providers
- safe for local development
- capable of production delivery without redesign

GoForj mail should not start as:

- a giant provider feature matrix
- a markdown design system project
- a clone of Laravel’s entire mail surface
- CI coupled to live third-party email providers

## Proposed Scope For V1

V1 should include:

- a `Mail` component
- adoption of `github.com/goforj/mail` as the underlying primitive
- a framework mail service/manager built on that package
- a local `log` transport
- an `smtp` transport
- one API transport
  - likely `resend` or `postmark`
- a small typed message model
- mail template rendering for HTML and plain text
- browser preview support
- a fake/testing mailer
- auth integration on top of the mail subsystem

V1 should not include:

- failover routing
- round robin delivery
- complex attachment APIs
- transport-specific extension hooks everywhere
- every provider under the sun
- a full markdown component system

## Proposed API Shape

The public mail API should be fluent.

That means GoForj should prefer:

- chainable message composition
- readable sender/recipient/subject/body setup
- a clean final `Send(...)` step

It should avoid forcing callers to always construct large raw structs manually when composing common transactional mail.

App code should depend on a mail service, not on provider SDKs.
That service should be backed by `github.com/goforj/mail`, not by provider SDKs directly inside GoForj.

Possible shape:

```go
type Service struct {
	mail *mail.Manager
}
```

Example usage:

```go
err := s.mail.
	To("alice@example.com", "Alice").
	Subject("Verify your email").
	HTML(renderedHTML).
	Text(renderedText).
	Send(ctx)
```

More likely framework-facing shape:

```go
type Manager struct {
	defaultMailer Mailer
}

func (m *Manager) New() *MessageBuilder
func (m *Manager) Preview(name string, data any) (RenderedMessage, error)

type MessageBuilder struct {
	// ...
}

func (b *MessageBuilder) From(email, name string) *MessageBuilder
func (b *MessageBuilder) ReplyTo(email, name string) *MessageBuilder
func (b *MessageBuilder) To(email, name string) *MessageBuilder
func (b *MessageBuilder) Cc(email, name string) *MessageBuilder
func (b *MessageBuilder) Bcc(email, name string) *MessageBuilder
func (b *MessageBuilder) Subject(value string) *MessageBuilder
func (b *MessageBuilder) HTML(value string) *MessageBuilder
func (b *MessageBuilder) Text(value string) *MessageBuilder
func (b *MessageBuilder) Header(key, value string) *MessageBuilder
func (b *MessageBuilder) Tag(value string) *MessageBuilder
func (b *MessageBuilder) Metadata(key, value string) *MessageBuilder
func (b *MessageBuilder) Send(ctx context.Context) error
```

The key rule:

- app and framework code should express message intent
- the transport layer should decide how that intent becomes SMTP or API calls

Raw `Message` structs should still exist internally and for edge cases, but the primary application-facing API should be fluent.

## Canonical Message Model

V1 should define a small portable envelope.

Possible shape:

```go
type Message struct {
	From     *Recipient
	ReplyTo  []Recipient
	To       []Recipient
	Cc       []Recipient
	Bcc      []Recipient
	Subject  string
	HTML     string
	Text     string
	Headers  map[string]string
	Tags     []string
	Metadata map[string]string
}

type Recipient struct {
	Email string
	Name  string
}
```

Why this shape:

- covers the common transactional mail path
- works for SMTP and common API providers
- leaves room for provider-specific tags/metadata without making them mandatory everywhere

What V1 should not try to model deeply:

- inline attachments
- arbitrary MIME tree manipulation
- provider-native advanced objects

Those can come later if real use cases justify them.

## Mailables

GoForj should have a first-class concept similar to a mailable, even if the exact API differs from Laravel.

Each mail type should be represented as a typed renderer with:

- a subject
- an HTML template
- a plain text template
- structured input data

Possible shape:

```go
type Mailable interface {
	Name() string
	Render(ctx context.Context) (RenderedMessage, error)
}

type RenderedMessage struct {
	Subject string
	HTML    string
	Text    string
}
```

For generated auth, examples would be:

- `PasswordResetMail`
- `EmailVerificationMail`

This gives GoForj:

- previewability
- testable rendered content
- transport-independent generation

## Templates

V1 should support:

- HTML templates
- plain text templates

Recommended direction:

- generated templates live in the app
- `goforj` provides the rendering and generated-app sending machinery
- the lower-level send primitive comes from `github.com/goforj/mail`
- app owners can override or extend templates without forking framework internals

The framework should not require a markdown-mail DSL in v1.

Plain templates are enough to establish the subsystem cleanly.

## Transports

V1 transports should be:

- `log`
- `smtp`
- one API provider

Recommended initial API provider:

- `resend`

Why:

- simple transactional API
- widely used
- cleaner first API transport than supporting many at once

Possible future additions:

- `postmark`
- `ses`
- failover mailer
- round-robin mailer

## Configuration Model

Mail should follow the broader GoForj pattern:

- env-driven configuration
- supported-driver controls
- one runtime-selected driver

Possible env shape:

```env
MAIL_DRIVER=log
MAIL_SUPPORTED_DRIVERS=log,smtp,resend

MAIL_FROM_ADDRESS=no-reply@example.com
MAIL_FROM_NAME=My App

MAIL_SMTP_HOST=localhost
MAIL_SMTP_PORT=1025
MAIL_SMTP_USERNAME=
MAIL_SMTP_PASSWORD=
MAIL_SMTP_TLS=auto

MAIL_RESEND_API_KEY=
```

Possible future shape for named mailers:

- `MAIL_DEFAULT_MAILER=default`
- `MAIL_MAILERS=transactional,bulk`

But V1 does not need multi-mailer config yet.

## Local Development Story

Mail must have a good local/dev path.

Default local behavior should be one of:

- `log` transport
- local SMTP sink like Mailpit

Requirements:

- developers must be able to inspect rendered messages easily
- local development must not send to live inboxes by accident
- previews should not require a configured provider

## Preview Model

GoForj should support previewing rendered mail in the browser.

V1 preview support should be enough to:

- render a mailable to HTML
- render a text version
- expose previews in local/dev

Possible preview surface:

- local-only web routes under a dev/Lighthouse area
- or a simple command that writes previews to disk/stdout

The important capability is:

- template iteration without actual delivery

## Queueing

Mail should eventually integrate cleanly with Jobs, but queueing should not block V1.

Recommended stance:

- V1 supports synchronous `Send`
- V2 adds explicit queued send helpers when Jobs integration is ready

That keeps the first slice smaller while leaving the right extension point.

## Testing Model

Mail must be testable without talking to providers.

V1 should include:

- fake mailer
- sent-message assertions
- rendered-content assertions

At minimum, tests should be able to assert:

- a mail was sent
- a mail was not sent
- recipient/subject expectations
- rendered HTML/text contains expected content

Provider verification rule:

- live-provider checks are manual smoke tests
- CI uses fakes and stubs only

## Auth Integration

Once `Mail` exists, generated auth should stop using the log-only delivery default as its real story.

Target auth behavior:

- if `Auth` and `Mail` are enabled, password reset and verification use the framework mail subsystem
- if `Auth` is enabled without `Mail`, local-only development conveniences may still exist
- non-local auth flows should not depend on token echo or log scraping as the intended production path

This keeps auth honest without making `Mail` mandatory for every generated app.

## Operational Defaults

Recommended defaults:

- local/dev default to `log` or Mailpit-friendly SMTP
- provider credentials are commented out by default in `.env`
- from-address config is explicit
- no hidden global rerouting behavior by default

Potential later feature:

- local-only global recipient override for safe staging smoke tests

That is useful, but not required for V1.

## Out Of Scope For V1

These should stay out until the core is proven:

- bulk email campaigns
- unsubscribe management
- deliverability analytics dashboards
- attachment-heavy workflows
- message stream management
- provider failover orchestration
- localization framework beyond basic app-driven template choice

## Proposed Implementation Order

1. Adopt `github.com/goforj/mail` as the sibling repo for the reusable primitive.
2. Add a `Mail` component and env contract in `goforj`.
3. Add GoForj-side manager/wiring on top of `github.com/goforj/mail`.
4. Implement or adopt `log`, `smtp`, and one API transport in the mail repo.
5. Add template rendering and mailables in `goforj`.
6. Add preview support.
7. Add fake/testing support.
8. Wire auth reset/verification onto framework mail.

## Success Criteria

This design is successful when:

- GoForj has a real framework-owned mail subsystem
- auth no longer relies on log-only delivery as the production story
- local developers can preview and inspect mail without sending it
- apps can use SMTP or one supported API provider without rewriting auth flows
- tests can validate mail behavior without live providers

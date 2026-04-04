# Notifications Design Sketch

This document sketches a possible first-class `notifications` primitive for GoForj.

Status:

- exploratory
- not committed to implementation
- intended to clarify the shape before any generator or runtime work begins

## Goal

Provide a first-class outbound notification primitive that lets app owners:

- write notification-producing application code once
- support local and production delivery models with the same app
- centralize formatting and presentation concerns
- swap providers without pushing provider payload details into business logic

The primitive should support the same broad GoForj pattern:

- env-scoped configuration
- generated managers and typed accessors
- compile-time `*_SUPPORTED_DRIVERS`
- runtime `*_DRIVER`

## Core Idea

App code should emit a generic structured notification message.

Drivers should be responsible for:

- transport
- channel-specific rendering
- truncation and fallback behavior
- provider-specific payload mapping

This is closer to structured logging than to hand-building provider-specific request bodies throughout the app.

## Proposed API Shape

Manager-based access:

```go
type Service struct {
	notifications *notifications.Manager
}
```

Usage:

```go
err := s.notifications.Alerts().Send(ctx, notifications.Message{
	Title: "Monitor Down",
	Body:  "Primary API is not responding",
	Level: notifications.LevelError,
	Fields: map[string]any{
		"monitor": "api-primary",
		"region":  "us-east-1",
		"status":  503,
	},
})
```

Important clarification:

- `Alerts()` should represent a notification route
- a route may deliver to one or many channels
- app code should not need to know whether that route fans out to Slack, email, Discord, or any combination of them

Likely generated accessors:

- `app.Notifications().Default()`
- `app.Notifications().Alerts()`
- `app.Notifications().Transactional()`
- `app.Notifications().Ops()`

Suggested route-oriented shape:

```go
type Route struct {
	deliveries []Delivery
}

func (r *Route) Send(ctx context.Context, msg Message) error {
	var errs []error
	for _, delivery := range r.deliveries {
		if err := delivery.Send(ctx, msg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
```

That makes `Alerts()` mean:

- one semantic notification route for the app
- many possible delivery targets under the hood

## Canonical Message

The primitive should define a small, broadly portable envelope.

Possible shape:

```go
type Message struct {
	Title  string
	Body   string
	Fields map[string]any
	Level  Level
	Tags   []string
	Meta   map[string]string
}
```

Possible level enum:

```go
type Level string

const (
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
	LevelSuccess Level = "success"
)
```

What this message should represent:

- human-facing communication intent
- structured context
- transport-agnostic semantics

What it should avoid:

- provider-specific knobs in the core type
- transport-specific fields like SMTP headers or Slack block JSON
- a giant lowest-common-denominator abstraction

## Formatter Model

Each driver should have a formatter or renderer that maps the canonical message into its transport-specific output.

Examples:

- Slack formatter renders title, body, and fields into blocks
- email formatter renders HTML and plain text
- SMS formatter collapses the message into a short title/body form
- webhook formatter emits structured JSON
- log formatter renders structured log output

This creates a clean split:

- app code owns semantic message intent
- driver owns transport and final representation

### Suggested Formatter Interfaces

One possible shape:

```go
type Formatter interface {
	Format(ctx context.Context, msg Message) (any, error)
}
```

That is still too abstract if taken literally, so the stronger implementation direction is:

```go
type Formatter[T any] interface {
	Format(ctx context.Context, msg Message) (T, error)
}

type Driver[T any] interface {
	Send(ctx context.Context, rendered T) error
}

type Delivery interface {
	Send(ctx context.Context, msg Message) error
}

type TypedDelivery[T any] struct {
	driver    Driver[T]
	formatter Formatter[T]
}

func (d *TypedDelivery[T]) Send(ctx context.Context, msg Message) error {
	rendered, err := d.formatter.Format(ctx, msg)
	if err != nil {
		return err
	}
	return d.driver.Send(ctx, rendered)
}
```

That keeps the layering clean:

- app code sends a canonical `Message`
- route fans out to one or more deliveries
- each delivery formats for its transport
- each driver sends its transport-specific payload

Manager shape could look like:

```go
type Manager struct {
	defaultRoute       *Route
	alerts             *Route
	transactional      *Route
}

func (m *Manager) Default() *Route       { return m.defaultRoute }
func (m *Manager) Alerts() *Route        { return m.alerts }
func (m *Manager) Transactional() *Route { return m.transactional }
```

And route construction could look like:

```go
func buildRoute(scope env.Scope) (*Route, error) {
	targets := strings.Split(scope.Get("TARGETS", "log"), ",")
	deliveries := make([]Delivery, 0, len(targets))

	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		delivery, err := buildDelivery(scope.Child(target))
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}

	return &Route{deliveries: deliveries}, nil
}
```

And one delivery could look like:

```go
func buildDelivery(scope env.Scope) (Delivery, error) {
	switch scope.Get("DRIVER", "log") {
	case "slack":
		driver := buildSlackDriver(scope)
		formatter := buildSlackFormatter(scope.Get("FORMATTER", "default"), scope)
		return &TypedDelivery[SlackRender]{
			driver:    driver,
			formatter: formatter,
		}, nil
	case "discord":
		driver := buildDiscordDriver(scope)
		formatter := buildDiscordFormatter(scope.Get("FORMATTER", "default"), scope)
		return &TypedDelivery[DiscordRender]{
			driver:    driver,
			formatter: formatter,
		}, nil
	case "resend":
		driver := buildResendDriver(scope)
		formatter := buildEmailFormatter(scope.Get("FORMATTER", "default"), scope)
		return &TypedDelivery[EmailRender]{
			driver:    driver,
			formatter: formatter,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported notification driver")
	}
}
```

The point is not that every driver uses every field. The point is that the formatter can produce one or more representations, and the driver consumes the representation that makes sense for that transport.

### Formatter Responsibilities

Formatters should own:

- title/body layout
- field ordering
- field display labels
- severity styling
- truncation rules
- link rendering
- transport-specific decoration such as icons or emoji

App code should not need to know:

- how Slack blocks are structured
- how Discord embeds should be assembled
- how HTML email should be laid out

### Example Formatter Policy

Given this message:

```go
notifications.Message{
	Title: "Monitor Down",
	Body:  "Primary API is not responding",
	Level: notifications.LevelError,
	Fields: map[string]any{
		"monitor": "api-primary",
		"region":  "us-east-1",
		"status":  503,
	},
}
```

An email formatter might render:

- subject: `[ERROR] Monitor Down`
- HTML with a title, summary text, and a small field table
- text alternative with simple key/value lines

A Slack formatter might render:

- a leading alert emoji based on level
- title and body as separate sections
- fields as compact block fields
- a colored visual treatment via emoji or markdown emphasis

A Discord formatter might render:

- a level-based emoji like `:rotating_light:`
- a short summary line in the message body
- fields as embed-style name/value pairs
- field-key-specific icons such as:
  - `monitor` -> `:satellite:`
  - `region` -> `:globe_with_meridians:`
  - `status` -> `:warning:`

### Example Formatter Implementations

Typed render outputs:

```go
type EmailRender struct {
	Subject string
	Text    string
	HTML    string
}

type SlackRender struct {
	Text   string
	Blocks []SlackBlock
}

type DiscordRender struct {
	Content string
	Embeds  []DiscordEmbed
}
```

Slack-style formatter sketch:

```go
type SlackFormatter struct {
	KeyLabels map[string]string
	KeyEmoji  map[string]string
}

func (f *SlackFormatter) Format(_ context.Context, msg Message) (SlackRender, error) {
	blocks := []any{
		map[string]any{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*%s*\\n%s", msg.Title, msg.Body),
			},
		},
	}

	fields := make([]map[string]any, 0, len(msg.Fields))
	for key, value := range msg.Fields {
		label := key
		if next, ok := f.KeyLabels[key]; ok && next != "" {
			label = next
		}
		if emoji, ok := f.KeyEmoji[key]; ok && emoji != "" {
			label = emoji + " " + label
		}
		fields = append(fields, map[string]any{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*%s:* %v", label, value),
		})
	}
	if len(fields) > 0 {
		blocks = append(blocks, map[string]any{
			"type":   "section",
			"fields": fields,
		})
	}

	return SlackRender{
		Text:   msg.Title + ": " + msg.Body,
		Blocks: blocks,
	}, nil
}
```

Discord-style formatter sketch:

```go
type DiscordFormatter struct {
	KeyLabels map[string]string
	KeyEmoji  map[string]string
	LevelEmoji map[Level]string
}

func (f *DiscordFormatter) Format(_ context.Context, msg Message) (DiscordRender, error) {
	levelEmoji := f.LevelEmoji[msg.Level]
	title := strings.TrimSpace(strings.Join([]string{levelEmoji, msg.Title}, " "))

	embedFields := make([]map[string]any, 0, len(msg.Fields))
	for key, value := range msg.Fields {
		label := key
		if next, ok := f.KeyLabels[key]; ok && next != "" {
			label = next
		}
		if emoji, ok := f.KeyEmoji[key]; ok && emoji != "" {
			label = emoji + " " + label
		}
		embedFields = append(embedFields, map[string]any{
			"name":   label,
			"value":  fmt.Sprint(value),
			"inline": true,
		})
	}

	return DiscordRender{
		Content: title,
		Embeds: []DiscordEmbed{
			{
				Description: msg.Body,
				Fields:      embedFields,
			},
		},
	}, nil
}
```

Email-style formatter sketch:

```go
type EmailFormatter struct{}

func (f *EmailFormatter) Format(_ context.Context, msg Message) (EmailRender, error) {
	var text strings.Builder
	text.WriteString(msg.Title)
	text.WriteString("\n\n")
	text.WriteString(msg.Body)
	for key, value := range msg.Fields {
		text.WriteString("\n")
		text.WriteString(key)
		text.WriteString(": ")
		text.WriteString(fmt.Sprint(value))
	}

	html := "<h1>" + html.EscapeString(msg.Title) + "</h1>" +
		"<p>" + html.EscapeString(msg.Body) + "</p>"

	return EmailRender{
		Subject: msg.Title,
		Text:    text.String(),
		HTML:    html,
	}, nil
}
```

Those examples are intentionally simple, but they show the intended layering:

- one canonical message type
- formatter-specific decoration and layout rules
- typed render output per transport
- driver-specific transport logic consuming that typed render

### Formatter Profiles

The app may want formatting policy that is distinct from the transport itself.

Example:

- Slack alerts for ops
- Slack digest summaries for business reporting
- Discord community updates
- transactional email

That suggests room for formatter profiles:

```go
NOTIFY_ALERTS_FORMATTER=ops_alert
NOTIFY_COMMUNITY_FORMATTER=community_update
NOTIFY_TRANSACTIONAL_FORMATTER=transactional_email
```

Possible model:

- drivers own transport
- formatter profiles own presentation rules
- one route can combine many deliveries
- each delivery can use its own formatter profile

For example:

- `NOTIFY_ALERTS_TARGETS=slack,email,discord`
- `NOTIFY_ALERTS_SLACK_DRIVER=slack`
- `NOTIFY_ALERTS_SLACK_FORMATTER=ops_alert`
- `NOTIFY_ALERTS_EMAIL_DRIVER=resend`
- `NOTIFY_ALERTS_EMAIL_FORMATTER=ops_email`
- `NOTIFY_ALERTS_DISCORD_DRIVER=discord`
- `NOTIFY_ALERTS_DISCORD_FORMATTER=ops_discord`

This would let one alert route fan out to multiple channels, each with different formatting behavior, without changing app code.

### Field Decoration

Field decoration is a strong fit for formatters.

Example rules:

- level-based icons or emoji
- field-key label maps:
  - `monitor` -> `Monitor`
  - `status` -> `HTTP Status`
  - `region` -> `Region`
- field-key decorations per transport:
  - Slack can prefix keys with emoji
  - Discord can use embed field titles with emoji
  - email can use plain labels without emoji

That means the same canonical message can be rendered differently per destination without putting those presentation rules into application code.

### Important Constraint

Formatters should not become an unbounded template engine too early.

Recommended starting point:

- built-in formatter profiles
- code-level formatter implementations
- simple config-driven label/icon overrides

Avoid initially:

- arbitrary remote template loading
- giant per-driver DSLs
- provider-specific message types escaping into app code

## Why This Model Is Attractive

- app code stays stable when providers change
- formatting is centralized instead of duplicated
- local development works naturally with `log` or `null`
- production can switch providers without rewriting domain logic
- teams can add branded or organization-specific formatting in one place

## Configuration Shape

Likely env shape:

```env
NOTIFY_SUPPORTED_DRIVERS=log,slack,discord,resend
NOTIFY_DRIVER=log

NOTIFY_ALERTS_TARGETS=slack,email,discord
NOTIFY_ALERTS_SLACK_DRIVER=slack
NOTIFY_ALERTS_EMAIL_DRIVER=resend
NOTIFY_ALERTS_DISCORD_DRIVER=discord

NOTIFY_TRANSACTIONAL_TARGETS=email
NOTIFY_TRANSACTIONAL_EMAIL_DRIVER=resend
```

Meaning:

- generated app supports `log`, `slack`, `discord`, and `resend`
- default notification transport logs locally
- `alerts` fans out to Slack, email, and Discord
- `transactional` sends email only

Same pattern as other primitives:

- `NOTIFY_SUPPORTED_DRIVERS` controls compile-time generated support
- `NOTIFY_DRIVER` and `NOTIFY_<NAME>_DRIVER` control runtime selection

## Candidate Drivers

Possible first set:

- `log`
- `null`
- `smtp`
- `resend`
- `postmark`
- `ses`
- `twilio`
- `slack`
- `discord`
- `webhook`

These are not all equivalent, which is why the canonical message plus formatter model matters.

## Important Design Constraint

Notifications are not as uniform as cache, storage, queue, or lock.

Examples:

- email has subject, HTML, text, sender, reply-to
- SMS has tighter length and simpler formatting
- chat tools prefer block/attachment-style rendering
- webhooks often want the raw structured payload

So the primitive should not pretend every driver exposes the same native payload shape.

Instead:

- keep the canonical message small
- let formatters adapt it
- keep provider-specific escape hatches minimal and deliberate

## Open Questions

1. Should this primitive be called `notifications`, `delivery`, or `outbound`?
2. Should the manager expose one generic `Send(...)`, or support richer typed helper methods later?
3. Should formatting be driver-owned only, or should the app be able to register named formatter profiles?
4. How much provider-specific metadata should be allowed on the canonical message?
5. Should templates/HTML rendering live inside the primitive, or above it in app code?

## Current Recommendation

If GoForj adds notifications as a primitive, the best version is:

- one canonical structured notification message
- route-based fanout to one or many deliveries
- per-driver renderer/formatter layer
- generated manager with named route accessors
- local/dev-friendly `log` and `null` drivers
- provider-specific payload logic kept out of app/business code

That is the cleanest path to making notifications feel like a real GoForj primitive instead of a bag of adapters.

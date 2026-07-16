# Auth

GoForj ships auth as a first-class framework component for web apps.

The baseline `Auth` component gives you:

- local username/email + password sign-in
- cookie-based browser auth with `HttpOnly` cookies
- short-lived access JWTs plus server-authoritative refresh sessions
- `me`, logout, logout-all, session listing, and per-session revoke
- password change
- password reset
- email verification
- login rate limiting and temporary account lockout
- auth-owned cleanup of stale auth rows

## Components

GoForj separates baseline auth from OAuth provider support.

- `Auth`: local accounts, sessions, password flows, verification, lockout, cleanup
- `OAuth`: optional provider sign-in and account linking on top of `Auth`

Rules:

- `Auth` requires `WebAPI` and a database
- `OAuth` requires `Auth` and a database

If you enable `Auth` without `OAuth`, generated apps do not include provider routes, provider files, or OAuth migrations.

## Generated HTTP API

Baseline auth routes:

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/logout-all`
- `POST /api/v1/auth/refresh`
- `GET /api/v1/auth/me`
- `POST /api/v1/auth/profile`
- `GET /api/v1/auth/sessions`
- `POST /api/v1/auth/sessions/:id/revoke`
- `POST /api/v1/auth/change-password`
- `POST /api/v1/auth/password-reset/request`
- `POST /api/v1/auth/password-reset/confirm`
- `POST /api/v1/auth/email-verification/request`
- `POST /api/v1/auth/email-verification/confirm`

OAuth routes when the `OAuth` component is enabled:

- `GET /api/v1/auth/oauth/:provider/start`
- `GET /api/v1/auth/oauth/:provider/callback`
- `POST /api/v1/auth/oauth/:provider/callback`
- `GET /api/v1/auth/oauth/:provider/link/start`

## Session Model

GoForj auth is server-authoritative.

- access tokens are short-lived JWTs
- refresh tokens are opaque
- refresh tokens are backed by `auth_sessions`
- logout and revoke operate on persisted session rows
- refresh rotation is mandatory

This means JWTs are not treated as the only source of truth.

## OAuth

The optional `OAuth` component currently includes built-in adapters for:

- GitHub
- Google
- Microsoft
- Apple

OAuth policy defaults:

- exact provider subject matches reuse the linked user
- same-email provider sign-in does not auto-link to an existing user
- linking is explicit
- provider-created users can exist before a local password is set

Generated `.env` files do not include OAuth credential stubs. Providers remain
disabled until their required credentials are configured, and each generated
app documents the GitHub, Google, Microsoft, and Apple credential matrix in
`internal/auth/README.md`.

## Local Development

Local auth remains easy to use:

- local bootstrap user support exists for development flows
- explicit auth commands exist for user creation and password management
- reset and verification tokens can be returned directly in local workflows

Non-local environments should use explicit operator setup and real mail/provider delivery paths.

## Testing

GoForj keeps auth heavily tested at the framework layer:

- generated auth package integration tests
- rendered-app integration across SQLite, MySQL, and Postgres
- session, reset, verification, lockout, and cleanup coverage
- fake/stub provider tests for OAuth behavior

Live third-party provider round-trips are treated as manual smoke validation, not CI requirements.

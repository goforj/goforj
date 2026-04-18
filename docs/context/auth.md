# Auth

This document captures the current generated auth model and the intended direction so future work does not reopen core design questions unnecessarily.

## Goal

Auth should be:

- secure by default
- simple in the generated app surface
- extensible enough to support local login first and additional providers later
- stable enough that future work adds capabilities without rewriting the session model

## Current Shape

The generated auth package currently implements:

- local username/email + password login
- cookie-based browser auth
- short-lived access token
- longer-lived refresh token
- session persistence in `auth_sessions`
- bootstrap local admin support for development and controlled environments

Main files:

- [service.go.tmpl](/workspace/code/goforj/templates/internal/auth/service.go.tmpl)
- [controller.go.tmpl](/workspace/code/goforj/templates/internal/auth/controller.go.tmpl)
- [user.go.tmpl](/workspace/code/goforj/templates/internal/auth/user.go.tmpl)
- [session.go.tmpl](/workspace/code/goforj/templates/internal/auth/session.go.tmpl)
- [service_integration_test.go.tmpl](/workspace/code/goforj/templates/internal/auth/service_integration_test.go.tmpl)

## HTTP Surface

Current routes:

- `POST /auth/login`
- `POST /auth/logout`
- `POST /auth/refresh`
- `GET /auth/me`

Behavior:

- `login` validates `login` + `password`
- `logout` revokes the current session when it can identify one
- `refresh` rotates the refresh secret and reissues cookies
- `me` is protected by auth middleware and returns the sanitized current user

Auth middleware path:

- `RequireAuth()` first tries the access cookie
- if access is missing or expired, it attempts refresh
- if refresh succeeds, cookies rotate and request continues
- otherwise cookies are cleared and request returns `401`

## Token And Session Model

### Access token

Access token is:

- JWT signed with `API_JWT_SECRET_KEY`
- stored in the `auth_access` cookie
- short-lived

Claims include:

- user id
- session id
- username
- standard `sub`, `iat`, `exp`

This means the access token is not the source of truth by itself. It is still tied to a persisted session row.

### Refresh token

Refresh token is:

- opaque from the app point of view
- stored in the `auth_refresh` cookie
- built as `sessionID.secret`
- validated by hashing only the secret and comparing against `auth_sessions.refresh_token_hash`

Important property:

- raw refresh secrets are not persisted
- only the hash is stored

### Session row

`auth_sessions` is the revocation and rotation anchor.

Current fields include:

- `id`
- `user_id`
- `refresh_token_hash`
- `expires_at`
- `revoked_at`
- `last_seen_at`
- `user_agent`
- `ip_address`

This is what lets the system:

- revoke sessions on logout
- reject expired sessions even if a JWT exists
- rotate refresh secrets safely
- update last seen metadata

## User Model

Current generated `users` shape is local-auth oriented:

- `username`
- `email`
- `display_name`
- `password_hash`
- `active`

`ByLogin` resolves by normalized username or email.

Current assumptions:

- username and email are the local identifiers
- password hash is only for local credential auth
- inactive users must not authenticate even with otherwise-valid credentials or sessions

## Security Properties To Preserve

These are not optional implementation details.

### 1. Invalid credentials must not leak which part failed

Current behavior intentionally collapses:

- missing user
- wrong password
- inactive user

into `ErrInvalidCredentials`.

Keep that property.

### 2. Sessions remain server-authoritative

Do not drift toward “JWT alone is enough”.

The current model checks the persisted session on access-token use and refresh-token use.

Keep revocation and expiry server-authoritative.

### 3. Refresh rotation is mandatory

`RefreshSession` rotates:

- refresh secret hash
- refresh expiry
- session metadata

Do not allow reusable long-lived refresh values if avoidable.

### 4. Raw refresh secrets are not persisted

Only the hash is stored in `auth_sessions`.

Keep it that way.

### 5. Cookies are `HttpOnly`

Current cookie behavior:

- `HttpOnly: true`
- `SameSite: Lax`
- `Secure` controlled by `AUTH_COOKIE_SECURE`

If future auth UX changes, do not casually relax these defaults.

### 6. Middleware should clear bad cookies

Current auth middleware and refresh/login flows clear cookies when auth is invalid.

Keep invalid browser state self-healing where possible.

## Environment Contract

Current auth env keys:

- `API_JWT_SECRET_KEY`
- `AUTH_ACCESS_TTL`
- `AUTH_REFRESH_TTL`
- `AUTH_COOKIE_SECURE`
- `AUTH_BOOTSTRAP_ENABLED`
- `AUTH_BOOTSTRAP_USERNAME`
- `AUTH_BOOTSTRAP_EMAIL`
- `AUTH_BOOTSTRAP_PASSWORD`

Defaults:

- access TTL: `15m`
- refresh TTL: `720h` via fallback path (`30 * 24h`)
- cookie secure: `auto`

`AUTH_COOKIE_SECURE=auto` means:

- secure on HTTPS
- not secure on plain HTTP

That is correct for local dev ergonomics.

## Bootstrap User Policy

`EnsureBootstrapUserFromEnv()` exists for:

- local development
- intentional bootstrap in controlled non-local environments

Current policy:

- username/password must be present
- local env can bootstrap by default
- non-local env requires `AUTH_BOOTSTRAP_ENABLED=true`
- bootstrap is a no-op if the user already exists
- bootstrap also no-ops when auth tables do not exist yet

That is the right safety posture.

Do not broaden bootstrap behavior casually.

## What Exists In Tests

The generated auth integration coverage currently verifies:

- bootstrap user creation
- login by username
- login by email
- invalid/inactive user rejection
- controller login/me/logout flow
- session cookies are set
- logout revokes the persisted session
- expired access token can transparently refresh through middleware
- refresh rotates refresh token hash and updates `last_seen_at`

Primary file:

- [service_integration_test.go.tmpl](/workspace/code/goforj/templates/internal/auth/service_integration_test.go.tmpl)

This is the main auth safety net today.

## Direction For Additional Providers

The current system should be treated as:

- a session and user identity core
- with one implemented credential type: local password login

Future providers should plug into that core, not replace it.

### Recommended model

Keep:

- `users` as the principal
- `auth_sessions` as the session authority

Add a provider identity layer instead of overloading local fields.

Likely next table/model:

- `auth_identities`

Suggested shape:

- `id`
- `user_id`
- `provider`
- `provider_subject`
- `email`
- `username`
- `metadata`
- `created_at`
- `updated_at`

With uniqueness around:

- `(provider, provider_subject)`

That gives a clean path for:

- local auth
- OAuth/OpenID providers
- magic-link or external identity providers later

### Important rule

Do not try to force every future provider into `users.password_hash`.

Local password auth should become one provider path, not the whole identity model.

### Durable split

Think of it as:

- `users`
  - principal/profile
- `auth_identities`
  - login methods / provider linkage
- `auth_sessions`
  - browser or client sessions

That is the shape most likely to avoid major rework later.

## What Should Probably Not Change Again Soon

- cookie-based browser auth as the default generated mode
- JWT access + persisted session authority
- refresh-token rotation
- login by username or email
- inactive-user rejection
- bootstrap admin guarded by env policy

Those are sound defaults.

## Likely Future Work

Reasonable next auth additions:

- provider identity table and repo
- password reset / recovery flow
- email verification if needed
- session listing and revocation UI
- optional API token or personal access token model if generated apps need non-browser auth

Do those as additions around the current session core, not by rewriting the core.

## Working Rule

If future auth work weakens revocation, rotation, or cookie safety for convenience, it is probably the wrong direction.

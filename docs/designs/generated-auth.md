# Generated Auth Design

## Purpose

This document is the design and implementation backlog for first-class generated auth in GoForj web apps.

The goal is not just "demo login works." The goal is a framework auth surface that:

- ships as a first-class `Auth` component
- works cleanly for local username/password login by default
- uses secure cookies instead of browser-managed tokens in local storage
- supports short-lived JWT access tokens plus refresh-backed session renewal
- is structured so additional auth capabilities and future providers can be added without redesigning the core account model

## Scope

The `Auth` component owns:

- auth-related migrations
- generated `internal/auth` code
- auth routes, middleware, and current-user context
- cookie and JWT issuance/verification
- session lifecycle policy
- password reset and email verification token lifecycle
- login rate-limit and lockout policy
- bootstrap operator user creation for local/dev workflows
- auth-owned scheduled cleanup

The `Auth` component should not implicitly mean:

- full admin UI
- roles/permissions
- OAuth providers

OAuth now exists as a separate optional component layered on top of `Auth`.

## Current Baseline

The generated auth baseline now includes:

- local username/email + password login
- server-authoritative `auth_sessions`
- short-lived access JWT plus opaque refresh token
- cookie-based browser auth with secure defaults
- in-flight refresh on expired access token
- session listing and per-session revoke
- logout-all
- password change
- password reset request + confirm
- email verification request + confirm
- login rate limiting and temporary account lockout
- bootstrap local admin creation
- explicit auth CLI user-management commands
- scheduled cleanup of stale auth rows

The separate `OAuth` component now includes:

- `auth_identities`
- `auth_oauth_states`
- provider registry and callback state handling
- built-in GitHub, Google, Microsoft, and Apple provider adapters
- OAuth login and explicit link flows
- provider-specific tests via fakes and stub servers

Dependency rules:

- `Auth` requires `WebAPI` and a database
- `Auth` implies `Mail`
- `OAuth` requires `Auth` and a database

## Canonical Data Model

Current auth-owned baseline model:

- `users`
- `auth_sessions`
- `auth_password_resets`
- `auth_email_verifications`
- `auth_login_attempts`

OAuth-owned model when the `OAuth` component is enabled:

- `auth_identities`
- `auth_oauth_states`

Current `users` fields:

- `id`
- `username`
- `email`
- `display_name`
- `avatar_url`
- `password_hash`
- `active`
- `failed_login_attempts`
- `email_verified_at`
- `last_login_at`
- `last_seen_at`
- `locked_until`
- `timezone`
- `locale`
- `created_at`
- `updated_at`

Model intent:

- `users` is the canonical app account table
- `auth_sessions` owns refresh/session lifecycle state
- password reset and email verification grants are explicit auth tables
- login pressure state is explicit and persistent
- `auth_identities` links external providers to canonical users without replacing the core user model
- application code should key off `users.id`, not session identifiers

Provider-ready policy:

- do not auto-link provider identities to existing users by shared email
- require explicit linking for same-email provider sign-in
- allow provider-created users to exist with no local password by using an unusable password hash until one is set

Current provider baseline:

- GitHub
- Google
- Microsoft
- Apple

## Session And Token Model

Default auth flow:

1. user submits local credentials to `POST /api/v1/auth/login`
2. auth service verifies credentials and policy state
3. auth service creates or rotates a server-side `auth_sessions` row
4. response sets:
   - short-lived access JWT cookie
   - longer-lived refresh cookie
5. protected requests use access JWT first
6. if access JWT is expired but refresh cookie is valid, middleware refreshes the session and continues the request

Important defaults:

- access token is short-lived
- refresh token is opaque, not another long-lived JWT
- refresh token is hashed server-side
- refresh rotation is explicit and tested
- logout revokes the server-side session

## Cookie Model

Preferred defaults:

- `HttpOnly`
- `SameSite=Lax`
- `Secure` when appropriate for the environment
- path `/`

Framework rule:

- no `localStorage` access tokens by default
- browser cookie handling is the standard generated path

## Generated HTTP Contract

Current generated auth API:

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/logout-all`
- `POST /api/v1/auth/refresh`
- `GET /api/v1/auth/me`
- `GET /api/v1/auth/sessions`
- `POST /api/v1/auth/sessions/:id/revoke`
- `POST /api/v1/auth/change-password`
- `POST /api/v1/auth/password-reset/request`
- `POST /api/v1/auth/password-reset/confirm`
- `POST /api/v1/auth/email-verification/request`
- `POST /api/v1/auth/email-verification/confirm`

Current generated OAuth API when the `OAuth` component is enabled:

- `GET /api/v1/auth/oauth/:provider/start`
- `GET /api/v1/auth/oauth/:provider/callback`
- `POST /api/v1/auth/oauth/:provider/callback`
- `GET /api/v1/auth/oauth/:provider/link/start`

Protected route behavior:

- unauthenticated request returns `401`
- expired access token with valid refresh session renews and proceeds
- invalid refresh path returns `401`
- account lockout returns `423`
- login rate limit returns `429`

## Token Exposure Rules

Password reset and email verification tokens are not assumed safe to echo in production.

Current rule:

- local app envs expose raw reset/verification tokens for developer workflows by default
- non-local envs must opt in explicitly
- OAuth env entries are present only when the `OAuth` component is enabled, and provider credentials are commented out by default

That keeps the generated auth contract usable without mail infrastructure while avoiding an unsafe production default.

## Cleanup And Operations

Auth owns cleanup through its own service hook and scheduler registration.

Current cleanup removes stale rows from:

- `auth_sessions`
- `auth_password_resets`
- `auth_email_verifications`
- `auth_login_attempts`

When `OAuth` is enabled, cleanup also removes stale rows from:

- `auth_oauth_states`

That cleanup is data hygiene, not the source of truth for validity. Expired/revoked sessions and expired/used tokens are already rejected at runtime.

## Testing Strategy

Confidence should come from testing the framework contract, not from talking to third-party providers in CI.

Current primary layers:

- generator/rendered-app integration tests across SQLite, MySQL, and Postgres
- generated auth package integration tests
- route and middleware behavior tests
- cookie behavior tests
- policy tests for reset, verification, rate limiting, and lockout
- fake-provider contract tests for auth-owned provider flow behavior
- stub-provider tests for GitHub, Google, Microsoft, and Apple adapters

Provider verification rule:

- keep real provider checks as manual smoke tests only
- CI should not depend on live third-party identity services

## Operational Notes

Bootstrap user behavior must not make startup fragile before migrations exist.

That means:

- bootstrap user creation safely no-ops before auth tables exist
- env bootstrap is local-only
- migration ordering stays deterministic
- auth cleanup stays inside auth-owned code paths

For explicit non-local user management, the generated command surface should be the operator path:

- `auth:create-user`
- `auth:set-password`
- `auth:bootstrap`

## Design Principles

When implementing auth in GoForj, prefer:

- secure defaults
- simple mental model
- explicit session state
- generated code that can be read and owned by app developers
- feature-gated behavior via `components.auth` and `components.oauth`

Avoid:

- `localStorage` token patterns by default
- hiding auth behavior inside unrelated components
- premature provider complexity when local auth is the only shipped mode
- CI dependence on real external identity providers

## Implementation Status

Completed foundation:

- [x] auth-enabled rendering and generated auth package
- [x] local login/logout/refresh/me
- [x] expanded user model
- [x] server-authoritative refresh/session store
- [x] cookie hardening and fail-closed JWT config
- [x] transparent refresh through middleware
- [x] session listing, revoke, and logout-all
- [x] password change
- [x] password reset request + confirm
- [x] email verification request + confirm
- [x] login rate limiting and account lockout
- [x] auth-owned scheduler cleanup
- [x] provider-ready identity model and fake-provider service tests
- [x] OAuth component split from baseline auth
- [x] built-in GitHub, Google, Microsoft, and Apple adapters
- [x] OAuth callback state model and routes
- [x] generated-app integration across SQLite/MySQL/Postgres

Still open:

- [ ] document generated auth routes/envs in user-facing docs if needed beyond internal docs
- [ ] document provider callback URL setup for the built-in OAuth providers
- [ ] decide whether bearer-token auth should exist alongside cookie auth
- [ ] decide whether refresh retry belongs only in middleware or also in frontend wrappers
- [ ] add admin/operator CLI for user/session lifecycle
- [ ] add provider-specific start/callback routes and OAuth state handling
- [ ] add Google provider implementation
- [ ] add GitHub/Facebook/etc. provider implementations as separate slices

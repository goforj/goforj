# Generated Auth Design

## Purpose

This document is the design and implementation backlog for first-class generated auth in GoForj web apps.

The goal is not just "demo login works." The goal is a framework auth surface that:

- ships as a first-class `Auth` component
- works cleanly for local username/password login by default
- uses secure cookies instead of browser-managed tokens in local storage
- supports short-lived JWT access tokens plus refresh-backed session renewal
- is structured so OAuth/OIDC providers can be added later without redesigning the core account model

## Scope

The `Auth` component should own:

- auth-related migrations
- generated `internal/auth` code
- auth routes such as login/logout/refresh/me
- auth middleware and current-user context
- cookie and JWT issuance/verification
- bootstrap operator user creation for local/dev workflows

The `Auth` component should not implicitly mean:

- full admin UI
- roles/permissions
- password reset
- email verification
- OAuth providers

Those can layer on later.

## Current Direction

The intended generated baseline is:

- `components.auth` enables auth generation
- local auth is the default v1 provider
- generated apps use secure `HttpOnly` cookies
- access tokens are short-lived JWTs
- refresh tokens are opaque and backed by a server-side session row
- protected routes use auth middleware
- expired access tokens can be renewed in flight when a valid refresh cookie exists

## Canonical Data Model

Current baseline model:

- `users`
- `auth_sessions`

Recommended near-term expansion for `users`:

- `id`
- `username`
- `email`
- `display_name`
- `avatar_url`
- `password_hash`
- `active`
- `email_verified_at`
- `last_login_at`
- `last_seen_at`
- `timezone`
- `locale`
- `created_at`
- `updated_at`

Model intent:

- `users` is the canonical app account table
- `auth_sessions` owns refresh/session lifecycle state
- application code should key off `users.id`, not session identifiers

## Session and Token Model

Planned default auth flow:

1. user submits local credentials to `POST /api/v1/auth/login`
2. auth service verifies credentials
3. auth service creates or rotates a server-side `auth_sessions` row
4. response sets:
   - short-lived access JWT cookie
   - longer-lived refresh cookie
5. protected requests use access JWT first
6. if access JWT is expired but refresh cookie is valid, middleware refreshes the session and continues the request

Important defaults:

- access token should be short-lived
- refresh token should be opaque, not another long-lived JWT
- refresh token should be hashed server-side
- refresh rotation should be explicit and tested
- logout should revoke the server-side session

## Cookie Model

Preferred defaults:

- `HttpOnly`
- `SameSite=Lax` unless a stronger/weaker policy is explicitly needed
- `Secure` when appropriate for the environment
- path-limited where practical

Framework rule:

- no localStorage access tokens by default
- browser cookie handling should be the standard generated path

## Generated HTTP Contract

The minimum generated auth API should be:

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/refresh`
- `GET /api/v1/auth/me`

Protected route behavior:

- unauthenticated request returns `401`
- expired access token with valid refresh session renews and proceeds
- invalid refresh path returns `401`

## Frontend Contract

Generated frontend behavior should be:

- load current user once on boot where appropriate
- use one shared authenticated fetch wrapper
- rely on cookies automatically
- retry at most once after refresh-related `401`
- redirect to `/login` only when the session is truly gone

The demo app should prove this model first, but the design belongs to the generated framework, not only the demo.

## Component Boundary

Auth should remain a dedicated `Auth` component, not be folded into `WebAPI` or `User`.

Reasoning:

- it owns migrations, routes, middleware, cookies, sessions, and auth policy
- `user` is too narrow a name for the feature boundary
- not every generated web app should be forced to include auth unless the component is enabled

## Provider-Ready Direction

Local auth is the only required v1 mode, but the framework should be structured so provider auth is addable later.

If provider support is introduced, the likely future shape is:

- `users`
- `auth_sessions`
- optionally `auth_identities`

Design intent:

- keep `users` as the canonical account record
- keep provider linkage out of the semantic meaning of `users`
- add `auth_identities` only when provider support becomes real, unless early future-proofing is judged worth the extra table

## Testing Strategy

Confidence should come from testing the framework contract, not from talking to Google in CI.

Primary test layers:

- generator tests for auth-enabled renders
- generated-app integration tests for login/logout/me/protected routes
- expiry and refresh tests for in-flight renewal
- cookie behavior tests
- auth service tests for session rotation and revocation

If provider support lands later:

- add fake-provider contract tests
- add callback-flow tests against a stub OAuth provider
- keep real provider checks as manual smoke tests only

## Operational Notes

The framework should support a bootstrap user path for local/dev environments, but it must not make startup fragile before migrations exist.

That means:

- bootstrap user creation must safely no-op before auth tables exist
- migration ordering needs to be deterministic
- legacy user-migration cleanup needs to remain part of the renderer so apps do not end up with conflicting schemas

## Design Principles

When implementing auth in GoForj, prefer:

- secure defaults
- simple mental model
- explicit session state
- generated code that can be read and owned by app developers
- feature-gated behavior via `components.auth`

Avoid:

- localStorage token patterns by default
- hiding auth behavior inside unrelated components
- premature provider complexity when local auth is the only shipped mode
- CI dependence on real external identity providers

## Implementation Tasks

## Current Baseline

- [ ] Commit the current `components.auth` groundwork in clean slices.
- [ ] Commit the current local auth backend foundation in clean slices.
- [ ] Commit the current demo login prove-out in clean slices.

## Immediate Cleanup

- [ ] Add focused renderer tests for `components.auth`:
  - [ ] auth-only app render
  - [ ] auth + web api render
  - [ ] auth + demo app render
- [ ] Add generated-app integration coverage for login success.
- [ ] Add generated-app integration coverage for invalid credentials.
- [ ] Add generated-app integration coverage for protected route `401`.
- [ ] Add generated-app integration coverage for protected route success after login.
- [ ] Add generated-app integration coverage for access-token expiry with in-flight refresh.
- [ ] Document generated auth env vars and defaults.

## User Model

- [ ] Expand `users` with `display_name`.
- [ ] Expand `users` with `avatar_url`.
- [ ] Expand `users` with `email_verified_at`.
- [ ] Expand `users` with `last_login_at`.
- [ ] Expand `users` with `last_seen_at`.
- [ ] Expand `users` with `timezone`.
- [ ] Expand `users` with `locale`.
- [ ] Keep `active` as part of the default user model.
- [ ] Update generated migrations for MySQL.
- [ ] Update generated migrations for Postgres.
- [ ] Update generated migrations for SQLite.
- [ ] Update auth service responses to return the expanded user shape.
- [ ] Add tests for bootstrap user creation with the expanded schema.

## Session Model

- [ ] Keep `auth_sessions` as the default refresh/session store.
- [ ] Ensure `auth_sessions.last_seen_at` is updated during authenticated requests.
- [ ] Ensure `auth_sessions.user_agent` is stored consistently.
- [ ] Ensure `auth_sessions.ip_address` is stored consistently.
- [ ] Add explicit session revocation tests.
- [ ] Add logout-all-sessions capability at the service layer.
- [ ] Add session cleanup/retention command or background cleanup path.

## Frontend Auth Contract

- [ ] Centralize authenticated fetch logic for protected frontend views.
- [ ] Add a shared current-user composable/store.
- [ ] Ensure current-user state is loaded once on app boot.
- [ ] Add route guards for protected demo pages.
- [ ] Add logout behavior in app chrome/user menu.
- [ ] Add redirect-back-to-original-route after login.
- [ ] Add login failure UX polish.
- [ ] Add signed-out/session-expired UX polish.

## Cookie and JWT Hardening

- [ ] Add tests for `HttpOnly` cookie behavior.
- [ ] Add tests for `SameSite` cookie behavior.
- [ ] Add tests for `Secure` cookie behavior.
- [ ] Add tests for expiry/max-age behavior.
- [ ] Add access-token expiry tests.
- [ ] Add refresh-token rotation tests.
- [ ] Add replay/reuse protection tests for revoked or rotated refresh tokens.
- [ ] Decide whether refresh rotation revokes the previous session row or mutates in place.
- [ ] Consider token versioning or session fingerprinting if needed.

## Password and Account Flows

- [ ] Add password change flow for authenticated users.
- [ ] Add password reset request flow.
- [ ] Add password reset confirm flow.
- [ ] Add email verification request flow.
- [ ] Add email verification confirm flow.
- [ ] Add inactive/suspended user handling tests.
- [ ] Add rate limiting for login.
- [ ] Add rate limiting for refresh.

## Provider-Ready Expansion

- [ ] Decide whether to add `auth_identities` now or defer until first provider integration.
- [ ] If added, create `auth_identities` migrations for MySQL.
- [ ] If added, create `auth_identities` migrations for Postgres.
- [ ] If added, create `auth_identities` migrations for SQLite.
- [ ] Keep `users` as the canonical app account table.
- [ ] Keep `auth_sessions` as the session/refresh lifecycle table.

## Provider Abstraction

- [ ] Define a provider contract at the service layer.
- [ ] Add local auth as one provider implementation under that contract.
- [ ] Add a fake/stub provider implementation for tests.
- [ ] Keep provider-specific HTTP/token exchange code outside the core auth service.

## OAuth / Provider Test Strategy

- [ ] Add identity model tests:
  - [ ] create user with local identity
  - [ ] create user with provider identity
  - [ ] link multiple identities to one user
  - [ ] reject duplicate `(provider, provider_subject)`
- [ ] Add provider auth service tests:
  - [ ] first provider login creates user + identity
  - [ ] repeat provider login reuses existing identity
  - [ ] inactive user cannot authenticate
- [ ] Add callback integration tests against a fake OAuth provider.
- [ ] Add policy tests for same email, different provider.
- [ ] Add policy tests for missing provider email.
- [ ] Add policy tests for unverified provider email.
- [ ] Add policy tests for linking provider to existing signed-in user.
- [ ] Keep real Google/GitHub checks as manual smoke tests, not primary CI coverage.

## Admin / Operator Concerns

- [ ] Add generated CLI command to create a user.
- [ ] Add generated CLI command to reset a password.
- [ ] Add generated CLI command to deactivate/reactivate a user.
- [ ] Add generated CLI command to list sessions.
- [ ] Add generated CLI command to revoke session(s).
- [ ] Add Lighthouse/admin UI support for users later.
- [ ] Add Lighthouse/admin UI support for sessions later.
- [ ] Add Lighthouse/admin UI support for auth diagnostics later.

## Open Design Decisions

- [ ] Decide whether local auth remains cookie-only by default or also supports bearer tokens.
- [ ] Decide whether refresh on expired access happens only in middleware or also through explicit frontend retry.
- [ ] Decide whether demo app should always imply `Auth` or whether that should become a separate toggle.
- [ ] Decide whether same-email provider identities auto-link or require explicit linking.
- [ ] Decide the default session lifetime for generated apps.
- [ ] Decide the default refresh rotation policy for generated apps.

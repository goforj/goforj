# Auth

This document captures the current generated auth model so future work extends it coherently instead of reopening solved design choices.

## Goal

Generated auth should be:

- secure by default
- simple to own in generated app code
- explicit about server-side session state
- extensible enough to support local auth first and more providers later

## Current Shape

The generated auth package currently implements:

- local username/email + password login
- cookie-based browser auth with `HttpOnly` cookies
- short-lived JWT access tokens
- opaque refresh tokens backed by server-side session rows
- session listing, per-session revoke, current-session logout, and logout-all
- password change with revocation of other sessions
- password reset request + confirm
- email verification request + confirm
- login rate limiting and temporary account lockout
- provider-ready `auth_identities` linkage plus provider service abstractions
- bootstrap local admin support for development and controlled environments
- auth-owned scheduled cleanup for stale auth rows

Component model:

- `Auth` is the baseline generated account/session system
- `Mail` is the generated outbound mail component used by auth delivery
- `OAuth` is a separate optional component layered on top of `Auth`
- `Auth` implies `Mail`
- `OAuth` requires both `Auth` and a database
- plain auth apps should render and run without any provider files, routes, or migrations

Main files:

- [service.go.tmpl](/workspace/code/goforj/templates/internal/auth/service.go.tmpl)
- [controller.go.tmpl](/workspace/code/goforj/templates/internal/auth/controller.go.tmpl)
- [user.go.tmpl](/workspace/code/goforj/templates/internal/auth/user.go.tmpl)
- [session.go.tmpl](/workspace/code/goforj/templates/internal/auth/session.go.tmpl)
- [password_reset.go.tmpl](/workspace/code/goforj/templates/internal/auth/password_reset.go.tmpl)
- [email_verification.go.tmpl](/workspace/code/goforj/templates/internal/auth/email_verification.go.tmpl)
- [login_attempt.go.tmpl](/workspace/code/goforj/templates/internal/auth/login_attempt.go.tmpl)
- [identity.go.tmpl](/workspace/code/goforj/templates/internal/auth/identity.go.tmpl)
- [service_integration_test.go.tmpl](/workspace/code/goforj/templates/internal/auth/service_integration_test.go.tmpl)

## HTTP Surface

Current routes:

- `POST /auth/login`
- `POST /auth/logout`
- `POST /auth/logout-all`
- `POST /auth/refresh`
- `GET /auth/me`
- `POST /auth/profile`
- `GET /auth/sessions`
- `POST /auth/sessions/:id/revoke`
- `POST /auth/change-password`
- `POST /auth/password-reset/request`
- `POST /auth/password-reset/confirm`
- `POST /auth/email-verification/request`
- `POST /auth/email-verification/confirm`

OAuth-only routes when the `OAuth` component is enabled:

- `GET /auth/oauth/:provider/start`
- `GET /auth/oauth/:provider/callback`
- `POST /auth/oauth/:provider/callback`
- `GET /auth/oauth/:provider/link/start`

Route behavior:

- `login` validates local credentials and sets auth cookies
- `logout` revokes the current session when one can be identified
- `logout-all` revokes every active session for the authenticated user
- `refresh` rotates the refresh secret and reissues cookies
- `me` returns the sanitized authenticated user payload
- `sessions` returns the authenticated user’s active sessions plus a derived `device_label`
- `sessions/:id/revoke` revokes one owned session
- `change-password` verifies the current password, updates the hash, and revokes other sessions
- `password-reset/request` creates a reset grant without leaking account existence
- `password-reset/confirm` redeems a reset grant, updates the password, and revokes sessions
- `email-verification/request` creates a verification grant for the authenticated user
- `email-verification/confirm` redeems a verification grant and sets `email_verified_at`

Auth middleware path:

- `RequireAuth()` first tries the access cookie
- if access is missing or expired, it attempts refresh
- if refresh succeeds during normal protected-request auth, the request continues without rotating the refresh secret
- explicit `POST /auth/refresh` remains the path that rotates the refresh secret
- explicit refresh rotation also stamps `auth_sessions.refresh_rotated_at`
- failed protected requests do not clear cookies anymore
- failed explicit refresh does not clear cookies anymore
- if refresh fallback still fails, the request returns `401`

Recent auth-race hardening:

- request auth now uses a per-session refresh lock so concurrent requests do not rotate the same session in parallel
- request auth also tolerates a very short same-session replay grace window for concurrent browser bursts immediately after access expiry
- this was added to address the real demo-app failure mode where a busy monitoring page could intermittently fall back to `/login` at the access-token boundary
- the main symptom in debug logs was:
  - `access_cookie_rejected` with `missing_access_cookie`
  - followed by `refresh_cookie_rejected` with `refresh_hash_mismatch`
  - followed by `request_auth_failed` / `401`
- the current generated contract is:
- middleware refresh recovers the request and reissues an access cookie
- middleware refresh only writes a refresh cookie when the presented refresh secret is still current
- middleware refresh does not rotate the refresh secret
- middleware refresh does not change `refresh_rotated_at`
- explicit refresh rotates the refresh secret
- explicit refresh updates `refresh_rotated_at`

Auth request debug visibility:

- auth failure/recovery paths now emit request-scoped debug logs through the app logger
- when debug logging is enabled, those entries attach to the request inspect timeline
- this helper is intentionally allowlist-based
- useful fields now include:
  - `auth_outcome`
  - `auth_reason`
  - `access_ttl`
  - `session_expires_at`
  - `session_idle_expires_at`
  - `session_last_seen_at`
  - request `path`, `method`, `ip_address`, `user_agent`
- this path must never log:
  - raw access tokens
  - raw refresh tokens
  - password hashes
  - password reset tokens
  - email verification tokens
  - OAuth provider tokens
  - full cookie values or auth headers
- this debug path was added specifically so session/refresh failures can be diagnosed from the first `401` inspect instead of only from stdout

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
- `refresh_rotated_at`
- `expires_at`
- `revoked_at`
- `last_seen_at`
- `user_agent`
- `ip_address`

This is what lets the system:

- revoke sessions on logout and logout-all
- reject expired or revoked sessions even if a JWT exists
- rotate refresh secrets safely
- list active sessions for a user
- surface stable session metadata back to clients

Important current implementation note:

- the request-auth hot path now uses cache-backed user reads on both the access-cookie path and the refresh-fallback path
- those reads now go through an explicit cached profile path instead of reusing the full user record shape
- that change was made specifically to stop healthy authenticated requests from issuing `SELECT * FROM users WHERE id = ?` on every request
- request auth now checks account activity through a short-lived cached security-state read instead of hitting `users` on every request
- repo-managed user writes refresh that security-state cache immediately, so normal app-driven deactivation remains authoritative
- auth repos still expose `ByIDFresh(...)` for the paths that truly need credential, lockout, or other security-sensitive facts
- normal request auth should prefer the explicit cached profile read plus cached security-state read rather than a full cached user row
- direct out-of-band SQL updates to `users.active` can lag until the security cache entry expires
- if repeated `query users` events reappear in request inspects, inspect the auth request path before assuming the cache layer regressed globally

## Account And Policy Data Model

Current generated `users` shape includes:

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

Additional auth-owned tables:

- `auth_sessions`
- `auth_password_resets`
- `auth_email_verifications`
- `auth_login_attempts`

OAuth-owned tables when the `OAuth` component is enabled:

- `auth_identities`
- `auth_oauth_states`

`ByLogin` resolves by normalized username or email.

Current assumptions:

- username and email are the local identifiers
- password hash is only for local credential auth
- inactive users must not authenticate even with otherwise-valid credentials or sessions
- `users.id` remains the canonical account identity
- provider identities link into users explicitly instead of replacing the `users` table
- same-email provider sign-in does not auto-link to an existing user; linking must be explicit

## OAuth Component

The `OAuth` component adds provider sign-in and linking on top of baseline auth.

It currently includes:

- `auth_identities`
- `auth_oauth_states`
- provider registry and callback state handling
- built-in GitHub, Google, Microsoft, and Apple provider adapters
- OAuth start/callback/link routes
- OAuth cleanup of expired callback state

Generated `.env` behavior:

- OAuth env vars are included only when the `OAuth` component is enabled
- provider credentials are commented out by default
- uncomment and fill only the providers you actually configure

Live-provider setup still requires operator configuration for:

- client IDs and secrets
- allowed callback URLs
- provider-specific registration settings

## Provider-Ready Identity Model

`auth_identities` is the provider-linkage layer for future OAuth providers.

Current fields include:

- `user_id`
- `provider`
- `provider_subject`
- `provider_email`
- `provider_email_verified`
- `provider_username`
- `provider_display_name`
- `provider_avatar_url`
- `last_login_at`

Current service seams include:

- `FindOrCreateUserFromProviderIdentity(...)`
- `LinkIdentity(...)`
- `UnlinkIdentity(...)`
- `Identities(...)`
- `AuthenticateWithProvider(...)`

Current provider abstraction includes:

- `OAuthProvider`
- `OAuthCallback`
- `ProviderIdentity`

Current policy:

- exact provider subject match reuses the linked user
- same-email provider identities require explicit linking
- provider-created users get an unusable local password hash until a local password is explicitly set
- unlinking the last remaining auth method is rejected
- third-party provider round-trips are proven with stub/fake-provider tests in CI, not live provider calls

## Security Properties To Preserve

These are not optional implementation details.

### 1. Invalid credentials must not leak which part failed

Current behavior intentionally collapses:

- missing user
- wrong password
- inactive user

into `ErrInvalidCredentials`.

The implementation also spends bcrypt work on missing-user failures to reduce timing differences.

### 2. Sessions remain server-authoritative

Do not drift toward “JWT alone is enough”.

The current model checks the persisted session on both access-token use and refresh-token use.

### 3. Refresh rotation is mandatory

`RefreshSession` rotates:

- refresh secret hash
- refresh expiry
- session metadata

Do not allow reusable long-lived refresh values if avoidable.

### 4. Raw secrets are not persisted

Only hashes are stored for:

- refresh secrets
- password reset tokens
- email verification tokens

### 5. Cookies are `HttpOnly`

Current cookie behavior:

- `HttpOnly: true`
- `SameSite: Lax`
- `Secure` controlled by `AUTH_COOKIE_SECURE`

### 6. Cookie clearing is owned by logout/revoke

Current auth middleware and failed explicit refresh responses do not clear cookies when auth is invalid.

Only logout and current-session revoke clear auth cookies. This avoids arbitrary protected-request `401` responses overwriting newer valid browser cookies during refresh races.

### 7. Failed login pressure is stateful and bounded

Current policy adds:

- per-identifier + IP login attempt tracking
- temporary `429` rate limiting
- temporary user-level account lockout via `users.locked_until`

That policy should stay explicit and test-backed.

## Environment Contract

Current auth env keys:

- `API_JWT_SECRET_KEY`
- `AUTH_ACCESS_TOKEN_TTL`
- `AUTH_SESSION_IDLE_TTL`
- `AUTH_SESSION_TTL`
- `AUTH_REMEMBER_SESSION_TTL`
- `AUTH_REGISTER_REQUIRES_EMAIL_VERIFICATION`
- `AUTH_PASSWORD_MIN_LENGTH`
- `AUTH_PASSWORD_REQUIRE_UPPER`
- `AUTH_PASSWORD_REQUIRE_LOWER`
- `AUTH_PASSWORD_REQUIRE_NUMBER`
- `AUTH_PASSWORD_REQUIRE_SYMBOL`
- `AUTH_COOKIE_SECURE`
- `AUTH_BOOTSTRAP_USERNAME`
- `AUTH_BOOTSTRAP_EMAIL`
- `AUTH_BOOTSTRAP_PASSWORD`
- `AUTH_PASSWORD_RESET_TTL`
- `AUTH_PASSWORD_RESET_RETURN_TOKEN`
- `AUTH_EMAIL_VERIFICATION_TTL`
- `AUTH_EMAIL_VERIFICATION_RETURN_TOKEN`
- `AUTH_LOGIN_LOCKOUT_ATTEMPTS`
- `AUTH_LOGIN_LOCKOUT_DURATION`
- `AUTH_LOGIN_RATE_LIMIT_ATTEMPTS`
- `AUTH_LOGIN_RATE_LIMIT_DURATION`

Defaults:

- access token TTL: `15m`
- session idle TTL: `2h`
- session TTL: `24h`
- remembered session TTL: `720h` via fallback path (`30 * 24h`)
- registration requires email verification: `false`
- password reset TTL: `1h`
- email verification TTL: `24h`
- password min length: `8`
- password uppercase required: `true`
- password lowercase required: `false`
- password number required: `false`
- password symbol required: `true`
- login lockout attempts: `5`
- login lockout duration: `15m`
- login rate limit attempts: `10`
- login rate limit duration: `15m`
- cookie secure: `auto`

Notes:

- the short-lived access token is refreshed automatically while the session is still valid
- idle expiry is based on session inactivity using the session `last_seen_at` timestamp
- local auth exposes both `POST /api/v1/auth/login` and `POST /api/v1/auth/register`
- when `AUTH_REGISTER_REQUIRES_EMAIL_VERIFICATION=true`, registration creates the user, sends verification email, and does not issue a session until verification is completed
- local password flows enforce the configured policy during register, password reset, and password change
- generated apps do not emit password policy env vars into `.env` by default; the built-in defaults apply unless the app opts to override them
- the local bootstrap user remains a convenience path and does not enforce the configured password policy

Important token-exposure rule:

- reset and verification tokens are exposed in responses only in local app envs by default
- non-local envs must opt in explicitly with `AUTH_PASSWORD_RESET_RETURN_TOKEN=true` or `AUTH_EMAIL_VERIFICATION_RETURN_TOKEN=true`

`AUTH_COOKIE_SECURE=auto` means:

- secure on HTTPS
- not secure on plain HTTP

That is correct for local dev ergonomics.

## Bootstrap User Policy

`EnsureBootstrapUserFromEnv()` exists for:

- local development
- rendered/integration convenience flows

Current policy:

- username/password must be present
- bootstrap is local-only
- bootstrap is a no-op if the user already exists
- bootstrap also no-ops when auth tables do not exist yet

Do not broaden bootstrap behavior casually.

For explicit operator workflows, prefer CLI commands instead of startup mutation:

- `auth:create-user`
- `auth:set-password`
- `auth:bootstrap`

## Cleanup And Scheduling

Auth owns cleanup behavior through `auth.Service.Cleanup`.

Current cleanup removes stale rows from:

- `auth_sessions`
- `auth_password_resets`
- `auth_email_verifications`
- `auth_login_attempts`

The scheduler registry should call the auth-owned cleanup hook instead of reaching into auth persistence directly.

## What Exists In Tests

The generated auth integration coverage currently verifies:

- bootstrap user creation and no-op before migrations
- login by username and email
- invalid/inactive user rejection
- fail-closed JWT secret behavior
- login/me/logout flow
- cookie defaults
- revoked/tampered/expired session rejection
- refresh rotation and replay rejection
- session listing and per-session revoke
- logout-all
- password change
- password reset lifecycle
- email verification lifecycle
- login rate limiting and account lockout
- provider identity creation, explicit linking, unlink rules, and fake-provider auth flows
- cleanup of stale auth rows
- repeated access-expiry recovery on the same session
- concurrent protected-request recovery after refresh rotation
- protected-request recovery with a stale browser refresh cookie after one refresh has already rotated the session
- cache-backed user lookup on the access-cookie auth path
- cache-backed user lookup on the refresh-fallback auth path

Rendered generated-app integration currently verifies the end-to-end contract across SQLite, MySQL, and Postgres, including:

- login/logout/me/refresh
- sessions and revoke
- logout-all
- password change
- password reset
- email verification
- missing-user rate limiting
- real account lockout

Primary files:

- [service_integration_test.go.tmpl](/workspace/code/goforj/templates/internal/auth/service_integration_test.go.tmpl)
- [auth_integration_test.go](/workspace/code/goforj/internal/forj/auth_integration_test.go)

Recent rendered-app validation:

- the auth race/regression suite was rerun from fresh renders under `/tmp`, not the repo tree
- generated SQLite, MySQL, and Postgres auth integration tests currently cover:
  - expired access-cookie recovery
  - concurrent protected-request recovery after rotation
  - repeated access-expiry cycles
  - stale browser refresh-cookie replay
  - stale refresh recovery that must not overwrite a newer browser refresh cookie
- those fresh `/tmp` renders are the current confidence anchor for the refresh-race work

## Direction For Additional Providers

The current system should be treated as:

- a user identity and session core
- with one implemented credential type: local password login

Future providers should plug into that core, not replace it.

Provider support is now model-ready but not provider-complete.

What already exists:

- canonical `users` accounts
- explicit `auth_identities` provider linkage
- provider callback/service abstractions
- fake-provider integration coverage

What still needs to be added per real provider:

- provider-specific start/callback HTTP routes
- OAuth state/nonce handling
- provider package implementations for Google/GitHub/Facebook/etc.
- any provider-specific scopes, profile mapping, and token storage rules

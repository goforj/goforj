# Auth Improvement Task List

Goal: make generated auth clean, flexible, trustable, robust, and performant enough for broad production use.

## Current Status

- Core auth correctness is in a good place.
- Fresh rendered auth suites are green for SQLite, MySQL, and Postgres.
- The remaining work is mostly observability, contract clarity, and future-regression prevention.

## P0: Correctness And Security

- [x] Fix stale-refresh grace cookie behavior.
  - Middleware refresh may recover a request during the short same-session grace window.
  - That recovery must not write an `auth_refresh` cookie built from a stale refresh secret.
  - Add a cookie-jar test that proves stale responses cannot overwrite a newer valid refresh cookie.

- [x] Replace the unbounded per-session refresh lock map.
  - Current `map[string]*sync.Mutex` can grow forever from arbitrary session IDs.
  - Prefer bounded striped locks keyed by session ID hash, or add ref-counted cleanup.
  - Keep explicit `/auth/refresh` serialized per session.

- [x] Resolve the cookie-clearing contract.
  - Document one policy and make code/tests match it.
  - Recommended: logout/current-session revoke clears cookies; failed protected auth and failed refresh do not clear cookies.

- [x] Keep account deactivation authoritative through repo-managed writes.
  - Request auth now uses a short-lived cached security-state read instead of querying `users.active` on every request.
  - User repo writes refresh that security cache immediately, so normal app-driven activation/deactivation remains authoritative.
  - Direct out-of-band SQL updates are now bounded by the security cache TTL.

## P1: Resilience And Race Safety

- [x] Add browser cookie-jar race tests.
  - Expired access token.
  - Explicit refresh rotates the refresh secret.
  - Concurrent stale protected requests arrive with old refresh cookie.
  - Apply responses to a cookie jar in varying order.
  - Assert the next protected request still succeeds after access expiry.

- [x] Add a session rotation marker.
  - Recommended implementation order:
    1. add `refresh_rotated_at` on `auth_sessions`
    2. update it only on explicit refresh rotation
    3. surface it in inspect/debug reasoning and tests
  - `refresh_rotated_at` is now persisted, cached, logged for debug/inspect context, and covered by generated auth integration tests.

- [x] Run rendered auth tests across SQLite, MySQL, and Postgres before release.
  - SQLite, MySQL, and Postgres are currently green from fresh `/tmp` renders.
  - MySQL/Postgres caught timestamp precision, cached timestamp, and container readiness issues.

## P2: Performance And Persistence Hygiene

- [x] Replace full user/session saves on request auth with narrow updates.
  - Use targeted `UPDATE users SET last_seen_at=?, updated_at=? WHERE id=?`.
  - Use targeted `UPDATE auth_sessions SET last_seen_at=?, updated_at=? WHERE id=?`.
  - Avoid accidental overwrite of unrelated fields from cached structs.

- [x] Separate cached display payload from fresh security facts.
  - Cache user payload for response ergonomics.
  - Read security-sensitive fields fresh or from an explicitly short-lived authoritative cache.
  - Security-sensitive fields include `active`, `locked_until`, and credential-adjacent paths.
  - Request auth now uses explicit cached profile reads, while credential and lockout-sensitive flows use fresh user reads.

- [x] Review session cache invalidation coverage.
  - Revoke one session.
  - Revoke all sessions.
  - Password change.
  - Password reset confirm.
  - Cleanup of expired sessions.
  - Goal: make stale session state mechanically hard to reintroduce.
  - Generated integration coverage now primes cache before these flows and proves revoked or deleted session state is observed correctly afterward.

## P3: Simplicity And API Clarity

- [x] Rename internal flows to make behavior obvious.
  - Suggested names: `RotateRefreshSession`, `RecoverRequestSession`, `IssueAccessForSession`, `IssueTokenPairForSession`.
  - Make it hard to accidentally rotate refresh secrets from middleware recovery.
  - Internal refresh helpers now distinguish explicit rotation, token-pair reissue, access-only recovery, refresh-cookie authentication, and serialized refresh execution more clearly.

- [x] Keep OAuth strictly layered on Auth.
  - Plain Auth renders without OAuth routes, env, migrations, or provider files.
  - OAuth adapters plug into users/sessions/identities instead of replacing core auth.
  - Rendered integration now asserts auth-only renders stay free of OAuth routes, env stubs, migrations, provider files, and wire bindings.

- [x] Tighten auth debug logging rules.
  - Never log raw access tokens, refresh tokens, reset tokens, verification tokens, password hashes, or secrets.
  - Session ID, user ID, auth outcome, and auth reason are acceptable for debug/inspect timelines.
  - The auth debug helper now documents an explicit allowlist and a "never log" set so future inspect/debug additions stay bounded.

## Validation Checklist

- [x] `go test ./internal/forj -run 'Auth|GeneratedAuth|Render' -count=1`
- [x] Render fresh project under `/tmp`.
- [x] `go test ./internal/auth -tags=integration,sqlite -count=1`
- [x] Run rendered MySQL auth tests.
- [x] Run rendered Postgres auth tests.
- [x] Run focused stale-refresh/cookie-jar tests repeatedly.

## Next Step

- Decide whether any additional public repo/API cleanup is still worth the churn.
- Otherwise this auth pass is functionally complete.

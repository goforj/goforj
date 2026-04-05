# Web Middleware Port Checklist

This checklist tracks Echo middleware features that GoForj should evaluate and, where appropriate, port into `github.com/goforj/web/middleware`.

Goals:

- make common middleware first-class in the standalone `web` library
- use Echo as the reference point for behavior and ergonomics
- keep tests in the `web` repo as the source of truth
- avoid hiding engine-specific requirements when a middleware needs deeper adapter hooks

Current stance:

- `web/middleware` should own the portable/common recipes
- adapter-specific compatibility can still exist for deeply transport-shaped behavior
- this checklist is about capability coverage, not blindly copying files one-for-one

## Legend

- `[x]` implemented in `web/middleware`
- `[ ]` not implemented yet
- `[~]` partial / needs refinement

## Core Middleware

- `[x]` `RequestID`
- `[x]` `Recover`
- `[x]` `RequestLogger`
- `[x]` `CORS`
- `[x]` `BasicAuth`
- `[x]` `KeyAuth`
- `[x]` `RateLimiter`
- `[x]` `BodyLimit`
- `[x]` `Timeout`
- `[x]` `ContextTimeout`

## Request/Response Mutation

- `[x]` `Compress` / `Gzip`
- `[x]` `Decompress`
- `[x]` `BodyDump`
- `[ ]` non-200 response body capture recipe
- `[x]` `MethodOverride`
- `[x]` `Rewrite`
- `[x]` redirect recipes
  - `HTTPSRedirect`
  - `HTTPSWWWRedirect`
  - `HTTPSNonWWWRedirect`
  - `WWWRedirect`
  - `NonWWWRedirect`
- `[x]` slash recipes
  - `AddTrailingSlash`
  - `RemoveTrailingSlash`

## Security Middleware

- `[ ]` `CSRF`
- `[x]` `Secure`

## Static / Content Serving

- `[ ]` `Static`
- `[ ]` `StaticWithConfig`

## Advanced / Infrastructure-Shaped

- `[ ]` `Proxy`

These are still worth tracking, but they may need more adapter/runtime-specific hooks than the other middleware.

## Utilities / Supporting APIs

These are not always middleware themselves, but they may be needed to support middleware parity.

- `[ ]` extractor helpers for:
  - header
  - query
  - param
  - cookie
  - form
- `[ ]` response/body observation hooks needed by:
  - compression
  - body dump
  - non-200 logging
- `[~]` richer request mutation hooks needed by:
  - rewrite
  - redirect
  - method override
- `[ ]` static file serving helpers needed by:
  - `Static`
  - SPA/static fallback patterns

## Suggested Order

Recommended next slices:

1. `BasicAuth`
2. `KeyAuth`
3. `BodyLimit`
4. `RateLimiter`
5. `Timeout`
6. `Compress`
7. `Rewrite`
8. `Redirect`
9. `Static`
10. `Secure`
11. `CSRF`
12. `Proxy`

## Notes

- `RequestLogger`, `Recover`, `RequestID`, and `CORS` are already first-class and should be refined in the `web` repo rather than reimplemented in GoForj.
- `Static`, `Compress`, and `Proxy` are common and desirable, but they will likely require more adapter hooks than the first wave.
- Echo remains the behavior reference for these recipes even as `web` grows its own first-class surface.

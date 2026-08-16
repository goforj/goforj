# Environment Contract Design

Status: implemented

## Purpose

GoForj projects need three things from environment configuration:

- a private place for local values and secrets
- a safe inventory teammates can clone
- a runnable, deterministic test profile for local and CI tests

The framework owns the mechanical synchronization between those surfaces so a developer does not have to reconstruct complete dotenv files by hand.

## File Contract

| File | Git policy | Purpose |
| --- | --- | --- |
| `.env` | ignored | Private local values and generated local secrets |
| `.env.example` | committed | Safe inventory and portable defaults |
| `.env.testing` | committed | Safe, deterministic values selected by `goforj/env` when tests load the environment |

GoForj uses `.env.testing`, not `.env.test`. Calls to `goforj/env` select it when `APP_ENV=testing` or when the process is running as a Go test. Generated application construction loads the environment; an arbitrary package test that never loads `goforj/env` does not read dotenv files merely because the test process exists.

Process environment variables retain precedence over dotenv files. CI can therefore use the committed `.env.testing` for the complete ordinary test contract and inject only credentials needed by explicit live-service tests.

## Developer Workflow

The natural project entry points `forj build`, `forj generate`, `forj dev`, and `forj run` automatically create a missing `.env` from `.env.example` after command validation. Read-only help, version, environment checks, invalid commands, and unrelated tooling do not create it. This makes a clone runnable without requiring knowledge of a separate setup command. Initialization:

- never replaces an existing `.env`
- creates the file with private permissions where supported
- generates fresh values for the framework-owned signing and diagnostic keys present in the Project contract
- leaves application-specific credentials blank
- never prints generated values

The lifecycle can also be invoked explicitly:

```bash
forj env:init
```

Use hidden terminal input for a local value so it does not enter shell history or the process argument list:

```bash
forj env:set DISCORD_TOKEN
```

The command updates `.env` and reports only the key name. The next generation lifecycle refreshes the safe contracts.

## Synchronization

Environment synchronization has one lifecycle owner: generation. `forj generate` refreshes the contracts directly, while `forj build`, `forj dev`, and project rendering receive the same behavior through their generation phase. Those callers do not independently synchronize the contracts.

Synchronization is stable and writes only when output changed:

1. Merge exact framework-owned keys from `.env` into `.env.example`.
2. Blank secret-looking values and all newly discovered application values before crossing the committed boundary.
3. Preserve existing committed comments, ordering, and project-owned values conservatively.
4. Derive `.env.testing` from the safe example.
5. Serialize same-project updates, then replace changed files through same-directory temporary files and roll back the example if test-profile publication fails.

The implementation does not prune unknown keys automatically. Removing project-owned configuration is a developer decision; the framework synchronizes framework-owned keys and adds missing contract entries.

## Secret Boundary

Secret-like names such as tokens, passwords, private keys, credentials, DSNs, API keys, personal access tokens, and webhooks are blank in `.env.example`. Redaction covers both `KEY=value` and dotenv-supported `KEY: value` assignments, decoded multiline credential URLs, and inline comments attached to redacted assignments. Active keys outside GoForj's portable letters/digits/underscores grammar stop synchronization instead of bypassing the transform.

Name classification is defense in depth rather than the only publication boundary. A key that appears in `.env` but not `.env.example` is published with a blank value regardless of its name. To share a non-secret application default, add that value explicitly to the reviewed `.env.example`; later synchronization preserves it instead of replacing it from private local state.

An existing `.env.testing` without GoForj's managed marker may contain historically private test credentials. Synchronization refuses to rewrite or expose that file. Move values that must remain private into `.env`, remove the legacy `.env.testing`, and run `forj generate` to create the committed deterministic profile.

`.env.testing` uses conspicuously public deterministic values for framework credentials. Examples include `DB_PASSWORD=test` and named `goforj-public-testing-*` signing values. Unknown application secrets remain blank. These values are test fixtures, not deployment credentials.

Contract files remain reviewable source files, and a repository secret scanner should still run in CI. Real deployment credentials must be supplied through the deployment environment or a secret provider.

## Test Defaults

The generated testing profile favors isolation and local execution:

- `APP_ENV=testing`
- `DB_HOST`, `REDIS_HOST`, and `MAIL_SMTP_HOST` use `127.0.0.1`
- NATS and RabbitMQ URLs use the generated local container service names
- database usernames and passwords use `test`
- database names gain a `_testing` suffix
- SQLite files gain `_testing` before `.db`, `.sqlite`, or `.sqlite3`
- framework signing values are deterministic and explicitly public
- bootstrap credentials and unknown secret-like values remain blank
- explicitly committed non-sensitive project values are copied from `.env.example`

Project-owned safe values already present in `.env.testing` are preserved, including names that happen to begin with common framework words. Only the exact rendered framework inventory and its named-App overlays are refreshed or pruned, so component and renderer changes cannot leave framework test settings stale.

## CI Workflow

CI should verify the committed contract without creating a local file:

```bash
forj env:check
go test ./...
```

`forj env:check` derives expected output from `.env` when it exists and otherwise from committed `.env.example`. It fails with the stale filenames and directs the developer to run `forj generate`; it never rewrites files. On a clean checkout it verifies consistency between the two committed contracts, but it cannot infer a required key omitted from both files without a local generation source.

Ordinary tests should require no live third-party secrets. Tests that intentionally contact an external service should receive only their required secrets through process environment variables and fail clearly when those values are absent.

## Safety Rules

- Never commit `.env` or real local credentials.
- Never accept secret values as command-line arguments.
- Never print generated or entered secret values.
- Redact before publishing either committed contract.
- Keep generated test credentials deterministic, public, and unmistakably non-production.
- Preserve process-environment precedence for CI and deployment overrides.
- Avoid rewriting synchronized files so generation does not create timestamp or Git churn.

## Future Work

External secret managers may later populate local or CI process environments, but they should layer on this contract instead of replacing the simple local workflow. Structured component metadata could also make required keys and test defaults more expressive than name-based classification. Neither is required for the safe default path implemented here.

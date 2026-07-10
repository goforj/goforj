# Env Contract Generation Design

Status:

- proposed
- intended for `forj generate`, `forj build`, and `forj dev`
- focused first on `.env.example` generation from local `.env`

## Purpose

This document defines a framework-level model for keeping local `.env` files
useful without asking developers to commit secrets or manually maintain a
separate environment contract.

The goal is simple:

```text
Developers edit .env.
GoForj generates the shareable safe shape.
GoForj refuses to leak obvious secrets.
```

## Problem

Generated GoForj apps commonly use `.env` as the local source of truth.

That is familiar and practical. Developers already understand:

```bash
cp .env.example .env
vim .env
forj dev
```

The tension is that `.env` often contains sensitive values:

```env
DISCORD_TOKEN=real-token
STRIPE_WEBHOOK_SECRET=real-secret
DB_PASSWORD=real-password
```

Committing those values is not acceptable, but asking developers to manually keep
`.env.example` updated is also not good enough. Humans forget. Frameworks should
remove that memory burden where the intended behavior is mechanical.

The same problem appears in integration tests. App functionality can depend on
env keys that are present in a developer's local `.env`, while the committed test
environment may be incomplete or stale.

## Goals

- Keep `.env` as the normal local developer source of truth.
- Generate `.env.example` from `.env`, selected components, and framework env
  metadata.
- Make `.env.example` safe to commit by redacting secrets by default.
- Include env contract refresh in the normal generation pipeline.
- Preserve stable sections and useful comments where practical.
- Keep generated changes reviewable and deterministic.
- Make secret classification visible enough that developers trust it.
- Support explicit overrides for ambiguous keys.
- Establish a path for test env contract generation without requiring live
  service secrets for ordinary tests.

## Non-Goals

- Do not encourage committing real `.env` files.
- Do not make developers manually maintain `.env.example` as the source of truth.
- Do not require Ship, 1Password, Doppler, or any external secret manager for the
  basic local workflow.
- Do not solve production secret delivery in the first slice.
- Do not overwrite curated `.env.test` files without an explicit command or
  policy.
- Do not perform deep static analysis as the first implementation.

## Core Model

Generated apps should treat these files differently:

```text
.env             local source of truth, ignored
.env.example     generated safe contract, committed
.env.test        committed only when values are safe and deterministic
.env.test.example generated safe test contract, optional
```

The default local workflow remains familiar:

```bash
forj dev
```

During the build watcher, GoForj refreshes generated app surfaces. Environment
contract generation becomes one of those surfaces.

## Command Shape

Environment contract generation should be part of normal generation:

```bash
forj generate
forj build
forj dev
```

Focused command:

```bash
forj generate --env
```

Optional explicit app command alias in rendered apps:

```bash
forj env:sync
```

Diagnostic command:

```bash
forj env:doctor
```

`forj env:doctor` should report stale contracts, missing required keys, and
secret classification without rewriting files unless a `--fix` flag is added
later.

## Generation Inputs

The generator should read:

- `.goforj.yml`
- local `.env` when it exists
- existing `.env.example` for preserved comments and section ordering
- generated component env declarations
- known framework env keys
- optional app env metadata in `.goforj.yml`

Later versions can add:

- package-level env declarations
- static scans for known env helper calls
- Ship-managed environment metadata
- external secret-manager metadata

The first implementation should not depend on static scanning. Component
metadata plus `.env` covers the most common and most intuitive workflow.

## Generated Output

Given local `.env`:

```env
APP_ENV=local
API_HTTP_PORT=3000

DISCORD_TOKEN=real-token
DISCORD_PUBLIC_KEY=public-key
DISCORD_APPLICATION_ID=123456

DB_DRIVER=sqlite
DB_DATABASE=./storage/app.db
```

Generated `.env.example` should be safe:

```env
APP_ENV=local
API_HTTP_PORT=3000

DISCORD_TOKEN=
DISCORD_PUBLIC_KEY=public-key
DISCORD_APPLICATION_ID=123456

DB_DRIVER=sqlite
DB_DATABASE=./storage/app.db
```

This keeps useful defaults while removing sensitive material.

## Secret Classification

Classify a key as secret when its name contains one of these tokens:

```text
SECRET
TOKEN
PASSWORD
PASS
PRIVATE_KEY
CREDENTIAL
CREDENTIALS
API_KEY
AUTH_KEY
WEBHOOK_SECRET
DSN
DATABASE_URL
```

Framework metadata should be able to mark a key as secret even when the heuristic
does not catch it.

Some keys look sensitive but are public by convention. Allow explicit overrides:

```yaml
env:
  keys:
    DISCORD_PUBLIC_KEY:
      secret: false
    SENTRY_DSN:
      secret: true
    DISCORD_TOKEN:
      secret: true
```

Default behavior should be conservative. If GoForj cannot confidently classify a
key, it can either keep the value only when it looks obviously local and harmless,
or blank it with a warning in verbose output.

## Value Policy

Use these rules when writing `.env.example`:

| Value type | Output |
| --- | --- |
| Known secret | blank value |
| Known public value | copied value |
| Component default | generated default |
| Required unknown secret | blank value plus optional comment |
| Unknown local-looking value | copied when safe |
| Unknown suspicious value | blank value |

Examples of local-looking safe values:

```text
local
test
true
false
3000
127.0.0.1
localhost
sqlite
workerpool
./storage/app.db
```

Examples of suspicious values:

```text
long random strings
base64-looking blobs
URLs with embedded credentials
JWT-looking values
PEM blocks
```

The first implementation can rely mostly on key classification and simple value
shape checks. It should not pretend to be a complete secret scanner.

## Sections And Comments

The generator should keep `.env.example` readable.

Preferred section sources:

1. existing `.env.example` section headers
2. component env metadata sections
3. framework default ordering
4. unknown local keys in an `# App` or `# Local app` section

When updating an existing `.env.example`:

- preserve known comments where practical
- update generated values in place
- add new keys near related keys
- remove keys only when an explicit pruning policy is enabled
- preserve newline-at-EOF behavior

The first version can be conservative and append unknown keys instead of
attempting complex comment surgery.

## Build And Dev Output

Normal output should be quiet.

When `.env.example` changes:

```text
env        updated .env.example
```

Verbose output can include classification:

```text
env        DISCORD_TOKEN redacted
env        DISCORD_PUBLIC_KEY public
env        STRIPE_WEBHOOK_SECRET redacted
```

If a likely secret would be copied, fail closed or warn loudly:

```text
env        refusing to copy suspicious value for CUSTOM_SESSION_KEY
```

The generator should never print raw secret values.

## Drift Behavior

`forj generate --env` should update files.

`forj env:doctor` should report drift:

```text
.env.example is stale

Added:
  DISCORD_TOKEN          secret
  DISCORD_APPLICATION_ID public

Changed:
  API_HTTP_PORT          public default changed from 3000 to 3001

Run:
  forj generate --env
```

CI can use a check mode later:

```bash
forj generate --env --check
```

That command should fail when generated output would change.

## Integration Test Env

The first-class test rule should be:

```text
ordinary tests should not require live third-party secrets
```

Generated apps can use committed safe test defaults:

```env
APP_ENV=test
APP_DEBUG=false
DB_DRIVER=sqlite
QUEUE_DRIVER=sync
EVENTS_DRIVER=inproc
MAIL_DRIVER=log
DISCORD_ENABLED=false
DISCORD_TOKEN=
```

Live third-party tests should be opt-in through explicit profiles:

```bash
forj test:integration --profile live-discord
```

If a profile needs secrets, it should fail early with a clear message:

```text
live-discord requires DISCORD_TOKEN
set it in .env, CI secrets, or a supported secret provider
```

Potential generated files:

```text
.env.test.example
```

Use `.env.test.example` as the generated safe contract for tests. Treat
`.env.test` as curated unless the project explicitly opts into generated test
env writes.

## Metadata Model

Component env declarations should eventually be structured, not hand-coded string
blocks.

Suggested shape:

```go
type EnvKey struct {
	Name        string
	Section     string
	Description string
	Default     string
	Required    bool
	Secret      bool
	TestDefault string
}
```

Generators for auth, database, cache, storage, queue, events, mail, metrics,
observability, and HTTP can all contribute declarations.

For app-specific keys, `.goforj.yml` can provide metadata:

```yaml
env:
  keys:
    DISCORD_TOKEN:
      section: Discord
      secret: true
      required: true
    DISCORD_PUBLIC_KEY:
      section: Discord
      secret: false
      required: true
    DISCORD_ENABLED:
      section: Discord
      default: "true"
      test_default: "false"
```

This keeps the common path automatic while giving teams a way to correct the
generator when heuristics are not enough.

## Relationship To Secret Stores

This design does not require a secret store.

Future integrations can provide values for local and CI workflows:

```bash
forj env pull --from ship --env development
forj secret set DISCORD_TOKEN
forj secret sync --to ship --env production
```

Those features should layer on top of the same env metadata and contract model.
They should not replace `.env` as the simple local entry point.

## Safety Rules

- Never commit generated files containing secret values.
- Never print raw secret values during generation.
- Treat secret redaction as fail-closed.
- Make secret classification inspectable.
- Keep `.env` ignored by default.
- Prefer blank secret values in `.env.example`.
- Require explicit opt-in for live integration test profiles.

## Implementation Phases

### Phase 1: `.env.example` generator

- Add `forj generate --env`.
- Read `.env`, `.goforj.yml`, and component metadata.
- Redact obvious secrets.
- Write deterministic `.env.example`.
- Include the env step in `forj generate`, `forj build`, and `forj dev`.
- Add focused tests for classification, redaction, and stable output.

### Phase 2: Metadata-backed components

- Move framework env keys into structured component declarations.
- Preserve section ordering and comments better.
- Add `.goforj.yml` env key overrides.
- Add `forj env:doctor`.

### Phase 3: Test env contracts

- Generate `.env.test.example`.
- Add test profile metadata.
- Teach `test:integration` to report missing profile secrets early.
- Keep `.env.test` curated unless the project opts into generated writes.

### Phase 4: Secret provider integration

- Add optional local encrypted secret storage or provider adapters.
- Add Ship-backed development and production secret sync.
- Add CI-friendly export/check flows.

## Open Questions

1. Should `.env.example` pruning ever remove keys that no longer appear in
   `.env` or metadata?
2. Should `forj dev` auto-write `.env.example` by default, or only report drift
   unless `FORJ_ENV_SYNC=auto` is enabled?
3. How should multi-app projects group app-prefixed env keys in the generated
   example?
4. Should local `.env.local` be read as an additional input, or should `.env`
   remain the only local source of truth for generation?
5. Should app-specific env metadata live in `.goforj.yml`, a dedicated
   `.goforj/env.yml`, or generated package declarations?

## Recommendation

Make `.env.example` generation a normal part of the GoForj generation pipeline.

Developers should be able to edit `.env` naturally and trust GoForj to maintain
the safe committed shape. The framework should carry the mechanical burden:
classify, redact, preserve, and report.

That is the GoForj-shaped solution: keep the familiar local workflow, then make
the safe path automatic.

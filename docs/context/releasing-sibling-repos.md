# Releasing Sibling Repos

This document explains the practical workflow for repos like `console`, `web`, and `queue` when GoForj depends on them.

## Common Development Mode

While a sibling repo is not yet released, use:

- `render.module_replaces`

to point the rendered app at the local repo checkout.

Example:

```yaml
render:
  module_replaces:
    github.com/goforj/web: ../web
```

## Standard Release Flow

1. make and verify changes in the sibling repo
2. tag/publish the sibling repo
3. bump GoForj to the new version
4. rerender and smoke test the app
5. remove local replaces if they are no longer needed

If the sibling repo is multi-module, "tag/publish the sibling repo" means all affected module tags, not just the root tag.

## For `console`

The console README and API index are generated from source-comment examples, so
package validation includes documentation regeneration:

```bash
cd ../console
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go generate .
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go vet ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go -C docs vet ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go -C examples vet ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test -race ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go -C docs test -race ./...
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go -C examples test ./...
git diff --exit-code -- README.md
```

Before bumping GoForj, verify that common operations still have both package and
instance forms, README examples show their output, and loader/progress behavior
remains useful for both terminals and redirected CI logs. Then tag and push the
console release before updating `goforj/go.mod`.

`render.module_replaces` does not replace the dependency used by the host
`forj` CLI. It only affects rendered applications, which currently keep their
generated `internal/console` package. Host integration therefore needs a tagged
console version, a GoForj module bump, and focused GoForj tests.

## For `web`

Typical local validation:

```bash
cd ../web
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

Then point the rendered app at local `web` until release is ready.

## For `queue`

Typical local validation:

```bash
cd ../queue
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./driver/redisqueue -count=1
```

Then tag/push modules before relying on those versions in GoForj.

## For `storage`

Typical validation now includes both unit/contract coverage and real integration coverage:

```bash
cd ../filesystem
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
cd integration
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test -tags=integration ./all -count=1
```

If storage changes affect docs/examples, also regenerate them before release:

```bash
cd ..
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go run ./docs/examplegen/main.go
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go run ./docs/readme/main.go
```

## Important Path Rules

For local replaces:

- use paths relative to the rendered project's `go.mod`
- do not use `~`
- preserve the same sibling layout when moving the project between environments

## Temporary State Is Fine

It is acceptable to keep a sibling repo in local-replace mode while the code is stabilizing.

Do not force release work prematurely if the code is still in active architecture churn.

## Multi-Module Release Cautions

Recent `storage` lessons:

- a local tag is not a release; verify the remote can actually resolve it
- it is easy to ship an inconsistent version if internal `go.mod` references were not rewritten before tagging
- if a bad intermediate version exists, do not try to mentally "fix" it; cut the next clean patch and move on
- after release, verify the rendered app is pulling the new submodule versions you expect

In practice:

- use the repo release script that rewrites internal module references
- then push `main`
- then push all module tags
- then confirm with `git ls-remote --tags origin` or `go list -m` against the released versions

## For `cache`

Typical validation now includes both unit coverage and real integration coverage:

```bash
cd ../cache
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
cd integration
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test -tags=integration ./all -count=1
```

If new cache APIs were added, also regenerate docs/examples before release:

```bash
cd ..
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go run ./docs/examplegen/main.go
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go run ./docs/readme/main.go
```

Recent cache-specific lesson:

- if a new API exists, examples and generated docs are part of `done`, not optional follow-up
- Lighthouse explorer work should not ship on top of local `replace` paths once the sibling repo has a proper release

## For `web`

Recent practical lesson:

- pseudo-versions and local replaces are fine while work is stabilizing
- but once `web` becomes a real dependency boundary, cut a real tag and move GoForj onto it
- this avoids CI failures from repo-local `replace ../web` assumptions

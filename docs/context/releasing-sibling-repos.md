# Releasing Sibling Repos

This document explains the practical workflow for repos like `web` and `queue` when GoForj depends on them.

## Common Development Mode

While a sibling repo is not yet released, use:

- `render.module_replaces`

to point the rendered app at the local repo checkout.

Example:

```yaml
render:
  module_replaces:
    github.com/goforj/web: /Users/cmiles/code/web
```

## Standard Release Flow

1. make and verify changes in the sibling repo
2. tag/publish the sibling repo
3. bump GoForj to the new version
4. rerender and smoke test the app
5. remove local replaces if they are no longer needed

If the sibling repo is multi-module, "tag/publish the sibling repo" means all affected module tags, not just the root tag.

## For `web`

Typical local validation:

```bash
cd /workspace/code/web
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
```

Then point the rendered app at local `web` until release is ready.

## For `queue`

Typical local validation:

```bash
cd /workspace/code/queue
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./driver/redisqueue -count=1
```

Then tag/push modules before relying on those versions in GoForj.

## For `storage`

Typical validation now includes both unit/contract coverage and real integration coverage:

```bash
cd /Users/cmiles/code/filesystem
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test ./...
cd /Users/cmiles/code/filesystem/integration
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go test -tags=integration ./all -count=1
```

If storage changes affect docs/examples, also regenerate them before release:

```bash
cd /Users/cmiles/code/filesystem
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go run ./docs/examplegen/main.go
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go run ./docs/readme/main.go
```

## Important Path Rules

For local replaces:

- use absolute paths
- do not use `~`
- do not assume relative paths work from every rendered app location

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

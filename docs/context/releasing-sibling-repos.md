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

## Important Path Rules

For local replaces:

- use absolute paths
- do not use `~`
- do not assume relative paths work from every rendered app location

## Temporary State Is Fine

It is acceptable to keep a sibling repo in local-replace mode while the code is stabilizing.

Do not force release work prematurely if the code is still in active architecture churn.

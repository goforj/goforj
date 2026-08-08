# Working on a starter kit

Templates are compiled into the binary. `templates/embed.go` uses
`//go:embed all:*`, and `internal/forj/project_renderer.go` renders from that
embedded copy with no filesystem fallback.

That means **editing anything under `templates/` has no effect until you rebuild**:

```bash
go install ./cmd/forj
```

A dev server watching a rendered project will faithfully re-render, but from the
snapshot the binary was built with. Stale output looks exactly like a broken
watcher, which is a slow thing to diagnose.

## Rendering from disk instead

Point `GOFORJ_TEMPLATES_DIR` at the `templates` directory and the renderer reads
it live:

```bash
export GOFORJ_TEMPLATES_DIR=$(git rev-parse --show-toplevel)/templates
forj new my-app
```

Paths inside the renderer are relative to `templates/`, so the variable must
point at that directory rather than the repository root.

If it is set to something that is not a directory, the renderer says so on
stderr and falls back to the embedded copy rather than failing silently.

Leave it unset for released binaries — the embedded copy is what makes `forj` a
single self-contained executable.

## Checking a frontend starter kit

The Vue kit's reference pages import each example twice: once as a component to
render, once with Vite's `?raw` suffix to display in the source panel. Adding an
example means adding both imports; the page will compile with only the first and
silently show no code.

Two failure modes the build will not catch:

- A vendored `ui/` module importing a package absent from `package.json`. If
  nothing imports the module it never enters the production graph, so
  `vue_starter_build.test.mjs` cannot see it either. Both `ui/chart` and
  `ui/table/utils.ts` shipped broken this way.
- An import removed from a page while the template still uses it. `vue-tsc`
  catches this; a plain `vite build` of an unreferenced path may not.

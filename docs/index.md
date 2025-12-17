---
layout: home

hero:
  name: "GoForj"
  text: "High-trust tooling forged for productivity and performance."
  tagline: "A Laravel-like developer experience, done the Go way: explicit, auditable, and production-first."
  image:
    src: ./assets/goforj-full.png
    alt: GoForj
  actions:
    - theme: brand
      text: Getting Started
      link: /getting-started
    - theme: alt
      text: What is GoForj?
      link: /about

features:
  - title: Opinionated, never opaque
    icon: 💠
    details: "Explicit configuration, no hidden globals, and clear escape hatches. You always know what is happening."

  - title: Scaffolds you can trust
    icon: 🧭
    details: "Core, Docker, API, UI, DB, scheduler, and jobs-generated as idiomatic Go you’d maintain by hand."

  - title: Production-first dev loop
    icon: 🔁
    details: "Wire install/generate, wgo, Docker up/down, DB wait, and aligned watch output-ready for real services."

  - title: Fast rerenders, safe skips
    icon: ⚡️
    details: "`forj render` is idempotent: created vs skipped counts, tidy module totals, and Wire ready-so `go run main.go` just works."

  - title: Tested, transparent, and fast
    icon: 🔍
    details: "Designed for high coverage, CI enforcement, and explicit behavior. Performance is a feature, not a footnote."

  - title: Batteries included, Go stays Go
    icon: 🛠️
    details: "Dependency injection via Wire, sensible env handling, Docker helpers, and watchers-without framework lock-in."

---

## Why developers pick GoForj first

- **From wizard to runnable**: `forj new` → `forj render` → `go run main.go` works on the first try. Wire install, generate, and tidy are baked in.
- **Production defaults, no guessing**: Docker up/down hooks, DB wait, and watches live explicitly in `.goforj.yml`-ready for teams to review.
- **Safe reruns by design**: Idempotent scaffolds with created/skipped reporting. Rerender after changing components without clobbering your work.
- **Performance-conscious**: Go-first, DI via Wire, no hidden runtime magic. You see every command we run.
- **Composable toolkit**: Use the pieces you need-CLI, API, UI, jobs, scheduler, Docker-without framework lock-in.

## Who it’s for

- Go engineers shipping APIs, CLIs, workers, schedulers, and internal tools.
- Teams that want auditable scaffolds, predictable hooks, and a consistent DX across services.
- Developers who value explicit code, reliable dev loops, and production-ready defaults.

## Start in minutes

```bash
go install github.com/goforj/goforj/cmd/forj@latest
forj new          # guided wizard (name, module, components, path)
forj render       # scaffolds, tidy modules, wire install+generate
forj dev          # pre-dev hooks + watches; forj down to tear down Docker
````

## Experience snapshot

* **Guided, aligned output**: Wizard panels, live path validation, and render logs with clear counts and module totals.
* **Instantly runnable**: Wire install+generate baked into render-no “run dev first” surprises.
* **Production-aware hooks**: Docker up/down, DB wait, and wgo are explicit in `.goforj.yml` for teammates to audit.
* **Idempotent by design**: Rerender without fear-created vs skipped is always reported.
* **Respect for Go**: No hidden globals or runtime magic. Everything is plain Go and shell you can read.



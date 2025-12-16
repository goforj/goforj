# What is GoForj?

GoForj is a Go ecosystem of high-trust tools forged for developer productivity and performance. It is not a monolithic framework. Each piece is independently useful, but together they deliver a Laravel-like developer experience while staying idiomatic, explicit, and performant Go.

## Why GoForj Exists

Go is great for production services, but teams still re-build the same foundations: structure, wiring, environments, jobs, schedulers, and scaffolds. GoForj provides these as a cohesive toolkit:

- Opinionated defaults with clear escape hatches
- Predictable project structure and scaffolds you can trust
- Dependency injection with Wire, generated as plain Go
- First-class support for HTTP, jobs, schedulers, and Docker-based dev setups
- Strong testing expectations and performance awareness

## Design Principles (always visible in the docs)

- High trust: explicit APIs, test coverage targets, CI + formatting baked in.
- Explicit over implicit: no hidden globals or runtime magic; config is visible.
- Production first: examples mirror real services, not toy snippets.
- Performance is a feature: avoid unnecessary allocations; note trade-offs.
- Developer experience matters: copy-paste-ready examples and clear guidance.

## What GoForj Gives You

- **Guided project creation**: `forj new` with a modern wizard (name, module, components, path, confirm) and validation on paths/components.
- **Scaffolds as product**: HTTP handlers, jobs, schedulers, Docker, env files, and Wire graphs generated as readable Go you would maintain by hand.
- **Dependency injection without magic**: Wire install + generate run automatically after render so `go run main.go` works immediately.
- **Dev loop you can trust**: `forj dev` runs pre-dev tasks (wire, wgo, docker up, db wait), attaches watches (App, Wire, Scheduler, Jobs, NPM), and `forj down` tears down Docker when configured.
- **Configuration you can audit**: `.goforj.yml` records project name, module, components, hooks, and watches; written to the chosen project path.
- **Explicit outputs**: Rendering shows created vs skipped counts, tidy module counts, and clear next steps.

## Who GoForj Is For

- Go developers building APIs, CLIs, background workers, schedulers, and internal tools.
- Teams that want consistent, auditable scaffolds and hooks across services.
- Engineers who value explicit code, predictable behavior, and long-term maintainability.

## Start Here (happy path)

```bash
go install github.com/goforj/goforj/cmd/forj@latest
forj new               # guided wizard with validation
forj render            # generate scaffolds and run wire/tidy
forj dev               # start dev loop with watches and pre-dev hooks
```

You get an immediately runnable project, with Docker helpers when selected, and a `.goforj.yml` that records how to run dev and down hooks.

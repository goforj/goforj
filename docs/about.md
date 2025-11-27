# What is GoForj?

*A developer’s framework for building full-stack Go applications - fast, clean, and scalable.*

GoForj is a **modern application framework for Go** that helps you build complete, production-ready services without the usual boilerplate. It gives you a clean project structure, code generation tools, dependency injection, migrations, HTTP routing, background jobs, and a powerful CLI - all wired together using Go best practices.

If you’ve ever wished Go had a framework with the ergonomics of **Rails**, the productivity of **Laravel**, and the clarity of **idiomatic Go**, GoForj is exactly that: **a forge for building real applications.**

# Why GoForj Exists

Go is fast, simple, and reliable - but building full applications from scratch often means re-inventing the same things:

* Directory structure
* Dependency injection
* HTTP controllers
* Migrations
* Jobs and scheduling
* App configuration
* Project scaffolding
* Local dev tools
* Docker environments

GoForj provides all of this out of the box in a way that feels **natural to Go**, not layered on top of it.

You keep Go’s simplicity. GoForj adds everything you’re missing.

# What GoForj Gives You

## A Complete Project Template

Start a new production-ready Go service in seconds:

```bash
forj new myapp
cd myapp
forj dev
```

You instantly get:

* a clean app structure
* preconfigured wiring
* HTTP server + routing
* database layer
* migrations
* job runner
* environment files
* Docker Compose setup
* auto-generated dependency injection via Wire

No setup. No plumbing. Just build.


## Code Generation That Works With You

GoForj ships with a CLI inspired by the best frameworks:

```bash
forj make:controller users
forj make:migration add_users_table
forj make:service billing
forj make:job nightly_cleanup
```

Each command generates real, idiomatic Go files - controllers, services, migrations, jobs, DI graphs - placed exactly where they belong.

You focus on logic.
GoForj handles the structure.

## Dependency Injection Without the Pain

GoForj uses **Google Wire** behind the scenes, generating your entire application graph automatically.

No manual wiring.
No guessing what depends on what.
No runtime magic - it’s all plain Go code.

Your app stays modular, testable, and explicit.

## Built-in HTTP Layer

GoForj includes:

* routing
* middleware
* request/response helpers
* controller scaffolding
* dependency-injected handlers

Launching APIs becomes *effortless*.

## First-Class Database Migrations

Just run:

```
forj migrate
```

Or generate new migrations:

```
forj make:migration create_orders
```

GoForj handles version tracking, SQL up/down migrations, and safe rollbacks.

## Background Jobs + Scheduler

Every app needs async work. GoForj includes:

* job definitions
* job registry
* worker execution
* scheduled tasks
* DI-aware job graph

A full job system - no external queue required.

## A Smoother Developer Experience

The `forj dev` command automatically:

* runs your app
* reloads on file changes
* manages environment variables
* spins up docker services if needed

Local development feels frictionless.

# Why Developers Love GoForj

### ✨ Everything is structured

Your app grows in a clean, predictable way.

### ✨ Batteries included, but not restrictive

You get the tools - you’re still writing idiomatic Go.

### ✨ No runtime trickery

All DI and scaffolding is generated as normal Go code.

### ✨ Production-ready from day one

Most frameworks help you start fast; GoForj helps you scale cleanly.

### ✨ Made for real services

Not demos, not toys - GoForj is designed for large, internal, enterprise-grade systems.

# Who GoForj Is For

GoForj is perfect for:

* Backend Go developers
* Teams building internal platforms or services
* Companies who want consistent structure across many Go apps
* Developers tired of wiring the same plumbing every project
* Anyone who wants to build Go services *faster* without giving up Go’s strengths

# The Philosophy of GoForj

GoForj is built on three principles:

### Go should stay Go

No magic, no hidden runtime behaviors.

### Developer experience matters

The framework should remove friction, not add concepts.

### Good structure scales

A well-organized project leads to fewer bugs, easier onboarding, and faster iteration.

# **Start Forging**

The fastest way to understand GoForj is to use it:

```
go install github.com/goforj/goforj/cmd/forj@latest
forj new myapp
forj dev
```

You’ll have a working application in under a minute - fully wired, fully structured, and ready for real development.

**GoForj helps you build better Go applications. Faster. Cleaner. Stronger.**

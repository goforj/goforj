<p align="center">
  <img src="./assets/goforj.png" alt="GoForj" style="border-radius: 5px"/>
</p>

<h1 align="center">GoForj</h1>

<p align="center"><em>⚒️ A non-framework, framework scaffolding and development tool — built for freedom, not chains.</em></p>

<br />

! This project is in very active development. Expect breaking changes and rapid iteration.

## ✨ What is GoForj?

GoForj is a **developer experience-first** CLI and framework core for Go applications.

It gives you **the power of a framework**, without forcing an entire architecture on you.

- 🚀 **Rapid CLI scaffolding** (commands, controllers, migrations, etc.)
- 🛠️ **Project structure bootstrapping**
- 🔌 **Modular plugin system** (opt-in migrations, generators, more)
- 🧩 **Wire/DI** first — true decoupled architecture
- ⚡ **Zero lock-in** — bring your own patterns
- 🐹 **Designed for Go developers** who want more structure, without being handcuffed

---

## 🏗 Example Generated CLI

```bash
🛠 GoForj CLI ❯ A non-framework framework scaffolding and development tool

🛠 Generators ❯
  make:command        Generate a new CLI command
  make:controller     Generate a new controller
  make:migration      Generate a new migration

🧱 Migrations ❯
  migrate             Migrate the database
  migrate:rollback    Rolls back the last migration
```

---

## 🔥 Key Concepts

| Concept | Description |
|:--------|:------------|
| **CLI Core** | Build rich CLIs with `kong`, structured your way |
| **Wire Dependency Injection** | Wire up commands, services, and plugins cleanly |
| **Generators** | Bootstrap your application scaffolding (commands, controllers, migrations) |
| **Plugins** | Add migrations, additional generators, custom features modularly |
| **Zero-Lock In** | You control the project structure, GoForj just accelerates you |

---

## ⚡ Why GoForj?

- **No hidden magic**
- **No heavy runtime dependencies**
- **Focus on what matters**: building your app
- **Developer Experience** prioritized at every step

---

## 🚀 Quickstart

```bash
# Install
go install github.com/goforj/goforj@latest

# Scaffold a new command
goforj make:command HelloWorld

# Scaffold a new controller
goforj make:controller UserController

# Create a new migration
goforge make:migration AddUsersTable

# Run database migrations
goforge migrate

# Rollback last migration
goforge migrate:rollback
```

---

## 💬 Philosophy

GoForj believes frameworks should:

- **Accelerate**, not **dominate** your codebase
- Be **invisible** when you don’t need them
- Be **infinitely extensible** when you grow

GoForj isn’t about "framework rules". It’s about **It’s about giving you tools and structure — without chains.**

---

## 📦 Roadmap

- [x] CLI scaffolding (make:command, make:controller, make:migration)
- [x] Wire-based plugin injection
- [x] Migration system
- [x] Grouped CLI help with rich formatting
- [x] Config scaffolding
- [x] Scheduled tasks (cron-like)
- [x] HTTP scaffolding (Echo/Fiber optional)
- [x] Full application templates
- [x] goforge new <app-name> (full app generator)
- [x] Queue workers (Asynq)

---

## 🤝 Contributing

PRs welcome!  

This project is built for developers who want the **freedom to choose** but **structure to grow**.

---

# ⚒️ Happy forging.

---


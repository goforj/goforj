# **Getting Started with GoForj**

GoForj is a DX-focused framework for building Go applications - giving you structure, scaffolding, and developer tooling without locking you into a rigid architecture.

This guide will help you install the CLI, create your first project, and run it locally.


# Install the GoForj CLI

Make sure you have [Go](https://go.dev/doc/install) 1.21+ installed, then run:

```bash
go install github.com/goforj/goforj/cmd/forj@latest
```

This installs the `forj` command into your `$GOBIN`.

Verify installation:

```bash
forj --version
```

# Create a New Project

Use `forj new` to scaffold a complete application:

```bash
forj new myapp
cd myapp
```

This generates a full project structure including:

* `main.go`
* a dependency-injected application graph
* configuration files
* HTTP server setup
* job runner hooks
* migrations folder
* Docker Compose environment
* prewired project layout
* Makefile and development helpers

GoForj projects follow a consistent, batteries-included structure while still leaving you free to shape your architecture.

# Run the Application**

Start your app in development mode:

```bash
forj dev
```

This will:

* start your app
* watch files for changes
* reload on save
* manage environment variables
* spin up dependent services (Docker) if configured

You now have a running GoForj application.

---

# **4. Generate Controllers, Services, Jobs, and More**

GoForj includes powerful scaffolding commands to accelerate development.

Examples:

```bash
# Create an HTTP controller
forj make:controller users

# Create a background job
forj make:job cleanup_reports

# Create a new database migration
forj make:migration add_users_table
```

Each generator produces idiomatic Go code placed in the correct project layer, wired with dependency injection, and ready for development.

---

# **5. Apply Database Migrations**

If your app uses a database, run migrations:

```bash
forj migrate
```

Migrations follow a simple up/down SQL pattern and are fully versioned.

---

# **6. Build for Production**

To build the final binary:

```bash
forj build
```

You’ll find the compiled binary in the project’s `bin/` directory.

---

# **7. Explore the Project Structure**

A newly generated GoForj app includes clearly separated layers:

```
myapp/
  cmd/
  internal/
    app/          # business services
    http/         # controllers, middleware, routes
    jobs/         # background workers
    database/     # migrations, DB config
    config/       # environment + settings
  wire/           # dependency injection graph
  .env            # environment config
  docker-compose.yml
```

Everything is organized for clarity and long-term maintainability.

---

# **What’s Next?**

* Explore the **Generators**
* Write your first **Controller**
* Create a **Service** and inject it
* Run a **Job**
* Add **Migrations**
* Build out your application using clean, explicit Go code

GoForj gives you a strong foundation, powerful tools, and proven patterns — but never locks you into a fixed architecture. Build the way **you** want, with the structure that helps you move faster.

# Beta Single-App Structure Migration Guide

This is a small structural migration for existing single-app projects.

Your app is still one app. The main change is that GoForj now separates the app's binary entrypoint from the app's composition files.

Your existing app becomes:

```text
app/          # default app composition
cmd/app/      # default app binary
internal/     # shared/domain code, mostly unchanged
```

The multi-app work is why this structure exists, but you do not need to create multiple apps to upgrade.

Move the old root entrypoint:

```text
main.go -> cmd/app/main.go
```

Move app registration and composition into `app/`.

These are the files that describe what your app boots with: routes, commands, lifecycle hooks, schedules, and Wire registration.

```text
Before:
internal/cmd/root_cmd.go
internal/http/routes_registry.go
internal/app/lifecycle_registry.go
internal/schedules/scheduler_registry.go
wire/app.go

After:
app/root_cmd.go
app/commands.go
app/routes.go
app/lifecycle.go
app/schedules.go
app/wire/
```

Keep normal business packages where they are:

```text
internal/users/
internal/orders/
internal/billing/
```

Update `.goforj.yml`:

```yaml
render:
  goforj_version: 0.18.0
```

Do not add `apps:` unless you are creating additional named apps.

Then validate:

```bash
forj build
forj route:list
forj run
forj dev
go test ./...
```

Existing commands still use the default app:

```bash
forj dev
forj migrate
forj make:controller users
forj build
```

Only create named apps later if the project actually needs another runnable boundary:

```bash
forj make:app admin
forj admin route:list
```

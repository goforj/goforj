# Generated App Extension Points

This document explains where users and agents should add custom app behavior in rendered apps.

## Lifecycle Hooks

Primary extension point:

- `internal/app/lifecycle_registry.go`

Use this when adding:

- startup hooks
- shutdown hooks
- custom app runtime wiring

## Routes

Use the app’s route registration surfaces and generated controller wiring.

If a route registration pattern should change for all apps, fix the template/framework source rather than only the rendered app.

## Commands

Custom commands belong in the generated command surfaces, not ad hoc shell wrappers around the app.

If the command shape itself should exist for all apps, fix the template source.

## Schedules

Primary schedule extension point:

- `internal/schedules/scheduler_registry.go`

Rules:

- keep the registry declarative
- give schedules stable explicit names
- prefer direct calls into owning domain types instead of thin scheduler-owned wrapper methods

Good pattern:

- `s.Every(30).Seconds().Name("monitor:poll").Do(s.monitorCheckJob.RunScheduledPoll)`

Avoid:

- accumulating many `runX` helper methods on `Scheduler`
- important anonymous callbacks that produce poor operator/debug visibility

## Lighthouse / Operator Glue

Lighthouse integration files are legitimate extension points when the concern is operator-facing rather than runtime-generic.

For scheduler specifically:

- `internal/schedules/lighthouse.go` is the right place for:
  - Lighthouse command registration
  - schedule DTO shaping for the UI
  - editor-oriented metadata such as explicit editor symbols

Do not force those operator concerns into generic runtime files just to reduce file count.

## Storage

Generated apps now have a richer storage surface than just a single default disk.

Important rules:

- default disk is required
- named disks are optional and may degrade independently
- Lighthouse storage behavior depends on the generated storage manager and discovery wiring

If a storage capability should exist across apps, the fix usually belongs in one of:

- `internal/generate/storages.go`
- `templates/internal/app/discovery.go.tmpl`
- Lighthouse templates under `templates/internal/lighthouse/...`

Do not patch only the rendered app if the intent is to change how disks are discovered, labeled, degraded, or surfaced in Lighthouse for all apps.

Common generated files worth inspecting during storage issues:

- `internal/storages/manager_gen.go`
- `internal/app/discovery.go`

## Environment Policy

Project-specific env values live in the rendered app.

Framework-wide env conventions belong in GoForj templates and runtime code.

Recent practical lesson:

- `.env` changes during `forj dev` are a supervisor/watcher concern, not just an app runtime concern
- if the bug is about stale env in build/run watchers, the fix belongs in `forj dev`, not in the generated app

## User Code vs Framework Code

A good rule:

- app-specific business behavior belongs in the rendered app
- framework/runtime shape belongs in GoForj source

If rerender should preserve the behavior, the real fix likely belongs in GoForj.

Applied to recent scheduler work:

- scheduler admin/introspection primitives belong in `scheduler/v2` when they are UI-agnostic
- Lighthouse command wiring and UI payload shaping still belong in the generated app

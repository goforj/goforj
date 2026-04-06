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

## Environment Policy

Project-specific env values live in the rendered app.

Framework-wide env conventions belong in GoForj templates and runtime code.

## User Code vs Framework Code

A good rule:

- app-specific business behavior belongs in the rendered app
- framework/runtime shape belongs in GoForj source

If rerender should preserve the behavior, the real fix likely belongs in GoForj.

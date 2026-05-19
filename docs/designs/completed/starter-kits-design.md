# Starter Kits Design

## Purpose

Starter kits are opinionated application scaffolds layered on top of GoForj's base component model.

They exist to answer a different question than components do.

- components answer: what capabilities does this app have?
- starter kits answer: what does the first usable product/app experience look like?

That distinction matters.

Starter kits should give users a stronger starting point than "empty app with selected components" without turning frontend/application opinions into framework primitives.

The desired model is:

- the framework stands on its own
- starter kits are optional
- starter kits provide a strong product-facing starting point
- the generated code is owned by the app after creation

## Goals

Starter kits should:

- be optional
- be selected after base components are chosen
- produce polished, buildable starting applications
- adapt to selected framework components instead of replacing them
- leave the generated app fully owned by the user
- make the first official kit feel production-minded, not demo-only

Starter kits should not:

- become hidden dependency bundles
- redefine core framework primitives
- require users to keep a starter package in sync forever
- prevent rendering a plain app with no starter kit
- introduce a second component system parallel to the real one

## Product Model

Starter kits are not primitives.

They are a GoForj-generated application layer composed from:

- frontend structure
- route/page organization
- design system defaults
- app shell layout
- auth-facing pages and flows when auth is enabled
- dashboard / home / settings starting points
- example patterns users can keep or delete

The base framework remains responsible for:

- auth
- mail
- jobs
- scheduler
- database
- API/runtime wiring
- rendering/generation infrastructure

The starter kit remains responsible for:

- frontend/app-shell experience
- page structure
- design system and visual defaults
- opinionated user-facing flows

## Position In The New Project Flow

Starter kit selection should be a separate wizard step, not a component.

Recommended flow:

1. project basics
2. component selection
3. starter kit selection when relevant
4. extras
5. runtime
6. project path
7. confirm

This preserves a clean mental model:

- components define framework capability
- starter kits define initial app/product shape

## Configuration Model

Starter-kit selection should be represented explicitly in project configuration.

It should not be inferred indirectly from generated frontend files or from component combinations.

The config model should have a clear, centralized value for:

- no starter kit
- Vue starter kit
- future React starter kit
- future `templ + htmx` starter kit

This should be modeled as a constrained starter-kit identity, not as an ad hoc string scattered through wizard code, renderer code, and tests.

The same principle used for the component catalog should be followed here:

- one centralized starter-kit catalog
- one place to define label, description, compatibility, and future variant metadata
- wizard behavior and renderer behavior should both read from that contract

## When The Starter Kit Step Appears

The starter-kit step should appear only when the selected components make it relevant.

Initial rule:

- if `WebUI` is enabled, show starter kit options
- if `WebUI` is not enabled, skip the starter-kit step entirely

This keeps v1 simple and avoids pretending there is a meaningful frontend starter decision for non-UI apps.

Possible future expansion:

- if GoForj later has a meaningful non-WebUI starter concept, model it explicitly instead of overloading this same step

## Compatibility And Assumptions

The first official Vue starter kit should be explicit about what it supports.

For v1, the intended compatibility model should be:

- `WebUI` is required
- `Auth` should be supported when enabled
- `Auth` should not be required
- `WebAPI` should be supported naturally if enabled
- the starter kit should not assume every app has every optional component

That means the Vue kit should render coherently for at least:

- `WebUI` only
- `WebUI + Auth`
- `WebUI + WebAPI`
- `WebUI + WebAPI + Auth`

If some combinations are intentionally unsupported, that should be stated explicitly in the starter-kit contract instead of being discovered accidentally during rendering.

## Initial Starter Kit Inventory

Official first-party direction:

- `None`
- `Vue`

Planned next kits after Vue is complete:

- `React`
- `templ + htmx`

That sequence matters.

Vue should be fully rounded and treated as the reference implementation for starter-kit architecture. React should follow once the architecture is proven. `templ + htmx` should be treated as a distinct interaction model, not merely a non-JS variant of the same frontend.

## Why Vue First

Vue is the right first kit because:

- it gives GoForj a strong full-stack frontend story early
- it pairs well with a Shadcn-style design-system approach through `shadcn-vue`
- it provides a practical path to polished auth/account/dashboard flows
- it is a good reference point for how later React starter kits should be structured

The first Vue kit should feel like an official "this is how GoForj apps can start" answer, not just "Vue support exists".

## Design System Direction

Starter kits should use Shadcn-style component ecosystems where that is a good fit.

Initial plan:

- Vue starter kit uses `shadcn-vue`
- React starter kit uses `shadcn/ui`

Why this fits:

- strong baseline component coverage
- good design quality out of the box
- source-owned components copied into the app
- easy for users to modify after generation

That last point is important. GoForj should not render a starter kit that depends on a private, magical UI layer users do not own. The app should own the resulting UI code.

## Product Reference Shape

The important product shape is:

- starter kits are a fast path, not a requirement
- starter kits should be the recommended way to get a polished full-stack app off the ground quickly
- starter kits should scaffold both frontend and backend-facing auth/application flows in a way users can inspect and learn from

The broader frontend lesson is also useful:

- users should be able to pick an official frontend starting point
- auth/account/app-shell flows should feel coherent immediately
- the resulting app should feel like a real starting product, not a thin demo

## Relationship To The Component Model

Starter kits must adapt to selected components instead of fighting them.

Examples:

- if `Auth` is enabled, the starter kit should render auth-facing pages and account flows
- if `Auth` is not enabled, auth-specific pages should not be rendered
- if `Mail` exists because `Auth` implies it, the starter kit still should not own mail behavior
- if `WebAPI` is enabled, the starter kit should integrate naturally with GoForj's API/runtime model
- if `WebAPI` is not enabled, the starter kit should still be able to render a coherent `WebUI` app shell if that combination remains supported

This means the starter kit is not one fixed output. It is an opinionated layer that adapts to the selected base capabilities.

## Relationship To Auth

Starter kits are not auth.

The auth component owns:

- routes
- controllers/handlers
- sessions
- verification/reset behavior
- security model

The starter kit should own the app-facing experience for auth when auth is enabled.

For the Vue starter kit, that means polished pages and flows for:

- sign in
- sign up, if supported
- forgot password
- reset password
- verify email states
- signed-in app shell
- basic account / profile / settings structure

If `Auth` is not enabled:

- those pages should not be rendered
- the kit should still provide a coherent signed-out and signed-in-neutral shell as appropriate

## Relationship To Mail

Starter kits should not own mail delivery behavior.

If the kit renders password reset or email verification experiences, it should assume the underlying framework auth/mail stack exists and provide the frontend/user-facing flow only.

## Relationship To Web API / Frontend Transport

Vue and React starter kits should work naturally with GoForj's backend/runtime model.

That likely means:

- structured frontend API usage
- generated client/route helpers where needed
- consistent session and CSRF behavior
- clear server-authoritative auth expectations

Some full-stack frameworks use an Inertia-like bridge heavily here. GoForj does not need to decide "copy that exactly" at the design-doc level.

The real question is:

- what is the thinnest, cleanest bridge between GoForj routes/controllers/auth and a modern frontend app?

The starter-kit design should leave room for that decision without requiring a large, magical bridge layer up front.

## Vue Starter Kit Scope

The Vue starter kit should be completed end to end before broadening to React or `templ + htmx`.

That means it should define:

- frontend project structure
- app shell layout
- navigation model
- page organization
- auth-facing pages and transitions when auth is enabled
- dashboard / landing page starting points
- form patterns
- table / empty / loading / error states
- design tokens / theme defaults
- dev/build expectations
- docs for how the generated frontend is structured

It should feel like a coherent app baseline, not a component demo.

## Frontend Tooling Contract

The Vue starter kit should have a clear tooling contract.

That contract should eventually define:

- the frontend build tool
- the package manager expectation
- how Shadcn-Vue components are introduced
- which generated scripts are expected in the app
- how frontend local development works with `forj dev`

For v1, the likely direction is:

- Vite-based frontend build
- normal source-owned frontend code in the generated app
- Shadcn-Vue components copied/generated into the app rather than hidden behind a private package

That tooling posture should be explicit because it directly affects starter-kit ownership and long-term maintainability.

## Suggested Vue Starter Kit Contents

The first Vue kit should likely include:

- application shell with top nav and/or sidebar
- home/dashboard page
- auth pages
- account/settings page skeleton
- profile form example
- form validation and submit states
- empty state patterns
- toast / alert patterns
- loading and error patterns
- responsive nav behavior
- mobile-aware app shell behavior

It should also include a small set of high-quality example pages so users can see:

- page composition
- layout composition
- form composition
- list/detail composition

## Wizard Model

The wizard should not ask too many frontend questions in v1.

Recommended initial experience:

- only show the starter-kit step when `WebUI` is enabled
- show a short list:
  - `None`
  - `Vue`
- describe the kit briefly in-line
- keep React and `templ + htmx` out until they are actually implemented

Possible later expansions:

- layout variant
- auth visual variant
- dashboard variant
- "include example pages" vs "minimal shell"

But v1 should stay focused.

## Render Model

Starter kits should render as a clearly separate layer on top of the base component render.

Conceptually:

1. render normal selected components
2. apply starter-kit templates/overrides where relevant

That separation is important because it keeps starter-kit logic understandable and makes it easier to:

- skip kits entirely
- add more kits later
- reason about ownership
- test starter-kit behavior without mixing it into every base template

## Render Ownership Boundaries

Starter-kit layering should have predictable ownership lines.

The base component render should continue to own:

- framework runtime wiring
- auth/mail/jobs/scheduler/database capability wiring
- low-level generated app structure that is not starter-specific

The starter kit may own or override:

- frontend app shell
- page structure
- starter-specific layouts
- starter-specific auth/account views
- design system assets and components it introduces

The goal is not "starter kits can override anything anywhere". The goal is a bounded opinion layer on top of a stable base render.

That boundary should stay explicit so starter kits do not turn into hidden forks of the full render pipeline.

## File Structure Direction

Starter-kit templates should remain clearly separated from normal component templates.

A likely direction is:

```text
templates/starter-kits/
  vue/
  react/
  templ_htmx/
```

Or a similar structure where starter-kit assets are clearly isolated.

That matters because starter-kit code is more opinionated than base framework code and should not be mixed into the general component template tree indistinguishably.

## Ownership Philosophy

Starter kits should provide ownership, not lock-in.

Meaning:

- after render, the app owns the starter-kit code
- users can modify it freely
- future GoForj upgrades should not assume starter-kit files remain untouched

The generated result should be a starting point, not a package users are expected to keep updating forever.

## Documentation Expectations

Each official starter kit should eventually have user-facing docs that explain:

- what the kit includes
- what component combinations it expects or adapts to
- the generated frontend structure
- how Shadcn components are sourced and owned
- how auth-facing flows are handled
- how users should extend it safely

The Vue starter kit should get this level of documentation as part of being "complete".

## Verification Bar For An Official Starter Kit

A starter kit should not be considered official and complete merely because it renders.

The minimum verification bar should include:

- generated app render passes
- generated app build passes
- generated app tests pass
- local development flow works
- auth-facing flows are rendered correctly when auth is enabled
- non-auth variant still renders coherently when auth is not enabled
- responsive/app-shell behavior is sane on desktop and mobile
- docs accurately describe what was generated

For the first Vue starter kit, "fully rounded" should mean it meets that bar, not just that it produces frontend files.

## Upgrade Philosophy

Starter-kit files should be considered app-owned after render.

GoForj should not assume those files remain upgrade-safe or untouched.

By contrast, base framework-managed wiring and generated infrastructure can continue following the normal GoForj generation model.

That distinction matters:

- starter-kit UX code is a starting point users will customize heavily
- framework infrastructure code remains part of GoForj's maintained generation surface

The design should preserve that difference clearly.

## v1 Scope

First slice should focus on:

- wizard support for starter-kit selection
- starter-kit step shown only when `WebUI` is enabled
- `None` and `Vue` starter-kit choices
- Vue starter kit rendered as a coherent frontend/app-shell/auth baseline
- Shadcn-Vue based design-system direction
- clear docs about what the Vue kit includes

## Explicitly Out Of Scope For First Slice

- React starter kit implementation
- `templ + htmx` starter kit implementation
- multiple visual/layout variants in the wizard
- team/workspace variants
- starter-kit marketplace/community kits
- alternate auth-provider starter families

Those should come only after the first-party Vue architecture is proven.

## Future Kit Contract

Vue should not become a one-off special case.

Future official kits such as React or `templ + htmx` should satisfy the same high-level contract:

- clear compatibility rules
- isolated starter-kit templates
- coherent app shell
- auth-aware adaptation
- source-owned generated code
- docs and verification bar equivalent to the Vue kit

This keeps the starter-kit architecture consistent as the catalog grows.

## Future Directions

Once Vue is mature, likely additions are:

- React starter kit
- `templ + htmx` starter kit
- richer settings/account/dashboard patterns
- official admin/observability/dashboard-focused kit variants later
- optional community-maintained starter kits after the model is stable

Longer term, GoForj could also support:

- starter-kit packages
- render-time starter selection outside the wizard
- more curated kit families

But that should happen only after first-party kits are stable and the ownership model is proven.

## Final Principle

Components define framework capability.

Starter kits define the initial application/product experience.

GoForj should keep those concepts separate so users can either:

- render a plain app from raw components
- or start from an official polished app baseline without losing ownership of the generated code

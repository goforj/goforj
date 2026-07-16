# templ + htmx Starter Kit Design

Status:

- completed for v1; maker commands and additional polish remain follow-up work
- intended as GoForj's first official server-rendered template starter kit
- complementary to the Vue and React starter kits

## Purpose

This design defines GoForj's official template-based starter kit.

The goal is to give GoForj a server-rendered UI option that fills the same
product role Laravel Blade fills in Laravel:

- simple first-page rendering
- server-owned routing and controller flow
- app-owned template files
- progressive interactivity without requiring a SPA
- strong defaults for auth, forms, layout, and partial updates

The recommended stack is:

- `templ`
- `htmx`
- Tailwind CSS
- Basecoat for shadcn-style HTML primitives

This kit should be named:

```text
templ_htmx
```

or another explicit identity that keeps both parts visible. Do not call it only
`html`, `server`, or `templates`; the stack choice is part of the value.

## Recommendation

GoForj should adopt `templ + htmx` as the first-party server-rendered starter
kit.

GoForj should not build a custom Blade-like template language.

The framework should instead provide:

- starter-kit selection and rendering
- app-local `templ` file layout
- generated view/controller examples
- htmx-aware form and partial response conventions
- auth-aware server-rendered pages
- `forj dev` integration for `templ generate` and CSS builds
- `forj make:*` commands for common template/page/partial additions

The generated app should own all view source after render.

## Reference Material

- Existing starter-kit architecture:
  `docs/designs/completed/starter-kits-design.md`
- Existing Vue starter kit:
  `templates/starter-kits/vue/frontend`
- Planned React starter kit:
  `docs/designs/completed/react-starter-kit-design.md`
- Go `html/template` contextual escaping:
  https://pkg.go.dev/html/template
- templ components:
  https://templ.guide/core-concepts/components/
- templ injection guidance:
  https://templ.guide/security/injection-attacks/
- templ attribute and URL sanitization:
  https://templ.guide/syntax-and-usage/attributes/
- templ IDE support:
  https://templ.guide/developer-tools/ide-support/
- htmx documentation:
  https://htmx.org/docs/
- templ + htmx docs:
  https://templ.guide/server-side-rendering/htmx/
- templUI component-library direction:
  https://templui.io/docs/introduction

## Why This Stack

### `templ`

`templ` is the best fit because it keeps the template system close to Go.

Important properties:

- components compile to Go functions
- components return `templ.Component`
- rendering uses `Render(ctx, io.Writer)`
- public/private component visibility follows Go naming rules
- templates can be shared through normal Go modules
- generated output is visible and buildable
- IDE/LSP support exists for common editors

This matches GoForj's existing posture:

- source-owned generated code
- compile-time feedback
- explicit app package boundaries
- no hidden runtime magic
- normal Go module behavior

### htmx

`htmx` is the right default progressive interaction layer because it keeps the
server-rendered contract intact.

Important properties:

- server handlers return HTML
- page fragments can be swapped without a full SPA
- forms and buttons can use normal HTTP verbs
- no build system is required for basic use
- it has a small dependency footprint
- it works naturally with server-rendered partials

GoForj should treat htmx as a progressive enhancement layer, not as a replacement
for normal links, forms, redirects, or HTTP semantics.

### Tailwind CSS And Basecoat

Tailwind is already aligned with the Vue and React starter-kit direction through
shadcn-style UI systems.

The `templ + htmx` kit should use Tailwind so visual language can stay close to
the Vue and React kits without requiring a JavaScript framework.

Basecoat should provide the default shadcn-style HTML component layer for the
templ starter kit. It is a better fit than framework-specific shadcn packages
because it targets plain HTML, ships Tailwind-compatible styles, supports
Lucide icon usage, and includes small optional JavaScript modules for interactive
components.

## Relationship To Vue And React Kits

The `templ + htmx` starter kit should be a visual and product parity
implementation of the Vue and React starter kits.

Users should be able to choose `Vue`, `React`, or `templ + htmx` and get the
same first impression:

- polished app shell
- coherent navigation
- auth pages
- settings/account pages
- dashboard starting point
- form, table, empty, loading, and error states
- responsive behavior

The difference should be the rendering and interaction model, not the visual
language, page taxonomy, or product quality.

The `templ + htmx` kit should match Vue and React in:

- sidebar structure
- site header and breadcrumbs
- brand/team switcher area
- main navigation groups
- user menu shape
- dashboard page composition
- auth page composition
- settings page composition
- component showcase coverage
- theme behavior
- spacing, typography, color tokens, and density
- desktop and mobile shell behavior

When users compare the three starter kits in a browser, they should feel like
the same GoForj starter app implemented through three different UI stacks.

It should differ only where the stack requires a different implementation:

- server routes own page rendering
- controllers return pages or fragments
- htmx enhances forms and partial updates
- minimal JavaScript is used for local UI behavior only

Vue and React remain the best choices for rich client-side applications. The
`templ + htmx` kit is the best choice for server-rendered apps, CRUD-heavy
internal tools, admin panels, simple SaaS dashboards, content-heavy apps, and
teams that want fewer frontend moving parts.

## Visual Parity Contract

The `templ + htmx` starter kit should match the same first-screen structure used
by Vue and React:

```text
App shell
  Sidebar
    Team switcher / brand area
    Main navigation
    Documents / secondary navigation
    User menu

  Main surface
    Header
      Sidebar trigger
      Breadcrumbs
      Page title/icon

    Content
      Current page

  Feedback
    Flash messages / alerts
    Loading and empty states
```

The target is not byte-for-byte markup parity. The target is visual parity:

- same layout rhythm
- same navigation model
- same page hierarchy
- same responsive behavior in practice
- same theme tokens where practical
- same quality bar for states and forms

If Vue or React adds a new official starter-kit page before this kit is built,
the `templ + htmx` implementation should copy the current route/page set unless
the page depends on SPA-only behavior.

## Product Positioning

The starter-kit catalog should eventually read like:

```text
None
Vue
React
templ + htmx
```

Suggested descriptions:

- `None`: Generate framework components without a frontend starter.
- `Vue`: Polished Vue SPA starter with app-owned shadcn-vue components.
- `React`: Polished React SPA starter with app-owned shadcn/ui components.
- `templ + htmx`: Server-rendered Go templates with progressive HTML updates.

The wizard should show `templ + htmx` only when `WebUI` is enabled.

## Goals

- Provide a first-class server-rendered UI path.
- Keep views app-owned after render.
- Avoid creating a GoForj-specific template language.
- Keep page rendering, partial rendering, and form handling ergonomic.
- Integrate naturally with GoForj auth, sessions, cookies, routes, and CSRF.
- Keep interactivity progressive and inspectable.
- Make the kit useful without requiring users to learn a full SPA stack.
- Keep build and dev commands deterministic.
- Support named apps under the same app-local ownership model as Vue and React.
- Preserve the starter-kit distinction from core framework primitives.

## Non-Goals

- Do not replace the Vue or React starter kits.
- Do not introduce Inertia, Livewire, Phoenix LiveView, or Hotwire clones in the
  first slice.
- Do not create a custom Blade-like GoForj template DSL.
- Do not make `templ` a required dependency for API-only or SPA starter apps.
- Do not depend on a private GoForj UI package for normal app UI.
- Do not make htmx the app's transport for JSON API routes.
- Do not add a full realtime/stateful component protocol in v1.
- Do not require JavaScript bundling for basic server-rendered pages.

## Starter Kit Identity

Add a first-party starter kit identity:

```go
project.StarterKitTemplHTMX
```

Config value:

```yaml
render:
  starter_kit: templ_htmx
```

Named app config:

```yaml
apps:
  admin:
    components: [web_ui]
    starter_kit: templ_htmx
```

The exact config string may use `templ-htmx` if GoForj prefers kebab-case for
user-facing names. Internally, use a stable enum value and keep parsing
backward-compatible if the spelling changes before release.

## File Layout

Recommended generated layout:

```text
cmd/<app>/frontend/
  package.json
  package-lock.json
  tailwind.config.ts
  src/
    style.css
  public/
    htmx.min.js
    goforj-logo.png

internal/views/
  pages/
    dashboard.templ
    auth.templ
    settings.templ
    components.templ
  partials/
    flash.templ
    form_errors.templ
    table.templ
  layouts/
    app.templ
    public.templ
  components/
    button.templ
    card.templ
    input.templ
    nav.templ
    sidebar.templ
  viewmodels/
    auth.go
    layout.go
    settings.go
```

For named apps, prefer app-scoped view packages if multiple WebUI apps can have
different views:

```text
internal/views/<app>/
  pages/
  partials/
  layouts/
  components/
  viewmodels/
```

If Go package ergonomics make `internal/views/<app>` awkward, use:

```text
app/<app>/views/
```

The deciding rule should be ownership:

- app-specific pages belong with the app
- reusable framework/runtime helpers belong in GoForj templates or sibling
  packages
- user-editable UI source should be obvious and app-owned

## Generated Source Ownership

The starter kit should copy source into the app.

GoForj should not assume it can safely overwrite user-edited `.templ`, CSS, or
small JavaScript files after initial creation.

Framework-managed generated files may still be rerendered, but app-owned starter
files should follow the same preservation model as Vue and React starter files.

## View Models

Use explicit view model structs for pages and partials.

Example shape:

```go
// DashboardPage is the data rendered by the dashboard view.
type DashboardPage struct {
	User      UserSummary
	Stats     []StatCard
	Recent    []ActivityItem
	Flash     FlashMessage
	CSRFToken string
}
```

Rules:

- do not pass raw repositories or services into views
- do not perform database calls inside `.templ` files
- controllers prepare view models
- views render view models
- partial view models should be small and named

This keeps rendering testable and prevents templates from becoming a second
service layer.

## Controller Model

Generated server-rendered controllers should use normal GoForj web/controller
patterns.

The controller decides whether to return:

- a full page
- an htmx partial
- a redirect
- an error page
- a JSON response for API routes, when explicitly building API behavior

Recommended helper concept:

```go
func RenderPage(ctx web.Context, component templ.Component) error
func RenderPartial(ctx web.Context, component templ.Component) error
func IsHTMX(ctx web.Context) bool
func Redirect(ctx web.Context, path string) error
```

The actual helper names should match the `web` repo's abstractions if this
belongs there. If the helper is generic web behavior, put it in `web`. If it is
GoForj template policy, keep it in GoForj generated code.

## htmx Request Semantics

GoForj should support a small, explicit htmx convention.

Useful request detection:

- `HX-Request`
- `HX-Target`
- `HX-Trigger`
- `HX-Current-URL`

Useful response headers:

- `HX-Redirect`
- `HX-Refresh`
- `HX-Trigger`
- `HX-Reswap`
- `HX-Retarget`

Do not wrap all controller responses in htmx abstractions. Keep ordinary HTTP
clear, and add htmx helpers only where they reduce repeated header handling.

## Route Shape

The starter kit should include page routes for the selected app.

Suggested routes when auth is enabled:

```text
GET  /
GET  /login
POST /login
POST /logout
GET  /register
POST /register
GET  /forgot-password
POST /forgot-password
GET  /reset-password
POST /reset-password
GET  /verify-email
POST /verify-email/request
GET  /settings
GET  /settings/profile
POST /settings/profile
GET  /settings/password
POST /settings/password
GET  /settings/appearance
POST /settings/appearance
```

Suggested routes without auth:

```text
GET /
GET /components
GET /components/forms
GET /components/data
```

The route names and paths do not need to mirror the Vue kit exactly, but they
should feel like the same GoForj starter product.

## Auth Integration

The starter kit owns user-facing auth pages. The auth component owns the auth
primitive.

The template kit should render auth-facing pages when `Auth` is enabled:

- sign in
- register, if registration exists
- forgot password
- reset password
- verify email
- profile settings
- password settings
- logout affordances

The template kit should not duplicate auth service behavior.

Controllers should call the generated auth service or auth HTTP surface through
normal app-owned code. They should not reimplement password validation, session
rotation, token verification, login attempt policy, or bootstrap behavior.

## CSRF And Form Policy

Server-rendered forms need a first-class CSRF story.

The design should include:

- generated CSRF middleware when `WebUI` renders forms
- helpers to place CSRF tokens in view models
- a standard hidden field component
- htmx-compatible CSRF header support
- clear failed-CSRF error behavior

If GoForj already has or later adds a generic CSRF primitive, this kit should
consume it rather than owning CSRF itself.

Form submissions should support both:

- normal browser POST redirect flow
- htmx partial replacement flow

The controller should choose the response based on request headers, not by
duplicating route trees.

## Validation And Errors

Use explicit form structs and validation results.

Recommended pattern:

```go
type LoginForm struct {
	Username string
	Password string
	Remember bool
}

type FormErrors map[string][]string
```

Views should receive:

- submitted values safe to re-display
- field-level errors
- form-level errors
- CSRF token
- flash messages

Do not put validation logic in templates.

## Flash Messages

The server-rendered kit should include a flash-message convention.

Flash messages should work for:

- full page redirects
- htmx partial responses
- auth success/failure states
- form validation failures

For htmx, support either:

- returning a flash partial in a known target
- emitting `HX-Trigger` events that update a flash/toast region

Start simple. Prefer visible inline feedback over a complex toast runtime in v1.

## Design System

The first version should use app-owned `templ` components, Tailwind CSS, and
Basecoat CSS/JavaScript as the plain-HTML shadcn layer.

Basecoat should be treated as a frontend asset dependency, not a framework
primitive. Generated templates should still be owned by the app, and GoForj
should namespace app-shell-only classes so Basecoat component classes do not
accidentally take over framework layout.

Do not hard-depend on templUI in v1.

Reason:

- Basecoat is directly aligned with the immediate need: shadcn-style UI without
  React or Vue
- templUI is directionally aligned with GoForj's source-owned component model
- templUI may become useful as inspiration or a future optional source
- GoForj should not let its official server-rendered starter depend on a young
  component ecosystem before the integration contract is proven

The generated component set should include:

- button
- link button
- input
- textarea
- select
- checkbox
- label
- field
- form error
- alert
- badge
- card
- table
- pagination
- avatar
- dropdown/menu if needed
- sidebar/nav
- breadcrumb
- empty state
- loading state

Keep components boring and maintainable. The template kit should be easy to
delete, copy, and modify.

## JavaScript Policy

Use minimal JavaScript.

Allowed by default:

- htmx
- a small app-owned local interactivity layer
- small app-owned scripts for navigation, theme, and focus behavior
- optional Alpine-style behavior only if a specific component truly needs it

Avoid in v1:

- large client-side state managers
- SPA routing
- client-side auth state machines
- bundler-required interactivity for basic pages
- inline scripts that make CSP harder

The preferred default is:

- static copied `htmx.min.js`
- app-owned `app.js` only if needed
- Tailwind build for CSS

## Local Interactivity Layer

`templ` handles rendering and htmx handles server-driven updates, but the starter
kit also needs a sanctioned path for local-only page behavior that should not hit
the backend.

Examples:

- table sorting
- table filtering over already-rendered rows
- column visibility
- local tabs
- disclosure panels
- combobox filtering over a small static option set
- command-menu filtering
- theme toggles
- sidebar collapse state
- copy-to-clipboard actions

The recommended v1 direction is a tiny app-owned vanilla JavaScript controller
layer, not a custom reactive framework.

Suggested generated shape:

```text
cmd/<app>/frontend/
  src/
    app.ts
    controllers/
      index.ts
      table.ts
      disclosure.ts
      theme.ts
```

Controllers should attach to explicit data attributes:

```html
<table data-gf-controller="table" data-gf-table>
  <input data-gf-table-filter placeholder="Filter">
  <th>
    <button data-gf-table-sort="name">Name</button>
  </th>
  <tr data-gf-table-row>
    <td data-gf-table-value="name">Acme</td>
  </tr>
</table>
```

Rules:

- the JavaScript layer is app-owned source, not a hidden GoForj runtime
- data attributes are the public contract between `templ` markup and local
  behavior
- controllers should be small, composable, and easy to delete
- controllers should not own auth, routing, persistence, or backend state
- htmx remains the path for server-side data changes
- local controllers are for behavior over data already present in the DOM
- generated examples should favor progressive enhancement and preserve ordinary
  form/link behavior where practical

This gives agents and users a stable place to add script-like behavior without
creating a SPA or forcing every small interaction through htmx.

Alpine.js remains a reasonable optional escape hatch for apps that prefer
declarative client-side state, but it should not be the required default for the
official kit. Keeping the default layer vanilla and data-attribute driven keeps
the generated output easier to audit, easier to test, and less dependent on a
second framework.

## Build And Dev Workflow

The generated app should have a deterministic local loop:

```bash
templ generate
cd cmd/app/frontend
npm install
npm run build
cd ../../..
GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache go build ./...
```

`forj dev` should:

- watch `.templ` files
- run `templ generate` after template changes
- watch Tailwind input files
- rebuild CSS after class changes
- restart the app only when generated Go output changes
- keep watcher output concise

`forj build` should:

- run `templ generate`
- build CSS assets
- run Wire/build as usual

The generated app should fail clearly if the `templ` CLI is missing.

## Dependency Model

Generated apps using this starter should include:

- `github.com/a-h/templ`
- htmx as a copied static asset or npm dependency
- Tailwind CSS tooling

Prefer copied static htmx for v1 unless the final frontend directory already
needs npm for Tailwind.

If npm is already required for Tailwind, htmx may be pinned through
`package.json` and copied/bundled into public assets during build. The important
rule is that production output must not depend on a CDN by default.

## Renderer Changes

Required GoForj changes:

- add `project.StarterKitTemplHTMX`
- add the kit to the starter-kit catalog
- show the kit in the wizard when `WebUI` is enabled
- accept `--starter-kit templ_htmx` in `make:app`
- persist the starter-kit choice in `.goforj.yml`
- render `templates/starter-kits/templ-htmx`
- support named app rendering
- preserve app-owned starter files on rerender
- include `templ generate` in build/dev flows for apps that select the kit
- include frontend CSS/static asset build steps where needed

The renderer should generalize starter-kit dispatch rather than adding another
one-off Vue-style path.

Suggested shape:

```text
scaffoldStarterKitForApp(app, starterKit, overwrite)
  starterKit == vue        -> copy starter-kits/vue/frontend
  starterKit == react      -> copy starter-kits/react/frontend
  starterKit == templ_htmx -> copy/render starter-kits/templ-htmx
```

The `templ_htmx` kit may need both raw-copy and rendered-template behavior
because package names and app names affect generated Go files.

## Maker Commands

The starter kit should eventually include maker commands.

Initial command candidates:

```bash
forj make:view users/index
forj make:page users
forj make:partial users/row
forj make:form users
```

For named apps:

```bash
forj admin make:page users
forj admin make:partial users/row
```

Maker output should include:

- `.templ` file
- view model file when needed
- controller method or route registration when explicitly requested
- tests only when the command generates meaningful behavior

Do not make maker commands mandatory for hand-authored templates. They are an
ergonomic accelerator, not the only supported path.

## Testing

Renderer tests:

- `forj new` can select `templ_htmx`
- `make:app --starter-kit templ_htmx` creates app-local template files
- named app templates render under the correct package/app boundary
- API-only apps do not render the template kit
- existing app-owned template files are preserved on rerender
- selected app config persists the starter-kit identity

Build tests:

- `templ generate` succeeds
- Tailwind/CSS build succeeds
- generated Go app builds
- generated Go tests pass
- embedded/static assets are available

Auth tests:

- auth-enabled render includes auth pages
- auth-disabled render omits auth pages and nav links
- login form posts to the expected route
- logout works from normal and htmx flows
- validation errors render without losing safe submitted values
- CSRF token appears in generated forms

htmx tests:

- htmx request returns partial HTML where expected
- normal request returns full page HTML
- redirects use normal HTTP for non-htmx requests
- htmx redirects use the selected htmx response convention
- flash/error regions update correctly

Visual tests:

- desktop app shell screenshot compared against Vue/React structure
- mobile app shell screenshot compared against Vue/React structure
- login/public shell screenshot compared against Vue/React structure
- dashboard screenshot compared against Vue/React structure
- settings/profile screenshot compared against Vue/React structure
- form validation screenshot

The screenshots do not need to be pixel-identical because the underlying DOM and
component implementations will differ. They should prove that the starter kits
look like the same GoForj product across Vue, React, and `templ + htmx`.

## Documentation

User-facing docs should explain:

- when to choose `templ + htmx`
- generated file layout
- how full pages differ from partials
- how htmx requests are detected
- how form validation works
- how CSRF is handled
- how auth pages use the auth component
- how to add a page
- how to add a partial
- how `templ generate` fits into `forj dev` and `forj build`
- how to customize or delete starter-kit components

Docs should be explicit that starter-kit UI source is app-owned after render.

## Alternatives Considered

### Custom GoForj Template Language

Rejected for v1.

A custom template language would create a large maintenance surface and compete
with Go's existing ecosystem. GoForj should spend its effort on integration,
generation, auth/forms ergonomics, and app structure rather than a new parser,
formatter, LSP, and escaping model.

### `html/template`

Keep as a conceptual baseline, but do not make it the official starter-kit UX.

It is secure and stable, but its composition model is less ergonomic than
`templ` for an official app starter with reusable components and typed view
models.

### Jet

Jet has familiar inheritance/composition features and auto-escaping, but it
introduces a separate runtime template DSL. That is less aligned with GoForj's
compile-time, Go-native generated-code model.

### Pongo2

Pongo2's Django-like syntax is approachable, but it also creates a separate
template language and runtime model. It does not give GoForj enough advantage
over `templ` to justify making it the official path.

### quicktemplate

quicktemplate is performance-oriented, but the official starter-kit decision
should optimize for maintainable app development and framework ergonomics before
raw template throughput.

### gomponents

gomponents is Go-native and type-safe, but authoring HTML entirely as Go
function calls is less approachable for page templates than `.templ` files.

### Inertia

Inertia can be useful for server-routed Vue/React apps, but it is not the
server-rendered template answer. It also adds a bridge protocol that should not
be the foundation of GoForj's first template kit.

### Hotwire/Turbo

Turbo is a credible HTML-over-the-wire option, but htmx is smaller, simpler,
and easier to integrate incrementally without importing Rails-shaped concepts.

### Livewire / LiveView Style Runtime

Do not build this in v1.

A stateful component protocol is a much larger product than a starter kit. If
GoForj later wants this, it should be designed as its own runtime subsystem, not
smuggled into the template starter.

## Implementation Phases

### Phase 1: Design And Identity

- add `StarterKitTemplHTMX`
- add catalog metadata
- add wizard and `make:app` selection
- add renderer dispatch
- document the selected stack

### Phase 2: Minimal Renderable Kit

- create starter-kit template directory
- add layout, dashboard, components page, and static assets
- add Tailwind build
- add `templ generate` build/dev support
- render and build a temp app

### Phase 3: Auth-Aware Pages

- add login/logout/register/reset/verify pages when `Auth` is enabled
- add profile/password/settings pages
- add form view models and validation rendering
- add CSRF support if not already present
- test normal and htmx flows

### Phase 4: htmx Patterns

- add partial response helpers
- add example table/list partial
- add flash/error update behavior
- add tests for full-page versus partial responses

### Phase 5: Maker Commands

- add `make:view`
- add `make:page`
- add `make:partial`
- add named-app routing behavior
- document generated output

### Phase 6: Polish And Verification

- responsive screenshots
- docs
- temp render smoke
- rendered app build/test
- final starter-kit catalog documentation

## Open Questions

- Should app-specific view packages live under `internal/views/<app>` or
  `app/<app>/views` for named apps?
- Should htmx be copied as a static asset by default or installed through npm
  because Tailwind already requires npm?
- Should CSRF be a generic `web` primitive, a GoForj generated helper, or part
  of a future security component?
- How much of the component set should be copied from a library such as templUI
  once that ecosystem stabilizes?
- Should `forj make:controller` learn server-rendered page output, or should
  template/page maker commands remain separate?

## Decisions

### Use `templ + htmx`

`templ + htmx` is the official server-rendered starter-kit direction.

### Do Not Build GoForj Blade

GoForj should not own a custom template language. It should own integration and
ergonomics around a Go-native template system.

### Keep The Kit Optional

The kit requires `WebUI` and should not affect API-only apps.

### Keep Views App-Owned

Generated templates, components, CSS, and small scripts belong to the app after
render and should be preserved on rerender.

### Keep Vue And React Separate

This is not a replacement for SPA starter kits. It is a distinct official path
for server-rendered apps.

### Match Vue And React Visually

The `templ + htmx` kit should look like the same GoForj starter app as Vue and
React. Stack choice should change implementation mechanics, not the user's first
impression.

### Keep htmx Thin

Use htmx to improve forms and partial updates. Do not build a LiveView-style
stateful runtime in the first server-rendered starter kit.

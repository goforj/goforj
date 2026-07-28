# React Starter Kit Design

## Purpose

This design defines the first GoForj React starter kit.

The React starter kit should be a visual and product parity implementation of
the existing Vue starter kit. Users should be able to choose `Vue` or `React`
and get the same application shape, layout, flows, and design quality.

The difference should be the frontend framework, not the product experience.

## Reference material

- Existing starter-kit architecture:
  `docs/designs/completed/starter-kits-design.md`
- Existing Vue starter kit:
  `templates/starter-kits/vue/frontend`
- shadcn/ui Vite installation:
  https://ui.shadcn.com/docs/installation/vite
- shadcn/ui `components.json`:
  https://ui.shadcn.com/docs/components-json

## Relationship to the Vue kit

The Vue starter kit is the reference implementation.

The React kit should match it in:

- app shell layout
- sidebar structure
- site header and breadcrumbs
- command menu behavior
- dashboard page
- auth pages
- settings pages
- component showcase pages
- theme behavior
- frontend env behavior
- app-local frontend location
- embedded `dist` behavior
- local development flow

The React kit should not invent a new visual language, alternate navigation
model, or separate page taxonomy.

When users compare the two starter kits in a browser, they should feel like the
same GoForj starter app implemented in two frontend frameworks.

## Technology stack

The React kit should use:

- React
- TypeScript
- Vite
- Tailwind CSS v4
- shadcn/ui
- lucide-react
- React Router
- Zod
- React Hook Form

The generated app should own the resulting frontend source and copied shadcn/ui
components. GoForj should not hide the UI behind a private package.

The initial package should mirror the Vue kit's tooling posture:

```text
cmd/<app>/frontend/
  package.json
  package-lock.json
  components.json
  vite.config.ts
  tsconfig.json
  index.html
  goforj.env.ts
  src/
  public/
```

## Design system

The React kit should use `shadcn/ui` with the same design settings as the Vue
kit wherever the ecosystems align.

Recommended `components.json` direction:

```json
{
  "$schema": "https://ui.shadcn.com/schema.json",
  "style": "new-york",
  "typescript": true,
  "tailwind": {
    "config": "",
    "css": "src/style.css",
    "baseColor": "neutral",
    "cssVariables": true,
    "prefix": ""
  },
  "iconLibrary": "lucide",
  "aliases": {
    "components": "@/components",
    "utils": "@/lib/utils",
    "ui": "@/components/ui",
    "lib": "@/lib",
    "hooks": "@/hooks"
  }
}
```

The React kit should use source-owned shadcn/ui components under:

```text
src/components/ui/
```

The component set should be the React equivalent of the Vue kit's visible UI
surface. If a Vue shadcn component does not have a direct React equivalent, the
React implementation should choose the closest shadcn/ui primitive and preserve
the same visual behavior.

## Visual parity contract

The React starter kit should match the Vue kit's first-screen structure:

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

  Command menu
  Toasts
```

The React kit should keep the same route map:

```text
/
/components
/components/overview
/components/forms
/components/navigation
/components/overlays
/components/data
/login
/forgot-password
/register
/reset-password
/verify-email
/settings
/settings/profile
/settings/password
/settings/appearance
```

If the Vue kit has additional routes when this work starts, the React kit should
copy the current Vue route set rather than this design's static list.

## File layout

Recommended React source layout:

```text
src/
  App.tsx
  main.tsx
  router.tsx
  style.css

  assets/
    goforj-logo.png

  components/
    AppSidebar.tsx
    CommandMenu.tsx
    NavDocuments.tsx
    NavMain.tsx
    NavSecondary.tsx
    NavUser.tsx
    SiteHeader.tsx
    TeamSwitcher.tsx
    ui/

  hooks/
    use-mobile.ts

  lib/
    auth.ts
    navigation.ts
    password-policy.ts
    theme.ts
    utils.ts

  views/
    DashboardView.tsx
    ForgotPasswordView.tsx
    LoginView.tsx
    RegisterView.tsx
    ResetPasswordView.tsx
    VerifyEmailView.tsx
    components/
      ComponentsDataView.tsx
      ComponentsFormsView.tsx
      ComponentsNavigationView.tsx
      ComponentsOverlaysView.tsx
      ComponentsOverviewView.tsx
    settings/
      SettingsAppearanceView.tsx
      SettingsLayoutView.tsx
      SettingsPasswordView.tsx
      SettingsProfileView.tsx
```

Use `.tsx` for React components and `.ts` for framework helpers.

## Routing

The React kit should use React Router with route metadata equivalent to the Vue
kit.

The routing layer should support:

- page titles
- public-shell routes
- auth-required routes
- settings child routes
- scroll reset or browser restoration behavior
- document title updates

The auth guard should behave like the Vue kit:

- public routes render without loading the current user first
- protected routes call `loadCurrentUser`
- unauthenticated protected routes redirect to `/login`
- focus/visibility resume should revalidate the current user

## Auth experience

When auth is enabled, React should match the Vue kit's user-facing auth surface:

- sign in
- create account
- forgot password
- reset password
- verify email
- signed-in dashboard
- profile settings
- password settings
- appearance settings
- sign out from user menu and command menu

The frontend should remain a client of the GoForj auth/backend behavior. The
starter kit owns the frontend experience, not the auth primitive.

When auth is not enabled, the React kit should still render a coherent app shell
without broken auth navigation.

## API and environment behavior

React should reuse the same frontend env contract as Vue.

The Vite config should:

- set `envDir` to the project root
- resolve GoForj frontend env through `goforj.env.ts`
- expose frontend-safe defines
- proxy `/api` to the selected app HTTP runtime
- use the same app-local frontend path under `cmd/<app>/frontend`

The starter kit should support named apps through the same frontend env
resolution behavior:

```text
FRONTEND_BACKEND_URL
<APP>_FRONTEND_BACKEND_URL
```

The React kit should not introduce a separate frontend env model.

## Component parity

The React kit should include React equivalents for the Vue kit's app-owned
components:

```text
AppSidebar
CommandMenu
NavDocuments
NavMain
NavSecondary
NavUser
SiteHeader
TeamSwitcher
```

It should also include the shadcn/ui primitives required by the visible pages.
The initial React component set should be generated or copied into the app, not
imported from a GoForj-owned UI package.

Likely required shadcn/ui components:

- accordion
- avatar
- badge
- breadcrumb
- button
- card
- checkbox
- command
- dialog
- dropdown-menu
- field/form primitives
- input
- label
- pagination
- select
- separator
- sheet
- sidebar
- skeleton
- sonner
- switch
- tabs
- textarea
- toggle
- tooltip

The exact list should be derived from the current Vue starter kit when the React
kit is implemented.

## Renderer changes

GoForj should add `react` as a first-party starter kit identity.

Required framework changes:

- add `project.StarterKitReact`
- add React to the starter-kit catalog
- show React in the starter-kit wizard when `WebUI` is enabled
- accept `--starter-kit react` in `make:app`
- persist `starter_kit: react` in `.goforj.yml`
- render `templates/starter-kits/react/frontend`
- scaffold React for named apps under `cmd/<app>/frontend`
- keep frontend build output embedded from `cmd/<app>/frontend/dist`
- include React in app-specific starter-kit selection

The renderer should generalize the existing Vue-specific functions instead of
copying special cases indefinitely.

Suggested direction:

```text
scaffoldStarterKitForApp(app, starterKit, overwrite)
  starterKit == vue   -> copy starter-kits/vue/frontend
  starterKit == react -> copy starter-kits/react/frontend
```

This keeps Vue and React parallel and leaves room for future kits.

## Local development

The React kit should use the same developer workflow as Vue:

```bash
cd cmd/app/frontend
npm install
npm run dev
npm run build
```

For named apps:

```bash
cd cmd/admin/frontend
npm install
npm run dev
npm run build
```

`forj dev` should install frontend dependencies for React the same way it does
for Vue when a starter kit defines an npm workflow.

Render next steps should refer to the selected framework:

```text
Sign into the React app locally with admin / admin
```

for React auth starter projects, matching the Vue hint.

## Testing

The React kit should have the same verification bar as Vue.

Renderer tests:

- `forj new` can select React
- `make:app --starter-kit react` creates app-local frontend files
- named app React frontend renders under `cmd/<app>/frontend`
- API-only apps do not render React frontend scaffolds
- existing frontend files are overwritten for initial project render when
  expected
- existing named-app frontend files are not overwritten during app re-render
- `dist/index.html` placeholder exists when the frontend has not been built
- logo/static assets are copied like Vue

Build tests:

- `npm install`
- `npm run build`
- generated Go binary embeds `frontend/dist`

Parity tests:

- route list matches Vue starter kit route list
- primary app-owned component names match Vue equivalents
- dashboard/auth/settings/component-showcase pages exist
- `components.json` uses `new-york`, `neutral`, CSS variables, lucide icons
- Vite proxy and frontend env behavior match Vue

Visual tests:

- desktop screenshot matches Vue structure
- mobile screenshot matches Vue structure
- login/public shell matches Vue structure
- dashboard shell matches Vue structure
- settings shell matches Vue structure

The screenshots do not need to be pixel-identical. They should verify layout
parity: sidebar/header/content structure, spacing rhythm, public-shell behavior,
and responsive behavior.

## Documentation

User-facing docs should describe React and Vue as equivalent official starter
kit choices:

```text
Vue
React
None
```

React docs should explain:

- generated frontend structure
- shadcn/ui ownership
- Vite build/dev flow
- auth page behavior
- frontend env behavior
- named app frontend paths
- how React parity with Vue is maintained

Atlas should eventually include a `goforj-react-starter-kit` skill parallel to
the Vue skill.

## Implementation phases

### Phase 1: Renderer identity

- add `StarterKitReact`
- add catalog entry
- update wizard selection
- update `make:app --starter-kit`
- add renderer dispatch for `react`

### Phase 2: React scaffold

- create `templates/starter-kits/react/frontend`
- add package files
- add shadcn/ui config
- add Vite/Tailwind config
- port shared env helper
- port assets

### Phase 3: Visual parity port

- port app shell
- port navigation
- port command menu
- port dashboard
- port auth pages
- port settings pages
- port component showcase pages

### Phase 4: Verification

- add renderer tests
- add package/build tests
- add route/component parity tests
- add screenshot tests for desktop/mobile parity
- update docs and Atlas skill plan

## Decisions

### Match Vue visually

React should be visually identical to Vue at the starter-kit level. Framework
choice should not change the app's first impression.

### Use shadcn/ui

React should use shadcn/ui directly, with app-owned generated components.

### Keep Vite

React should use Vite like Vue. Do not introduce Next.js, Remix, or React Router
framework mode in the first React kit.

### Keep app-local frontend ownership

React frontend source and build output should live under `cmd/<app>/frontend`,
matching Vue and the multi-app frontend model.

### Do not create divergent app concepts

React should not introduce routes, layouts, auth semantics, or frontend env
behavior that Vue does not have unless the Vue kit is updated first.

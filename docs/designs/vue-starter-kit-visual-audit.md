# Vue Starter Kit — Visual & Design System Audit

**Scope:** `templates/starter-kits/vue/frontend/`
**Method:** source read of the full template + live inspection of the prod build at `localhost:3000` (light + dark, 1440px)
**Date:** 2026-08-07

---

## Status

**All four waves are done.** What follows is the original audit; corrections found while implementing are
marked inline. Three findings in it were wrong and are withdrawn — see *Corrections*.

Deliberately **not** done, with reasons:

- **Shadow, type, and spacing scales (#7).** shadcn-vue defines none of these either — only a radius scale.
  Adding them would diverge from the reference the kit follows. The `gap-5` outliers were normalised by hand
  instead.
- **Field-level errors in settings (#28).** Those two screens validate a whole form at once, so a form-level
  message is the honest shape. Field-level validation is already demonstrated on the forms page through the
  vendored form primitives and vee-validate.
- **A brand hue (#8).** The kit stays neutral, matching both shadcn-vue and the Laravel starter kit. Only the
  focus ring was corrected, since it was near-black in light mode and invisible against a dark fill.

### How the reference pages work now

Each example lives in `views/components/examples/` and is imported twice by the page that shows it: once as a
component to render, once with Vite's `?raw` suffix to list. One file is both the running example and the
listing, so they cannot drift. All 28 specimens carry their source.

`Specimen` is for a single component; `Showcase` with `ShowcaseRow` is for a composed flow that uses several.
Tile specimens across columns when every card in the group is one control, stack them when any card holds a
composition — otherwise short cards leave voids beneath them.

Also fixed, and **not** in the original audit because I did not find them until later:

- `ui/chart/` imported `@unovis/vue` and `ui/table/utils.ts` imported `@tanstack/vue-table`. Neither was in
  `package.json`. Both modules were unreachable from their barrels, so the build stayed green and the
  production-graph test could not see them. Both deleted.
- A generated project introduced itself as GoForj — sidebar, tab title, and all five auth screens. Vite
  already resolved the app name and discarded it; now exposed as `VITE_APP_NAME`.
- The base stylesheet applied a pill background to every `<code>`, so any code block drew a box per line.
- `TeamSwitcher` rendered `plan` as its label, used `name` only as alt text, and switched nothing.

### Corrections

Three findings below described shadcn's own design rather than a defect, and one fix diverged from both
reference kits. All are annotated where they appear.

- **No font is loaded** — withdrawn. shadcn-vue declares no `--font-*` either; the system stack is the design.
- **`--card` equals `--background` in light** — withdrawn. shadcn ships exactly this; cards separate by border.
- **Chart hues differ between themes** — withdrawn. shadcn does the same.
- **`text-white` on destructive button/badge** — I repointed these at `--destructive-foreground`, then reverted.
  shadcn removed that token, which is *why* both it and the Laravel kit use `text-white`.

---

## Verdict

The kit is *structurally* sound — it's a clean, current shadcn-vue vendor (Tailwind v4 CSS-first, no legacy config, 60+ primitives, correct `@theme inline` setup). The problem is that **nothing above the primitive layer was designed.** Tokens were hand-merged from two different shadcn generations and never reconciled; page composition was assembled from docs demos and never edited down.

Three things a developer notices in the first sixty seconds:

1. **Light mode has no surface hierarchy.** `--card` and `--background` are both `#fff`. Every card floats on a hairline. Dark mode *does* have elevation — so the surface model means different things in each theme.
2. **The app shell is misaligned.** Header is 49px, sidebar header is 64px. The logo hangs 15px below the topbar rule on every screen. Measured live.
3. **Placeholder artifacts ship.** A black `Product Hero Media` slab, three `github.com/shadcn.png` avatars, and shadcn checkout/System-Settings demo copy inside a card titled "Environment controls and staged rollout."

Everything below is verified — line numbers are real, contrast ratios are computed, rendered claims were checked in the browser.

---

## P0 — Broken. Fix regardless of design direction.

### 1. `hsl(var(--token))` is invalid; sidebar tooltip arrows render as nothing — ✅ FIXED
`style.css:168,176`

```css
border-right: 10px solid hsl(var(--border));   /* --border is already hsl(0 0% 92.8%) */
```

Expands to `hsl(hsl(0 0% 92.8%))` → declaration dropped. Since `style.css:156-157` sets `width:0;height:0`, **both arrow pseudo-elements are fully transparent.** Live code — applied at `SidebarMenuButton.vue:40`.

```css
/* :168 */ border-right: 10px solid var(--border);
/* :176 */ border-right: 8px solid var(--card);
```

> **Correction to this audit.** The draft said the inner arrow should be `--popover`. It shouldn't —
> `TooltipContent.vue:28` is `bg-card text-foreground border-border`, and `SidebarMenuButton.vue:41`
> passes `:show-arrow="false"` so the CSS pseudo-element *is* the arrow. `--card` is correct.

Same bug latent at `ui/sidebar/index.ts:45` (`shadow-[0_0_0_1px_hsl(var(--sidebar-border))]`) — also fixed.

**Verified live:** `::before` now computes to `lab(100 0 0 / 0.1)`, exactly matching the `--border` token;
`::after` computes to `lab(7.78 0 0)`, exactly matching the tooltip's background. The arrow renders.

### 2. `text-destructive-foreground` used on light surfaces → invisible text — ✅ FIXED
`MenubarItem.vue:30`, `ContextMenuItem.vue:32`, `CalendarCellTrigger.vue:30`, `RangeCalendarCellTrigger.vue:34`

`--destructive-foreground` is `hsl(0 0% 98%)` on a `bg-popover` of `hsl(0 0% 100%)` → **1.05:1 in light mode.** Destructive menu items are literally unreadable. Dark mode is fine, which is why it survived.

**Fix:** → `text-destructive` at all four sites. Reserve `--destructive-foreground` for text *on* a destructive fill — which is exactly where `button/index.ts:14` and `badge/index.ts:16` currently hardcode `text-white` instead. Both swapped to the token; no visual change (98% white vs 100% white), but they now follow the theme.

**Verified live, light mode:** destructive-on-popover went **1.04:1 → 3.76:1**.

> **Carried into Wave 2.** 3.76:1 clears WCAG 1.4.11 (3:1, non-text/large) but not 4.5:1 for body text.
> The remaining gap is `--destructive` itself — `hsl(0 84.2% 60.2%)` is too light for text on white.
> The ramp rework should land it near `oklch(0.577 0.245 27.325)` (~4.8:1). Same applies to
> `--destructive-foreground` on a destructive fill, measured at 3.61:1 — unchanged from the old
> `text-white`, so this is a pre-existing gap, not a regression.

### 3. No catch-all route — 404s render a blank app — ✅ FIXED
`router.ts:28-52` had no `/:pathMatch(.*)*`. Confirmed live: `localhost:3000/dashboard` rendered the full shell, breadcrumb reading "Dashboard", and an **entirely empty content area**.

Added `views/NotFoundView.vue` (built from the existing `Empty` primitive, `FileQuestion`/`House`/`ArrowLeft` — all icons already used elsewhere in the kit) and wired `/:pathMatch(.*)*` as the last route. It echoes the attempted path and offers "Back to dashboard" plus a history-aware "Go back". Sits behind the existing `beforeEach` auth guard, so unauthenticated bad URLs still route to login.

### 4. `--sidebar-primary` and `--sidebar-primary-foreground` are both `#fff` in dark — ✅ FIXED
`style.css:109-110` — `hsl(360 100% 100%)` and `hsl(0 0% 100%)`. Any `bg-sidebar-primary text-sidebar-primary-foreground` pairing is 1:1. Latent today; a trap for the first person who builds a sidebar CTA. Now mirrors `--primary`/`--primary-foreground` (`92.2%` / `20.5%`).

---

## P1 — Design system foundation

### 5. Light and dark are not the same theme

`style.css:47-79` is all `hsl()`. `style.css:83-101` switches to `oklch()`. Then `style.css:102-115` reverts to `hsl()` **inside the same `.dark` block.** Light greys are sRGB-uniform, dark greys are OKLab-uniform, so they can't be tuned together — and `bg-primary/90` interpolates differently per theme.

The tell: `--sidebar-background` (shadcn v0 naming) and `--sidebar` (current naming) are *both* defined, and in dark they disagree — `hsl(0 0% 7%)` vs `hsl(240 5.9% 10%)`. `@theme inline:35` maps to `--sidebar-background`, so `--sidebar` is dead code. Lucky, because it's the only blue-tinted value in a `"baseColor": "neutral"` palette.

**Fix:** convert everything to `oklch`, delete `--sidebar-background`, point `--color-sidebar` at `--sidebar`.

### 6. The light ramp is collapsed — measured contrast

| Pair | Values | Ratio |
|---|---|---|
| `--card` vs `--background` (`:49` / `:47`) | 100% / 100% | **1.000:1** |
| `--border` vs `--secondary` (`:63` / `:55`) | 92.8% / 92.1% | **1.018:1** |
| `--border` vs `--input` (`:63` / `:64`) | 92.8% / 89.8% | 1.076:1 |
| `--muted`/`--accent` vs bg | 96.1% / 100% | 1.090:1 |

Three distinct problems:

- **`--card` == `--background`.** Zero elevation in light mode. Verified live: the hero card on `/components/overview` has no perceptible boundary against the page. Dark mode *does* separate them (0.205 vs 0.145) — so surfaces mean different things per theme.
- **`--border` (92.8%) is lighter than `--input` (89.8%)** — inverted vs upstream. A form inside a card has heavier hairlines than the card.
- **`--border` vs `--secondary` at 1.018:1** — a bordered secondary element has a completely invisible border.

All three border/input values are far below WCAG 1.4.11's 3:1 minimum for non-text contrast.

```css
/* :root */
--card:      oklch(0.99 0 0);   /* a real half-step above white */
--secondary: oklch(0.97 0 0);
--muted:     oklch(0.97 0 0);
--accent:    oklch(0.97 0 0);
--border:    oklch(0.90 0 0);
--input:     oklch(0.90 0 0);
```

Restores monotonic ordering: background 1.0 → card 0.99 → muted/secondary 0.97 → border/input 0.90.

### 7. No scales exist — only colors and radii

`@theme inline` (`style.css:6-43`) defines `--radius-*` and `--color-*`. **Zero** `--shadow-*`, `--text-*`, `--font-*`, `--tracking-*`, `--spacing`.

Consequences, measured:
- **8 shadow steps in play** with no rule: `shadow-xs` ×16, `shadow-md` ×10, `shadow-sm` ×8, `shadow-lg` ×8, `shadow-none` ×6, `shadow-xl` ×3, `shadow-2xl` ×1. `DashboardView.vue:3` uses `shadow-xl` while the Card primitive is `shadow-sm`.
- **No font is loaded anywhere.** No `@font-face`, nothing in `index.html`, no `--font-sans`. Confirmed live — computed `font-family` is Tailwind's stock `ui-sans-serif, system-ui, …`. The kit renders as SF Pro / Segoe UI Variable / Roboto depending on OS: three different typographic personalities, none of which the tracking was tuned for. `ChartContainer.vue:53` already references `var(--font-sans)` expecting it to exist.
- **4 distinct tracking values** for the same micro-label treatment: `0.16em` (`NavSecondary.vue:12,22,32`), `0.2em` (auth views), `0.24em` (`SiteHeader.vue:19`), `0.28em` (`LoginSplitView.vue:194,235,282`).
- **`focus-visible:ring-[3px]` repeated 15×** as an arbitrary value.

Add `--shadow-{xs..xl}`, `--text-{xs..3xl}` with paired line-height/letter-spacing, `--font-sans`/`--font-mono`, `--tracking-label`, `--ring-width`. Then a surface rule: page → none, card → `sm`, dropdown/popover/tooltip → `md`, dialog/sheet → `lg`.

### 8. Focus rings are inconsistent, and one region is a different color

Five treatments across `src/`: `ring-[3px]` (15 components, the house style), `ring-2` (5), `ring-4` (2), `ring-1`/`ring-0`/`ring-offset-1` (5). Worse — **`focus:` instead of `focus-visible:`** at `DialogContent.vue:46`, `SheetContent.vue:55`, `PinInputSlot.vue:19`, so those fire on mouse click.

And `--ring` is achromatic while `--sidebar-ring` is `hsl(217.2 91.2% 59.8%)` (bright blue) in *both* themes. Keyboard focus is a grey smudge in main content and a blue halo in the sidebar. In light mode `--ring` is near-black at 50% — invisible against `bg-primary`.

**Fix:** one ring width token; normalize `focus:` → `focus-visible:`; delete `--sidebar-ring`, point at `--ring`; make `--ring` a real accent (`oklch(0.708 0 0)`, or the brand hue the kit doesn't have yet).

### 9. No `--success` / `--warning` tokens
Only `--destructive` exists, so four auth views hardcode `text-green-600` (`VerifyEmailView.vue:18`, `ResetPasswordView.vue:56`, `RegisterView.vue:72`, `ForgotPasswordView.vue:34`). Against `oklch(0.145)` in dark that's ~3.1:1 — fails AA for body text. Success is also styled as bare centered text while errors get a bordered tinted box: two severity treatments for one system.

### 10. Chart tokens change hue identity between themes — and are entirely unused
Light (`:66-70`) is warm: `12° 173° 197° 43° 27°`. Dark (`:102-106`) is unrelated and cool: `220° 160° 30° 280° 340°`. Series 1 goes orange-red → blue on theme toggle. And `grep chart-[1-5]` outside `style.css` returns **zero hits** — 10 dead tokens, alongside a fully-built `ui/chart/` that no view imports.

### 11. Four gradient classes, three formulas, and two that do nothing
`.main-surface` (`:137`) and `.main-surface-login` (`:179`) mix `--background` with `--card` — which in light mode are the same color, so both are **white→white, zero effect**. `.main-content-area` (`:188`) fades a 16%-alpha background over the background, nested *inside* `.main-surface` at `App.vue:35`. `.dark .main-surface-login` (`:184`) abandons tokens entirely for hardcoded `hsl(0 0% 3.9%)` → `hsl(0 0% 5.5%)`, neither of which matches `--background`.

Separately, `App.vue:8` and `App.vue:35` apply `app-shell-login` and `login-content-area` — **neither class is defined anywhere.**

**Fix:** delete all four; one `--surface-app` token. Get elevation from `--card` once #6 lands.

---

## P2 — Composition and shell

### 12. Header/sidebar misalignment — measured live: 49px vs 64px — ✅ FIXED
`App.vue:6` sets `--header-height: calc(var(--spacing) * 12)` = 48px. The sidebar header is `SidebarHeader.vue:14` `p-2` (16) + `TeamSwitcher.vue:27` `!py-1` (8) + `:34` logo `h-10` (40) = 64px. **The logo hangs 15px below the topbar rule on every screen.** Dead `SiteHeader.vue:15` uses a third value, `h-14`.

Took `--header-height: calc(var(--spacing) * 16)` — matching the sidebar rather than shrinking the logo, since the 64px sidebar header is the more standard app-shell proportion. **Verified live:** header 64px + 1px border vs sidebar header 64px; the logo now sits on the topbar rule.

### 13. Placeholder artifacts that ship to users

| Where | What |
|---|---|
| `ComponentsOverviewView.vue:84-88` | 16:9 box, hardcoded dark gradient, label **"Product Hero Media"**. Not dark-gated — renders as a **solid black slab in light mode.** Verified. |
| `ComponentsOverviewView.vue:154,158,162` | `github.com/shadcn.png`, `maxleiter.png`, `evilrabbit.png`. Ships the shadcn maintainers' faces, 404s offline, leaks a github.com request on first paint. |
| `ComponentsFormsView.vue:694-754` | Card titled "Environment controls and staged rollout" containing: a `<FieldSeparator>Appearance Settings</FieldSeparator>` for a section that doesn't exist; the shadcn "Kubernetes / GPU workloads" demo; **"Wallpaper Tinting / Allow the wallpaper to be tinted"** (macOS System Settings copy); and "How did you hear about us?". Four unrelated subjects in one card. |
| `ComponentsFormsView.vue:27-46` | "Payment Method / John Doe / 1234 5678 9012 3456 / CVV 123" — shadcn checkout demo verbatim. `:483` `placeholder="@shadcn"`. |
| `ComponentsFormsView.vue:910` | Textarea default value is meta copy *about the template*, inside a product settings form. |
| `ComponentsDataView.vue:341` | Sample row `'Lighthouse WS request' / 'Unauthorized'` — a goforj dev-tooling debug artifact as demo data. |

### 14. `border-dashed` used as permanent decoration
`ComponentsOverviewView.vue:20` — the entire docs-link callout Card. Dashed borders conventionally mean *empty / drop here / unfinished*; this is a permanent element and reads as a TODO. (`:149`,`:180` are redundant — `Empty.vue:14` already sets it.)

### 15. Six copy-pasted page headers, zero shared component
No `PageHeader` exists. Two variants:
- **Hero** (~95% identical): `DashboardView.vue:3-18`, `ComponentsOverviewView.vue:3-18`
- **Eyebrow+title+description** (4 byte-identical copies): `Components{Forms,Navigation,Overlays,Data}View.vue:3-14`

`CardTitle` consequently renders at **five different treatments** — `text-3xl md:text-5xl tracking-tight` (heroes), bare `text-3xl` (sub-pages, no tracking, no responsive bump), `text-xl`, `text-lg`, and unsized. Settings (`SettingsLayoutView.vue`) invents a *sixth*, non-Card pattern. Sub-page titles are nominally the same level as the hero but visually a different component.

Measured live on `/components/overview`: 7 distinct font sizes, with a jump from **20px straight to 48px** — nothing occupies the middle. That's landing-page proportion in an app shell.

**Fix:** one `<PageHeader eyebrow title description :hero>`. Six call sites collapse to six one-liners and the drift disappears by construction.

### 16. Card-in-card at matching radius, ~30 instances
`Card.vue:15` is `rounded-xl border shadow-sm`. Pages then use `rounded-xl border p-4` as an inline card clone — child radius exactly matches parent, both `bg-card`/transparent, only a hairline between them. Classic muddy-surface signature.

Sites: `ComponentsFormsView.vue:132,156,207,269,318,420,492,563,610,651,707,714,777,807,828`; `ComponentsDataView.vue:59,91,115,145,213,241,246,251`; `ComponentsOverlaysView.vue:97,121,156,185,193`; `ComponentsNavigationView.vue:23,45,144,159,211`.

Three-level nesting: `ComponentsFormsView.vue:807`→`828`; `ComponentsNavigationView.vue:176`→`205` (a real `<Card>` inside a Carousel) →`211`.

**Fix:** nested surfaces get `rounded-lg bg-muted/40 border-0` — radius one step down, tone instead of stroke. Never nest `<Card>` in `<Card>`.

### 17. Spacing has no rhythm
- **`gap-5` (20px) appears 13×**, sitting between the `gap-4`/`gap-6` used everywhere else, so nothing aligns across card boundaries. Sibling cards in the same grid row disagree: `ComponentsOverlaysView.vue:22` `gap-4` vs `:184` `gap-5`. `ComponentsDataView.vue` uses `gap-6`/`gap-5`/`gap-3`/`gap-3` in one file.
- **Header gutter ≠ content gutter above `lg`.** `App.vue:18` is `px-4 lg:px-6`; `App.vue:35` is `p-4` with no bump. The breadcrumb sits 24px from the edge while the first card sits at 16px — an 8px misalignment on every page.
- **Double hero padding.** `Card.vue:15` ships `py-6`; `DashboardView.vue:4` and `ComponentsOverviewView.vue:4` add `p-6 md:p-8` on top → 48px effective, 56px at `md`. No other card does this.
- `py-10` at `ComponentsOverviewView.vue:149,180` is silently discarded above `md` (`Empty.vue:14`'s `md:p-12` wins).
- Off-scale one-offs: `min-h-[430px]`, `h-[350px]`, `w-[11.5rem]`, `max-w-[54rem]` (864px, applied to a Calendar in a `0.85fr` column — a dead constraint from an older layout), `!pl-1` ×3.

### 18. No shared container; seven ad-hoc max-widths
`App.vue:35` sets no max-width, so pages stretch edge-to-edge on a 2560px monitor. Each page then invents its own: `max-w-3xl`, `max-w-2xl`, `max-w-xl`, `max-w-xs`, `max-w-md`, `max-w-[54rem]`. And `max-w-3xl` means the *heading block* on heroes but the *description* on sub-pages.

### 19. Hardcoded colors bypassing tokens
`dark:text-slate-300` (`DashboardView.vue:13`, `ComponentsOverviewView.vue:13`) — `slate` appears nowhere else, a straight palette leak. Plus `dark:text-white`, `dark:border-white/15`, `dark:bg-white/10`, and raw `hsl(0_0%_5%)` gradients across `DashboardView.vue:3,6,7` and `ComponentsOverviewView.vue:3,10,85`. All eight exist *because* the hero paints its own background instead of using a token, then hand-patches foregrounds to survive it.

In primitives: `button/index.ts:14` and `badge/index.ts:16` `text-white`; `Slider.vue:40` `bg-white` (a pure-white puck on an `oklch(0.145)` track); `Sonner.vue:53-59` duplicates `--popover-foreground` as a literal.

Off-scale opacities: `bg-primary/8` vs `/10` for the same selected state, with border alphas at `/20`, `/25`, `/30` — three strokes for one affordance.

### 20. ~400 lines of dead, off-brand code — ✅ FIXED (both deleted, 399 lines)
- **`LoginSplitView.vue` (364 lines) is not routed.** `router.ts:10` imports only `LoginView`; zero references anywhere. It's also visibly a different product: logo `h-12` centered vs `3.5rem` left-aligned with a hardcoded `drop-shadow`; heading `text-xl font-medium` vs `clamp(1.85rem,2.5vw,2.2rem)/700/-0.04em`; `max-w-sm` vs `min(100%,28rem)`; its own card radius and shadow; a `960px` breakpoint matching no Tailwind step; ~13 sites of raw `rgba()`/`hsl()`/hardcoded slate bypassing every token; and a prefilled `ref('admin')` credential.
- **`SiteHeader.vue` has zero imports.** `App.vue:13-33` defines its own inline header. The dead file also renders a "Sign in" button in authed app chrome.

### 21. Auth screens: 5× copy-paste with measurable drift
`LoginView`, `RegisterView`, `ForgotPasswordView`, `ResetPasswordView`, `VerifyEmailView` lines 1-15 are byte-identical. The footer link block, the bordered error `<p>`, and the show/hide password button are each duplicated 4-5×.

Where they diverge: **submit button top margin is `mt-4` / `mt-4` / `mt-2` / none / none** — flipping Login → Register shifts the CTA 8px. `VerifyEmailView.vue:17` is missing the `<form>` wrapper the other four have, so its CTA sits at a different offset *and* Enter-to-submit doesn't work. `RegisterView.vue:38-45` password has `pr-16` + a Show button; `:61-67` confirm has neither — two stacked inputs with different inner padding.

**Fix:** `AuthLayout.vue` + `PasswordInput.vue` + `AuthError.vue`. ~120 lines collapse to four components and the drift becomes impossible.

### 22. Sidebar affordances
- **Two of three nav groups vanish when collapsed.** `NavDocuments.vue:2` and `NavSecondary.vue:2` are `v-if="isMobile || state === 'expanded'"` — Resources and Shortcuts disappear rather than collapsing to icons, unlike `NavMain`. This also nullifies the `mt-auto` at `AppSidebar.vue:58`, and makes the `:tooltip` props at `NavDocuments.vue:6` / `NavSecondary.vue:6` dead code (they can only fire when the group is hidden).
- **No disclosure affordance when expanded.** `NavMain.vue:27` renders `SidebarMenuSub` only when the route is already active. Verified live: on `/`, "Components" shows no chevron and no children. Collapsed mode gets a dropdown; expanded mode gets nothing.
- **Breadcrumb is hardcoded.** `App.vue:24` is a literal `<BreadcrumbLink href="#">Application</BreadcrumbLink>` — styled clickable, goes nowhere. `:28` renders `route.meta.title`, so `/components/forms` reads "Components Forms" instead of `Components / Forms`. Meanwhile `lib/navigation.ts:30-37` exports `findAppNavItem` which could build a real trail and is **used nowhere.**
- **No theme toggle in the header.** `App.vue:32` right side is empty; theme switching is buried in `/settings/appearance`.
- `AppSidebar.vue:32-33,41` hardcode `github.com/goforj/goforj`, `goforj.dev`, and the team name "GoForj Starter Kit" — a developer's own app permanently links to the template author's repo.
- `AppSidebar.vue:37` labels the shortcut `Ctrl + K` while `App.vue:184` accepts `metaKey` — Mac users are told the wrong key.
- `TeamSwitcher.vue:19` `activeTeam = props.teams[0]`, no dropdown, no selection state. Renders `plan` as the visible label and uses `name` only as alt text. The name promises an affordance that isn't there.
- `App.vue:85-86` falls back to `name: 'Signed out'` / `email: 'No active session'` — rendered with full avatar chrome and initials "SI", so it looks like a user named "Signed out".

### 23. Settings is the least-finished surface
Verified live: `/settings/appearance` is a heading, a three-item segmented control, and ~500px of empty space. Specifically:
- `SettingsAppearanceView.vue:8-20` is a hand-rolled segmented control — no `<form>`, no save affordance, no toast (Profile and Password both toast), no error path, no `role="radiogroup"`. The `<button>`s carry only `transition-colors` — **the one place in the app with no focus ring.** Use the already-vendored `ui/toggle-group`.
- Two error patterns coexist: bare `<p class="text-sm text-destructive">` in settings vs the bordered box in auth. Neither is field-level — `SettingsProfileView.vue:66`'s "Name and email are required" appears at the form bottom, not on the input.
- `SettingsLayoutView.vue:17` active state is `bg-muted text-foreground`; the sidebar's is `bg-sidebar-accent text-sidebar-accent-foreground`. Two different "selected nav item" treatments one click apart.
- `SettingsProfileView.vue:56-61` awaits `loadCurrentUser()` in `onMounted` with no skeleton — inputs render empty then populate.
- Settings buttons show a label swap with no spinner; all five auth screens show spinner + label swap.

### 24. `DashboardView` is a marketing page, not a dashboard
The post-login landing route renders a hero + three feature cards + three "next steps" boxes. No metrics, no table, no chart — while `ui/chart/` ships fully built and imported by zero views. The first screen a user sees is the weakest visual argument the kit makes.

**Fix:** move the hero to `/welcome`; give `/` a stat row, one `ChartContainer`, and the resource table already written at `ComponentsDataView.vue:67-88`.

---

## Suggested sequence

**Wave 1 — correctness. ✅ Done.** #1 tooltip arrows, #2 invisible destructive text, #3 catch-all route, #4 white-on-white sidebar tokens, #12 header height, #20 deleted `LoginSplitView.vue` + `SiteHeader.vue`.

Files touched: `style.css`, `router.ts`, `App.vue`, `views/NotFoundView.vue` (new), `ui/sidebar/index.ts`, `ui/button/index.ts`, `ui/badge/index.ts`, `ui/menubar/MenubarItem.vue`, `ui/context-menu/ContextMenuItem.vue`, `ui/calendar/CalendarCellTrigger.vue`, `ui/range-calendar/RangeCalendarCellTrigger.vue`.

Verified against the running dev server: arrow pseudo-elements resolve to their tokens, header/sidebar align at 64px, `/dashboard` renders the 404, no console errors. `templates/embed.go` uses `//go:embed all:*` so the deletions need no manifest update. **Not run here** — `vite build`, `vue-tsc`, and the Go template tests; the sandbox npm registry is gated and Go isn't installed.

**Wave 2 — token foundation (1-2 days).** Convert to a single oklch ramp (#5), fix the collapsed light scale (#6), add shadow/type/font/tracking/ring scales (#7), normalize focus rings (#8), add `--success`/`--warning` (#9), unify chart hues (#10), collapse the four gradient classes to one `--surface-app` (#11). Do this before touching pages — most P2 hardcoded-color findings dissolve once the tokens are right.

**Wave 3 — composition (2-3 days).** Extract `PageHeader` (#15) and `AuthLayout`/`PasswordInput` (#21) — these two alone remove the majority of the drift. Then de-nest cards (#16), normalize spacing to a 4/6 scale and add a shared container (#17, #18), strip the placeholder artifacts (#13, #14), replace the marketing dashboard (#24).

**Wave 4 — shell polish (1 day).** Sidebar collapse behavior and disclosure chevrons, real breadcrumbs via the already-written `findAppNavItem`, a header theme toggle, settings parity (#22, #23).

Waves 1 and 2 are where the visible quality jump is. Wave 2 is also the one that has to happen first — fixing pages before tokens means fixing them twice.

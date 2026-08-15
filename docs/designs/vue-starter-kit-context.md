# Vue starter kit — working context

Handoff notes for `refactor/frontend-starter-kit-cleanup`. The audit and its
findings live in `vue-starter-kit-visual-audit.md`; this file is the operational
knowledge that is easy to lose and expensive to rediscover.

## The dev loop, which is the first thing to know

Templates are compiled into the binary. `templates/embed.go` uses
`//go:embed all:*` and `internal/forj/project_renderer.go` renders from that
embedded copy.

**Editing anything under `templates/` has no effect until `go install ./cmd/forj`.**

A dev server watching a rendered project re-renders faithfully — from the
snapshot the binary was built with. Stale output is indistinguishable from a
broken watcher, and diagnosing it cost most of a session. Symptoms: a change you
just made does not appear, repeatedly, while earlier changes did.

Set `GOFORJ_TEMPLATES_DIR=<repo>/templates` to render from disk instead. Paths in
the renderer are relative to `templates/`, so it must point at that directory.
See `docs/maintainer/starter-kit-development.md`.

**Never run a render inside the goforj repo itself.** It generates an app over
the top of goforj, deleting `wire/*.go`, `project/config.go` and
`internal/cmd/root_cmd.go`, and injecting the generated app's dependencies into
goforj's own `go.mod` without matching `go.sum` entries. The symptom is
`go install` failing with "missing go.sum entry".

## What the reference pages are

Five routes under `/components`. Every card shows the source that built it.

Each example lives in `views/components/examples/` and is imported **twice** by
its page:

```ts
import DialogExample from './examples/DialogExample.vue'
import DialogExampleSource from './examples/DialogExample.vue?raw'
```

One file is both the running example and the listing, so they cannot drift. This
is the reason hand-written snippets were rejected. Adding an example means adding
both imports — the page compiles with only the first and silently shows no code.

`?raw` needs `"types": ["vite/client"]` in `tsconfig.json`.

### Which primitive to use

- **`Specimen`** — one component. Name is the heading, import path lives in the
  source panel, `data-import` on the section keeps the mapping machine-readable.
- **`Showcase`** + **`ShowcaseRow`** — a composed flow using several components.
  Takes `source` too; a composition is the most useful thing on the page to copy.
- **`ComponentTag`** — inline component name beside a row label.

### Layout rules learned the hard way

- **A grid of specimens must never stretch.** Specimen content is fixed height,
  so the extra space always lands inside a visible border and reads as a card
  that failed to load. Use `items-start`. This mistake was made on three separate
  pages before the rule was written down.
- Tile across columns when every card in the group is a single control. Stack
  when any card holds a composition — otherwise short cards leave voids.
- Overlays is 3-up, Forms is 2-up, Navigation and Overview are mixed, Data is
  single column except the calendar pair.

## Traps in the vendored `ui/` tree

`ui/` is a shadcn-vue vendor and should stay re-syncable with the CLI. Do not
edit it casually. Things found in it:

- **`code:not(pre code)`** — the base stylesheet applies a pill background to
  every `<code>`. Unscoped, every line of a code block draws its own box.
- **`Empty` sets `border-dashed` with no border width**, so it cannot draw its
  dashed edge alone. Call sites must pass `border`.
- **`PinInputSlot` and `InputOTPSlot` are styled to sit flush** — end radii and
  the left border come from `first:`/`last:`. A `gap` between them strips the
  middle slots of both. `InputOTP` uses two groups with a separator, which is
  the pattern that *is* meant to be spaced.
- **`Field orientation="responsive"`** flips to a row at `@md/field-group`
  (448px container). Inside a narrow column it goes horizontal with no room and
  crushes descriptions to one word per line.
- **`FieldSeparator` is `h-5` with `-my-2`**, tuned for `FieldGroup`'s `gap-7`.
  In a plain `gap-6` column it adds ~50px of air.
- **`reka-ui`'s `AspectRatio` does not forward `class`** to the element it
  sizes. Constrain it from a wrapper.

## Theme

Every token is `oklch`, both modes. The light block was Laravel's `hsl` and the
dark block was a later shadcn `oklch` release; they are now one colour space.
The conversion was colour-preserving — verified pixel-exact on sampled tokens.

`--sidebar-background` and `--sidebar` both existed and disagreed in dark mode.
Only the first was mapped, so the second was dead *and* wrong. Now one token.

Added `--success` (auth screens used a literal `text-green-600` with no dark
variant) and five `--code-*` tokens for the syntax highlighter.

`--destructive` is still light for body text — 3.76:1 on white. It matches both
shadcn and the Laravel kit, so it was left alone deliberately.

## Reference kits

The theme derives from the Laravel starter kit at `~/code/laravel-test/my-app`.
Its `:root` was copied verbatim. Check it *and* shadcn-vue before calling
something a defect — three audit findings turned out to be describing shadcn's
own design:

- `--card` equals `--background` in light. Intentional; cards separate by border.
- Chart hues differ between themes. shadcn does this too.
- No font is loaded. shadcn declares no `--font-*` either.

Laravel keeps appearance in settings only, with no header toggle. A header theme
toggle was added and then reverted for this reason.

## What the build cannot catch

`vue_starter_build.test.mjs` asserts the five component views are in the
production module graph. It structurally cannot see a vendored module that
nothing imports — which is how `ui/chart` (importing `@unovis/vue`) and
`ui/table/utils.ts` (importing `@tanstack/vue-table`) both shipped with
undeclared dependencies. Both were deleted.

A test that walks every import and fails on a bare specifier absent from
`package.json` would have caught both. Not written yet.

## Checks worth re-running after edits

Not automated; run ad hoc from `src/`:

1. Template tag balance, anchored on the top-level `<template>` block. Naive
   parsing over the whole file reads TypeScript generics like
   `defineProps<SidebarProps>` as HTML.
2. Component tags used in a template with no matching import. An unused-import
   check cannot find these — it only looks at what *is* imported.
3. Every relative and `@/` import resolves, including `?raw`.
4. Bare specifiers all present in `package.json`.

## Open

- `items-start` on the Forms grid was committed but had not appeared in a render
  as of the last check.
- The Go change allowing `GOFORJ_TEMPLATES_DIR` is **not compile-verified** —
  there was no Go toolchain available. Syntax and imports were checked statically.
- `ThemeLogo` and the light/dark tile assets are recent work not covered here.

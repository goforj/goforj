# Documentation and branding

Read this when building or reviewing a README, documentation generator, example index, logo, feature table, or driver matrix.

## Source ownership

GoDoc owns API descriptions. Canonical runnable programs own demonstrated workflows and expected output. Metadata or marked regions may associate APIs with snippets, but should not create a second handwritten implementation. Templates and generators own their checked-in projections.

Change the authoritative source, regenerate every affected projection, and verify that generation again leaves no diff. Keep generators deterministic. Fail generation for stale API names, absent examples, missing snippet boundaries, unresolved targets, and incomplete evidence where those surfaces are generated.

The docs site's library pages are projections of sibling READMEs. If site registration or transformation is in scope, inspect the current documentation backend and its repository instructions. Do not fix a generated library page in isolation or add a parallel hand-maintained copy.

## README reading order

Use the closest sibling's established structure while applying these maintainer preferences:

1. A centered library logo, one-sentence benefit, and compact status badges.
2. Centered section navigation when provided.
3. A concise feature table explaining concrete outcomes.
4. A visible quick start that compiles and runs with stated prerequisites.
5. Driver choices and installation details appropriate to the library.
6. Runnable example commands, a focused API index, and inline API examples.
7. Concise development guidance linking to deeper contracts and operational notes.

Adapt ordering for the library's actual setup needs. Avoid a contract, disclaimer, or architecture wall before showing useful code. Do not collapse the primary quick start by default. Comments should explain the steps and relevant reasons, such as ownership, explicit provisioning, or waiting for visibility, rather than narrating individual statements. Keep error handling honest and expected output beside the call that produces it.

Use a plausible application model rather than a token one-field struct when richer data helps demonstrate the library. For example, a catalog item can include a title, description, category, integer price in cents, and availability. Keep its schema, sample values, query, and output consistent. Prefer a package-scope exported model such as `Product` when illustrating a reusable application type, with a name-leading doc comment; capitalizing a function-local type does not make it a package API. Keep these example types out of the library's public API. Adapt the model to the domain rather than copying search's fields everywhere.

Write runnable-example instructions for readers who want to try the library. Lead with where to run the commands and the required Go version. Prefer a compact fenced `sh` block with brief command comments. Separate offline and credentialed commands when their prerequisites differ. Avoid a heading and block per trivial command, long command tables, and repeated provider setup prose. Inline-code table cells do not provide the code-block copy affordance. A short sentence can say that tests verify output; runner details and provider authentication belong in linked documentation.

For unpublished libraries, provide a clone-and-run quick start that works for a new GitHub reader, without maintainer-specific paths such as `~/code/<library>`. An application installation command may pin a real, publicly verified pseudo-version; do not invent a tag or require local replacements to conceal invalid published requirements. State the unreleased status and minimum Go requirements accurately. Keep the primary example self-contained or link its complete executable program near the snippet so imports are easy to find.

Explain which driver packages are included in the root module. Give each independently installed optional module its own labeled, copyable command; do not make readers substitute names into another driver's command. Once releases are published, use `@latest` for README installation commands and verify it selects the intended release. Keep explicit versions in release notes and dependency manifests, and keep the badge showing the actual released version. Keep installation focused on setup rather than repeating partial code examples. Keep framework inventory provenance and historical project names in development or research notes, not the library's public pitch.

## Useful API navigation

Curate the index around operations application developers use. Do not reproduce every exported DTO, implementation method, constant, or plumbing interface. Keep qualified names only when the package or receiver is useful for understanding or disambiguation; avoid repeating the root package prefix everywhere.

Use a two-column category/functions table, preserving the target's established label: search and storage use **Group | Functions**, while notification uses **Category | Functions**. Use bold, right-aligned categories and left-aligned functions, with related linked operations in one row separated by ` · `. Preserve a deliberate category and function order. Each function keeps its own destination even when several share a category. A row labeled Driver constructors should enumerate the supported driver catalog rather than a handpicked subset. Derive its entries and configuration examples from the canonical executable programs, including constructor helpers those programs exercise, and fail generation when a supported driver lacks a constructor or matching destination. Give each constructor a matching heading when the full catalog needs direct navigation. Do not substitute a Task/API/Example table or add a repeated example-title column.

API names link to corresponding example headings inside the README. Each linked snippet must demonstrate that API; a valid anchor pointing to unrelated code is still wrong. The Go generator should verify symbol existence, a demonstrated call in the canonical snippet, and that the snippet is inside the target heading's section. Heading existence and a matching call elsewhere on the page are insufficient. Respect subsection boundaries and ignore heading-like text inside fenced code. Give separate provider examples their own headings when a shared heading makes navigation ambiguous. Use stable anchors so wording improvements do not break index navigation.

The destination structure must match the index too: category sections followed by API-named headings, not a flat list of task titles. APIs demonstrated together may share a heading such as `Definition · NewIndex` and one example block, with a unique anchor for each API. Descriptive workflow titles can appear as explanatory prose below those headings. Generate any repeated projections from the same canonical program so they cannot drift. Preserve existing example anchors used elsewhere when changing the presentation.

Keep example headings as plain text, without inline-code styling. Format example configuration structs and nested values across readable lines rather than compressing them into one-line literals. Do not style every API name as inline code within the index or wrap links in decorative tilde or backtick markup. Link complete programs from a compact source index or an appropriate source link; a command table is not required. Avoid repeated `Full program` and `Executable test` destinations after every snippet. The program is the source; the shared runner tests it.

Show example statements directly, as in cache's API examples. Put snippet markers inside helper functions so `func run(...) error`, package/import declarations, argument parsing, and fixture setup stay outside README blocks unless they are the example's subject. State necessary inputs and link the complete program. When only an error is needed, use `if err := store.Set(ctx, name, value); err != nil { ... }`. A discarded value can use `if _, err := reader.Get(ctx, name); err != nil { ... }`. Keep assignments separate when a returned value or error is needed afterward. Do not add nesting, discard needed values, or hide errors behind helpers solely to shorten a snippet. Reduce repeated setup instead.

When contracts need their own section, prefer concise `How it works` bullets with bold labels before a long example catalog. Keep deeper limits in linked documentation. A related pair of reads can illustrate reuse without implying batching or an atomic snapshot.

Use a real feature matrix and, when drivers exist, a driver matrix based on tested behavior. Label native limits and differences accurately instead of using an unexplained universal checkmark. Use the current feature and driver treatment: a left-aligned fixed-size icon beside a searchable text label, with consistent cell padding and vertical alignment. The API category column uses its separate right-aligned treatment.

Allow feature and driver labels to wrap on phones. Wide fixed-width, nonwrapping name cells can squeeze descriptions even when the table technically fits.

Render tested versions as plain text in driver tables. Avoid inline-code backticks or decorative wrapping around version labels.

## Logo and badge system

Inspect the original README header artwork of several siblings before creating a logo. Common library headers show a roughly 300px-wide centered image. Reuse their visual vocabulary, proportions, transparency, and typography while making the new library distinguishable. Do not invent a new visual system for each icon or flatten an existing logo into an unrelated placeholder.

The current search and cache marks use a blue Go gopher, large white eyes, cream muzzle and paws, heavy dark navy outlines, and a white outer sticker border. A domain prop explains the library, such as search's magnifier and result list or cache's speed gauge. A large rounded lowercase white wordmark with a dark outline anchors the composition. Preserve the transparent canvas, readable wordmark, and complete silhouette; do not export a black background matte or crop the sticker border. Inspect the actual images before extending this family so this description does not become a substitute for the artwork.

For a new mascot logo, use original sibling images as visual references and create an original domain-specific composition. When using image generation, request actual transparency and inspect the final file, including its alpha channel and appearance at README size. Preserve the prompt, reference provenance, and intended output path alongside the final artwork, as search does in `docs/assets/logo-prompt.txt`. This is reproducibility guidance for a raster mascot, separate from the code-native SVG badge workflow below.

Locate the documentation site's current design sources. If a `design-system.html` exists, inspect it; if not, find the maintained theme guidance, theme CSS, and original artwork. In the current documentation checkout, `ai/site-theme-system.md` maps those sources. Do not assume an old design-file path still exists or copy remembered color values without checking.

Feature and driver cells should use repository-owned SVG icons at their intended square size, typically 22 by 22 CSS pixels, followed by real HTML or Markdown text. Provider names must remain selectable and discoverable with browser Find; alt text or text embedded inside an SVG is not a substitute. Keep the icon outside the label's link so its underline cannot extend toward the icon. Use separate image and text elements with consistent vertical alignment. Do not shrink a wide icon-and-label badge into an icon-sized box, which makes the entire badge illegible.

Inspect current generators and source artwork rather than copying remembered sibling styles. `notification/docs/gen/tables.go`, its presentation metadata and `docs/assets/badges/` demonstrate the icon-plus-text tables; search remains useful for original artwork and other established generator patterns.

Prefer repository-owned SVG output for these simple vector assets. Embed vector paths or artwork so GitHub does not need third-party nested SVG fetches. Include accessible titles or alt text, consistent dimensions, spacing, and text baselines. Keep brand-icon provenance and license information with the source assets when needed. Do not add an image-generation dependency for code-native vectors.

Status badges should be compact, with readable text, consistent spacing, rounded outer ends and no clipped values. Apply clipping to the complete label/value background where needed to preserve both rounded ends. Use a real status for an unreleased library rather than a broken latest-tag badge. Test-count and coverage badges should be generated from evidence. Codecov, workflow, and package-reference badges must target the actual repository, workflow filename, branch, and module path. Public README badge URLs must not contain upload secrets or copied credential-like query parameters.

Inspect current summary badges in siblings such as cache, str, execx, and crypt. Match their compact flat Shields-style appearance: gray labels, blue metadata, green passing coverage/test values, and a neutral unreleased status where appropriate. Repository-owned, evidence-driven SVGs can preserve that appearance without oversized bespoke status pills.

Keep applicable badges together in the same centered badge block so they flow inline and wrap naturally. Compare sibling inventories for package reference, license, CI, Security, supported Go version, Codecov, release/version, and executed test counts. Do not add redundant per-driver unit-coverage badges when the aggregate unit report already covers every first-party driver module. Distinguish unit, integration, and merged coverage labels accurately.

## Render verification

Check the actual GitHub rendering after an authorized push, or use the GitHub Markdown renderer and an isolated preview before publication. Verify logo and badge loading, table alignment, desktop and phone layouts, light and dark contrast, readable icon and text size, code fences, and internal navigation. Long commands may scroll within code blocks; the whole page should not overflow. A local file existing is not proof that its raw URL works on GitHub, especially while a repository is private or an asset has not been pushed.

Distinguish Markdown structure and link checks from visual inspection: successful rendering alone does not verify alignment, clipping or legibility.

Check remote asset availability without leaking credentials. If visual verification needs a browser, use a fresh context and inspect only task-related pages. Report whether the result was verified locally, rendered through GitHub Markdown, or inspected on the live repository.

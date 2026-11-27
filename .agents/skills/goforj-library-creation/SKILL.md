---
name: goforj-library-creation
description: Build a standalone GoForj sibling library, or bring a new library or driver family to release readiness, using current GoForj API, examples, generated documentation, branding, testing, and repository conventions. Use for library creation and substantial library readiness work, not ordinary application development or a small unrelated package fix.
---

# GoForj Library Creation

Build a reusable Go library that feels native to the GoForj family and works independently of the framework. Treat implementation, runnable examples, generated documentation, visual presentation, and verification as parts of the deliverable.

For a full library build, read [the search reference implementation and acceptance checks](references/search-reference.md). It maps this workflow to concrete working artifacts. Use those artifacts as implementation references, then prove the corresponding outcomes for the new library; matching prose or filenames alone is insufficient.

## Establish the actual scope

Read applicable repository instructions and inspect `git status` before editing. Preserve unrelated work. Locate the GoForj checkout, the target library, relevant sibling checkouts, and the documentation-site checkout from the current workspace; do not assume the maintainer's home directory or a particular checkout name.

Find the relevant design under GoForj's `docs/designs/` and read [repository ownership](../../../docs/context/repo-boundaries-and-ownership.md). A proposal explains intent; current implementation and the user's later decisions establish what actually ships. Record the intended library boundary, primary workflow, drivers, minimum Go version, and any explicit exclusions before implementing.

Challenge that scope against the domain's basic expectations before polishing the API. For libraries that persist sensitive values or claim encryption, read [security and persistence](references/security-and-persistence.md). An externally managed plaintext mount reader does not provide encrypted local storage. Make that distinction explicit instead of quietly treating a missing core guarantee as out of scope.

Study sibling source, generators, examples, tests, artwork, module layout, and workflows. Do not copy only README styling or treat every historical convention as current. Use [implementation and validation](references/implementation-and-validation.md) to select useful precedents and establish the engineering plan.

For an existing library, improve the established design in place. A readiness review does not authorize a new module topology, a wholesale generator rewrite, or a public API redesign.

Keep these scopes separate:

- Standalone library: domain API, drivers, portable behavior, fakes, examples, documentation, and tests.
- Framework integration: component selection, generated configuration, managers, lifecycle wiring, commands, and Lighthouse. Include only when requested.
- External setup and publication: GitHub repository settings, Codecov, webhooks, tags, releases, and docs-site registration. Include only the actions authorized for the current task.

Proceed with routine reversible implementation choices. Carry earlier authorization forward; do not repeatedly ask about an approved step. Missing hosted credentials or an explicit driver exclusion must not become an invented integration result or an unrequested provider addition.

## Build the library and its evidence together

Define a small application-facing API and explicit portable guarantees before implementing drivers. Constructors should make dependency ownership clear. Separate pure construction from connection or provisioning operations where applicable; an explicitly named `Open` may perform I/O. Preserve meaningful backend differences instead of claiming identical semantics.

Keep optional dependencies and their Go requirements isolated when that benefits consumers. Inventory every `go.mod`, intentional relative replacement, module manifest, and release script. Do not impose search's module topology, driver list, limits, receipt design, or minimum Go version on another domain.

If the library has drivers, implement representative behavior early enough to test the abstraction against materially different backends. Use real backend integration tests and a shared conformance suite for the portable contract. Start each disposable service once per test-package run, isolate test data within it, and clean up even after failures. Embedded drivers use real embedded engines. A pure utility library does not need invented drivers or containers. Follow the detailed guidance in [implementation and validation](references/implementation-and-validation.md).

The default target for a new library under this workflow is **100% unit statement coverage**, measured separately from integration coverage. Preserve an existing library's established coverage policy unless changing it is in scope. Do not obtain the target with exclusions, fabricated evidence, rounded percentages, or integration-only execution. Test each new validation branch and failure mode directly.

For new libraries, keep examples in the top-level `examples/` package or module, with runnable programs such as `examples/<workflow>/main.go`. Use one authoritative workflow implementation. A shared test runner should discover and execute those programs and verify their output. Generate README snippets from the same source; do not maintain a second implementation in example tests. Preserve coverage and race instrumentation when execution uses subprocesses.

## Make the first read useful

Read [documentation and branding](references/documentation-and-branding.md) when creating the README, API index, generators, logos, badges, or feature and driver tables.

Lead with the library's benefit and a visible, working quick start. Keep detailed contracts and operational caveats in focused documentation linked from the README. Use a two-column category/functions API index, preserving the target's established Group or Category label. Its function links jump to matching example headings **inside the README**. The target section must contain the actual demonstrated operation. Example headings are plain text. Link complete programs from a runnable-example index rather than repeating source and test links beside each snippet.

Generate documentation and assets from their authoritative sources, regenerate all checked-in projections, and verify a second generation produces no diff. Test that anchors resolve, indexed APIs have matching examples, expected output is checked, and public support claims come from executed tests. Use plain language and avoid em dashes.

Use realistic application models in examples, with the schema, data and displayed results kept consistent. Match the index's categories and API names in the destination headings; consult the documentation reference for the precise layout.

Show example statements without helper-function wrappers or CLI scaffolding. Keep runnable commands in compact fenced blocks with short labels. Use scoped error checks when no returned value or error is needed afterward; preserve normal Go scope when it is.

When performance comparison is requested, read [performance evidence and graphs](references/performance.md). Match sibling SVG presentation, measure the same validated workload across drivers, and distinguish a new measurement run from regenerating charts of an existing snapshot.

## Configure the repository when authorized

Read [repository setup and release](references/repository-and-release.md) for CI, Codecov, repository metadata, notifications, and publication.

Prepare concrete settings and workflow changes before requesting any missing access. Existing credentials do not authorize additional services or publications. Reuse the maintainer's Git identity and process-local authentication. Never copy tokens, Codecov upload secrets, webhook URLs containing secrets, or credentials from another README into generated files.

## Finish with several review passes

Review the completed library through distinct lenses:

1. **Consumer API and compatibility:** useful workflow, ownership, defaults, errors, cancellation, concurrency, and module boundaries.
2. **Driver correctness:** shared behavior, backend-specific limits, malformed responses, partial failures, visibility, precision, and native schema or lifecycle behavior where applicable.
3. **Security and resources:** domain completeness, credential and key ownership, persistence guarantees where applicable, error redaction, input and response bounds, allocation budgets, and vulnerability reachability on a patched supported toolchain.
4. **Documentation and operations:** runnable examples, generated parity, accurate tables, visible quick start, readable artwork, links, CI, coverage uploads, and independent module publication when in scope.

These are review lenses, not a requirement to spawn agents. Reproduce concrete findings, add regression coverage, fix them, and rerun the checks affected by the change. Preserve public APIs, configuration, persisted formats, and minimum Go versions by default; identify concrete migrations when a compatibility change is necessary.

Use focused Conventional Commits, preferably separating drivers and unrelated features. Before committing, inspect status and the staged diff and stage only intended paths. Before an authorized push, check remote divergence. Do not create release tags merely to make a badge look complete.

Report what shipped, the concrete fixes, validation evidence, and any remaining external setup or CI results. Distinguish source validation, public installation, live-provider verification, and versioned publication. Report readiness against the agreed verification scope, identify pending gates, and distinguish prepared, executed, passed, pushed, and published. Do not describe the library as ready while an in-scope driver, required check, or authorized integration remains incomplete.

Coverage and green checks establish tested behavior, not completeness of the design. Resolve missing core domain guarantees before making a readiness claim.

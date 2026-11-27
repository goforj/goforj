# Repository setup and release

Read this when preparing CI, Codecov, GitHub metadata, notifications, or a library release. These instructions describe how to complete authorized setup; they do not authorize external changes merely because the skill is invoked.

## Repository identity and metadata

Verify the actual owner, repository, authenticated identity, access, and visibility before external mutations. Preserve the maintainer's existing Git author and committer identity. Follow the workspace credential instructions, including the provider index, and keep supplied authentication process-local. Never change persistent CLI authentication as a side effect of using a task token.

When requested, write a concise repository description explaining the Go developer benefit and primary capability. Choose accurate discoverability topics such as Go, Golang, the domain, and implemented backends. Inspect sibling metadata for consistency. Avoid keyword stuffing, unimplemented capabilities, or claims that metadata guarantees search ranking.

Repository creation, visibility changes, app installation changes, homepage updates, pushes, release tags, and publication remain within the user's actual authorized scope. Carry explicit approvals forward. If missing account verification prevents setup, identify the exact pending step and continue independent local work. Do not report prepared settings as enabled.

## CI and coverage

Adapt current sibling workflows and pin actions and scanners according to the current maintenance policy. Verify versions instead of freezing the skill to today's pins.

Include the applicable checks:

- Root and optional modules on their stated minimum Go lines, plus the supported current line.
- Formatting, module inventory, tidy parity, unit tests, race tests, vet, and executable examples.
- Full real-backend integration tests, including required native extensions and authenticated fixtures.
- Two generations with no tracked diff for generated documentation and assets.
- Vulnerability scanning on a patched supported toolchain, with relevant published modules and local module resolution accounted for.
- Unit coverage uploads, combined or integration uploads when useful, and strict unit coverage enforcement.

Treat Codecov as both repository integration and measured evidence. Inspect an established sibling's current configuration and the target's existing app access. If the authorized task includes setup, enable access for the new repository in the existing GitHub App installation rather than creating redundant installations. Store any required upload token as a repository secret through the authorized interface; never put it in a README, workflow literal, badge, or log.

Configure project and patch checks against the unit coverage flag when the policy is 100% unit coverage. Keep integration evidence separate so real-backend runs cannot conceal missing unit tests. Ensure upload failures fail CI. Do not enable carryforward behavior that makes an absent current run look fresh without an explicit reason. Verify the actual upload and check result for the pushed commit, not just the presence of a YAML file or badge.

## Existing notification routes

When matching the family's webhooks is requested, inspect a current authorized sibling and the target before writing. The workspace instructions may document the existing route map; verify it against live configuration. Do not copy obsolete catch-all hooks or duplicate existing destinations.

The established family routes are:

| Destination | GitHub events |
| --- | --- |
| Issues | `issues`, `issue_comment`, `discussion`, `discussion_comment` |
| Pull requests | `pull_request`, `pull_request_review`, `pull_request_review_comment` |
| Stars | `watch` |
| Builds | `check_suite` |
| Releases | `release` |

Native Discord GitHub-compatible destinations end in `/github`, use JSON payloads, and keep SSL verification enabled. Ordinary pull-request conversation uses `issue_comment`; review comments use the review routes. Build routing uses aggregate check-suite events.

Full webhook URLs contain secrets. Retrieve and use them only inside the authorized requesting process, keep them out of command arguments and output, and never commit them. Verify configuration and recent delivery results. If a dedicated test delivery is needed, keep it within the user's notification-setup authorization. Avoid duplicate retries when delivery state is uncertain.

## Candidate archive verification

When the repository supports prepublication archive checks, test the exact committed candidate before creating tags. Freeze one commit for source files, module boundaries, manifests, Go requirements, licenses, and archive metadata; do not mix moving HEAD or working-tree state into that snapshot. Keep external dependency checksum verification active and candidate caches isolated. Exercise each runtime module at its declared minimum and the combined dependency graph, with meaningful behavior checks for new exported capabilities.

Record the candidate commit and archive checksums. Local-proxy success is a rehearsal, not evidence of public availability. A later source change requires fresh candidate evidence. Preserve the separate public-consumer check below.

## Public-consumer verification

Prove actual public installation before describing a revision as installable. A placeholder root requirement such as an unpublished `v0.0.0` remains invalid even when relative replacements let repository tests pass. Pin an existing public revision for prerelease consumers; retain intentional replacements for development.

Use a Go verification command following the repository's tooling layout, with a Make target and CI coverage when appropriate. Resolve the actual pushed commit or version through the public module proxy and normal checksum database, outside the repository with a fresh module cache, `GOWORK=off`, no local replacements, no rewritten requirements and no source overlays. Isolate workspace, private-module bypass and persistent Go settings that could silently change resolution. Exercise the supported minimum Go line as well as applicable CI toolchains.

Check the largest supported composition and each runtime module independently. An explicit root upgrade in the combined consumer can mask an optional module's invalid or outdated minimum. Compile the relevant driver packages and execute a small behavior check for the public guarantees that depend on the selected root, using local deterministic fixtures rather than live provider calls. Confirm selected module versions and reject replacements.

Do not treat rewritten candidate archives or a local proxy as proof of public installation. Record the exact verified revision. Keep README install commands on that known revision until a newer revision or real release is verified; avoid a self-referential commit hash. Notification's `docs/consumer`, `docs/gen/consumer.go` and `make verify-consumer` provide a concrete implementation to inspect, not a requirement to duplicate its module count or provider contracts.

## Independent module publication

Inventory module manifests, existing tags, release scripts, and first-release version conventions before proposing a release. A root tag does not publish nested module versions. Tooling modules should not be tagged merely because they contain a `go.mod`.

Use the repository's release planning and tagging scripts when available. Align the requirements and sums of independently published modules with the chosen release. Preflight every tag before creating any of them. Preserve intentional development replacements; validate the released consumer independently of those replacements.

Prepare release-facing README text and the authoritative version badge before tagging, then regenerate and verify their projections. Push the intended release commit and check CI and Security for that exact SHA. Recheck remote tags and ensure the release script tags the reviewed commit. Do not move or overwrite an existing published tag to include a later fix.

Compare origin's advertised tag targets with the tested commit, resolving lightweight and annotated tags correctly. A matching local tag alone does not establish the remote publication state.

For an authorized first release, publish prerequisites in dependency order and verify each version resolves independently. Test a clean consumer outside the repository with `GOWORK=off`, no local replacements, and the intended module versions. Successful repository tests or a visible root tag are insufficient evidence that every driver module is available.

Use the public module proxy and normal checksum verification for the post-publication check. If candidate archives were tested through a local proxy at the same version, use a separate fresh module cache so those archives cannot masquerade as the published versions. Confirm the root resolves before pushing a driver tag that requires it. Temporary proxy propagation delays justify bounded retries; inspect current remote state before retrying any publication mutation.

Create the GitHub release from the verified existing root tag with notes that identify the independently tagged modules, installation commands, minimum Go requirements, and meaningful shipped behavior. Follow the sibling convention for a single coordinated release announcement unless the project already publishes module-specific releases. Verify the resulting release URL, tag target, and public clean-consumer result before reporting publication complete.

Make the public notes useful to consumers: lead with the concrete developer benefit, then use short sections such as What's better, Try it, and Upgrading. Use a few concise bullets and a copyable installation block. Describe improvements accurately and positively without burying readers in implementation details or overstating measurements. State concrete compatibility and operational actions separately. Link a repository release record for module inventory, detailed validation, benchmarks, and technical explanations rather than turning the announcement into an audit log. If only the note wording needs correction after publication, edit the existing release body; do not create another release or move tags.

Tagging and publishing are separate from committing. Do not create a release to repair an unreleased badge. Register the library on the documentation site or wire it into generated Apps only when that separate integration is in scope; update authoritative catalogs or generators and regenerate their outputs.

Keep source quality, public installation, live-provider verification and versioned publication separate in the final assessment. Passing local protocol fixtures does not establish hosted delivery; missing release tags does not invalidate a verified prerelease install. Qualify initial-release suitability against the user-approved verification scope and list any remaining required gates. Check the final pushed SHA rather than borrowing CI results from a previous commit.

Finish with the exact pushed commit and observed CI, upload, app-access, webhook-delivery, and publication state relevant to the request. State concrete pending items without presenting them as successful setup.

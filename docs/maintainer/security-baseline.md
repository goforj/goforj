# GoForj Security Baseline

This document defines the security review surface for GoForj and its sibling repositories. Repository controls are necessary but do not replace organization settings, deployment review, or application-specific threat modeling.

## Assessment scope

The GoForj organization currently contains 25 active repositories. The public repositories are `.github`, `atlas`, `cache`, `collection`, `console`, `crypt`, `docs`, `env`, `events`, `execx`, `godump`, `goforj`, `harbor`, `httpx`, `mail`, `metrics`, `null`, `queue`, `scheduler`, `storage`, `str`, `web`, and `wire`. `ship` and `demo-repository` are private.

The repositories have different risk profiles:

| Group | Repositories | Primary security concerns |
| --- | --- | --- |
| Framework and generation | `goforj`, `atlas`, `docs` | Generated dependency integrity, command execution, template safety, development control-plane authentication |
| Data and messaging | `cache`, `queue`, `storage`, `events` | Driver consistency, tenant boundaries, deserialization, remote service credentials, retries, distributed coordination |
| Network and process | `httpx`, `web`, `execx`, `mail` | SSRF, redirects, header handling, command arguments, transport security, credential exposure |
| Identity and sensitive values | `crypt`, `env`, `null` | Key management, secret handling, unsafe defaults, serialization, error disclosure |
| Build and developer tooling | `wire`, `console`, `godump`, `collection`, `str`, `scheduler`, `metrics` | Generated code integrity, terminal escape handling, sensitive output, dependency and release provenance |
| Applications | `harbor`, `ship`, `demo-repository` | Authentication, authorization, update delivery, container and desktop packaging, production secrets |
| Organization policy | `.github` | Inherited reporting policy, reusable workflows, contribution guidance, and ownership |

## Required control coverage

| Surface | Repository evidence | Organization or deployment evidence |
| --- | --- | --- |
| Vulnerability reporting | Supported versions, confidential contact, response targets | Private reporting enabled and a maintained response rotation |
| Source analysis | Tests, race checks where relevant, `go vet`, CodeQL, and focused fuzzing for parsers or protocol boundaries | Required checks protected on default branches |
| Dependency risk | `govulncheck -test` for every Go module, npm audit for every lockfile, and reviewed exceptions with owners and expiry dates | Dependabot alerts and security updates enabled |
| Secret prevention | Full-history Gitleaks checks with narrow synthetic-value exceptions | GitHub secret scanning and push protection enabled |
| CI supply chain | Explicit least-privilege permissions and immutable action commit pins | Actions allowlist and restricted workflow approval policy |
| Container supply chain | Base image digests, verified downloads, configuration scanning, image vulnerability scanning, and image SBOMs | Registry retention, signing, admission policy, and rebuild cadence |
| Release integrity | Versioned source, module inventory, release SBOM, and reproducible release instructions | Artifact attestations, protected tags, and signing policy |
| Runtime boundaries | Repository-specific threat model and secure defaults | Network policy, identity, authorization, encryption, audit retention, and incident response |

## Current ecosystem gap register

PR #109 establishes the first repository-local baseline for `goforj`. The other repositories do not yet inherit that workflow or dependency configuration.

The 2026-09-04 review actively scanned every repository checkout with dependency manifests using `govulncheck -test ./...` and production npm audits. The current PR branch has separate clean Go and npm gates, so its results supersede the older `goforj` workspace checkout included in that inventory. The `.github` and `docs` repositories were reviewed from local checkouts, and the private `demo-repository` was reviewed through a temporary authenticated clone. Full-history Gitleaks scans found no unaccepted secrets in `goforj` or any in-scope sibling repository. `ship` and `harbor` were excluded from the sibling remediation pass by request.

GitHub reported 221 open Dependabot alerts on the `goforj` default branch during this review: 4 critical, 97 high, 103 moderate, and 17 low. The review token could not read alert details, so an administrator must reconcile that queue against the current branch scans, remove stale alerts, and assign any remaining findings. A passing pull request scan does not by itself close default-branch alerts.

| Result | Repositories | Required response |
| --- | --- | --- |
| Reachable Go findings | `atlas`, `cache`, `docs`, `events`, `harbor`, `httpx`, `queue`, `ship`, `storage`, `web` | Triage each advisory in its owning repository, update reachable dependencies, and use only scoped, expiring exceptions for demonstrated non-applicability |
| Production npm findings | The `docs`, `ship`, and `harbor` frontends | Update lockfiles at their authoritative source, regenerate checked-in output, and confirm the generated application is clean |
| No reachable Go findings in the active scan | `collection`, `console`, `crypt`, `env`, `execx`, `godump`, `mail`, `metrics`, `null`, `scheduler`, `str`, `wire` | Adopt recurring scans so this point-in-time result does not become stale |
| Go modules without analyzable packages | Documentation, example, or integration modules in `cache`, `collection`, `events`, `mail`, `metrics`, and `queue` | Record these as not applicable or add a buildable validation surface; do not report them as clean scans |
| No Go or npm dependency manifest | `.github`, `demo-repository` | Maintain workflow supply-chain controls and recurring full-history secret scanning |

Finding counts are triage inputs, not severity by themselves. Optional drivers, examples, benchmarks, generated applications, and test dependencies have different deployment exposure, but each published or generated surface still needs an explicit disposition.

| Priority | Scope | Required follow-up |
| --- | --- | --- |
| 1 | `goforj/.github` | Publish an inherited `SECURITY.md` and reusable Go, npm, secret, CodeQL, and release-security workflows |
| 1 | `ship`, `harbor`, `demo-repository` | Apply least-privilege workflow permissions, immutable action pins, application threat modeling, and container or desktop artifact scanning |
| 1 | `cache`, `queue`, `storage`, `events` | Inventory every nested driver and integration module, scan test dependencies, and add protocol and untrusted-input fuzz targets |
| 2 | All remaining Go libraries | Adopt the reusable baseline, enable dependency automation, and publish SBOM and provenance evidence with releases |
| 2 | `wire` | Replace legacy v2 GitHub Actions and pin every action to an immutable commit |
| 2 | Generated container templates | Maintain image versions and digests outside Dependabot because `.tmpl` files are not covered by Docker ecosystem updates |

The container gate blocks fixable high and critical findings in the CI test image. Unfixed findings cannot be remediated by this repository, so CI records all high and critical findings as a retained report while the blocking pass ignores only findings with no upstream fix. Maintainers must review that report during dependency updates and convert any longer-lived risk acceptance into a scoped, expiring exception.

Generated Compose image references remain tag-based and therefore mutable. The security workflow inventories every current reference and retains a vulnerability report and SBOM for the resolved image. The remaining digest-pinning and automated-update gap is owned by `@cmilesio` with a target date of 2026-12-04. Until that is complete, release review must compare the resolved image digest with the prior retained inventory.

GoForj releases currently use annotated, unsigned source tags, GitHub Releases do not cover recent tags, and the 14-day CI SBOM artifact is not tied to a release. A tag-triggered workflow must verify the annotated tag and retain release-keyed SBOMs for every discovered module and lockfile. Artifact attestations must cover only artifacts the workflow actually builds. This release-integrity work is owned by `@cmilesio` with a target date of 2026-12-04.

The CI test image currently trusts the Docker apt key downloaded during its build without checking an expected fingerprint, and Docker packages are not version pinned. Fingerprint verification and an explicit package update policy are owned by `@cmilesio` with a target date of 2026-12-04.

Remote backup repositories are trusted operational inputs. Object metadata is bounded before download, but the current storage interface returns whole objects, so a compromised repository can still create memory pressure before the actual size is compared with metadata. A streaming, size-limited storage read contract is owned by `@cmilesio` with a target date of 2027-03-04.

Repository files cannot prove organization settings. An organization administrator must separately verify two-factor authentication enforcement, default workflow token permissions, branch rules, protected tags, private vulnerability reporting, Dependabot alerts, secret scanning, push protection, and code scanning. These settings were not visible to the token used for this assessment and remain unverified until that administrative review is recorded.

## Rollout acceptance

A sibling repository is covered only when all applicable modules, lockfiles, workflows, images, and release paths are inventoried. A green root-module test alone is not sufficient for a multi-module repository. Each adoption PR should record any control that is not applicable, who approved the exception, and when it expires.

Security exceptions must be narrow and machine-checkable. An advisory exception must bind the advisory to the expected dependency and package scope. It must fail closed when its expiry date passes or when a new package path becomes reachable.

Release readiness requires no unaccepted reachable vulnerability findings, a secret scan, an SBOM for each published artifact or module family, immutable CI dependencies, and successful tests using published sibling versions with `GOWORK=off` where applicable.

# GoForj Security Baseline

This document records the repository-local security baseline for the GoForj framework and the maintenance obligations that automation cannot close. The public ecosystem overview belongs in the [GoForj security assurance documentation](https://goforj.dev/security/assurance).

A control is active only when its configuration is present on the default branch and its latest required run passes. Documentation describes the intended baseline, while workflow logs, Security tab results, dependency graphs, and source revisions provide the evidence.

## Assessment Scope

The GoForj organization has 25 active repositories. The current baseline includes 23: `.github`, `atlas`, `cache`, `collection`, `console`, `crypt`, `demo-repository`, `docs`, `env`, `events`, `execx`, `godump`, `goforj`, `httpx`, `mail`, `metrics`, `null`, `queue`, `scheduler`, `storage`, `str`, `web`, and `wire`.

Ship and Harbor are explicitly excluded. They require independent assessment and must not inherit a positive or negative conclusion from this baseline.

As of September 5, 2026, the baseline is merged in all 22 non-docs repositories. The docs application remains pending in [docs PR #40](https://github.com/goforj/docs/pull/40). The public assurance pages remain a draft in [docs PR #41](https://github.com/goforj/docs/pull/41) until the docs controls are active on its default branch.

## Core Repository Threat Boundaries

| Boundary | Trust Decision | Required Behavior |
| --- | --- | --- |
| Developer commands | `forj dev`, generation, integration tests, and profiling execute project-defined processes with the invoking user's permissions | Run them only in trusted repositories and keep command arguments explicit |
| Project generation | Configuration, names, paths, templates, dependency catalogs, and checked-in generated output cross into a new App | Validate before writing, change authoritative generators first, regenerate every mirror, and verify byte-stable output |
| Backup repositories | The operator chooses the repository, but its listings, manifests, metadata, and objects are untrusted inputs during reads | Require canonical relative paths, enforce logical prefixes and size limits, validate the complete delete set before mutation, and reject collisions |
| Backup integrity | SHA-256 checksums detect accidental or post-creation changes | Do not treat an unkeyed checksum as proof of producer identity; restore only operator-controlled backup sets |
| Local developer services | Generated Compose services are development conveniences that may expose databases, brokers, mail, and diagnostics | Bind host ports to `127.0.0.1` by default and require explicit configuration for wider access |
| Generated Apps | GoForj supplies code and secure-shaped defaults, while the deployed App remains application-owned | Application owners provide authorization, secrets, TLS, network policy, data classification, retention, and deployment testing |
| Sibling libraries | Drivers and reusable contracts are maintained in their owning repositories | Fix reusable behavior in the sibling, update every GoForj pin and generated dependency catalog, and validate published versions with `GOWORK=off` |
| CI and dependencies | Pull requests, third-party actions, package registries, images, and advisory data influence build decisions | Use least privilege, immutable action commits, dynamic manifest discovery, verified evidence, and narrow exceptions |

## Automated Controls

| Control | Repository Evidence | What It Does Not Prove |
| --- | --- | --- |
| Security inventory | `scripts/check-security-inventory.sh` and the security workflow discover every `go.mod` and `package-lock.json`, then require Dependabot parity | It does not discover undeclared runtime dependencies |
| Go vulnerability analysis | `scripts/check-vulnerabilities.sh` runs pinned govulncheck across modules, build tags, tests, and supported platform paths | It cannot find private advisories, unknown flaws, or application logic vulnerabilities |
| npm vulnerability analysis | The security workflow audits every discovered lockfile | It does not analyze browser behavior or dependencies absent from the lockfile |
| Dependency Review | Pull-request dependency deltas fail at the configured severity | It does not replace default-branch vulnerability reconciliation |
| Secret scanning | Gitleaks scans full Git history with redacted output | Pattern matching cannot prove that no secret exists or inspect external secret stores |
| Code scanning | Organization-managed CodeQL analyzes Go, JavaScript, TypeScript, and Actions where configured | Organization enrollment is an administrator-owned setting, and static analysis is not penetration testing |
| SBOM generation | Validated CycloneDX artifacts cover every discovered Go module and nonempty npm application | Fourteen-day CI artifacts are not signed release attestations |
| Container checks | Trivy inventories framework build images and generated template images; policy checks inspect generated container configuration | Image scans do not prove deployment admission, network policy, or runtime hardening |
| Behavioral verification | Unit, integration, race, platform, render, watcher stress, and verifier calibration jobs exercise high-risk behavior | Tests prove only represented cases and environments |
| Workflow integrity | Third-party actions use immutable commits and jobs declare scoped permissions | A pinned action can later receive an advisory and still needs update monitoring |

## Exception Policy

Security exceptions must be narrow, owned, expiring, and machine-checkable. A vulnerability exception must bind an advisory to the expected dependency and exact package or package prefix. New findings, expanded reachability, scanner errors, and expired entries must fail the workflow.

The current `goforj` govulncheck exceptions cover two Docker daemon advisories. GoForj imports client-side packages and does not embed the affected daemon behavior. Both records expire on December 4, 2026 and are owned by `@cmilesio`.

An exception is not a permanent acceptance. Review it when the upstream project publishes a fix, the dependency graph changes, a new package path becomes reachable, or its expiry approaches.

## Security Change Workflow

1. Identify the untrusted input, privileged operation, and owning repository before changing code.
2. Classify compatibility impact separately for source API, configuration, persisted data, runtime behavior, operations, and minimum Go version.
3. Change the authoritative generator, template, catalog, or implementation rather than only a checked-in mirror.
4. Add direct tests for the new validation branch and failure mode. For destructive operations, prove the complete input is valid before the first mutation.
5. Discover and validate every relevant Go module and npm lockfile. Do not rely on a root-only test.
6. Regenerate tracked output and verify that a second generation produces no diff.
7. Run the security inventory and applicable workflow-equivalent checks.
8. Record any residual risk with an owner, exact applicability, review condition, and expiry.
9. Update the public assurance or production-hardening documentation when user or operator responsibility changes.

Useful local checks from the repository root include:

```bash
bash scripts/check-security-inventory.sh
go test ./...
go vet ./...
```

Run nested modules independently. Use `GOWORK=off` when proving behavior against published sibling versions, and render test Apps only in a temporary directory outside this repository.

CI remains authoritative for the full build-tag, platform, integration, container, CodeQL, secret-history, SBOM, and verifier matrix.

## Residual Risk Register

| Risk | Current Mitigation | Owner and Target |
| --- | --- | --- |
| Generated Compose image tags remain mutable | CI records resolved image inventories, vulnerability reports, and SBOMs; release review compares resolved digests | `@cmilesio`, December 4, 2026 |
| Release SBOMs are short-lived CI artifacts and source tags are unsigned | Versioned source and per-revision SBOM validation remain available; release-keyed attestations are still required | `@cmilesio`, December 4, 2026 |
| The CI test image trusts the downloaded Docker apt key without an expected fingerprint and Docker packages are not version-pinned | Container builds and vulnerability scans gate fixable high and critical findings | `@cmilesio`, December 4, 2026 |
| Remote backup reads materialize whole objects before actual-size comparison | Metadata, declared size, object count, logical prefix, and checksum checks bound accepted backup content | `@cmilesio`, March 4, 2027 |
| Repository files cannot prove organization security settings | An administrator must verify two-factor enforcement, workflow token defaults, branch rules, protected tags, private reporting, Dependabot, secret scanning, push protection, and CodeQL | Organization administrator review required |

## Release Acceptance

A GoForj release requires no unaccepted reachable vulnerability finding, successful required tests, full secret-history scanning, verified SBOM coverage for each discovered manifest, immutable CI dependencies, and explicit disposition of container findings.

Where a release integrates newly published sibling versions, validate the largest supported generated App composition with `GOWORK=off`. Confirm that module selection does not resolve through local replacements and that every independently versioned nested module is published before updating downstream pins.

Release signing, protected tags, and retained release attestations remain separate controls until the residual work above is complete.

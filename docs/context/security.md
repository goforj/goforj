# Security Context

Read this file when a change touches untrusted input, project generation, backup or restore behavior, process execution, developer-service exposure, dependencies, CI workflows, secrets, authentication, or release integrity.

Use [GoForj Security Baseline](../maintainer/security-baseline.md) for the detailed control and residual-risk record. Use the [public security assurance documentation](https://goforj.dev/security/assurance) for enterprise-facing scope, evidence, limitations, and deployment responsibilities.

## Ownership First

Security fixes should land where the behavior is owned.

| Concern | Owner |
| --- | --- |
| App policy, generators, templates, CLI behavior, backup orchestration, and developer workflow | `goforj` |
| Reusable cache, queue, storage, events, web, mail, process, encryption, environment, or wiring behavior | The corresponding sibling repository |
| Generated application business rules, authorization, data policy, and deployment configuration | The generated App and its operators |
| Organization CodeQL, Dependency Review, branch rules, reporting, secret scanning, and push protection | GitHub organization administrators |

Do not introduce a new framework API solely to claim interoperability or centralize behavior that already belongs to a sibling. When a sibling fix is required, publish and validate that module before updating GoForj dependency catalogs and generated output.

## Core Invariants

- Validate names, paths, prefixes, metadata, and the complete target set before a destructive call.
- Reject absolute paths, traversal, cross-prefix objects, noncanonical names, and unsafe size declarations at the trust boundary.
- Claim new backup destinations exclusively and keep new backup files and directories owner-only.
- Treat backup checksums as integrity evidence, not producer authentication.
- Bind generated developer-service ports to loopback unless wider access is explicit.
- Keep secrets out of source, templates, generated output, logs, diagnostics, test fixtures, and CI artifacts.
- Pass untrusted pull-request values into scripts through environment variables instead of interpolating them into executable shell text.
- Pin third-party workflow actions to immutable commits and grant permissions per job.
- Discover every module, lockfile, generated dependency catalog, fixture, and build-tag path that can affect published or generated code.
- Fail when a scanner cannot analyze an input. Do not turn an error into an empty successful result.
- Keep vulnerability exceptions exact, owned, expiring, and automatically invalidated when applicability changes.
- Preserve compatibility unless a fixed dependency or implementation constraint requires a change, then state the concrete migration.

## Review Questions

Before implementation, answer:

1. What input can an untrusted repository, user, service, dependency, or backup producer control?
2. What filesystem, process, network, credential, deletion, or generated-code capability can that input reach?
3. Which repository and authoritative source own the behavior?
4. Can the operation validate everything before changing state?
5. Which direct failure tests prove the boundary?
6. Which nested modules, platforms, build tags, templates, fixtures, and generated outputs must remain in parity?
7. Does the change alter application or operator responsibility that belongs in public docs?
8. What remains unproven after CI passes?

## Validation Path

Start with the smallest direct tests for the changed boundary. Then validate every relevant module and generated mirror.

```bash
bash scripts/check-security-inventory.sh
go test ./...
go vet ./...
```

Use the repository's test render commands for generated behavior, but place rendered Apps under `/tmp`, never inside the GoForj repository. Use `GOWORK=off` for the published-version integration pass.

The pull request must leave the full required CI matrix green. A successful root test does not replace security scans, nested module tests, race and platform jobs, generated-output parity, container checks, or organization-managed CodeQL.

## Evidence and Limits

Workflow logs and artifacts belong to one source revision. Preserve them outside normal GitHub artifact retention when an assessment needs longer-lived evidence.

Passing automation does not establish that an App has correct authorization, secure deployment networking, protected secrets, sufficient audit retention, signed releases, or an exercised incident response process. Keep those claims with the application and organization that own them.

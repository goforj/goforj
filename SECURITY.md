# Security Policy

## Supported versions

GoForj provides security fixes on the latest published minor release. Consumers should update to the latest release before reporting a vulnerability or requesting support.

## Reporting a vulnerability

Report suspected vulnerabilities through [GitHub private vulnerability reporting](https://github.com/goforj/goforj/security/advisories/new). Do not include sensitive details in a public issue.

Include the affected version, configuration, reproduction steps, expected impact, and any known mitigations. Maintainers aim to acknowledge reports within three business days and provide an initial assessment within seven business days.

## Security boundaries

GoForj is a developer tool and application framework. Commands such as `forj dev`, project generation, integration testing, and profiling intentionally execute toolchain commands and project-defined processes with the invoking user's permissions. Only run them in repositories you trust.

Generated applications inherit their own dependencies and deployment configuration. Application owners remain responsible for access control, secret storage, network policy, TLS termination, data classification, container hardening, and security testing of the deployed application.

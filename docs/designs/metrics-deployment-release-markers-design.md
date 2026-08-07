# Metrics Deployment and Release Markers Design

## Status

- Design status: implemented
- Planning date: 2026-07-31
- Implemented: 2026-08-01
- Scope: App metrics, vmagent target discovery, Grafana, and future Lighthouse summaries

## Summary

Deployment identity should be attached by the scrape system, not added as dynamic labels to every framework metric at instrumentation time.

Each scrape target may carry one value for a small, fixed set of labels such as `environment`, `release`, and `revision`. The existing target labels (`app`, `environment`, `process`, and `service`) remain the model to extend. Exact deployment events belong in the deployment system or dashboard annotations rather than in request, queue, scheduler, or resource metric vectors.

## Current Behavior

A GoForj App owns a Prometheus-compatible registry, while vmagent discovers endpoints through `metrics-targets.json`. That target file already attaches bounded deployment-context labels outside the App:

```json
{
  "app": "app",
  "environment": "local",
  "process": "app",
  "service": "example"
}
```

Framework metric vectors deliberately use surface-specific labels. Adding release data in every observer would duplicate configuration, expand every vector, and couple instrumentation to a deployment mechanism.

## Decision

Use scrape-target labels for current deployment identity:

- `environment`: stable deployment environment, preserving the existing label
- `release`: operator-facing release version, when known
- `revision`: immutable source or artifact revision, when known
- `app`, `process`, and `service`: preserve their existing meanings

The label key set is fixed. A target has at most one value for each key, values are normalized and length-bounded, and absent metadata omits the optional label rather than inventing `unknown` series.

Framework-managed target generation reads optional `APP_VERSION` and `APP_REVISION` values and publishes the discovery file atomically. Static local generation may omit release metadata. Kubernetes, service discovery, or hosted deployments should produce the same logical labels through their native relabeling configuration.

Do not add `release`, `revision`, deployment timestamps, branch names, image digests, or rollout IDs to every framework metric constructor. Scrape-time labels still affect stored series, but they avoid hot-path propagation and keep cardinality controlled at the target boundary.

## Deployment Events

Current identity and event history are different concerns.

- Queries group or compare metrics by the target's `release` or `revision`.
- A deployment start, promotion, rollback, or completion is an event emitted by CI/CD to the dashboard annotation or event store.
- GoForj should not synthesize a perpetually increasing deployment-event metric.
- A single optional `app_build_info`-style info metric may expose build metadata when an App needs self-description without scrape discovery, but it is not the default release-marker mechanism and must not be joined into every dashboard query.

This lets Grafana show rollout lines without making the App registry an event database. Lighthouse may later consume the same normalized identity and event stream, but it should not receive a separate metric label contract.

## Cardinality and Retention

The allowed keys are bounded, but `revision` values naturally change. Operators should retain only the dimensions they need and use backend retention for old revisions. Free-form values such as deployer, pull-request title, branch, host-generated rollout UUID, or timestamp are annotation fields, not metric labels.

For rolling releases, two revisions may legitimately coexist. Queries that do not compare releases should aggregate without `release` and `revision`; release dashboards may retain them explicitly.

## Compatibility

- Existing metric names and vector label lists do not change.
- Existing `metrics-targets.json` remains valid because new target labels are optional.
- Existing dashboards continue to query `app`, `environment`, `process`, and `service`.
- Target metadata can be introduced independently by deployment environments.
- Lighthouse remains downstream of the proven Prometheus contract.

## Validation Plan

Implementation should prove that target labels appear on scraped series, two rolling revisions can be queried independently, ordinary dashboards still aggregate across revisions, malformed or oversized values are rejected by the target producer, and generated local targets remain valid JSON.

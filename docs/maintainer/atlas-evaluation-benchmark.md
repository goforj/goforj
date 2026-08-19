# Atlas evaluation benchmark

This page records the latest complete comparison of GoForj's baseline
`AGENTS.md` guidance against no framework guidance. It exists so maintainers can
see the measured result for every promoted evaluation without reconstructing a
benchmark from individual attempt artifacts.

> [!IMPORTANT]
> This is a provisional diagnostic benchmark, not a release qualification.
> The `unconfined-local` backend can verify the resulting Project, but it cannot
> authoritatively prove command use, filesystem isolation, or process cleanup.
> Results marked **Rerun** were measured before their verifier contract was
> corrected and must be replaced by a complete benchmark against the current
> contracts.

## Latest complete scorecard

| Treatment | Passed | Failed | Measured | Pass rate |
| --- | ---: | ---: | ---: | ---: |
| No framework guidance | 18 | 12 | 30 | 60.0% |
| Baseline `AGENTS.md` | 22 | 8 | 30 | 73.3% |

The observed difference is **+4 evaluations**, or **+13.3 percentage points**,
for baseline guidance. Each evaluation has one paired trial, so this run is a
coverage and calibration signal rather than evidence of a stable treatment
effect. Repeated trials are required before making a reliability claim.

This scorecard measured `AGENTS.md`, not the complete Atlas experience. The evaluation runner now exposes an incremental treatment ladder so later benchmarks can attribute improvements to the surface that caused them:

| Profile | Project instructions | Recommended skills | Atlas MCP |
| --- | :---: | :---: | :---: |
| `none` | — | — | — |
| `agents` | ✓ | — | — |
| `agents-skills` | ✓ | ✓ | — |
| `atlas` | ✓ | ✓ | ✓ |

Compare adjacent profiles when measuring one surface. Use `none` versus `atlas` when measuring the complete user experience. Do not present the 60-session benchmark above as evidence for skills or MCP until those treatments have their own completed attempts.

## Capability coverage

The versioned Atlas coverage catalog maps framework capabilities to promoted evaluations and classifies them as `smoke`, `core`, or `extended`. This keeps planned gaps visible without creating weak source-token evals merely to make the count larger.

```sh
forj atlas:eval coverage
forj atlas:eval coverage --format markdown
```

`smoke` is the fastest release-critical portfolio, `core` includes smoke plus representative framework behavior, and `extended` includes specialized or expensive coverage. Tier selection is cumulative.

The current catalog covers **40 of 57** named capabilities. The 17 planned gaps remain visible in the report instead of being represented by low-signal token checks. `add-resilient-job` is the first composite eval added from this map: one agent task measures typed job generation, retry safety, idempotent behavior, cancellation, and timeout policy together.

## Results by evaluation

`Pass` means the agent completed and every available outcome-gating diagnostic
check passed. Quality observations and workflow checks requiring unavailable
trusted telemetry do not contribute to this diagnostic result.

| Evaluation | No guidance | `AGENTS.md` | Observed difference | Contract status |
| --- | ---: | ---: | --- | --- |
| add-app-command | Pass | Pass | Same | Current |
| add-app-lifecycle-hook | Pass | Pass | Same | Current |
| add-cached-repository | Fail | Fail | Same | **Rerun** |
| add-database-transaction | Fail | Pass | Guidance passed | **Rerun** |
| add-event-subscriber | Fail | Pass | Guidance passed | **Rerun** |
| add-http-controller | Pass | Pass | Same | Current |
| add-job | Pass | Pass | Same | Current |
| add-mail-workflow | Pass | Pass | Same | Current |
| add-migration | Pass | Pass | Same | Current |
| add-named-app-route | Fail | Pass | Guidance passed | **Rerun** |
| add-named-cache | Fail | Fail | Same | **Rerun** |
| add-named-resource | Fail | Fail | Same | **Rerun** |
| add-named-storage | Fail | Fail | Same | **Rerun** |
| add-outbound-http-integration | Pass | Pass | Same | Current |
| add-resilient-job | Not measured | Not measured | Pending benchmark | Current |
| add-route-middleware | Pass | Pass | Same | Current |
| add-schedule | Fail | Pass | Guidance passed | **Rerun** |
| add-upload-workflow | Pass | Pass | Same | Current |
| add-validated-write-endpoint | Pass | Pass | Same | Current |
| build-json-api-feature | Pass | Pass | Same | Current |
| choose-storage-for-files | Fail | Pass | Guidance passed | Current |
| create-additional-app | Pass | Fail | Guidance regressed | **Rerun** |
| create-model | Pass | Pass | Same | Current |
| dispatch-event-followup-job | Pass | Pass | Same | Current |
| model-relationships | Pass | Pass | Same | Current |
| protect-route-with-auth | Pass | Pass | Same | Current |
| publish-domain-event | Fail | Fail | Same | **Rerun** |
| repair-wire-provider | Pass | Pass | Same | Current |
| runtime-observability | Fail | Fail | Same | **Rerun** |
| schedule-existing-job | Fail | Fail | Same | **Rerun** |
| serve-cacheable-image | Pass | Pass | Same | Current |
| unknown-framework-shape | Not measured | Not measured | Pending benchmark | Current |

## Measurement identity

| Property | Value |
| --- | --- |
| Measured | 2026-08-17 15:44–16:23 UTC |
| Evaluations | 30 promoted `core` evaluations |
| Attempts | 60 completed provider sessions; one paired trial per evaluation |
| Treatments | `none`, `agents` |
| Agent | Codex 0.147.0 |
| Model | `openai/gpt-5.6-sol` |
| Backend | `unconfined-local` |
| Atlas | `v0.3.2-0.20260817151453-4db01752305a` |
| GoForj binary | `sha256:fd354045e358841f727e9812964462bcb6de2bd0ea8f8aae0ca42bdf6d1c167a` |
| Agent binary | `sha256:7c16f9159aa8cf388d375cfd3150fed4dbc331c56cbdf16947fefbcdd8a5c43c` |
| Catalog | `sha256:df0a33b539e9e2cc4ac37f426d1647ff4e975b5bf2d85f8b04503928ea4a5ebf` |

The retained manifests authenticate the GoForj binary digest, but this run
reported the framework version as `(devel)` with no commit. Later runner changes
corrected that provenance gap. The next complete benchmark must use a runner
built by `make eval-runner` and record an exact GoForj revision.

## Extending this benchmark

Run the next adjacent paired comparison with a clean, revision-stamped runner. Start with the smoke tier before expanding to the complete promoted portfolio:

```sh
make eval-runner EVAL_RUNNER=/tmp/forj-eval

/tmp/forj-eval atlas:eval suite core \
  --tier smoke \
  --control agents \
  --treatment agents-skills \
  --model gpt-5.6-sol \
  --workers 4 \
  --trials 1 \
  --credential /secure/path/to/auth.json \
  --artifacts /secure/path/to/evaluation-artifacts \
  --artifact-key /secure/path/to/evaluation-artifact.key
```

Before publishing another scorecard:

1. authenticate every completed attempt with its external artifact key;
2. require the same number of attempts from each selected profile for every included evaluation;
3. exclude cancelled, incomplete, or mixed-revision attempts rather than
   silently changing the denominator;
4. record the exact agent, model, Atlas, GoForj, catalog, and backend identities;
5. keep framework outcome, workflow conformance, and diagnostic contract checks
   distinct; and
6. keep each comparison labeled with its own treatments and protocol instead of combining unlike attempts into one score; and
7. preserve the previous report in Git history.

Run the smoke tier first to catch treatment and verifier regressions without paying for the complete portfolio. Increase `--trials` when measuring treatment reliability. A single paired trial is useful for broad framework coverage, but it is not enough to estimate model variance or confidence intervals.

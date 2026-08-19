# Atlas evaluation benchmark

This page records the latest coverage-complete diagnostic snapshot of how a
fresh Codex agent builds GoForj applications under the four supported guidance
profiles. It keeps the aggregate score, every evaluation result, provenance,
and known limitations together so maintainers do not have to reconstruct the
benchmark from individual artifacts.

> [!IMPORTANT]
> These are authenticated local diagnostic results, not a release gate or a
> reliability guarantee. The `unconfined-local` backend can verify the resulting
> Project, but it cannot authoritatively prove command use, filesystem
> isolation, credential isolation, or process cleanup. Most matrix cells contain
> one attempt, so the percentages describe this measured snapshot rather than
> the probability that a future agent will succeed.

## Result

| Guidance profile | Passed | Failed | Evaluations | Pass rate |
| --- | ---: | ---: | ---: | ---: |
| No framework guidance | 25 | 7 | 32 | 78.1% |
| `AGENTS.md` | 29 | 3 | 32 | 90.6% |
| `AGENTS.md` and recommended skills | 31 | 1 | 32 | 96.9% |
| `AGENTS.md`, skills, and Atlas MCP | 31 | 1 | 32 | 96.9% |

The rolling snapshot observed a 12.5-point increase from no guidance to
`AGENTS.md`, another 6.3 points when recommended skills were present, and no
aggregate change when Atlas MCP was added on top of those surfaces. The complete
experience finished 6 evaluations, or 18.8 points, above no guidance.

Those are observed differences, not causal treatment estimates. Attempts were
fresh and profile-isolated, but the snapshot was assembled from exact revision
cohorts while verifier defects were corrected. A repeated paired protocol is
required to estimate treatment reliability.

The treatment ladder is cumulative:

| Profile | Project instructions | Recommended skills | Atlas MCP |
| --- | :---: | :---: | :---: |
| `none` | - | - | - |
| `agents` | yes | - | - |
| `agents-skills` | yes | yes | - |
| `atlas` | yes | yes | yes |

## Results by evaluation

`Pass` means the latest retained attempt completed with no failed core
structural, build, registration, or behavior check. For
`unknown-framework-shape`, a verifier-confirmed safe abstention is a pass.
Quality observations and workflow checks that require unavailable trusted
telemetry do not contribute to this diagnostic result.

| Evaluation | No guidance | `AGENTS.md` | Skills | Atlas |
| --- | :---: | :---: | :---: | :---: |
| `add-app-command` | Pass | Pass | Pass | Pass |
| `add-app-lifecycle-hook` | Pass | Pass | Pass | Pass |
| `add-cached-repository` | Pass | Pass | Pass | Pass |
| `add-database-transaction` | Pass | Fail | Pass | Pass |
| `add-event-subscriber` | Fail | Pass | Pass | Pass |
| `add-http-controller` | Pass | Pass | Pass | Pass |
| `add-job` | Fail | Pass | Pass | Pass |
| `add-mail-workflow` | Fail | Pass | Pass | Pass |
| `add-migration` | Pass | Pass | Pass | Pass |
| `add-named-app-route` | Pass | Pass | Pass | Pass |
| `add-named-cache` | Pass | Pass | Pass | Pass |
| `add-named-resource` | Pass | Pass | Pass | Pass |
| `add-named-storage` | Pass | Pass | Pass | Pass |
| `add-outbound-http-integration` | Pass | Pass | Pass | Pass |
| `add-resilient-job` | Fail | Pass | Pass | Pass |
| `add-route-middleware` | Fail | Pass | Pass | Pass |
| `add-schedule` | Pass | Pass | Pass | Pass |
| `add-upload-workflow` | Pass | Pass | Pass | Pass |
| `add-validated-write-endpoint` | Pass | Pass | Pass | Pass |
| `build-json-api-feature` | Pass | Pass | Pass | Pass |
| `choose-storage-for-files` | Pass | Pass | Pass | Pass |
| `create-additional-app` | Pass | Pass | Pass | Pass |
| `create-model` | Pass | Pass | Pass | Pass |
| `dispatch-event-followup-job` | Pass | Pass | Pass | Pass |
| `model-relationships` | Pass | Pass | Pass | Pass |
| `protect-route-with-auth` | Pass | Pass | Pass | Pass |
| `publish-domain-event` | Fail | Fail | Fail | Fail |
| `repair-wire-provider` | Fail | Pass | Pass | Pass |
| `runtime-observability` | Pass | Fail | Pass | Pass |
| `schedule-existing-job` | Pass | Pass | Pass | Pass |
| `serve-cacheable-image` | Pass | Pass | Pass | Pass |
| `unknown-framework-shape` | Pass | Pass | Pass | Pass |

The remaining guided failure is `publish-domain-event`. Agents generated the
event shape but did not consistently update an existing service test after the
service signature gained a request context, leaving the Project unable to
compile. That is retained as a product and guidance signal rather than rerun
until green.

## Quality signal

Thirteen evaluations require the candidate to add a focused test. This
observation is separate from the core result because an application can satisfy
its behavior contract while still omitting maintainable regression coverage.

| Guidance profile | Added focused test | Missing focused test | Measured |
| --- | ---: | ---: | ---: |
| No framework guidance | 9 | 4 | 13 |
| `AGENTS.md` | 11 | 2 | 13 |
| `AGENTS.md` and recommended skills | 12 | 1 | 13 |
| `AGENTS.md`, skills, and Atlas MCP | 11 | 2 | 13 |

The Atlas profile made 137 MCP calls across 17 of its 43 retained attempts.
The remaining 26 Atlas attempts completed without an MCP call. An `atlas`
profile therefore measures availability of the complete surface, not proof that
the agent used MCP or that MCP caused the result.

## Variance and contract correction

The retained corpus contains 207 authenticated provider attempts: 128 cells in
the latest matrix and 79 targeted repeats. Seven cells produced both a pass and
a failure under the same verifier release, including cached repositories,
database transactions, event subscribers, named resources, validated writes,
and model creation. This is direct evidence that one attempt per cell is broad
coverage, not a stable reliability estimate.

Live runs also found verifier defects. Corrections removed implementation-name
overfitting, aligned probes with current generator shapes, scoped source
ownership correctly, removed stale probe imports, and made the validated-write
probe initialize its fixture through `NewRepository()`. Corrected contracts
were rerun in affected profiles; invalid results remain retained for diagnosis
but do not replace newer cells in the table.

Two attempted v0.4.12 reruns were excluded before aggregation because their
manifests reported Atlas v0.4.1. A clean archive runner with a verified v0.4.12
module graph reran the pair successfully. The provenance check prevented a
commit-stamped but dependency-stale binary from entering the scorecard.

## Measurement identity

| Property | Value |
| --- | --- |
| Measured | 2026-08-19 13:07-18:35 UTC |
| Evaluations | 32 promoted evaluations |
| Matrix | 128 latest cells across `none`, `agents`, `agents-skills`, and `atlas` |
| Retained attempts | 207 authenticated provider sessions, including targeted repeats |
| Agent | Codex 0.147.0 |
| Model | `openai/gpt-5.6-sol` |
| Backend | `unconfined-local` |
| GoForj exact-main cohorts | `cdea5dee`, `9479bd8d`, `7dd45481`, `3bae5261`, `fb8c3db3` |
| Atlas verifier cohorts | v0.4.7, v0.4.8, v0.4.9, v0.4.11, v0.4.12 |
| Final corrected runner | GoForj `fb8c3db3`, Atlas v0.4.12, `sha256:cb771c3b237eeef1447ce2f548b44bdbdde90cec06e8d6ad49aa89f14641c62e` |
| Catalog | `sha256:df0a33b539e9e2cc4ac37f426d1647ff4e975b5bf2d85f8b04503928ea4a5ebf` |
| Agent binary | `sha256:7c16f9159aa8cf388d375cfd3150fed4dbc331c56cbdf16947fefbcdd8a5c43c` |

Every included attempt was re-authenticated with the external artifact key
before aggregation. The scorecard intentionally uses the latest corrected
chronological attempt for each evaluation and profile instead of averaging incompatible
verifier contracts. This makes it a rolling calibrated snapshot, not a single
revision benchmark.

The included cohort directories beneath the evaluation state root are:

```text
2026-08-19-v047-exact-main-none-agents-smoke
2026-08-19-v047-exact-main-agents-skills-smoke
2026-08-19-v047-exact-main-skills-atlas-smoke
2026-08-19-v048-exact-main-promoted-none-agents-v2
2026-08-19-v048-exact-main-promoted-skills-atlas
2026-08-19-v049-exact-main-affected-skills-atlas
2026-08-19-v049-exact-main-affected-wave2
2026-08-19-v049-exact-main-affected-wave3
2026-08-19-v0411-exact-main-final-wave1
2026-08-19-v0411-exact-main-final-wave2
2026-08-19-v0412-exact-main-validated-final2
```

`2026-08-19-v0412-exact-main-validated-final` is the excluded stale-module
cohort and must not be included in later aggregation.

## Capability coverage

The versioned Atlas catalog maps 40 of 57 named capabilities to promoted
evaluations. The other 17 remain planned rather than being represented by weak
source-token checks.

```sh
forj atlas:eval coverage
forj atlas:eval coverage --format markdown
```

`smoke` is the fastest release-critical portfolio, `core` adds representative
framework behavior, and `extended` adds specialized or expensive coverage.
Tier selection is cumulative. The capability count, not the number of prompts,
is the useful coverage measure.

## Updating the benchmark

Build a runner only from a clean GoForj checkout:

```sh
make eval-runner EVAL_RUNNER=/tmp/forj-eval

go version -m /tmp/forj-eval
```

Verify the expected Atlas module in that metadata before starting paid
sessions. Then run the smallest adjacent treatment comparison that answers the
question:

```sh
/tmp/forj-eval atlas:eval suite core \
  --tier smoke \
  --control agents \
  --treatment agents-skills \
  --model gpt-5.6-sol \
  --workers 4 \
  --trials 3 \
  --credential /secure/path/to/auth.json \
  --artifacts /secure/path/to/evaluation-artifacts \
  --artifact-key /secure/path/to/evaluation-artifact.key
```

Before replacing this page:

1. authenticate every included attempt with its external artifact key;
2. preserve evaluation, profile, and trial denominators;
3. record exact agent, model, Atlas, GoForj, catalog, and backend identities;
4. keep framework outcome, workflow conformance, and quality observations
   separate;
5. classify verifier defects and rerun only affected cells;
6. report mixed pass/fail repeats instead of selecting the green result; and
7. keep unavailable trusted evidence ineligible until an authoritative backend
   exists.

Do not run another complete matrix merely to improve the headline percentage.
Use repeated paired trials to answer a specific reliability, regression, or
guidance-attribution question, and preserve previous scorecards in Git history.

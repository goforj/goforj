# Atlas live evaluations

The [Atlas evaluation benchmark](atlas-evaluation-benchmark.md) records the
latest complete per-evaluation comparison, its measurement identity, and any
results that require remeasurement after contract changes.

Live evaluations authenticate the GoForj source identity in their signed `run.json` before starting a provider session. Build an evaluation runner from a checkout with:

```sh
make eval-runner EVAL_RUNNER=/tmp/forj-eval
```

The command requires a clean checkout, exports the selected commit into a private source snapshot, and builds only that snapshot with a full Git revision and explicit clean-state stamp. This prevents concurrent worktree changes from entering a binary attributed to an earlier revision. Use that binary for `atlas:eval` commands when evaluating unpublished changes. Other builds are accepted only when Go build metadata provides an immutable module version or a full revision with an explicit dirty state; incomplete source identities are rejected before a provider session starts.

## Resuming the Work

The evaluation system spans Atlas and GoForj. Atlas owns prompts, treatments, workflow expectations, verifiers, behavior probes, artifacts, and the capability catalog. GoForj owns rendered scenario Projects, native guidance materialization, the command surface, and executable calibration against real framework output.

Read these sources in order before changing the portfolio:

1. `docs/designs/atlas-live-agent-evaluation-design.md` for the normative architecture and trust boundaries.
2. Atlas's `docs/live-agent-evaluation-plan.md` for implementation ownership and the maintainer checklist.
3. [The benchmark](atlas-evaluation-benchmark.md) for the latest completed provider measurement and its exact identity.
4. The current coverage report for promoted behavior and planned gaps:

```sh
forj atlas:eval coverage
forj atlas:eval coverage --format markdown
```

Current shipping checkpoint, recorded 2026-08-23: Atlas `v0.4.13` points to
`5c9fe023`; GoForj `main` at `d40562ee` selects it and contains the matching
data-resource scenario and verifier calibration. The latest GoForj tag remains
`v0.26.1` and predates that integration. Final release-qualified evidence must
therefore use a later GoForj tag rather than describing `d40562ee` as already
released.

At the `d40562ee` checkpoint, using Atlas `v0.4.13`, the catalog maps 41 of 57 named capabilities. Sixteen gaps remain planned deliberately. Run the coverage command for the current count rather than copying this checkpoint into another document. Do not infer coverage from the number of scenarios or evaluation manifests, and do not close a gap with a verifier that merely searches for source tokens.

## Program Operating Model

The benchmark measures whether a fresh agent can turn a natural request into a correct, maintainable GoForj application. The resulting application is the primary result. Generator use, skill use, MCP calls, and agent explanations are attribution evidence; none can turn broken application behavior into a pass.

GoForj owns the product surfaces the agent must understand and the Projects used to measure them:

- `AGENTS.md` establishes durable framework rules even when no optional integration is installed;
- native skills teach focused workflows and judgment that would overwhelm baseline instructions;
- Atlas MCP supplies contextual discovery and inspection when the agent needs more than static guidance;
- generators and commands create code and update protected registration points;
- generated conventions, diagnostics, documentation, and inspection commands help the agent recover when the first action is incomplete; and
- scenario Projects provide realistic starting states without leaking the expected solution.

Prefer application-shaped evaluations that cross related surfaces. A scenario should prove the user-visible outcome, important framework boundaries, and any workflow whose use materially protects registration or generated code. Do not add shallow tests simply to increase the evaluation count. Keep unsupported behavior visible as a planned capability until an economical verifier can observe it.

When a run fails, classify the smallest responsible surface before changing anything: guidance, skill, MCP discovery, command or generator behavior, generated code, framework diagnostics, documentation, framework API, or verifier. Improve that surface and rerun the adjacent treatment comparison that can attribute the effect. This prevents every agent failure from becoming more prompt text.

Use the evaluation program at three cadences:

| Cadence | Evidence | Purpose |
| --- | --- | --- |
| Pull request | Fast deterministic runner, verifier unit, and rendered-Project checks | Reject framework regressions without provider cost. |
| Evaluation-surface pull request | Golden, alternate, mutant, and hidden behavior calibration | Reject contract and harness regressions where those checks provide direct value. |
| Provider smoke | Fresh sessions over the smallest release-critical tier | Detect tool-discovery and agent-behavior regressions early. |
| Release benchmark | Repeated paired trials over the promoted portfolio | Publish reliability, per-capability outcomes, failure classes, and treatment attribution. |

The `verifier calibration` workflow runs when a pull request changes Atlas integration, scenario, template, or module inputs. Maintainers can also start it manually. The ordinary framework integration profile deliberately excludes these exhaustive fixture permutations:

```sh
forj test:integration framework --framework-profile=verifiercalibration --shard=1/24
```

Fixture calibration proves that reviewed examples are classified correctly. Provider smoke and release benchmarks remain the evidence that an agent can discover and complete the task.

A report should preserve the denominator and show more than one aggregate percentage. Retain per-evaluation results, capability coverage, failure taxonomy, exact Atlas, GoForj, model, agent, catalog, and backend identities, plus the backend's trust limitations. If corrected contracts or new evaluations have not been rerun, mark them unmeasured rather than carrying an old pass forward.

## Treatment Ladder

The guidance profiles isolate different parts of the developer experience:

| Profile | `AGENTS.md` | Recommended skills | Atlas MCP |
| --- | :---: | :---: | :---: |
| `none` | — | — | — |
| `agents` | yes | — | — |
| `agents-skills` | yes | yes | — |
| `atlas` | yes | yes | yes |

The published scorecard includes all four profiles for the 32 evaluations promoted at its recorded revision. Atlas `v0.4.13` and GoForj `d40562ee` add `create-data-resource` as a thirty-third promoted evaluation. One fresh `atlas`-profile attempt passed its outcome checks, but it was not a treatment comparison and its temporary artifacts were not retained. Keep it outside the published matrix until a rerun retains authenticated evidence. Use adjacent comparisons when a new measurement is intended to attribute a change to one guidance surface:

```sh
/tmp/forj-eval atlas:eval suite core \
  --tier smoke \
  --control agents \
  --treatment agents-skills \
  --workers 4 \
  --trials 1 \
  --model gpt-5.6-sol \
  --credential /secure/path/to/auth.json \
  --artifacts /secure/path/to/evaluation-artifacts \
  --artifact-key /secure/path/to/evaluation-artifact.key
```

Run `none` against `atlas` only to measure the combined experience. The CLI prints the number of provider sessions, maximum wall time, and scratch estimate before it loads provider authority; review those values before continuing. The current matrix is a coverage-complete diagnostic snapshot assembled from exact revision cohorts and targeted reruns. It is not a confidence interval or a claim that one attempt predicts future reliability.

## Adding an Evaluation

Prefer one realistic task that crosses related surfaces over several narrow tests. The retry-safe report job is the reference shape: it measures typed generator use, an ID-only payload, retry and timeout policy, current-state repository lookup, deterministic storage, event follow-up registration, and cancellation in one coherent workflow.

1. Add or extend the reviewed target Project in `internal/scenarios/specs/`.
2. Add the natural prompt and manifest in Atlas without revealing the expected generator invocation.
3. Map the evaluation to named capabilities in Atlas's `eval/coverage.yaml`.
4. Add structural contracts only for durable framework and application boundaries.
5. Add a supervisor-owned behavior probe when the outcome cannot be established from compilation and registration.
6. Calibrate the golden Project and at least one compiling semantic mutant in `internal/forj/atlaseval/preparer_integration_test.go`.
7. Run at least one fresh provider attempt after deterministic calibration. A golden Project proves the verifier accepts reviewed code; it does not prove an agent can discover the workflow.
8. Run all relevant modules with `GOWORK=off`, using a released Atlas version and no local replacement for final evidence.

Render scenario Projects only under `/tmp`. Keep unrelated worktree files out of commits, and validate the root, `integration`, and `tools/renderwarm` modules independently when their dependency boundary is affected.

## What Remains

The highest-value next implementation is the authoritative container or VM backend and its negative isolation suite. It must keep signing authority and sibling trials outside the candidate boundary, capture commands through a supervisor-owned stream, enforce command and process limits online, and prove cleanup. Until that exists, local results remain diagnostic and unavailable trusted command, isolation, and cleanup evidence must remain ineligible.

After that trust boundary exists, the next paid measurement should answer a specific adjacent-treatment question with repeated paired trials, beginning with the smoke tier. Do not pay for another complete matrix merely to replace stochastic failures with green cells. Publish a new scorecard only when its protocol, denominator, retained authenticated artifacts, and revision identities are explicit. The newly promoted `create-data-resource` evaluation also needs retained evidence before it joins that scorecard.

Some capability gaps require new infrastructure rather than more source contracts. Browser-observed frontend loading and flicker, deployment rollback, graceful multi-runtime shutdown, and production driver swaps should remain planned until their verifier can observe the real behavior economically and deterministically.

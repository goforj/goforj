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

At this checkpoint, the catalog maps 40 of 57 named capabilities. Seventeen gaps remain planned deliberately. Run the coverage command for the current count rather than copying this checkpoint into another document. Do not infer coverage from the number of scenarios or evaluation manifests, and do not close a gap with a verifier that merely searches for source tokens.

## Treatment Ladder

The guidance profiles isolate different parts of the developer experience:

| Profile | `AGENTS.md` | Recommended skills | Atlas MCP |
| --- | :---: | :---: | :---: |
| `none` | — | — | — |
| `agents` | yes | — | — |
| `agents-skills` | yes | yes | — |
| `atlas` | yes | yes | yes |

The published 60-session scorecard measured `none` against `agents`. It says nothing yet about the incremental value of native skills or Atlas MCP. Measure those surfaces through adjacent comparisons before making a claim about them:

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

Repeat with `agents-skills` and `atlas` after the first adjacent comparison is complete. Run `none` against `atlas` only to measure the combined experience. The CLI prints the number of provider sessions, maximum wall time, and scratch estimate before it loads provider authority; review those values before continuing.

## Adding an Evaluation

Prefer one realistic task that crosses related surfaces over several narrow tests. The retry-safe report job is the reference shape: it measures typed generator use, an ID-only payload, retry and timeout policy, current-state repository lookup, deterministic storage, event follow-up registration, and cancellation in one coherent workflow.

1. Add or extend the reviewed target Project in `internal/scenarios/specs/`.
2. Add the natural prompt and manifest in Atlas without revealing the expected generator invocation.
3. Map the evaluation to named capabilities in Atlas's `eval/coverage.yaml`.
4. Add structural contracts only for durable framework and application boundaries.
5. Add a supervisor-owned behavior probe when the outcome cannot be established from compilation and registration.
6. Calibrate the golden Project and at least one compiling semantic mutant in `internal/forj/atlaseval/preparer_integration_test.go`.
7. Run all relevant modules with `GOWORK=off`, using an exact remote Atlas pseudo-version or release and no local replacement.

Render scenario Projects only under `/tmp`. Keep unrelated worktree files out of commits, and validate the root, `integration`, and `tools/renderwarm` modules independently when their dependency boundary is affected.

## What Remains

The next measurement work is the smoke-tier `agents` versus `agents-skills` comparison, followed by `agents-skills` versus `atlas`. The next architectural phase is an authoritative container or VM backend with negative isolation tests. Until that backend exists, local results remain diagnostic: unavailable trusted command, isolation, and cleanup evidence must remain ineligible rather than being promoted into a pass.

Some capability gaps require new infrastructure rather than more source contracts. Browser-observed frontend loading and flicker, deployment rollback, graceful multi-runtime shutdown, and production driver swaps should remain planned until their verifier can observe the real behavior economically and deterministically.

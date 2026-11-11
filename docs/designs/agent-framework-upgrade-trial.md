# Agent Framework Upgrade Trial

Status: one independent pilot, partial upgrade; full acceptance remains blocked.

## Question and result

Can a fresh agent use the framework plan and target snapshot to upgrade a real,
customized application while preserving its workflows?

The first pilot produced a substantial, reviewable reconciliation. Independent
validation found no lost baseline test passes or changes to existing tests and
migration SQL. The candidate adopted new framework behavior and built successfully.
The actor reported a partial upgrade, and existing application test failures
prevent full acceptance. Retaining an application's infrastructure policy is not
itself evidence of an incomplete framework upgrade. One pilot cannot establish
reliability across applications or repeated agent runs.

The experiment also reproduced a tooling sequence that can corrupt an upgrade's
dependency state. That finding needs resolution before treating the workflow as
a dependable unattended upgrade mechanism.

## Trial boundaries

- Source: committed Ditracker revision `6fe4b4842a08b131ba6723d5ce16fa3dcc431e49`.
- Framework: `2ca6df1a0143457a475e19258f233b5447150749`, snapshot protocol v3.
- Target binary SHA-256: `ff4db9f03005a975f39692e5ee9fe2db8d35aa29d6ed08cfa35063cb6a7ec105`.
- Baseline provenance: unknown. This pilot tests unversioned inspection, not an
  authenticated release-to-release upgrade.
- The actor was a separate Codex session with no parent conversation or prescribed
  reconciliation patch. It received the full 636-record plan, target snapshot,
  target binary, and a disposable Git copy of the application.
- Private environment layers and symlinks were excluded from the committed app
  archive. The original application, its uncommitted changes, and production
  services were outside the actor's authorized workspace.
- Existing tests and migration SQL could not be changed. New tests were allowed.
  Fake frontend placeholders, owner dev/deploy commands, pushes, and publication
  were prohibited.
- One environment clarification permitted a scratch render workspace. Before that
  clarification, the task's write boundary prevented materializing raw assets.
  No reconciliation recipe was supplied. This intervention is part of the result.
- After the actor stopped, the evaluator ran independent checks. A second agent
  reviewed the source diff without modifying it or running competing builds.

The pilot's original target binary was checked against its recorded digest before
and after use. The reusable preparation command additionally freezes an executable
copy per trial.

The retained trial and candidate are local disposable artifacts. This PR does
not vendor application source, generated baselines, or the candidate patch.

## Independent observations

| Check | Original application copy | Reconciled candidate |
| --- | --- | --- |
| Unit test/subtest passes | 1,453 | 1,825, with every baseline passing identity retained |
| Auth integration test/subtest passes | 103 | 103, no skipped auth tests |
| Full Go suite | Blocked by missing frontend output and two stale constructor calls | Blocked by the same two constructor calls |
| Trace correlation probe | Both event and HTTP-access cases fail behaviorally | Both cases pass, plus their parent test |
| Real app binary | Missing frontend embed output | Builds with actual frontend output |
| Build with exact Go 1.26.1 toolchain | Not required to establish the missing-asset baseline | Passes; module minimum remains 1.26.1 |
| Frontend Node tests | 112 of 113 pass | Same 112 pass and same existing assertion fails |
| Existing tests and migration SQL | Frozen | Byte-for-byte unchanged |
| Source changes caused by independent Go validation | None | None |

The trace probe uses existing public logger APIs and a supplied OpenTelemetry
span context. It verifies structured trace/span identity for both a normal event
and an HTTP access log. The same probe passes on a pristine target render, so it
has both a negative control and a positive control. Go overlays keep the probe
outside app source; canonical paths and required test identities prevent a
silent zero-test invocation from counting as success.

The candidate's unit run has three additional skipped subprocess-helper entry
points from the target's privacy tests. Their parent privacy tests pass. These
are recorded for review, never counted as passes or silently waived by the
conservative evaluator.

The read-only review found no actionable app-behavior regression. Auth, Discord,
access, app routes/commands/schedules, relevant app injectors, resource-driver
production code, database connection policy, project adapters, and `.goforj.yml`
match the baseline. The generated graph retains the app's runtime services and
named resources. This is source-preservation evidence, not live-service or
production-rollout validation.

## Scope and remaining decisions

The actor supplied exactly one decision for each of the 636 plan records:
538 preserved, 53 applied, 26 adapted, 17 regenerated, and 2 deferred. These are
the actor's original labels; the MariaDB deferral classification is corrected below.

The candidate includes target logging/export, tracing, HTTP handling, maintenance,
runtime deadlines, lifecycle cleanup, and Lighthouse assets. Independent inventory
found no missing managed artifacts or derived contracts. Eighty-one managed source
files still differ from defaults; those include preserved tests and intentional
app adaptations. One raw schedule template retains the app's existing schedule
registration contract. A default-byte mismatch alone does not establish failure.

Two existing Go test packages have stale application constructor calls. The
frontend has an existing page-persistence assertion failure. Neither was hidden
by editing tests. The application version marker was not advanced to imply
complete adoption.

The actor classified retaining MariaDB 10.11.6 as deferred work because the target
scaffold already uses 12.3.3 and automatic-upgrade configuration. That classification
was too broad: we did not establish that this database change is required by the
framework code changes. The app's image and configuration remained unchanged, and
no database was touched. Retaining them is an app infrastructure policy choice;
matching every scaffold default is not an acceptance requirement. An actual server
upgrade would need its own operational validation.

## Reproduced tooling failure

The evaluator reproduced this sequence in another disposable candidate copy:

1. With compatible pins intact, resource generation, including forced queue-manager
   regeneration, preserves Go 1.26.1 and queue/redisqueue v0.2.1.
2. Remove private env layers and the built app binary, then invoke the target
   binary's top-level `--help` from that copy.
3. Help discovery attempts an app build. It rewrites resource source and prunes
   driver requirements using absent-env defaults, despite being a help request.
4. Restore nonsecret driver selections and run resource generation again.
5. The absent Redis driver requirement resolves to v0.3.0. Queue also moves to
   v0.3.0, whose module metadata requires Go 1.27.0. The app's Go directive also
   becomes 1.27.0. Generated queue source
   still supplies `queue.Config.Observer`; compiling it fails because that
   version lacks the field.

The dependency problem is sequence-dependent. It was not reproduced on the final
candidate while its compatible explicit pins remained intact. The actor's initial
report overstated that distinction; a post-trial audit withdrew the broader claim.
The evaluator then established the precise help/pruning/regeneration sequence
with before/after module files and a failing queue compilation.

The host Go toolchain is 1.27.0. A newly initialized scratch project's Go 1.27.0
directive alone is not evidence that dependencies forced a minimum-version
increase in an existing app. The final candidate builds with exact Go 1.26.1.

These findings concern existing help/build/generation behavior. This inspection
PR does not silently change those production workflows.

## Reproduce a fresh trial

Build the desired renderer, then prepare a frozen application copy:

```sh
go build -o /tmp/forj-agent-target ./cmd/forj
python3 scripts/spike-agent-upgrade.py prepare \
  --source ~/code/ditracker \
  --revision 6fe4b4842a08b131ba6723d5ce16fa3dcc431e49 \
  --forj /tmp/forj-agent-target
```

Give the printed `TASK.md` to a fresh agent session. Do not provide a successful
candidate or a prescribed patch. Once that session has stopped, evaluate its copy:

```sh
python3 scripts/spike-agent-upgrade.py evaluate /tmp/goforj-agent-upgrade-CASE
```

Preparation freezes a private executable copy for the trial. Evaluation verifies
that executable and the input packet before running tests. It rejects explicit
failed-test or package-failure events even if a test process incorrectly exits
zero, and terminates validation process groups on timeout. A real child-process
regression test checks that a timed-out descendant cannot write into a later
phase. These local controls are not an executable sandbox.

The driver retains source/mode fingerprints, target-packet hashes, complete Go
logs, test identities, protected-file changes, residual artifacts, and a summary
outside the application. The source revision is resolved before archiving; an
advancing source checkout cannot silently change the case. Environment values
and symlink targets are not fingerprinted. Node dependency directories and Git
metadata are excluded from source inventory.

Status distinguishes a blocked case, observed regressions, and source changes
during validation. Even `measured-gates-pass` requires manual review of scope,
remaining differences, skipped tests, and operational decisions. The driver is a
Ditracker pilot harness, not a generic production upgrade executor. Its standard
checks cover Go and auth; this pilot also independently compared the frontend
Node suites and verified the exact minimum-toolchain build.

## What this changes in the design

The next proof needs the following boundaries before repeated acceptance runs:

1. Safe CLI discovery and target materialization must preserve the candidate and
   provide source, assets, and dependency intent in disposable workspaces.
2. Generator execution must retain framework-compatible pins when adding or
   restoring drivers. App resource selections must be captured without secrets;
   default environment reconstruction is insufficient.
3. Plans must distinguish app policy from required operational transitions and
   record unresolved decisions. Partial work must not advance adoption metadata.
4. Maintainers need behavioral acceptance checks for the intended upstream changes,
   alongside immutable app regression tests. A green compile alone is insufficient.
5. Repeat the trial across apps, configurations, authenticated baselines, and fresh
   sessions, including controlled bad reconciliations. Measure complete upgrades,
   safe refusals, and regressions separately.

This pilot supports continued development of agent-assisted reconciliation. It
establishes a way to falsify an upgrade claim and identifies concrete obstacles;
it does not establish reliable arbitrary-app upgrading.

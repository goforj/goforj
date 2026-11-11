#!/usr/bin/env python3
"""Prepare and independently evaluate a disposable, unversioned Ditracker upgrade trial."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import runpy
import shutil
import signal
import subprocess
import tempfile


HERE = Path(__file__).resolve().parent
HELPERS = runpy.run_path(str(HERE / "spike-framework-upgrade.py"))
HOLDOUT = "TestAgentUpgradeContextCorrelation"
HOLDOUT_TESTS = {HOLDOUT, HOLDOUT + "/event", HOLDOUT + "/http-access"}


def digest(path):
    """Fingerprint a local input without copying it into an application repository."""
    fingerprint = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1 << 20), b""):
            fingerprint.update(block)
    return fingerprint.hexdigest()


def environment():
    """Keep provider credentials and production application settings out of test processes."""
    result = {key: value for key, value in os.environ.items() if key in ("PATH", "HOME", "GOCACHE", "GOMODCACHE", "SYSTEMROOT")}
    result.update(TMPDIR="/tmp", GOWORK="off", APP_ENV="testing", DB_DRIVER="sqlite",
                  API_JWT_SECRET_KEY="public-agent-upgrade-test-key-0123456789")
    result.setdefault("GOCACHE", "/tmp/gocache")
    return result


def inventory(root):
    """Record source bytes and modes while avoiding symlink targets and generated cache directories."""
    files = {}
    for directory, directories, names in os.walk(root, followlinks=False):
        for name in directories:
            if name.lower().startswith(".env"):
                files[str((Path(directory) / name).relative_to(root))] = {"private": True}
        directories[:] = sorted(name for name in directories if name not in (".git", "node_modules") and not name.lower().startswith(".env"))
        for name in sorted(names + [name for name in directories if (Path(directory) / name).is_symlink()]):
            path = Path(directory) / name
            relative = path.relative_to(root)
            if name.lower().startswith(".env"):
                files[str(relative)] = {"private": True}
            elif path.is_symlink():
                files[str(relative)] = {"symlink": os.readlink(path)}
            elif path.is_file():
                files[str(relative)] = {"sha256": digest(path), "mode": path.stat().st_mode & 0o777}
    return files


def prepare(source, renderer, revision="HEAD"):
    """Freeze an app and target packet before a separate session receives the upgrade task."""
    root = Path(tempfile.mkdtemp(prefix="goforj-agent-upgrade-", dir="/tmp")).resolve()
    baseline, app, packet = root / "baseline", root / "app", root / "packet"
    baseline.mkdir()
    packet.mkdir()
    revision = subprocess.check_output(["git", "rev-parse", "--verify", "--end-of-options", revision + "^{commit}"], cwd=source, text=True).strip()
    binary_digest = digest(renderer)
    frozen_renderer = packet / "forj"
    shutil.copy2(renderer, frozen_renderer)
    frozen_renderer.chmod(0o500)
    if digest(frozen_renderer) != binary_digest or digest(renderer) != binary_digest:
        raise RuntimeError("target binary changed while being frozen")
    HELPERS["copy_committed_app"](source, baseline, revision)
    shutil.copytree(baseline, app)
    for command in (["git", "init", "-q"], ["git", "add", "-f", "--", "."],
                    ["git", "commit", "-q", "-m", "chore: establish disposable upgrade trial baseline"]):
        subprocess.run(command, cwd=app, check=True)
    for name, flags in (("plan.json", ["--plan", "--json"]), ("target-snapshot.json", ["--snapshot"])):
        result = subprocess.run([str(frozen_renderer), "render", *flags], cwd=app, env=environment(), capture_output=True, text=True, check=True, timeout=60)
        (packet / name).write_text(result.stdout)
    if digest(frozen_renderer) != binary_digest:
        raise RuntimeError("frozen renderer changed during preparation")
    task = (HERE / "fixtures/agent-upgrade-task.md").read_text()
    (packet / "TASK.md").write_text(task.replace("{{WORKSPACE}}", str(root)).replace("{{RENDERER}}", str(frozen_renderer)))
    metadata = {"workspace": str(root), "source_revision": revision,
                "binary": str(frozen_renderer), "source_binary": str(renderer), "binary_sha256": binary_digest, "baseline_provenance": "unknown", "files": inventory(baseline),
                "packet_sha256": {name: digest(packet / name) for name in ("TASK.md", "plan.json", "target-snapshot.json")}}
    (root / "trial.json").write_text(json.dumps(metadata, indent=2) + "\n")
    return {"workspace": str(root), "task": str(packet / "TASK.md")}


def parse_tests(path, exit_code):
    """Keep test identity distinct from aggregate counts, package failures, and zero-test invocations."""
    passed, failed, skipped, build_failed, packages = set(), set(), set(), set(), set()
    for line in path.read_text().splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(event, dict):
            continue
        package = event.get("Package") or event.get("ImportPath") or "unknown"
        action = event.get("Action")
        if action in ("start", "pass", "fail", "skip", "build-fail") and package != "unknown":
            packages.add(package)
        identity = package + "::" + event["Test"] if event.get("Test") else None
        if identity and action in ("pass", "fail", "skip"):
            {"pass": passed, "fail": failed, "skip": skipped}[action].add(identity)
        elif action in ("fail", "build-fail"):
            build_failed.add(package)
    return {"exit_code": exit_code, "passed": sorted(passed), "failed": sorted(failed),
            "skipped": sorted(skipped), "package_failures": sorted(build_failed), "packages": sorted(packages)}


def run_go(app, evaluation, name, arguments, timeout=300):
    """Retain complete local evidence while treating timeouts as unsuccessful validation."""
    log = evaluation / (name + ".json")
    with log.open("w") as output:
        process = subprocess.Popen(["go", *arguments], cwd=app, env=environment(), stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
        try:
            exit_code = process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            exit_code = 124
        finally:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait()
    return parse_tests(log, exit_code)


def protected_changes(before, after):
    """Identify test weakening and migration-history changes independently of the actor's report."""
    return sorted(name for name in before if (
        name.endswith("_test.go") or ".test." in name or ".spec." in name or
        (name.startswith("migrations/") and name.endswith(".sql"))
    ) and before[name] != after.get(name))


def holdout_overlay(app, evaluation, name):
    """Canonicalize Go overlay paths so macOS /tmp aliases cannot silently omit the tests."""
    virtual = app / "internal/logger/agent_upgrade_holdout_test.go"
    if virtual.exists() or virtual.is_symlink():
        raise RuntimeError("independent probe would replace an existing app test")
    module_source = (app / "go.mod").read_text()
    module = re.search(r'(?m)^module\s+"?([a-zA-Z0-9._~/-]+)"?\s*$', module_source)
    if module is None:
        raise RuntimeError("unsupported module declaration for independent probe")
    probe = evaluation / (name + "_probe_test.go")
    probe.write_text((HERE / "fixtures/agent-upgrade-context_test.go.tmpl").read_text().replace("{{MODULE}}", module[1]))
    overlay = evaluation / (name + "-overlay.json")
    overlay.write_text(json.dumps({"Replace": {str(virtual.resolve()): str(probe.resolve())}}))
    return overlay


def coverage_losses(results):
    """Detect disappearing failed or skipped tests and packages hidden by new module boundaries."""
    losses = {}
    for suite in ("unit", "auth"):
        baseline, candidate = results["baseline"][suite], results["app"][suite]
        required = set(baseline["passed"]) | set(baseline["failed"])
        missing_tests = required - (set(candidate["passed"]) | set(candidate["failed"]))
        missing_tests |= set(baseline["skipped"]) - (set(candidate["passed"]) | set(candidate["skipped"]))
        losses[suite] = {"tests": sorted(missing_tests), "packages": sorted(set(baseline["packages"]) - set(candidate["packages"]))}
    return losses


def assess_results(results, protected):
    """Refuse green claims from empty runs, lost behaviors, skipped coverage, or an ineffective negative control."""
    regressions = {suite: sorted(set(results["baseline"][suite]["passed"]) - set(results["app"][suite]["passed"])) for suite in ("unit", "auth")}
    candidate = results["app"]
    holdout_names = {identity.split("::", 1)[1] for identity in candidate["holdout"]["passed"]}
    baseline_holdout = {identity.split("::", 1)[1] for identity in results["baseline"]["holdout"]["failed"]}
    gates_pass = not protected and not any(regressions.values()) and all(value["exit_code"] == 0 for value in candidate.values())
    gates_pass = gates_pass and all(not value["failed"] and not value["package_failures"] for value in candidate.values())
    gates_pass = gates_pass and HOLDOUT_TESTS.issubset(holdout_names) and HOLDOUT_TESTS.issubset(baseline_holdout)
    gates_pass = gates_pass and all(results[side][suite]["passed"] for side in ("baseline", "app") for suite in ("unit", "auth"))
    gates_pass = gates_pass and all(set(candidate[suite]["skipped"]).issubset(results["baseline"][suite]["skipped"]) for suite in ("unit", "auth"))
    gates_pass = gates_pass and not any(value["tests"] or value["packages"] for value in coverage_losses(results).values())
    return gates_pass, regressions


def trial_status(accepted, protected, regressions, validation_changes, losses):
    """Distinguish observed regressions from incomplete acceptance and validation side effects."""
    if validation_changes:
        return "invalidated"
    if protected or any(regressions.values()) or any(value["tests"] or value["packages"] for value in losses.values()):
        return "regressed"
    return "measured-gates-pass" if accepted else "blocked"


def residual_artifacts(snapshot, files):
    """Expose unadopted target artifacts without pretending app-specific source must equal a default."""
    result = {"raw_asset_differences": [], "source_differences": [], "missing_managed": [], "missing_derived": []}
    for artifact in snapshot["artifacts"]:
        name, ownership = artifact["path"], artifact["ownership"]
        if ownership == "app-derived" and name not in files:
            # A frontend dist contract is a directory rather than one fingerprinted file.
            if not any(path.startswith(name + "/") for path in files):
                result["missing_derived"].append(name)
        if ownership not in ("framework-template", "framework-asset"):
            continue
        if name not in files:
            result["missing_managed"].append(name)
            continue
        expected = artifact.get("digest") or hashlib.sha256(artifact.get("source", "").encode()).hexdigest()
        if files[name].get("sha256") != expected:
            category = "raw_asset_differences" if ownership == "framework-asset" else "source_differences"
            result[category].append(name)
    return {key: sorted(value) for key, value in result.items()}


def app_build_arguments(output):
    """Require an executable main package instead of accepting a successful library-archive build."""
    return ["build", "-buildmode=exe", "-mod=readonly", "-o", str(output), "./cmd/app"]


def evaluate(root):
    """Run after the actor stops, preserving baseline failures and refusing to equate passing gates with complete coverage."""
    metadata = json.loads((root / "trial.json").read_text())
    before = metadata["files"]
    if digest(Path(metadata["binary"])) != metadata["binary_sha256"]:
        raise RuntimeError("frozen renderer changed; the trial requires a new preparation")
    for name, expected in metadata["packet_sha256"].items():
        if digest(root / "packet" / name) != expected:
            raise RuntimeError("actor input packet changed; the trial requires a new preparation")
    if inventory(root / "baseline") != before:
        raise RuntimeError("baseline source changed; the trial cannot be independently compared")
    evaluation = root / "evaluation"
    evaluation.mkdir(exist_ok=True)
    results = {}
    candidate_before_validation = inventory(root / "app")
    for name in ("baseline", "app"):
        app = root / name
        overlay = holdout_overlay(app, evaluation, name)
        results[name] = {
            "unit": run_go(app, evaluation, name + "-unit", ["test", "-p", "1", "-mod=readonly", "-json", "./...", "-count=1"]),
            "auth": run_go(app, evaluation, name + "-auth", ["test", "-mod=readonly", "-json", "-tags=integration", "./internal/auth", "-count=1"]),
            "holdout": run_go(app, evaluation, name + "-holdout", ["test", "-mod=readonly", "-json", "-overlay", str(overlay), "./internal/logger", "-run", "^" + HOLDOUT + "$", "-count=1"]),
            "build": run_go(app, evaluation, name + "-build", app_build_arguments(evaluation / (name + "-binary"))),
        }
    if inventory(root / "baseline") != before:
        raise RuntimeError("baseline validation changed source; the trial requires investigation")
    after = inventory(root / "app")
    validation_changes = sorted(name for name in set(candidate_before_validation) | set(after) if candidate_before_validation.get(name) != after.get(name))
    changed = sorted(name for name in set(before) | set(after) if before.get(name) != after.get(name))
    protected = protected_changes(before, after)
    gates_pass, regressions = assess_results(results, protected)
    losses = coverage_losses(results)
    gates_pass = gates_pass and not validation_changes
    summary = {"status": trial_status(gates_pass, protected, regressions, validation_changes, losses), "manual_upgrade_review_required": True,
               "source_revision": metadata["source_revision"], "binary_sha256": metadata["binary_sha256"],
               "changed_files": changed, "protected_changes": protected, "validation_changes": validation_changes,
               "lost_baseline_test_passes": regressions, "lost_baseline_coverage": losses, "results": results,
               "residual_artifacts": residual_artifacts(json.loads((root / "packet/target-snapshot.json").read_text()), after)}
    (evaluation / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    return {"workspace": str(root), "status": summary["status"], "changed_files": len(changed), "protected_changes": protected,
            "lost_baseline_test_passes": {name: len(values) for name, values in regressions.items()}, "lost_baseline_coverage": losses, "summary": str(evaluation / "summary.json")}


def main():
    """Separate preparation from evaluation so no orchestrating agent edits the candidate during the trial."""
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    creation = commands.add_parser("prepare")
    creation.add_argument("--source", required=True, type=Path)
    creation.add_argument("--forj", required=True, type=Path)
    creation.add_argument("--revision", default="HEAD")
    validation = commands.add_parser("evaluate")
    validation.add_argument("workspace", type=Path)
    arguments = parser.parse_args()
    if arguments.command == "prepare":
        result = prepare(arguments.source.expanduser().resolve(strict=True), arguments.forj.expanduser().resolve(strict=True), arguments.revision)
    else:
        result = evaluate(arguments.workspace.expanduser().resolve(strict=True))
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()

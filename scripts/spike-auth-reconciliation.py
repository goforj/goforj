#!/usr/bin/env python3
"""Probe a real auth fix and a destructive workflow replacement in a disposable Ditracker copy."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import runpy
import subprocess
import tempfile


REGRESSION = '''//go:build integration

package auth

import (
    "errors"
    "testing"
    "time"
)

// TestUpgradeProbeOAuthStateSingleUse preserves one-time and expiry checks across application-specific OAuth flows.
func TestUpgradeProbeOAuthStateSingleUse(t *testing.T) {
    for _, expired := range []bool{false, true} {
        name := "replayed"
        if expired { name = "expired" }
        t.Run(name, func(t *testing.T) {
            h := newAuthIntegrationHarness(t)
            now := time.Now().UTC()
            expiry := now.Add(time.Minute)
            if expired { expiry = now.Add(-time.Minute) }
            state := &OAuthStateToken{Provider: "fake", State: name, CodeVerifier: "verifier", ExpiresAt: expiry}
            if err := h.oauthStates.Save(state); err != nil { t.Fatal(err) }
            if !expired {
                if err := h.oauthStates.MarkUsed(state.ID, now); err != nil { t.Fatal(err) }
            }
            if err := h.oauthStates.MarkUsed(state.ID, now); !errors.Is(err, ErrInvalidOAuthState) {
                t.Fatalf("one-time or expiry check not enforced: %v", err)
            }
        })
    }
}
'''

CLAIM_METHOD = '''func (r *OAuthStateRepo) MarkUsed(id int64, usedAt time.Time) error {
    conn, err := r.connection()
    if err != nil { return err }
    result := conn.Model(&OAuthStateToken{}).
        Where("id = ? AND used_at IS NULL AND expires_at > ?", id, usedAt.UTC()).
        Updates(map[string]any{"used_at": usedAt.UTC(), "updated_at": usedAt.UTC()})
    if result.Error != nil { return result.Error }
    if result.RowsAffected != 1 { return ErrInvalidOAuthState }
    return nil
}'''


def replace_method(source, receiver, method, replacement):
    """Refuse unknown method shapes rather than applying a heuristic edit elsewhere."""
    pattern = r"(?ms)^func \(" + re.escape(receiver) + r"\) " + re.escape(method) + r"\(.*?^}"
    result, count = re.subn(pattern, lambda match: replacement, source)
    if count != 1:
        raise ValueError(f"expected exactly one {method} method")
    return result


def auth_fingerprints(root):
    """Record all auth Go source so preserving neighboring app changes is independently checked."""
    return {path.name: hashlib.sha256(path.read_bytes()).hexdigest() for path in sorted((root / "internal/auth").glob("*.go"))}


def run_tests(root, workspace, environment, name, expression=None):
    """Retain JSON logs while returning only test identities and outcomes, never auth payloads."""
    command = ["go", "test", "-mod=readonly", "-json", "-tags=integration", "./internal/auth", "-count=1"]
    if expression:
        command += ["-run", expression]
    log = workspace / f"{name}.json"
    with log.open("w") as output:
        result = subprocess.run(command, cwd=root, env=environment, stdout=output, stderr=subprocess.STDOUT, timeout=180)
    events = []
    for line in log.read_text().splitlines():
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    summary = {
        "phase": name,
        "exit_code": result.returncode,
        "passed_tests_and_subtests": sum(event.get("Action") == "pass" and bool(event.get("Test")) for event in events),
        "failed_tests": [event["Test"] for event in events if event.get("Action") == "fail" and event.get("Test")],
        "passed_tests": sorted({event["Test"] for event in events if event.get("Action") == "pass" and event.get("Test")}),
        "skipped_tests": [event["Test"] for event in events if event.get("Action") == "skip" and event.get("Test")],
    }
    print(json.dumps({key: value for key, value in summary.items() if key != "passed_tests"}), flush=True)
    return summary


def require_complete_suite(summary, required=()):
    """Reject skipped or missing behaviors even when aggregate pass counts happen to increase."""
    if (summary["exit_code"] or not summary["passed_tests"] or summary["skipped_tests"]
            or not set(required).issubset(summary["passed_tests"])):
        raise RuntimeError(f"{summary['phase']} does not establish every required passing behavior")


def require_behavioral_failure(summary, required):
    """Require the intended test failures, rather than accepting compilation errors or a different regression."""
    if not summary["exit_code"] or not set(required).issubset(summary["failed_tests"]) or summary["skipped_tests"]:
        raise RuntimeError(f"{summary['phase']} did not reject every intended regression behaviorally")


def main():
    """Exercise baseline, red/green security coverage, and a behaviorally rejected reconciliation."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--forj", required=True, type=Path)
    parser.add_argument("--ditracker", required=True, type=Path)
    arguments = parser.parse_args()
    helpers = runpy.run_path(str(Path(__file__).with_name("spike-framework-upgrade.py")))
    workspace = Path(tempfile.mkdtemp(prefix="goforj-auth-reconciliation-", dir="/tmp"))
    root = workspace / "ditracker"
    root.mkdir()
    source = arguments.ditracker.expanduser().resolve(strict=True)
    helpers["copy_committed_app"](source, root)
    before = auth_fingerprints(root)
    environment = {key: value for key, value in os.environ.items() if key in ("PATH", "HOME", "GOCACHE", "GOMODCACHE")}
    environment.update({"TMPDIR": "/tmp", "GOWORK": "off", "DB_DRIVER": "sqlite", "APP_ENV": "testing",
                        "API_JWT_SECRET_KEY": "public-spike-test-key-not-a-real-secret-0123456789"})

    environment.setdefault("GOCACHE", "/tmp/gocache")
    phases = []
    baseline = run_tests(root, workspace, environment, "baseline")
    phases.append(baseline)
    require_complete_suite(baseline)
    regression = root / "internal/auth/upgrade_probe_integration_test.go"
    if regression.exists():
        raise RuntimeError("probe would replace an existing app test")
    regression.write_text(REGRESSION)
    red = run_tests(root, workspace, environment, "regression-before-fix", "^TestUpgradeProbeOAuthStateSingleUse$")
    phases.append(red)
    regression_tests = {"TestUpgradeProbeOAuthStateSingleUse", "TestUpgradeProbeOAuthStateSingleUse/replayed", "TestUpgradeProbeOAuthStateSingleUse/expired"}
    require_behavioral_failure(red, regression_tests)
    state = root / "internal/auth/oauth_state.go"
    state.write_text(replace_method(state.read_text(), "r *OAuthStateRepo", "MarkUsed", CLAIM_METHOD))
    subprocess.run(["gofmt", "-w", str(state), str(regression)], check=True)
    green = run_tests(root, workspace, environment, "regression-after-fix", "^TestUpgradeProbeOAuthStateSingleUse$")
    phases.append(green)
    require_complete_suite(green, regression_tests)
    binary = arguments.forj.expanduser().resolve(strict=True)
    snapshot = json.loads(subprocess.check_output([str(binary), "render", "--snapshot"], cwd=root, env=environment, text=True, timeout=30))
    incoming = next(artifact["source"] for artifact in snapshot["artifacts"] if artifact["path"] == "internal/auth/service.go")
    method = re.search(r"(?ms)^func \(s \*Service\) AuthenticateRequest\(.*?^}", incoming)
    if method is None:
        raise RuntimeError("incoming request-auth method shape is unsupported")
    service = root / "internal/auth/service.go"
    original_service = service.read_bytes()
    service.write_text(replace_method(service.read_text(), "s *Service", "AuthenticateRequest", method.group()))
    try:
        negative = run_tests(root, workspace, environment, "destructive-workflow-replacement", "^TestImpersonationRoundTripIsSessionScopedAndPrivate$")
        phases.append(negative)
        require_behavioral_failure(negative, {"TestImpersonationRoundTripIsSessionScopedAndPrivate"})
    finally:
        service.write_bytes(original_service)
    final = run_tests(root, workspace, environment, "reconciled-auth-suite")
    phases.append(final)
    require_complete_suite(final, set(baseline["passed_tests"]) | regression_tests)
    after = auth_fingerprints(root)
    changed = sorted(name for name in set(before) | set(after) if before.get(name) != after.get(name))
    if changed != ["oauth_state.go", "upgrade_probe_integration_test.go"]:
        raise RuntimeError("candidate changed unexpected auth source")
    summary = {"workspace": str(workspace), "revision": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=source, text=True).strip(),
               "changed_auth_files": changed, "phases": phases}
    (workspace / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    print(json.dumps({"workspace": str(workspace), "changed_auth_files": changed}), flush=True)


if __name__ == "__main__":
    main()

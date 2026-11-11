"""Verify that the upgrade experiment cannot claim success from weaker test evidence."""

import copy
import io
import json
import os
from pathlib import Path
import runpy
import shutil
import signal
import subprocess
import sys
import time
import tarfile
import tempfile
import unittest
from unittest.mock import Mock, patch


PROBE = runpy.run_path(str(Path(__file__).with_name("spike-auth-reconciliation.py")))


class EvidenceTests(unittest.TestCase):
    """Exercise false-positive evidence that aggregate pass counts do not detect."""

    def test_complete_suite_preserves_test_identities(self):
        """A larger suite cannot compensate for a missing required app behavior."""
        evidence = {"phase": "final", "exit_code": 0, "passed_tests": ["Existing", "New"], "skipped_tests": []}
        PROBE["require_complete_suite"](evidence, {"Existing"})
        with self.assertRaises(RuntimeError):
            PROBE["require_complete_suite"](evidence, {"Removed"})

    def test_complete_suite_rejects_skips_failures_and_empty_runs(self):
        """An exit code alone cannot establish that the intended suite executed."""
        for change in ({"exit_code": 1}, {"passed_tests": []}, {"skipped_tests": ["Existing"]}):
            evidence = {"phase": "baseline", "exit_code": 0, "passed_tests": ["Existing"], "skipped_tests": []}
            evidence.update(change)
            with self.subTest(change=change), self.assertRaises(RuntimeError):
                PROBE["require_complete_suite"](evidence)

    def test_behavioral_failure_requires_each_regression(self):
        """A failing parent, compiler error, or unrelated failure does not prove both regressions."""
        required = {"Parent/replay", "Parent/expiry"}
        evidence = {"phase": "red", "exit_code": 1, "failed_tests": list(required), "skipped_tests": []}
        PROBE["require_behavioral_failure"](evidence, required)
        for change in ({"exit_code": 0}, {"failed_tests": []}, {"failed_tests": ["Parent"]},
                       {"failed_tests": ["Parent/replay"]}, {"skipped_tests": ["Parent/expiry"]}):
            invalid = evidence | change
            with self.subTest(change=change), self.assertRaises(RuntimeError):
                PROBE["require_behavioral_failure"](invalid, required)


class ArchiveTests(unittest.TestCase):
    """Keep the disposable reference copier aligned with the planner's private-path policy."""

    def test_environment_layers_and_symlinks_are_excluded(self):
        """Case variants of private env files must never enter the copied workspace."""
        archive = io.BytesIO()
        with tarfile.open(fileobj=archive, mode="w") as output:
            for name in (".env", ".ENV.production", "nested/.Env.testing", "internal/owner.go"):
                entry = tarfile.TarInfo(name)
                entry.size = len(b"canary")
                output.addfile(entry, io.BytesIO(b"canary"))
            link = tarfile.TarInfo("private-link")
            link.type = tarfile.SYMTYPE
            link.linkname = "/outside"
            output.addfile(link)
        helper = runpy.run_path(str(Path(__file__).with_name("spike-framework-upgrade.py")))
        with tempfile.TemporaryDirectory(dir="/tmp") as directory:
            root = Path(directory)
            with patch("subprocess.check_output", return_value=archive.getvalue()) as command:
                helper["copy_committed_app"](root, root, "frozen-revision")
                command.assert_called_once_with(["git", "archive", "frozen-revision"], cwd=root)
            self.assertEqual([str(path.relative_to(root)) for path in root.rglob("*") if path.is_file()], ["internal/owner.go"])


class AgentTrialTests(unittest.TestCase):
    """Keep independent agent scoring stricter than a successful compiler exit."""

    def setUp(self):
        """Load the trial helpers without preparing or evaluating an application."""
        self.trial = runpy.run_path(str(Path(__file__).with_name("spike-agent-upgrade.py")))

    def test_preparation_rejects_renderer_changes(self):
        """A partially copied or changing executable cannot become a comparison reference."""
        original_copy = shutil.copy2
        for change in ("copy", "source", "snapshot"):
            with self.subTest(change=change), tempfile.TemporaryDirectory(dir="/tmp") as directory:
                root = Path(directory)
                case = root / "trial"
                case.mkdir()
                renderer = root / "renderer"
                renderer.write_bytes(b"original renderer")

                def copy_renderer(source, destination):
                    """Inject a deterministic replacement during the copy boundary."""
                    original_copy(source, destination)
                    if change == "copy":
                        destination.write_bytes(b"partial copy")
                    elif change == "source":
                        source.write_bytes(b"replaced source")

                def archive(_source, baseline, _revision):
                    """Provide a regular source file without invoking Git or the renderer."""
                    (baseline / "owner.go").write_text("package owner")

                def run(command, **_options):
                    """Replace the frozen executable after a protocol invocation."""
                    if command[0] != "git":
                        frozen = Path(command[0])
                        frozen.chmod(0o600)
                        frozen.write_bytes(b"changed during snapshot")
                    return subprocess.CompletedProcess(command, 0, stdout="{}")

                with patch("tempfile.mkdtemp", return_value=str(case)),                      patch("subprocess.check_output", return_value="frozen-revision\n"),                      patch("shutil.copy2", side_effect=copy_renderer),                      patch.dict(self.trial["prepare"].__globals__["HELPERS"], {"copy_committed_app": archive}),                      patch("subprocess.run", side_effect=run), self.assertRaises(RuntimeError):
                    self.trial["prepare"](root, renderer)

    def test_log_parser_preserves_package_identity(self):
        """Equal test names in different packages cannot conceal a lost behavior."""
        with tempfile.TemporaryDirectory(dir="/tmp") as directory:
            log = Path(directory) / "tests.json"
            log.write_text('\n'.join([
                '{"Action":"pass","Package":"first","Test":"TestSame"}',
                '{"Action":"skip","Package":"second","Test":"TestSame"}',
                '{"Action":"build-fail","ImportPath":"third"}',
                'compiler diagnostic',
                'null',
                '[]',
            ]))
            result = self.trial["parse_tests"](log, 1)
            self.assertEqual(result["passed"], ["first::TestSame"])
            self.assertEqual(result["skipped"], ["second::TestSame"])
            self.assertEqual(result["package_failures"], ["third"])

    def test_inventory_never_opens_private_layers_or_symlink_targets(self):
        """New local env layers and external targets cannot leak into source fingerprints."""
        with tempfile.TemporaryDirectory(dir="/tmp") as directory:
            root = Path(directory)
            (root / ".ENV.local").write_text("private canary")
            (root / "owner.go").write_text("package owner")
            outside = root / "outside"
            outside.mkdir()
            (outside / "secret").write_text("external canary")
            app = root / "app"
            app.mkdir()
            (app / "external").symlink_to(outside, target_is_directory=True)
            files = self.trial["inventory"](app)
            self.assertEqual(files, {"external": {"symlink": str(outside)}})
            with patch("pathlib.Path.open", side_effect=AssertionError("private bytes opened")):
                (root / "owner.go").unlink()
                shutil.rmtree(outside)
                files = self.trial["inventory"](root)
            self.assertEqual(files[".ENV.local"], {"private": True})

    def test_evaluation_rejects_changed_packet_or_baseline_before_running_tools(self):
        """A comparison cannot use a silently replaced target or rewritten baseline."""
        with tempfile.TemporaryDirectory(dir="/tmp") as directory:
            root = Path(directory)
            (root / "packet").mkdir()
            (root / "baseline").mkdir()
            packet = root / "packet/TASK.md"
            packet.write_text("frozen task")
            source = root / "baseline/owner.go"
            source.write_text("package owner")
            renderer = root / "renderer"
            renderer.write_bytes(b"frozen renderer")
            metadata = {"binary": str(renderer), "binary_sha256": self.trial["digest"](renderer), "files": self.trial["inventory"](root / "baseline"), "packet_sha256": {"TASK.md": self.trial["digest"](packet)}}
            (root / "trial.json").write_text(json.dumps(metadata))
            packet.write_text("changed task")
            with patch("subprocess.Popen") as tools, self.assertRaises(RuntimeError):
                self.trial["evaluate"](root)
            tools.assert_not_called()
            packet.write_text("frozen task")
            source.write_text("package changed")
            with patch("subprocess.Popen") as tools, self.assertRaises(RuntimeError):
                self.trial["evaluate"](root)
            tools.assert_not_called()
            source.write_text("package owner")
            renderer.write_bytes(b"replaced renderer")
            with patch("subprocess.Popen") as tools, self.assertRaises(RuntimeError):
                self.trial["evaluate"](root)
            tools.assert_not_called()

    def test_probe_does_not_replace_existing_test_files(self):
        """Overlay creation must fail rather than masking an app-owned regression test."""
        with tempfile.TemporaryDirectory(dir="/tmp") as directory:
            root = Path(directory)
            app = root / "app"
            (app / "internal/logger").mkdir(parents=True)
            (app / "go.mod").write_text("module example.com/owner\n")
            existing = app / "internal/logger/agent_upgrade_holdout_test.go"
            existing.write_text("owner test")
            with self.assertRaises(RuntimeError):
                self.trial["holdout_overlay"](app, root, "collision")
            self.assertEqual(existing.read_text(), "owner test")

    def test_probe_rejects_unsupported_module_declarations(self):
        """A probe cannot invent an import path when the application's module is ambiguous."""
        with tempfile.TemporaryDirectory(dir="/tmp") as directory:
            root = Path(directory)
            for declaration in ("no module declaration", "module invalid path"):
                (root / "go.mod").write_text(declaration)
                with self.subTest(declaration=declaration), self.assertRaises(RuntimeError):
                    self.trial["holdout_overlay"](root, root, "invalid-module")

    def test_build_gate_requires_a_main_package(self):
        """A successful Go archive build must not count as a runnable application."""
        if shutil.which("go") is None:
            self.skipTest("Go toolchain unavailable")
        with tempfile.TemporaryDirectory(dir="/tmp") as directory:
            root = Path(directory)
            command = root / "cmd/app"
            command.mkdir(parents=True)
            (root / "go.mod").write_text("module example.com/buildproof\n\ngo 1.25.0\n")
            source, output = command / "main.go", root / "application"
            source.write_text("package main\nfunc main(){}\n")
            result = self.trial["run_go"](root, root, "executable", self.trial["app_build_arguments"](output))
            self.assertEqual(result["exit_code"], 0)
            executable = output.read_bytes()
            self.assertNotEqual(executable[:7], b"!<arch>")
            if os.name != "nt":
                self.assertTrue(output.stat().st_mode & 0o111)
            source.write_text("package app\nfunc main(){}\n")
            result = self.trial["run_go"](root, root, "archive", self.trial["app_build_arguments"](output))
            self.assertNotEqual(result["exit_code"], 0)
            self.assertEqual(output.read_bytes(), executable, "failed build replaced the prior executable with an archive")

    def test_timeout_stops_descendant_processes(self):
        """A timed-out compiler cannot mutate a later validation phase."""
        self.assert_descendant_cleanup(False)

    def test_success_stops_descendant_processes(self):
        """A successful validation cannot leave an untracked background writer."""
        self.assert_descendant_cleanup(True)

    def assert_descendant_cleanup(self, success):
        """Exercise a real descendant waiting to write after its leader completes."""
        if not hasattr(os, "killpg"):
            self.skipTest("the disposable trial harness requires POSIX process groups")
        real_popen = subprocess.Popen
        processes = []
        with tempfile.TemporaryDirectory(dir="/tmp") as directory:
            root = Path(directory)
            marker, ready, gate = root / "late-write", root / "ready", root / "release-child"
            child_code = ("import time\nfrom pathlib import Path\ngate=Path(" + repr(str(gate)) +
                          ")\nwhile not gate.exists(): time.sleep(.01)\nPath(" + repr(str(marker)) + ").write_text('late')\n")
            parent_code = ("import subprocess,sys,time;from pathlib import Path;subprocess.Popen([sys.executable,'-c'," +
                           repr(child_code) + "]);Path(" + repr(str(ready)) + ").write_text('ready');time.sleep(" + ("0" if success else "5") + ")")

            def spawn(_command, **options):
                """Start a real process tree and wait for its readiness before applying the short test deadline."""
                self.assertTrue(options["start_new_session"])
                process = real_popen([sys.executable, "-c", parent_code], **options)
                processes.append(process)
                deadline = time.monotonic() + 5
                while not ready.exists() and process.poll() is None and time.monotonic() < deadline:
                    time.sleep(.01)
                self.assertTrue(ready.exists())
                return process

            try:
                with patch("subprocess.Popen", side_effect=spawn):
                    result = self.trial["run_go"](root, root, "timeout", ["test"], timeout=.1)
                self.assertEqual(result["exit_code"], 0 if success else 124)
                gate.write_text("release")
                time.sleep(.65)
                self.assertFalse(marker.exists(), "a descendant survived the validation deadline")
            finally:
                for process in processes:
                    try:
                        os.killpg(process.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    process.wait(timeout=5)

    def test_timeout_handles_process_exit_race(self):
        """A process that exits during deadline cleanup still produces an unsuccessful validation."""
        if not hasattr(os, "killpg"):
            self.skipTest("the disposable trial harness requires POSIX process groups")
        process = Mock(pid=12345)
        process.wait.side_effect = [subprocess.TimeoutExpired("go", 1), -9]
        with tempfile.TemporaryDirectory(dir="/tmp") as directory:
            root = Path(directory)
            with patch("subprocess.Popen", return_value=process), patch("os.killpg", side_effect=ProcessLookupError):
                result = self.trial["run_go"](root, root, "timeout-race", ["test"], timeout=1)
            self.assertEqual(result["exit_code"], 124)
            self.assertEqual(process.wait.call_count, 2)

    def test_protected_files_cannot_be_removed_or_rewritten(self):
        """An unchanged pass count cannot excuse modified tests or migration history."""
        before = {"internal/auth/app_test.go": {"sha256": "original"}, "migrations/one.sql": {"sha256": "history"}, "app/routes.go": {"sha256": "routes"}}
        after = {"internal/auth/app_test.go": {"sha256": "weakened"}, "app/routes.go": {"sha256": "adapted"}}
        self.assertEqual(self.trial["protected_changes"](before, after), ["internal/auth/app_test.go", "migrations/one.sql"])

    def test_residuals_separate_forks_missing_assets_and_directory_outputs(self):
        """A passing build cannot hide absent target assets or missing generation contracts."""
        snapshot = {"artifacts": [
            {"path": "internal/custom.go", "ownership": "framework-template", "source": "incoming"},
            {"path": "ui/new.js", "ownership": "framework-asset", "digest": "incoming"},
            {"path": "ui/fork.js", "ownership": "framework-asset", "digest": "incoming"},
            {"path": "cmd/app/frontend/dist", "ownership": "app-derived"},
            {"path": "build/openapi.json", "ownership": "app-derived"},
        ]}
        files = {"internal/custom.go": {"sha256": "app policy"}, "ui/fork.js": {"sha256": "app asset"}, "cmd/app/frontend/dist/index.html": {"sha256": "built"}}
        result = self.trial["residual_artifacts"](snapshot, files)
        self.assertEqual(result["source_differences"], ["internal/custom.go"])
        self.assertEqual(result["raw_asset_differences"], ["ui/fork.js"])
        self.assertEqual(result["missing_managed"], ["ui/new.js"])
        self.assertEqual(result["missing_derived"], ["build/openapi.json"])

    def test_status_distinguishes_blockers_from_regressions(self):
        """Pre-existing failures must not be reported as newly introduced behavior regressions."""
        status = self.trial["trial_status"]
        self.assertEqual(status(True, [], {"unit": []}, [], {}), "measured-gates-pass")
        self.assertEqual(status(False, [], {"unit": []}, [], {}), "blocked")
        self.assertEqual(status(False, ["weakened_test.go"], {"unit": []}, [], {}), "regressed")
        self.assertEqual(status(False, [], {"unit": ["LostBehavior"]}, [], {}), "regressed")
        self.assertEqual(status(False, [], {"unit": []}, ["concurrent_write.go"], {}), "invalidated")

    def test_nested_modules_cannot_hide_baseline_packages(self):
        """Unchanged failing tests and packages without tests must remain visible to validation."""
        if shutil.which("go") is None:
            self.skipTest("Go toolchain unavailable")
        with tempfile.TemporaryDirectory(dir="/tmp") as directory:
            root = Path(directory)
            (root / "go.mod").write_text("module example.com/coverage\n\ngo 1.25.0\n")
            for name, source in {"healthy": "func TestPolicy(t *testing.T){}", "broken": 'func TestExisting(t *testing.T){t.Fatal("existing defect")}'}.items():
                package = root / name
                package.mkdir()
                (package / "policy_test.go").write_text('package ' + name + '\nimport "testing"\n' + source)
            (root / "notests").mkdir()
            (root / "notests/policy.go").write_text("package notests\n")
            baseline = self.trial["run_go"](root, root, "baseline", ["test", "-json", "./...", "-count=1"])
            before = self.trial["inventory"](root)
            for name in ("broken", "notests"):
                (root / name / "go.mod").write_text("module example.com/coverage/" + name + "\n\ngo 1.25.0\n")
            candidate = self.trial["run_go"](root, root, "candidate", ["test", "-json", "./...", "-count=1"])
            self.assertEqual(candidate["exit_code"], 0)
            self.assertEqual(self.trial["protected_changes"](before, self.trial["inventory"](root)), [])
            results = {"baseline": {"unit": baseline, "auth": baseline}, "app": {"unit": candidate, "auth": candidate}}
            losses = self.trial["coverage_losses"](results)
            self.assertEqual(losses["unit"]["packages"], ["example.com/coverage/broken", "example.com/coverage/notests"])
            self.assertEqual(losses["unit"]["tests"], ["example.com/coverage/broken::TestExisting"])
            self.assertEqual(self.trial["trial_status"](False, [], {"unit": []}, [], losses), "regressed")
            results["app"] = results["baseline"]
            unchanged = self.trial["coverage_losses"](results)
            self.assertEqual(unchanged["unit"], {"packages": [], "tests": []})
            self.assertEqual(self.trial["trial_status"](False, [], {"unit": []}, [], unchanged), "blocked")

    def test_assessment_rejects_false_success(self):
        """Exercise no-op upgrades, zero-test runs, regressions, skips, and protected edits."""
        empty = {"exit_code": 0, "passed": [], "failed": [], "skipped": [], "package_failures": [], "packages": []}
        results = {side: {suite: copy.deepcopy(empty) for suite in ("unit", "auth", "holdout", "build")} for side in ("baseline", "app")}
        for side in results.values():
            side["unit"]["passed"] = ["app::TestPolicy"]
            side["auth"]["passed"] = ["auth::TestImpersonation"]
        names = ["logger::" + name for name in self.trial["HOLDOUT_TESTS"]]
        results["baseline"]["holdout"]["failed"] = names
        results["baseline"]["holdout"]["exit_code"] = 1
        results["app"]["holdout"]["passed"] = names
        self.assertTrue(self.trial["assess_results"](results, [])[0])
        self.assertFalse(self.trial["assess_results"](results, ["modified_test.go"])[0])
        changes = [
            ("unit", "exit_code", 1), ("unit", "passed", []),
            ("unit", "failed", ["app::TestNewBrokenBehavior"]),
            ("unit", "package_failures", ["app"]),
            ("auth", "passed", ["auth::DifferentTest"]),
            ("auth", "skipped", ["auth::NewSkippedBehavior"]),
            ("holdout", "passed", []), ("build", "exit_code", 1),
        ]
        for suite, key, value in changes:
            invalid = copy.deepcopy(results)
            invalid["app"][suite][key] = value
            with self.subTest(suite=suite, key=key):
                self.assertFalse(self.trial["assess_results"](invalid, [])[0])
        invalid = copy.deepcopy(results)
        invalid["baseline"]["holdout"]["failed"] = []
        self.assertFalse(self.trial["assess_results"](invalid, [])[0])
        for field, identity in (("failed", "app::PreviouslyBroken"), ("skipped", "app::PreviouslySkipped"), ("packages", "app/previously-broken")):
            invalid = copy.deepcopy(results)
            invalid["baseline"]["unit"][field] = [identity]
            with self.subTest(disappearing=field):
                self.assertFalse(self.trial["assess_results"](invalid, [])[0])
            repaired = copy.deepcopy(invalid)
            if field == "packages":
                repaired["app"]["unit"][field] = [identity]
            else:
                repaired["app"]["unit"]["passed"].append(identity)
            self.assertTrue(self.trial["assess_results"](repaired, [])[0])


if __name__ == "__main__":
    unittest.main()

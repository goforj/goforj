"""Verify that the upgrade experiment cannot claim success from weaker test evidence."""

import io
from pathlib import Path
import runpy
import tarfile
import tempfile
import unittest
from unittest.mock import patch


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
            with patch("subprocess.check_output", return_value=archive.getvalue()):
                helper["copy_committed_app"](root, root)
            self.assertEqual([str(path.relative_to(root)) for path in root.rglob("*") if path.is_file()], ["internal/owner.go"])


if __name__ == "__main__":
    unittest.main()

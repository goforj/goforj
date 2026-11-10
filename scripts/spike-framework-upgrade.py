#!/usr/bin/env python3
"""Inspect committed reference apps in disposable copies without exporting private env files."""

import argparse
import collections
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import stat
import subprocess
import tarfile
import tempfile


def tree_digest(root):
    """Fingerprint regular-file bytes and modes before and after inspection."""
    digest = hashlib.sha256()
    for path in sorted(root.rglob("*")):
        if path.is_file():
            digest.update(str(path.relative_to(root)).encode())
            digest.update(str(stat.S_IMODE(path.stat().st_mode)).encode())
            digest.update(path.read_bytes())
    return digest.hexdigest()


def copy_committed_app(source, destination):
    """Copy regular tracked files while excluding all environment-file layers."""
    archive = subprocess.check_output(["git", "archive", "HEAD"], cwd=source)
    with tarfile.open(fileobj=io.BytesIO(archive)) as entries:
        for entry in entries:
            path = PurePosixPath(entry.name)
            if not entry.isfile() or any(part.startswith(".env") for part in path.parts):
                continue
            if path.is_absolute() or ".." in path.parts:
                raise ValueError("archive contains an unsafe path")
            target = destination.joinpath(*path.parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(entries.extractfile(entry).read())
            target.chmod(entry.mode)


def main():
    """Run the actual CLI protocol and retain plans outside the source repositories."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--forj", required=True, type=Path)
    parser.add_argument("--project", required=True, action="append", type=Path)
    parser.add_argument("--from-renderer", type=Path)
    arguments = parser.parse_args()
    binary = arguments.forj.expanduser().resolve(strict=True)
    workspace = Path(tempfile.mkdtemp(prefix="goforj-framework-upgrade-", dir="/tmp"))
    results = []
    environment = {key: value for key, value in os.environ.items() if key in ("PATH", "HOME")}
    environment.update({"TMPDIR": "/tmp", "APP_ENV": "local"})
    for index, project in enumerate(arguments.project):
        source = project.expanduser().resolve(strict=True)
        destination = workspace / f"{index}-{source.name}"
        destination.mkdir()
        revision = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=source, text=True).strip()
        copy_committed_app(source, destination)
        before = tree_digest(destination)
        command = [str(binary), "render", "--plan", "--json"]
        if arguments.from_renderer:
            command += ["--from-renderer", str(arguments.from_renderer.expanduser().resolve(strict=True))]
        result = subprocess.run(command, cwd=destination, env=environment, capture_output=True, text=True, timeout=90)
        if result.returncode:
            raise RuntimeError(f"inspection failed for {source.name}; exit code {result.returncode}")
        plan = json.loads(result.stdout)
        unchanged = before == tree_digest(destination)
        if not unchanged:
            raise RuntimeError(f"inspection changed reference app {source.name}")
        (workspace / f"{index}-{source.name}-plan.json").write_text(json.dumps(plan, indent=2) + "\n")
        results.append({
            "project": source.name,
            "revision": revision,
            "scope": plan["scope"],
            "planned_artifacts": len(plan["files"]),
            "unchanged_bytes_and_modes": unchanged,
            "ownerships": dict(sorted(collections.Counter(file["ownership"] for file in plan["files"]).items())),
            "actions": dict(sorted(collections.Counter(file["action"] for file in plan["files"]).items())),
        })
    summary = {"workspace": str(workspace), "references": results}
    (workspace / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()

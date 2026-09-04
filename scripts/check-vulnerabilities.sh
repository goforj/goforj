#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
module="${1:-.}"
report="$(mktemp)"
found="$(mktemp)"
allowed="$(mktemp)"
unexpected="$(mktemp)"
trap 'rm -f "$report" "$found" "$allowed" "$unexpected"' EXIT

set +e
GOWORK=off govulncheck -C "$repo_root/$module" ./... >"$report" 2>&1
status=$?
set -e

cat "$report"
if [[ "$status" -eq 0 ]]; then
  exit 0
fi
if [[ "$status" -ne 3 ]]; then
  exit "$status"
fi

grep -Eo 'GO-[0-9]{4}-[0-9]+' "$report" | sort -u >"$found"
grep -E '^GO-[0-9]{4}-[0-9]+$' "$repo_root/.github/govulncheck-allowlist.txt" | sort -u >"$allowed"
comm -23 "$found" "$allowed" >"$unexpected"
if [[ -s "$unexpected" ]]; then
  echo "unexpected reachable vulnerabilities:" >&2
  sed 's/^/  /' "$unexpected" >&2
  exit 1
fi

echo "all reported vulnerabilities have reviewed applicability records"

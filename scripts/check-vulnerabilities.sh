#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
module="${1:-.}"
tags="${2:-}"
report="$(mktemp)"
found="$(mktemp)"
unexpected="$(mktemp)"
reviewed="$(mktemp)"
exceptions="$repo_root/.github/govulncheck-exceptions.json"
trap 'rm -f "$report" "$found" "$unexpected" "$reviewed"' EXIT

set +e
scan_args=(-test -format json)
if [[ -n "$tags" ]]; then
  scan_args+=(-tags "$tags")
fi
GOWORK=off govulncheck "${scan_args[@]}" -C "$repo_root/$module" ./... >"$report" 2>&1
status=$?
set -e

if [[ "$status" -ne 0 ]]; then
  cat "$report"
  exit "$status"
fi

jq -r '
  select(.finding != null and .finding.trace[-1].function != null)
  | [
      .finding.osv,
      .finding.trace[0].module,
      .finding.trace[0].package,
      .finding.trace[0].function
    ]
  | @tsv
' "$report" | sort -u >"$found"

today="$(date -u +%F)"
if ! jq -e --arg today "$today" 'all(.[]; .expires >= $today)' "$exceptions" >/dev/null; then
  echo "govulncheck exception has expired" >&2
  exit 1
fi

while IFS=$'\t' read -r advisory dependency package symbol; do
  if [[ -z "$advisory" ]]; then
    continue
  fi
  if jq -e \
    --arg advisory "$advisory" \
    --arg dependency "$dependency" \
    --arg package "$package" \
    --arg today "$today" \
    'any(.[];
      . as $exception
      | $exception.id == $advisory
      and $exception.module == $dependency
      and $exception.expires >= $today
      and (
        any($exception.exact_packages[]; . == $package)
        or any($exception.package_prefixes[]; . as $prefix | $package == $prefix or ($package | startswith($prefix + "/")))
      )
    )' "$exceptions" >/dev/null; then
    printf '%s\t%s\t%s\n' "$advisory" "$dependency" "$package" >>"$reviewed"
    continue
  fi
  printf '%s\t%s\t%s\t%s\n' "$advisory" "$dependency" "$package" "$symbol" >>"$unexpected"
done <"$found"

if [[ -s "$unexpected" ]]; then
  echo "unexpected reachable vulnerabilities:" >&2
  sed 's/^/  /' "$unexpected" >&2
  text_args=(-test)
  if [[ -n "$tags" ]]; then
    text_args+=(-tags "$tags")
  fi
  GOWORK=off govulncheck "${text_args[@]}" -C "$repo_root/$module" ./... >&2 || true
  exit 1
fi

if [[ -s "$found" ]]; then
  sort -u "$reviewed" | sed 's/^/reviewed exception: /'
  echo "all reachable vulnerabilities have current, scoped applicability records"
else
  echo "no reachable vulnerabilities found"
fi

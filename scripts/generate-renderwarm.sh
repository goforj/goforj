#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out_dir="$repo_root/tools/renderwarm"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

imports_file="$tmp_dir/imports.txt"
modules_file="$tmp_dir/modules.txt"
go_files_file="$tmp_dir/go-template-files.txt"
raw_imports_file="$tmp_dir/raw-imports.txt"

module_for_import() {
  case "$1" in
    github.com/charmbracelet/lipgloss/*) echo "github.com/charmbracelet/lipgloss" ;;
    github.com/goforj/events/eventscore) echo "github.com/goforj/events/eventscore" ;;
    github.com/goforj/events/eventscore/*) echo "github.com/goforj/events/eventscore" ;;
    github.com/goforj/events/driver/gcppubsubevents) echo "github.com/goforj/events/driver/gcppubsubevents" ;;
    github.com/goforj/events/driver/kafkaevents) echo "github.com/goforj/events/driver/kafkaevents" ;;
    github.com/goforj/events/driver/natsevents) echo "github.com/goforj/events/driver/natsevents" ;;
    github.com/goforj/events/driver/natsjetstreamevents) echo "github.com/goforj/events/driver/natsjetstreamevents" ;;
    github.com/goforj/events/driver/redisevents) echo "github.com/goforj/events/driver/redisevents" ;;
    github.com/goforj/events/driver/snsevents) echo "github.com/goforj/events/driver/snsevents" ;;
    github.com/goforj/events/driver/*) echo "$1" ;;
    github.com/goforj/events/*) echo "github.com/goforj/events" ;;
    github.com/goforj/cache/driver/rediscache) echo "github.com/goforj/cache/driver/rediscache" ;;
    github.com/goforj/queue/*) echo "github.com/goforj/queue" ;;
    github.com/klauspost/compress/*) echo "github.com/klauspost/compress" ;;
    github.com/labstack/echo/v4/*) echo "github.com/labstack/echo/v4" ;;
    github.com/redis/go-redis/v9/*) echo "github.com/redis/go-redis/v9" ;;
    github.com/shirou/gopsutil/v4/*) echo "github.com/shirou/gopsutil/v4" ;;
    github.com/stretchr/testify/*) echo "github.com/stretchr/testify" ;;
    golang.org/x/net/*) echo "golang.org/x/net" ;;
    golang.org/x/term/*) echo "golang.org/x/term" ;;
    golang.org/x/tools/*) echo "golang.org/x/tools" ;;
    gorm.io/gorm/*) echo "gorm.io/gorm" ;;
    *)
      root_module_for_import "$1"
      ;;
  esac
}

root_module_for_import() {
  local import_path="$1"
  local match
  match="$(
    awk -v import_path="$import_path" '
      $1 == "require" || $1 == "(" || $1 == ")" || $1 ~ /^\/\// { next }
      $2 ~ /^v/ {
        module = $1
        if (import_path == module || index(import_path, module "/") == 1) {
          if (length(module) > length(best)) {
            best = module
          }
        }
      }
      END {
        if (best != "") {
          print best
        }
      }
    ' "$repo_root/go.mod"
  )"
  if [[ -n "$match" ]]; then
    echo "$match"
    return 0
  fi
  inferred_module_for_import "$import_path"
}

inferred_module_for_import() {
  local import_path="$1"
  case "$import_path" in
    gopkg.in/*)
      echo "$import_path" | awk -F/ '{ print $1 "/" $2 }'
      ;;
    github.com/*/*/v[0-9]*|golang.org/x/*/v[0-9]*)
      echo "$import_path" | awk -F/ '{ print $1 "/" $2 "/" $3 "/" $4 }'
      ;;
    github.com/*/*/v[0-9]*/*|golang.org/x/*/v[0-9]*/*)
      echo "$import_path" | awk -F/ '{ print $1 "/" $2 "/" $3 "/" $4 }'
      ;;
    github.com/*/*/*|golang.org/x/*/*|gorm.io/*/*)
      echo "$import_path" | awk -F/ '{ print $1 "/" $2 "/" $3 }'
      ;;
    *)
      echo "$import_path"
      ;;
  esac
}

root_version_for_module() {
  awk -v module="$1" '
    $1 == module && $2 ~ /^v/ {
      print $2
      exit
    }
  ' "$repo_root/go.mod"
}

override_version_for_module() {
  case "$1" in
    github.com/goforj/cache) echo "v0.4.0" ;;
    github.com/goforj/cache/cachecore) echo "v0.4.0" ;;
    github.com/goforj/cache/driver/rediscache) echo "v0.4.0" ;;
    github.com/goforj/events) echo "v0.2.0" ;;
    github.com/goforj/events/eventscore) echo "v0.2.0" ;;
    github.com/goforj/events/driver/gcppubsubevents) echo "v0.2.0" ;;
    github.com/goforj/events/driver/kafkaevents) echo "v0.2.0" ;;
    github.com/goforj/events/driver/natsevents) echo "v0.2.0" ;;
    github.com/goforj/events/driver/natsjetstreamevents) echo "v0.2.0" ;;
    github.com/goforj/events/driver/redisevents) echo "v0.2.0" ;;
    github.com/goforj/events/driver/snsevents) echo "v0.2.0" ;;
    github.com/goforj/godump) echo "v1.9.1" ;;
    github.com/goforj/httpx) echo "v1.1.0" ;;
    github.com/goforj/mail) echo "v0.3.1" ;;
    github.com/goforj/mail/mailses) echo "v0.3.1" ;;
    github.com/goforj/metrics) echo "v0.2.0" ;;
    github.com/goforj/null/v6) echo "v6.0.2" ;;
    github.com/goforj/queue) echo "v0.2.1" ;;
    github.com/goforj/scheduler/v2) echo "v2.1.4" ;;
    github.com/goforj/web) echo "v0.6.0" ;;
    github.com/klauspost/compress) echo "v1.18.4" ;;
    github.com/labstack/echo/v4) echo "v4.15.1" ;;
    github.com/redis/go-redis/v9) echo "v9.17.2" ;;
    github.com/shirou/gopsutil/v4) echo "v4.26.2" ;;
    golang.org/x/net) echo "v0.48.0" ;;
    golang.org/x/tools) echo "v0.38.0" ;;
    gorm.io/driver/mysql) echo "v1.6.0" ;;
    gorm.io/driver/postgres) echo "v1.6.0" ;;
    *)
      return 1
      ;;
  esac
}

version_for_module() {
  local module="$1"
  local version
  version="$(root_version_for_module "$module")"
  if [[ -n "$version" ]]; then
    echo "$version"
    return 0
  fi
  version="$(override_version_for_module "$module" || true)"
  if [[ -n "$version" ]]; then
    echo "$version"
    return 0
  fi
  latest_version_for_module "$module"
}

latest_version_for_module() {
  local module="$1"
  GOWORK=off go list -m -f '{{.Version}}' "${module}@latest" 2>/dev/null
}

cd "$repo_root"

find templates -type f \( -name '*.go.tmpl' -o -name 'embed.go' \) | sort > "$go_files_file"

if [[ ! -s "$go_files_file" ]]; then
  echo "no Go template files found" >&2
  exit 1
fi

: > "$raw_imports_file"
while IFS= read -r path; do
  if [[ "$path" == *_test.go.tmpl ]]; then
    awk '
      /^[[:space:]]*import[[:space:]]*\(/ {
        in_import = 1
        next
      }
      in_import && /^[[:space:]]*\)/ {
        in_import = 0
        next
      }
      in_import {
        print
        next
      }
      /^[[:space:]]*import[[:space:]]+/ {
        print
      }
    ' "$path" | grep -hEo '"(github\.com/[^"]+|golang\.org/x/[^"]+|gorm\.io/[^"]+|gopkg\.in/[^"]+)"' >> "$raw_imports_file" || true
    continue
  fi
  grep -hEo '"(github\.com/[^"]+|golang\.org/x/[^"]+|gorm\.io/[^"]+|gopkg\.in/[^"]+)"' "$path" >> "$raw_imports_file" || true
done < "$go_files_file"

{
  sed 's/^"//' "$raw_imports_file" \
    | sed 's/"$//' \
    | grep -Ev '^$' \
    | grep -E '^(github\.com|golang\.org/x|gorm\.io|gopkg\.in)/[A-Za-z0-9._/~+-]+(/[A-Za-z0-9._/~+-]+)*$' \
    | grep -Ev '^github\.com/goforj/goforj(/|$)'
  # Scheduler's module graph otherwise seeds the older driver used only by its examples.
  echo "github.com/goforj/cache/driver/rediscache"
  # Web's index package remains on str v1 while rendered templates use str/v2.
  echo "github.com/goforj/str"
} | sort -u > "$imports_file"

if [[ ! -s "$imports_file" ]]; then
  echo "no external Go imports found" >&2
  exit 1
fi

{
  while IFS= read -r pkg; do
    module_for_import "$pkg"
  done < "$imports_file"
  # httpx v1 selects an older godump, so warm builds pin the release validated by the host module.
  echo "github.com/goforj/godump"
} | sort -u > "$modules_file"

mkdir -p "$out_dir"

{
  echo "// Code generated by scripts/generate-renderwarm.sh; DO NOT EDIT."
  echo
  echo "module github.com/goforj/goforj/tools/renderwarm"
  echo
  awk '/^go / { print; exit }' "$repo_root/go.mod"
  echo
  echo "require ("
  while IFS= read -r module; do
    version="$(version_for_module "$module" || true)"
    if [[ -z "$version" ]]; then
      echo "missing pinned version for module: $module" >&2
      exit 1
    fi
    printf "\t%s %s\n" "$module" "$version"
  done < "$modules_file"
  echo ")"
} > "$out_dir/go.mod"

{
  echo "// Code generated by scripts/generate-renderwarm.sh; DO NOT EDIT."
  echo
  echo "package main"
  echo
  echo "import ("
  while IFS= read -r pkg; do
    printf '\t_ "%s"\n' "$pkg"
  done < "$imports_file"
  echo ")"
  echo
  echo "func main() {}"
} > "$out_dir/main.go"

echo "generated tools/renderwarm for $(wc -l < "$imports_file" | tr -d ' ') imports"

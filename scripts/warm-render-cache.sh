#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

imports_file="$tmp_dir/imports.txt"
modules_file="$tmp_dir/modules.txt"
go_files_file="$tmp_dir/go-template-files.txt"
raw_imports_file="$tmp_dir/raw-imports.txt"

module_for_import() {
  case "$1" in
    github.com/charmbracelet/lipgloss/*) echo "github.com/charmbracelet/lipgloss" ;;
    github.com/klauspost/compress/*) echo "github.com/klauspost/compress" ;;
    github.com/goforj/queue/*) echo "github.com/goforj/queue" ;;
    github.com/labstack/echo/v4/*) echo "github.com/labstack/echo/v4" ;;
    github.com/redis/go-redis/v9/*) echo "github.com/redis/go-redis/v9" ;;
    github.com/shirou/gopsutil/v4/*) echo "github.com/shirou/gopsutil/v4" ;;
    github.com/stretchr/testify/*) echo "github.com/stretchr/testify" ;;
    golang.org/x/net/*) echo "golang.org/x/net" ;;
    golang.org/x/term/*) echo "golang.org/x/term" ;;
    golang.org/x/tools/*) echo "golang.org/x/tools" ;;
    gorm.io/gorm/*) echo "gorm.io/gorm" ;;
    *) echo "$1" ;;
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

version_for_module() {
  case "$1" in
    github.com/goforj/cache) echo "v0.1.4" ;;
    github.com/goforj/cache/cachecore) echo "v0.1.4" ;;
    github.com/goforj/cache/driver/rediscache) echo "v0.1.4" ;;
    github.com/goforj/httpx) echo "v1.1.0" ;;
    github.com/goforj/null/v6) echo "v6.0.2" ;;
    github.com/goforj/queue) echo "v0.1.5" ;;
    github.com/goforj/scheduler) echo "v1.3.0" ;;
    github.com/klauspost/compress) echo "v1.18.4" ;;
    github.com/labstack/echo/v4) echo "v4.15.1" ;;
    github.com/redis/go-redis/v9) echo "v9.18.0" ;;
    github.com/shirou/gopsutil/v4) echo "v4.26.2" ;;
    golang.org/x/net) echo "v0.20.0" ;;
    golang.org/x/tools) echo "v0.38.0" ;;
    gorm.io/driver/mysql) echo "v1.6.0" ;;
    gorm.io/driver/postgres) echo "v1.6.0" ;;
    *)
      root_version_for_module "$1"
      ;;
  esac
}

cd "$repo_root"

find templates -type f \( -name '*.go.tmpl' -o -name 'embed.go' \) | sort > "$go_files_file"

if [[ ! -s "$go_files_file" ]]; then
  echo "no Go template files found to warm render cache" >&2
  exit 1
fi

: > "$raw_imports_file"
while IFS= read -r path; do
  grep -hEo '"(github\.com/[^"]+|golang\.org/x/[^"]+|gorm\.io/[^"]+|gopkg\.in/[^"]+)"' "$path" >> "$raw_imports_file" || true
done < "$go_files_file"

sed 's/^"//' "$raw_imports_file" \
  | sed 's/"$//' \
  | grep -Ev '^$' \
  | grep -E '^(github\.com|golang\.org/x|gorm\.io|gopkg\.in)/[A-Za-z0-9._/~+-]+(/[A-Za-z0-9._/~+-]+)*$' \
  | grep -Ev '^github\.com/goforj/goforj(/|$)' \
  | sort -u \
  > "$imports_file"

if [[ ! -s "$imports_file" ]]; then
  echo "no external Go imports found in render templates" >&2
  exit 1
fi

: > "$modules_file"
while IFS= read -r pkg; do
  module_for_import "$pkg"
done < "$imports_file" | sort -u > "$modules_file"

{
  echo 'package renderwarm'
  echo
  echo 'import ('
  while IFS= read -r pkg; do
    printf '\t_ "%s"\n' "$pkg"
  done < "$imports_file"
  echo ')'
} > "$tmp_dir/warm.go"

cd "$tmp_dir"

echo "warming render cache with $(wc -l < "$imports_file" | tr -d ' ') imports"
go mod init renderwarm >/dev/null 2>&1
go mod edit -go="$(awk '/^go / { print $2; exit }' "$repo_root/go.mod")"
while IFS= read -r module; do
  version="$(version_for_module "$module")"
  if [[ -z "$version" ]]; then
    echo "no pinned version for module: $module" >&2
    exit 1
  fi
  go mod edit -require="${module}@${version}"
done < "$modules_file"
go mod tidy
go test -run '^$' .
echo "render cache warm complete"

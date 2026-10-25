#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
starter_root="$repo_root/templates/starter-kits"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

# ImageMagick rasterizes the existing tile artwork for Apple home-screen icons.
magick -background none "$starter_root/vue/frontend/src/assets/goforj-tile-light.svg" \
  -resize 180x180 -strip "PNG32:$tmp_dir/goforj-touch-icon.png"

for kit in vue react templ-htmx; do
  cp "$tmp_dir/goforj-touch-icon.png" "$starter_root/$kit/frontend/public/goforj-touch-icon.png"
done

for kit in react templ-htmx; do
  for theme in light dark; do
    cp "$starter_root/react/frontend/src/assets/goforj-tile-$theme.svg" \
      "$starter_root/$kit/frontend/public/goforj-tile-$theme.svg"
  done
done

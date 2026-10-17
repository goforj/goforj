#!/bin/sh
set -eu
export GOTOOLCHAIN=local GOWORK=off

# Rebuild the same upstream releases with patched Go, networking, archive and
# compression dependencies instead of suppressing vulnerability findings.
build_root="$(mktemp -d)"
trap 'rm -rf "$build_root"' EXIT
module_cache="$(go env GOMODCACHE)"
plugin_dir=/usr/libexec/docker/cli-plugins
license_dir=/usr/share/doc/docker-plugins
mkdir -p "$plugin_dir" "$license_dir"

go mod download "github.com/docker/buildx@v${BUILDX_VERSION}"
cp -R "$module_cache/github.com/docker/buildx@v${BUILDX_VERSION}" "$build_root/buildx"
chmod -R u+w "$build_root/buildx"
cd "$build_root/buildx"
cp LICENSE "$license_dir/buildx-LICENSE"
go get "golang.org/x/net@${PLUGIN_X_NET_VERSION}" "github.com/klauspost/compress@${PLUGIN_COMPRESS_VERSION}" "github.com/moby/go-archive@${PLUGIN_GO_ARCHIVE_VERSION}"
CGO_ENABLED=0 go build -mod=mod -trimpath -buildvcs=false \
    -ldflags "-s -w -X github.com/docker/buildx/version.Version=v${BUILDX_VERSION}+goforj.1 -X github.com/docker/buildx/version.Package=github.com/docker/buildx" \
    -o "$plugin_dir/docker-buildx" ./cmd/buildx

go mod download "github.com/docker/compose/v5@v${COMPOSE_VERSION}"
cp -R "$module_cache/github.com/docker/compose/v5@v${COMPOSE_VERSION}" "$build_root/compose"
chmod -R u+w "$build_root/compose"
cd "$build_root/compose"
cp LICENSE "$license_dir/compose-LICENSE"
go get "golang.org/x/net@${PLUGIN_X_NET_VERSION}" "github.com/klauspost/compress@${PLUGIN_COMPRESS_VERSION}"
CGO_ENABLED=0 go build -mod=mod -trimpath -buildvcs=false \
    -ldflags "-s -w -X github.com/docker/compose/v5/internal.Version=v${COMPOSE_VERSION}+goforj.1" \
    -o "$plugin_dir/docker-compose" ./cmd

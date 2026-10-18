#!/bin/sh
set -eu
export GOWORK=off

# Preserve the packaged engine releases and build profiles while patching their
# embedded decoder. The compiler for application tests remains unchanged.
build_root="$(mktemp -d)"
trap 'rm -rf "$build_root"' EXIT
mkdir -p /out/bin /out/licenses
cat > "$build_root/buildinfo.go" <<'GO'
package main

import (
    "debug/buildinfo"
    "fmt"
    "os"
)

// main reads build settings without evaluating shell-quoted linker arguments.
func main() {
    info, err := buildinfo.ReadFile(os.Args[1])
    if err != nil {
        panic(err)
    }
    if os.Args[2] == "toolchain" {
        fmt.Print(info.GoVersion)
        return
    }
    for _, setting := range info.Settings {
        if setting.Key == os.Args[2] {
            fmt.Print(setting.Value)
            return
        }
    }
    if os.Args[2] == "-buildmode" {
        fmt.Print("exe")
    }
}
GO
GOTOOLCHAIN=local go build -o "$build_root/buildinfo" "$build_root/buildinfo.go"

# patch_minimum retains newer upstream fixes instead of downgrading them.
patch_minimum() {
    dependency="$1"
    minimum="$2"
    selected="$(go list -m -f '{{.Version}}' "$dependency")"
    first="$(printf '%s\n%s\n' "$minimum" "$selected" | sort -V | head -n 1)"
    case "$selected" in
        "$minimum"-*) go get "$dependency@$minimum" ;;
        *) if [ "$first" != "$minimum" ]; then go get "$dependency@$minimum"; fi ;;
    esac
}

# build_binary preserves the original compiler, tags, CGO and linker metadata.
build_binary() {
    original="$1"
    package="$2"
    revision_flag="$3"
    flags="$("$build_root/buildinfo" "$original" -ldflags)"
    tags="$("$build_root/buildinfo" "$original" -tags)"
    mode="$("$build_root/buildinfo" "$original" -buildmode)"
    cgo="$("$build_root/buildinfo" "$original" CGO_ENABLED)"
    CGO_ENABLED="$cgo" go build -mod=mod -trimpath -buildvcs=false \
        -buildmode="$mode" -tags="$tags" -ldflags "$flags -X $revision_flag" \
        -o "/out/bin/$(basename "$original")" "$package"
}

docker_commit="$(dockerd --version | awk '{print $5}')"
export GOTOOLCHAIN="$("$build_root/buildinfo" /usr/bin/dockerd toolchain)"
go mod download -json "github.com/moby/moby/v2@$docker_commit" > /out/licenses/moby-source.json
source_dir="$(go list -m -f '{{.Dir}}' "github.com/moby/moby/v2@$docker_commit")"
cp -R "$source_dir" "$build_root/moby"
chmod -R u+w "$build_root/moby"
cd "$build_root/moby"
cp LICENSE /out/licenses/moby-LICENSE
cp NOTICE /out/licenses/moby-NOTICE
patch_minimum github.com/klauspost/compress "$PLUGIN_COMPRESS_VERSION"
patch_minimum golang.org/x/net "$PLUGIN_X_NET_VERSION"
build_binary /usr/bin/dockerd ./cmd/dockerd \
    "github.com/moby/moby/v2/dockerversion.GitCommit=$docker_commit-goforj.1"
go list -m all > /out/licenses/moby-modules.txt

containerd_version="$(containerd --version | awk '{print $3}')"
containerd_revision="$(containerd --version | awk '{print $4}')"
export GOTOOLCHAIN="$("$build_root/buildinfo" /usr/bin/containerd toolchain)"
go mod download -json "github.com/containerd/containerd/v2@$containerd_version" > /out/licenses/containerd-source.json
source_dir="$(go list -m -f '{{.Dir}}' "github.com/containerd/containerd/v2@$containerd_version")"
cp -R "$source_dir" "$build_root/containerd"
chmod -R u+w "$build_root/containerd"
cd "$build_root/containerd"
cp LICENSE /out/licenses/containerd-LICENSE
cp NOTICE /out/licenses/containerd-NOTICE
patch_minimum github.com/klauspost/compress "$PLUGIN_COMPRESS_VERSION"
patch_minimum golang.org/x/net "$PLUGIN_X_NET_VERSION"
build_binary /usr/bin/containerd ./cmd/containerd \
    "github.com/containerd/containerd/v2/version.Revision=$containerd_revision-goforj.1"
build_binary /usr/bin/ctr ./cmd/ctr \
    "github.com/containerd/containerd/v2/version.Revision=$containerd_revision-goforj.1"
go list -m all > /out/licenses/containerd-modules.txt

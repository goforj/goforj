package env

import "runtime"

// goos and goarch are internal shims that allow tests to override the
// detected operating system and architecture.
//
// In production builds, they default to runtime.GOOS and runtime.GOARCH.
// In tests, they can be temporarily replaced to simulate other platforms.
var (
	goos   = runtime.GOOS
	goarch = runtime.GOARCH
)

// OS returns the current operating system identifier.
//
// This corresponds to runtime.GOOS and will be one of:
//
//   - "linux"
//   - "darwin"
//   - "windows"
//   - "freebsd", "openbsd", "netbsd", "dragonfly"
//   - "solaris", "aix", etc.
//
// Example:
//
//    fmt.Println(env.OS()) // prints: linux
//
func OS() string {
	return goos
}

// Arch returns the CPU architecture the binary is running on.
//
// This corresponds to runtime.GOARCH and may be:
//
//   - "amd64"
//   - "arm64"
//   - "386"
//   - "arm"
//   - etc.
//
// Example:
//
//    fmt.Println(env.Arch()) // prints: arm64
//
func Arch() string {
	return goarch
}

// IsLinux reports whether the runtime OS is Linux.
//
// Example:
//
//    if env.IsLinux() {
//        fmt.Println("Running on Linux")
//    }
//
func IsLinux() bool {
	return goos == "linux"
}

// IsMac reports whether the runtime OS is macOS (Darwin).
//
// Example:
//
//    if env.IsMac() {
//        fmt.Println("Running on macOS")
//    }
//
func IsMac() bool {
	return goos == "darwin"
}

// IsWindows reports whether the runtime OS is Windows.
//
// Example:
//
//    if env.IsWindows() {
//        fmt.Println("Running on Windows")
//    }
//
func IsWindows() bool {
	return goos == "windows"
}

// IsBSD reports whether the runtime OS is any BSD variant.
//
// BSD identifiers include:
//   - "freebsd"
//   - "openbsd"
//   - "netbsd"
//   - "dragonfly"
//
// Example:
//
//    if env.IsBSD() {
//        fmt.Println("Running on a BSD system")
//    }
//
func IsBSD() bool {
	switch goos {
	case "freebsd", "openbsd", "netbsd", "dragonfly":
		return true
	}
	return false
}

// IsUnix reports whether the OS is Unix-like.
//
// This returns true for:
//   - Linux
//   - macOS (Darwin)
//   - BSD variants
//   - Solaris, AIX
//
// Example:
//
//    if env.IsUnix() {
//        fmt.Println("POSIX-compliant system detected")
//    }
//
func IsUnix() bool {
	switch goos {
	case "linux", "darwin", "freebsd", "openbsd", "netbsd", "dragonfly", "solaris", "aix":
		return true
	}
	return false
}

// IsContainerOS reports whether this OS is *typically* used as a container base.
//
// This does NOT indicate whether you are *inside* a container —
// only that this OS is the kind most often found in container images.
//
// Currently returns true for Linux.
//
// Example:
//
//    if env.IsContainerOS() {
//        fmt.Println("Likely running in a Docker-optimized OS image")
//    }
//
func IsContainerOS() bool {
	return goos == "linux"
}

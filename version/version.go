package version

import (
	"regexp"
	"runtime/debug"
	"strings"
)

var (
	// Version is the module version (from Go module metadata). Defaults to "dev".
	Version = "dev"
	// Commit is the VCS revision if available.
	Commit = "none"
	// Dirty reports whether the VCS tree was modified.
	Dirty = false
)

// GoForjConfigVersion is the scaffold/config version written to .goforj.yml.
// Bump this intentionally when config/render behavior changes in a way you
// want recorded in project config.
const GoForjConfigVersion = "0.18.0"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" {
			Version = info.Main.Version
		}
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				Commit = s.Value
			case "vcs.modified":
				Dirty = s.Value == "true"
			}
		}
	}
}

// String returns a human-friendly version string.
func String() string {
	s := Version

	if isCleanReleaseVersion(s) && !Dirty {
		return s
	}

	commit := shortCommit(Commit)
	if commit != "" && commit != "none" {
		s += " (" + commit
		if Dirty {
			s += "+dirty"
		}
		return s + ")"
	}

	if Dirty {
		s += " (dirty)"
	}

	return s
}

var semverPattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
var releaseVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

// Semver returns the configured scaffold/config semantic version.
func Semver() string { return GoForjConfigVersion }

func isCleanReleaseVersion(raw string) bool {
	return releaseVersionPattern.MatchString(strings.TrimSpace(raw))
}

func shortCommit(raw string) string {
	commit := strings.TrimSpace(raw)
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func normalizeSemver(raw string) string {
	value := strings.TrimSpace(raw)
	switch value {
	case "", "dev", "(devel)":
		return "0.0.0-dev"
	}

	matches := semverPattern.FindStringSubmatch(value)
	if len(matches) == 0 {
		return "0.0.0-dev"
	}

	normalized := matches[1] + "." + matches[2] + "." + matches[3]
	if matches[4] != "" {
		normalized += matches[4]
	}
	if matches[5] != "" {
		normalized += matches[5]
	}
	return normalized
}

package version

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
)

var (
	// Version is the module version (from Go module metadata). Defaults to "dev".
	Version = "dev"
	// Commit is the VCS revision if available.
	Commit = "none"
	// BuildDirty is the linker- or build-metadata VCS dirty state used to authenticate development builds.
	// It remains unknown when neither source provides an explicit value.
	BuildDirty = "unknown"
	// Dirty reports whether the VCS tree was modified.
	Dirty = false
)

// GoForjConfigVersion is the scaffold/config version written to .goforj.yml.
// Bump this intentionally when config/render behavior changes in a way you
// want recorded in project config.
const GoForjConfigVersion = "0.19.0"

// init fills release metadata from Go build information when linker values are unavailable.
func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		applyBuildInfo(info)
	}
	Dirty = BuildDirty == "true"
}

// applyBuildInfo preserves linker stamps while accepting equivalent Go VCS metadata from ordinary attributable builds.
func applyBuildInfo(info *debug.BuildInfo) {
	if info == nil {
		return
	}
	if Version == "dev" && info.Main.Version != "" {
		Version = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if Commit == "none" {
				Commit = setting.Value
			}
		case "vcs.modified":
			if BuildDirty == "unknown" {
				BuildDirty = setting.Value
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
var fullCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Semver returns the configured scaffold/config semantic version.
func Semver() string { return GoForjConfigVersion }

// EvaluationIdentity returns the source identity that an authenticated live evaluation may record.
// Immutable, clean release versions identify their source by module version. Every other build
// must carry the full revision and an explicit dirty-state stamp from the repository build path.
func EvaluationIdentity() (version, commit string, dirty bool, err error) {
	version = strings.TrimSpace(Version)
	if isImmutableModuleVersion(version) && (BuildDirty == "unknown" || BuildDirty == "false") {
		return version, "", false, nil
	}

	commit = strings.TrimSpace(Commit)
	if !fullCommitPattern.MatchString(commit) {
		return "", "", false, fmt.Errorf("development build provenance requires a full Git commit; rebuild with make eval-runner")
	}
	if BuildDirty != "true" && BuildDirty != "false" {
		return "", "", false, fmt.Errorf("development build provenance requires an explicit dirty-state stamp; rebuild with make eval-runner")
	}
	return version, commit, BuildDirty == "true", nil
}

// isImmutableModuleVersion accepts published semantic versions, including prereleases and Go pseudo-versions, as reconstructable source identities.
func isImmutableModuleVersion(raw string) bool {
	return semverPattern.MatchString(strings.TrimSpace(raw))
}

// isCleanReleaseVersion centralizes the is clean release version decision for its callers.
func isCleanReleaseVersion(raw string) bool {
	return releaseVersionPattern.MatchString(strings.TrimSpace(raw))
}

// shortCommit keeps development version strings readable while retaining a useful revision prefix.
func shortCommit(raw string) string {
	commit := strings.TrimSpace(raw)
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// normalizeSemver keeps normalize semver handling consistent across callers.
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

package version

import (
	"runtime/debug"
)

var (
	// Version is the module version (from Go module metadata). Defaults to "dev".
	Version = "dev"
	// Commit is the VCS revision if available.
	Commit = "none"
	// Dirty reports whether the VCS tree was modified.
	Dirty = false
)

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
	if Commit != "" && Commit != "none" {
		commit := Commit
		if len(commit) > 7 {
			commit = commit[:7]
		}
		s += " (" + commit
		if Dirty {
			s += "+dirty"
		}
		s += ")"
	} else if Dirty {
		s += " (dirty)"
	}
	return s
}

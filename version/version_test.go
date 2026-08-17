package version

import (
	"runtime/debug"
	"testing"
)

func TestString(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalBuildDirty := BuildDirty
	originalDirty := Dirty
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDirty = originalBuildDirty
		Dirty = originalDirty
	})

	testCases := []struct {
		name    string
		version string
		commit  string
		dirty   bool
		want    string
	}{
		{
			name:    "clean tagged release",
			version: "v0.9.0",
			commit:  "6d7e0fdb25f9",
			want:    "v0.9.0",
		},
		{
			name:    "dirty tagged release",
			version: "v0.9.0",
			commit:  "6d7e0fdb25f9",
			dirty:   true,
			want:    "v0.9.0 (6d7e0fd+dirty)",
		},
		{
			name:    "pseudo version",
			version: "v0.9.1-0.20260526192855-6d7e0fdb25f9",
			commit:  "6d7e0fdb25f9",
			want:    "v0.9.1-0.20260526192855-6d7e0fdb25f9 (6d7e0fd)",
		},
		{
			name:    "dirty pseudo version",
			version: "v0.9.1-0.20260526192855-6d7e0fdb25f9",
			commit:  "6d7e0fdb25f9",
			dirty:   true,
			want:    "v0.9.1-0.20260526192855-6d7e0fdb25f9 (6d7e0fd+dirty)",
		},
		{
			name:    "dirty build without commit",
			version: "(devel)",
			commit:  "none",
			dirty:   true,
			want:    "(devel) (dirty)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			Version = tc.version
			Commit = tc.commit
			Dirty = tc.dirty

			got := String()
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// TestEvaluationIdentityRequiresAuthenticatedDevelopmentProvenance prevents paid evaluations from claiming an unverifiable checkout.
func TestEvaluationIdentityRequiresAuthenticatedDevelopmentProvenance(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalBuildDirty := BuildDirty
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDirty = originalBuildDirty
	})

	commit := "0123456789012345678901234567890123456789"
	testCases := []struct {
		name       string
		version    string
		commit     string
		buildDirty string
		wantDirty  bool
		wantErr    bool
	}{
		{name: "clean installed release", version: "v0.9.0", commit: "none", buildDirty: "unknown"},
		{name: "clean installed prerelease", version: "v0.9.0-rc.1", commit: "none", buildDirty: "unknown"},
		{name: "clean installed pseudo-version", version: "v0.9.1-0.20260817165422-57a28f252a0b", commit: "none", buildDirty: "unknown"},
		{name: "clean development build", version: "devel", commit: commit, buildDirty: "false"},
		{name: "dirty development build", version: "devel", commit: commit, buildDirty: "true", wantDirty: true},
		{name: "development build without commit", version: "devel", commit: "none", buildDirty: "false", wantErr: true},
		{name: "development build without dirty stamp", version: "devel", commit: commit, buildDirty: "unknown", wantErr: true},
		{name: "release with malformed dirty stamp", version: "v0.9.0", commit: commit, buildDirty: "maybe", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			Version = tc.version
			Commit = tc.commit
			BuildDirty = tc.buildDirty

			gotVersion, gotCommit, gotDirty, err := EvaluationIdentity()
			if tc.wantErr {
				if err == nil {
					t.Fatal("EvaluationIdentity() succeeded")
				}
				return
			}
			if err != nil {
				t.Fatalf("EvaluationIdentity(): %v", err)
			}
			if gotVersion != tc.version || gotDirty != tc.wantDirty {
				t.Fatalf("EvaluationIdentity() = (%q, %q, %t), want version %q and dirty %t", gotVersion, gotCommit, gotDirty, tc.version, tc.wantDirty)
			}
			if tc.version == "devel" && gotCommit != commit {
				t.Fatalf("EvaluationIdentity() commit = %q, want %q", gotCommit, commit)
			}
		})
	}
}

// TestApplyBuildInfoAcceptsOrdinaryVCSMetadata proves attributable Go builds satisfy the same identity contract as explicit linker stamps.
func TestApplyBuildInfoAcceptsOrdinaryVCSMetadata(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalBuildDirty := BuildDirty
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDirty = originalBuildDirty
	})

	Version = "dev"
	Commit = "none"
	BuildDirty = "unknown"
	commit := "0123456789012345678901234567890123456789"
	applyBuildInfo(&debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: commit},
			{Key: "vcs.modified", Value: "false"},
		},
	})
	gotVersion, gotCommit, gotDirty, err := EvaluationIdentity()
	if err != nil {
		t.Fatalf("EvaluationIdentity(): %v", err)
	}
	if gotVersion != "(devel)" || gotCommit != commit || gotDirty {
		t.Fatalf("EvaluationIdentity() = (%q, %q, %t), want attributable clean development build", gotVersion, gotCommit, gotDirty)
	}
}

func TestNormalizeSemver(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want string
	}{
		{name: "release without v", in: "1.2.3", want: "1.2.3"},
		{name: "release with v", in: "v1.2.3", want: "1.2.3"},
		{name: "prerelease", in: "1.2.3-rc.1", want: "1.2.3-rc.1"},
		{name: "build metadata", in: "1.2.3+build.9", want: "1.2.3+build.9"},
		{name: "pre and build", in: "v1.2.3-rc.1+abc", want: "1.2.3-rc.1+abc"},
		{name: "dev default", in: "dev", want: "0.0.0-dev"},
		{name: "devel default", in: "(devel)", want: "0.0.0-dev"},
		{name: "invalid default", in: "main", want: "0.0.0-dev"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeSemver(tc.in)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

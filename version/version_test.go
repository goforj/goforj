package version

import "testing"

func TestString(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalDirty := Dirty
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
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

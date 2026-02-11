package version

import "testing"

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

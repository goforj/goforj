//go:build !windows

package build

import "testing"

// TestRunPreflightExecutableNamePreservesNonWindowsName verifies non-Windows run artifacts remain extensionless.
func TestRunPreflightExecutableNamePreservesNonWindowsName(t *testing.T) {
	t.Parallel()
	if got := runPreflightExecutableName(); got != "app" {
		t.Fatalf("runPreflightExecutableName() = %q, want %q", got, "app")
	}
}

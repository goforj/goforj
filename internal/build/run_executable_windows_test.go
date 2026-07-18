//go:build windows

package build

import "testing"

// TestRunPreflightExecutableNameUsesWindowsSuffix verifies Windows run artifacts can be launched directly.
func TestRunPreflightExecutableNameUsesWindowsSuffix(t *testing.T) {
	t.Parallel()
	if got := runPreflightExecutableName(); got != "app.exe" {
		t.Fatalf("runPreflightExecutableName() = %q, want %q", got, "app.exe")
	}
}

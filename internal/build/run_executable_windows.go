//go:build windows

package build

// runPreflightExecutableName includes the suffix required by Windows process creation.
func runPreflightExecutableName() string {
	return "app.exe"
}

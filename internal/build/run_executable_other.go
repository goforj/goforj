//go:build !windows

package build

// runPreflightExecutableName preserves the extensionless executable name used by non-Windows hosts.
func runPreflightExecutableName() string {
	return "app"
}

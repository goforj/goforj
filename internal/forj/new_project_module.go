package forj

import (
	"fmt"
	"strings"

	"golang.org/x/mod/module"
)

// validateNewProjectModulePath validates only Go's local module-path grammar and never resolves the path remotely.
func validateNewProjectModulePath(value string) error {
	path := strings.TrimSpace(value)
	if path == "" {
		return fmt.Errorf("Go module path is required.")
	}
	if separator := strings.Index(path, "://"); separator >= 0 {
		suggestion := strings.TrimSuffix(path[separator+3:], "/")
		if suggestion != "" {
			return fmt.Errorf("Go module path must be an import path without a URL scheme. Use %q instead.", suggestion)
		}
		return fmt.Errorf("Go module path must be an import path without a URL scheme.")
	}
	if err := module.CheckImportPath(path); err != nil {
		return fmt.Errorf("Go module path %q is invalid: %v.", path, err)
	}
	if _, _, ok := module.SplitPathVersion(path); !ok {
		return fmt.Errorf("Go module path %q has an invalid major-version suffix.", path)
	}
	if path == "go" || path == "toolchain" {
		return fmt.Errorf("Go module path %q is reserved by the Go toolchain.", path)
	}
	return nil
}

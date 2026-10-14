package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/appassets"
)

// TestAppDependenciesReuseAndInvalidation covers lockfiles, manifests, missing installs, and corrupt receipts.
func TestAppDependenciesReuseAndInvalidation(t *testing.T) {
	for _, change := range []string{"none", "manifest", "lock", "missing", "corrupt"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			asset := appassets.Asset{App: "app", Name: "frontend", Root: "frontend", Prepare: conventionalSPAInstallCommand}
			writeBuildAssetTestFile(t, root, "frontend/package.json", `{"dependencies":{}}`)
			writeBuildAssetTestFile(t, root, "frontend/package-lock.json", `{"lockfileVersion":3}`)
			writeBuildAssetTestFile(t, root, "frontend/node_modules/installed", "ready")
			if current, err := appDependenciesCurrent(root, asset); err != nil || current {
				t.Fatalf("unrecorded install: %t %v", current, err)
			}
			if err := recordAppDependencies(root, asset); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "manifest":
				writeBuildAssetTestFile(t, root, "frontend/package.json", `{"name":"changed"}`)
			case "lock":
				writeBuildAssetTestFile(t, root, "frontend/package-lock.json", `{"lockfileVersion":2}`)
			case "missing":
				if err := os.RemoveAll(filepath.Join(root, "frontend/node_modules")); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				writeBuildAssetTestFile(t, root, "frontend/node_modules/.forj-install-receipt", "corrupt")
			}
			current, err := appDependenciesCurrent(root, asset)
			if err != nil || current != (change == "none") {
				t.Fatalf("freshness: %t %v", current, err)
			}
		})
	}
}

// TestAppDependenciesRequireConventionalLockedInstall keeps custom setup and unlocked installs executing normally.
func TestAppDependenciesRequireConventionalLockedInstall(t *testing.T) {
	root := t.TempDir()
	for _, prepare := range []string{"", "npm install", conventionalSPAInstallCommand} {
		asset := appassets.Asset{Root: "frontend", Prepare: prepare}
		if current, err := appDependenciesCurrent(root, asset); err != nil || current {
			t.Fatalf("unexpected dependency hit: %t %v", current, err)
		}
		if err := recordAppDependencies(root, asset); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAppDependenciesReportUnreadableInputsAndMissingTooling preserves failures rather than treating broken setup as cached.
func TestAppDependenciesReportUnreadableInputsAndMissingTooling(t *testing.T) {
	root := t.TempDir()
	asset := appassets.Asset{Root: "frontend", Prepare: conventionalSPAInstallCommand}
	writeBuildAssetTestFile(t, root, "frontend/package-lock.json", "lock")
	if _, err := appDependenciesCurrent(root, asset); err == nil {
		t.Fatal("missing manifest accepted")
	}
	writeBuildAssetTestFile(t, root, "frontend/package.json", "manifest")
	t.Setenv("PATH", t.TempDir())
	if _, err := appDependenciesCurrent(root, asset); err == nil {
		t.Fatal("missing Node accepted")
	}
}

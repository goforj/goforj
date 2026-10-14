package build

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/goforj/goforj/internal/appassets"
)

// appDependencyDigest limits dependency reuse to the conventional npm setup with a reproducible lockfile.
func appDependencyDigest(root string, asset appassets.Asset) (string, error) {
	if asset.Prepare != conventionalSPAInstallCommand {
		return "", nil
	}
	directory := filepath.Join(root, asset.Root)
	if filepath.IsAbs(asset.Root) {
		directory = asset.Root
	}
	lock, err := os.ReadFile(filepath.Join(directory, "package-lock.json"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "package.json"))
	if err != nil {
		return "", err
	}
	node, err := exec.Command("node", "--version").Output()
	if err != nil {
		return "", fmt.Errorf("inspect npm dependency toolchain: %w", err)
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s", manifest, lock, asset.Prepare, node, runtime.GOOS, runtime.GOARCH)))
	return hex.EncodeToString(digest[:]), nil
}

// appDependencyReceipt lives with node_modules so deleting installed dependencies also retires their receipt.
func appDependencyReceipt(root string, asset appassets.Asset) string {
	directory := filepath.Join(root, asset.Root)
	if filepath.IsAbs(asset.Root) {
		directory = asset.Root
	}
	return filepath.Join(directory, "node_modules", ".forj-install-receipt")
}

// appDependenciesCurrent avoids reinstalling a verified dependency tree when only frontend source changed.
func appDependenciesCurrent(root string, asset appassets.Asset) (bool, error) {
	digest, err := appDependencyDigest(root, asset)
	if err != nil || digest == "" {
		return false, err
	}
	receipt, err := os.ReadFile(appDependencyReceipt(root, asset))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return string(receipt) == digest, nil
}

// recordAppDependencies advances the install receipt only after the dependency command succeeds.
func recordAppDependencies(root string, asset appassets.Asset) error {
	digest, err := appDependencyDigest(root, asset)
	if err != nil || digest == "" {
		return err
	}
	return os.WriteFile(appDependencyReceipt(root, asset), []byte(digest), 0644)
}

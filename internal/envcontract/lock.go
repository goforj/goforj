package envcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	projectLockTimeout = 10 * time.Second
	staleProjectLock   = time.Minute
)

// withProjectLock serializes contract snapshots and updates across GoForj processes for one project.
func withProjectLock(root string, run func() error) error {
	digest := sha256.Sum256([]byte(filepath.Clean(root)))
	lockName := "goforj-environment-" + hex.EncodeToString(digest[:]) + ".lock"
	lockPath := filepath.Join(os.TempDir(), lockName)
	deadline := time.Now().Add(projectLockTimeout)
	for {
		err := os.Mkdir(lockPath, 0o700)
		if err == nil {
			defer func() { _ = os.Remove(lockPath) }()
			return run()
		}
		if !os.IsExist(err) {
			return fmt.Errorf("acquire project environment lock: %w", err)
		}
		info, statErr := os.Stat(lockPath)
		if statErr == nil && time.Since(info.ModTime()) > staleProjectLock {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for project environment lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

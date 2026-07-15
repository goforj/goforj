package apiindex

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/goforj/web/webindex"
)

// artifactPublicationLock limits publication ownership to the release operation required by the transaction.
type artifactPublicationLock interface {
	// Release relinquishes ownership after the complete artifact transaction finishes.
	Release() error
}

// artifactLockCoordinator serializes a complete artifact transaction before its compare-and-swap check begins.
type artifactLockCoordinator interface {
	// acquire waits for exclusive ownership of the artifact set.
	acquire(paths paths) (artifactPublicationLock, error)
}

// webindexArtifactLockCoordinator preserves interoperability with publishers using webindex directly.
type webindexArtifactLockCoordinator struct{}

// acquire takes the persistent webindex advisory lock beside the artifact set.
func (webindexArtifactLockCoordinator) acquire(paths paths) (artifactPublicationLock, error) {
	if _, err := artifactDirectory(paths); err != nil {
		return nil, err
	}
	publication, err := webindex.AcquireArtifactPublicationLock(
		context.Background(),
		paths.out,
		paths.diagnostics,
		paths.openAPI,
	)
	if err != nil {
		return nil, fmt.Errorf("lock API index artifacts: %w", err)
	}
	return publication, nil
}

// artifactDirectory requires one lock domain because GoForj publishes the files as one set.
func artifactDirectory(paths paths) (string, error) {
	directory := filepath.Clean(filepath.Dir(paths.out))
	for _, path := range []string{paths.diagnostics, paths.openAPI} {
		candidate := filepath.Clean(filepath.Dir(path))
		if candidate != directory {
			return "", fmt.Errorf("API index artifacts must share a directory for coordinated publication: %q and %q", paths.out, path)
		}
	}
	return directory, nil
}

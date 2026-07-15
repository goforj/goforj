package apiindex

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/goforj/web/webindex"
)

// TestArtifactLockSharesWebindexProcessLock verifies framework promotion cannot interleave with a direct publisher in the same process.
func TestArtifactLockSharesWebindexProcessLock(t *testing.T) {
	buildDir := filepath.Join(t.TempDir(), "build")
	paths := paths{
		out:         filepath.Join(buildDir, "api_index.json"),
		diagnostics: filepath.Join(buildDir, "api_index.diagnostics.json"),
		openAPI:     filepath.Join(buildDir, "openapi.json"),
	}
	direct, err := webindex.AcquireArtifactPublicationLock(
		context.Background(),
		paths.out,
		paths.diagnostics,
		paths.openAPI,
	)
	if err != nil {
		t.Fatalf("acquire direct webindex publication lock: %v", err)
	}
	t.Cleanup(func() { _ = direct.Release() })

	started := make(chan struct{})
	acquired := make(chan artifactPublicationLock, 1)
	errors := make(chan error, 1)
	go func() {
		close(started)
		lock, lockErr := (webindexArtifactLockCoordinator{}).acquire(paths)
		if lockErr != nil {
			errors <- lockErr
			return
		}
		acquired <- lock
	}()
	waitForPublicationSignal(t, "GoForj lock acquisition to start", started)

	select {
	case lock := <-acquired:
		_ = lock.Release()
		t.Fatal("GoForj acquired the artifact lock while direct webindex publication still held it")
	case err := <-errors:
		t.Fatalf("acquire GoForj artifact lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if err := direct.Release(); err != nil {
		t.Fatalf("release direct webindex publication lock: %v", err)
	}
	select {
	case lock := <-acquired:
		if err := lock.Release(); err != nil {
			t.Fatalf("release GoForj artifact lock: %v", err)
		}
	case err := <-errors:
		t.Fatalf("acquire GoForj artifact lock after release: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("GoForj did not acquire the shared artifact lock after direct publication released it")
	}
}

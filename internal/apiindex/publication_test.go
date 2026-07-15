package apiindex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/project"
)

// prepareTestCandidate exposes status formatting without keeping a test seam in production code.
func prepareTestCandidate(runner *Runner) (*preparedCandidate, string, error) {
	prepared, report, err := runner.prepareDefault(runOptions{})
	return prepared, report.status(), err
}

// TestCandidateRemainsIsolatedUntilPublication verifies preparation alone cannot replace active artifacts.
func TestCandidateRemainsIsolatedUntilPublication(t *testing.T) {
	root, paths := writeStagedFixture(t)
	restoreWorkingDirectory := changeWorkingDirectory(t, root)
	defer restoreWorkingDirectory()

	pending, status, err := prepareTestCandidate(newTestRunner())
	if err != nil {
		t.Fatalf("prepare API index: %v", err)
	}
	if !strings.HasPrefix(status, "app app, changed, ") || !strings.Contains(status, " operation") || !strings.Contains(status, " schema") || !strings.Contains(status, " diagnostic") {
		t.Fatalf("prepare status does not include outcome and counts: %q", status)
	}
	stagingDir := pending.stagingDir

	assertArtifactContents(t, paths, previousArtifacts(paths))

	pending.discard()
	if _, err := os.Stat(stagingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory remained after discard: %v", err)
	}
}

// TestCandidatePublishPromotesValidatedArtifacts verifies explicit publication replaces the complete active set.
func TestCandidatePublishPromotesValidatedArtifacts(t *testing.T) {
	root, paths := writeStagedFixture(t)
	restoreWorkingDirectory := changeWorkingDirectory(t, root)
	defer restoreWorkingDirectory()

	pending, _, err := prepareTestCandidate(newTestRunner())
	if err != nil {
		t.Fatalf("prepare API index: %v", err)
	}
	stagingDir := pending.stagingDir
	want := map[string][]byte{
		paths.out:         append([]byte(nil), pending.candidates.out.content...),
		paths.diagnostics: append([]byte(nil), pending.candidates.diagnostics.content...),
		paths.openAPI:     append([]byte(nil), pending.candidates.openAPI.content...),
	}

	if err := pending.Publish(); err != nil {
		t.Fatalf("publish candidate: %v", err)
	}
	assertArtifactContents(t, paths, want)
	for path, content := range want {
		if !json.Valid(content) {
			t.Fatalf("published artifact %s is invalid JSON", path)
		}
	}

	pending.discard()
	if _, err := os.Stat(stagingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory remained after publication: %v", err)
	}
}

// TestCandidatePublishRejectsMissingStagingState verifies invalid lifecycle construction cannot silently succeed.
func TestCandidatePublishRejectsMissingStagingState(t *testing.T) {
	pending := &preparedCandidate{}
	err := pending.publish()
	if err == nil || !strings.Contains(err.Error(), "candidate has no staging directory") {
		t.Fatalf("publish error = %v, want missing staging state", err)
	}
}

// TestNoChangePublicationPreservesModTimes verifies identical contracts do not trigger watcher-visible writes.
func TestNoChangePublicationPreservesModTimes(t *testing.T) {
	root, paths := writeStagedFixture(t)
	restoreWorkingDirectory := changeWorkingDirectory(t, root)
	defer restoreWorkingDirectory()

	runner := newTestRunner()
	if _, err := runner.RunDefault(Options{}); err != nil {
		t.Fatalf("first API index run: %v", err)
	}
	fixedTime := time.Unix(1_700_000_000, 123_000_000)
	for _, path := range artifactPaths(paths) {
		if err := os.Chtimes(path, fixedTime, fixedTime); err != nil {
			t.Fatalf("set artifact modtime %s: %v", path, err)
		}
	}

	status, err := runner.RunDefault(Options{})
	if err != nil {
		t.Fatalf("no-change API index run: %v", err)
	}
	if !strings.HasPrefix(status, "app app, unchanged, ") || !strings.Contains(status, " operation") || !strings.Contains(status, " schema") || !strings.Contains(status, " diagnostic") {
		t.Fatalf("no-change status does not include outcome and counts: %q", status)
	}
	for _, path := range artifactPaths(paths) {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat artifact %s: %v", path, err)
		}
		if !info.ModTime().Equal(fixedTime) {
			t.Fatalf("artifact %s modtime = %s, want %s", path, info.ModTime(), fixedTime)
		}
	}
	assertNoStagingDirectories(t, filepath.Dir(paths.out))
}

// TestCLIOnlyCleanupWaitsForPublication verifies stale artifacts remain active until cleanup is committed.
func TestCLIOnlyCleanupWaitsForPublication(t *testing.T) {
	root := t.TempDir()
	config := `render:
  components:
    cli: true
apps:
  ship:
    components:
      cli: true
`
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	t.Setenv("FORJ_APP", "ship")
	paths := defaultPaths(project.DefaultNamedApp("ship"))
	previous := previousArtifacts(paths)
	writeArtifacts(t, root, paths, previous)
	restoreWorkingDirectory := changeWorkingDirectory(t, root)
	defer restoreWorkingDirectory()

	pending, status, err := prepareTestCandidate(newTestRunner())
	if err != nil {
		t.Fatalf("prepare CLI-only cleanup: %v", err)
	}
	if status != "app ship, cleaned (no web API), 0 operations, 0 schemas, 0 diagnostics" {
		t.Fatalf("cleanup status = %q", status)
	}
	assertArtifactContents(t, paths, previous)

	if err := pending.Publish(); err != nil {
		t.Fatalf("publish CLI-only cleanup: %v", err)
	}
	for _, path := range artifactPaths(paths) {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale CLI-only artifact %s remained: %v", path, err)
		}
	}
}

// TestCandidateRejectsConcurrentActiveArtifactChange verifies publication uses the prepare snapshot as a compare-and-swap boundary.
func TestCandidateRejectsConcurrentActiveArtifactChange(t *testing.T) {
	root, paths := writeStagedFixture(t)
	restoreWorkingDirectory := changeWorkingDirectory(t, root)
	defer restoreWorkingDirectory()

	pending, _, err := prepareTestCandidate(newTestRunner())
	if err != nil {
		t.Fatalf("prepare API index: %v", err)
	}
	defer pending.discard()
	concurrent := []byte("{\"generation\":\"concurrent\"}\n")
	if err := os.WriteFile(paths.diagnostics, concurrent, 0o644); err != nil {
		t.Fatalf("write concurrent artifact: %v", err)
	}

	err = pending.Publish()
	if err == nil || !strings.Contains(err.Error(), "active artifacts changed after candidate preparation") {
		t.Fatalf("publish error = %v, want concurrent-change rejection", err)
	}
	if !strings.Contains(err.Error(), "app app, rejected") {
		t.Fatalf("publish error = %v, want App-scoped rejected outcome", err)
	}
	previous := previousArtifacts(paths)
	previous[paths.diagnostics] = concurrent
	assertArtifactContents(t, paths, previous)
}

// TestCandidatePublicationLockSerializesInterleavedWriters verifies identical writers converge without crossing the per-file CAS gap.
func TestCandidatePublicationLockSerializesInterleavedWriters(t *testing.T) {
	root, paths := writeStagedFixture(t)
	restoreWorkingDirectory := changeWorkingDirectory(t, root)
	defer restoreWorkingDirectory()

	runner := newTestRunner()
	first, _, err := prepareTestCandidate(runner)
	if err != nil {
		t.Fatalf("prepare first API index writer: %v", err)
	}
	defer first.discard()
	second, _, err := prepareTestCandidate(runner)
	if err != nil {
		t.Fatalf("prepare second API index writer: %v", err)
	}
	defer second.discard()

	firstMutation := make(chan struct{})
	releaseFirst := make(chan struct{})
	first.afterArtifactMutation = func(index int, _ string) {
		if index != 0 {
			return
		}
		close(firstMutation)
		<-releaseFirst
	}
	firstResult := make(chan error, 1)
	go func() { firstResult <- first.publish() }()
	select {
	case <-firstMutation:
	case <-time.After(5 * time.Second):
		t.Fatal("first publisher did not reach deterministic interleaving point")
	}

	secondResult := make(chan error, 1)
	go func() { secondResult <- second.publish() }()
	select {
	case err := <-secondResult:
		t.Fatalf("second publisher bypassed shared lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstResult; err != nil {
		t.Fatalf("first publication failed: %v", err)
	}
	if err := <-secondResult; err != nil {
		t.Fatalf("identical second publication should converge after lock handoff: %v", err)
	}

	want := map[string][]byte{
		paths.out:         first.candidates.out.content,
		paths.diagnostics: first.candidates.diagnostics.content,
		paths.openAPI:     first.candidates.openAPI.content,
	}
	assertArtifactContents(t, paths, want)
}

// TestPreparedCLIOnlyCleanupTreatsAlreadyAbsentSetAsSuccess verifies identical cleanup writers converge after lock handoff.
func TestPreparedCLIOnlyCleanupTreatsAlreadyAbsentSetAsSuccess(t *testing.T) {
	root, paths, first, _ := prepareCLIOnlyCleanupFixture(t)
	restoreWorkingDirectory := changeWorkingDirectory(t, root)
	defer restoreWorkingDirectory()

	second, _, err := prepareTestCandidate(newTestRunner())
	if err != nil {
		t.Fatalf("prepare second CLI-only cleanup: %v", err)
	}
	if err := first.publish(); err != nil {
		t.Fatalf("publish first CLI-only cleanup: %v", err)
	}
	if err := second.publish(); err != nil {
		t.Fatalf("identical second cleanup should converge: %v", err)
	}
	for _, path := range artifactPaths(paths) {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("CLI-only artifact %q remained after converged cleanup: %v", path, err)
		}
	}
}

// TestPreparedCLIOnlyCleanupRollsBackInjectedMidSetFailure verifies tombstones preserve the complete stale generation on failure.
func TestPreparedCLIOnlyCleanupRollsBackInjectedMidSetFailure(t *testing.T) {
	root, paths, pending, previous := prepareCLIOnlyCleanupFixture(t)
	restoreWorkingDirectory := changeWorkingDirectory(t, root)
	defer restoreWorkingDirectory()

	injected := errors.New("injected cleanup rename failure")
	pending.renameFile = func(oldPath string, newPath string) error {
		if filepath.Clean(oldPath) == filepath.Join(root, paths.diagnostics) {
			return injected
		}
		return os.Rename(oldPath, newPath)
	}
	err := pending.publish()
	if !errors.Is(err, injected) {
		t.Fatalf("cleanup error = %v, want injected rename failure", err)
	}
	assertArtifactContents(t, paths, previous)
	assertNoTombstones(t, filepath.Dir(paths.out))
}

// TestPreparedCLIOnlyCleanupJoinsRollbackFailure verifies the triggering cleanup error is not lost when restoration also fails.
func TestPreparedCLIOnlyCleanupJoinsRollbackFailure(t *testing.T) {
	root, paths, pending, _ := prepareCLIOnlyCleanupFixture(t)
	restoreWorkingDirectory := changeWorkingDirectory(t, root)
	defer restoreWorkingDirectory()

	cleanupErr := errors.New("injected cleanup failure")
	rollbackErr := errors.New("injected cleanup rollback failure")
	pending.renameFile = func(oldPath string, newPath string) error {
		if filepath.Clean(oldPath) == filepath.Join(root, paths.diagnostics) {
			return cleanupErr
		}
		if filepath.Clean(newPath) == filepath.Join(root, paths.out) && strings.Contains(filepath.Base(oldPath), ".forj-api-index-remove-") {
			return rollbackErr
		}
		return os.Rename(oldPath, newPath)
	}
	err := pending.publish()
	if !errors.Is(err, cleanupErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("joined cleanup error = %v, want cleanup and rollback causes", err)
	}
}

// TestRollbackArtifactsJoinsEveryFailure verifies rollback reports every artifact it could not restore.
func TestRollbackArtifactsJoinsEveryFailure(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "missing-first", "api_index.json")
	secondPath := filepath.Join(root, "missing-second", "openapi.json")
	err := rollbackArtifacts([]publication{
		{activePath: firstPath, active: fileSnapshot{exists: true, content: []byte("{}\n")}, wasPublished: true},
		{activePath: secondPath, active: fileSnapshot{exists: true, content: []byte("{}\n")}, wasPublished: true},
	})
	if err == nil {
		t.Fatal("expected rollback failures")
	}
	for _, path := range []string{firstPath, secondPath} {
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("rollback error %q does not include %s", err, path)
		}
	}
}

// prepareCLIOnlyCleanupFixture creates one deferred cleanup transaction with three distinct stale artifacts.
func prepareCLIOnlyCleanupFixture(t *testing.T) (string, paths, *preparedCandidate, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	config := `render:
  components:
    cli: true
apps:
  ship:
    components:
      cli: true
`
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write CLI-only project config: %v", err)
	}
	t.Setenv("FORJ_APP", "ship")
	paths := defaultPaths(project.DefaultNamedApp("ship"))
	previous := previousArtifacts(paths)
	writeArtifacts(t, root, paths, previous)

	restoreWorkingDirectory := changeWorkingDirectory(t, root)
	pending, _, err := prepareTestCandidate(newTestRunner())
	restoreWorkingDirectory()
	if err != nil {
		t.Fatalf("prepare CLI-only cleanup fixture: %v", err)
	}
	if pending == nil || !pending.remove {
		t.Fatal("CLI-only fixture did not produce deferred cleanup")
	}
	return root, paths, pending, previous
}

// writeStagedFixture creates an API-capable app with a previous complete artifact set.
func writeStagedFixture(t *testing.T) (string, paths) {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root)
	config := `render:
  components:
    cli: true
    web_api: true
`
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	routesPath := filepath.Join(root, "app", "routes.go")
	if err := os.MkdirAll(filepath.Dir(routesPath), 0o755); err != nil {
		t.Fatalf("create app route directory: %v", err)
	}
	if err := os.WriteFile(routesPath, []byte("package app\n"), 0o644); err != nil {
		t.Fatalf("write app route composition: %v", err)
	}
	paths := defaultPaths(project.DefaultApp())
	writeArtifacts(t, root, paths, previousArtifacts(paths))
	return root, paths
}

// previousArtifacts returns distinct valid documents so tests can detect any premature replacement.
func previousArtifacts(paths paths) map[string][]byte {
	return map[string][]byte{
		paths.out:         []byte("{\"previous\":\"manifest\"}\n"),
		paths.diagnostics: []byte("{\"previous\":\"diagnostics\"}\n"),
		paths.openAPI:     []byte("{\"previous\":\"openapi\"}\n"),
	}
}

// writeArtifacts installs a complete artifact set beneath a temporary project root.
func writeArtifacts(t *testing.T, root string, paths paths, contents map[string][]byte) {
	t.Helper()
	for _, path := range artifactPaths(paths) {
		absolutePath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
			t.Fatalf("create artifact directory: %v", err)
		}
		content, ok := contents[path]
		if !ok {
			content = []byte("{}\n")
		}
		if err := os.WriteFile(absolutePath, content, 0o644); err != nil {
			t.Fatalf("write artifact %s: %v", path, err)
		}
	}
}

// assertArtifactContents verifies that all active paths still form the expected generation.
func assertArtifactContents(t *testing.T, paths paths, want map[string][]byte) {
	t.Helper()
	for _, path := range artifactPaths(paths) {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read artifact %s: %v", path, err)
		}
		if string(content) != string(want[path]) {
			t.Fatalf("artifact %s = %q, want %q", path, content, want[path])
		}
	}
}

// artifactPaths keeps lifecycle assertions ordered consistently with publication.
func artifactPaths(paths paths) []string {
	return []string{paths.out, paths.diagnostics, paths.openAPI}
}

// changeWorkingDirectory scopes the process-wide working directory change used by default app resolution.
func changeWorkingDirectory(t *testing.T, root string) func() {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	return func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}
}

// assertNoStagingDirectories catches leaked candidates after every immediate publication path.
func assertNoStagingDirectories(t *testing.T, artifactDir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(artifactDir, ".forj-api-index-stage-*"))
	if err != nil {
		t.Fatalf("glob API index staging directories: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("API index staging directories remained: %v", matches)
	}
}

// assertNoTombstones catches cleanup state that should have been restored or disposed.
func assertNoTombstones(t *testing.T, artifactDir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(artifactDir, ".forj-api-index-remove-*"))
	if err != nil {
		t.Fatalf("glob API index tombstones: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("API index tombstones remained: %v", matches)
	}
}

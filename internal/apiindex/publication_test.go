package apiindex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goforj/goforj/project"
)

// TestCandidateRemainsIsolatedUntilPublication verifies preparation alone cannot replace active artifacts.
func TestCandidateRemainsIsolatedUntilPublication(t *testing.T) {
	fixture := writeStagedFixture(t)

	prepared, err := newTestRunner().prepareDefault(runOptions{root: fixture.root})
	if err != nil {
		t.Fatalf("prepare API index: %v", err)
	}
	pending := prepared.candidate
	status := prepared.report.status()
	if !strings.HasPrefix(status, "app app, changed, ") || !strings.Contains(status, " operation") || !strings.Contains(status, " schema") || !strings.Contains(status, " diagnostic") {
		t.Fatalf("prepare status does not include outcome and counts: %q", status)
	}
	stagingDir := pending.stagingDir

	assertArtifactContents(t, fixture.paths, fixture.previous)

	pending.discard()
	if _, err := os.Stat(stagingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory remained after discard: %v", err)
	}
}

// TestCandidatePublishPromotesValidatedArtifacts verifies explicit publication replaces the complete active set.
func TestCandidatePublishPromotesValidatedArtifacts(t *testing.T) {
	fixture := writeStagedFixture(t)

	prepared, err := newTestRunner().prepareDefault(runOptions{root: fixture.root})
	if err != nil {
		t.Fatalf("prepare API index: %v", err)
	}
	pending := prepared.candidate
	stagingDir := pending.stagingDir
	want := map[string][]byte{
		fixture.paths.out:         append([]byte(nil), pending.candidates.out.content...),
		fixture.paths.diagnostics: append([]byte(nil), pending.candidates.diagnostics.content...),
		fixture.paths.openAPI:     append([]byte(nil), pending.candidates.openAPI.content...),
	}

	if err := pending.Publish(); err != nil {
		t.Fatalf("publish candidate: %v", err)
	}
	assertArtifactContents(t, fixture.paths, want)
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
	fixture := writeStagedFixture(t)

	runner := newTestRunner()
	if _, err := runner.RunDefault(Options{Root: fixture.root}); err != nil {
		t.Fatalf("first API index run: %v", err)
	}
	fixedTime := time.Unix(1_700_000_000, 123_000_000)
	for _, path := range artifactPaths(fixture.paths) {
		if err := os.Chtimes(path, fixedTime, fixedTime); err != nil {
			t.Fatalf("set artifact modtime %s: %v", path, err)
		}
	}

	status, err := runner.RunDefault(Options{Root: fixture.root})
	if err != nil {
		t.Fatalf("no-change API index run: %v", err)
	}
	if !strings.HasPrefix(status, "app app, unchanged, ") || !strings.Contains(status, " operation") || !strings.Contains(status, " schema") || !strings.Contains(status, " diagnostic") {
		t.Fatalf("no-change status does not include outcome and counts: %q", status)
	}
	for _, path := range artifactPaths(fixture.paths) {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat artifact %s: %v", path, err)
		}
		if !info.ModTime().Equal(fixedTime) {
			t.Fatalf("artifact %s modtime = %s, want %s", path, info.ModTime(), fixedTime)
		}
	}
	assertNoStagingDirectories(t, filepath.Dir(fixture.paths.out))
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
	paths := rootDefaultPaths(root, defaultPaths(project.DefaultNamedApp("ship")))
	previous := previousArtifacts(paths)
	writeArtifacts(t, paths, previous)

	prepared, err := newTestRunner().prepareDefault(runOptions{root: root})
	if err != nil {
		t.Fatalf("prepare CLI-only cleanup: %v", err)
	}
	pending := prepared.candidate
	status := prepared.report.status()
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
	fixture := writeStagedFixture(t)

	prepared, err := newTestRunner().prepareDefault(runOptions{root: fixture.root})
	if err != nil {
		t.Fatalf("prepare API index: %v", err)
	}
	pending := prepared.candidate
	defer pending.discard()
	concurrent := []byte("{\"generation\":\"concurrent\"}\n")
	if err := os.WriteFile(fixture.paths.diagnostics, concurrent, 0o644); err != nil {
		t.Fatalf("write concurrent artifact: %v", err)
	}

	err = pending.Publish()
	if err == nil || !strings.Contains(err.Error(), "active artifacts changed after candidate preparation") {
		t.Fatalf("publish error = %v, want concurrent-change rejection", err)
	}
	if !strings.Contains(err.Error(), "app app, rejected") {
		t.Fatalf("publish error = %v, want App-scoped rejected outcome", err)
	}
	want := previousArtifacts(fixture.paths)
	want[fixture.paths.diagnostics] = concurrent
	assertArtifactContents(t, fixture.paths, want)
}

// TestCandidatePublicationLockSerializesInterleavedWriters verifies identical writers converge without crossing the per-file CAS gap.
func TestCandidatePublicationLockSerializesInterleavedWriters(t *testing.T) {
	fixture := writeStagedFixture(t)

	runner := newTestRunner()
	firstRun, err := runner.prepareDefault(runOptions{root: fixture.root})
	if err != nil {
		t.Fatalf("prepare first API index writer: %v", err)
	}
	first := firstRun.candidate
	defer first.discard()
	secondRun, err := runner.prepareDefault(runOptions{root: fixture.root})
	if err != nil {
		t.Fatalf("prepare second API index writer: %v", err)
	}
	second := secondRun.candidate
	defer second.discard()
	locks := newQueuedPublicationCoordinator()
	first.locks = locks
	second.locks = locks

	firstMutation := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			close(releaseFirst)
		})
	}
	publisherDone := make(chan struct{}, 2)
	startedPublishers := 0
	startPublisher := func(candidate *preparedCandidate, result chan<- error) {
		startedPublishers++
		go func() {
			defer func() {
				publisherDone <- struct{}{}
			}()
			result <- candidate.publish()
		}()
	}
	t.Cleanup(func() {
		release()
		locks.stop()
		waitForPublisherCleanup(t, startedPublishers, publisherDone)
	})
	first.afterArtifactMutation = func(index int, _ string) {
		if index != 0 {
			return
		}
		close(firstMutation)
		select {
		case <-releaseFirst:
		case <-time.After(publicationTestTimeout):
			t.Error("first publisher remained held at the interleaving point")
		}
	}
	firstResult := make(chan error, 1)
	startPublisher(first, firstResult)
	select {
	case <-firstMutation:
	case err := <-firstResult:
		t.Fatalf("first publisher exited before the interleaving point: %v", err)
	case <-time.After(publicationTestTimeout):
		t.Fatal("first publisher did not reach the interleaving point")
	}

	secondResult := make(chan error, 1)
	startPublisher(second, secondResult)
	waitForPublicationSignal(t, "second publisher to enter the held lock", locks.secondAcquireStarted)
	release()
	firstErr := waitForPublicationResult(t, "first publisher", firstResult)
	secondErr := waitForPublicationResult(t, "second publisher", secondResult)
	if firstErr != nil {
		t.Fatalf("first publication failed: %v", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("identical second publication should converge after lock handoff: %v", secondErr)
	}

	want := map[string][]byte{
		fixture.paths.out:         first.candidates.out.content,
		fixture.paths.diagnostics: first.candidates.diagnostics.content,
		fixture.paths.openAPI:     first.candidates.openAPI.content,
	}
	assertArtifactContents(t, fixture.paths, want)
}

// publicationTestTimeout bounds every worker and coordination wait in publication tests.
const publicationTestTimeout = 5 * time.Second

// queuedPublicationCoordinator deterministically exposes when a second publisher is waiting for ownership.
type queuedPublicationCoordinator struct {
	token                chan struct{}
	acquireCalls         atomic.Int32
	secondAcquireStarted chan struct{}
	stopped              chan struct{}
}

// queuedPublicationLock makes release safe when normal completion and test cleanup converge on the same owner.
type queuedPublicationLock struct {
	coordinator *queuedPublicationCoordinator
	releaseOnce sync.Once
}

// newQueuedPublicationCoordinator starts with one token so the first publisher can reach the controlled mutation point without extra setup.
func newQueuedPublicationCoordinator() *queuedPublicationCoordinator {
	coordinator := &queuedPublicationCoordinator{
		token:                make(chan struct{}, 1),
		secondAcquireStarted: make(chan struct{}),
		stopped:              make(chan struct{}),
	}
	coordinator.token <- struct{}{}
	return coordinator
}

// acquire records the second attempt while the first publisher still owns the single publication token.
func (c *queuedPublicationCoordinator) acquire(_ paths) (artifactPublicationLock, error) {
	if c.acquireCalls.Add(1) == 2 {
		close(c.secondAcquireStarted)
	}
	select {
	case <-c.token:
		return &queuedPublicationLock{coordinator: c}, nil
	case <-c.stopped:
		return nil, errors.New("queued publication coordinator stopped")
	}
}

// Release hands ownership to the next admitted publisher without allowing transactions to overlap.
func (l *queuedPublicationLock) Release() error {
	l.releaseOnce.Do(func() {
		select {
		case l.coordinator.token <- struct{}{}:
		case <-l.coordinator.stopped:
		}
	})
	return nil
}

// stop releases blocked test publishers when an earlier assertion ends the test.
func (c *queuedPublicationCoordinator) stop() {
	close(c.stopped)
}

// waitForPublicationSignal bounds synchronization failures so a broken coordinator cannot hang the package suite.
func waitForPublicationSignal(t *testing.T, description string, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(publicationTestTimeout):
		t.Fatalf("timed out waiting for %s", description)
	}
}

// waitForPublicationResult bounds worker completion so a lock regression fails with its publication context.
func waitForPublicationResult(t *testing.T, description string, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(publicationTestTimeout):
		t.Fatalf("timed out waiting for %s", description)
		return nil
	}
}

// waitForPublisherCleanup bounds the complete worker drain so coordination failures cannot hang the package suite.
func waitForPublisherCleanup(t *testing.T, expected int, completed <-chan struct{}) {
	t.Helper()
	timer := time.NewTimer(publicationTestTimeout)
	defer timer.Stop()
	for remaining := expected; remaining > 0; remaining-- {
		select {
		case <-completed:
		case <-timer.C:
			t.Errorf("%d publication goroutine(s) remained blocked during cleanup", remaining)
			return
		}
	}
}

// TestPreparedCLIOnlyCleanupTreatsAlreadyAbsentSetAsSuccess verifies identical cleanup writers converge after lock handoff.
func TestPreparedCLIOnlyCleanupTreatsAlreadyAbsentSetAsSuccess(t *testing.T) {
	fixture := prepareCLIOnlyCleanupFixture(t)

	secondRun, err := newTestRunner().prepareDefault(runOptions{root: fixture.root})
	if err != nil {
		t.Fatalf("prepare second CLI-only cleanup: %v", err)
	}
	if err := fixture.candidate.publish(); err != nil {
		t.Fatalf("publish first CLI-only cleanup: %v", err)
	}
	if err := secondRun.candidate.publish(); err != nil {
		t.Fatalf("identical second cleanup should converge: %v", err)
	}
	for _, path := range artifactPaths(fixture.paths) {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("CLI-only artifact %q remained after converged cleanup: %v", path, err)
		}
	}
}

// TestPreparedCLIOnlyCleanupRollsBackInjectedMidSetFailure verifies tombstones preserve the complete stale generation on failure.
func TestPreparedCLIOnlyCleanupRollsBackInjectedMidSetFailure(t *testing.T) {
	fixture := prepareCLIOnlyCleanupFixture(t)

	injected := errors.New("injected cleanup rename failure")
	fixture.candidate.renameFile = func(oldPath string, newPath string) error {
		if filepath.Clean(oldPath) == fixture.paths.diagnostics {
			return injected
		}
		return os.Rename(oldPath, newPath)
	}
	err := fixture.candidate.publish()
	if !errors.Is(err, injected) {
		t.Fatalf("cleanup error = %v, want injected rename failure", err)
	}
	assertArtifactContents(t, fixture.paths, fixture.previous)
	assertNoTombstones(t, filepath.Dir(fixture.paths.out))
}

// TestPreparedCLIOnlyCleanupJoinsRollbackFailure verifies the triggering cleanup error is not lost when restoration also fails.
func TestPreparedCLIOnlyCleanupJoinsRollbackFailure(t *testing.T) {
	fixture := prepareCLIOnlyCleanupFixture(t)

	cleanupErr := errors.New("injected cleanup failure")
	rollbackErr := errors.New("injected cleanup rollback failure")
	fixture.candidate.renameFile = func(oldPath string, newPath string) error {
		if filepath.Clean(oldPath) == fixture.paths.diagnostics {
			return cleanupErr
		}
		if filepath.Clean(newPath) == fixture.paths.out && strings.Contains(filepath.Base(oldPath), ".forj-api-index-remove-") {
			return rollbackErr
		}
		return os.Rename(oldPath, newPath)
	}
	err := fixture.candidate.publish()
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

// cliOnlyCleanupFixture keeps the deferred cleanup and its expected stale generation together.
type cliOnlyCleanupFixture struct {
	root      string
	paths     paths
	candidate *preparedCandidate
	previous  map[string][]byte
}

// prepareCLIOnlyCleanupFixture creates one deferred cleanup transaction with three distinct stale artifacts.
func prepareCLIOnlyCleanupFixture(t *testing.T) cliOnlyCleanupFixture {
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
	paths := rootDefaultPaths(root, defaultPaths(project.DefaultNamedApp("ship")))
	previous := previousArtifacts(paths)
	writeArtifacts(t, paths, previous)

	prepared, err := newTestRunner().prepareDefault(runOptions{root: root})
	if err != nil {
		t.Fatalf("prepare CLI-only cleanup fixture: %v", err)
	}
	pending := prepared.candidate
	if pending == nil || !pending.remove {
		t.Fatal("CLI-only fixture did not produce deferred cleanup")
	}
	return cliOnlyCleanupFixture{
		root:      root,
		paths:     paths,
		candidate: pending,
		previous:  previous,
	}
}

// stagedFixture owns a project root and the complete rooted artifact generation prepared beneath it.
type stagedFixture struct {
	root     string
	paths    paths
	previous map[string][]byte
}

// writeStagedFixture creates an API-capable app with a previous complete artifact set.
func writeStagedFixture(t *testing.T) stagedFixture {
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
	paths := rootDefaultPaths(root, defaultPaths(project.DefaultApp()))
	previous := previousArtifacts(paths)
	writeArtifacts(t, paths, previous)
	return stagedFixture{root: root, paths: paths, previous: previous}
}

// previousArtifacts returns distinct valid documents so tests can detect any premature replacement.
func previousArtifacts(paths paths) map[string][]byte {
	return map[string][]byte{
		paths.out:         []byte("{\"previous\":\"manifest\"}\n"),
		paths.diagnostics: []byte("{\"previous\":\"diagnostics\"}\n"),
		paths.openAPI:     []byte("{\"previous\":\"openapi\"}\n"),
	}
}

// writeArtifacts installs a complete artifact set at the fixture-owned rooted paths.
func writeArtifacts(t *testing.T, paths paths, contents map[string][]byte) {
	t.Helper()
	for _, path := range artifactPaths(paths) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create artifact directory: %v", err)
		}
		content, ok := contents[path]
		if !ok {
			content = []byte("{}\n")
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
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

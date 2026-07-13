package devwatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

type fakeDevWatchBackend struct {
	events  chan devWatchRawEvent
	updates chan devWatchBackendUpdate

	mu            sync.Mutex
	roots         []string
	shouldDescend func(string) bool
	stopOnce      sync.Once
}

type blockingFailDevWatchBackend struct {
	started chan struct{}
}

type failingStartDevWatchBackend struct {
	err error
}

// start blocks until shutdown so the engine's startup/close handoff is deterministic.
func (b *blockingFailDevWatchBackend) start(ctx context.Context, _ []string, _ func(string) bool) (devWatchBackendStart, error) {
	close(b.started)
	<-ctx.Done()
	return devWatchBackendStart{}, errors.New("backend startup failed")
}

// start returns a deterministic notification setup failure for automatic fallback tests.
func (b *failingStartDevWatchBackend) start(context.Context, []string, func(string) bool) (devWatchBackendStart, error) {
	return devWatchBackendStart{}, b.err
}

// newFakeDevWatchBackend returns a controllable physical event source for engine tests.
func newFakeDevWatchBackend() *fakeDevWatchBackend {
	return &fakeDevWatchBackend{
		events:  make(chan devWatchRawEvent, 32),
		updates: make(chan devWatchBackendUpdate, 8),
	}
}

// start captures shared coverage inputs and returns the fake event channels.
func (b *fakeDevWatchBackend) start(
	_ context.Context,
	roots []string,
	shouldDescend func(string) bool,
) (devWatchBackendStart, error) {
	b.mu.Lock()
	b.roots = slices.Clone(roots)
	b.shouldDescend = shouldDescend
	b.mu.Unlock()
	return devWatchBackendStart{
		events:             b.events,
		updates:            b.updates,
		watchedDirectories: len(roots),
		stop: func() error {
			b.stopOnce.Do(func() {
				close(b.events)
				close(b.updates)
			})
			return nil
		},
	}, nil
}

// TestDevWatchMatcherSemantics covers readable and explicit matcher behavior.
func TestDevWatchMatcherSemantics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		pattern  string
		matches  []string
		excludes []string
	}{
		{
			name:     "suffix",
			pattern:  ".go",
			matches:  []string{"main.go", "internal/app/main.go"},
			excludes: []string{"main.go.txt"},
		},
		{
			name:     "dotfile",
			pattern:  ".env",
			matches:  []string{".env", "cmd/app/.env"},
			excludes: []string{"production.env", ".env.local"},
		},
		{
			name:     "basename prefix",
			pattern:  ".env.*",
			matches:  []string{".env.local", "cmd/app/.env.test"},
			excludes: []string{".env", "app.env.local"},
		},
		{
			name:     "exact basename",
			pattern:  "package.json",
			matches:  []string{"package.json", "frontend/package.json"},
			excludes: []string{"package.json.bak"},
		},
		{
			name:     "exact path",
			pattern:  "./cmd/app/frontend/dist/index.html",
			matches:  []string{"cmd/app/frontend/dist/index.html"},
			excludes: []string{"frontend/dist/index.html", "cmd/app/frontend/dist/app.html"},
		},
		{
			name:     "regular expression",
			pattern:  `re:^schemas/.+\.(graphql|json)$`,
			matches:  []string{"schemas/app.graphql", "schemas/nested/app.json"},
			excludes: []string{"internal/schemas/app.graphql", "schemas/app.go"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			matcher, err := NewMatcher(test.pattern)
			if err != nil {
				t.Fatalf("NewMatcher() error = %v", err)
			}
			for _, candidate := range test.matches {
				if !matcher.Matches(candidate) {
					t.Errorf("matcher %q did not match %q", test.pattern, candidate)
				}
			}
			for _, candidate := range test.excludes {
				if matcher.Matches(candidate) {
					t.Errorf("matcher %q unexpectedly matched %q", test.pattern, candidate)
				}
			}
		})
	}
}

// TestDevWatchMatcherRejectsInvalidRegex keeps raw escape-hatch failures focused.
func TestDevWatchMatcherRejectsInvalidRegex(t *testing.T) {
	t.Parallel()
	_, err := NewMatcher("re:[")
	if err == nil || !strings.Contains(err.Error(), "compile watch regular expression") {
		t.Fatalf("NewMatcher() error = %v, want focused compile error", err)
	}
}

// TestLegacyRegexpMatcherSemantics covers wgo dot escaping, path trimming, and unanchored matching.
func TestLegacyRegexpMatcherSemantics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		pattern  string
		matches  []string
		excludes []string
	}{
		{
			name:     "extension dot becomes literal",
			pattern:  `^cmd/.+.go$`,
			matches:  []string{"cmd/main.go"},
			excludes: []string{"cmd/mainXgo"},
		},
		{
			name:     "current directory prefix is removed",
			pattern:  `./cmd/app/main.go$`,
			matches:  []string{"cmd/app/main.go", "other/cmd/app/main.go"},
			excludes: []string{"cmd/app/main.go.bak"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			matcher, err := NewLegacyRegexpMatcher(test.pattern)
			if err != nil {
				t.Fatalf("NewLegacyRegexpMatcher() error = %v", err)
			}
			for _, candidate := range test.matches {
				if !matcher.Matches(candidate) {
					t.Errorf("legacy pattern %q did not match %q", test.pattern, candidate)
				}
			}
			for _, candidate := range test.excludes {
				if matcher.Matches(candidate) {
					t.Errorf("legacy pattern %q unexpectedly matched %q", test.pattern, candidate)
				}
			}
		})
	}
}

// TestDevWatchEngineSharesRootsAndCoalesces verifies shared subscriptions and trailing-edge batching.
func TestDevWatchEngineSharesRootsAndCoalesces(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	goMatcher := mustDevWatchMatcher(t, ".go")
	fakeBackend := newFakeDevWatchBackend()
	engine, err := NewEngine(EngineConfig{
		Backend:        BackendNotify,
		backendForTest: fakeBackend,
		Watchers: []Spec{
			{Name: "app", Roots: []string{root}, Includes: []Matcher{goMatcher}, Debounce: 25 * time.Millisecond},
			{Name: "worker", Roots: []string{root}, Includes: []Matcher{goMatcher}, Debounce: 25 * time.Millisecond},
		},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	fakeBackend.mu.Lock()
	roots := slices.Clone(fakeBackend.roots)
	fakeBackend.mu.Unlock()
	if !reflect.DeepEqual(roots, []string{root}) {
		t.Fatalf("physical roots = %#v, want one shared root", roots)
	}

	changedPath := filepath.Join(root, "main.go")
	fakeBackend.events <- devWatchRawEvent{path: changedPath, op: OpCreate}
	fakeBackend.events <- devWatchRawEvent{path: changedPath, op: OpWrite}
	first := awaitDevWatchEvent(t, engine.Events(), 2*time.Second)
	second := awaitDevWatchEvent(t, engine.Events(), 2*time.Second)
	if got := []string{first.Watcher, second.Watcher}; !reflect.DeepEqual(got, []string{"app", "worker"}) {
		t.Fatalf("watcher batches = %#v, want config order", got)
	}
	for _, event := range []Event{first, second} {
		if len(event.Changes) != 1 {
			t.Fatalf("%s changes = %#v, want one coalesced change", event.Watcher, event.Changes)
		}
		wantOperation := OpCreate | OpWrite
		if event.Changes[0].Op != wantOperation {
			t.Fatalf("%s operation = %v, want %v", event.Watcher, event.Changes[0].Op, wantOperation)
		}
	}
	assertNoDevWatchEvent(t, engine.Events(), 75*time.Millisecond)
}

// TestDevWatchEngineSustainedEditorStormCoalesces verifies a long burst of
// truncate/write and atomic-save events remains one bounded trailing-edge batch.
func TestDevWatchEngineSustainedEditorStormCoalesces(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	targetPath := filepath.Join(root, "main.go")
	swapPath := filepath.Join(root, "main.swap.go")
	const cycles = 256

	// Editors report truncation as a write and atomic saves as create/write/rename
	// activity on a temporary file followed by replacement activity on the target.
	fakeBackend := newFakeDevWatchBackend()
	fakeBackend.events = make(chan devWatchRawEvent, cycles*5+1)
	engine, err := NewEngine(EngineConfig{
		Backend:        BackendNotify,
		backendForTest: fakeBackend,
		Watchers: []Spec{{
			Name: "app", Roots: []string{root}, Includes: []Matcher{mustDevWatchMatcher(t, ".go")},
			Debounce: 500 * time.Millisecond,
		}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	fakeBackend.events <- devWatchRawEvent{path: targetPath, op: OpCreate}
	for range cycles {
		fakeBackend.events <- devWatchRawEvent{path: targetPath, op: OpWrite}
		fakeBackend.events <- devWatchRawEvent{path: swapPath, op: OpCreate}
		fakeBackend.events <- devWatchRawEvent{path: swapPath, op: OpWrite}
		fakeBackend.events <- devWatchRawEvent{path: swapPath, op: OpRename}
		fakeBackend.events <- devWatchRawEvent{path: targetPath, op: OpCreate}
		time.Sleep(3 * time.Millisecond)
		select {
		case early := <-engine.Events():
			t.Fatalf("debounce published before the continuous editor storm settled: %#v", early)
		default:
		}
	}

	event := awaitDevWatchEvent(t, engine.Events(), 3*time.Second)
	if event.Watcher != "app" {
		t.Fatalf("Watcher = %q, want app", event.Watcher)
	}
	if len(event.Changes) != 2 {
		t.Fatalf("Changes = %#v, want two coalesced paths", event.Changes)
	}
	operations := make(map[string]Op, len(event.Changes))
	for _, change := range event.Changes {
		operations[change.Path] = change.Op
	}
	if got, want := operations[targetPath], OpCreate|OpWrite; got != want {
		t.Fatalf("target operations = %v, want %v", got, want)
	}
	if got, want := operations[swapPath], OpCreate|OpWrite|OpRename; got != want {
		t.Fatalf("swap operations = %v, want %v", got, want)
	}
	assertNoDevWatchEvent(t, engine.Events(), 650*time.Millisecond)
}

// TestDevWatchSpecDebounceDefaults distinguishes an omitted debounce from an intentional zero-duration setting.
func TestDevWatchSpecDebounceDefaults(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	normalized, _, err := normalizeDevWatchSpecs([]Spec{
		{Name: "default", Roots: []string{root}},
		{Name: "immediate", Roots: []string{root}, DebounceSet: true},
	})
	if err != nil {
		t.Fatalf("normalizeDevWatchSpecs() error = %v", err)
	}
	if normalized[0].Debounce != DefaultDebounce {
		t.Fatalf("omitted debounce = %s, want %s", normalized[0].Debounce, DefaultDebounce)
	}
	if normalized[1].Debounce != 0 {
		t.Fatalf("explicit zero debounce = %s, want 0s", normalized[1].Debounce)
	}
}

// TestDevWatchEngineRejectsInvalidPhysicalRoots keeps health from claiming coverage for files or symlink aliases.
func TestDevWatchEngineRejectsInvalidPhysicalRoots(t *testing.T) {
	base := t.TempDir()
	regularFile := filepath.Join(base, "root.go")
	if err := os.WriteFile(regularFile, []byte("package root\n"), 0o600); err != nil {
		t.Fatalf("write regular-file root: %v", err)
	}
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("create symlink target: %v", err)
	}
	symlink := filepath.Join(base, "linked-root")
	symlinkErr := os.Symlink(target, symlink)

	tests := []struct {
		name      string
		root      string
		wantError string
		skip      bool
	}{
		{name: "regular file", root: regularFile, wantError: "is not a directory"},
		{name: "symbolic link", root: symlink, wantError: "must not be a symbolic link", skip: symlinkErr != nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.skip {
				t.Skipf("symbolic links are unavailable: %v", symlinkErr)
			}
			engine, err := NewEngine(EngineConfig{Watchers: []Spec{{Name: "invalid", Roots: []string{test.root}}}})
			if err != nil {
				t.Fatalf("NewEngine() error = %v", err)
			}
			err = engine.Start(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Start() error = %v, want %q", err, test.wantError)
			}
			if health := engine.Health(); health.State != HealthDegraded || health.Err == nil {
				t.Fatalf("Health() = %#v, want degraded root validation", health)
			}
			if err := engine.Close(); err != nil {
				t.Fatalf("Close() after failed validation = %v", err)
			}
		})
	}
}

// TestDevWatchEngineDirectoryPruningUsesAllSubscribers protects shared-root exclusion behavior.
func TestDevWatchEngineDirectoryPruningUsesAllSubscribers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	vendor := filepath.Join(root, "vendor")
	fakeBackend := newFakeDevWatchBackend()
	engine, err := NewEngine(EngineConfig{
		Backend:        BackendNotify,
		backendForTest: fakeBackend,
		Watchers: []Spec{
			{
				Name:              "app",
				Roots:             []string{root},
				DirectoryExcludes: []Matcher{mustDevWatchMatcher(t, "vendor")},
			},
			{
				Name:  "vendor generator",
				Roots: []string{root},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	fakeBackend.mu.Lock()
	shouldDescend := fakeBackend.shouldDescend
	fakeBackend.mu.Unlock()
	if !shouldDescend(vendor) {
		t.Fatal("shared backend pruned a directory needed by another subscriber")
	}
}

// TestDevWatchLegacyDirectoryRegexSemantics preserves wgo full-path matching and default pruning.
func TestDevWatchLegacyDirectoryRegexSemantics(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	spec := Spec{
		Name:                 "legacy",
		Roots:                []string{root},
		Includes:             []Matcher{mustDevWatchMatcher(t, ".go")},
		DirectoryIncludes:    []Matcher{mustDevWatchMatcher(t, `re:^cmd$`)},
		DirectoryExcludes:    []Matcher{mustDevWatchMatcher(t, `re:^vendor$`)},
		LegacyDirectoryRegex: true,
	}
	if !devWatchSpecMatches(spec, "cmd/main.go") {
		t.Fatal("legacy directory include did not match the full relative directory")
	}
	if devWatchSpecMatches(spec, "cmd/nested/main.go") {
		t.Fatal("legacy directory include incorrectly inherited through an ancestor match")
	}
	if devWatchLegacyDirectoryAccessible(spec, "vendor/pkg") {
		t.Fatal("legacy directory exclusion did not prune its descendant subtree")
	}
	if devWatchSpecNeedsDirectory(spec, filepath.Join(root, "vendor")) {
		t.Fatal("legacy excluded directory remained in physical watch coverage")
	}
	if devWatchSpecNeedsDirectory(spec, filepath.Join(root, ".git")) {
		t.Fatal("legacy VCS directory bypassed wgo default pruning")
	}
	if devWatchSpecNeedsDirectory(spec, filepath.Join(root, "node_modules")) {
		t.Fatal("legacy module directory bypassed wgo default pruning")
	}

	includedHidden := spec
	includedHidden.DirectoryIncludes = []Matcher{mustDevWatchMatcher(t, `re:^\.github$`)}
	if !devWatchSpecNeedsDirectory(includedHidden, filepath.Join(root, ".github")) {
		t.Fatal("explicit legacy directory include did not override default hidden-directory pruning")
	}
}

// TestDevWatchLegacyNestedRootRestoresPrunedDescendants verifies ordered roots
// retain wgo's distinction between physical ancestor pruning and file matching.
func TestDevWatchLegacyNestedRootRestoresPrunedDescendants(t *testing.T) {
	t.Parallel()
	outer := t.TempDir()
	nested := filepath.Join(outer, "vendor")
	child := filepath.Join(nested, "pkg")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("create nested legacy root: %v", err)
	}
	exclude, err := NewLegacyRegexpMatcher(`^vendor$`)
	if err != nil {
		t.Fatalf("compile legacy directory exclusion: %v", err)
	}
	include, err := NewMatcher(".go")
	if err != nil {
		t.Fatalf("compile Go matcher: %v", err)
	}
	spec := Spec{
		Name:                 "legacy",
		Roots:                []string{outer},
		Includes:             []Matcher{include},
		DirectoryExcludes:    []Matcher{exclude},
		LegacyDirectoryRegex: true,
	}
	if devWatchSpecNeedsDirectory(spec, nested) {
		t.Fatal("outer root did not prune its excluded vendor subtree")
	}

	spec.Roots = append(spec.Roots, nested)
	if !devWatchSpecNeedsDirectory(spec, nested) {
		t.Fatal("explicit nested root did not restore physical coverage")
	}
	filePath := filepath.Join(child, "main.go")
	relativePath, ok := relativeDevWatchPath(spec.Roots, filePath)
	if !ok || relativePath != "vendor/pkg/main.go" {
		t.Fatalf("ordered root resolution = (%q, %t), want outer-relative path", relativePath, ok)
	}
	if !devWatchSpecMatches(spec, relativePath) {
		t.Fatal("legacy full-directory matching re-applied an ancestor-only exclusion")
	}
	if devWatchSpecMatches(spec, "vendor/main.go") {
		t.Fatal("legacy exclusion stopped matching its exact full relative directory")
	}
}

// TestDevWatchPollBackendLifecycleAndHealth covers snapshot changes and recoverable root failures.
func TestDevWatchPollBackendLifecycleAndHealth(t *testing.T) {
	root := t.TempDir()
	engine, err := NewEngine(EngineConfig{
		Backend:      BackendPoll,
		PollInterval: 15 * time.Millisecond,
		Watchers: []Spec{{
			Name:     "app",
			Roots:    []string{root},
			Includes: []Matcher{mustDevWatchMatcher(t, ".go")},
			Debounce: 5 * time.Millisecond,
		}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	filePath := filepath.Join(root, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write created file: %v", err)
	}
	assertDevWatchOperation(t, awaitDevWatchEvent(t, engine.Events(), 2*time.Second), OpCreate)

	if err := os.WriteFile(filePath, []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatalf("write changed file: %v", err)
	}
	assertDevWatchOperation(t, awaitDevWatchEvent(t, engine.Events(), 2*time.Second), OpWrite)

	if err := os.Remove(filePath); err != nil {
		t.Fatalf("remove watched file: %v", err)
	}
	assertDevWatchOperation(t, awaitDevWatchEvent(t, engine.Events(), 2*time.Second), OpRemove)

	if err := os.Remove(root); err != nil {
		t.Fatalf("remove watched root: %v", err)
	}
	degraded := awaitDevWatchHealth(t, engine.HealthUpdates(), HealthDegraded, 2*time.Second)
	if degraded.Err == nil {
		t.Fatal("degraded polling health did not retain its scan error")
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("restore watched root: %v", err)
	}
	awaitDevWatchHealth(t, engine.HealthUpdates(), HealthHealthy, 2*time.Second)
	if health := engine.Health(); health.State != HealthHealthy {
		t.Fatalf("Health().State = %q, want healthy", health.State)
	}
}

// TestDevWatchPollSnapshotDetectsSameMetadataReplacement proves editor renames cannot hide behind preserved metadata.
func TestDevWatchPollSnapshotDetectsSameMetadataReplacement(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "main.go")
	original := []byte("package one\n")
	replacement := []byte("package two\n")
	if len(original) != len(replacement) {
		t.Fatal("test fixture requires equal-length contents")
	}
	if err := os.WriteFile(filePath, original, 0o640); err != nil {
		t.Fatalf("write original file: %v", err)
	}
	originalInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat original file: %v", err)
	}
	before, err := snapshotDevWatchRoots(context.Background(), []string{root}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("snapshot original file: %v", err)
	}

	temporaryPath := filepath.Join(root, ".main.go.next")
	if err := os.WriteFile(temporaryPath, replacement, originalInfo.Mode().Perm()); err != nil {
		t.Fatalf("write replacement file: %v", err)
	}
	if err := os.Chtimes(temporaryPath, originalInfo.ModTime(), originalInfo.ModTime()); err != nil {
		t.Fatalf("restore replacement timestamps: %v", err)
	}
	if err := os.Rename(temporaryPath, filePath); err != nil {
		t.Fatalf("publish replacement file: %v", err)
	}
	replacementInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat replacement file: %v", err)
	}
	if replacementInfo.Size() != originalInfo.Size() || replacementInfo.Mode() != originalInfo.Mode() ||
		!replacementInfo.ModTime().Equal(originalInfo.ModTime()) {
		t.Fatalf(
			"replacement metadata = size %d mode %v mtime %v, want size %d mode %v mtime %v",
			replacementInfo.Size(),
			replacementInfo.Mode(),
			replacementInfo.ModTime(),
			originalInfo.Size(),
			originalInfo.Mode(),
			originalInfo.ModTime(),
		)
	}
	after, err := snapshotDevWatchRoots(context.Background(), []string{root}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("snapshot replacement file: %v", err)
	}

	events := make(chan devWatchRawEvent, 1)
	publishDevWatchSnapshotDiff(context.Background(), events, before, after)
	select {
	case event := <-events:
		if event.path != filePath || event.op != OpWrite {
			t.Fatalf("replacement event = %#v, want write for %q", event, filePath)
		}
	default:
		t.Fatal("same-metadata replacement produced no polling event")
	}
}

// TestDevWatchAutoFallbackReportsHealthyPolling ensures a recovered setup failure is not surfaced as degradation.
func TestDevWatchAutoFallbackReportsHealthyPolling(t *testing.T) {
	root := t.TempDir()
	engine, err := NewEngine(EngineConfig{
		Backend:        BackendAuto,
		PollInterval:   10 * time.Millisecond,
		backendForTest: &failingStartDevWatchBackend{err: errors.New("notifications unavailable")},
		Watchers: []Spec{{
			Name:        "app",
			Roots:       []string{root},
			Includes:    []Matcher{mustDevWatchMatcher(t, ".go")},
			Debounce:    5 * time.Millisecond,
			DebounceSet: true,
		}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	health := engine.Health()
	if health.State != HealthHealthy || health.Backend != BackendPoll || health.Err != nil {
		t.Fatalf("Health() = %#v, want healthy polling fallback without an error", health)
	}
	select {
	case reportedErr := <-engine.Errors():
		t.Fatalf("Errors() reported recovered notification setup failure: %v", reportedErr)
	case <-time.After(50 * time.Millisecond):
	}

	createdPath := filepath.Join(root, "fallback.go")
	if err := os.WriteFile(createdPath, []byte("package fallback\n"), 0o600); err != nil {
		t.Fatalf("write file for polling fallback: %v", err)
	}
	change := awaitDevWatchChange(t, engine.Events(), createdPath, 2*time.Second)
	if change.Op&OpCreate == 0 {
		t.Fatalf("polling fallback operation = %v, want create", change.Op)
	}
}

// TestDevWatchFSNotifyBackendRecursiveLifecycle covers new subtrees, rename, and removal events.
func TestDevWatchFSNotifyBackendRecursiveLifecycle(t *testing.T) {
	root := t.TempDir()
	engine, err := NewEngine(EngineConfig{
		Backend: BackendNotify,
		Watchers: []Spec{{
			Name:     "app",
			Roots:    []string{root},
			Includes: []Matcher{mustDevWatchMatcher(t, ".go")},
			Debounce: 20 * time.Millisecond,
		}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	nested := filepath.Join(root, "new", "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	createdPath := filepath.Join(nested, "created.go")
	if err := os.WriteFile(createdPath, []byte("package nested\n"), 0o600); err != nil {
		t.Fatalf("create nested Go file: %v", err)
	}
	createdEvent := awaitDevWatchEvent(t, engine.Events(), 2*time.Second)
	if createdEvent.Changes[0].Op&(OpCreate|OpWrite) == 0 {
		t.Fatalf("nested create operation = %v, want create or write", createdEvent.Changes[0].Op)
	}

	renamedPath := filepath.Join(nested, "renamed.go")
	if err := os.Rename(createdPath, renamedPath); err != nil {
		t.Fatalf("rename watched file: %v", err)
	}
	renameEvent := awaitDevWatchEvent(t, engine.Events(), 2*time.Second)
	if !devWatchEventHasOperation(renameEvent, OpRename) {
		t.Fatalf("rename changes = %#v, want rename operation", renameEvent.Changes)
	}

	if err := os.Remove(renamedPath); err != nil {
		t.Fatalf("remove watched file: %v", err)
	}
	removeEvent := awaitDevWatchEvent(t, engine.Events(), 2*time.Second)
	if !devWatchEventHasOperation(removeEvent, OpRemove) {
		t.Fatalf("remove changes = %#v, want remove operation", removeEvent.Changes)
	}
}

// TestDevWatchFSNotifyBackendRecoversOutermostRoot covers deletion, recreation, and atomic root replacement.
func TestDevWatchFSNotifyBackendRecoversOutermostRoot(t *testing.T) {
	tests := []struct {
		name    string
		restore func(*testing.T, string, string) string
	}{
		{
			name: "delete and recreate",
			restore: func(t *testing.T, base string, root string) string {
				t.Helper()
				if err := os.RemoveAll(root); err != nil {
					t.Fatalf("remove watched root: %v", err)
				}
				return ""
			},
		},
		{
			name: "atomic replacement",
			restore: func(t *testing.T, base string, root string) string {
				t.Helper()
				displaced := filepath.Join(base, "displaced")
				if err := os.Rename(root, displaced); err != nil {
					t.Fatalf("rename watched root away: %v", err)
				}
				return filepath.Join(base, "replacement")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "watched")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatalf("create watched root: %v", err)
			}
			initialPath := filepath.Join(root, "initial.go")
			if err := os.WriteFile(initialPath, []byte("package initial\n"), 0o600); err != nil {
				t.Fatalf("write initial file: %v", err)
			}
			replacement := filepath.Join(base, "replacement")
			if test.name == "atomic replacement" {
				if err := os.Mkdir(replacement, 0o700); err != nil {
					t.Fatalf("create replacement root: %v", err)
				}
			}

			engine, err := NewEngine(EngineConfig{
				Backend: BackendNotify,
				Watchers: []Spec{{
					Name:        "app",
					Roots:       []string{root},
					Includes:    []Matcher{mustDevWatchMatcher(t, ".go")},
					Debounce:    5 * time.Millisecond,
					DebounceSet: true,
				}},
			})
			if err != nil {
				t.Fatalf("NewEngine() error = %v", err)
			}
			if err := engine.Start(context.Background()); err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			t.Cleanup(func() { _ = engine.Close() })

			replacement = test.restore(t, base, root)
			degraded := awaitDevWatchHealth(t, engine.HealthUpdates(), HealthDegraded, 2*time.Second)
			if degraded.Err == nil {
				t.Fatal("root removal did not retain its coverage error")
			}

			if replacement == "" {
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatalf("recreate watched root: %v", err)
				}
			} else if err := os.Rename(replacement, root); err != nil {
				t.Fatalf("install replacement root: %v", err)
			}
			awaitDevWatchHealth(t, engine.HealthUpdates(), HealthHealthy, 2*time.Second)

			recoveredPath := filepath.Join(root, "recovered.go")
			if err := os.WriteFile(recoveredPath, []byte("package recovered\n"), 0o600); err != nil {
				t.Fatalf("write file beneath recovered root: %v", err)
			}
			change := awaitDevWatchChange(t, engine.Events(), recoveredPath, 2*time.Second)
			if change.Op&(OpCreate|OpWrite) == 0 {
				t.Fatalf("recovered root operation = %v, want create or write", change.Op)
			}
			if health := engine.Health(); health.State != HealthHealthy || health.Err != nil {
				t.Fatalf("Health() after root recovery = %#v, want healthy without an error", health)
			}
		})
	}
}

// TestDevWatchFSNotifyRecoveryPublishesPartialRootDiscoveries prevents later unavailable roots from swallowing events.
func TestDevWatchFSNotifyRecoveryPublishesPartialRootDiscoveries(t *testing.T) {
	base := t.TempDir()
	firstRoot := filepath.Join(base, "first")
	secondRoot := filepath.Join(base, "second")
	if err := os.Mkdir(firstRoot, 0o700); err != nil {
		t.Fatalf("create first root: %v", err)
	}
	discoveredPath := filepath.Join(firstRoot, "discovered.go")
	if err := os.WriteFile(discoveredPath, []byte("package discovered\n"), 0o600); err != nil {
		t.Fatalf("write discovered file: %v", err)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("fsnotify.NewWatcher() error = %v", err)
	}
	t.Cleanup(func() { _ = watcher.Close() })

	watched := make(map[string]struct{})
	recoveryDirectories := make(map[string]struct{})
	knownDirectories := make(map[string]struct{})
	knownFiles := make(map[string]struct{})
	events := make(chan devWatchRawEvent, 8)
	updates := make(chan devWatchBackendUpdate, 8)
	state := devWatchFSNotifyRuntimeState{retryNeeded: true}
	retryDevWatchFSNotifyCoverage(
		context.Background(),
		watcher,
		[]string{firstRoot, secondRoot},
		func(string) bool { return true },
		watched,
		recoveryDirectories,
		knownDirectories,
		knownFiles,
		events,
		updates,
		&state,
	)
	select {
	case event := <-events:
		if event.path != discoveredPath || event.op != OpCreate {
			t.Fatalf("partial recovery event = %#v, want create for %q", event, discoveredPath)
		}
	default:
		t.Fatal("partial recovery swallowed a discovered file event")
	}
	if state.healthy {
		t.Fatal("partial recovery reported healthy while the second root was unavailable")
	}

	if err := os.Mkdir(secondRoot, 0o700); err != nil {
		t.Fatalf("restore second root: %v", err)
	}
	retryDevWatchFSNotifyCoverage(
		context.Background(),
		watcher,
		[]string{firstRoot, secondRoot},
		func(string) bool { return true },
		watched,
		recoveryDirectories,
		knownDirectories,
		knownFiles,
		events,
		updates,
		&state,
	)
	if !state.healthy || state.retryNeeded {
		t.Fatalf("completed recovery state = %#v, want healthy", state)
	}
	select {
	case event := <-events:
		t.Fatalf("completed recovery repeated a known discovery: %#v", event)
	default:
	}
}

// TestDevWatchFSNotifyIgnoresVanishedNonRootScan verifies temporary directories do not degrade root coverage.
func TestDevWatchFSNotifyIgnoresVanishedNonRootScan(t *testing.T) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("fsnotify.NewWatcher() error = %v", err)
	}
	t.Cleanup(func() { _ = watcher.Close() })
	root := t.TempDir()
	err = addDevWatchDirectories(
		context.Background(),
		watcher,
		filepath.Join(root, "already-gone"),
		false,
		func(string) bool { return true },
		make(map[string]struct{}),
		make(map[string]struct{}),
		make(map[string]struct{}),
		nil,
	)
	if err != nil {
		t.Fatalf("optional vanished subtree error = %v, want benign completion", err)
	}
}

// TestDevWatchPollPathErrorDistinguishesRootLoss keeps transient entries healthy without hiding root removal.
func TestDevWatchPollPathErrorDistinguishesRootLoss(t *testing.T) {
	root := filepath.Join(t.TempDir(), "watched")
	if err := devWatchPollPathError(root, filepath.Join(root, "transient.go"), os.ErrNotExist); err != nil {
		t.Fatalf("transient non-root error = %v, want nil", err)
	}
	if err := devWatchPollPathError(root, root, os.ErrNotExist); err == nil {
		t.Fatal("root disappearance error = nil, want coverage failure")
	}
}

// TestDevWatchFSNotifyBackendDiscoversMissingNestedRoot protects explicit roots that restore an excluded subtree.
func TestDevWatchFSNotifyBackendDiscoversMissingNestedRoot(t *testing.T) {
	outerRoot := t.TempDir()
	nestedRoot := filepath.Join(outerRoot, "generated", "client")
	engine, err := NewEngine(EngineConfig{
		Backend: BackendNotify,
		Watchers: []Spec{{
			Name:              "generated client",
			Roots:             []string{nestedRoot, outerRoot},
			Includes:          []Matcher{mustDevWatchMatcher(t, ".go")},
			DirectoryExcludes: []Matcher{mustDevWatchMatcher(t, "generated")},
			Debounce:          10 * time.Millisecond,
			DebounceSet:       true,
		}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	if err := os.MkdirAll(nestedRoot, 0o700); err != nil {
		t.Fatalf("create explicit nested root: %v", err)
	}
	createdPath := filepath.Join(nestedRoot, "client.go")
	if err := os.WriteFile(createdPath, []byte("package client\n"), 0o600); err != nil {
		t.Fatalf("create file beneath explicit nested root: %v", err)
	}
	event := awaitDevWatchEvent(t, engine.Events(), 2*time.Second)
	if len(event.Changes) != 1 || event.Changes[0].RelativePath != "client.go" {
		t.Fatalf("nested root changes = %#v, want one root-relative client.go change", event.Changes)
	}
}

// TestDevWatchFSNotifyBackendReportsRemovedSubtreeFiles keeps notify lifecycle parity with polling snapshots.
func TestDevWatchFSNotifyBackendReportsRemovedSubtreeFiles(t *testing.T) {
	tests := []struct {
		name      string
		operation Op
		remove    func(string, string) error
	}{
		{
			name:      "remove",
			operation: OpRemove,
			remove: func(subtree string, _ string) error {
				return os.RemoveAll(subtree)
			},
		},
		{
			name:      "rename",
			operation: OpRename,
			remove: func(subtree string, destination string) error {
				return os.Rename(subtree, destination)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "watched")
			subtree := filepath.Join(root, "nested")
			if err := os.MkdirAll(subtree, 0o700); err != nil {
				t.Fatalf("create watched subtree: %v", err)
			}
			includedPath := filepath.Join(subtree, "included.go")
			if err := os.WriteFile(includedPath, []byte("package nested\n"), 0o600); err != nil {
				t.Fatalf("create included file: %v", err)
			}

			engine, err := NewEngine(EngineConfig{
				Backend: BackendNotify,
				Watchers: []Spec{{
					Name:        "app",
					Roots:       []string{root},
					Includes:    []Matcher{mustDevWatchMatcher(t, ".go")},
					Debounce:    10 * time.Millisecond,
					DebounceSet: true,
				}},
			})
			if err != nil {
				t.Fatalf("NewEngine() error = %v", err)
			}
			if err := engine.Start(context.Background()); err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			t.Cleanup(func() { _ = engine.Close() })

			if err := test.remove(subtree, filepath.Join(base, "renamed")); err != nil {
				t.Fatalf("%s watched subtree: %v", test.name, err)
			}
			event := awaitDevWatchEvent(t, engine.Events(), 2*time.Second)
			if !devWatchEventContainsChange(event, includedPath, test.operation) {
				t.Fatalf("%s changes = %#v, want operation %v for included file", test.name, event.Changes, test.operation)
			}
		})
	}
}

// TestDevWatchEngineCloseStopsDelivery verifies clean, idempotent shutdown.
func TestDevWatchEngineCloseStopsDelivery(t *testing.T) {
	t.Parallel()
	fakeBackend := newFakeDevWatchBackend()
	engine, err := NewEngine(EngineConfig{
		Backend:        BackendNotify,
		backendForTest: fakeBackend,
		Watchers:       []Spec{{Name: "app", Roots: []string{t.TempDir()}}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if health := engine.Health(); health.State != HealthStopped {
		t.Fatalf("Health().State = %q, want stopped", health.State)
	}
	if _, open := <-engine.Events(); open {
		t.Fatal("Events() remained open after Close()")
	}
}

// TestDevWatchEngineCloseDuringFailedStart verifies Close cannot wait on a run loop startup never launched.
func TestDevWatchEngineCloseDuringFailedStart(t *testing.T) {
	t.Parallel()
	backend := &blockingFailDevWatchBackend{started: make(chan struct{})}
	engine, err := NewEngine(EngineConfig{
		Backend:        BackendNotify,
		backendForTest: backend,
		Watchers:       []Spec{{Name: "app", Roots: []string{t.TempDir()}}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	startResult := make(chan error, 1)
	go func() {
		startResult <- engine.Start(context.Background())
	}()
	<-backend.started
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- engine.Close()
	}()
	select {
	case err := <-startResult:
		if err == nil {
			t.Fatal("Start() error = nil, want backend startup failure")
		}
	case <-time.After(time.Second):
		t.Fatal("Start() remained blocked after concurrent Close()")
	}
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() remained blocked after startup failed")
	}
}

// TestDevWatchNotifyIgnoresRemovedPrunedDirectory keeps legacy no-file watchers from treating directories as files.
func TestDevWatchNotifyIgnoresRemovedPrunedDirectory(t *testing.T) {
	root := t.TempDir()
	ignored := filepath.Join(root, "node_modules")
	if err := os.MkdirAll(ignored, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	engine, err := NewEngine(EngineConfig{
		Backend: BackendNotify,
		Watchers: []Spec{{
			Name: "legacy", Roots: []string{root}, LegacyDirectoryRegex: true,
		}},
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = engine.Close() }()
	if err := os.RemoveAll(ignored); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}
	select {
	case event := <-engine.Events():
		t.Fatalf("Events() emitted pruned directory removal: %#v", event)
	case <-time.After(300 * time.Millisecond):
	}
}

// mustDevWatchMatcher compiles a matcher or stops the current test.
func mustDevWatchMatcher(t *testing.T, pattern string) Matcher {
	t.Helper()
	matcher, err := NewMatcher(pattern)
	if err != nil {
		t.Fatalf("NewMatcher(%q) error = %v", pattern, err)
	}
	return matcher
}

// awaitDevWatchEvent waits for one logical watcher batch.
func awaitDevWatchEvent(t *testing.T, events <-chan Event, timeout time.Duration) Event {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("dev watch event channel closed")
		}
		return event
	case <-time.After(timeout):
		t.Fatal("timed out waiting for dev watch event")
		return Event{}
	}
}

// awaitDevWatchChange ignores unrelated lifecycle batches until the requested path is observed.
func awaitDevWatchChange(t *testing.T, events <-chan Event, changedPath string, timeout time.Duration) Change {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("dev watch event channel closed")
			}
			for _, change := range event.Changes {
				if change.Path == changedPath {
					return change
				}
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for dev watch change %q", changedPath)
			return Change{}
		}
	}
}

// assertNoDevWatchEvent verifies that debounce did not leak an extra batch.
func assertNoDevWatchEvent(t *testing.T, events <-chan Event, duration time.Duration) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("unexpected dev watch event: %#v", event)
	case <-time.After(duration):
	}
}

// assertDevWatchOperation verifies a single-change polling event.
func assertDevWatchOperation(t *testing.T, event Event, operation Op) {
	t.Helper()
	if len(event.Changes) != 1 || event.Changes[0].Op != operation {
		t.Fatalf("changes = %#v, want one %v operation", event.Changes, operation)
	}
}

// awaitDevWatchHealth waits for a particular physical coverage state.
func awaitDevWatchHealth(
	t *testing.T,
	updates <-chan Health,
	want HealthState,
	timeout time.Duration,
) Health {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case health, ok := <-updates:
			if !ok {
				t.Fatal("dev watch health channel closed")
			}
			if health.State == want {
				return health
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for dev watch health %q", want)
			return Health{}
		}
	}
}

// devWatchEventHasOperation reports whether any coalesced path contains an operation.
func devWatchEventHasOperation(event Event, operation Op) bool {
	for _, change := range event.Changes {
		if change.Op&operation != 0 {
			return true
		}
	}
	return false
}

// devWatchEventContainsChange reports whether one path contains the expected coalesced operation.
func devWatchEventContainsChange(event Event, changedPath string, operation Op) bool {
	for _, change := range event.Changes {
		if change.Path == changedPath && change.Op&operation != 0 {
			return true
		}
	}
	return false
}

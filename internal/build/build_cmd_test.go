package build

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/apiindex"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/web/webindex"
)

type stubAPIIndexer struct {
	root string
}

// Prepare writes lightweight artifacts so build tests can focus on pipeline sequencing.
func (s stubAPIIndexer) Prepare(apiindex.Options) (apiindex.Preparation, error) {
	if err := s.writeArtifacts(); err != nil {
		return apiindex.Preparation{Status: "app app, rejected, 0 operations, 0 schemas, 0 diagnostics"}, err
	}
	return apiindex.Preparation{Status: "app app, changed, 0 operations, 0 schemas, 0 diagnostics"}, nil
}

// writeArtifacts preserves the original build integration fixture without exposing Runner internals.
func (s stubAPIIndexer) writeArtifacts() error {
	if err := os.MkdirAll(filepath.Join(s.root, "build"), 0o755); err != nil {
		return err
	}
	for _, name := range []string{"api_index.json", "api_index.diagnostics.json", "openapi.json"} {
		if err := os.WriteFile(filepath.Join(s.root, "build", name), []byte("{}"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// TestCmdRunExecutesBuildPipeline verifies a direct build follows the same transactional indexing boundary as watcher builds.
func TestCmdRunExecutesBuildPipeline(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":          "module example.com/test\n\ngo 1.24\n",
		"cmd/app/main.go": "package main\nfunc main() {}\n",
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	appLogger := logger.NewSilentLogger()
	build := NewCmd(appLogger, stubAPIIndexer{root: root})
	build.Root = root
	if err := build.Run(); err != nil {
		t.Fatalf("build run failed: %v", err)
	}

	for _, p := range []string{
		filepath.Join(root, "build", "api_index.json"),
		filepath.Join(root, "build", "api_index.diagnostics.json"),
		filepath.Join(root, "build", "openapi.json"),
		filepath.Join(root, "bin", "app"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected artifact %s: %v", p, err)
		}
	}
}

func TestRunPlainGoBuildPublishesExecutableAtomically(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":          "module example.com/test\n\ngo 1.24\n",
		"cmd/app/main.go": "package main\nfunc main() {}\n",
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	binPath := filepath.Join(root, "bin", "app")
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("not executable"), 0o644); err != nil {
		t.Fatalf("write stale binary: %v", err)
	}
	legacyCachePath := filepath.Join(root, "bin", ".forj-build-cache", "app")
	if err := os.MkdirAll(filepath.Dir(legacyCachePath), 0o755); err != nil {
		t.Fatalf("mkdir legacy cache: %v", err)
	}
	if err := os.WriteFile(legacyCachePath, []byte("old shared cache binary"), 0o755); err != nil {
		t.Fatalf("write legacy cache binary: %v", err)
	}
	cmd := &Cmd{Root: root}
	if _, err := cmd.runPlainGoBuild(root, []string{"-o", "./bin/app", "./cmd/app"}); err != nil {
		t.Fatalf("runPlainGoBuild returned error: %v", err)
	}
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("stat built binary: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Fatalf("expected built binary to be executable, mode %s", info.Mode())
	}
	publishPaths, err := filepath.Glob(filepath.Join(root, "bin", ".app.*.publish"))
	if err != nil {
		t.Fatalf("glob publish temporary build outputs: %v", err)
	}
	if len(publishPaths) != 0 {
		t.Fatalf("expected publish temporary build output to be cleaned up, got %v", publishPaths)
	}
	if _, err := os.Stat(filepath.Join(root, "bin", ".app.ready")); err != nil {
		t.Fatalf("expected build ready stamp: %v", err)
	}
	cachePath := filepath.Join(root, "bin", ".forj-build-cache", "app.target", "app")
	cacheInfo, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("stat stable build cache output: %v", err)
	}
	if runtime.GOOS != "windows" && cacheInfo.Mode()&0o111 == 0 {
		t.Fatalf("expected cached build output to be executable, mode %s", cacheInfo.Mode())
	}
	if _, err := os.Stat(legacyCachePath); !os.IsNotExist(err) {
		t.Fatalf("expected superseded stable build output to be removed, got %v", err)
	}
	cacheEntries, err := os.ReadDir(filepath.Dir(cachePath))
	if err != nil {
		t.Fatalf("read target build cache dir: %v", err)
	}
	cacheEntryNames := make([]string, 0, len(cacheEntries))
	for _, entry := range cacheEntries {
		cacheEntryNames = append(cacheEntryNames, entry.Name())
	}
	wantCacheEntries := []string{webindex.ArtifactPublicationLockFilename, "app"}
	if !reflect.DeepEqual(cacheEntryNames, wantCacheEntries) {
		t.Fatalf("target build cache entries = %v, want %v", cacheEntryNames, wantCacheEntries)
	}
}

// TestRunPlainGoBuildReusesStableLinkerOutput verifies unchanged builds retain the stable output path that lets Go reuse linker work.
func TestRunPlainGoBuildReusesStableLinkerOutput(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/stablebuild\n\ngo 1.24\n",
		"cmd/app/main.go": `package main

import "fmt"

func main() { fmt.Print("ready") }
`,
	}
	for relativePath, contents := range files {
		absolutePath := filepath.Join(root, relativePath)
		if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
			t.Fatalf("create %s directory: %v", relativePath, err)
		}
		if err := os.WriteFile(absolutePath, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", relativePath, err)
		}
	}

	binaryName := "app"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	command := &Cmd{Root: root}
	args := []string{"-o", filepath.ToSlash(filepath.Join(".", "bin", binaryName)), "./cmd/app"}
	if _, err := command.runPlainGoBuild(root, args); err != nil {
		t.Fatalf("first runPlainGoBuild: %v", err)
	}
	cachePath := filepath.Join(root, "bin", ".forj-build-cache", binaryName+".target", binaryName)
	firstCacheInfo, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("stat first stable output: %v", err)
	}

	if _, err := command.runPlainGoBuild(root, args); err != nil {
		t.Fatalf("second runPlainGoBuild: %v", err)
	}
	cacheInfo, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("stat reused stable output: %v", err)
	}
	if !os.SameFile(firstCacheInfo, cacheInfo) {
		t.Fatal("stable output was replaced during an unchanged build")
	}
	output, err := exec.Command(filepath.Join(root, "bin", binaryName)).CombinedOutput()
	if err != nil {
		t.Fatalf("run published executable: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if string(output) != "ready" {
		t.Fatalf("published executable output = %q, want ready", output)
	}
}

// TestPlanAtomicGoBuildReusesStableOutputWithUniquePublicationPaths verifies overlapping builds share only the serialized linker target.
func TestPlanAtomicGoBuildReusesStableOutputWithUniquePublicationPaths(t *testing.T) {
	root := t.TempDir()
	firstPlan, err := planAtomicGoBuild(root, []string{"-o", "./bin/app", "./cmd/app"})
	if err != nil {
		t.Fatalf("plan first atomic build: %v", err)
	}
	if firstPlan == nil {
		t.Fatal("expected first build output to use atomic publishing")
	}
	defer firstPlan.cleanup()

	secondPlan, err := planAtomicGoBuild(root, []string{"-o", "./bin/app", "./cmd/app"})
	if err != nil {
		t.Fatalf("plan second atomic build: %v", err)
	}
	if secondPlan == nil {
		t.Fatal("expected second build output to use atomic publishing")
	}
	defer secondPlan.cleanup()

	if firstPlan.build != secondPlan.build {
		t.Fatalf("expected stable build output, got %q and %q", firstPlan.build, secondPlan.build)
	}
	if !reflect.DeepEqual(firstPlan.args, secondPlan.args) {
		t.Fatalf("expected stable go build args, got %#v and %#v", firstPlan.args, secondPlan.args)
	}
	if firstPlan.publish == secondPlan.publish {
		t.Fatalf("expected unique publication outputs, got %q", firstPlan.publish)
	}
	if firstPlan.ready != filepath.Join(root, "bin", ".app.ready") || secondPlan.ready != filepath.Join(root, "bin", ".app.ready") {
		t.Fatalf("expected shared ready stamp path, got %q and %q", firstPlan.ready, secondPlan.ready)
	}
	if filepath.Dir(firstPlan.build) != filepath.Join(root, "bin", ".forj-build-cache", "app.target") {
		t.Fatalf("expected per-target hidden build output dir, got %q", firstPlan.build)
	}
	if firstPlan.legacy != filepath.Join(root, "bin", ".forj-build-cache", "app") || secondPlan.legacy != firstPlan.legacy {
		t.Fatalf("expected shared superseded build output, got %q and %q", firstPlan.legacy, secondPlan.legacy)
	}
	for _, path := range []string{firstPlan.publish, secondPlan.publish} {
		if filepath.Dir(path) != filepath.Join(root, "bin") {
			t.Fatalf("expected publication output beside final executable, got %q", path)
		}
	}
	if strings.Contains(strings.Join(firstPlan.args, " "), "./bin/app") || strings.Contains(strings.Join(secondPlan.args, " "), "./bin/app") {
		t.Fatalf("expected go build args to avoid final output path, got %#v and %#v", firstPlan.args, secondPlan.args)
	}
}

// TestAtomicBuildPlanCleanupSupersededOutputPreservesDeterministicTarget keeps migration cleanup away from the reusable linker output.
func TestAtomicBuildPlanCleanupSupersededOutputPreservesDeterministicTarget(t *testing.T) {
	root := t.TempDir()
	stable := filepath.Join(root, ".forj-build-cache", "app.target", "app")
	legacy := filepath.Join(root, ".forj-build-cache", "app")
	if err := os.MkdirAll(filepath.Dir(stable), 0o755); err != nil {
		t.Fatalf("create stable output directory: %v", err)
	}
	if err := os.WriteFile(stable, []byte("stable"), 0o755); err != nil {
		t.Fatalf("write stable output: %v", err)
	}
	if err := os.WriteFile(legacy, []byte("legacy"), 0o755); err != nil {
		t.Fatalf("write legacy output: %v", err)
	}

	plan := &atomicBuildPlan{build: stable, legacy: legacy}
	plan.cleanupSupersededBuildOutput()

	contents, err := os.ReadFile(stable)
	if err != nil {
		t.Fatalf("read deterministic target: %v", err)
	}
	if string(contents) != "stable" {
		t.Fatalf("deterministic target = %q, want stable", contents)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("superseded output remains after cleanup: %v", err)
	}
}

// TestPlanAtomicGoBuildIsolatesStableOutputsPerTarget verifies independent app targets do not contend on one directory-scoped lock.
func TestPlanAtomicGoBuildIsolatesStableOutputsPerTarget(t *testing.T) {
	root := t.TempDir()
	appPlan, err := planAtomicGoBuild(root, []string{"-o", "./bin/app", "./cmd/app"})
	if err != nil {
		t.Fatalf("plan app build: %v", err)
	}
	reportingPlan, err := planAtomicGoBuild(root, []string{"-o", "./bin/reporting", "./cmd/reporting"})
	if err != nil {
		t.Fatalf("plan reporting build: %v", err)
	}
	defer appPlan.cleanup()
	defer reportingPlan.cleanup()

	if filepath.Dir(appPlan.build) == filepath.Dir(reportingPlan.build) {
		t.Fatalf("independent target build outputs share lock directory %q", filepath.Dir(appPlan.build))
	}
	if appPlan.build != filepath.Join(root, "bin", ".forj-build-cache", "app.target", "app") {
		t.Fatalf("app stable output = %q", appPlan.build)
	}
	if reportingPlan.build != filepath.Join(root, "bin", ".forj-build-cache", "reporting.target", "reporting") {
		t.Fatalf("reporting stable output = %q", reportingPlan.build)
	}
}

// TestUniqueBuildPublicationNameRemainsUniqueConcurrently verifies rapid builds cannot share a publication path.
func TestUniqueBuildPublicationNameRemainsUniqueConcurrently(t *testing.T) {
	const count = 128
	names := make(chan string, count)
	var builds sync.WaitGroup
	for range count {
		builds.Add(1)
		go func() {
			defer builds.Done()
			names <- uniqueBuildPublicationName("app")
		}()
	}
	builds.Wait()
	close(names)

	seen := make(map[string]struct{}, count)
	for name := range names {
		if _, exists := seen[name]; exists {
			t.Fatalf("uniqueBuildPublicationName() repeated %q", name)
		}
		seen[name] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("uniqueBuildPublicationName() produced %d names, want %d", len(seen), count)
	}
}

// TestPublishBuiltBinaryFallsBackToPrivateCopy verifies filesystems without hard links retain complete atomic publication.
func TestPublishBuiltBinaryFallsBackToPrivateCopy(t *testing.T) {
	root := t.TempDir()
	plan := &atomicBuildPlan{
		build:   filepath.Join(root, ".forj-build-cache", "app.target", "app"),
		publish: filepath.Join(root, ".app.fallback.publish"),
		final:   filepath.Join(root, "app"),
	}
	if err := os.MkdirAll(filepath.Dir(plan.build), 0o755); err != nil {
		t.Fatalf("create stable output directory: %v", err)
	}
	if err := os.WriteFile(plan.build, []byte("complete binary"), 0o755); err != nil {
		t.Fatalf("write stable output: %v", err)
	}
	if err := os.WriteFile(plan.final, []byte("previous binary"), 0o755); err != nil {
		t.Fatalf("write previous published output: %v", err)
	}

	linkErr := errors.New("hard links unavailable")
	if err := publishBuiltBinaryWithLink(plan, func(string, string) error { return linkErr }); err != nil {
		t.Fatalf("publish with copy fallback: %v", err)
	}
	contents, err := os.ReadFile(plan.final)
	if err != nil {
		t.Fatalf("read published output: %v", err)
	}
	if string(contents) != "complete binary" {
		t.Fatalf("published output = %q", contents)
	}
	if _, err := os.Stat(plan.build); err != nil {
		t.Fatalf("stable output was not retained: %v", err)
	}
	if _, err := os.Stat(plan.publish); !os.IsNotExist(err) {
		t.Fatalf("private publication path remains after rename: %v", err)
	}
}

// TestConcurrentAtomicBuildPublicationNeverLaunchesPartialBinary verifies
// overlapping publishers cannot expose a missing, malformed, or partial executable.
func TestConcurrentAtomicBuildPublicationNeverLaunchesPartialBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows locks running executables, while dev launches snapshots instead of the published path")
	}
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/publicationrace\n\ngo 1.24\n",
		"cmd/alpha/main.go": `package main

import "fmt"

func main() { fmt.Print("alpha") }
`,
		"cmd/beta/main.go": `package main

import "fmt"

func main() { fmt.Print("beta") }
`,
	}
	for relativePath, contents := range files {
		absolutePath := filepath.Join(root, relativePath)
		if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", relativePath, err)
		}
		if err := os.WriteFile(absolutePath, []byte(contents), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", relativePath, err)
		}
	}

	buildPackage := func(packagePath string) error {
		command := &Cmd{Root: root}
		_, buildErr := command.runPlainGoBuild(root, []string{"-o", "./bin/app", packagePath})
		return buildErr
	}
	if err := buildPackage("./cmd/alpha"); err != nil {
		t.Fatalf("seed published binary: %v", err)
	}

	type launchResult struct {
		count int
		err   error
	}
	stopLaunches := make(chan struct{})
	launchStarted := make(chan struct{})
	launchResults := make(chan launchResult, 1)
	var successfulLaunches atomic.Int64
	binaryPath := filepath.Join(root, "bin", "app")
	go func() {
		result := launchResult{}
		defer func() { launchResults <- result }()
		for {
			select {
			case <-stopLaunches:
				return
			default:
			}
			output, runErr := exec.Command(binaryPath).CombinedOutput()
			if runErr != nil {
				result.err = fmt.Errorf("launch published binary: %w: %s", runErr, strings.TrimSpace(string(output)))
				return
			}
			version := strings.TrimSpace(string(output))
			if version != "alpha" && version != "beta" {
				result.err = fmt.Errorf("published binary returned incomplete version %q", version)
				return
			}
			result.count++
			successfulLaunches.Add(1)
			if result.count == 1 {
				close(launchStarted)
			}
		}
	}()
	select {
	case <-launchStarted:
	case result := <-launchResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		t.Fatal("binary launch loop stopped before its first successful execution")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the first published binary launch")
	}

	const iterations = 6
	publishersReady := make(chan struct{}, 2)
	beginBuilds := make(chan struct{})
	buildErrors := make(chan error, 2)
	var builds sync.WaitGroup
	for _, packagePath := range []string{"./cmd/alpha", "./cmd/beta"} {
		packagePath := packagePath
		builds.Add(1)
		go func() {
			defer builds.Done()
			publishersReady <- struct{}{}
			<-beginBuilds
			for iteration := 1; iteration <= iterations; iteration++ {
				if buildErr := buildPackage(packagePath); buildErr != nil {
					buildErrors <- fmt.Errorf("build %s iteration %d: %w", packagePath, iteration, buildErr)
					return
				}
			}
		}()
	}
	for range 2 {
		<-publishersReady
	}
	launchesBeforePublication := successfulLaunches.Load()
	close(beginBuilds)
	builds.Wait()
	launchesDuringPublication := successfulLaunches.Load() - launchesBeforePublication
	close(stopLaunches)
	var result launchResult
	select {
	case result = <-launchResults:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out stopping the published binary launch loop")
	}
	close(buildErrors)
	for buildErr := range buildErrors {
		t.Error(buildErr)
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.count < iterations {
		t.Fatalf("successful launches = %d, want at least %d during repeated publication", result.count, iterations)
	}
	if launchesDuringPublication == 0 {
		t.Fatal("no binary launch overlapped the repeated publication window")
	}

	cachePath := filepath.Join(root, "bin", ".forj-build-cache", "app.target", "app")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("Stat(stable build cache) error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(cachePath))
	if err != nil {
		t.Fatalf("ReadDir(target build cache) error = %v", err)
	}
	entryNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		entryNames = append(entryNames, entry.Name())
	}
	wantEntries := []string{webindex.ArtifactPublicationLockFilename, "app"}
	if !reflect.DeepEqual(entryNames, wantEntries) {
		t.Fatalf("target build cache entries = %v, want %v", entryNames, wantEntries)
	}
	publishPaths, err := filepath.Glob(filepath.Join(root, "bin", ".app.*.publish"))
	if err != nil {
		t.Fatalf("Glob(publication outputs) error = %v", err)
	}
	if len(publishPaths) != 0 {
		t.Fatalf("publication outputs remain after concurrent builds: %v", publishPaths)
	}
}

// TestCmdRunWithTimingsPrintsStepDurations verifies explicit timing output remains available when transient progress is disabled.
func TestCmdRunWithTimingsPrintsStepDurations(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":          "module example.com/test\n\ngo 1.24\n",
		"cmd/app/main.go": "package main\nfunc main() {}\n",
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	appLogger := logger.NewSilentLogger()
	build := NewCmd(appLogger, stubAPIIndexer{root: root})
	build.Root = root
	build.Timings = true

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	runErr := build.Run()
	_ = w.Close()
	if runErr != nil {
		t.Fatalf("build run failed: %v", runErr)
	}

	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	output := out.String()
	for _, expected := range []string{
		"forj build wire:",
		"forj build generate:",
		"forj build build:api-index:",
		"forj build go build:",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected timings output to contain %q, got %q", expected, output)
		}
	}
}

func TestShouldRetryWire(t *testing.T) {
	retryable := `type-check failed for test/wire: wire/app.go:12:2: could not import test/internal/cmd`
	if !shouldRetryWire(retryable) {
		t.Fatalf("expected import-cascade wire error to be retryable")
	}

	nonRetryable := `wire: /private/tmp/test/wire/app.go:181:14: queue.DriverSync undefined`
	if shouldRetryWire(nonRetryable) {
		t.Fatalf("expected direct symbol error not to be retryable")
	}
}

func TestBuildArgsAppendDefaultPackageWhenOnlyFlagsProvided(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "app"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/app: %v", err)
	}
	cmd := &Cmd{Root: root, Args: []string{"-o", "./bin/app"}}
	got := requireBuildArgs(t, cmd)
	want := []string{"-o", "./bin/app", "./cmd/app"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("build args = %#v, want %#v", got, want)
	}
}

// TestBuildArgsAppendDefaultPackageAfterDoubleDashTags keeps a tag value from masquerading as the package target.
func TestBuildArgsAppendDefaultPackageAfterDoubleDashTags(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "app"), 0o755); err != nil {
		t.Fatalf("create default app target: %v", err)
	}
	command := &Cmd{Root: root, Args: []string{"--tags", "dev"}}
	got := requireBuildArgs(t, command)
	want := []string{"--tags", "dev", "./cmd/app"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("double-dash tag build args = %v, want %v", got, want)
	}
}

// TestCmdRunPassesBuildTagsToAPIIndex proves the pipeline and final go build share one invocation tag selection.
func TestCmdRunPassesBuildTagsToAPIIndex(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/taggedbuild\n\ngo 1.24\n",
		"cmd/app/main.go": `//go:build tagged

package main
func main() {}
`,
	}
	for relative, contents := range files {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create tagged build directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write tagged build fixture: %v", err)
		}
	}

	var indexedTags []string
	preparer := recordingAPIIndexPreparer{prepare: func(options apiindex.Options) (apiindex.Preparation, error) {
		indexedTags = append([]string(nil), options.BuildTags...)
		return apiindex.Preparation{Status: "app app, changed, 0 operations, 0 schemas, 0 diagnostics"}, nil
	}}
	command := NewCmd(logger.NewSilentLogger(), preparer)
	command.Root = root
	command.SkipWire = true
	command.Args = []string{"-tags", "tagged", "-o", "./bin/app", "./cmd/app"}
	if err := command.Run(); err != nil {
		t.Fatalf("run tagged build pipeline: %v", err)
	}
	if !reflect.DeepEqual(indexedTags, []string{"tagged"}) {
		t.Fatalf("API index build tags = %v, want tagged", indexedTags)
	}
}

func TestBuildArgsUseActiveConventionalTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "reporting"), 0o755); err != nil {
		t.Fatalf("mkdir cmd/reporting: %v", err)
	}
	t.Setenv("FORJ_APP", "reporting")

	cmd := &Cmd{Root: root}
	got := requireBuildArgs(t, cmd)
	want := []string{"-o", "bin/reporting", "./cmd/reporting"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("build args = %#v, want %#v", got, want)
	}
}

func TestLoadWirePathsUsesActiveConventionalTarget(t *testing.T) {
	root := t.TempDir()
	wireDir := filepath.Join("app", "reporting", "wire")
	if err := os.MkdirAll(filepath.Join(root, wireDir), 0o755); err != nil {
		t.Fatalf("mkdir target wire dir: %v", err)
	}
	t.Setenv("FORJ_APP", "reporting")

	got := loadWirePaths(root)
	want := []string{filepath.Join(root, wireDir)}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("wire paths = %#v, want %#v", got, want)
	}
}

func TestRunWireCommandPrintsDetailSeparately(t *testing.T) {
	root := t.TempDir()
	wireDir := filepath.Join(root, "wire")
	if err := os.MkdirAll(wireDir, 0o755); err != nil {
		t.Fatalf("mkdir wire dir: %v", err)
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	buildFailingWireFixture(t, binDir)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	pipeline := NewPipeline(logger.NewSilentLogger(), nil)
	_, runErr := pipeline.runWireCommand(wireDir, false)
	_ = w.Close()
	if runErr == nil {
		t.Fatal("expected wire command to fail")
	}
	if strings.Contains(runErr.Error(), "multiple bindings") || strings.Contains(runErr.Error(), "current:") {
		t.Fatalf("expected short wire error, got %q", runErr)
	}

	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"multiple bindings for Example",
		"current:",
		"<- provider \"NewExample\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected stderr to contain %q, got %q", want, output)
		}
	}
}

// buildFailingWireFixture compiles a native helper so command diagnostics are exercised without a host shell dependency.
func buildFailingWireFixture(t *testing.T, binDir string) {
	t.Helper()
	sourcePath := filepath.Join(t.TempDir(), "main.go")
	source := `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "wire: /tmp/test/wire.go:13:2: multiple bindings for Example")
	fmt.Fprintln(os.Stderr, "current:")
	fmt.Fprintln(os.Stderr, "  <- provider \"NewExample\"")
	os.Exit(1)
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write wire fixture source: %v", err)
	}
	wireName := "wire"
	if runtime.GOOS == "windows" {
		wireName += ".exe"
	}
	command := exec.Command("go", "build", "-o", filepath.Join(binDir, wireName), sourcePath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build wire fixture: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestMissingModulePackages(t *testing.T) {
	detail := `internal/storages/manager_gen.go:14:2: no required module provides package github.com/goforj/storage/driver/redisstorage; to add it:

go get github.com/goforj/storage/driver/redisstorage
other.go:3:1: no required module provides package example.com/foo/bar; to add it:
	go get example.com/foo/bar`

	got := missingModulePackages(detail)
	want := []string{
		"github.com/goforj/storage/driver/redisstorage",
		"example.com/foo/bar",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("missingModulePackages() = %v, want %v", got, want)
	}
}

func TestAttemptMissingModuleRecovery(t *testing.T) {
	build := &Cmd{}
	var got []string
	build.goGetFunc = func(packages []string) error {
		got = append(got, packages...)
		return nil
	}

	recovered, err := build.attemptMissingModuleRecovery(".", `internal/storages/manager_gen.go:14:2: no required module provides package github.com/goforj/storage/driver/redisstorage; to add it:

go get github.com/goforj/storage/driver/redisstorage`)
	if err != nil {
		t.Fatalf("attemptMissingModuleRecovery() error = %v", err)
	}
	if !recovered {
		t.Fatalf("attemptMissingModuleRecovery() recovered = false, want true")
	}
	if strings.Join(got, ",") != "github.com/goforj/storage/driver/redisstorage" {
		t.Fatalf("go get packages = %v", got)
	}
	if build.lastBuildStatus != "synced deps, retried" {
		t.Fatalf("lastBuildStatus = %q", build.lastBuildStatus)
	}
}

// TestRunGoBuildReportsModuleRecoveryFailure verifies a failed go get is not hidden behind the compiler error that triggered it.
func TestRunGoBuildReportsModuleRecoveryFailure(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":  "module example.com/recovery\n\ngo 1.26\n",
		"main.go": "package main\n\nimport _ \"example.invalid/missing\"\n\nfunc main() {}\n",
	}
	for relativePath, contents := range files {
		if err := os.WriteFile(filepath.Join(root, relativePath), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", relativePath, err)
		}
	}
	recoveryErr := errors.New("go get denied")
	build := &Cmd{goGetFunc: func([]string) error { return recoveryErr }}

	err := build.runGoBuild(root, []string{"."}, goBuildOptions{allowRecovery: true})
	if !errors.Is(err, recoveryErr) || !strings.Contains(err.Error(), "recover missing build modules") {
		t.Fatalf("runGoBuild error = %v, want module recovery failure", err)
	}
}

// TestBuildProgressMarkers verifies machine-readable progress remains stable for watcher and editor integrations.
func TestBuildProgressMarkers(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":          "module example.com/test\n\ngo 1.24\n",
		"cmd/app/main.go": "package main\nfunc main() {}\n",
	}
	for rel, contents := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	t.Setenv("FORJ_BUILD_PROGRESS", "1")

	appLogger := logger.NewSilentLogger()
	build := NewCmd(appLogger, stubAPIIndexer{root: root})
	build.Root = root

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	runErr := build.Run()
	_ = w.Close()
	if runErr != nil {
		t.Fatalf("build run failed: %v", runErr)
	}

	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	output := out.String()
	for _, expected := range []string{
		"__FORJ_BUILD_PROGRESS__ step 1/4 generate",
		"__FORJ_BUILD_PROGRESS__ step 2/4 wire",
		"__FORJ_BUILD_PROGRESS__ step 3/4 build:api-index",
		"__FORJ_BUILD_PROGRESS__ step 4/4 go build",
		"__FORJ_BUILD_PROGRESS__ done",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected progress output to contain %q, got %q", expected, output)
		}
	}
}

func TestBuildProgressReporterNoopsWithoutTTY(t *testing.T) {
	t.Setenv("FORJ_BUILD_PROGRESS", "")
	t.Setenv("FORJ_DEBUG", "")
	t.Setenv("DEBUG", "")

	origStderr := os.Stderr
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() {
		_ = w.Close()
		os.Stderr = origStderr
	}()

	if _, ok := newBuildProgressReporter(false, RunOptions{}).(buildProgressNoop); !ok {
		t.Fatal("expected build progress reporter to be noop when stderr is not a TTY")
	}
}

// TestAppHelpBuildTransientProgressScopesClearingToHelp verifies ordinary builds retain their durable completion line.
func TestAppHelpBuildTransientProgressScopesClearingToHelp(t *testing.T) {
	t.Setenv("FORJ_COMMAND_ORIGIN", AppHelpCommandOrigin)
	if !appHelpBuildTransientProgress() {
		t.Fatal("expected App help build origin to enable transient progress")
	}
	t.Setenv("FORJ_COMMAND_ORIGIN", "dev_command")
	if appHelpBuildTransientProgress() {
		t.Fatal("expected non-help build origin to keep durable progress")
	}
}

// TestBuildArgsDoNotInjectLaunchState verifies executable behavior is derived
// from generated app source rather than artifact-specific linker values.
func TestBuildArgsDoNotInjectLaunchState(t *testing.T) {
	cmd := &Cmd{Root: t.TempDir()}
	args := requireBuildArgs(t, cmd)
	got := strings.Join(args, " ")
	if strings.Contains(got, "-ldflags") {
		t.Fatalf("buildArgs() injected launch state: %v", args)
	}
}

// TestBuildArgsAddsCompiledEnvDefaultsLdflags verifies unset-only values remain
// available through the independent compiled environment channel.
func TestBuildArgsAddsCompiledEnvDefaultsLdflags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cmd := &Cmd{
		Root:        root,
		EnvDefaults: "FEATURE_A=true,FEATURE_B=false",
	}

	args := requireBuildArgs(t, cmd)
	got := strings.Join(args, " ")
	wantPayload := base64.StdEncoding.EncodeToString([]byte("FEATURE_A=true,FEATURE_B=false"))
	if !strings.Contains(got, "-X example.com/demo/internal/cmd.CompiledEnvDefaultsBase64="+wantPayload) {
		t.Fatalf("expected compiled env default ldflags in build args, got %v", args)
	}
}

// TestBuildArgsAddsCompiledEnvOverridesLdflags verifies forced values remain
// available without coupling them to executable launch behavior.
func TestBuildArgsAddsCompiledEnvOverridesLdflags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cmd := &Cmd{
		Root:         root,
		EnvOverrides: "FEATURE_A=true,FEATURE_B=false",
	}

	args := requireBuildArgs(t, cmd)
	got := strings.Join(args, " ")
	wantPayload := base64.StdEncoding.EncodeToString([]byte("FEATURE_A=true,FEATURE_B=false"))
	if !strings.Contains(got, "-X example.com/demo/internal/cmd.CompiledEnvOverridesBase64="+wantPayload) {
		t.Fatalf("expected compiled env override ldflags in build args, got %v", args)
	}
}

// TestBuildArgsMergesCompiledEnvWithExistingLdflags verifies removing launch
// injection does not disturb the generic linker-flag merge path.
func TestBuildArgsMergesCompiledEnvWithExistingLdflags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cmd := &Cmd{
		Root:        root,
		EnvDefaults: "FEATURE_A=true",
		Args:        []string{"-trimpath", "-ldflags", "-s -w", "-o", "./bin/app", "."},
	}

	args := requireBuildArgs(t, cmd)
	got := strings.Join(args, " ")
	wantPayload := base64.StdEncoding.EncodeToString([]byte("FEATURE_A=true"))
	want := "-s -w -X example.com/demo/internal/cmd.CompiledEnvDefaultsBase64=" + wantPayload
	if !strings.Contains(got, want) {
		t.Fatalf("buildArgs() did not preserve and extend linker flags: %v", args)
	}
}

// TestValidateCompiledEnvRejectsMalformedEnvDefaults verifies malformed
// unset-only values still fail before invoking the compiler.
func TestValidateCompiledEnvRejectsMalformedEnvDefaults(t *testing.T) {
	cmd := &Cmd{
		EnvDefaults: "BROKEN",
	}
	if err := cmd.validateCompiledEnv(cmd.Root); err == nil {
		t.Fatal("expected malformed env defaults to fail")
	}
}

// TestValidateCompiledEnvRejectsMalformedEnvOverrides verifies malformed
// forced values still fail before invoking the compiler.
func TestValidateCompiledEnvRejectsMalformedEnvOverrides(t *testing.T) {
	cmd := &Cmd{
		EnvOverrides: "BROKEN",
	}
	if err := cmd.validateCompiledEnv(cmd.Root); err == nil {
		t.Fatal("expected malformed env overrides to fail")
	}
}

// requireBuildArgs keeps argument-shape assertions focused while still failing on package-probe errors.
func requireBuildArgs(t *testing.T, command *Cmd) []string {
	t.Helper()
	args, err := command.buildArgs(command.Root)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	return args
}

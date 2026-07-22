package forj

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goforj/console"
	"github.com/goforj/goforj/project"
)

// consoleLoaderWriter provides a concurrency-safe terminal descriptor for loader integration tests.
type consoleLoaderWriter struct {
	mu     sync.Mutex
	output bytes.Buffer
}

// Write records one loader frame without allowing the animation goroutine to race with assertions.
func (w *consoleLoaderWriter) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.output.Write(value)
}

// Fd exposes a stable synthetic descriptor consumed by the injected terminal detector.
func (*consoleLoaderWriter) Fd() uintptr {
	return 91
}

// String returns a snapshot of all recorded loader frames.
func (w *consoleLoaderWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.output.String()
}

// TestExternalConsoleSemanticMarks verifies GoForj retains its user-facing marks after the package extraction.
func TestExternalConsoleSemanticMarks(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	colorEnabled := false
	unicodeEnabled := true
	runtime := console.New(console.Config{
		Stdout:         &stdout,
		Stderr:         &stderr,
		ColorEnabled:   &colorEnabled,
		UnicodeEnabled: &unicodeEnabled,
	})

	runtime.Actionf("building")
	runtime.Successf("ready")
	runtime.Errorf("failed")

	if got, want := stdout.String(), "· building\n✔ ready\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "✖ failed\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

// TestRunWithLoaderPropagatesErrorsAndReleasesTheTransient verifies the extracted loader owns cleanup on every return path.
func TestRunWithLoaderPropagatesErrorsAndReleasesTheTransient(t *testing.T) {
	previous := console.Default()
	t.Cleanup(func() {
		console.SetDefault(previous)
	})

	output := &consoleLoaderWriter{}
	colorEnabled := false
	unicodeEnabled := false
	animationsEnabled := true
	console.SetDefault(console.New(console.Config{
		Stdout:            output,
		Stderr:            output,
		ColorEnabled:      &colorEnabled,
		UnicodeEnabled:    &unicodeEnabled,
		AnimationsEnabled: &animationsEnabled,
		LoaderInterval:    time.Hour,
		IsTerminal:        func(int) bool { return true },
	}))

	wantErr := errors.New("render failed")
	firstCalls := 0
	err := runWithLoader("Rendering project files", func() error {
		firstCalls++
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runWithLoader() error = %v, want %v", err, wantErr)
	}
	if firstCalls != 1 {
		t.Fatalf("first callback calls = %d, want 1", firstCalls)
	}

	secondCalls := 0
	if err := runWithLoader("Retrying project files", func() error {
		secondCalls++
		return nil
	}); err != nil {
		t.Fatalf("second runWithLoader() error = %v", err)
	}
	if secondCalls != 1 {
		t.Fatalf("second callback calls = %d, want 1", secondCalls)
	}
	if got := strings.Count(output.String(), "\r\x1b[2K"); got < 4 {
		t.Fatalf("transient clear writes = %d, want at least 4: %q", got, output.String())
	}
}

// TestRunDevBuildJobWithLoaderAnimatesBootstrapBuild verifies initial App compilation uses the shared console loader and clears it on success.
func TestRunDevBuildJobWithLoaderAnimatesBootstrapBuild(t *testing.T) {
	previous := console.Default()
	t.Cleanup(func() {
		console.SetDefault(previous)
	})
	t.Chdir(t.TempDir())

	output := &consoleLoaderWriter{}
	colorEnabled := false
	unicodeEnabled := true
	animationsEnabled := true
	console.SetDefault(console.New(console.Config{
		Stdout:            output,
		Stderr:            output,
		ColorEnabled:      &colorEnabled,
		UnicodeEnabled:    &unicodeEnabled,
		AnimationsEnabled: &animationsEnabled,
		LoaderInterval:    time.Hour,
		IsTerminal:        func(int) bool { return true },
	}))

	err := runDevBuildJobWithLoader(output, output, "Building app", devBuildJob{
		app:     project.DefaultApp(),
		command: "true",
	})
	if err != nil {
		t.Fatalf("runDevBuildJobWithLoader() error = %v", err)
	}
	if !strings.Contains(output.String(), "Building app") {
		t.Fatalf("loader omitted build label: %q", output.String())
	}
	if got := strings.Count(output.String(), "\r\x1b[2K"); got < 2 {
		t.Fatalf("loader clear writes = %d, want at least 2: %q", got, output.String())
	}
}

// TestRunDevBuildJobWithLoaderKeepsRedirectedOutputStable protects CI logs from spinner frames and carriage-return redraws.
func TestRunDevBuildJobWithLoaderKeepsRedirectedOutputStable(t *testing.T) {
	previous := console.Default()
	t.Cleanup(func() {
		console.SetDefault(previous)
	})
	t.Chdir(t.TempDir())

	var output bytes.Buffer
	colorEnabled := false
	unicodeEnabled := true
	animationsEnabled := true
	console.SetDefault(console.New(console.Config{
		Stdout:            &output,
		Stderr:            &output,
		ColorEnabled:      &colorEnabled,
		UnicodeEnabled:    &unicodeEnabled,
		AnimationsEnabled: &animationsEnabled,
		IsTerminal:        func(int) bool { return false },
	}))

	err := runDevBuildJobWithLoader(&output, &output, "Building app", devBuildJob{
		app:     project.DefaultApp(),
		command: "true",
	})
	if err != nil {
		t.Fatalf("runDevBuildJobWithLoader() error = %v", err)
	}
	if got, want := output.String(), "· Building app\n"; got != want {
		t.Fatalf("redirected loader output = %q, want %q", got, want)
	}
}

// TestRunDevBuildJobWithLoaderPreservesSuccessfulCustomOutput keeps owner-authored build messages visible after transient progress completes.
func TestRunDevBuildJobWithLoaderPreservesSuccessfulCustomOutput(t *testing.T) {
	previous := console.Default()
	t.Cleanup(func() {
		console.SetDefault(previous)
	})
	t.Chdir(t.TempDir())

	var output bytes.Buffer
	colorEnabled := false
	unicodeEnabled := true
	animationsEnabled := true
	console.SetDefault(console.New(console.Config{
		Stdout:            &output,
		Stderr:            &output,
		ColorEnabled:      &colorEnabled,
		UnicodeEnabled:    &unicodeEnabled,
		AnimationsEnabled: &animationsEnabled,
		IsTerminal:        func(int) bool { return false },
	}))

	err := runDevBuildJobWithLoader(&output, &output, "Building app", devBuildJob{
		app:     project.DefaultApp(),
		command: "printf 'generated assets\\n'; printf 'build warning\\n' >&2",
	})
	if err != nil {
		t.Fatalf("runDevBuildJobWithLoader() error = %v", err)
	}
	for _, expected := range []string{"· Building app", "generated assets", "build warning"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("successful custom build omitted %q: %q", expected, output.String())
		}
	}
}

// TestRunDevBuildJobWithLoaderReplaysFailureAfterCleanup keeps compilation diagnostics visible without corrupting the loader line.
func TestRunDevBuildJobWithLoaderReplaysFailureAfterCleanup(t *testing.T) {
	previous := console.Default()
	t.Cleanup(func() {
		console.SetDefault(previous)
	})
	t.Chdir(t.TempDir())

	output := &consoleLoaderWriter{}
	colorEnabled := false
	unicodeEnabled := true
	animationsEnabled := true
	console.SetDefault(console.New(console.Config{
		Stdout:            output,
		Stderr:            output,
		ColorEnabled:      &colorEnabled,
		UnicodeEnabled:    &unicodeEnabled,
		AnimationsEnabled: &animationsEnabled,
		LoaderInterval:    time.Hour,
		IsTerminal:        func(int) bool { return true },
	}))

	err := runDevBuildJobWithLoader(output, output, "Building app", devBuildJob{
		app:     project.DefaultApp(),
		command: "printf 'compile failed\\n' >&2; exit 7",
	})
	if err == nil {
		t.Fatal("expected bootstrap build failure")
	}
	for _, expected := range []string{"Build failed for app", "compile failed"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("bootstrap failure omitted %q: %q", expected, output.String())
		}
	}
	if got := strings.Count(output.String(), "\r\x1b[2K"); got < 2 {
		t.Fatalf("loader was not cleared before failure replay: %q", output.String())
	}
}

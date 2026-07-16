package forj

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goforj/console"
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

package forj

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goforj/console"
	"github.com/goforj/goforj/internal/devwatch"
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

// TestDevSuccessLineRetainsSemanticColor verifies preparation blocks preserve their stronger completion treatment.
func TestDevSuccessLineRetainsSemanticColor(t *testing.T) {
	previous := console.Default()
	t.Cleanup(func() {
		console.SetDefault(previous)
	})

	var output bytes.Buffer
	colorEnabled := true
	unicodeEnabled := true
	console.SetDefault(console.New(console.Config{
		Stdout:         &output,
		Stderr:         &output,
		ColorEnabled:   &colorEnabled,
		UnicodeEnabled: &unicodeEnabled,
	}))

	block := newDevTaskOutputBlock("Preparing App", &output, &output, nil)
	block.successLabel = "Prepared"
	stream := block.stdoutWriter()
	writeDevSuccessLine(stream, "Build", "376ms")
	if _, err := io.WriteString(stream, "✔ migrations complete (0)\n"); err != nil {
		t.Fatalf("write child success: %v", err)
	}
	for _, expected := range []string{
		console.ColorGreen + "✔" + console.ColorReset,
		console.ColorBoldWhite + "Build" + console.ColorReset,
		console.ColorGray + "376ms" + console.ColorReset,
		console.ColorGreen + "✔" + console.ColorReset + " migrations complete (0)",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("success output omitted %q: %q", expected, output.String())
		}
	}
	if strings.Contains(output.String(), console.ColorGreen+"migrations complete") {
		t.Fatalf("success output colored the child statement: %q", output.String())
	}
	if got := stripANSI(output.String()); !strings.Contains(got, "✔ Build · 376ms") {
		t.Fatalf("success output retained loose lifecycle spacing: %q", got)
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

// TestDevSetupAndTeardownTasksUseCoordinatedLoaders verifies lifecycle commands animate without hiding child output.
func TestDevSetupAndTeardownTasksUseCoordinatedLoaders(t *testing.T) {
	previous := console.Default()
	t.Cleanup(func() {
		console.SetDefault(previous)
	})

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

	if err := runDevTasks([]project.DevTask{{
		Name: "Start development services",
		Cmd:  "printf 'services ready\\n'",
	}}); err != nil {
		t.Fatalf("runDevTasks() error = %v", err)
	}
	if err := runDevDownTasks([]project.DevTask{{
		Name: "Stop development services",
		Cmd:  "printf 'services stopped\\n'",
	}}); err != nil {
		t.Fatalf("runDevDownTasks() error = %v", err)
	}

	for _, expected := range []string{
		"Start development services",
		"services ready",
		"Stop development services",
		"services stopped",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("lifecycle loader omitted %q: %q", expected, output.String())
		}
	}
	for _, unexpected := range []string{"Running pre-dev setup", "Bringing down resources"} {
		if strings.Contains(output.String(), unexpected) {
			t.Fatalf("lifecycle output retained redundant orchestration line %q: %q", unexpected, output.String())
		}
	}
	if got := strings.Count(output.String(), "\r\x1b[2K"); got < 4 {
		t.Fatalf("lifecycle loader clear writes = %d, want at least 4: %q", got, output.String())
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

// TestRunDevSPABuildWithLoaderMakesFrontendWorkVisible verifies the longest default frontend phase uses shared loader policy.
func TestRunDevSPABuildWithLoaderMakesFrontendWorkVisible(t *testing.T) {
	previous := console.Default()
	t.Cleanup(func() {
		console.SetDefault(previous)
	})
	root := t.TempDir()

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

	err := runDevSPABuildWithLoader(output, output, "Building app frontend", devCompiledWatcher{
		Command: devwatch.Command{
			Shell: "printf 'frontend assets ready\\n'",
			Dir:   root,
		},
	})
	if err != nil {
		t.Fatalf("runDevSPABuildWithLoader() error = %v", err)
	}
	for _, expected := range []string{"Building app frontend", "frontend assets ready"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("frontend loader omitted %q: %q", expected, output.String())
		}
	}
	if got := strings.Count(output.String(), "\r\x1b[2K"); got < 2 {
		t.Fatalf("frontend loader clear writes = %d, want at least 2: %q", got, output.String())
	}
}

// TestRunDevSPABuildWithLoaderReplaysFailureDiagnostics verifies quiet frontend progress never hides Vite output.
func TestRunDevSPABuildWithLoaderReplaysFailureDiagnostics(t *testing.T) {
	previous := console.Default()
	t.Cleanup(func() {
		console.SetDefault(previous)
	})

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

	err := runDevSPABuildWithLoader(output, output, "Building app frontend", devCompiledWatcher{
		Command: devwatch.Command{Shell: "printf 'Vite could not resolve ./src/missing\\n' >&2; exit 7"},
	})
	if err == nil {
		t.Fatal("expected frontend build failure")
	}
	if !strings.Contains(output.String(), "Vite could not resolve ./src/missing") {
		t.Fatalf("frontend failure diagnostic was hidden: %q", output.String())
	}
	lastClear := strings.LastIndex(output.String(), "\r\x1b[2K")
	diagnostic := strings.Index(output.String(), "Vite could not resolve")
	if lastClear < 0 || diagnostic < lastClear {
		t.Fatalf("frontend diagnostic was written before loader cleanup: %q", output.String())
	}
}

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

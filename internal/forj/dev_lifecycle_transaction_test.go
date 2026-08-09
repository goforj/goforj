package forj

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/project"
)

type devLifecycleRecordingWriter struct {
	bytes.Buffer
	began     []devLifecycleTransaction
	completed []devLifecycleTransactionSummary
}

type devLifecycleOrchestrationWriter struct {
	bytes.Buffer
	compact bool
}

// compactLifecycleTransactionActive reports the test writer's configured transaction mode.
func (w *devLifecycleOrchestrationWriter) compactLifecycleTransactionActive() bool {
	return w.compact
}

// BeginLifecycleTransaction records the transaction selected by the runner.
func (w *devLifecycleRecordingWriter) BeginLifecycleTransaction(transaction devLifecycleTransaction) {
	w.began = append(w.began, transaction)
}

// CompleteLifecycleTransaction records the structured work summary selected by the runner.
func (w *devLifecycleRecordingWriter) CompleteLifecycleTransaction(_ string, _ time.Duration, summary devLifecycleTransactionSummary) {
	w.completed = append(w.completed, summary)
}

// FailLifecycleTransaction satisfies the lifecycle controller contract for runner boundary tests.
func (*devLifecycleRecordingWriter) FailLifecycleTransaction(string, time.Duration, error) {}

// TestDevRestartTransactionCollapsesSuccessfulInfrastructureOutput verifies success retains one durable high-signal line.
func TestDevRestartTransactionCollapsesSuccessfulInfrastructureOutput(t *testing.T) {
	transaction := newDevRestartTransaction([]string{"Build App", "Build app SPA frontend", "Run App"}, false)
	buffer := &devBubbleLifecycleTransaction{transaction: transaction}
	if !buffer.retain([]string{"Watchers stopping", "HTTP server shut down", "Starting Run App"}) {
		t.Fatal("expected compact transaction to retain lifecycle output")
	}

	lines := buffer.successLines(1034*time.Millisecond, devLifecycleTransactionSummary{
		BuildElapsed:   380 * time.Millisecond,
		MigrateElapsed: 340 * time.Millisecond,
	})
	if len(lines) != 1 {
		t.Fatalf("restart success lines = %#v, want one summary", lines)
	}
	line := stripANSI(lines[0])
	for _, want := range []string{"Restarted", "build 380ms", "migrate 340ms", "1.03s"} {
		if !strings.Contains(line, want) {
			t.Fatalf("success summary missing %q: %q", want, line)
		}
	}
	for _, hidden := range buffer.lines {
		if strings.Contains(line, hidden) {
			t.Fatalf("success summary leaked retained output %q", hidden)
		}
	}
}

// TestDevStartupTransactionPreservesInitialLifecycleOutput keeps the first App boot visible after readiness.
func TestDevStartupTransactionPreservesInitialLifecycleOutput(t *testing.T) {
	transaction := newDevStartupTransaction(&project.Config{}, []string{"Build App", "Run App"}, false)
	buffer := &devBubbleLifecycleTransaction{transaction: transaction}
	startupOutput := []string{
		"01:04:29.753 Jobs Starting queue worker",
		"01:04:29.755 HTTP Routes registered",
	}
	if !buffer.retain(startupOutput) {
		t.Fatal("expected compact startup transaction to retain lifecycle output")
	}

	lines := buffer.successLines(66*time.Millisecond, devLifecycleTransactionSummary{})
	if len(lines) != len(startupOutput)+2 {
		t.Fatalf("startup success lines = %#v", lines)
	}
	plain := stripANSI(strings.Join(lines, "\n"))
	for _, want := range []string{"┏ App startup", "Build App", "Run App", "Starting queue worker", "Routes registered", "┗", "Ready", "66ms"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("startup transcript omitted %q:\n%s", want, plain)
		}
	}
}

// TestBeginActiveDevRestartTransactionCoversAutomaticRebuilds verifies runner-owned rebuilds use the same boundary as manual restarts.
func TestBeginActiveDevRestartTransactionCoversAutomaticRebuilds(t *testing.T) {
	writer := &devLifecycleRecordingWriter{}
	active, err := beginActiveDevRestartTransaction(writer, &project.Config{}, []string{"Build App", "Build app SPA frontend", "Run App"})
	if err != nil {
		t.Fatalf("beginActiveDevRestartTransaction() error = %v", err)
	}
	if active == nil || len(writer.began) != 1 {
		t.Fatalf("active transaction = %#v, began = %#v", active, writer.began)
	}
	if got := stripANSI(writer.began[0].inProgressLine()); !strings.Contains(got, "Restarting") || !strings.Contains(got, "SPA") {
		t.Fatalf("restart transaction line = %q", got)
	}
}

// TestCompactLifecycleTransactionOmitsTypedWatcherNarration verifies the shelf heading replaces redundant runner state.
func TestCompactLifecycleTransactionOmitsTypedWatcherNarration(t *testing.T) {
	writer := &devLifecycleOrchestrationWriter{compact: true}
	streamer := &devwatchStreamer{ch: make(chan devwatchLine, 1)}
	emitWatcherLifecycleSummary(writer, streamer, []string{"Build App", "Run App"}, watcherStateStopped)
	if writer.Len() != 0 {
		t.Fatalf("compact lifecycle wrote watcher narration: %q", writer.String())
	}
	select {
	case line := <-streamer.ch:
		if !strings.Contains(stripANSI(line.Line), "Watchers stopped") {
			t.Fatalf("streamed watcher narration = %q", line.Line)
		}
	default:
		t.Fatal("compact lifecycle discarded watcher narration from the diagnostic stream")
	}

	writer.compact = false
	emitWatcherLifecycleSummary(writer, nil, []string{"Build App", "Run App"}, watcherStateStopped)
	if !strings.Contains(stripANSI(writer.String()), "Watchers stopped") {
		t.Fatalf("detailed lifecycle omitted watcher narration: %q", writer.String())
	}
}

// TestCompactLifecycleTransactionOmitsTriggerNarration verifies launch markers still complete startup without entering the shelf.
func TestCompactLifecycleTransactionOmitsTriggerNarration(t *testing.T) {
	writer := &devLifecycleOrchestrationWriter{compact: true}
	triggered := false
	lifecycle := newDevwatchLifecycleState(1, []string{"Run App"})
	lifecycle.separators = false
	watcherWriter := newDevwatchWriterForApp(
		writer,
		nil,
		"stdout",
		"Run App",
		"./bin/app",
		project.DefaultAppName,
		0,
		false,
		lifecycle,
		func() { triggered = true },
	)
	if _, err := watcherWriter.Write([]byte(watcherTriggerMarker + "\n")); err != nil {
		t.Fatalf("write trigger marker: %v", err)
	}
	if !triggered {
		t.Fatal("suppressed trigger narration did not report process launch")
	}
	if writer.Len() != 0 {
		t.Fatalf("compact lifecycle wrote trigger narration: %q", writer.String())
	}
}

// TestCompactLifecycleTransactionOmitsRetainedTriggerNarration keeps failure buffering from hiding TUI ownership.
func TestCompactLifecycleTransactionOmitsRetainedTriggerNarration(t *testing.T) {
	writer := &devLifecycleOrchestrationWriter{compact: true}
	tail := newDevTaskOutputTail(40)
	lifecycle := newDevwatchLifecycleState(1, []string{"Run App"})
	lifecycle.separators = false
	spec := devCompiledWatcher{
		ID:      "structured:app:run",
		Name:    "Run App",
		App:     project.DefaultAppName,
		Kind:    devWatcherAppRun,
		Command: devwatch.Command{Shell: "./bin/app"},
	}
	configureCompiledDevCommand(
		&spec,
		nil,
		writer,
		writer,
		tail,
		false,
		lifecycle,
		nil,
		0,
		false,
		false,
	)
	if _, err := spec.Command.Stdout.Write([]byte(watcherTriggerMarker + "\n")); err != nil {
		t.Fatalf("write retained trigger marker: %v", err)
	}
	if writer.Len() != 0 || tail.String() != "" {
		t.Fatalf("compact lifecycle retained trigger narration: output=%q tail=%q", writer.String(), tail.String())
	}
	writer.compact = false
	if _, err := spec.Command.Stdout.Write([]byte(watcherTriggerMarker + "\n")); err != nil {
		t.Fatalf("write detailed retained trigger marker: %v", err)
	}
	for label, output := range map[string]string{"output": writer.String(), "tail": tail.String()} {
		if !strings.Contains(output, "Starting Run App") {
			t.Fatalf("detailed lifecycle omitted trigger narration from %s: %q", label, output)
		}
	}
}

// TestWaitDevLifecycleReadinessUsesTheConventionalAppHealthBoundary verifies framework startup output stays buffered until HTTP is serving.
func TestWaitDevLifecycleReadinessUsesTheConventionalAppHealthBoundary(t *testing.T) {
	requested := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requested <- request.URL.Path
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv("APP_URL", server.URL+"/public-prefix")

	config := &project.Config{Render: project.RenderConfig{Components: project.Components{WebAPI: true}}}
	controller := &devWatcherController{tasks: map[string]*devWatcherTask{
		"runtime": {spec: devCompiledWatcher{Kind: devWatcherAppRun, App: project.DefaultAppName}},
	}}
	if err := waitDevLifecycleReadiness(context.Background(), config, controller); err != nil {
		t.Fatalf("waitDevLifecycleReadiness() error = %v", err)
	}
	if path := <-requested; path != devLifecycleReadyPath {
		t.Fatalf("readiness path = %q, want %q", path, devLifecycleReadyPath)
	}
}

// TestWaitDevLifecycleReadinessSkipsCustomRuntimes verifies GoForj does not infer readiness for processes whose protocol it does not own.
func TestWaitDevLifecycleReadinessSkipsCustomRuntimes(t *testing.T) {
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{WebAPI: true}}}
	controller := &devWatcherController{tasks: map[string]*devWatcherTask{
		"runtime": {spec: devCompiledWatcher{Kind: devWatcherAppRun, App: project.DefaultAppName, FullProcessOverride: true}},
	}}
	if err := waitDevLifecycleReadiness(context.Background(), config, controller); err != nil {
		t.Fatalf("waitDevLifecycleReadiness() error = %v", err)
	}
}

// TestDevLifecycleReadinessURLRejectsRelativeURLs keeps readiness attribution tied to a concrete App endpoint.
func TestDevLifecycleReadinessURLRejectsRelativeURLs(t *testing.T) {
	if _, err := devLifecycleReadinessURL("localhost:3000"); err == nil {
		t.Fatal("devLifecycleReadinessURL() error = nil, want relative URL error")
	}
}

// TestDevRestartTransactionExpandsFailureOutput verifies failures retain the child command context needed for diagnosis.
func TestDevRestartTransactionExpandsFailureOutput(t *testing.T) {
	transaction := newDevRestartTransaction([]string{"Build App", "Run App"}, false)
	buffer := &devBubbleLifecycleTransaction{transaction: transaction}
	buffer.retain([]string{"Build App", "go build ./...", "cmd/app/main.go:42:17: undefined: server.New"})

	lines := buffer.failureLines(842*time.Millisecond, errors.New("go build failed"))
	joined := stripANSI(strings.Join(lines, "\n"))
	for _, want := range []string{"Restart failed", "842ms", "Build App", "go build ./...", "undefined: server.New", "go build failed"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("failure output missing %q:\n%s", want, joined)
		}
	}
}

// TestDevLifecycleDetailedOutputDoesNotRetainOutput keeps explicitly requested diagnostics live.
func TestDevLifecycleDetailedOutputDoesNotRetainOutput(t *testing.T) {
	t.Setenv("FORJ_DEBUG", "1")
	t.Setenv("DEBUG", "")
	if !devLifecycleDetailedOutput(nil) {
		t.Fatal("expected FORJ_DEBUG to expand lifecycle output")
	}
	transaction := newDevRestartTransaction([]string{"Run App"}, true)
	buffer := &devBubbleLifecycleTransaction{transaction: transaction}
	if buffer.retain([]string{"Starting Run App"}) {
		t.Fatal("detailed transaction unexpectedly retained live output")
	}

	t.Setenv("FORJ_DEBUG", "")
	if !devLifecycleDetailedOutput([]devCompiledWatcher{{Verbose: true}}) {
		t.Fatal("expected verbose watcher configuration to expand lifecycle output")
	}
}

// TestDevStartupTransactionSummarizesPersistentResources verifies initial readiness points at the header instead of replaying URLs.
func TestDevStartupTransactionSummarizesPersistentResources(t *testing.T) {
	config := &project.Config{}
	config.Render.Components.Mail = true
	config.Render.Components.Docker = true
	t.Setenv("APP_URL", "http://localhost:9000")
	t.Setenv("COMPOSE_PROFILES", "mailpit")

	transaction := newDevStartupTransaction(config, []string{"Run App"}, false)
	line := stripANSI(transaction.successLine(120*time.Millisecond, devLifecycleTransactionSummary{}))
	if !strings.Contains(line, "Ready") || !strings.Contains(line, "App :9000") || !strings.Contains(line, "resources") {
		t.Fatalf("initial summary = %q", line)
	}
	if strings.Contains(line, "http://") {
		t.Fatalf("initial summary repeated resource URL: %q", line)
	}
}

// TestDevReadySummaryAnnouncementIsPersistentTUIOnly verifies restarts do not replay resource URLs in the transcript.
func TestDevReadySummaryAnnouncementIsPersistentTUIOnly(t *testing.T) {
	if shouldPrintDevReadySummary(&devBubbleWriter{}, true) {
		t.Fatal("persistent TUI unexpectedly requested a repeated resource announcement")
	}
	if !shouldPrintDevReadySummary(io.Discard, true) {
		t.Fatal("plain output did not preserve repeated readiness behavior")
	}
}

// TestDevwatchWriterOmitsDecorativeSeparatorsForTransactionOutput verifies normal TUI rendering does not infer lifecycle from App text.
func TestDevwatchWriterOmitsDecorativeSeparatorsForTransactionOutput(t *testing.T) {
	var out bytes.Buffer
	lifecycle := newDevwatchLifecycleState(1, []string{"Run App"})
	lifecycle.separators = false
	writer := newDevwatchWriter(&out, nil, "stdout", "Run App", "./bin/app", lifecycle)
	if _, err := writer.Write([]byte(watcherTriggerMarker + "\n")); err != nil {
		t.Fatalf("write trigger: %v", err)
	}
	plain := stripANSI(out.String())
	if strings.Contains(plain, "Startup") || strings.Contains(plain, "Shutdown") {
		t.Fatalf("transaction output included decorative separator: %q", plain)
	}
}

// TestDevwatchWriterReportsTriggerAfterPersistingItsOutputBatch prevents restart completion from overtaking adjacent child output.
func TestDevwatchWriterReportsTriggerAfterPersistingItsOutputBatch(t *testing.T) {
	var out bytes.Buffer
	seen := ""
	lifecycle := newDevwatchLifecycleState(1, []string{"Run App"})
	lifecycle.separators = false
	writer := newDevwatchWriterForApp(
		&out,
		nil,
		"stdout",
		"Run App",
		"./bin/app",
		"app",
		0,
		false,
		lifecycle,
		func() { seen = out.String() },
	)
	if _, err := writer.Write([]byte(watcherTriggerMarker + "\nHTTP server started\n")); err != nil {
		t.Fatalf("write startup batch: %v", err)
	}
	if !strings.Contains(seen, "Starting Run App") || !strings.Contains(seen, "HTTP server started") {
		t.Fatalf("trigger callback overtook persisted output: %q", seen)
	}
}

// TestDevWatcherStartupTrackerWaitsForTriggerAndOutcome verifies readiness cannot race ahead of process output setup.
func TestDevWatcherStartupTrackerWaitsForTriggerAndOutcome(t *testing.T) {
	tracker := newDevWatcherStartupTracker([]devWatcherStartupExpectation{{ID: "run", WaitOutcome: true}})
	tracker.noteOutcome("run", nil)
	select {
	case <-tracker.done:
		t.Fatal("startup completed before the wrapper trigger")
	default:
	}
	tracker.noteTrigger("run")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := tracker.wait(ctx); err != nil {
		t.Fatalf("wait startup: %v", err)
	}
}

// TestDevWatcherStartupTrackerReturnsTaskFailure verifies restart transactions receive structured watcher context.
func TestDevWatcherStartupTrackerReturnsTaskFailure(t *testing.T) {
	tracker := newDevWatcherStartupTracker([]devWatcherStartupExpectation{{ID: "build", WaitOutcome: true}})
	want := &devWatcherStartupError{Watcher: "Build App", Command: "go build ./...", Err: errors.New("exit status 1")}
	tracker.noteOutcome("build", want)

	err := tracker.wait(context.Background())
	if !errors.Is(err, want.Err) {
		t.Fatalf("startup error = %v, want wrapped process error", err)
	}
	for _, part := range []string{"Build App", "go build ./...", "exit status 1"} {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("startup error missing %q: %v", part, err)
		}
	}
}

// TestDevWatcherStartupTrackerCompletesAfterConcurrentFailures verifies one retained cause does not strand another failed task.
func TestDevWatcherStartupTrackerCompletesAfterConcurrentFailures(t *testing.T) {
	tracker := newDevWatcherStartupTracker([]devWatcherStartupExpectation{
		{ID: "build", WaitOutcome: true},
		{ID: "frontend", WaitOutcome: true},
	})
	first := errors.New("build failed")
	tracker.noteOutcome("build", first)
	tracker.noteOutcome("frontend", errors.New("frontend failed"))
	if err := tracker.wait(context.Background()); !errors.Is(err, first) {
		t.Fatalf("startup error = %v, want first failure", err)
	}
}

// TestDevWatcherStartupTrackerTreatsCustomLaunchAsReady verifies long-running custom processes do not block the outer loop.
func TestDevWatcherStartupTrackerTreatsCustomLaunchAsReady(t *testing.T) {
	tracker := newDevWatcherStartupTracker([]devWatcherStartupExpectation{{ID: "desktop"}})
	tracker.noteTrigger("desktop")
	if err := tracker.wait(context.Background()); err != nil {
		t.Fatalf("wait custom startup: %v", err)
	}
}

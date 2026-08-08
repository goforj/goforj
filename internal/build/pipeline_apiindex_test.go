package build

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/apiindex"
	"github.com/goforj/goforj/internal/logger"
)

// pipelineAPIIndexTestTimeout bounds synchronization failures without making ordinary pipeline work timing-dependent.
const pipelineAPIIndexTestTimeout = 5 * time.Second

// recordingAPIIndexPreparer exposes one focused preparation function to build integration tests.
type recordingAPIIndexPreparer struct {
	prepare func(apiindex.Options) (apiindex.Preparation, error)
}

// TestPipelineRunDoesNotChangeProcessWorkingDirectory proves an in-flight build cannot redirect unrelated goroutines into its project root.
func TestPipelineRunDoesNotChangeProcessWorkingDirectory(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("read working directory: %v", err)
	}
	root := t.TempDir()
	preparing := make(chan struct{})
	release := make(chan struct{})
	preparer := recordingAPIIndexPreparer{prepare: func(options apiindex.Options) (apiindex.Preparation, error) {
		if options.Root != root {
			return apiindex.Preparation{}, errors.New("API index received the wrong project root")
		}
		close(preparing)
		<-release
		return apiindex.Preparation{Status: "skipped"}, nil
	}}
	pipeline := NewPipeline(logger.NewSilentLogger(), preparer)
	done := make(chan error, 1)
	go func() {
		done <- pipeline.Run(root, "test", Step{Name: "finish", Run: func(stepRoot string) (string, error) {
			if stepRoot != root {
				return "", errors.New("final step received the wrong project root")
			}
			return "finished", nil
		}}, RunOptions{SkipWire: true})
	}()

	select {
	case <-preparing:
	case err := <-done:
		t.Fatalf("pipeline stopped before API index preparation: %v", err)
	case <-time.After(pipelineAPIIndexTestTimeout):
		t.Fatal("pipeline did not reach API index preparation")
	}
	current, err := os.Getwd()
	if err != nil {
		close(release)
		t.Fatalf("read working directory during pipeline: %v", err)
	}
	if current != workingDirectory {
		close(release)
		t.Fatalf("working directory changed to %q during pipeline, want %q", current, workingDirectory)
	}
	close(release)
	if err := awaitPipelineCompletion(t, done); err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
}

// TestPipelinePreparationOnlyStopsBeforeIndexAndFinal verifies dev waves can stabilize writable inputs separately.
func TestPipelinePreparationOnlyStopsBeforeIndexAndFinal(t *testing.T) {
	indexed := false
	finished := false
	pipeline := NewPipeline(logger.NewSilentLogger(), recordingAPIIndexPreparer{prepare: func(apiindex.Options) (apiindex.Preparation, error) {
		indexed = true
		return apiindex.Preparation{}, nil
	}})
	err := pipeline.Run(t.TempDir(), "build", Step{Name: "finish", Run: func(string) (string, error) {
		finished = true
		return "", nil
	}}, RunOptions{PreparationOnly: true, SkipWire: true})
	if err != nil {
		t.Fatalf("prepare pipeline: %v", err)
	}
	if indexed || finished {
		t.Fatalf("preparation continued into index or final: indexed=%t finished=%t", indexed, finished)
	}
}

// TestPipelineSkipPreparationStartsFromStableSnapshot verifies compile phases do not touch generated project inputs.
func TestPipelineSkipPreparationStartsFromStableSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("render: [invalid\n"), 0o644); err != nil {
		t.Fatalf("write invalid project config: %v", err)
	}
	indexed := false
	finished := false
	pipeline := NewPipeline(logger.NewSilentLogger(), recordingAPIIndexPreparer{prepare: func(apiindex.Options) (apiindex.Preparation, error) {
		indexed = true
		return apiindex.Preparation{Status: "indexed"}, nil
	}})
	err := pipeline.Run(root, "build", Step{Name: "finish", Run: func(string) (string, error) {
		finished = true
		return "", nil
	}}, RunOptions{SkipPreparation: true})
	if err != nil {
		t.Fatalf("compile stable snapshot: %v", err)
	}
	if !indexed || !finished {
		t.Fatalf("stable snapshot skipped index or final: indexed=%t finished=%t", indexed, finished)
	}
}

// TestPipelineRejectsConflictingDevPhases covers the coordinator's invalid internal state.
func TestPipelineRejectsConflictingDevPhases(t *testing.T) {
	pipeline := NewPipeline(logger.NewSilentLogger(), recordingAPIIndexPreparer{})
	err := pipeline.Run(t.TempDir(), "build", Step{Name: "finish"}, RunOptions{
		PreparationOnly: true,
		SkipPreparation: true,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot prepare and skip preparation") {
		t.Fatalf("conflicting phase error = %v", err)
	}
}

// awaitPipelineCompletion prevents a post-release pipeline regression from hanging the package test process.
func awaitPipelineCompletion(t *testing.T, done <-chan error) error {
	t.Helper()
	timer := time.NewTimer(pipelineAPIIndexTestTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		t.Fatal("timed out waiting for pipeline completion")
		return nil
	}
}

// recordingAPIIndexCandidate exposes publication callbacks without depending on staged artifact internals.
type recordingAPIIndexCandidate struct {
	publish func() error
	discard func() error
}

// Prepare records the pipeline request without requiring API-index implementation details.
func (p recordingAPIIndexPreparer) Prepare(options apiindex.Options) (apiindex.Preparation, error) {
	return p.prepare(options)
}

// Publish records when the build pipeline crosses its final success boundary.
func (c recordingAPIIndexCandidate) Publish() error {
	return c.publish()
}

// Discard records candidate cleanup and returns the outcome needed to verify pipeline error joining.
func (c recordingAPIIndexCandidate) Discard() error {
	return c.discard()
}

// TestPipelineRunReportsCandidateDiscardFailures verifies cleanup cannot disappear behind either success or an earlier pipeline failure.
func TestPipelineRunReportsCandidateDiscardFailures(t *testing.T) {
	finalErr := errors.New("final step failed")
	publishErr := errors.New("publication failed")
	discardErr := errors.New("discard failed")
	tests := []struct {
		name        string
		finalErr    error
		publishErr  error
		wantPublish bool
	}{
		{name: "successful pipeline", wantPublish: true},
		{name: "final step failure", finalErr: finalErr},
		{name: "publication failure", publishErr: publishErr, wantPublish: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publishCalls := 0
			discardCalls := 0
			candidate := recordingAPIIndexCandidate{
				publish: func() error {
					publishCalls++
					return test.publishErr
				},
				discard: func() error {
					discardCalls++
					return discardErr
				},
			}
			preparer := recordingAPIIndexPreparer{prepare: func(apiindex.Options) (apiindex.Preparation, error) {
				return apiindex.Preparation{Candidate: candidate, Status: "prepared"}, nil
			}}
			pipeline := NewPipeline(logger.NewSilentLogger(), preparer)
			err := pipeline.Run(t.TempDir(), "test", Step{
				Name: "final",
				Run:  func(string) (string, error) { return "finished", test.finalErr },
			}, RunOptions{SkipWire: true})

			if !errors.Is(err, discardErr) {
				t.Fatalf("pipeline error = %v, want discard failure", err)
			}
			for _, primaryErr := range []error{test.finalErr, test.publishErr} {
				if primaryErr != nil && !errors.Is(err, primaryErr) {
					t.Fatalf("pipeline error = %v, want primary failure %v", err, primaryErr)
				}
			}
			wantPublishCalls := 0
			if test.wantPublish {
				wantPublishCalls = 1
			}
			if publishCalls != wantPublishCalls {
				t.Fatalf("publish calls = %d, want %d", publishCalls, wantPublishCalls)
			}
			if discardCalls != 1 {
				t.Fatalf("discard calls = %d, want 1", discardCalls)
			}
		})
	}
}

// TestRunFinalAndPublishAPIIndexPublishesAfterSuccess verifies compilation remains the candidate's commit gate.
func TestRunFinalAndPublishAPIIndexPublishesAfterSuccess(t *testing.T) {
	var events []string
	candidate := recordingAPIIndexCandidate{
		publish: func() error {
			events = append(events, "publish")
			return nil
		},
		discard: func() error { return nil },
	}
	status, err := runFinalAndPublishAPIIndex(t.TempDir(), Step{
		Name: "go build",
		Run: func(string) (string, error) {
			events = append(events, "final")
			return "compiled", nil
		},
	}, candidate)
	if err != nil {
		t.Fatalf("run final step and publish: %v", err)
	}
	if status != "compiled" {
		t.Fatalf("final status = %q, want compiled", status)
	}
	if want := []string{"final", "publish"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

// TestRunFinalAndPublishAPIIndexReturnsPublicationFailure verifies a successful binary cannot hide a failed contract commit.
func TestRunFinalAndPublishAPIIndexReturnsPublicationFailure(t *testing.T) {
	publishErr := errors.New("publish candidate")
	candidate := recordingAPIIndexCandidate{
		publish: func() error { return publishErr },
		discard: func() error { return nil },
	}
	status, err := runFinalAndPublishAPIIndex(t.TempDir(), Step{
		Name: "go build",
		Run:  func(string) (string, error) { return "compiled", nil },
	}, candidate)
	if !errors.Is(err, publishErr) {
		t.Fatalf("publication error = %v, want %v", err, publishErr)
	}
	if status != "" {
		t.Fatalf("failed publication status = %q, want empty", status)
	}
}

// TestPipelinePrepareAPIIndexThreadsStrictAndReportsCounts verifies ordinary build pipelines share strict policy and report formatting.
func TestPipelinePrepareAPIIndexThreadsStrictAndReportsCounts(t *testing.T) {
	strict := false
	receivedRoot := ""
	preparer := recordingAPIIndexPreparer{prepare: func(options apiindex.Options) (apiindex.Preparation, error) {
		strict = options.Strict
		receivedRoot = options.Root
		return apiindex.Preparation{Status: "app customer-portal, unchanged, 7 operations, 3 schemas, 1 diagnostic"}, nil
	}}
	pipeline := NewPipeline(logger.NewSilentLogger(), preparer)
	root := t.TempDir()
	preparation, err := pipeline.prepareAPIIndex(root, true)
	if err != nil {
		t.Fatalf("prepare strict pipeline API index: %v", err)
	}
	if preparation.Candidate != nil {
		t.Fatal("expected focused preparer not to create a transaction")
	}
	if !strict {
		t.Fatal("expected strict policy to reach the shared preparer")
	}
	if receivedRoot != root {
		t.Fatalf("API index root = %q, want %q", receivedRoot, root)
	}
	want := "app customer-portal, unchanged, 7 operations, 3 schemas, 1 diagnostic"
	if preparation.Status != want {
		t.Fatalf("pipeline API index status = %q, want %q", preparation.Status, want)
	}
}

// TestBuildAndRunCommandsExposeAPIIndexStrictFlag verifies ordinary commands avoid claiming the generic strict flag.
func TestBuildAndRunCommandsExposeAPIIndexStrictFlag(t *testing.T) {
	appLogger := logger.NewSilentLogger()
	runner := apiindex.NewRunner(ActiveApp)
	buildCommand := NewCmd(appLogger, runner)
	buildParser, err := kong.New(buildCommand)
	if err != nil {
		t.Fatalf("create build parser: %v", err)
	}
	if _, err := buildParser.Parse([]string{"--api-index-strict"}); err != nil {
		t.Fatalf("parse build API index strict flag: %v", err)
	}
	if !buildCommand.APIIndexStrict {
		t.Fatal("expected build --api-index-strict to be enabled")
	}

	runCommand := NewRunCmd(appLogger, runner)
	runParser, err := kong.New(runCommand)
	if err != nil {
		t.Fatalf("create run API index strict parser: %v", err)
	}
	if _, err := runParser.Parse([]string{"--api-index-strict"}); err != nil {
		t.Fatalf("parse run API index strict flag: %v", err)
	}
	if !runCommand.APIIndexStrict {
		t.Fatal("expected run --api-index-strict to be enabled")
	}

	for name, command := range map[string]any{
		"build": NewCmd(appLogger, runner),
		"run":   NewRunCmd(appLogger, runner),
	} {
		parser, parseErr := kong.New(command)
		if parseErr != nil {
			t.Fatalf("create %s generic strict parser: %v", name, parseErr)
		}
		_, _ = parser.Parse([]string{"--strict"})
		switch typed := command.(type) {
		case *Cmd:
			if typed.APIIndexStrict {
				t.Fatal("build --strict unexpectedly enabled API index strict mode")
			}
		case *RunCmd:
			if typed.APIIndexStrict {
				t.Fatal("run --strict unexpectedly enabled API index strict mode")
			}
		}
	}
}

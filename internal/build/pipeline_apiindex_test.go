package build

import (
	"errors"
	"reflect"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/apiindex"
	"github.com/goforj/goforj/internal/logger"
)

// recordingAPIIndexPreparer exposes one focused preparation function to build integration tests.
type recordingAPIIndexPreparer struct {
	prepare func(apiindex.Options) (apiindex.Candidate, string, error)
}

// recordingAPIIndexCandidate exposes publication callbacks without depending on staged artifact internals.
type recordingAPIIndexCandidate struct {
	publish func() error
	discard func()
}

// Prepare records the pipeline request without requiring API-index implementation details.
func (p recordingAPIIndexPreparer) Prepare(options apiindex.Options) (apiindex.Candidate, string, error) {
	return p.prepare(options)
}

// Publish records when the build pipeline crosses its final success boundary.
func (c recordingAPIIndexCandidate) Publish() error {
	return c.publish()
}

// Discard records candidate cleanup after either pipeline outcome.
func (c recordingAPIIndexCandidate) Discard() {
	c.discard()
}

// TestRunFinalAndPublishAPIIndexPublishesAfterSuccess verifies compilation remains the candidate's commit gate.
func TestRunFinalAndPublishAPIIndexPublishesAfterSuccess(t *testing.T) {
	var events []string
	candidate := recordingAPIIndexCandidate{
		publish: func() error {
			events = append(events, "publish")
			return nil
		},
		discard: func() {},
	}
	status, err := runFinalAndPublishAPIIndex(Step{
		Name: "go build",
		Run: func() (string, error) {
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
		discard: func() {},
	}
	status, err := runFinalAndPublishAPIIndex(Step{
		Name: "go build",
		Run:  func() (string, error) { return "compiled", nil },
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
	preparer := recordingAPIIndexPreparer{prepare: func(options apiindex.Options) (apiindex.Candidate, string, error) {
		strict = options.Strict
		return nil, "app customer-portal, unchanged, 7 operations, 3 schemas, 1 diagnostic", nil
	}}
	pipeline := NewPipeline(logger.NewSilentLogger(), preparer)
	pending, status, err := pipeline.prepareAPIIndex(true)
	if err != nil {
		t.Fatalf("prepare strict pipeline API index: %v", err)
	}
	if pending != nil {
		t.Fatal("expected focused preparer not to create a transaction")
	}
	if !strict {
		t.Fatal("expected strict policy to reach the shared preparer")
	}
	want := "app customer-portal, unchanged, 7 operations, 3 schemas, 1 diagnostic"
	if status != want {
		t.Fatalf("pipeline API index status = %q, want %q", status, want)
	}
}

// TestBuildAndRunCommandsExposeAPIIndexStrictFlag verifies ordinary commands avoid claiming the generic strict flag.
func TestBuildAndRunCommandsExposeAPIIndexStrictFlag(t *testing.T) {
	appLogger := logger.NewSilentLogger()
	runner := apiindex.NewRunner(appLogger, ActiveApp)
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

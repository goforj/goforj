package apiindex

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// stubDefaultRunner records standalone command options without exposing Runner internals.
type stubDefaultRunner struct {
	run func(Options) (string, error)
}

// RunDefault delegates to the scenario callback so command tests can control status and failure.
func (r stubDefaultRunner) RunDefault(options Options) (string, error) {
	return r.run(options)
}

// TestCmdRegistersAndRunsStandalone verifies strict policy and structured results use the shared runner path.
func TestCmdRegistersAndRunsStandalone(t *testing.T) {
	called := false
	strict := false
	var buildTags []string
	runner := stubDefaultRunner{run: func(options Options) (string, error) {
		called = true
		strict = options.Strict
		buildTags = append([]string(nil), options.BuildTags...)
		return "app app, changed, 4 operations, 2 schemas, 1 diagnostic", nil
	}}
	var output bytes.Buffer
	command := &Cmd{runner: runner, stdout: &output}
	root := struct {
		Cmd Cmd `cmd:""`
	}{
		Cmd: *command,
	}
	parser, err := kong.New(&root)
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}
	context, err := parser.Parse([]string{"build:api-index", "--strict", "--tags=contract,dev"})
	if err != nil {
		t.Fatalf("parse standalone API index command: %v", err)
	}
	if err := context.Run(); err != nil {
		t.Fatalf("run standalone API index command: %v", err)
	}
	if !called {
		t.Fatal("expected standalone command to use the shared API index runner")
	}
	if !strict {
		t.Fatal("expected --strict to reach the shared API index runner")
	}
	if !reflect.DeepEqual(buildTags, []string{"contract", "dev"}) {
		t.Fatalf("standalone build tags = %v, want contract and dev", buildTags)
	}
	want := "app app, changed, 4 operations, 2 schemas, 1 diagnostic\n"
	if output.String() != want {
		t.Fatalf("standalone output = %q, want %q", output.String(), want)
	}
}

// TestCmdStrictErrorIncludesReportAndCause verifies failed standalone indexing remains contextual and unwraps the analyzer error.
func TestCmdStrictErrorIncludesReportAndCause(t *testing.T) {
	diagnosticsErr := errors.New("strict diagnostics")
	runner := stubDefaultRunner{run: func(options Options) (string, error) {
		if !options.Strict {
			t.Fatal("expected strict standalone options")
		}
		return "app customer-portal, rejected, 6 operations, 4 schemas, 2 diagnostics", diagnosticsErr
	}}
	var output bytes.Buffer
	command := &Cmd{runner: runner, Strict: true, stdout: &output}

	err := command.Run()
	if !errors.Is(err, diagnosticsErr) {
		t.Fatalf("standalone strict error = %v, want wrapped analyzer error", err)
	}
	wantContext := "app customer-portal, rejected, 6 operations, 4 schemas, 2 diagnostics"
	if !strings.Contains(err.Error(), wantContext) {
		t.Fatalf("standalone strict error %q does not include %q", err, wantContext)
	}
	if output.Len() != 0 {
		t.Fatalf("strict failure wrote a success report: %q", output.String())
	}
}

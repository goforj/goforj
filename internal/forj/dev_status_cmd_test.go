package forj

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/project"
)

// TestParseDevStatusComposeOutputAcceptsSupportedFormats verifies compatibility with both Compose JSON encodings.
func TestParseDevStatusComposeOutputAcceptsSupportedFormats(t *testing.T) {
	t.Parallel()
	records := []devStatusComposeRecord{
		{ID: "a1", Name: "demo-api-1", Project: "demo", Service: "api", State: "running", Health: "healthy"},
		{ID: "b2", Name: "demo-api-2", Project: "demo", Service: "api", State: "running"},
	}
	arraySource, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	ndjsonSource := []byte(strings.Join([]string{
		`{"ID":"a1","Name":"demo-api-1","Project":"demo","Service":"api","State":"running","Health":"healthy","Command":"must not escape"}`,
		`{"ID":"b2","Name":"demo-api-2","Project":"demo","Service":"api","State":"running"}`,
	}, "\n"))

	for _, test := range []struct {
		name   string
		source []byte
	}{
		{name: "array", source: arraySource},
		{name: "newline delimited", source: ndjsonSource},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDevStatusComposeOutput(test.source)
			if err != nil {
				t.Fatalf("parseDevStatusComposeOutput() error = %v", err)
			}
			if !reflect.DeepEqual(got, records) {
				t.Fatalf("parseDevStatusComposeOutput() = %#v, want %#v", got, records)
			}
		})
	}
}

// TestAggregateDevStatusServicesCombinesReplicasDeterministically verifies stable service and container ordering.
func TestAggregateDevStatusServicesCombinesReplicasDeterministically(t *testing.T) {
	t.Parallel()
	records := []devStatusComposeRecord{
		{ID: "api-2", Name: "demo-api-2", Project: "demo", Service: "api", State: "running", Health: "healthy"},
		{ID: "worker-1", Name: "demo-worker-1", Project: "demo", Service: "worker", State: "exited", ExitCode: 1},
		{ID: "api-1", Name: "demo-api-1", Project: "demo", Service: "api", State: "running"},
	}

	projectName, services, err := aggregateDevStatusServices(records)
	if err != nil {
		t.Fatalf("aggregateDevStatusServices() error = %v", err)
	}
	if projectName != "demo" {
		t.Fatalf("aggregateDevStatusServices() project = %q, want demo", projectName)
	}
	want := []DevStatusService{
		{
			ID:     "api",
			Name:   "api",
			Kind:   "compose",
			State:  "ready",
			Active: true,
			Containers: []DevStatusContainer{
				{ID: "api-1", Name: "demo-api-1", State: "running"},
				{ID: "api-2", Name: "demo-api-2", State: "running", Health: "healthy"},
			},
		},
		{
			ID:    "worker",
			Name:  "worker",
			Kind:  "compose",
			State: "failed",
			Containers: []DevStatusContainer{
				{ID: "worker-1", Name: "demo-worker-1", State: "exited", ExitCode: 1},
			},
		},
	}
	if !reflect.DeepEqual(services, want) {
		t.Fatalf("aggregateDevStatusServices() = %#v, want %#v", services, want)
	}
}

// TestAggregateDevStatusServiceStateCoversRuntimeStates verifies the service vocabulary remains stable across Compose states.
func TestAggregateDevStatusServiceStateCoversRuntimeStates(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		containers []DevStatusContainer
		state      string
		active     bool
	}{
		{name: "ready", containers: []DevStatusContainer{{State: "running", Health: "healthy"}}, state: "ready", active: true},
		{name: "working", containers: []DevStatusContainer{{State: "restarting"}}, state: "working", active: true},
		{name: "degraded health", containers: []DevStatusContainer{{State: "running", Health: "unhealthy"}}, state: "degraded", active: true},
		{name: "degraded replicas", containers: []DevStatusContainer{{State: "running"}, {State: "exited"}}, state: "degraded", active: true},
		{name: "failed", containers: []DevStatusContainer{{State: "exited", ExitCode: 2}}, state: "failed", active: false},
		{name: "stopped", containers: []DevStatusContainer{{State: "exited"}}, state: "stopped", active: false},
		{name: "unavailable", containers: []DevStatusContainer{{State: "unknown"}}, state: "unavailable", active: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			state, active := aggregateDevStatusServiceState(test.containers)
			if state != test.state || active != test.active {
				t.Fatalf("aggregateDevStatusServiceState() = (%q, %t), want (%q, %t)", state, active, test.state, test.active)
			}
		})
	}
}

// TestDevStatusCmdReportsCapabilityBoundaries verifies no-Compose and custom-task results remain successful typed reports.
func TestDevStatusCmdReportsCapabilityBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		tasks     []project.DevTask
		supported bool
		problem   string
	}{
		{name: "no Compose", tasks: nil, supported: true},
		{
			name:      "custom Compose task",
			tasks:     []project.DevTask{{Name: "Run Docker Compose", Cmd: "docker compose --profile custom up -d"}},
			supported: false,
			problem:   devStatusUnsupportedComposeProblem,
		},
		{
			name:      "custom Compose task name",
			tasks:     []project.DevTask{{Name: "Start containers", Cmd: "docker compose up -d"}},
			supported: false,
			problem:   devStatusUnsupportedComposeProblem,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			report := runDevStatusCommandForTest(t, test.tasks, nil)
			if report.Supported != test.supported {
				t.Fatalf("supported = %t, want %t", report.Supported, test.supported)
			}
			if report.Problem != test.problem {
				t.Fatalf("problem = %q, want %q", report.Problem, test.problem)
			}
			if report.Services == nil || len(report.Services) != 0 {
				t.Fatalf("services = %#v, want non-nil empty array", report.Services)
			}
		})
	}
}

// TestDevStatusCmdUsesMatchingComposeCLI verifies both conventions become argument-vector queries without a shell.
func TestDevStatusCmdUsesMatchingComposeCLI(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		startup    string
		executable string
		arguments  []string
	}{
		{
			name:       "standalone binary",
			startup:    "docker-compose up -d",
			executable: "docker-compose",
			arguments:  []string{"ps", "--all", "--format", "json"},
		},
		{
			name:       "Docker plugin",
			startup:    "docker compose up -d",
			executable: "docker",
			arguments:  []string{"compose", "ps", "--all", "--format", "json"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var executable string
			var arguments []string
			runner := func(_ context.Context, gotExecutable string, gotArguments []string, stdout io.Writer) error {
				executable = gotExecutable
				arguments = append([]string(nil), gotArguments...)
				_, err := io.WriteString(stdout, "[]")
				return err
			}
			report := runDevStatusCommandForTest(t, []project.DevTask{{Name: "Run Docker Compose", Cmd: test.startup}}, runner)
			if report.Problem != "" {
				t.Fatalf("problem = %q, want empty", report.Problem)
			}
			if executable != test.executable || !reflect.DeepEqual(arguments, test.arguments) {
				t.Fatalf("runner = %q %#v, want %q %#v", executable, arguments, test.executable, test.arguments)
			}
		})
	}
}

// TestDevStatusCmdReportsObservationFailures verifies malformed and oversized output never break the JSON capability contract.
func TestDevStatusCmdReportsObservationFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		runner devStatusComposeRunner
	}{
		{
			name: "runtime failure",
			runner: func(context.Context, string, []string, io.Writer) error {
				return errors.New("runtime unavailable")
			},
		},
		{
			name: "malformed JSON",
			runner: func(_ context.Context, _ string, _ []string, stdout io.Writer) error {
				_, err := io.WriteString(stdout, "{")
				return err
			},
		},
		{
			name: "oversized JSON",
			runner: func(_ context.Context, _ string, _ []string, stdout io.Writer) error {
				_, err := stdout.Write(bytes.Repeat([]byte{' '}, devStatusMaximumOutputBytes+1))
				return err
			},
		},
		{
			name: "unsafe field",
			runner: func(_ context.Context, _ string, _ []string, stdout io.Writer) error {
				_, err := io.WriteString(stdout, `[{"ID":"bad id","Name":"demo-api-1","Project":"demo","Service":"api","State":"running"}]`)
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			report := runDevStatusCommandForTest(t, []project.DevTask{{Name: "Run Docker Compose", Cmd: "docker compose up -d"}}, test.runner)
			if !report.Supported {
				t.Fatalf("supported = false, want true")
			}
			if report.Problem == "" || len([]rune(report.Problem)) > devStatusMaximumProblemRunes {
				t.Fatalf("problem = %q, want bounded diagnostic", report.Problem)
			}
			if report.Services == nil || len(report.Services) != 0 {
				t.Fatalf("services = %#v, want non-nil empty array", report.Services)
			}
		})
	}
}

// TestDevStatusCmdCommandWiring verifies the hidden root command accepts its required machine flag.
func TestDevStatusCmdCommandWiring(t *testing.T) {
	t.Parallel()
	root := RootCmd{}
	parser, err := kong.New(&root)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	context, err := parser.Parse([]string{"dev:status", "--json"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if context.Command() != "dev:status" {
		t.Fatalf("Command() = %q, want dev:status", context.Command())
	}
	if !root.DevStatusCmd.JSON {
		t.Fatalf("DevStatusCmd.JSON = false, want true")
	}
}

// TestDevStatusCmdRequiresJSON verifies the human invocation cannot accidentally receive a machine-only payload.
func TestDevStatusCmdRequiresJSON(t *testing.T) {
	t.Parallel()
	err := (&DevStatusCmd{}).Run()
	if err == nil || !strings.Contains(err.Error(), "requires --json") {
		t.Fatalf("Run() error = %v, want --json requirement", err)
	}
}

// runDevStatusCommandForTest executes the command with deterministic task and process boundaries.
func runDevStatusCommandForTest(t *testing.T, tasks []project.DevTask, runner devStatusComposeRunner) DevStatusReport {
	t.Helper()
	var output bytes.Buffer
	command := &DevStatusCmd{
		JSON:   true,
		stdout: &output,
		loadTasks: func() ([]project.DevTask, error) {
			return append([]project.DevTask(nil), tasks...), nil
		},
		run: runner,
	}
	if err := command.Run(); err != nil {
		t.Fatalf("DevStatusCmd.Run() error = %v", err)
	}
	var report DevStatusReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; output = %q", err, output.String())
	}
	return report
}

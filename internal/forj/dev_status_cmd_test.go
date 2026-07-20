package forj

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/forj/resources"
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
			if report.Resources == nil || len(report.Resources) != 0 {
				t.Fatalf("resources = %#v, want non-nil empty array", report.Resources)
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
			if report.Resources == nil || len(report.Resources) != 0 {
				t.Fatalf("resources = %#v, want non-nil empty array", report.Resources)
			}
		})
	}
}

// TestDevStatusCmdProjectsHostResources verifies assigned addresses and explicit ownership without running generated App code.
func TestDevStatusCmdProjectsHostResources(t *testing.T) {
	t.Parallel()
	config := &project.Config{}
	config.Render.Components.WebAPI = true
	config.Render.Components.Docker = true
	loadCount := 0
	runCount := 0
	var output bytes.Buffer
	command := &DevStatusCmd{
		JSON:   true,
		stdout: &output,
		loadProject: func() (devStatusProjectContext, error) {
			loadCount++
			return devStatusProjectContext{
				config: config,
				environment: map[string]string{
					"APP_URL":                "http://localhost:3000",
					"IP_ADDRESS":             "127.77.42.18",
					"LIGHTHOUSE_URL":         "ws://localhost:3000/lighthouse/ws/agent",
					"API_SWAGGER_ENABLED":    "true",
					"COMPOSE_PROFILES":       "mailpit,grafana",
					"MAILPIT_HTTP_PORT":      "8025",
					"OBSERVABILITY_VM_PORT":  "8428",
					"GRAFANA_PORT":           "13001",
					"GRAFANA_ADMIN_USER":     "must-not-escape",
					"GRAFANA_ADMIN_PASSWORD": "also-must-not-escape",
				},
				tasks: []project.DevTask{{Name: "Run Docker Compose", Cmd: "docker compose up -d"}},
			}, nil
		},
		run: func(_ context.Context, executable string, arguments []string, stdout io.Writer) error {
			runCount++
			if executable != "docker" || !reflect.DeepEqual(arguments, []string{"compose", "ps", "--all", "--format", "json"}) {
				t.Fatalf("Compose query = %q %#v", executable, arguments)
			}
			_, err := io.WriteString(stdout, "[]")
			return err
		},
	}
	if err := command.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if loadCount != 1 || runCount != 1 {
		t.Fatalf("loads = %d, Compose queries = %d; want one host config load and one Compose query", loadCount, runCount)
	}
	var report DevStatusReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if report.Problem != "" || report.ResourceProblem != "" || report.Resources == nil {
		t.Fatalf("status problems/resources = (%q, %q, %#v)", report.Problem, report.ResourceProblem, report.Resources)
	}
	for _, expected := range []struct {
		id      string
		url     string
		app     string
		service string
	}{
		{id: "app", url: "http://127.77.42.18:3000", app: project.DefaultAppName},
		{id: "api", url: "http://127.77.42.18:3000", app: project.DefaultAppName},
		{id: "swagger", url: "http://127.77.42.18:3000/swagger", app: project.DefaultAppName},
		{id: "lighthouse", url: "http://127.77.42.18:3000/lighthouse", app: project.DefaultAppName},
		{id: "mailpit", url: "http://127.77.42.18:8025", service: "mailpit"},
		{id: "victoria-metrics", url: "http://127.77.42.18:8428", service: "victoriametrics"},
		{id: "grafana", url: "http://127.77.42.18:13001", service: "grafana"},
	} {
		resource, found := devStatusResourceByID(report.Resources, expected.id)
		if !found || resource.URL != expected.url || resource.App != expected.app || resource.Service != expected.service {
			t.Errorf("resource %q = %#v found=%t", expected.id, resource, found)
		}
	}
	serialized := output.String()
	for _, secret := range []string{"must-not-escape", "also-must-not-escape", `"auth":`, `"source":`} {
		if strings.Contains(strings.ToLower(serialized), strings.ToLower(secret)) {
			t.Fatalf("status output exposed excluded resource data %q: %s", secret, serialized)
		}
	}
}

// TestDevStatusCmdKeepsResourceFailuresIndependent verifies Compose state survives partial registry errors.
func TestDevStatusCmdKeepsResourceFailuresIndependent(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	command := &DevStatusCmd{
		JSON:   true,
		stdout: &output,
		loadProject: func() (devStatusProjectContext, error) {
			return devStatusProjectContext{
				config:      &project.Config{},
				environment: map[string]string{},
				tasks:       []project.DevTask{{Name: "Run Docker Compose", Cmd: "docker-compose up -d"}},
			}, nil
		},
		resolveResources: func(context.Context, *project.Config, map[string]string) ([]resources.Resource, error) {
			return []resources.Resource{{ID: "app", Name: "App", Category: "app", URL: "http://localhost:3000", Enabled: true, App: project.DefaultAppName, Owner: "goforj"}}, errors.New("secondary resolver unavailable")
		},
		run: func(_ context.Context, _ string, _ []string, stdout io.Writer) error {
			_, err := io.WriteString(stdout, `[{"ID":"mysql-1","Name":"demo-mysql-1","Project":"demo","Service":"mysql","State":"running","Health":"healthy"}]`)
			return err
		},
	}
	if err := command.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	var report DevStatusReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if report.Problem != "" || report.ResourceProblem == "" || len([]rune(report.ResourceProblem)) > devStatusMaximumProblemRunes {
		t.Fatalf("problems = (%q, %q), want bounded resource-only problem", report.Problem, report.ResourceProblem)
	}
	if len(report.Services) != 1 || report.Services[0].ID != "mysql" || len(report.Resources) != 1 || report.Resources[0].ID != "app" {
		t.Fatalf("partial report = services %#v resources %#v", report.Services, report.Resources)
	}
}

// TestProjectDevStatusResourcesValidatesAndBounds verifies disabled, duplicate, unsafe, and oversized entries are contained.
func TestProjectDevStatusResourcesValidatesAndBounds(t *testing.T) {
	t.Parallel()
	resourcesToProject := []resources.Resource{
		{ID: "disabled", Name: "Disabled", Category: "tool", Enabled: false},
		{ID: "mailpit", Name: "Mailpit", Category: "mail", URL: "http://localhost:8025", Description: "Local inbox.", Enabled: true, Service: "mailpit", Auth: "secret", Source: "profile"},
		{ID: "mailpit", Name: "Duplicate", Category: "mail", Enabled: true},
		{ID: "unsafe", Name: "Unsafe\nName", Category: "tool", Enabled: true},
		{ID: "credential-url", Name: "Credential URL", Category: "tool", URL: "http://user:password@localhost:3000", Enabled: true},
		{ID: "dual-owner", Name: "Dual owner", Category: "tool", Enabled: true, App: "app", Service: "tool"},
	}
	projected, err := projectDevStatusResources(resourcesToProject)
	if err == nil {
		t.Fatal("projectDevStatusResources() error = nil, want partial validation problem")
	}
	if len(projected) != 1 || projected[0].ID != "mailpit" || projected[0].Service != "mailpit" {
		t.Fatalf("projected resources = %#v", projected)
	}
	serialized, marshalErr := json.Marshal(projected)
	if marshalErr != nil {
		t.Fatalf("json.Marshal() error = %v", marshalErr)
	}
	if bytes.Contains(serialized, []byte("secret")) || bytes.Contains(serialized, []byte("profile")) {
		t.Fatalf("projected resources exposed excluded fields: %s", serialized)
	}

	many := make([]resources.Resource, 0, devStatusMaximumResources+1)
	for index := 0; index <= devStatusMaximumResources; index++ {
		many = append(many, resources.Resource{ID: fmt.Sprintf("resource-%03d", index), Name: "Resource", Category: "tool", Enabled: true})
	}
	bounded, err := projectDevStatusResources(many)
	if err == nil || len(bounded) != devStatusMaximumResources {
		t.Fatalf("bounded resources = %d error=%v, want %d and error", len(bounded), err, devStatusMaximumResources)
	}
}

// TestDevStatusCmdOmitsDisabledResources verifies registry filtering cannot advertise dormant project surfaces.
func TestDevStatusCmdOmitsDisabledResources(t *testing.T) {
	t.Parallel()
	config := &project.Config{}
	config.Render.Components.WebAPI = true
	config.Render.Components.Docker = true
	var output bytes.Buffer
	command := &DevStatusCmd{
		JSON:   true,
		stdout: &output,
		loadProject: func() (devStatusProjectContext, error) {
			return devStatusProjectContext{
				config: config,
				environment: map[string]string{
					"LIGHTHOUSE_ENABLED":  "false",
					"API_SWAGGER_ENABLED": "false",
					"COMPOSE_PROFILES":    "mailpit-debug,grafana-preview",
				},
			}, nil
		},
	}
	if err := command.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	var report DevStatusReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, id := range []string{"lighthouse", "swagger", "mailpit", "victoria-metrics", "grafana"} {
		if _, found := devStatusResourceByID(report.Resources, id); found {
			t.Fatalf("disabled resource %q present in %#v", id, report.Resources)
		}
	}
	if report.Resources == nil {
		t.Fatal("resources = nil, want stable array")
	}
}

// TestProcessEnvironmentMapPreservesValues verifies process entries split once and ignore malformed pseudo-keys.
func TestProcessEnvironmentMapPreservesValues(t *testing.T) {
	t.Parallel()
	got := processEnvironmentMap([]string{"APP_URL=http://localhost:3000?a=b", "EMPTY=", "MALFORMED", "=platform"})
	want := map[string]string{"APP_URL": "http://localhost:3000?a=b", "EMPTY": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("processEnvironmentMap() = %#v, want %#v", got, want)
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

// devStatusResourceByID returns one projected resource for focused contract assertions.
func devStatusResourceByID(projectResources []DevStatusResource, id string) (DevStatusResource, bool) {
	for _, resource := range projectResources {
		if resource.ID == id {
			return resource, true
		}
	}
	return DevStatusResource{}, false
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
		loadProject: func() (devStatusProjectContext, error) {
			return devStatusProjectContext{
				config:      &project.Config{},
				environment: map[string]string{},
				tasks:       append([]project.DevTask(nil), tasks...),
			}, nil
		},
		resolveResources: func(context.Context, *project.Config, map[string]string) ([]resources.Resource, error) {
			return []resources.Resource{}, nil
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

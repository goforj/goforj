package forj

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

// TestProjectDescribeCmdReportsDeterministicStaticTopology verifies the descriptor has no dotenv or process-state dependency.
func TestProjectDescribeCmdReportsDeterministicStaticTopology(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd", "billing"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "billing", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("APP_KEY=must-not-escape\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	config := &project.Config{
		ProjectName:  "orders",
		GoModuleName: "example.com/orders",
		Render: project.RenderConfig{
			GoForjVersion: "0.19.0",
			Components:    project.Components{WebAPI: true},
		},
		Apps: map[string]project.AppConfig{
			"billing": {Components: project.Components{WebUI: true}},
		},
		AppKey: "must-not-escape",
	}
	t.Setenv("GOFORJ_PROJECT_DESCRIBE_TEST", "ambient-value")

	run := func() ([]byte, ProjectDescribeReport) {
		var output bytes.Buffer
		command := &ProjectDescribeCmd{
			JSON:       true,
			stdout:     &output,
			loadConfig: func() (*project.Config, error) { return config, nil },
			discover: func(string) (projectlayout.Discovery, error) {
				return projectlayout.Discover(root)
			},
			cliVersion: func() string { return "v1.2.3" },
		}
		if err := command.Run(); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		var report ProjectDescribeReport
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		return output.Bytes(), report
	}

	firstJSON, first := run()
	secondJSON, second := run()
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("descriptor changed between identical reads\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
	}
	if got := os.Getenv("GOFORJ_PROJECT_DESCRIBE_TEST"); got != "ambient-value" {
		t.Fatalf("descriptor changed process environment = %q", got)
	}
	if strings.Contains(string(firstJSON), "must-not-escape") {
		t.Fatalf("descriptor exposed a secret-like environment value: %s", firstJSON)
	}
	if first.SchemaVersion != projectDescribeSchemaVersion || first.Project.Name != "orders" || first.Project.Module != "example.com/orders" {
		t.Fatalf("descriptor identity = %#v", first)
	}
	if first.Project.ConfigDigest == "" || !strings.HasPrefix(first.Project.ConfigDigest, "sha256:") {
		t.Fatalf("config digest = %q", first.Project.ConfigDigest)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("descriptor object changed between identical reads\nfirst:  %#v\nsecond: %#v", first, second)
	}
	wantApps := []ProjectDescribeApp{
		{
			ID: "app", Name: "app", Entrypoint: "cmd/app/main.go",
			Runtimes: []ProjectDescribeRuntime{{ID: "http", Kind: "http", DefaultPort: 3000, PublicURL: true, ReadinessPath: "/-/ready"}},
		},
		{
			ID: "billing", Name: "billing", Entrypoint: "cmd/billing/main.go",
			Runtimes: []ProjectDescribeRuntime{{ID: "http", Kind: "http", DefaultPort: 3001, PublicURL: true, ReadinessPath: "/-/ready"}},
		},
	}
	if !reflect.DeepEqual(first.Apps, wantApps) {
		t.Fatalf("apps = %#v, want %#v", first.Apps, wantApps)
	}
	if !reflect.DeepEqual(first.GoForj.CLICapabilities, []string{projectDescribeCapability}) || len(first.GoForj.GeneratedProject.Capabilities) != 0 {
		t.Fatalf("capabilities = %#v", first.GoForj)
	}
}

// TestProjectDescribeCmdReportsInputFailures verifies malformed configuration and unreadable App discovery fail without a partial report.
func TestProjectDescribeCmdReportsInputFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		loadConfig func() (*project.Config, error)
		discover   func(string) (projectlayout.Discovery, error)
		want       string
	}{
		{
			name: "configuration",
			loadConfig: func() (*project.Config, error) {
				return nil, os.ErrNotExist
			},
			want: "load project configuration",
		},
		{
			name: "discovery",
			loadConfig: func() (*project.Config, error) {
				return &project.Config{}, nil
			},
			discover: func(string) (projectlayout.Discovery, error) {
				return projectlayout.Discovery{}, os.ErrPermission
			},
			want: "discover project Apps",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			command := &ProjectDescribeCmd{JSON: true, stdout: &output, loadConfig: test.loadConfig, discover: test.discover}
			err := command.Run()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Run() error = %v, want %q", err, test.want)
			}
			if output.Len() != 0 {
				t.Fatalf("Run() wrote partial report: %s", output.Bytes())
			}
		})
	}
}

// TestProjectDescribeDigestChangesWithTopology verifies the digest changes when the advertised static intent changes.
func TestProjectDescribeDigestChangesWithTopology(t *testing.T) {
	t.Parallel()
	withoutHTTP, err := projectDescribeDigest(&project.Config{Render: project.RenderConfig{}}, []ProjectDescribeApp{{ID: "app", Name: "app", Entrypoint: "cmd/app/main.go", Runtimes: []ProjectDescribeRuntime{}}})
	if err != nil {
		t.Fatalf("projectDescribeDigest() error = %v", err)
	}
	withHTTP, err := projectDescribeDigest(&project.Config{Render: project.RenderConfig{Components: project.Components{WebAPI: true}}}, []ProjectDescribeApp{{ID: "app", Name: "app", Entrypoint: "cmd/app/main.go", Runtimes: []ProjectDescribeRuntime{{ID: "http", Kind: "http", DefaultPort: 3000, PublicURL: true, ReadinessPath: "/-/ready"}}}})
	if err != nil {
		t.Fatalf("projectDescribeDigest() error = %v", err)
	}
	if withoutHTTP == withHTTP {
		t.Fatalf("digest = %q for distinct topology", withHTTP)
	}
}

// TestProjectDescribeCmdRequiresJSON verifies a human invocation cannot accidentally receive a machine contract.
func TestProjectDescribeCmdRequiresJSON(t *testing.T) {
	t.Parallel()
	err := (&ProjectDescribeCmd{}).Run()
	if err == nil || !strings.Contains(err.Error(), "requires --json") {
		t.Fatalf("Run() error = %v, want --json requirement", err)
	}
}

// TestProjectDescribeCommandWiring verifies the nested root command selects the descriptor mode.
func TestProjectDescribeCommandWiring(t *testing.T) {
	t.Parallel()
	root := RootCmd{}
	parser, err := kong.New(&root)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	context, err := parser.Parse([]string{"project:describe", "--json"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if context.Command() != "project:describe" || !root.ProjectDescribeCmd.JSON {
		t.Fatalf("command = %q %#v, want project:describe JSON mode", context.Command(), root.ProjectDescribeCmd)
	}
}

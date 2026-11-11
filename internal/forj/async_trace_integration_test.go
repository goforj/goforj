//go:build integration

package forj

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/coredeps"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestRenderedAsyncTraceSeparateProcesses verifies generated tracing across independent NATS clients and process lifetimes.
func TestRenderedAsyncTraceSeparateProcesses(t *testing.T) {
	broker, err := testkit.StartTestcontainer(t.Logf, testcontainers.ContainerRequest{
		Image: "nats:2.14.3-alpine", ExposedPorts: []string{"4222/tcp", "8222/tcp"},
		Cmd: []string{"--http_port", "8222"}, WaitingFor: wait.ForListeningPort("4222/tcp"),
	}, "4222/tcp", 30*time.Second, "async tracing NATS")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broker.Stop)
	monitorPort, err := broker.Container.MappedPort(context.Background(), "8222/tcp")
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("/tmp", "forj-async-trace-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	env := map[string]string{
		"GOWORK": "off", "TMPDIR": "/tmp",
		"QUEUE_DRIVER": "nats", "QUEUE_SUPPORTED_DRIVERS": "sync,nats", "QUEUE_WORKERS": "1",
		"QUEUE_URL":     "nats://" + net.JoinHostPort(broker.Host, broker.Port),
		"EVENTS_DRIVER": "nats", "EVENTS_SUPPORTED_DRIVERS": "inproc,nats",
		"EVENTS_URL":                         "nats://" + net.JoinHostPort(broker.Host, broker.Port),
		"EVENTS_CONTEXT_PROPAGATION_ENABLED": "true",
		"ASYNC_NATS_MONITOR":                 "http://" + net.JoinHostPort(broker.Host, monitorPort.Port()),
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := testkit.ReplaceOrAppendEnvValues([]string{filepath.Join(root, ".env")}, env); err != nil {
		t.Fatal(err)
	}
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{
		ProjectName: "AsyncTracing", GoModuleName: "example.com/asynctracing",
		Render: project.RenderConfig{Components: project.Components{CLI: true, Jobs: true, Events: true, Observability: true, Metrics: true}},
	}, EnvOverrides: env})
	if err := testkit.ReplaceOrAppendEnvValues([]string{filepath.Join(root, ".env")}, env); err != nil {
		t.Fatal(err)
	}
	generateRenderedProject(t, root, env)
	fixture, err := os.ReadFile(filepath.Join("testdata", "async_trace_process_test.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "observability", "async_trace_process_test.go"), fixture, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-race", "-count=1", "-run", "^(TestAsyncSeparateProcesses|TestProcessForwardTelemetry)$", "-timeout", "90s", "./internal/observability")
	cmd.Dir = root
	env["FORJ_ASYNC_DEMO_OTLP_ENDPOINT"] = os.Getenv("FORJ_ASYNC_DEMO_OTLP_ENDPOINT")
	env["FORJ_ASYNC_DEMO_REPORT"] = os.Getenv("FORJ_ASYNC_DEMO_REPORT")
	cmd.Env = testkit.IntegrationGoProcessEnv(t, env)
	if out, err := cmd.CombinedOutput(); err != nil {
		manifest, readErr := os.ReadFile(filepath.Join(root, "go.mod"))
		t.Fatalf("separate-process async tracing: %v\n%s\nrendered go.mod (read error: %v):\n%s", err, out, readErr, manifest)
	} else {
		t.Logf("%s", out)
	}
}

// TestRenderedAsyncTraceDriverRegenerationPins prevents adding a transport from silently selecting an incompatible latest module.
func TestRenderedAsyncTraceDriverRegenerationPins(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "forj-async-regeneration-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	env := map[string]string{"GOWORK": "off", "TMPDIR": "/tmp", "QUEUE_DRIVER": "workerpool", "QUEUE_SUPPORTED_DRIVERS": "workerpool"}
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{
		ProjectName: "AsyncRegeneration", GoModuleName: "example.com/asyncregeneration",
		Render: project.RenderConfig{Components: project.Components{CLI: true, Jobs: true}},
	}, EnvOverrides: env})
	env["QUEUE_DRIVER"] = "nats"
	env["QUEUE_SUPPORTED_DRIVERS"] = "workerpool,nats"
	if err := testkit.ReplaceOrAppendEnvValues([]string{filepath.Join(root, ".env")}, env); err != nil {
		t.Fatal(err)
	}
	generateRenderedProject(t, root, env)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Path}} {{.Version}}", "github.com/goforj/queue", "github.com/goforj/queue/driver/natsqueue")
	cmd.Dir = root
	cmd.Env = testkit.IntegrationGoProcessEnv(t, env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect regenerated transport modules: %v\n%s", err, output)
	}
	for _, module := range []string{"github.com/goforj/queue", "github.com/goforj/queue/driver/natsqueue"} {
		want := module + " " + coredeps.MustVersionFor(module)
		if !strings.Contains(string(output), want+"\n") {
			t.Errorf("regeneration must seed validated transport pins before tidy; want %s; got:\n%s", want, output)
		}
	}
}

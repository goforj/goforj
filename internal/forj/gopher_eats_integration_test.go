//go:build integration_eats

package forj

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestRenderedGopherEats exercises the selectable Vue example as three real generated app binaries.
func TestRenderedGopherEats(t *testing.T) {
	broker, err := testkit.StartTestcontainer(t.Logf, testcontainers.ContainerRequest{Image: "nats:2.14.3-alpine", ExposedPorts: []string{"4222/tcp"}, WaitingFor: wait.ForListeningPort("4222/tcp")}, "4222/tcp", 30*time.Second, "Gopher Eats NATS")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broker.Stop)
	root, err := os.MkdirTemp("/tmp", "gopher-eats-acceptance-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	endpoint := "nats://" + net.JoinHostPort(broker.Host, broker.Port)
	env := map[string]string{"GOWORK": "off", "TMPDIR": "/tmp", "QUEUE_DRIVER": "nats", "QUEUE_SUPPORTED_DRIVERS": "nats", "QUEUE_URL": endpoint, "EVENTS_DRIVER": "nats", "EVENTS_SUPPORTED_DRIVERS": "nats", "EVENTS_URL": endpoint, "EATS_TEST_BROKER_URL": endpoint}
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{ProjectName: "Gopher Eats", GoModuleName: "example.com/gophereats", Render: project.RenderConfig{Example: "gopher_eats", Components: project.Components{CLI: true}, AgentGuidance: project.AgentGuidanceNone}}, EnvOverrides: env})
	commands := []struct {
		dir  string
		name string
		args []string
	}{
		{dir: "cmd/app/frontend", name: "npm", args: []string{"ci", "--no-audit", "--no-fund"}},
		{dir: "cmd/app/frontend", name: "npm", args: []string{"run", "build"}},
		{dir: "cmd/app/frontend", name: "node", args: []string{"--test", "tests/eats-model.test.mjs"}},
		{name: "go", args: []string{"build", "-race", "-o", "./bin/", "./cmd/app", "./cmd/kitchen", "./cmd/delivery"}},
		{name: "go", args: []string{"test", "-race", "-tags=integration_eats", "./internal/eats", "-count=1", "-v", "-timeout=2m"}},
	}
	for _, step := range commands {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		command := exec.CommandContext(ctx, step.name, step.args...)
		command.Dir = filepath.Join(root, step.dir)
		command.Env = testkit.IntegrationGoProcessEnv(t, env)
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", step.name, step.args, err, output)
		}
		t.Logf("%s %v\n%s", step.name, step.args, output)
	}
	if err := filepath.WalkDir(filepath.Join(root, "cmd/app/frontend/dist"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(filepath.Join(root, "cmd/app/frontend/dist"), path)
		if err != nil {
			return err
		}
		built, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		committed, err := templatesFS.ReadFile("gopher_eats/frontend/dist/" + filepath.ToSlash(relative))
		if errors.Is(err, fs.ErrNotExist) {
			committed, err = templatesFS.ReadFile("starter-kits/vue/frontend/public/" + filepath.ToSlash(relative))
		}
		if err != nil {
			return err
		}
		if !bytes.Equal(built, committed) {
			t.Errorf("Gopher Eats asset differs from its source build: %s", relative)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

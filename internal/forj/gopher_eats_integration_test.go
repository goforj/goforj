//go:build integration_eats

package forj

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/envfile"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestRenderedGopherEats exercises the selectable Vue example as three real generated app binaries.
func TestRenderedGopherEats(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "gopher-eats-acceptance-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	env := map[string]string{"GOWORK": "off", "TMPDIR": "/tmp"}
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{ProjectName: "Gopher Eats", GoModuleName: "example.com/gophereats", Render: project.RenderConfig{Example: "gopher_eats", Components: project.Components{CLI: true}, AgentGuidance: project.AgentGuidanceNone}}, EnvOverrides: env})
	env["EATS_TEST_BROKER_URL"] = startGopherEatsComposeNATS(t, root)
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

// startGopherEatsComposeNATS exercises the generated local broker instead of concealing missing setup with an external fixture.
func startGopherEatsComposeNATS(t *testing.T, root string) string {
	t.Helper()
	for _, filename := range []string{".env", ".env.host"} {
		body, err := os.ReadFile(filepath.Join(root, filename))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(body), "\n")
		if filename == ".env" {
			profiles, _ := envfile.Lookup(lines, "COMPOSE_PROFILES")
			if !exactCSVToken(profiles, "nats") {
				t.Fatal("generated demo did not activate the NATS Compose profile")
			}
		}
		host := "nats"
		if filename == ".env.host" {
			host = "localhost"
		}
		for _, key := range []string{"QUEUE_URL", "EVENTS_URL"} {
			value, _ := envfile.Lookup(lines, key)
			if value != "nats://goforj:goforj@"+host+":4222" {
				t.Fatalf("%s %s does not point to the generated authenticated broker", filename, key)
			}
		}
	}
	overrides := map[string]string{}
	for _, key := range []string{"NATS_CLIENT_PORT", "NATS_MONITORING_PORT"} {
		port, err := testkit.AllocateRenderedComposePort()
		if err != nil {
			t.Fatal(err)
		}
		overrides[key] = strconv.Itoa(port)
	}
	projectName := filepath.Base(root)
	run := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "docker", append([]string{"compose", "-p", projectName}, args...)...)
		command.Dir = root
		command.Env = testkit.IntegrationGoProcessEnv(t, overrides)
		return command.CombinedOutput()
	}
	t.Cleanup(func() {
		if output, err := run("--profile", "*", "down", "--volumes", "--remove-orphans"); err != nil {
			t.Errorf("clean up demo broker: %v\n%s", err, output)
		}
	})
	if output, err := run("up", "-d", "--wait", "--wait-timeout", "60", "nats"); err != nil {
		t.Fatalf("start generated NATS service: %v\n%s", err, output)
	}
	return fmt.Sprintf("nats://goforj:goforj@127.0.0.1:%s", overrides["NATS_CLIENT_PORT"])
}

package forj

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
)

// TestTestOpenAPICmdRunValidatesGeneratesAndCompilesClient verifies the complete stage order while replacing only Docker-backed work.
func TestTestOpenAPICmdRunValidatesGeneratesAndCompilesClient(t *testing.T) {
	cmd := NewTestOpenAPICmd(logger.NewSilentLogger())
	defaultCompileClient := cmd.compileClientFn

	var specPath string
	var clientDir string
	steps := make([]string, 0, 3)
	cmd.validateSpecFn = func(image, path string, _ bool) error {
		steps = append(steps, "validate")
		if image != openAPIToolsImage {
			t.Fatalf("validator image = %q, want %q", image, openAPIToolsImage)
		}
		specPath = path
		if filepath.Base(path) != "openapi.json" || filepath.Base(filepath.Dir(path)) != "build" {
			t.Fatalf("unexpected generated spec path %q", path)
		}
		relative, err := filepath.Rel("/tmp", path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			t.Fatalf("generated spec must remain under /tmp, got %q", path)
		}

		document, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		contents := string(document)
		if !strings.Contains(contents, `"openapi": "3.`) {
			t.Fatalf("expected OpenAPI version field, got: %s", contents)
		}
		if !strings.Contains(contents, `"/items/{id}"`) {
			t.Fatalf("expected app-scoped fixture route, got: %s", contents)
		}

		workspace := filepath.Dir(filepath.Dir(path))
		controller, err := os.ReadFile(filepath.Join(workspace, "internal", "hello", "controller.go"))
		if err != nil {
			return err
		}
		if !strings.Contains(string(controller), "ctx web.Context") || !strings.Contains(string(controller), `ctx.Query("full")`) {
			t.Fatalf("fixture does not use native web.Context accessors: %s", controller)
		}
		if strings.Contains(string(controller), "echo.") {
			t.Fatalf("fixture retained legacy Echo context: %s", controller)
		}
		if _, err := os.Stat(filepath.Join(workspace, "app", "routes.go")); err != nil {
			return err
		}
		return nil
	}
	cmd.generateClientFn = func(image, path, destination string, _ bool) error {
		steps = append(steps, "generate")
		if image != openAPIToolsImage {
			t.Fatalf("generator image = %q, want %q", image, openAPIToolsImage)
		}
		if path != specPath {
			t.Fatalf("generator spec path = %q, want validator path %q", path, specPath)
		}
		clientDir = destination
		if destination != filepath.Join(filepath.Dir(filepath.Dir(specPath)), "client") {
			t.Fatalf("unexpected client destination %q", destination)
		}
		return writeGeneratedClientModule(destination, `package openapiclient

// Healthy reports whether the generated-client compile fixture is usable.
func Healthy() bool { return true }
`, `package openapiclient

import "testing"

// TestHealthy keeps the generated-client smoke test observable to go test.
func TestHealthy(t *testing.T) {
	if !Healthy() {
		t.Fatal("expected healthy client")
	}
}
`)
	}
	cmd.compileClientFn = func(destination string, silent bool) error {
		steps = append(steps, "compile")
		if destination != clientDir {
			t.Fatalf("tested client directory = %q, want %q", destination, clientDir)
		}
		return defaultCompileClient(destination, silent)
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if want := []string{"validate", "generate", "compile"}; !reflect.DeepEqual(steps, want) {
		t.Fatalf("stage order = %#v, want %#v", steps, want)
	}
}

// TestTestOpenAPICmdRunPropagatesStageFailures verifies that each failed stage stops every dependent stage.
func TestTestOpenAPICmdRunPropagatesStageFailures(t *testing.T) {
	stageError := errors.New("injected stage failure")
	testCases := []struct {
		name      string
		failStage string
		wantSteps []string
	}{
		{name: "validation", failStage: "validate", wantSteps: []string{"validate"}},
		{name: "generation", failStage: "generate", wantSteps: []string{"validate", "generate"}},
		{name: "client compile", failStage: "compile", wantSteps: []string{"validate", "generate", "compile"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cmd := NewTestOpenAPICmd(logger.NewSilentLogger())
			steps := make([]string, 0, 3)
			cmd.validateSpecFn = func(_ string, _ string, _ bool) error {
				steps = append(steps, "validate")
				if testCase.failStage == "validate" {
					return stageError
				}
				return nil
			}
			cmd.generateClientFn = func(_ string, _ string, _ string, _ bool) error {
				steps = append(steps, "generate")
				if testCase.failStage == "generate" {
					return stageError
				}
				return nil
			}
			cmd.compileClientFn = func(_ string, _ bool) error {
				steps = append(steps, "compile")
				if testCase.failStage == "compile" {
					return stageError
				}
				return nil
			}

			err := cmd.Run()
			if !errors.Is(err, stageError) {
				t.Fatalf("run error = %v, want injected failure", err)
			}
			if !reflect.DeepEqual(steps, testCase.wantSteps) {
				t.Fatalf("stage order = %#v, want %#v", steps, testCase.wantSteps)
			}
		})
	}
}

// TestTestOpenAPICmdRunPropagatesClientCompileFailure verifies that a real package compile failure escapes the workflow unchanged by later work.
func TestTestOpenAPICmdRunPropagatesClientCompileFailure(t *testing.T) {
	cmd := NewTestOpenAPICmd(logger.NewSilentLogger())
	cmd.validateSpecFn = func(_ string, _ string, _ bool) error {
		return nil
	}
	cmd.generateClientFn = func(_ string, _ string, destination string, _ bool) error {
		return writeGeneratedClientModule(destination, "package openapiclient\n\n// Broken remains incomplete so the real compile stage must reject it.\nfunc Broken(\n", "")
	}

	err := cmd.Run()
	if err == nil || !strings.Contains(err.Error(), "generated client compilation failed") {
		t.Fatalf("run error = %v, want generated client compilation failure", err)
	}
}

// writeGeneratedClientModule writes a minimal stand-in for Docker output so the real compile stage remains covered.
func writeGeneratedClientModule(clientDir, clientSource, testSource string) error {
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		"go.mod":    "module example.com/generated-openapi-client\n\ngo 1.25.0\n",
		"client.go": clientSource,
	}
	if testSource != "" {
		files["client_test.go"] = testSource
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(clientDir, name), []byte(contents), 0o644); err != nil {
			return err
		}
	}
	return nil
}

package generate

import (
	"bytes"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/templates"
)

// TestGenerateAsyncManagersUpgradesLegacySources keeps both ordinary generation paths independent of a full render.
func TestGenerateAsyncManagersUpgradesLegacySources(t *testing.T) {
	for _, resource := range []string{"events", "queues"} {
		t.Run(resource, func(t *testing.T) {
			root := mustTempGeneratedModuleRoot(t, "forj-legacy-async-*", filepath.Join("internal", resource))
			required := []string{"github.com/goforj/env/v2", "github.com/goforj/str/v2", "github.com/goforj/events", "github.com/goforj/events/eventscore", "github.com/goforj/queue"}
			writeFixtureGoMod(t, root, fixtureModuleSpec("example.com/legacyasync", required, []string{"go.opentelemetry.io/otel", "go.opentelemetry.io/otel/trace"}, nil))
			generate := generateEventFiles
			environment := map[string]string{"EVENTS_DRIVER": "inproc", "EVENTS_CONTEXT_PROPAGATION_ENABLED": "true", "EVENTS_AUDIT_DRIVER": "null", "EVENTS_AUDIT_CONTEXT_PROPAGATION_ENABLED": "false"}
			owned := map[string][]byte{}
			if resource == "events" {
				for _, name := range []string{"event.go", "bus_transport.go"} {
					source, err := os.ReadFile(filepath.Join("testdata", "legacy_async", name+".txt"))
					if err != nil {
						t.Fatal(err)
					}
					owned[filepath.Join(root, "internal", resource, name)] = source
				}
			} else {
				generate = generateQueueFiles
				environment = map[string]string{"QUEUE_DRIVER": "workerpool", "QUEUE_AUDIT_DRIVER": "null"}
				// Queue runtime collaborators predate the adapter and remain application-owned.
				writeQueueRuntimeFixture(t, root)
				for _, path := range []string{"internal/runtime/source.go", "internal/inspects/manager.go"} {
					source, err := os.ReadFile(filepath.Join(root, path))
					if err != nil {
						t.Fatal(err)
					}
					owned[filepath.Join(root, path)] = source
				}
			}
			for path, source := range owned {
				if err := os.WriteFile(path, source, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			input := fixtureGenerationInput(root, environment)
			if _, err := generate(input); err != nil {
				t.Fatal(err)
			}
			if changed, err := generate(input); err != nil || changed != 0 {
				t.Fatalf("second generation changed=%d, error=%v", changed, err)
			}
			for path, source := range owned {
				actual, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(actual, source) {
					t.Fatalf("application-owned source changed: %s, error=%v", path, err)
				}
			}
			runFixtureGoModTidy(t, root, nil)
			runFixtureGoTest(t, root, "./internal/"+resource, "TestDoesNotExist", nil)
		})
	}
}

// TestGenerateMessageContextPropagatorMatchesRender keeps full renders and both manager generators on one canonical source.
func TestGenerateMessageContextPropagatorMatchesRender(t *testing.T) {
	root := t.TempDir()
	source, err := templates.FS.ReadFile("internal/inspects/propagation_gen.go.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	want, err := format.Source(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "internal", "inspects", "propagation_gen.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := generateMessageContextPropagator(root); err != nil || changed {
		t.Fatalf("regenerating rendered adapter changed=%v, error=%v", changed, err)
	}
}

// TestGenerateMessageContextPropagatorWriteFailure reports a blocked generated directory without replacing its contents.
func TestGenerateMessageContextPropagatorWriteFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal")
	if err := os.WriteFile(path, []byte("owned"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := generateMessageContextPropagator(root); err == nil || changed {
		t.Fatalf("blocked adapter write changed=%v, error=%v", changed, err)
	}
}

// TestGenerateEventFilesRequiresModule prevents unresolved imports when the target is not a Go module.
func TestGenerateEventFilesRequiresModule(t *testing.T) {
	root := t.TempDir()
	if _, err := generateEventFiles(fixtureGenerationInput(root, nil)); err == nil || !strings.Contains(err.Error(), "read go.mod") {
		t.Fatalf("missing module error=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "internal")); !os.IsNotExist(err) {
		t.Fatalf("invalid target wrote generated files: %v", err)
	}
}

// TestGenerateAsyncManagersParsesModuleDeclarations preserves valid Go module syntax in generated local imports.
func TestGenerateAsyncManagersParsesModuleDeclarations(t *testing.T) {
	for _, declaration := range []string{`module "example.com/quoted"`, "module example.com/quoted // application module"} {
		for _, resource := range []string{"events", "queues"} {
			t.Run(resource+"/"+declaration, func(t *testing.T) {
				root := t.TempDir()
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(declaration+"\n\ngo 1.24\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				generator := generateEventFiles
				if resource == "queues" {
					generator = generateQueueFiles
				}
				if _, err := generator(fixtureGenerationInput(root, nil)); err != nil {
					t.Fatal(err)
				}
				source, err := os.ReadFile(filepath.Join(root, "internal", resource, "manager_gen.go"))
				if err != nil || !strings.Contains(string(source), `"example.com/quoted/internal/inspects"`) {
					t.Fatalf("generated local import is incorrect: %v", err)
				}
			})
		}
	}
}

// TestReadModuleNameRequiresDirective rejects manifests without a usable local import prefix.
func TestReadModuleNameRequiresDirective(t *testing.T) {
	for _, source := range []string{"go 1.24\n", "module \"\"\n"} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readModuleName(root); err == nil || !strings.Contains(err.Error(), "module name not found") {
			t.Fatalf("missing module directive error=%v", err)
		}
	}
}

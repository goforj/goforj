package generate

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/coredeps"
	"golang.org/x/mod/modfile"
)

// writeModuleRequirementsFixture keeps generation pin tests independent from actual dependency resolution.
func writeModuleRequirementsFixture(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// readModuleRequirementsFixture returns semantic manifest state so formatting cannot conceal a changed user constraint.
func readModuleRequirementsFixture(t *testing.T, root string) *modfile.File {
	t.Helper()
	path := filepath.Join(root, "go.mod")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := modfile.Parse(path, content, nil)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

// moduleRequirementsFixtureVersions indexes requirements without relying on module formatting or indirect annotations.
func moduleRequirementsFixtureVersions(manifest *modfile.File) map[string]string {
	versions := make(map[string]string)
	for _, requirement := range manifest.Require {
		versions[requirement.Mod.Path] = requirement.Mod.Version
	}
	return versions
}

// TestGenerationPinsBeforeTidy covers command and direct entrypoints before any resolver can choose a newer optional transport.
func TestGenerationPinsBeforeTidy(t *testing.T) {
	for _, command := range []bool{false, true} {
		name := "project"
		if command {
			name = "command"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeModuleRequirementsFixture(t, root, map[string]string{
				"go.mod":      "module example.com/pins\n\ngo 1.24\n\nrequire (\n github.com/goforj/queue v0.2.1\n github.com/goforj/events v0.2.1\n)\n",
				".goforj.yml": "project_name: Pins\nmodule_name: example.com/pins\nrender:\n  components: [cli, jobs, events]\n",
				".env":        "QUEUE_DRIVER=nats\nQUEUE_SUPPORTED_DRIVERS=sync,nats\nEVENTS_DRIVER=nats\nEVENTS_SUPPORTED_DRIVERS=inproc,nats\n",
			})
			called := 0
			tidy := func(string) error {
				called++
				manifest := readModuleRequirementsFixture(t, root)
				versions := moduleRequirementsFixtureVersions(manifest)
				for _, module := range []string{"github.com/goforj/queue", "github.com/goforj/queue/driver/natsqueue", "github.com/goforj/events", "github.com/goforj/events/eventscore", "github.com/goforj/events/driver/natsevents"} {
					if versions[module] != coredeps.MustVersionFor(module) {
						t.Errorf("tidy saw %s@%s want validated %s", module, versions[module], coredeps.MustVersionFor(module))
					}
				}
				for _, module := range []string{"github.com/goforj/cache", "github.com/goforj/storage", "github.com/goforj/mail", "github.com/goforj/queue/driver/redisqueue"} {
					if versions[module] != "" {
						t.Errorf("unimported feature dependency added: %s", module)
					}
				}
				if manifest.Go.Version != "1.24" {
					t.Errorf("Go version changed to %s", manifest.Go.Version)
				}
				return nil
			}
			if command {
				restore := useGenerationTestRoot(t, root)
				defer restore()
				for range 2 {
					if err := (&Cmd{Queue: true, Events: true}).run(tidy); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				for index := range 2 {
					result, err := generateProjectFiles(root, GenerationSelection{Queue: true, Events: true}, tidy)
					if err != nil {
						t.Fatal(err)
					}
					if result.TotalFiles != 5 || (index == 1 && result.ChangedFiles != 0) {
						t.Fatalf("generation %d result=%+v", index, result)
					}
				}
			}
			want := 1
			// Explicit generate commands retain their dependency refresh; build-time generation keeps its unchanged-artifact fast path.
			if command {
				want = 2
			}
			if called != want {
				t.Fatalf("tidy calls=%d want=%d across stable regeneration", called, want)
			}
		})
	}
}

// TestGeneratedRequirementsPreserveConsumerConstraints keeps newer requirements and active replacements in control of user resolution.
func TestGeneratedRequirementsPreserveConsumerConstraints(t *testing.T) {
	for _, replacement := range []string{"", "replace github.com/goforj/queue => ../queue\n", "replace github.com/goforj/queue v0.1.0 => ../queue\n"} {
		t.Run(strings.TrimSpace(replacement), func(t *testing.T) {
			root := t.TempDir()
			queueVersion := "v0.3.0"
			if replacement != "" {
				queueVersion = "v0.1.0"
			}
			writeModuleRequirementsFixture(t, root, map[string]string{
				"go.mod":                         "module example.com/pins\n\ngo 1.24.6\n\nrequire (\n github.com/goforj/queue " + queueVersion + "\n example.com/user v1.2.3 // indirect\n)\n\n" + replacement + "replace example.com/user => ../custom\nreplace github.com/goforj/queue/driver/natsqueue => ../natsqueue\n",
				"internal/queues/manager_gen.go": "package queues\nimport (\n _ \"github.com/goforj/queue/queueconfig\"\n _ \"github.com/goforj/queue/driver/natsqueue\"\n _ \"github.com/goforj/queue/driver/redisqueue\"\n)\n",
			})
			before := readModuleRequirementsFixture(t, root)
			if err := os.Chmod(filepath.Join(root, "go.mod"), 0o600); err != nil {
				t.Fatal(err)
			}
			changed, err := syncGeneratedModuleRequirements(root, GenerationSelection{Queue: true})
			if err != nil || !changed {
				t.Fatalf("changed=%v error=%v", changed, err)
			}
			after := readModuleRequirementsFixture(t, root)
			versions := moduleRequirementsFixtureVersions(after)
			if versions["github.com/goforj/queue"] != queueVersion || versions["example.com/user"] != "v1.2.3" || after.Go.Version != "1.24.6" {
				t.Fatalf("consumer requirements changed: versions=%v go=%s", versions, after.Go.Version)
			}
			if versions["github.com/goforj/queue/driver/natsqueue"] != "" || versions["github.com/goforj/queue/driver/redisqueue"] != coredeps.MustVersionFor("github.com/goforj/queue/driver/redisqueue") {
				t.Fatalf("replacement or catalog default changed: versions=%v", versions)
			}
			for index, replace := range before.Replace {
				if replace.Old != after.Replace[index].Old || replace.New != after.Replace[index].New {
					t.Fatal("consumer replacement changed")
				}
			}
			info, err := os.Stat(filepath.Join(root, "go.mod"))
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("manifest permissions changed: %v %v", info, err)
			}
			if changed, err := syncGeneratedModuleRequirements(root, GenerationSelection{Queue: true}); err != nil || changed {
				t.Fatalf("repeat changed=%v error=%v", changed, err)
			}
		})
	}
}

// TestGeneratedRequirementsPreserveMissingRequirementReplacements compares seeded resolution with Go's own replacement selection.
func TestGeneratedRequirementsPreserveMissingRequirementReplacements(t *testing.T) {
	const module = "github.com/goforj/events/driver/natsevents"
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goBinary += ".exe"
	}
	for _, scenario := range []struct {
		name       string
		directives string
		version    string
		directory  string
		excluded   bool
	}{
		{name: "versioned", directives: "replace " + module + " v0.2.1 => ./low\n", version: "v0.2.1", directory: "low"},
		{name: "highest version", directives: "replace " + module + " v0.2.2 => ./high\nreplace " + module + " v0.2.1 => ./low\n", version: "v0.2.2", directory: "high"},
		{name: "wildcard", directives: "replace " + module + " => ./low\n", version: "v0.0.0-00010101000000-000000000000", directory: "low"},
		{name: "versioned and wildcard", directives: "replace " + module + " => ./low\nreplace " + module + " v0.2.2 => ./high\n", version: "v0.2.2", directory: "high"},
		{name: "excluded lower version", directives: "replace " + module + " v0.2.1 => ./low\nreplace " + module + " v0.2.2 => ./high\nexclude " + module + " v0.2.1\n", version: "v0.2.2", directory: "high"},
		{name: "excluded highest version", directives: "replace " + module + " v0.2.1 => ./low\nreplace " + module + " v0.2.2 => ./high\nexclude " + module + " v0.2.2\n", excluded: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var baseline struct {
				tidyFailed bool
				version    string
				listFailed bool
				listOutput string
			}
			for _, seeded := range []bool{false, true} {
				root := t.TempDir()
				root, err := filepath.EvalSymlinks(root)
				if err != nil {
					t.Fatal(err)
				}
				writeModuleRequirementsFixture(t, root, map[string]string{
					"go.mod":                         "module example.com/replacements\n\ngo 1.24.6\n\n" + scenario.directives,
					"internal/events/manager_gen.go": "package events\nimport _ \"" + module + "\"\n",
					"low/go.mod":                     "module " + module + "\n\ngo 1.24.6\n",
					"low/driver.go":                  "package natsevents\n",
					"high/go.mod":                    "module " + module + "\n\ngo 1.24.6\n",
					"high/driver.go":                 "package natsevents\n",
				})
				if seeded {
					if changed, err := syncGeneratedModuleRequirements(root, GenerationSelection{Events: true}); changed || err != nil {
						t.Fatalf("replacement should resolve through tidy: changed=%v error=%v", changed, err)
					}
				}
				tidy := exec.Command(goBinary, "mod", "tidy")
				tidy.Dir = root
				tidy.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOWORK=off", "GOTOOLCHAIN=local")
				tidyOutput, tidyErr := tidy.CombinedOutput()
				if tidyErr != nil && !scenario.excluded {
					t.Fatalf("seeded=%v tidy: %v\n%s", seeded, tidyErr, tidyOutput)
				}
				manifest := readModuleRequirementsFixture(t, root)
				if got := moduleRequirementsFixtureVersions(manifest)[module]; !scenario.excluded && (got != scenario.version || manifest.Go.Version != "1.24.6") {
					t.Fatalf("seeded=%v version=%s want=%s go=%s", seeded, got, scenario.version, manifest.Go.Version)
				}
				list := exec.Command(goBinary, "list", "-m", "-json", module)
				list.Dir = root
				list.Env = tidy.Env
				output, err := list.CombinedOutput()
				if scenario.excluded {
					encodedRoot, marshalErr := json.Marshal(root)
					if marshalErr != nil {
						t.Fatal(marshalErr)
					}
					observed := baseline
					observed.tidyFailed = tidyErr != nil
					observed.version = moduleRequirementsFixtureVersions(manifest)[module]
					observed.listFailed = err != nil
					observed.listOutput = strings.NewReplacer(string(encodedRoot[1:len(encodedRoot)-1]), "<fixture>", root, "<fixture>").Replace(string(output))
					if !seeded {
						baseline = observed
					} else if observed != baseline {
						t.Fatalf("seeded replacement differs from native resolution: got=%+v baseline=%+v", observed, baseline)
					}
					continue
				}
				if err != nil {
					t.Fatalf("seeded=%v list: %v\n%s", seeded, err, output)
				}
				var resolved struct {
					Version string
					Replace *struct{ Dir string }
				}
				if err := json.Unmarshal(output, &resolved); err != nil {
					t.Fatal(err)
				}
				wantDir, err := filepath.EvalSymlinks(filepath.Join(root, scenario.directory))
				if err != nil {
					t.Fatal(err)
				}
				if resolved.Version != scenario.version || resolved.Replace == nil || resolved.Replace.Dir != wantDir {
					t.Fatalf("seeded=%v resolved=%+v replacement=%+v", seeded, resolved, resolved.Replace)
				}
			}
		})
	}
}

// TestGeneratedRequirementsSelectionAndMissingManifest keeps disabled features and artifact-only fixtures outside dependency mutation.
func TestGeneratedRequirementsSelectionAndMissingManifest(t *testing.T) {
	root := t.TempDir()
	writeModuleRequirementsFixture(t, root, map[string]string{
		"go.mod":                         "module example.com/pins\n\ngo 1.24\n",
		"internal/events/manager_gen.go": "package events\nimport _ \"github.com/goforj/events\"\n",
	})
	before, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range []GenerationSelection{{}, {Queue: true}, {Observability: true}} {
		if changed, err := syncGeneratedModuleRequirements(root, selection); err != nil || changed {
			t.Fatalf("disabled selection=%+v changed=%v error=%v", selection, changed, err)
		}
	}
	after, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("disabled selection rewrote the manifest")
	}
	artifactRoot := t.TempDir()
	writeModuleRequirementsFixture(t, artifactRoot, map[string]string{".env": "EVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc\n"})
	if changed, err := syncGeneratedModuleRequirements(artifactRoot, GenerationSelection{Events: true}); err != nil || changed {
		t.Fatalf("missing manifest changed=%v error=%v", changed, err)
	}
	if _, err := os.Stat(filepath.Join(artifactRoot, "go.mod")); !os.IsNotExist(err) {
		t.Fatalf("artifact-only generation created manifest: %v", err)
	}
}

// TestGeneratedRequirementFailures prevents manifest/import errors from reaching dependency resolution.
func TestGeneratedRequirementFailures(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{name: "manifest read", files: map[string]string{"go.mod/child": ""}, want: "read generated module requirements"},
		{name: "manifest module missing", files: map[string]string{"go.mod": "go 1.24\n"}, want: "module directive is missing"},
		{name: "manifest parse", files: map[string]string{"go.mod": "module example.com/pins\ninvalid directive\n"}, want: "parse generated module requirements"},
		{name: "imports read", files: map[string]string{"go.mod": "module example.com/pins\n", "internal/queues/manager_gen.go/child": ""}, want: "read generated imports"},
		{name: "imports parse", files: map[string]string{"go.mod": "module example.com/pins\n", "internal/queues/manager_gen.go": "package queues\nimport (\n"}, want: "parse generated imports"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			writeModuleRequirementsFixture(t, root, scenario.files)
			if changed, err := syncGeneratedModuleRequirements(root, GenerationSelection{Queue: true}); err == nil || changed || !strings.Contains(err.Error(), scenario.want) {
				t.Fatalf("changed=%v error=%v want=%s", changed, err, scenario.want)
			}
		})
	}
}

// TestGenerationPinFailuresSkipTidy retains the resolver boundary on malformed manifests and reports tidy failures after successful pinning.
func TestGenerationPinFailuresSkipTidy(t *testing.T) {
	for _, command := range []bool{false, true} {
		for _, malformed := range []bool{false, true} {
			root := t.TempDir()
			manifest := "module example.com/pins\n\ngo 1.24\n"
			if malformed {
				manifest += "invalid directive\n"
			}
			writeModuleRequirementsFixture(t, root, map[string]string{
				"go.mod":      manifest,
				".goforj.yml": "project_name: Pins\nmodule_name: example.com/pins\nrender:\n  components: [cli, events]\n",
				".env":        "EVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc\n",
			})
			failure := errors.New("tidy failed")
			called := false
			tidy := func(string) error { called = true; return failure }
			var err error
			if command {
				restore := useGenerationTestRoot(t, root)
				err = (&Cmd{Events: true}).run(tidy)
				restore()
			} else {
				_, err = generateProjectFiles(root, GenerationSelection{Events: true}, tidy)
			}
			if malformed {
				if err == nil || called || !strings.Contains(err.Error(), "parse generated module requirements") {
					t.Fatalf("malformed=%v command=%v called=%v error=%v", malformed, command, called, err)
				}
			} else if !errors.Is(err, failure) || !called {
				t.Fatalf("command=%v called=%v error=%v", command, called, err)
			}
		}
	}
}

// TestGeneratedRequirementsResolveLongestModulePrefix distinguishes nested driver modules from their root API dependency.
func TestGeneratedRequirementsResolveLongestModulePrefix(t *testing.T) {
	root := t.TempDir()
	writeModuleRequirementsFixture(t, root, map[string]string{
		"internal/queues/manager_gen.go": "package queues\nimport (\n _ \"context\"\n _ \"example.com/user\"\n _ \"github.com/goforj/queue/queueconfig\"\n _ \"github.com/goforj/queue/driver/natsqueue\"\n)\n",
	})
	got, err := generatedModuleRequirements(root, GenerationSelection{Queue: true})
	want := []string{"github.com/goforj/queue", "github.com/goforj/queue/driver/natsqueue"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("requirements=%v want=%v error=%v", got, want, err)
	}
}

// TestGeneratedRequirementsReportWriteFailure keeps filesystem failures visible before dependency resolution.
func TestGeneratedRequirementsReportWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write permission-restricted fixture files")
	}
	root := t.TempDir()
	writeModuleRequirementsFixture(t, root, map[string]string{
		"go.mod":                         "module example.com/pins\n\ngo 1.24\n",
		"internal/queues/manager_gen.go": "package queues\nimport _ \"github.com/goforj/queue\"\n",
	})
	path := filepath.Join(root, "go.mod")
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if changed, err := syncGeneratedModuleRequirements(root, GenerationSelection{Queue: true}); changed || err == nil || !strings.Contains(err.Error(), "write generated module requirements") {
		t.Fatalf("changed=%v error=%v", changed, err)
	}
}

// TestGeneratedRequirementsIncludeSharedPropagation pins the adapter's direct SDK API imports only for selected async resources.
func TestGeneratedRequirementsIncludeSharedPropagation(t *testing.T) {
	for _, selection := range []GenerationSelection{{Queue: true}, {Events: true}, {Queue: true, Events: true}, {}} {
		root := t.TempDir()
		writeModuleRequirementsFixture(t, root, map[string]string{
			"go.mod": "module example.com/adapterpins\n\ngo 1.26.0\n",
			"internal/inspects/propagation_gen.go": `package inspects
import (
 "go.opentelemetry.io/otel/propagation"
 "go.opentelemetry.io/otel/trace"
)
var _ propagation.TextMapCarrier
var _ trace.TracerProvider
`,
		})
		changed, err := syncGeneratedModuleRequirements(root, selection)
		want := selection.Queue || selection.Events
		if err != nil || changed != want {
			t.Fatalf("selection=%+v changed=%v want=%v error=%v", selection, changed, want, err)
		}
		versions := moduleRequirementsFixtureVersions(readModuleRequirementsFixture(t, root))
		for _, module := range []string{"go.opentelemetry.io/otel", "go.opentelemetry.io/otel/trace"} {
			if want && versions[module] != coredeps.MustVersionFor(module) || !want && versions[module] != "" {
				t.Fatalf("selection=%+v %s=%q", selection, module, versions[module])
			}
		}
		if changed, err := syncGeneratedModuleRequirements(root, selection); err != nil || changed {
			t.Fatalf("stable repeat selection=%+v changed=%v error=%v", selection, changed, err)
		}
	}
}

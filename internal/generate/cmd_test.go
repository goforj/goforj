package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateProjectFilesUsesPluralServicePackageDirs(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	for _, dir := range []string{
		filepath.Join(projectDir, "internal", "caches"),
		filepath.Join(projectDir, "internal", "mail"),
		filepath.Join(projectDir, "internal", "queues"),
		filepath.Join(projectDir, "internal", "runtime"),
		filepath.Join(projectDir, "internal", "storages"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	writeQueueRuntimeFixture(t, projectDir)
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(strings.Join([]string{
		"CACHE_DRIVER=memory",
		"CACHE_SUPPORTED_DRIVERS=memory",
		"MAIL_DRIVER=log",
		"MAIL_SUPPORTED_DRIVERS=log",
		"QUEUE_DRIVER=null",
		"QUEUE_SUPPORTED_DRIVERS=null",
		"STORAGE_DRIVER=local",
		"STORAGE_SUPPORTED_DRIVERS=local",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error { return nil }
	defer func() { goModTidyRunner = orig }()

	total, changed, err := GenerateProjectFiles(projectDir, true, true, true, false, false, false)
	if err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	if total != 8 {
		t.Fatalf("total files = %d, want %d", total, 8)
	}
	if changed == 0 {
		t.Fatal("expected generated files to be written")
	}

	for _, path := range []string{
		filepath.Join(projectDir, "internal", "caches", "manager_gen.go"),
		filepath.Join(projectDir, "internal", "mail", "manager_gen.go"),
		filepath.Join(projectDir, "internal", "queues", "manager_gen.go"),
		filepath.Join(projectDir, "internal", "storages", "manager_gen.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}

func TestGenerateProjectFilesRunsGoModTidyWhenDBGenerationRuns(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "internal", "database"), 0o755); err != nil {
		t.Fatalf("mkdir database dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte("DB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\n"), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	called := 0
	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error {
		called++
		if dir != projectDir {
			t.Fatalf("goModTidyRunner dir = %q, want %q", dir, projectDir)
		}
		return nil
	}
	defer func() { goModTidyRunner = orig }()

	total, changed, err := GenerateProjectFiles(projectDir, false, false, false, false, true, false)
	if err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	if total != 1 {
		t.Fatalf("total files = %d, want %d", total, 1)
	}
	if changed == 0 {
		t.Fatal("expected generated db file to be written")
	}
	if called != 1 {
		t.Fatalf("goModTidyRunner called %d times, want 1", called)
	}
}

func TestGenerateProjectFilesSkipsGoModTidyWhenNothingChanged(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "internal", "database"), 0o755); err != nil {
		t.Fatalf("mkdir database dir: %v", err)
	}

	t.Setenv("DB_DRIVER", "mysql")
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte("DB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\n"), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	if _, err := GenerateDBFiles(projectDir); err != nil {
		t.Fatalf("seed generated db file: %v", err)
	}

	called := 0
	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error {
		called++
		return nil
	}
	defer func() { goModTidyRunner = orig }()

	total, changed, err := GenerateProjectFiles(projectDir, false, false, false, false, true, false)
	if err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	if total != 1 {
		t.Fatalf("total files = %d, want %d", total, 1)
	}
	if changed != 0 {
		t.Fatalf("changed files = %d, want 0", changed)
	}
	if called != 0 {
		t.Fatalf("goModTidyRunner called %d times, want 0", called)
	}
}

func TestCmdRunRunsGoModTidyWhenDBGenerationRuns(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "database"), 0o755); err != nil {
		t.Fatalf("mkdir database dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\n"), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	called := 0
	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error {
		called++
		if dir != "." {
			t.Fatalf("goModTidyRunner dir = %q, want %q", dir, ".")
		}
		return nil
	}
	defer func() { goModTidyRunner = orig }()

	cmd := &Cmd{DB: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Cmd.Run returned error: %v", err)
	}
	if called != 1 {
		t.Fatalf("goModTidyRunner called %d times, want 1", called)
	}
}

// TestCmdRunSkipsStaleStorageWhenComponentDisabled verifies default generation trusts project intent instead of generated-directory residue.
func TestCmdRunSkipsStaleStorageWhenComponentDisabled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "storages"), 0o755); err != nil {
		t.Fatalf("mkdir stale Storage package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("project_name: Test\nmodule_name: example.test/app\nrender:\n  component_contract: 1\n  components:\n    cli: true\n    storage: false\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("STORAGE_DRIVER=unknown\nSTORAGE_SUPPORTED_DRIVERS=unknown\n"), 0o644); err != nil {
		t.Fatalf("write stale Storage environment: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	if err := (&Cmd{}).Run(); err != nil {
		t.Fatalf("generate project with disabled Storage: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join("internal", "storages", "manager_gen.go")); !os.IsNotExist(statErr) {
		t.Fatalf("disabled Storage generation wrote manager_gen.go: %v", statErr)
	}
}

// TestCmdRunRejectsExplicitStorageWhenComponentDisabled verifies an explicit request cannot override durable component intent.
func TestCmdRunRejectsExplicitStorageWhenComponentDisabled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "storages"), 0o755); err != nil {
		t.Fatalf("mkdir stale Storage package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("project_name: Test\nmodule_name: example.test/app\nrender:\n  component_contract: 1\n  components:\n    cli: true\n    storage: false\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("STORAGE_DRIVER=unknown\nSTORAGE_SUPPORTED_DRIVERS=unknown\n"), 0o644); err != nil {
		t.Fatalf("write stale Storage environment: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	err = (&Cmd{Storage: true}).Run()
	if err == nil || !strings.Contains(err.Error(), "Storage component is disabled") {
		t.Fatalf("explicit disabled Storage error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join("internal", "storages", "manager_gen.go")); !os.IsNotExist(statErr) {
		t.Fatalf("disabled Storage generation wrote manager_gen.go: %v", statErr)
	}
}

// TestCmdRunGeneratesStorageFromEnabledProjectConfig verifies config can authorize generation before the package directory exists.
func TestCmdRunGeneratesStorageFromEnabledProjectConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("project_name: Test\nmodule_name: example.test/app\nrender:\n  component_contract: 1\n  components:\n    cli: true\n    storage: true\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("STORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local\n"), 0o644); err != nil {
		t.Fatalf("write Storage environment: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "storages")); !os.IsNotExist(err) {
		t.Fatalf("Storage package exists before generation: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	originalTidy := goModTidyRunner
	goModTidyRunner = func(string) error { return nil }
	t.Cleanup(func() { goModTidyRunner = originalTidy })
	if err := (&Cmd{}).Run(); err != nil {
		t.Fatalf("generate enabled Storage: %v", err)
	}
	if _, err := os.Stat(filepath.Join("internal", "storages", "manager_gen.go")); err != nil {
		t.Fatalf("enabled Storage manager was not generated: %v", err)
	}
}

// TestCmdRunUsesJobsIntentForQueueGeneration verifies config intent wins over stale directories while legacy projects retain directory fallback.
func TestCmdRunUsesJobsIntentForQueueGeneration(t *testing.T) {
	disabledConfig := "project_name: Test\nmodule_name: example.test/app\nrender:\n  component_contract: 1\n  components:\n    cli: true\n    jobs: false\n"
	enabledConfig := "project_name: Test\nmodule_name: example.test/app\nrender:\n  component_contract: 1\n  components:\n    cli: true\n    jobs: true\n"
	tests := []struct {
		name          string
		config        string
		staleJobs     bool
		staleQueue    bool
		explicitQueue bool
		wantError     string
		wantGenerated bool
	}{
		{name: "disabled config ignores stale directories", config: disabledConfig, staleJobs: true, staleQueue: true},
		{name: "explicit Queue rejects disabled config", config: disabledConfig, staleQueue: true, explicitQueue: true, wantError: "Background Jobs component is disabled"},
		{name: "enabled config creates Queue package", config: enabledConfig, wantGenerated: true},
		{name: "legacy project uses Queue directory", staleQueue: true, wantGenerated: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if test.staleJobs {
				if err := os.MkdirAll(filepath.Join(root, "internal", "jobs"), 0o755); err != nil {
					t.Fatalf("create stale Jobs package: %v", err)
				}
			}
			if test.staleQueue {
				if err := os.MkdirAll(filepath.Join(root, "internal", "queues"), 0o755); err != nil {
					t.Fatalf("create Queue package: %v", err)
				}
			}
			if test.config != "" {
				if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(test.config), 0o644); err != nil {
					t.Fatalf("write project config: %v", err)
				}
			}
			environment := "QUEUE_DRIVER=workerpool\nQUEUE_SUPPORTED_DRIVERS=workerpool\n"
			if !test.wantGenerated {
				environment = "QUEUE_DRIVER=unknown\nQUEUE_SUPPORTED_DRIVERS=unknown\n"
			}
			if err := os.WriteFile(filepath.Join(root, ".env"), []byte(environment), 0o644); err != nil {
				t.Fatalf("write Queue environment: %v", err)
			}
			if test.wantGenerated {
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
					t.Fatalf("write go.mod: %v", err)
				}
			}

			originalWD, err := os.Getwd()
			if err != nil {
				t.Fatalf("get working directory: %v", err)
			}
			if err := os.Chdir(root); err != nil {
				t.Fatalf("change working directory: %v", err)
			}
			originalTidy := goModTidyRunner
			goModTidyRunner = func(string) error { return nil }
			t.Cleanup(func() {
				goModTidyRunner = originalTidy
				_ = os.Chdir(originalWD)
			})

			cmd := &Cmd{Queue: test.explicitQueue}
			err = cmd.Run()
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Queue generation error = %v, want %q", err, test.wantError)
				}
			} else if err != nil {
				t.Fatalf("generate Queue surface: %v", err)
			}
			for _, name := range []string{"manager_gen.go", "accessors_gen.go"} {
				_, statErr := os.Stat(filepath.Join("internal", "queues", name))
				if got := statErr == nil; got != test.wantGenerated {
					t.Fatalf("generated Queue file %s presence = %t, want %t: %v", name, got, test.wantGenerated, statErr)
				}
			}
		})
	}
}

// TestCmdRunRejectsExplicitEventsWhenComponentDisabled verifies stale package directories cannot authorize Events generation.
func TestCmdRunRejectsExplicitEventsWhenComponentDisabled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "events"), 0o755); err != nil {
		t.Fatalf("mkdir stale Events package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("project_name: Test\nmodule_name: example.test/app\nrender:\n  component_contract: 1\n  components:\n    cli: true\n    events: false\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=redis\n"), 0o644); err != nil {
		t.Fatalf("write stale Events environment: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	err = (&Cmd{Events: true}).Run()
	if err == nil || !strings.Contains(err.Error(), "Events component is disabled") {
		t.Fatalf("explicit disabled Events error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join("internal", "events", "manager_gen.go")); !os.IsNotExist(statErr) {
		t.Fatalf("disabled Events generation wrote manager_gen.go: %v", statErr)
	}
}

// TestCmdRunGeneratesEventsFromEnabledProjectConfig verifies config authorizes generation without directory-based selection.
func TestCmdRunGeneratesEventsFromEnabledProjectConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "events"), 0o755); err != nil {
		t.Fatalf("mkdir Events package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("project_name: Test\nmodule_name: example.test/app\nrender:\n  component_contract: 1\n  components:\n    cli: true\n    events: true\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("EVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc\n"), 0o644); err != nil {
		t.Fatalf("write Events environment: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	originalTidy := goModTidyRunner
	goModTidyRunner = func(string) error { return nil }
	t.Cleanup(func() { goModTidyRunner = originalTidy })
	if err := (&Cmd{Events: true}).Run(); err != nil {
		t.Fatalf("generate enabled Events: %v", err)
	}
	if _, err := os.Stat(filepath.Join("internal", "events", "manager_gen.go")); err != nil {
		t.Fatalf("enabled Events manager was not generated: %v", err)
	}
}

func TestCmdRunGeneratesObservabilityTargetsWithoutGoModTidy(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "containers", "observability", "vmagent"), 0o755); err != nil {
		t.Fatalf("mkdir vmagent dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal", "http"), 0o755); err != nil {
		t.Fatalf("mkdir http dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(strings.Join([]string{
		"APP_NAME=Test App",
		"APP_ENV=local",
		"OBSERVABILITY_METRICS_TARGET_HOST=localhost",
		"METRICS_API_PORT=9100",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	called := 0
	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error {
		called++
		return nil
	}
	defer func() { goModTidyRunner = orig }()

	cmd := &Cmd{Observability: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Cmd.Run returned error: %v", err)
	}
	if called != 0 {
		t.Fatalf("goModTidyRunner called %d times, want 0", called)
	}

	content, err := os.ReadFile(filepath.Join(root, "containers", "observability", "vmagent", "metrics-targets.json"))
	if err != nil {
		t.Fatalf("read metrics-targets.json: %v", err)
	}
	if string(content) == "" {
		t.Fatal("expected generated metrics targets content")
	}
}

func TestGenerateProjectFilesSkipsGoModTidyForObservabilityOnlyChanges(t *testing.T) {
	projectDir := t.TempDir()
	for _, dir := range []string{
		filepath.Join(projectDir, "internal", "storages"),
		filepath.Join(projectDir, "containers", "observability", "vmagent"),
		filepath.Join(projectDir, "internal", "http"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	t.Setenv("STORAGE_DRIVER", "local")
	if err := os.WriteFile(filepath.Join(projectDir, ".env"), []byte(strings.Join([]string{
		"STORAGE_DRIVER=local",
		"STORAGE_SUPPORTED_DRIVERS=local",
		"APP_NAME=Test App",
		"OBSERVABILITY_METRICS_TARGET_HOST=host.docker.internal",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	if _, err := GenerateStorageFiles(projectDir); err != nil {
		t.Fatalf("seed generated storage file: %v", err)
	}

	called := 0
	orig := goModTidyRunner
	goModTidyRunner = func(dir string) error {
		called++
		return nil
	}
	defer func() { goModTidyRunner = orig }()

	total, changed, err := GenerateProjectFiles(projectDir, true, false, false, false, false, true)
	if err != nil {
		t.Fatalf("GenerateProjectFiles returned error: %v", err)
	}
	if total != 3 {
		t.Fatalf("total files = %d, want %d", total, 3)
	}
	if changed != 1 {
		t.Fatalf("changed files = %d, want %d", changed, 1)
	}
	if called != 0 {
		t.Fatalf("goModTidyRunner called %d times, want 0", called)
	}
}

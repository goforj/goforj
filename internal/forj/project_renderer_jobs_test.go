package forj

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestValidateJobsRenderTransitionIgnoresNonSourceArtifacts verifies empty generated directories and runtime Queue data do not prove that a project still owns Jobs source.
func TestValidateJobsRenderTransitionIgnoresNonSourceArtifacts(t *testing.T) {
	tests := []struct {
		name        string
		directories []string
		files       map[string]string
	}{
		{
			name: "empty generated directories",
			directories: []string{
				filepath.Join("internal", "jobs", "archive"),
				filepath.Join("internal", "queues", "stale", "nested"),
			},
		},
		{
			name: "runtime Queue data",
			files: map[string]string{
				filepath.Join("_data", "queues", "default.db"): "queued runtime data\n",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useJobsRendererRoot(t)
			for _, path := range test.directories {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatalf("create Jobs artifact directory %s: %v", path, err)
				}
			}
			for path, contents := range test.files {
				writeJobsRendererFile(t, path, contents)
			}

			components := project.Components{CLI: true}
			renderer := &ProjectRenderer{config: jobsRendererConfig(components)}
			if err := renderer.validateJobsRenderTransition(components); err != nil {
				t.Fatalf("non-source Jobs artifact blocked steady-state render: %v", err)
			}
			for _, path := range test.directories {
				if info, err := os.Stat(path); err != nil || !info.IsDir() {
					t.Fatalf("Jobs preflight changed artifact directory %s: info=%v err=%v", path, info, err)
				}
			}
			for path, contents := range test.files {
				if got := readJobsRendererFile(t, path); got != contents {
					t.Fatalf("Jobs preflight changed artifact file %s: %q", path, got)
				}
			}
		})
	}
}

// TestValidateJobsRenderTransitionRejectsMeaningfulProjectResidue verifies Jobs source blocks unsupported removal without being changed.
func TestValidateJobsRenderTransitionRejectsMeaningfulProjectResidue(t *testing.T) {
	for _, residuePath := range projectJobsResiduePaths() {
		residuePath := residuePath
		t.Run(strings.ReplaceAll(residuePath, string(filepath.Separator), "_"), func(t *testing.T) {
			useJobsRendererRoot(t)
			fixturePath := residuePath
			if residuePath == filepath.Join("internal", "jobs") || residuePath == filepath.Join("internal", "queues") {
				fixturePath = filepath.Join(residuePath, "owner.go")
			}
			contents := "fixture\n"
			if filepath.Ext(fixturePath) == ".go" {
				contents = "package fixture\n"
			}
			writeJobsRendererFile(t, fixturePath, contents)

			components := project.Components{CLI: true}
			renderer := &ProjectRenderer{config: jobsRendererConfig(components)}
			err := renderer.validateJobsRenderTransition(components)
			if err == nil || !strings.Contains(err.Error(), residuePath) {
				t.Fatalf("transition error = %v, want residue path %s", err, residuePath)
			}
			if _, statErr := os.Stat(fixturePath); statErr != nil {
				t.Fatalf("Jobs preflight changed residue %s: %v", fixturePath, statErr)
			}
		})
	}
}

// TestEnabledJobsRerenderCleansLegacyFrameworkFiles verifies obsolete framework files do not block an enabled rerender and remain cleanup-owned.
func TestEnabledJobsRerenderCleansLegacyFrameworkFiles(t *testing.T) {
	useJobsRendererRoot(t)
	for _, path := range legacyJobsFrameworkPaths() {
		writeJobsRendererFile(t, path, "package jobs\n")
	}
	customPath := filepath.Join("internal", "jobs", "reports", "custom_job.go")
	customContents := "package reports\n\nvar customJobSentinel = true\n"
	writeJobsRendererFile(t, customPath, customContents)
	writeJobsRendererFile(t, filepath.Join("project", "config.go"), "package project\n")
	writeJobsRendererFile(t, filepath.Join("internal", "lighthouse", "project_config_patch.go"), "package lighthouse\n")

	components := project.Components{CLI: true, Jobs: true}
	config := jobsRendererConfig(components)
	config.GoModuleName = "example.test/legacy-jobs"
	renderer := &ProjectRenderer{config: config}
	if err := renderer.validateJobsRenderTransition(components); err != nil {
		t.Fatalf("legacy framework files blocked enabled Jobs rerender: %v", err)
	}
	if err := renderer.cleanupLegacyGeneratedFiles(); err != nil {
		t.Fatalf("clean legacy generated files: %v", err)
	}
	for _, path := range legacyJobsFrameworkPaths() {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy Jobs framework file %s remains after cleanup: %v", path, err)
		}
	}
	if got := readJobsRendererFile(t, customPath); got != customContents {
		t.Fatalf("custom Jobs source changed during legacy cleanup: %q", got)
	}
}

// TestAppJobsSurfaceExistsUsesAppReceiverSyntax verifies only Queue accessors on an App receiver prove the generated App API exists.
func TestAppJobsSurfaceExistsUsesAppReceiverSyntax(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     bool
	}{
		{
			name: "queue accessor",
			contents: `package wire

type App struct{}

func (a *App) Queue() any { return nil }
`,
			want: true,
		},
		{
			name: "queues accessor",
			contents: `package wire

type App struct{}

func (a App) Queues() any { return nil }
`,
			want: true,
		},
		{
			name: "comments strings fields free functions and other receivers",
			contents: `package wire

// func (a *App) Queue() any is retained as migration documentation only.
const migrationNote = "func (a *App) Queues()"

type App struct {
	Queue string
}

type Worker struct{}

func Queue() any { return nil }
func (w *Worker) Queues() any { return nil }
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "app.go")
			writeJobsRendererFile(t, path, test.contents)
			got, err := appJobsSurfaceExists(path)
			if err != nil {
				t.Fatalf("inspect App Jobs surface: %v", err)
			}
			if got != test.want {
				t.Fatalf("Jobs surface presence = %t, want %t", got, test.want)
			}
		})
	}
}

// TestRenderRejectsJobsRemovalBeforeWrites verifies full-render preflight runs before environment, migration, configuration, or scaffold writes.
func TestRenderRejectsJobsRemovalBeforeWrites(t *testing.T) {
	useJobsRendererRoot(t)
	config := jobsRendererConfig(project.Components{CLI: true})
	config.ProjectName = "Jobs Removal"
	config.GoModuleName = "example.test/jobs-removal"
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	configBefore := readJobsRendererFile(t, ".goforj.yml")
	writeJobsRendererFile(t, ".env", "OWNER_SENTINEL=unchanged\n")
	jobPath := filepath.Join("internal", "jobs", "custom_job.go")
	writeJobsRendererFile(t, jobPath, "package jobs\n\nvar ownerSentinel = true\n")
	legacyOwnerPath := filepath.Join("app", "wire", "inject_controllers_app.go")
	currentOwnerPath := filepath.Join("app", "wire", "inject_http_controllers_app.go")
	legacyOwnerContents := "package wire\n\nvar ownerSentinel = true\n"
	writeJobsRendererFile(t, legacyOwnerPath, legacyOwnerContents)

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	err := renderer.Render(ComponentRenderInput{renderAll: true})
	if err == nil || !strings.Contains(err.Error(), filepath.Join("internal", "jobs")) {
		t.Fatalf("full Jobs removal error = %v, want internal/jobs residue", err)
	}
	if got := readJobsRendererFile(t, ".goforj.yml"); got != configBefore {
		t.Fatalf("failed Jobs preflight rewrote project config\nbefore:\n%s\nafter:\n%s", configBefore, got)
	}
	if got := readJobsRendererFile(t, ".env"); got != "OWNER_SENTINEL=unchanged\n" {
		t.Fatalf("failed Jobs preflight rewrote environment: %q", got)
	}
	if got := readJobsRendererFile(t, legacyOwnerPath); got != legacyOwnerContents {
		t.Fatalf("failed Jobs preflight changed legacy owner %s", legacyOwnerPath)
	}
	if _, statErr := os.Stat(currentOwnerPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed Jobs preflight created migrated owner %s: %v", currentOwnerPath, statErr)
	}
	if _, statErr := os.Stat("go.mod"); !os.IsNotExist(statErr) {
		t.Fatalf("failed Jobs preflight created go.mod: %v", statErr)
	}
}

// TestRenderAppOnlyRejectsJobsRemovalBeforeWrites verifies prospective App participation is checked before config, environment, owner, or migration writes.
func TestRenderAppOnlyRejectsJobsRemovalBeforeWrites(t *testing.T) {
	useJobsRendererRoot(t)
	app := project.DefaultNamedApp("worker")
	config := jobsRendererConfig(project.Components{CLI: true})
	config.ProjectName = "Jobs App Removal"
	config.GoModuleName = "example.test/jobs-app-removal"
	config.Apps = map[string]project.AppConfig{
		app.Name: {Components: project.Components{CLI: true, Jobs: true}},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	configBefore := readJobsRendererFile(t, ".goforj.yml")
	writeJobsRendererFile(t, ".env", "QUEUE_DRIVER=workerpool\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis\n")
	appSurfacePath := filepath.Join(app.WireDir, "app.go")
	writeJobsRendererFile(t, appSurfacePath, `package workerapp

type App struct{}

func (a *App) Queue() any { return nil }
`)
	legacyOwnerPath := filepath.Join("app", "wire", "inject_controllers_app.go")
	currentOwnerPath := filepath.Join("app", "wire", "inject_http_controllers_app.go")
	legacyOwnerContents := "package wire\n\nvar ownerSentinel = true\n"
	writeJobsRendererFile(t, legacyOwnerPath, legacyOwnerContents)

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	err := renderer.RenderAppOnly(app, makeapp.RenderOptions{Components: project.Components{CLI: true}, SkipWire: true})
	if err == nil || !strings.Contains(err.Error(), appSurfacePath) {
		t.Fatalf("App-only Jobs removal error = %v, want %s", err, appSurfacePath)
	}
	if got := readJobsRendererFile(t, ".goforj.yml"); got != configBefore {
		t.Fatalf("failed App-only Jobs preflight rewrote project config\nbefore:\n%s\nafter:\n%s", configBefore, got)
	}
	if got := readJobsRendererFile(t, legacyOwnerPath); got != legacyOwnerContents {
		t.Fatalf("failed App-only Jobs preflight changed legacy owner %s", legacyOwnerPath)
	}
	if _, statErr := os.Stat(currentOwnerPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed App-only Jobs preflight created migrated owner %s: %v", currentOwnerPath, statErr)
	}
	if _, statErr := os.Stat(app.Entrypoint); !os.IsNotExist(statErr) {
		t.Fatalf("failed App-only Jobs preflight rendered App entrypoint %s: %v", app.Entrypoint, statErr)
	}
}

// TestRenderAppOnlyAddsJobsWithoutChangingAppOwnedFiles verifies additive Jobs enablement creates new boundaries while preserving every existing owner file byte-for-byte.
func TestRenderAppOnlyAddsJobsWithoutChangingAppOwnedFiles(t *testing.T) {
	useJobsRendererRoot(t)
	app := project.DefaultNamedApp("api")
	config := jobsRendererConfig(project.Components{CLI: true})
	config.ProjectName = "Additive Jobs"
	config.GoModuleName = "example.test/additive-jobs"
	config.Apps = map[string]project.AppConfig{
		app.Name: {Components: project.Components{CLI: true}},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	initialRenderer := NewProjectRenderer(logger.NewSilentLogger())
	initialRenderer.config = config
	if err := initialRenderer.renderApp(app); err != nil {
		t.Fatalf("render Jobs-disabled App fixture: %v", err)
	}
	ownerPaths := make([]string, 0)
	for _, mapping := range initialRenderer.appOwnedMappings(app) {
		ownerPaths = append(ownerPaths, mapping.dest)
		contents := readJobsRendererFile(t, mapping.dest)
		writeJobsRendererFile(t, mapping.dest, contents+"\n// OwnerSentinel proves additive renders preserve this file.\n")
	}
	wantHashes := jobsRendererFileHashes(t, ownerPaths)

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.RenderAppOnly(app, makeapp.RenderOptions{
		Components: project.Components{CLI: true, Jobs: true},
		SkipWire:   true,
	}); err != nil {
		t.Fatalf("add Jobs to existing App: %v", err)
	}
	gotHashes := jobsRendererFileHashes(t, ownerPaths)
	for _, path := range ownerPaths {
		if gotHashes[path] != wantHashes[path] {
			t.Fatalf("additive Jobs render changed App-owned file %s", path)
		}
	}
	for _, path := range []string{
		filepath.Join(app.WireDir, "inject_jobs.go"),
		filepath.Join(app.WireDir, "inject_jobs_app.go"),
		filepath.Join("internal", "jobs", "runtime.go"),
		filepath.Join("internal", "queues", "manager_gen.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("additive Jobs render omitted %s: %v", path, err)
		}
	}
	if exists, err := appJobsSurfaceExists(filepath.Join(app.WireDir, "app.go")); err != nil {
		t.Fatalf("inspect additive App Jobs surface: %v", err)
	} else if !exists {
		t.Fatal("additive render did not generate App Queue accessors")
	}
	environment := readJobsRendererFile(t, ".env")
	for _, want := range []string{
		"QUEUE_DRIVER=workerpool",
		"QUEUE_SUPPORTED_DRIVERS=workerpool,redis",
		"API_QUEUE_DRIVER=workerpool",
	} {
		if !strings.Contains(environment, want) {
			t.Fatalf("additive Jobs render omitted %q:\n%s", want, environment)
		}
	}
	loaded, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("reload additive Jobs config: %v", err)
	}
	if !loaded.Apps[app.Name].Components.Jobs {
		t.Fatalf("additive render did not persist App Jobs participation: %#v", loaded.Apps[app.Name].Components)
	}
}

// TestRemoveAppRejectsLastJobsOwnerBeforeWrites verifies removing the last Jobs App cannot delete owner source or strand shared Jobs code.
func TestRemoveAppRejectsLastJobsOwnerBeforeWrites(t *testing.T) {
	useJobsRendererRoot(t)
	app := project.DefaultNamedApp("worker")
	config := jobsRendererConfig(project.Components{CLI: true})
	config.ProjectName = "Jobs Owner Removal"
	config.GoModuleName = "example.test/jobs-owner-removal"
	config.Apps = map[string]project.AppConfig{
		app.Name: {Components: project.Components{CLI: true, Jobs: true}},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	configBefore := readJobsRendererFile(t, ".goforj.yml")
	appSentinel := filepath.Join(app.AppDir, "owner.go")
	writeJobsRendererFile(t, appSentinel, "package workerapp\n\nvar ownerSentinel = true\n")
	writeJobsRendererFile(t, app.Entrypoint, "package main\n")
	writeJobsRendererFile(t, ".env", "WORKER_QUEUE_DRIVER=workerpool\n")
	jobPath := filepath.Join("internal", "jobs", "custom_job.go")
	writeJobsRendererFile(t, jobPath, "package jobs\n")

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	result, err := renderer.RemoveApp(app)
	if err == nil || !strings.Contains(err.Error(), "last App using Jobs") || !strings.Contains(err.Error(), filepath.Join("internal", "jobs")) {
		t.Fatalf("remove last Jobs App error = %v", err)
	}
	if result.Changed() {
		t.Fatalf("failed Jobs App removal reported changes: %#v", result)
	}
	for _, path := range []string{appSentinel, app.Entrypoint, ".env", jobPath} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("failed Jobs App removal changed %s: %v", path, statErr)
		}
	}
	if got := readJobsRendererFile(t, ".goforj.yml"); got != configBefore {
		t.Fatalf("failed Jobs App removal rewrote project config\nbefore:\n%s\nafter:\n%s", configBefore, got)
	}
}

// TestRemoveAppRejectsLastJobsAppAccessor verifies syntax-aware App residue protects owner files even when shared source is absent.
func TestRemoveAppRejectsLastJobsAppAccessor(t *testing.T) {
	useJobsRendererRoot(t)
	app := project.DefaultNamedApp("worker")
	config := jobsRendererConfig(project.Components{CLI: true})
	config.Apps = map[string]project.AppConfig{
		app.Name: {Components: project.Components{CLI: true, Jobs: true}},
	}
	appPath := filepath.Join(app.WireDir, "app.go")
	writeJobsRendererFile(t, appPath, `package workerapp

type App struct{}

func (a *App) Queues() any { return nil }
`)
	renderer := &ProjectRenderer{config: config}
	err := renderer.validateRemoveAppTransition(app)
	if err == nil || !strings.Contains(err.Error(), "last App using Jobs") || !strings.Contains(err.Error(), appPath) {
		t.Fatalf("last Jobs App accessor error = %v", err)
	}
}

// TestMigrateAppOwnedWireFilenamesPreservesLegacyJobsOwner verifies the old top-level Jobs injector moves byte-for-byte to the default App owner path.
func TestMigrateAppOwnedWireFilenamesPreservesLegacyJobsOwner(t *testing.T) {
	useJobsRendererRoot(t)
	source := filepath.Join("wire", "inject_jobs_app.go")
	target := filepath.Join(project.DefaultApp().WireDir, "inject_jobs_app.go")
	contents := "package wire\n\n// Custom Jobs providers must survive migration.\nvar ownerSentinel = true\n"
	writeJobsRendererFile(t, source, contents)
	renderer := &ProjectRenderer{config: jobsRendererConfig(project.Components{CLI: true, Jobs: true})}

	if err := renderer.migrateAppOwnedWireFilenames(); err != nil {
		t.Fatalf("migrate legacy Jobs owner: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("legacy Jobs owner still exists after migration: %v", err)
	}
	if got := readJobsRendererFile(t, target); got != contents {
		t.Fatalf("migrated Jobs owner changed\nwant:\n%s\ngot:\n%s", contents, got)
	}
}

// TestMigrateAppOwnedWireFilenamesRejectsJobsOwnerCollisionBeforeMoves verifies Jobs conflicts are discovered before unrelated owner migrations can write.
func TestMigrateAppOwnedWireFilenamesRejectsJobsOwnerCollisionBeforeMoves(t *testing.T) {
	useJobsRendererRoot(t)
	legacyJobs := filepath.Join("wire", "inject_jobs_app.go")
	currentJobs := filepath.Join(project.DefaultApp().WireDir, "inject_jobs_app.go")
	legacyEvents := filepath.Join(project.DefaultApp().WireDir, "inject_event_subscribers.go")
	currentEvents := eventSubscriberOwnerPath(project.DefaultApp())
	writeJobsRendererFile(t, legacyJobs, "package wire\n\nvar legacyJobsOwner = true\n")
	writeJobsRendererFile(t, currentJobs, "package wire\n\nvar currentJobsOwner = true\n")
	writeJobsRendererFile(t, legacyEvents, "package wire\n\nvar legacyEventsOwner = true\n")
	config := jobsRendererConfig(project.Components{CLI: true, Events: true, Jobs: true})
	renderer := &ProjectRenderer{config: config}

	err := renderer.migrateAppOwnedWireFilenames()
	if err == nil || !strings.Contains(err.Error(), legacyJobs) || !strings.Contains(err.Error(), currentJobs) || !strings.Contains(err.Error(), "both exist") {
		t.Fatalf("Jobs owner collision error = %v", err)
	}
	if _, statErr := os.Stat(legacyEvents); statErr != nil {
		t.Fatalf("Jobs collision preflight moved unrelated Events owner: %v", statErr)
	}
	if _, statErr := os.Stat(currentEvents); !os.IsNotExist(statErr) {
		t.Fatalf("Jobs collision preflight created unrelated Events owner: %v", statErr)
	}
}

// TestQueueDriverDefaultFromEnvValidatesOwnerContract prevents incremental Apps from copying an invalid project Queue selection.
func TestQueueDriverDefaultFromEnvValidatesOwnerContract(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		want        string
		wantError   string
	}{
		{name: "missing environment uses workerpool", want: "workerpool"},
		{name: "valid shared driver", environment: "QUEUE_DRIVER=redis\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis\n", want: "redis"},
		{name: "unknown active driver", environment: "QUEUE_DRIVER=unknown\nQUEUE_SUPPORTED_DRIVERS=unknown\n", wantError: "unsupported driver"},
		{name: "active driver excluded", environment: "QUEUE_DRIVER=redis\nQUEUE_SUPPORTED_DRIVERS=workerpool\n", wantError: "excludes active"},
		{name: "fallback driver excluded", environment: "QUEUE_SUPPORTED_DRIVERS=redis\n", wantError: "excludes active"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if test.environment != "" {
				writeJobsRendererFile(t, path, test.environment)
			}
			got, err := queueDriverDefaultFromEnv(path)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Queue driver error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve Queue driver: %v", err)
			}
			if got != test.want {
				t.Fatalf("Queue driver = %q, want %q", got, test.want)
			}
		})
	}
}

// jobsRendererConfig returns an explicit-contract fixture so omitted primitive components remain disabled.
func jobsRendererConfig(components project.Components) *project.Config {
	return &project.Config{
		Render: project.RenderConfig{
			Components:               components,
			ComponentContractVersion: project.CurrentComponentContractVersion,
		},
	}
}

// useJobsRendererRoot changes into an isolated render directory for the duration of one test.
func useJobsRendererRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change to Jobs render directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return root
}

// writeJobsRendererFile writes one isolated fixture with its conventional parent directories.
func writeJobsRendererFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

// readJobsRendererFile reads one ownership-sensitive fixture.
func readJobsRendererFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(contents)
}

// jobsRendererFileHashes captures byte-level identities for App-owned files before an additive render.
func jobsRendererFileHashes(t *testing.T, paths []string) map[string][sha256.Size]byte {
	t.Helper()
	hashes := make(map[string][sha256.Size]byte, len(paths))
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read App-owned file %s: %v", path, err)
		}
		hashes[path] = sha256.Sum256(contents)
	}
	return hashes
}

package forj

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestValidateManagedDevLifecycleAcceptsGeneratedLegacyTasks verifies exact framework tasks are migrated in memory before admission.
func TestValidateManagedDevLifecycleAcceptsGeneratedLegacyTasks(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{Docker: true}},
		Dev: project.DevConfig{
			Pre: []project.DevTask{
				{Name: "Run Docker Compose", Cmd: "docker-compose up -d"},
				{Name: "Waiting for Database to be ready", Cmd: generatedMySQLDevWaitCommand},
				legacyGeneratedDevFrontendInstallTask(project.DefaultApp()),
			},
			Down: []project.DevTask{{Name: "Docker Compose Down", Cmd: "docker-compose down"}},
		},
	}

	if err := validateManagedDevLifecycle(config); err != nil {
		t.Fatalf("validateManagedDevLifecycle() error = %v", err)
	}
	for index, task := range config.Dev.Pre {
		if task.ID != "" || task.Phase != "" {
			t.Fatalf("validation mutated config.Dev.Pre[%d] = %#v", index, task)
		}
	}
	if config.Dev.Down[0].ID != "" || config.Dev.Down[0].Phase != "" {
		t.Fatalf("validation mutated config.Dev.Down = %#v", config.Dev.Down)
	}
}

// TestValidateManagedDevLifecycleRejectsUnphasedCustomTasks verifies ambiguous owner tasks fail before a managed session is opened.
func TestValidateManagedDevLifecycleRejectsUnphasedCustomTasks(t *testing.T) {
	config := &project.Config{Dev: project.DevConfig{
		Pre: []project.DevTask{{Name: "Prepare local data", Cmd: "make seed"}},
	}}

	err := validateManagedDevLifecycle(config)
	if err == nil || !strings.Contains(err.Error(), "no managed lifecycle phase") {
		t.Fatalf("validateManagedDevLifecycle() error = %v, want unphased-task guidance", err)
	}
	if !strings.Contains(err.Error(), "forj render") {
		t.Fatalf("validateManagedDevLifecycle() error = %v, want render guidance", err)
	}
}

// TestValidateManagedDevLifecycleRejectsMalformedMetadata verifies unknown and duplicate IDs remain fail-closed.
func TestValidateManagedDevLifecycleRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		config *project.Config
		want   string
	}{
		{
			name: "unknown phase",
			config: &project.Config{Dev: project.DevConfig{Pre: []project.DevTask{{
				Name: "custom", Cmd: "make custom", ID: "custom.task", Phase: project.DevTaskPhase("later"),
			}}}},
			want: "unknown lifecycle phase",
		},
		{
			name: "duplicate ID",
			config: &project.Config{Dev: project.DevConfig{
				Pre:  []project.DevTask{{Name: "one", Cmd: "one", ID: "same", Phase: project.DevTaskPhasePreCompose}},
				Down: []project.DevTask{{Name: "two", Cmd: "two", ID: "same", Phase: project.DevTaskPhaseComposeDown}},
			}},
			want: "duplicate dev task id",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateManagedDevLifecycle(test.config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateManagedDevLifecycle() error = %v, want %q", err, test.want)
			}
		})
	}
}

// TestPlanManagedDevLifecyclePreservesExplicitPhaseOrder verifies generated legacy tasks are normalized before planning.
func TestPlanManagedDevLifecyclePreservesExplicitPhaseOrder(t *testing.T) {
	config := &project.Config{Dev: project.DevConfig{Pre: []project.DevTask{
		{Name: "Migrate data", Cmd: "make migrate", ID: "migrate", Phase: project.DevTaskPhasePostMigrate},
		{Name: "Run Compose", Cmd: "docker-compose up -d", ID: project.DevTaskIDCompose, Phase: project.DevTaskPhaseCompose},
		{Name: "Prepare", Cmd: "make prepare", ID: "prepare", Phase: project.DevTaskPhasePreCompose},
		{Name: "Wait", Cmd: "make wait", ID: "wait", Phase: project.DevTaskPhasePostCompose},
	}}}

	plan, err := planManagedDevLifecycle(config)
	if err != nil {
		t.Fatalf("planManagedDevLifecycle() error = %v", err)
	}
	if got := plan.preCompose[0].ID; got != "prepare" {
		t.Fatalf("pre-compose task ID = %q, want prepare", got)
	}
	if got := plan.compose[0].ID; got != project.DevTaskIDCompose {
		t.Fatalf("compose task ID = %q, want %q", got, project.DevTaskIDCompose)
	}
	if got := plan.postCompose[0].ID; got != "wait" {
		t.Fatalf("post-compose task ID = %q, want wait", got)
	}
	if got := plan.postMigrate[0].ID; got != "migrate" {
		t.Fatalf("post-migrate task ID = %q, want migrate", got)
	}
}

// TestRunManagedDevInitialLifecycleWaitsAtComposeBarrier verifies post-Compose work cannot run before Harbor acknowledgment.
func TestRunManagedDevInitialLifecycleWaitsAtComposeBarrier(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	frontendRoot := filepath.Join(root, "cmd", "app", "frontend")
	if err := os.MkdirAll(frontendRoot, 0o755); err != nil {
		t.Fatalf("mkdir frontend: %v", err)
	}
	logPath := filepath.Join(root, "managed-startup.log")
	config := devInitialLifecycleTestConfig(logPath)
	config.Dev.Pre = []project.DevTask{
		{Name: "Prepare", Cmd: appendDevLifecycleTestLine(logPath, "pre"), ID: "prepare", Phase: project.DevTaskPhasePreCompose},
		{Name: "Compose", Cmd: appendDevLifecycleTestLine(logPath, "compose"), ID: project.DevTaskIDCompose, Phase: project.DevTaskPhaseCompose},
		{Name: "Database ready", Cmd: appendDevLifecycleTestLine(logPath, "post-compose"), ID: "database-ready", Phase: project.DevTaskPhasePostCompose},
		{Name: "Generate", Cmd: appendDevLifecycleTestLine(logPath, "post-migrate"), ID: "generate", Phase: project.DevTaskPhasePostMigrate},
	}
	barrier := func(context.Context) error {
		file, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = file.WriteString("barrier\n")
		return err
	}

	if err := runManagedDevInitialLifecycle(config, io.Discard, io.Discard, t.Context(), barrier); err != nil {
		t.Fatalf("runManagedDevInitialLifecycle() error = %v", err)
	}
	assertDevLifecycleTestLines(t, logPath, []string{"pre", "spa", "app", "compose", "barrier", "post-compose", "post-migrate", "app"})
}

// TestRunManagedDevInitialLifecycleStopsOnBarrierFailure verifies a failed Harbor acknowledgment does not run post-Compose work.
func TestRunManagedDevInitialLifecycleStopsOnBarrierFailure(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	logPath := filepath.Join(root, "managed-failure.log")
	config := &project.Config{Dev: project.DevConfig{Pre: []project.DevTask{
		{Name: "Compose", Cmd: appendDevLifecycleTestLine(logPath, "compose"), ID: project.DevTaskIDCompose, Phase: project.DevTaskPhaseCompose},
		{Name: "Post compose", Cmd: appendDevLifecycleTestLine(logPath, "post-compose"), ID: "post-compose", Phase: project.DevTaskPhasePostCompose},
	}, Apps: map[string]project.DevApp{
		project.DefaultAppName: {Build: &project.DevAppCommand{Disabled: true}},
	}}}
	wantErr := errors.New("route not ready")
	err := runManagedDevInitialLifecycle(config, io.Discard, io.Discard, t.Context(), func(context.Context) error {
		return wantErr
	})
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("runManagedDevInitialLifecycle() error = %v, want barrier failure", err)
	}
	data, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if got := strings.TrimSpace(string(data)); got != "compose" {
		t.Fatalf("managed barrier failure transcript = %q, want only compose", got)
	}
}

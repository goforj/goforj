package forj

import (
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

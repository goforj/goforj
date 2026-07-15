package generate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/project"
)

var goModTidyRunner = runGoModTidy

// Cmd selects generated resource packages and derived project files.
type Cmd struct {
	Storage       bool `help:"Generate storage code"`
	Cache         bool `help:"Generate cache code"`
	Mail          bool `help:"Generate mail code"`
	Queue         bool `help:"Generate queue code"`
	Events        bool `help:"Generate events code"`
	DB            bool `help:"Generate DB connection accessors"`
	Observability bool `help:"Generate observability-derived files"`
}

// NewCmd returns a generate command with the conventional all-resources default.
func NewCmd() *Cmd {
	return &Cmd{}
}

// Signature declares the generated command name and help text.
func (*Cmd) Signature() string {
	return `name:"generate" help:"Generate application code and derived files"`
}

// Run regenerates the selected project resources from the project-owned environment snapshot.
func (c *Cmd) Run() error {
	restoreEnvironment, err := loadGenerationEnvironment(".")
	if err != nil {
		return err
	}
	defer restoreEnvironment()
	selected := c.Storage || c.Cache || c.Mail || c.Queue || c.Events || c.DB || c.Observability
	ranStorage := false
	ranCache := false
	ranMail := false
	ranQueue := false
	ranEvents := false
	ranDB := false
	if !selected || c.Storage {
		storageEnabled, err := projectStorageEnabled(".")
		if err != nil && (!os.IsNotExist(err) || c.Storage) {
			return fmt.Errorf("load project Storage selection: %w", err)
		}
		if c.Storage && !storageEnabled {
			return fmt.Errorf("cannot generate Storage: the Storage component is disabled in .goforj.yml")
		}
		if storageEnabled {
			if _, err := GenerateStorageFiles("."); err != nil {
				return err
			}
			ranStorage = true
		}
	}
	if !selected || c.Cache {
		if _, err := os.Stat(filepath.Join("internal", "caches")); err == nil {
			if _, err := GenerateCacheFiles("."); err != nil {
				return err
			}
			ranCache = true
		}
	}
	if !selected || c.Mail {
		if _, err := os.Stat(filepath.Join("internal", "mail")); err == nil {
			if _, err := GenerateMailFiles("."); err != nil {
				return err
			}
			ranMail = true
		}
	}
	if !selected || c.Queue {
		if _, err := os.Stat(filepath.Join("internal", "queues")); err == nil {
			if _, err := GenerateQueueFiles("."); err != nil {
				return err
			}
			ranQueue = true
		}
	}
	if !selected || c.Events {
		eventsEnabled, err := projectEventsEnabled(".")
		if err != nil && (!os.IsNotExist(err) || c.Events) {
			return fmt.Errorf("load project Events selection: %w", err)
		}
		if c.Events && !eventsEnabled {
			return fmt.Errorf("cannot generate Events: the Events component is disabled in .goforj.yml")
		}
		if eventsEnabled {
			if _, err := GenerateEventFiles("."); err != nil {
				return err
			}
			ranEvents = true
		}
	}
	if !selected || c.DB {
		if _, err := os.Stat(filepath.Join("internal", "database")); err == nil {
			if _, err := GenerateDBFiles("."); err != nil {
				return err
			}
			ranDB = true
		}
	}
	if !selected || c.Observability {
		if _, err := os.Stat(filepath.Join("containers", "observability", "vmagent")); err == nil {
			if _, err := GenerateObservabilityFiles("."); err != nil {
				return err
			}
		}
	}
	if ranStorage || ranCache || ranMail || ranQueue || ranEvents || ranDB {
		if err := goModTidyRunner("."); err != nil {
			return err
		}
	}
	return nil
}

// GenerateProjectFiles regenerates selected resources beneath projectDir and reports total and changed files.
func GenerateProjectFiles(projectDir string, includeStorage, includeCache, includeQueue, includeEvents, includeDB, includeObservability bool) (int, int, error) {
	restoreEnvironment, err := loadGenerationEnvironment(projectDir)
	if err != nil {
		return 0, 0, err
	}
	defer restoreEnvironment()
	totalFiles := 0
	changedFiles := 0
	ranAny := false
	modTidyNeeded := false
	if includeStorage {
		written, err := GenerateStorageFiles(projectDir)
		if err != nil {
			return totalFiles, changedFiles, err
		}
		ranAny = true
		totalFiles += 2
		changedFiles += written
		modTidyNeeded = modTidyNeeded || written > 0
	}
	if includeCache {
		if _, err := os.Stat(filepath.Join(projectDir, "internal", "caches")); err == nil {
			written, err := GenerateCacheFiles(projectDir)
			if err != nil {
				return totalFiles, changedFiles, err
			}
			ranAny = true
			totalFiles += 2
			changedFiles += written
			modTidyNeeded = modTidyNeeded || written > 0
		}
	}
	if _, err := os.Stat(filepath.Join(projectDir, "internal", "mail")); err == nil {
		written, err := GenerateMailFiles(projectDir)
		if err != nil {
			return totalFiles, changedFiles, err
		}
		ranAny = true
		totalFiles += 2
		changedFiles += written
		modTidyNeeded = modTidyNeeded || written > 0
	}
	if includeQueue {
		if _, err := os.Stat(filepath.Join(projectDir, "internal", "queues")); err == nil {
			written, err := GenerateQueueFiles(projectDir)
			if err != nil {
				return totalFiles, changedFiles, err
			}
			ranAny = true
			totalFiles += 2
			changedFiles += written
			modTidyNeeded = modTidyNeeded || written > 0
		}
	}
	if includeEvents {
		written, err := GenerateEventFiles(projectDir)
		if err != nil {
			return totalFiles, changedFiles, err
		}
		ranAny = true
		totalFiles += 2
		changedFiles += written
		modTidyNeeded = modTidyNeeded || written > 0
	}
	if includeDB {
		if _, err := os.Stat(filepath.Join(projectDir, "internal", "database")); err == nil {
			written, err := GenerateDBFiles(projectDir)
			if err != nil {
				return totalFiles, changedFiles, err
			}
			ranAny = true
			totalFiles++
			changedFiles += written
			modTidyNeeded = modTidyNeeded || written > 0
		}
	}
	if includeObservability {
		if _, err := os.Stat(filepath.Join(projectDir, "containers", "observability", "vmagent")); err == nil {
			written, err := GenerateObservabilityFiles(projectDir)
			if err != nil {
				return totalFiles, changedFiles, err
			}
			totalFiles++
			changedFiles += written
		}
	}
	if ranAny && modTidyNeeded {
		if err := goModTidyRunner(projectDir); err != nil {
			return totalFiles, changedFiles, err
		}
	}
	return totalFiles, changedFiles, nil
}

// projectEventsEnabled resolves Events participation from project configuration rather than generated-directory residue.
func projectEventsEnabled(projectDir string) (bool, error) {
	config, err := project.LoadProjectConfigAt(projectDir)
	if err != nil {
		return false, err
	}
	return project.ProjectComponents(config).Events, nil
}

// projectStorageEnabled resolves Storage participation from project configuration rather than generated-directory residue.
func projectStorageEnabled(projectDir string) (bool, error) {
	config, err := project.LoadProjectConfigAt(projectDir)
	if err != nil {
		return false, err
	}
	return project.ProjectComponents(config).Storage, nil
}

// runGoModTidy refreshes dependencies without exposing project resource credentials to Go or invoked VCS processes.
func runGoModTidy(projectDir string) error {
	if _, err := os.Stat(filepath.Join(projectDir, "go.mod")); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = projectDir
	cmd.Env = generationSubprocessEnvironment()
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}
	return nil
}

// generationSubprocessEnvironment retains the developer's toolchain environment while removing temporary generator inputs.
func generationSubprocessEnvironment() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, assignment := range os.Environ() {
		key, _, _ := strings.Cut(assignment, "=")
		if isGenerationEnvironmentKey(key) {
			continue
		}
		environment = append(environment, assignment)
	}
	return environment
}

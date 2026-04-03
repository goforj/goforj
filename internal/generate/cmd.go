package generate

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/goforj/env/v2"
)

var goModTidyRunner = runGoModTidy

type Cmd struct {
	Storage bool `help:"Generate storage code"`
	Cache   bool `help:"Generate cache code"`
	Queue   bool `help:"Generate queue code"`
	Events  bool `help:"Generate events code"`
	DB      bool `help:"Generate DB connection accessors"`
}

func NewCmd() *Cmd {
	return &Cmd{}
}

func (*Cmd) Signature() string {
	return `name:"generate" help:"Generate application code and derived files"`
}

func (c *Cmd) Run() error {
	if err := env.Load(); err != nil {
		return err
	}
	selected := c.Storage || c.Cache || c.Queue || c.Events || c.DB
	ranStorage := false
	ranCache := false
	ranQueue := false
	ranEvents := false
	ranDB := false
	if !selected || c.Storage {
		if _, err := GenerateStorageFiles("."); err != nil {
			return err
		}
		ranStorage = true
	}
	if !selected || c.Cache {
		if _, err := os.Stat(filepath.Join("internal", "caches")); err == nil {
			if _, err := GenerateCacheFiles("."); err != nil {
				return err
			}
			ranCache = true
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
		if _, err := os.Stat(filepath.Join("internal", "events")); err == nil {
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
	if ranStorage || ranCache || ranQueue || ranEvents || ranDB {
		if err := goModTidyRunner("."); err != nil {
			return err
		}
	}
	return nil
}

func GenerateProjectFiles(projectDir string, includeStorage, includeCache, includeQueue, includeEvents, includeDB bool) (int, int, error) {
	if err := env.Load(); err != nil {
		return 0, 0, err
	}
	totalFiles := 0
	changedFiles := 0
	ranAny := false
	if includeStorage {
		written, err := GenerateStorageFiles(projectDir)
		if err != nil {
			return totalFiles, changedFiles, err
		}
		ranAny = true
		totalFiles += 2
		changedFiles += written
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
		}
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
		}
	}
	if includeEvents {
		if _, err := os.Stat(filepath.Join(projectDir, "internal", "events")); err == nil {
			written, err := GenerateEventFiles(projectDir)
			if err != nil {
				return totalFiles, changedFiles, err
			}
			ranAny = true
			totalFiles++
			changedFiles += written
		}
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
		}
	}
	if ranAny {
		if err := goModTidyRunner(projectDir); err != nil {
			return totalFiles, changedFiles, err
		}
	}
	return totalFiles, changedFiles, nil
}

func runGoModTidy(projectDir string) error {
	if _, err := os.Stat(filepath.Join(projectDir, "go.mod")); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = projectDir
	cmd.Env = os.Environ()
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}
	return nil
}

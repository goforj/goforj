package generate

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/goforj/env/v2"
)

type Cmd struct {
	Storage bool `help:"Generate storage code"`
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
	selected := c.Storage || c.DB
	ranStorage := false
	if !selected || c.Storage {
		if _, err := GenerateStorageFiles("."); err != nil {
			return err
		}
		ranStorage = true
	}
	if !selected || c.DB {
		if _, err := os.Stat(filepath.Join("internal", "dbconns")); err == nil {
			if _, err := GenerateDBFiles("."); err != nil {
				return err
			}
		}
	}
	if ranStorage {
		if err := runGoModTidy("."); err != nil {
			return err
		}
	}
	return nil
}

func GenerateProjectFiles(projectDir string, includeStorage, includeDB bool) (int, int, error) {
	if err := env.Load(); err != nil {
		return 0, 0, err
	}
	totalFiles := 0
	changedFiles := 0
	if includeStorage {
		written, err := GenerateStorageFiles(projectDir)
		if err != nil {
			return totalFiles, changedFiles, err
		}
		totalFiles += 2
		changedFiles += written
	}
	if includeDB {
		if _, err := os.Stat(filepath.Join(projectDir, "internal", "dbconns")); err == nil {
			written, err := GenerateDBFiles(projectDir)
			if err != nil {
				return totalFiles, changedFiles, err
			}
			totalFiles++
			changedFiles += written
		}
	}
	return totalFiles, changedFiles, nil
}

func runGoModTidy(projectDir string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = projectDir
	cmd.Env = os.Environ()
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}
	return nil
}

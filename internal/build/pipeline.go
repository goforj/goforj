package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/logger"
)

type Step struct {
	Name string
	Run  func() error
}

type Pipeline struct {
	logger   *logger.AppLogger
	apiIndex *APIIndexRunner
}

func NewPipeline(appLogger *logger.AppLogger, apiIndex *APIIndexRunner) Pipeline {
	return Pipeline{
		logger:   appLogger,
		apiIndex: apiIndex,
	}
}

func (p Pipeline) Run(root string, kind string, final Step) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	originalWD, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(absRoot); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(originalWD) }()

	steps := []Step{
		{Name: "generate", Run: p.generateProjectFiles},
		{Name: "build:api-index", Run: p.apiIndex.RunQuiet},
		final,
	}

	debug := debugEnabled()
	for _, step := range steps {
		if debug {
			p.logger.Info().Str("kind", kind).Str("step", step.Name).Msg("Running pipeline step")
		}
		if err := step.Run(); err != nil {
			return err
		}
	}
	if debug {
		p.logger.Info().Str("kind", kind).Int("steps", len(steps)).Msg("Pipeline completed")
	}
	return nil
}

func (p Pipeline) generateProjectFiles() error {
	generatedFiles, err := generate.GenerateProjectFiles(".", true, hasDir(filepath.Join(".", "internal", "dbconns")))
	if err != nil {
		return fmt.Errorf("generate project files: %w", err)
	}
	if debugEnabled() {
		p.logger.Info().Int("files", generatedFiles).Msg("Generated project files")
	}
	return nil
}

func hasDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func debugEnabled() bool {
	for _, key := range []string{"FORJ_DEBUG", "APP_DEBUG", "DEBUG"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

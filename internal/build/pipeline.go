package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

type Step struct {
	Name string
	Run  func() (string, error)
}

type Pipeline struct {
	logger   *logger.AppLogger
	apiIndex *APIIndexRunner
}

type RunOptions struct {
	Timings bool
}

func NewPipeline(appLogger *logger.AppLogger, apiIndex *APIIndexRunner) Pipeline {
	return Pipeline{
		logger:   appLogger,
		apiIndex: apiIndex,
	}
}

func (p Pipeline) Run(root string, kind string, final Step, opts RunOptions) error {
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
		{Name: "wire", Run: p.runWireGenerate},
		{Name: "generate", Run: p.generateProjectFiles},
		{Name: "build:api-index", Run: p.runAPIIndex},
		final,
	}

	debug := debugEnabled()
	for _, step := range steps {
		if debug {
			p.logger.Info().Str("kind", kind).Str("step", step.Name).Msg("Running pipeline step")
		}
		startedAt := time.Now()
		status, err := step.Run()
		if err != nil {
			return err
		}
		if opts.Timings {
			timing := time.Since(startedAt).Round(time.Millisecond)
			if strings.TrimSpace(status) != "" {
				fmt.Fprintf(os.Stderr, "forj %s %s: %s (%s)\n", kind, step.Name, timing, status)
			} else {
				fmt.Fprintf(os.Stderr, "forj %s %s: %s\n", kind, step.Name, timing)
			}
		}
	}
	if debug {
		p.logger.Info().Str("kind", kind).Int("steps", len(steps)).Msg("Pipeline completed")
	}
	return nil
}

func (p Pipeline) runWireGenerate() (string, error) {
	ran := false
	for _, wirePath := range loadWirePaths() {
		info, err := os.Stat(wirePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("wire path %q: %w", wirePath, err)
		}
		if !info.IsDir() {
			continue
		}
		shouldRun, err := wirePathStale(wirePath)
		if err != nil {
			return "", err
		}
		if !shouldRun {
			continue
		}
		ran = true

		cmd := exec.Command("wire")
		cmd.Dir = wirePath
		if debugEnabled() {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return "", fmt.Errorf("wire (%s): %w", wirePath, err)
			}
			continue
		}

		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			detail := strings.TrimSpace(stderr.String())
			if detail == "" {
				detail = strings.TrimSpace(stdout.String())
			}
			if detail != "" {
				return "", fmt.Errorf("wire (%s): %w (%s)", wirePath, err, detail)
			}
			return "", fmt.Errorf("wire (%s): %w", wirePath, err)
		}
	}
	if !ran {
		return "no changes", nil
	}
	return "", nil
}

func wirePathStale(wirePath string) (bool, error) {
	generatedPath := filepath.Join(wirePath, "wire_gen.go")
	generatedInfo, err := os.Stat(generatedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("wire path %q: stat wire_gen.go: %w", wirePath, err)
	}

	latestSourceMod, hasSource, err := latestWireSourceModTime(wirePath)
	if err != nil {
		return false, err
	}
	if !hasSource {
		return false, nil
	}
	return latestSourceMod.After(generatedInfo.ModTime()), nil
}

func latestWireSourceModTime(wirePath string) (time.Time, bool, error) {
	var latest time.Time
	var found bool

	err := filepath.WalkDir(wirePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "wire_gen.go" {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !found || info.ModTime().After(latest) {
			latest = info.ModTime()
			found = true
		}
		return nil
	})
	if err != nil {
		return time.Time{}, false, fmt.Errorf("wire path %q: scan sources: %w", wirePath, err)
	}
	return latest, found, nil
}

func (p Pipeline) generateProjectFiles() (string, error) {
	generatedFiles, changedFiles, err := generate.GenerateProjectFiles(
		".",
		true,
		hasDir(filepath.Join(".", "internal", "cache")),
		hasDir(filepath.Join(".", "internal", "dbconns")),
	)
	if err != nil {
		return "", fmt.Errorf("generate project files: %w", err)
	}
	if debugEnabled() {
		p.logger.Info().Int("files", generatedFiles).Msg("Generated project files")
	}
	if changedFiles == 0 {
		return "no changes", nil
	}
	return fmt.Sprintf("%d files", changedFiles), nil
}

func (p Pipeline) runAPIIndex() (string, error) {
	status, err := p.apiIndex.RunDefaultWithStatus()
	if err != nil {
		return "", err
	}
	return status, nil
}

func loadWirePaths() []string {
	config, err := project.LoadProjectConfig()
	if err != nil {
		return []string{"wire"}
	}
	if len(config.Dev.WirePaths) == 0 {
		return []string{"wire"}
	}
	return config.Dev.WirePaths
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

package forj

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/goforj/forj/internal/logger"
)

type RunCmd struct {
	Args []string `arg:"" optional:"" help:"Arguments to pass to 'go run main.go'"`

	logger *logger.AppLogger
}

func NewRunCmd(logger *logger.AppLogger) *RunCmd {
	return &RunCmd{logger: logger}
}

func (c *RunCmd) Run() error {
	config, err := LoadProjectConfig()
	if err != nil {
		return fmt.Errorf("failed to load .goforj.yml: %w", err)
	}

	var appWatch *DevWatch
	for _, w := range config.DevWatches {
		if strings.EqualFold(w.Name, "App") {
			appWatch = &w
			break
		}
	}
	if appWatch == nil {
		return fmt.Errorf("no DevWatch named 'App' found in .goforj.yml")
	}

	// Combine: wgo <watch args> bash -c 'go run main.go <args...>'
	var args []string
	args = append(args, strings.Fields(appWatch.Watch)...)
	runCmd := fmt.Sprintf("go run main.go %s", strings.Join(c.Args, " "))
	args = append(args, "bash", "-c", runCmd)

	cmd := exec.Command("wgo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	c.logger.Info().Msgf("Starting watcher: go run main.go %s", strings.Join(c.Args, " "))
	return cmd.Run()
}

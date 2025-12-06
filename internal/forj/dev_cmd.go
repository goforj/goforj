package forj

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/goforj/forj/internal/logger"
)

type DevCmd struct {
	logger *logger.AppLogger
}

func NewDevCmd(logger *logger.AppLogger) *DevCmd {
	return &DevCmd{logger: logger}
}

const (
	colorReset     = "\033[0m"
	colorBoldWhite = "\033[1;97m"
	colorGray      = "\033[90m"
)

func (c *DevCmd) Run() error {
	config, err := LoadProjectConfig()
	if err != nil {
		return err
	}

	if len(config.DevWatches) == 0 {
		fmt.Println("No dev watches defined in .goforj.yml")
		return nil
	}

	// 🐾 Run pre-dev commands if any
	if len(config.PreDev) > 0 {
		fmt.Println("🔧 Running pre-dev setup:")
		for _, task := range config.PreDev {
			fmt.Printf(" > %s...\n", task.Name)
			cmd := exec.Command("bash", "-c", task.Cmd)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("pre-dev task '%s' failed: %v", task.Name, err)
			}
		}
	}

	fmt.Println("🚀 Running dev watchers:")

	var segments []string
	for _, watch := range config.DevWatches {
		fmt.Printf(" - Preparing %s...\n", watch.Name)

		// Build wgo command cleanly
		echoCmd := fmt.Sprintf(
			"echo -e \"%sGoForj Watcher › %s%s%s%s\"",
			colorBoldWhite,
			colorReset,
			colorGray,
			watch.Name,
			colorReset,
		)

		wgoCmd := fmt.Sprintf(
			"wgo %s bash -c '%s && %s'",
			watch.Watch,
			echoCmd,
			watch.Exec,
		)

		segments = append(segments, wgoCmd)
	}

	// Build a full command with ::
	fullCmd := strings.Join(segments, " :: ")

	cmd := exec.Command("bash", "-c", fullCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// strip any APP_ env vars from the command
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "APP_") {
			cmd.Env = append(cmd.Env, env) // Keep other env vars
		}
	}

	fmt.Println("\n🐾 Launching dev watchers [command]")
	fmt.Println(fullCmd)
	fmt.Println()

	return cmd.Run()
}

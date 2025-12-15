package forj

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/goforj/goforj/internal/logger"
)

type DownCmd struct {
	logger *logger.AppLogger
}

func NewDownCmd(logger *logger.AppLogger) *DownCmd {
	return &DownCmd{logger: logger}
}

// Run executes dev_down tasks to tear down resources.
func (c *DownCmd) Run() error {
	config, err := LoadProjectConfig()
	if err != nil {
		return err
	}

	if len(config.DevDown) == 0 {
		fmt.Println("No dev_down tasks defined in .goforj.yml")
		return nil
	}

	fmt.Println("Bringing down resources:")
	for _, task := range config.DevDown {
		fmt.Printf(" > %s...\n", task.Name)
		cmd := exec.Command("bash", "-c", task.Cmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("dev_down task '%s' failed: %v", task.Name, err)
		}
	}

	return nil
}

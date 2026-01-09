package forj

import (
	"fmt"
	"os"

	"github.com/goforj/execx"
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

	if len(config.Dev.Down) == 0 {
		fmt.Printf("%s No dev down tasks defined in .goforj.yml\n", warnMark())
		return nil
	}

	fmt.Printf("%s Bringing down resources:\n", actionMark())
	for _, task := range config.Dev.Down {
		fmt.Printf(" %s %s\n", infoMark(), task.Name)
		res, err := execx.Command("bash", "-c", task.Cmd).
			EnvInherit().
			StdinReader(os.Stdin).
			StdoutWriter(os.Stdout).
			StderrWriter(os.Stderr).
			Run()
		if err != nil {
			return fmt.Errorf("dev_down task '%s' failed: %v", task.Name, err)
		}
		if !res.OK() {
			return fmt.Errorf("dev_down task '%s' failed with exit code %d", task.Name, res.ExitCode)
		}
	}

	return nil
}

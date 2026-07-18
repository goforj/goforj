package forj

import (
	"fmt"
	"os"

	"github.com/goforj/console"
	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

type DownCmd struct {
	logger *logger.AppLogger
}

func (*DownCmd) Signature() string {
	return `name:"down" help:"Bring down development resources"`
}

func NewDownCmd(logger *logger.AppLogger) *DownCmd {
	return &DownCmd{logger: logger}
}

// Run executes dev_down tasks to tear down resources.
func (c *DownCmd) Run() error {
	config, err := project.LoadProjectConfig()
	if err != nil {
		return err
	}

	tasks := effectiveDevDownTasks(config)
	if len(tasks) == 0 {
		console.Warnf("No dev down tasks defined in .goforj.yml")
		return nil
	}

	console.Actionf("Bringing down resources:")
	for _, task := range tasks {
		console.Infof("%s", task.Name)
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

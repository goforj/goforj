package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/logger"
)

type WgoCmd struct {
	Label string   `help:"Label for watcher output" short:"l"`
	Args  []string `arg:"" optional:"" help:"wgo args followed by the command to run"`

	logger *logger.AppLogger
}

func NewWgoCmd(logger *logger.AppLogger) *WgoCmd {
	return &WgoCmd{logger: logger}
}

// Run executes wgo with GoForj defaults.
func (c *WgoCmd) Run() error {
	wgoArgs, cmdArgs := splitWgoArgs(c.Args)
	if len(cmdArgs) == 0 {
		return fmt.Errorf("missing command to run (use `--` to separate wgo args from the command)")
	}

	label := strings.TrimSpace(c.Label)
	if label == "" {
		label = defaultWatcherLabel(cmdArgs[0])
	}

	execMsg := fmt.Sprintf(
		" · %sGoForj Watcher%s > %s%s%s",
		colorBoldWhite,
		colorReset,
		colorGray,
		label,
		colorReset,
	)

	args := append([]string{}, wgoArgs...)
	args = append(args, "-log-prefix=", "-exec-log", "-exec-msg", execMsg)
	args = append(args, cmdArgs...)

	c.logger.Info().Msgf("forj wgo > wgo %s", strings.Join(args, " "))
	res, err := execx.Command("wgo", args...).
		EnvInherit().
		StdinReader(os.Stdin).
		StdoutWriter(os.Stdout).
		StderrWriter(os.Stderr).
		Run()
	if err != nil {
		return err
	}
	if !res.OK() {
		return fmt.Errorf("wgo exited with code %d", res.ExitCode)
	}
	return nil
}

// splitWgoArgs splits wgo flags from the command to run.
func splitWgoArgs(args []string) (wgoArgs []string, cmdArgs []string) {
	for i, arg := range args {
		if arg == "--" {
			return args[:i], args[i+1:]
		}
	}
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return args[:i], args[i:]
		}
	}
	return args, nil
}

// defaultWatcherLabel derives a label from the command name.
func defaultWatcherLabel(cmd string) string {
	base := filepath.Base(cmd)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" {
		return "Watcher"
	}
	return base
}

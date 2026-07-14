package bench

import (
	"fmt"
	"os"
	"strings"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
)

func repoForjExecutable(modCache, buildCache string) (string, func(), error) {
	return testkit.BuildForjBinary(modCache, buildCache)
}

func runStep(log *logger.AppLogger, silent bool, name, dir, modCache, buildCache string, args []string) error {
	cmd := execx.Command(args[0], args[1:]...).
		Dir(dir).
		EnvAppend(map[string]string{
			"GOMODCACHE": modCache,
			"GOCACHE":    buildCache,
		})

	if !silent {
		cmd = cmd.ShadowPrint(
			execx.WithFormatter(func(ev execx.ShadowEvent) string {
				switch ev.Phase {
				case execx.ShadowBefore:
					return fmt.Sprintf("%s %s", console.ActionMark(), ev.Command)
				case execx.ShadowAfter:
					return fmt.Sprintf("%s %s (%s)", console.InfoMark(), ev.Command, ev.Duration)
				default:
					return fmt.Sprintf("%s %s", console.InfoMark(), ev.Command)
				}
			}),
		)
	}

	res, err := cmd.Run()
	if err != nil || !res.OK() {
		if !silent {
			console.Errorf("%s failed", name)
		}
		if err == nil {
			err = fmt.Errorf("command failed with exit code %d", res.ExitCode)
		}
		errMsg := strings.TrimSpace(res.Stderr)
		if errMsg == "" {
			errMsg = err.Error()
		}
		if errMsg == "" {
			errMsg = "command failed"
		}
		log.Error().
			Str("step", name).
			Str("stdout", strings.TrimSpace(res.Stdout)).
			Err(fmt.Errorf("%s", errMsg)).
			Msg("Step failed")
		return err
	}
	return nil
}

// writeYAML stamps the current component contract so omitted primitive names retain their disabled meaning.
func writeYAML(path string, cfg project.Config) error {
	cfg.Render.StarterKit = project.NormalizeStarterKit(cfg.Render.StarterKit)
	cfg.Render.ComponentContractVersion = project.CurrentComponentContractVersion
	if strings.TrimSpace(cfg.Render.GoForjVersion) == "" {
		cfg.Render.GoForjVersion = version.Semver()
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

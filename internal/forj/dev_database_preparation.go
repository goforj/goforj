package forj

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/project"
)

const devDatabaseTimeout = 30 * time.Second
const devDatabaseRetryPeriod = 500 * time.Millisecond

// devDatabaseCommandRunner keeps native execution at one boundary for readiness and preparation.
type devDatabaseCommandRunner func(context.Context, string, []string, string) (execx.Result, error)

// devDatabasePreparation shares successful preparation with historical hook positions within one startup.
type devDatabasePreparation struct {
	ctx      context.Context
	config   *project.Config
	run      devDatabaseCommandRunner
	prepared map[string]bool
}

// newDevDatabasePreparation gives each startup its own cancellation and preparation lifetime.
func newDevDatabasePreparation(ctx context.Context, config *project.Config) *devDatabasePreparation {
	if ctx == nil {
		ctx = context.Background()
	}
	return &devDatabasePreparation{ctx: ctx, config: config, run: runDevDatabaseCommand, prepared: map[string]bool{}}
}

// prepare handles selected local databases independently from optional automatic migrations.
func (preparation *devDatabasePreparation) prepare(stdout, stderr io.Writer, services ...string) error {
	if !preparation.config.Render.Components.Docker {
		return nil
	}
	databases, err := devDatabasesForApps(preparation.config, activeDevAppsForConfig(preparation.config))
	if err != nil {
		return err
	}
	started := time.Now()
	if len(services) == 0 {
		services = []string{"mysql", "postgres"}
	}
	prepared := false
	for _, driver := range services {
		if preparation.prepared[driver] {
			continue
		}
		names := []string{}
		for _, database := range databases {
			if database.Driver == driver {
				names = append(names, database.Name)
			}
		}
		if len(names) == 0 {
			continue
		}
		result, err := prepareDevDatabase(preparation.ctx, driver, names, preparation.run)
		if err != nil {
			return err
		}
		if _, err = io.WriteString(stdout, result.Stdout); err != nil {
			return err
		}
		if _, err = io.WriteString(stderr, result.Stderr); err != nil {
			return err
		}
		preparation.prepared[driver] = true
		prepared = true
	}
	if prepared {
		writeDevSuccessLine(stdout, "Databases", formatDevElapsed(time.Since(started)))
	}
	return nil
}

// prepareDevDatabase keeps probing and authenticated provisioning inside one bounded operation.
func prepareDevDatabase(ctx context.Context, driver string, names []string, run devDatabaseCommandRunner) (execx.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, devDatabaseTimeout)
	defer cancel()
	var probe []string
	var script string
	switch driver {
	case "mysql":
		probe = []string{"mariadb-admin", "ping", "--protocol=socket", "--connect-timeout=1"}
		script = devMySQLPreparationScript
	case "postgres":
		probe = []string{"pg_isready", "-h", "postgres", "-p", "5432", "-t", "1"}
		script = devPostgresPreparationScript
	default:
		return execx.Result{}, fmt.Errorf("unsupported development database driver %q", driver)
	}
	if err := waitDevDatabase(ctx, driver, probe, run); err != nil {
		return execx.Result{}, err
	}
	result, err := run(ctx, driver, append([]string{"sh", "-s", "--"}, names...), script)
	if ctx.Err() != nil {
		return result, fmt.Errorf("prepare %s database: %w", driver, ctx.Err())
	}
	if err != nil {
		return result, fmt.Errorf("prepare %s database: %w", driver, err)
	}
	if !result.OK() {
		return result, fmt.Errorf("prepare %s database: %s", driver, devDatabaseFailure(result))
	}
	return result, nil
}

// waitDevDatabase retains the last native diagnostic while Go owns retries and cancellation.
func waitDevDatabase(ctx context.Context, driver string, arguments []string, run devDatabaseCommandRunner) error {
	last := execx.Result{ExitCode: -1}
	for {
		if ctx.Err() != nil {
			return fmt.Errorf("wait for %s database: %w (last probe: %s)", driver, ctx.Err(), devDatabaseFailure(last))
		}
		result, err := run(ctx, driver, arguments, "")
		last = result
		if ctx.Err() != nil {
			continue
		}
		if err != nil {
			return fmt.Errorf("check %s database readiness: %w", driver, err)
		}
		if result.OK() {
			return nil
		}
		timer := time.NewTimer(devDatabaseRetryPeriod)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

// devDatabaseFailure uses native diagnostics without presenting a command containing credentials.
func devDatabaseFailure(result execx.Result) string {
	if diagnostic := strings.TrimSpace(result.Stderr + "\n" + result.Stdout); diagnostic != "" {
		return diagnostic
	}
	return fmt.Sprintf("exit code %d", result.ExitCode)
}

// runDevDatabaseCommand passes arguments directly and supplies the container script through stdin.
func runDevDatabaseCommand(ctx context.Context, service string, arguments []string, input string) (execx.Result, error) {
	return devComposeCommand(append([]string{"exec", "-T", service}, arguments...)...).
		WithContext(ctx).
		EnvInherit().
		StdinString(input).
		Run()
}

// runDevPreTasks preserves custom hook ordering around exact historical readiness positions.
func runDevPreTasks(preparation *devDatabasePreparation, tasks []project.DevTask, stdout, stderr io.Writer, observers ...*devStartupProgress) error {
	for _, task := range tasks {
		if service := legacyDevDatabaseTaskService(task); service != "" {
			if preparation.prepared[service] {
				continue
			}
			if err := runDevPhaseOutput("Preparing dev databases", stdout, stderr, func(out, errOut io.Writer) error { return preparation.prepare(out, errOut, service) }, observers...); err != nil {
				return err
			}
			continue
		}
		if err := runDevTasks([]project.DevTask{task}, observers...); err != nil {
			return err
		}
	}
	return nil
}

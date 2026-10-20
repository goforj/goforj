package forj

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/project"
)

// TestDevDatabasePreparationUsesNativeArguments checks the same runner contract for both supported database services.
func TestDevDatabasePreparationUsesNativeArguments(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			calls := 0
			run := func(ctx context.Context, service string, args []string, input string) (execx.Result, error) {
				calls++
				if service != driver {
					t.Fatalf("service=%q", service)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("command has no bounded context")
				}
				if calls == 1 {
					if driver == "mysql" {
						if !reflect.DeepEqual(args, []string{"sh", "-s"}) || input != devMySQLReadinessScript {
							t.Fatalf("MySQL readiness did not use its fixed stdin script: %q", args)
						}
					} else if input != "" || args[0] == "sh" {
						t.Fatalf("Postgres probe uses shell/input: %q %q", args, input)
					}
				} else {
					expectedScript := devMySQLPreparationScript
					if driver == "postgres" {
						expectedScript = devPostgresPreparationScript
					}
					if !reflect.DeepEqual(args, []string{"sh", "-s", "--", "billing", "db"}) || input != expectedScript {
						t.Fatalf("prepare args=%q input=%q", args, input)
					}
					if strings.Contains(input, "sleep") || strings.Contains(input, "until ") {
						t.Fatal("container script owns readiness loop")
					}
				}
				return execx.Result{}, nil
			}
			if _, err := prepareDevDatabase(t.Context(), driver, []string{"billing", "db"}, run); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatalf("calls=%d, want one probe and one preparation", calls)
			}
		})
	}
}

// TestDevDatabaseReadinessRetriesThenSucceeds verifies transient native failures remain retryable.
func TestDevDatabaseReadinessRetriesThenSucceeds(t *testing.T) {
	calls := 0
	run := func(context.Context, string, []string, string) (execx.Result, error) {
		calls++
		if calls == 1 {
			return execx.Result{ExitCode: 1, Stderr: "socket not ready"}, nil
		}
		return execx.Result{}, nil
	}
	if err := waitDevDatabase(t.Context(), "mysql", nil, "", run); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("probe calls=%d", calls)
	}
}

// TestDevDatabasePreparationReportsFailures covers cancellation, native errors, unsupported services, and failed SQL.
func TestDevDatabasePreparationReportsFailures(t *testing.T) {
	t.Run("deadline retains diagnostic", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		calls := 0
		run := func(context.Context, string, []string, string) (execx.Result, error) {
			calls++
			return execx.Result{ExitCode: 1, Stderr: "socket unavailable"}, nil
		}
		_, err := prepareDevDatabase(ctx, "mysql", []string{"db"}, run)
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "wait for mysql database") {
			t.Fatalf("error=%v calls=%d", err, calls)
		}
	})
	t.Run("last native diagnostic", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		run := func(context.Context, string, []string, string) (execx.Result, error) {
			cancel()
			return execx.Result{ExitCode: 1, Stderr: "socket unavailable"}, nil
		}
		_, err := prepareDevDatabase(ctx, "mysql", []string{"db"}, run)
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "socket unavailable") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("canceled before execution", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := prepareDevDatabase(ctx, "mysql", nil, func(context.Context, string, []string, string) (execx.Result, error) {
			t.Fatal("ran canceled command")
			return execx.Result{}, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
	t.Run("launch failure", func(t *testing.T) {
		failure := errors.New("compose missing")
		_, err := prepareDevDatabase(t.Context(), "mysql", nil, func(context.Context, string, []string, string) (execx.Result, error) { return execx.Result{}, failure })
		if !errors.Is(err, failure) {
			t.Fatal(err)
		}
	})
	for _, mode := range []string{"sql exit", "sql launch", "sql cancellation"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("SQL transport failed")
			run := func(_ context.Context, _ string, _ []string, input string) (execx.Result, error) {
				if input == devMySQLReadinessScript {
					return execx.Result{}, nil
				}
				if mode == "sql cancellation" {
					cancel()
					return execx.Result{}, ctx.Err()
				}
				if mode == "sql launch" {
					return execx.Result{}, failure
				}
				return execx.Result{ExitCode: 9, Stderr: "authentication rejected"}, nil
			}
			_, err := prepareDevDatabase(ctx, "mysql", []string{"db"}, run)
			if err == nil || mode == "sql exit" && !strings.Contains(err.Error(), "authentication rejected") || mode == "sql launch" && !errors.Is(err, failure) || mode == "sql cancellation" && !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	t.Run("unsupported driver", func(t *testing.T) {
		_, err := prepareDevDatabase(t.Context(), "other", nil, nil)
		if err == nil {
			t.Fatal("unsupported service admitted")
		}
	})
	t.Run("empty diagnostic", func(t *testing.T) {
		if got := devDatabaseFailure(execx.Result{ExitCode: 7}); got != "exit code 7" {
			t.Fatal(got)
		}
	})
}

// TestDevDatabasePreparationIndependentFromMigrations proves one CLI preparation also replaces old hook positions.
func TestDevDatabasePreparationIndependentFromMigrations(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DATABASE", "db")
	t.Setenv("COMPOSE_PROFILES", "mysql")
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{Docker: true, DatabaseMySQL: true}}}
	preparation := newDevDatabasePreparation(t.Context(), config)
	calls := 0
	preparation.run = func(context.Context, string, []string, string) (execx.Result, error) {
		calls++
		return execx.Result{}, nil
	}
	var output bytes.Buffer
	if err := runDevPreTasks(preparation, []project.DevTask{{Name: "Waiting for Database to be ready", Cmd: legacyMySQLDevWaitCommand}}, &output, &output); err != nil {
		t.Fatal(err)
	}
	if err := runDevAppSetup(preparation, &output, &output); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !preparation.prepared["mysql"] || strings.Count(output.String(), "Databases") != 1 {
		t.Fatalf("calls=%d output=%s", calls, &output)
	}
}

// TestDevLegacyDatabaseTaskIdentities protects all known generated commands while excluding owner customization.
func TestDevLegacyDatabaseTaskIdentities(t *testing.T) {
	for _, test := range []struct{ command, service string }{
		{legacyMySQLDevWaitCommand, "mysql"}, {legacySocketMySQLDevWaitCommand, "mysql"}, {legacyPostgresDevWaitCommand, "postgres"},
		{strings.ReplaceAll(legacyMySQLDevWaitCommand, "mariadb", "mysql"), "mysql"},
		{strings.Replace(legacyPostgresDevWaitCommand, "docker-compose", "docker compose", 1), "postgres"},
		{legacyMySQLDevWaitCommand + " && echo owner", ""},
	} {
		task := project.DevTask{Name: "Waiting for Database to be ready", Cmd: test.command}
		if got := legacyDevDatabaseTaskService(task); got != test.service {
			t.Fatalf("service=%q want=%q", got, test.service)
		}
		task.Name = "Owner readiness"
		if got := legacyDevDatabaseTaskService(task); got != "" {
			t.Fatalf("owner task claimed: %q", got)
		}
	}
}

// TestDevDatabaseScriptsAuthenticateAndFailFast executes the exact embedded scripts with controlled clients.
func TestDevDatabaseScriptsAuthenticateAndFailFast(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	for _, driver := range []string{"mysql", "postgres"} {
		for _, mode := range []string{"success", "authentication"} {
			t.Run(driver+mode, func(t *testing.T) {
				root := t.TempDir()
				clients := `record_database_client() {
    if [ "$DATABASE_TEST_MODE" = authentication ]; then
        printf '%s\n' 'authentication rejected' >&2
        return 9
    fi
    printf '%s\n' "$@" >> "$DATABASE_TEST_OUTPUT"
    while IFS= read -r line; do
        printf '%s\n' "$line" >> "$DATABASE_TEST_OUTPUT"
    done
    return 0
}
mariadb() { record_database_client "$@"; }
psql() { record_database_client "$@"; }
`
				script := devMySQLPreparationScript
				if driver == "postgres" {
					script = devPostgresPreparationScript
				}
				command := exec.CommandContext(t.Context(), shell, "-c", clients+script, "database-setup", "billing", "db")
				command.Env = append(os.Environ(), "DATABASE_TEST_MODE="+mode, "DATABASE_TEST_OUTPUT="+filepath.Join(root, "queries"), "MARIADB_ROOT_PASSWORD=fake", "MARIADB_USER=app", "POSTGRES_PASSWORD=fake", "POSTGRES_USER=postgres")
				output, err := command.CombinedOutput()
				if mode == "authentication" {
					if err == nil || !strings.Contains(string(output), "authentication rejected") {
						t.Fatalf("error=%v output=%s", err, output)
					}
					return
				}
				if err != nil {
					t.Fatalf("script error=%v output=%s", err, output)
				}
				queries, err := os.ReadFile(filepath.Join(root, "queries"))
				if err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"billing", "db"} {
					if !strings.Contains(string(queries), name) {
						t.Fatalf("missing database %q: %s", name, queries)
					}
				}
			})
		}
	}
}

// TestDevDatabasePreparationSkipsNonContainerProjects leaves external database lifecycles with the project owner.
func TestDevDatabasePreparationSkipsNonContainerProjects(t *testing.T) {
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{DatabaseMySQL: true}}}
	preparation := newDevDatabasePreparation(t.Context(), config)
	preparation.run = func(context.Context, string, []string, string) (execx.Result, error) {
		t.Fatal("probed external database")
		return execx.Result{}, nil
	}
	if err := preparation.prepare(io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}

// TestDevDatabasePreparationWithoutLegacyHook proves readiness does not depend on either YAML hooks or automatic migrations.
func TestDevDatabasePreparationWithoutLegacyHook(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DATABASE", "db")
	t.Setenv("COMPOSE_PROFILES", "mysql")
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{Docker: true, DatabaseMySQL: true}}}
	preparation := newDevDatabasePreparation(t.Context(), config)
	calls := 0
	preparation.run = func(context.Context, string, []string, string) (execx.Result, error) {
		calls++
		return execx.Result{}, nil
	}
	if err := runDevAppSetup(preparation, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want readiness and preparation without migrations", calls)
	}
}

// TestDevLegacyDatabaseHookKeepsOwnerOrdering preserves hooks that depended on the historical readiness position.
func TestDevLegacyDatabaseHookKeepsOwnerOrdering(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("shell unavailable")
	}
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DATABASE", "db")
	t.Setenv("COMPOSE_PROFILES", "mysql")
	path := filepath.Join(root, "order.log")
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{Docker: true, DatabaseMySQL: true}}}
	preparation := newDevDatabasePreparation(t.Context(), config)
	preparation.run = func(_ context.Context, _ string, _ []string, input string) (execx.Result, error) {
		if input == devMySQLPreparationScript {
			file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.WriteString(file, "database\n")
			if closeErr := file.Close(); err != nil || closeErr != nil {
				t.Fatalf("append order: %v %v", err, closeErr)
			}
		}
		return execx.Result{}, nil
	}
	tasks := []project.DevTask{
		{Name: "Before", Cmd: appendDevLifecycleTestLine(path, "before")},
		{Name: "Waiting for Database to be ready", Cmd: legacyMySQLDevWaitCommand},
		{Name: "After", Cmd: appendDevLifecycleTestLine(path, "after")},
		{Name: "Waiting for Database to be ready", Cmd: legacySocketMySQLDevWaitCommand},
	}
	if err := runDevPreTasks(preparation, tasks, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	assertDevLifecycleTestLines(t, path, []string{"before", "database", "after"})
}

// rejectedDatabaseWriter models a broken output destination after successful native preparation.
type rejectedDatabaseWriter struct{}

// Write preserves output failures rather than reporting a fully prepared startup to a disconnected caller.
func (rejectedDatabaseWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// TestDevDatabasePreparationPreservesFailures keeps invalid selection, native setup, and output errors visible.
func TestDevDatabasePreparationPreservesFailures(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DATABASE", "db")
	t.Setenv("COMPOSE_PROFILES", "mysql")
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{Docker: true, DatabaseMySQL: true}}}
	for _, mode := range []string{"invalid name", "native failure", "stdout failure", "stderr failure"} {
		t.Run(mode, func(t *testing.T) {
			preparation := newDevDatabasePreparation(t.Context(), config)
			failure := errors.New("native unavailable")
			preparation.run = func(context.Context, string, []string, string) (execx.Result, error) {
				if mode == "invalid name" {
					t.Fatal("invalid database reached native execution")
				}
				if mode == "native failure" {
					return execx.Result{}, failure
				}
				return execx.Result{Stdout: "prepared", Stderr: "notice"}, nil
			}
			var stdout, stderr io.Writer = io.Discard, io.Discard
			if mode == "invalid name" {
				t.Setenv("DB_DATABASE", "invalid-name")
			}
			if mode == "stdout failure" {
				stdout = rejectedDatabaseWriter{}
			}
			if mode == "stderr failure" {
				stderr = rejectedDatabaseWriter{}
			}
			err := preparation.prepare(stdout, stderr)
			if err == nil || len(preparation.prepared) > 0 {
				t.Fatalf("error=%v prepared=%v", err, preparation.prepared)
			}
			if mode == "native failure" && !errors.Is(err, failure) {
				t.Fatal(err)
			}
			if strings.HasSuffix(mode, "failure") && mode != "native failure" && !errors.Is(err, io.ErrClosedPipe) {
				t.Fatal(err)
			}
		})
	}
}

// TestDevLegacyDatabasePositionPreparesOnlyItsService preserves later owner hooks that start another selected service.
func TestDevLegacyDatabasePositionPreparesOnlyItsService(t *testing.T) {
	withConventionalApp(t, "reporting")
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DATABASE", "db")
	t.Setenv("REPORTING_DB_DRIVER", "postgres")
	t.Setenv("REPORTING_DB_DATABASE", "reporting")
	t.Setenv("COMPOSE_PROFILES", "mysql,postgres")
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{Docker: true, DatabaseMySQL: true, DatabasePostgres: true}},
		Apps:   map[string]project.AppConfig{"reporting": {Components: project.Components{DatabasePostgres: true}}},
	}
	preparation := newDevDatabasePreparation(t.Context(), config)
	calls := []string{}
	preparation.run = func(_ context.Context, service string, _ []string, input string) (execx.Result, error) {
		if input == devMySQLPreparationScript || input == devPostgresPreparationScript {
			calls = append(calls, service)
		}
		return execx.Result{}, nil
	}
	if err := runDevPreTasks(preparation, []project.DevTask{{Name: "Waiting for Database to be ready", Cmd: legacyMySQLDevWaitCommand}}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"mysql"}) {
		t.Fatalf("legacy MySQL hook prepared %q", calls)
	}
	if err := runDevAppSetup(preparation, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"mysql", "postgres"}) {
		t.Fatalf("final preparations=%q", calls)
	}
}

// TestDevPreTaskFailuresStopLaterHooks keeps native preparation and owner task failures from being swallowed.
func TestDevPreTaskFailuresStopLaterHooks(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("shell unavailable")
	}
	for _, kind := range []string{"owner", "database"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			t.Setenv("DB_DRIVER", "mysql")
			t.Setenv("DB_DATABASE", "db")
			t.Setenv("COMPOSE_PROFILES", "mysql")
			config := &project.Config{Render: project.RenderConfig{Components: project.Components{Docker: true, DatabaseMySQL: true}}}
			preparation := newDevDatabasePreparation(t.Context(), config)
			failure := errors.New("native preparation failed")
			preparation.run = func(context.Context, string, []string, string) (execx.Result, error) { return execx.Result{}, failure }
			task := project.DevTask{Name: "Owner task", Cmd: "exit 19"}
			if kind == "database" {
				task = project.DevTask{Name: "Waiting for Database to be ready", Cmd: legacyMySQLDevWaitCommand}
			}
			path := filepath.Join(root, "unexpected.log")
			tasks := []project.DevTask{task, {Name: "Later hook", Cmd: appendDevLifecycleTestLine(path, "unexpected")}}
			err := runDevPreTasks(preparation, tasks, io.Discard, io.Discard)
			if err == nil || kind == "database" && !errors.Is(err, failure) {
				t.Fatalf("error=%v", err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("later hook ran: %v", err)
			}
		})
	}
}

// TestDevMySQLReadinessAuthenticatesAndRejectsInitialization executes the exact probe against controlled client responses.
func TestDevMySQLReadinessAuthenticatesAndRejectsInitialization(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	for _, mode := range []string{"authentication", "temporary", "ready", "unexpected"} {
		t.Run(mode, func(t *testing.T) {
			result, err := runDevMySQLReadinessFixture(t.Context(), shell, mode, devMySQLReadinessScript)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "ready" {
				if !result.OK() {
					t.Fatalf("authenticated permanent server rejected: %s", result.Stderr)
				}
				return
			}
			if result.OK() {
				t.Fatalf("%s server admitted", mode)
			}
			if mode == "authentication" && !strings.Contains(result.Stderr, "authentication rejected") {
				t.Fatalf("authentication diagnostic lost: %s", result.Stderr)
			}
		})
	}
}

// runDevMySQLReadinessFixture checks authenticated client arguments without exposing a password through argv.
func runDevMySQLReadinessFixture(ctx context.Context, shell, mode, script string) (execx.Result, error) {
	client := `mariadb() {
    [ "$MYSQL_PWD" = "$MARIADB_ROOT_PASSWORD" ] || return 91
    [ "$#" = 7 ] || return 92
    [ "$1" = --protocol=socket ] || return 93
    [ "$2" = --connect-timeout=1 ] || return 94
    [ "$3" = -uroot ] || return 95
    [ "$4" = --batch ] || return 96
    [ "$5" = --skip-column-names ] || return 97
    [ "$6" = -e ] || return 98
    [ "$7" = 'SELECT @@skip_networking' ] || return 99
    case "$DATABASE_TEST_MODE" in
        authentication) printf '%s\n' 'authentication rejected' >&2; return 9 ;;
        temporary) printf '%s\n' 1 ;;
        ready) printf '%s\n' 0 ;;
        unexpected) printf '%s\n' unavailable ;;
    esac
}
`
	command := exec.CommandContext(ctx, shell, "-c", client+script)
	command.Env = append(os.Environ(), "DATABASE_TEST_MODE="+mode, "MARIADB_ROOT_PASSWORD=fake-readiness-password")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	result := execx.Result{Stdout: stdout.String(), Stderr: stderr.String()}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	return result, err
}

// TestDevMySQLPreparationWaitsForAuthenticatedPermanentServer proves readiness failures cannot run provisioning early or twice.
func TestDevMySQLPreparationWaitsForAuthenticatedPermanentServer(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	probes, provisions := 0, 0
	run := func(ctx context.Context, _ string, _ []string, input string) (execx.Result, error) {
		if input == devMySQLReadinessScript {
			mode := []string{"authentication", "temporary", "ready"}[probes]
			probes++
			return runDevMySQLReadinessFixture(ctx, shell, mode, input)
		}
		provisions++
		if probes != 3 || input != devMySQLPreparationScript {
			t.Fatalf("provisioning preceded authenticated readiness: probes=%d", probes)
		}
		return execx.Result{}, nil
	}
	if _, err := prepareDevDatabase(t.Context(), "mysql", []string{"db"}, run); err != nil {
		t.Fatal(err)
	}
	if probes != 3 || provisions != 1 {
		t.Fatalf("probes=%d provisions=%d", probes, provisions)
	}
}

// TestDevMySQLPreparationCancellationDuringAuthentication preserves cancellation and forbids provisioning after a rejected probe.
func TestDevMySQLPreparationCancellationDuringAuthentication(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	_, err := prepareDevDatabase(ctx, "mysql", []string{"db"}, func(_ context.Context, _ string, _ []string, input string) (execx.Result, error) {
		calls++
		if input != devMySQLReadinessScript {
			t.Fatal("provisioned after authentication rejection")
		}
		cancel()
		return execx.Result{ExitCode: 9, Stderr: "authentication rejected"}, nil
	})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "authentication rejected") || calls != 1 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}

package forj

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/project"
)

// TestMySQLPreparationUsesSocketAndFailsVisibly executes the actual shell scripts against controlled client outcomes.
func TestMySQLPreparationUsesSocketAndFailsVisibly(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	for _, test := range []struct {
		name, mode, script string
		wantFailure        bool
	}{
		{name: "generated task", script: generatedMySQLDevWaitCommand},
		{name: "multiple databases", script: mysqlCreateDatabasesScript([]string{"billing", "db"})},
		{name: "unavailable socket", mode: "unavailable", script: mysqlCreateDatabasesScript([]string{"db"}), wantFailure: true},
		{name: "authentication failure", mode: "authentication", script: mysqlCreateDatabasesScript([]string{"db"}), wantFailure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			clients := map[string]string{
				"docker-compose": `shift 3; exec "$@"`,
				"sleep":          `exit 0`,
				"mariadb-admin": `case " $* " in *" --protocol=socket "*) ;; *) exit 7;; esac
printf 'probe\n' >> "$MYSQL_TEST_PROBES"
if [ "$MYSQL_TEST_MODE" = unavailable ]; then echo 'socket unavailable' >&2; exit 1; fi`,
				"mariadb": `case " $* " in *" --protocol=socket "*) ;; *) exit 7;; esac
if [ "$MYSQL_TEST_MODE" = authentication ]; then echo 'authentication rejected' >&2; exit 9; fi
printf '%s\n' "$@" >> "$MYSQL_TEST_QUERIES"`,
			}
			for name, body := range clients {
				if err := os.WriteFile(filepath.Join(root, name), []byte("#!"+shell+"\n"+body+"\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, shell, "-c", test.script)
			command.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "MYSQL_TEST_MODE="+test.mode,
				"MYSQL_TEST_PROBES="+filepath.Join(root, "probes"), "MYSQL_TEST_QUERIES="+filepath.Join(root, "queries"),
				"MARIADB_DATABASE=app", "MARIADB_USER=app", "MARIADB_ROOT_PASSWORD=test-password")
			output, err := command.CombinedOutput()
			if (err != nil) != test.wantFailure {
				t.Fatalf("script error = %v, output = %s", err, output)
			}
			if ctx.Err() != nil {
				t.Fatal("readiness loop did not terminate")
			}
			queries, _ := os.ReadFile(filepath.Join(root, "queries"))
			switch test.mode {
			case "unavailable":
				probes, err := os.ReadFile(filepath.Join(root, "probes"))
				if err != nil || strings.Count(string(probes), "probe\n") != 61 || len(queries) != 0 || !strings.Contains(string(output), "60 attempts") || !strings.Contains(string(output), "socket unavailable") {
					t.Fatalf("unavailable result: probes=%q queries=%q output=%s error=%v", probes, queries, output, err)
				}
			case "authentication":
				if len(queries) != 0 || !strings.Contains(string(output), "authentication rejected") {
					t.Fatalf("authentication failure was hidden: queries=%q output=%s", queries, output)
				}
			default:
				if !strings.Contains(string(queries), "CREATE DATABASE IF NOT EXISTS `") || strings.Contains(string(queries), "\\`") {
					t.Fatalf("SQL arguments = %s", queries)
				}
			}
		})
	}
}

// TestLegacyMySQLReadinessUpgradePreservesCustomTasks upgrades only exact framework-owned bootstrap commands.
func TestLegacyMySQLReadinessUpgradePreservesCustomTasks(t *testing.T) {
	for _, test := range []struct {
		name, command string
		upgrade       bool
	}{
		{name: "Waiting for Database to be ready", command: legacyGeneratedMySQLDevWaitCommand, upgrade: true},
		{name: "Waiting for Database to be ready", command: strings.ReplaceAll(legacyGeneratedMySQLDevWaitCommand, "mariadb", "mysql"), upgrade: true},
		{name: "Waiting for Database to be ready", command: strings.Replace(legacyGeneratedMySQLDevWaitCommand, "docker-compose", "docker compose", 1), upgrade: true},
		{name: "Owner readiness", command: legacyGeneratedMySQLDevWaitCommand},
		{name: "Waiting for Database to be ready", command: legacyGeneratedMySQLDevWaitCommand + " && echo owner-customized"},
	} {
		t.Run(test.name+test.command, func(t *testing.T) {
			t.Chdir(t.TempDir())
			config := &project.Config{}
			config.Render.Components.Docker = true
			config.Dev.Pre = []project.DevTask{{Name: test.name, Cmd: test.command}}
			got := effectiveDevPreTasksWithLegacy(config, true)
			want := test.command
			if test.upgrade {
				want = generatedMySQLDevWaitCommand
			}
			if len(got) == 0 || got[0].Cmd != want || config.Dev.Pre[0].Cmd != test.command {
				t.Fatalf("resolved tasks = %#v, want first command %q and unchanged source", got, want)
			}
		})
	}
}

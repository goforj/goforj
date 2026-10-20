//go:build linux && integration_mysql_readiness

package forj

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/execx"
)

const nativeMySQLReadinessImage = "mariadb:12.3.3@sha256:dd9b303aed4f4890ed09f766d8ca9ddfd176c0c6f6267feff53b3192ec65a979"

// TestNativeMySQLAuthenticatedReadiness proves fresh concurrent initializations cannot admit the temporary socket server.
func TestNativeMySQLAuthenticatedReadiness(t *testing.T) {
	if os.Getenv("GOFORJ_NATIVE_MYSQL_READINESS") != "1" {
		t.Skip("set GOFORJ_NATIVE_MYSQL_READINESS=1 on a disposable Docker worker")
	}
	for index := 0; index < 3; index++ {
		t.Run(fmt.Sprintf("fresh-%d", index+1), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			directory := t.TempDir()
			if err := os.Chmod(directory, 0o755); err != nil {
				t.Fatal(err)
			}
			// Hold the image's init scripts after root setup so temporary-server rejection is deterministic.
			gate := "while [ ! -f /goforj-fixture/release ]; do sleep 0.1; done\n"
			if err := os.WriteFile(filepath.Join(directory, "hold.sh"), []byte(gate), 0o644); err != nil {
				t.Fatal(err)
			}
			name := fmt.Sprintf("goforj-mysql-readiness-%d-%d-%d", os.Getpid(), time.Now().UnixNano(), index)
			result, err := runNativeMySQLReadinessCommand(ctx, "", "docker", "run", "--detach", "--name", name,
				"--label", "goforj.fixture=mysql-authenticated-readiness", "--tmpfs", "/var/lib/mysql:rw",
				"--mount", "type=bind,source="+directory+",target=/goforj-fixture,readonly",
				"--mount", "type=bind,source="+filepath.Join(directory, "hold.sh")+",target=/docker-entrypoint-initdb.d/99-goforj-readiness.sh,readonly",
				"--env", "MARIADB_ROOT_PASSWORD", "--env", "MARIADB_USER", "--env", "MARIADB_PASSWORD", "--env", "MARIADB_DATABASE", nativeMySQLReadinessImage)
			if err != nil || !result.OK() {
				t.Fatalf("start fresh MariaDB: %v, %s", err, result.Stderr)
			}
			containerID := strings.TrimSpace(result.Stdout)
			if len(containerID) != 64 || strings.Trim(containerID, "0123456789abcdef") != "" {
				t.Fatalf("Docker did not return an exact container identity: %q", containerID)
			}
			t.Cleanup(func() {
				cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cleanupCancel()
				if t.Failed() {
					logs, _ := runNativeMySQLReadinessCommand(cleanupContext, "", "docker", "logs", "--tail", "100", containerID)
					t.Logf("owned MariaDB fixture logs:\n%s\n%s", logs.Stdout, logs.Stderr)
				}
				removed, removeErr := runNativeMySQLReadinessCommand(cleanupContext, "", "docker", "rm", "--force", "--volumes", containerID)
				if removeErr != nil || !removed.OK() {
					t.Errorf("remove exact MariaDB fixture: %v, %s", removeErr, removed.Stderr)
				}
			})
			probes, provisions := 0, 0
			temporaryRejected, oldPingAccepted := false, false
			run := func(ctx context.Context, service string, arguments []string, input string) (execx.Result, error) {
				if service != "mysql" {
					t.Fatalf("unexpected database service %q", service)
				}
				if input == devMySQLReadinessScript {
					probes++
				} else {
					provisions++
					if !temporaryRejected || !oldPingAccepted || input != devMySQLPreparationScript {
						t.Fatal("provisioning preceded authenticated permanent-server readiness")
					}
				}
				args := append([]string{"exec", "-i", containerID}, arguments...)
				result, err := runNativeMySQLReadinessCommand(ctx, input, "docker", args...)
				if input == devMySQLReadinessScript && !result.OK() && strings.Contains(result.Stderr, "initialization server is still active") {
					temporaryRejected = true
					ping, pingErr := runNativeMySQLReadinessCommand(ctx, "", "docker", "exec", containerID, "mariadb-admin", "ping", "--protocol=socket", "--connect-timeout=1")
					if pingErr != nil || !ping.OK() {
						t.Fatalf("old unauthenticated ping did not expose the temporary-server false positive: %v, %s", pingErr, ping.Stderr)
					}
					oldPingAccepted = true
					if writeErr := os.WriteFile(filepath.Join(directory, "release"), []byte("continue initialization\n"), 0o644); writeErr != nil {
						t.Fatal(writeErr)
					}
				}
				return result, err
			}
			started := time.Now()
			if _, err := prepareDevDatabase(ctx, "mysql", []string{"billing", "db"}, run); err != nil {
				t.Fatal(err)
			}
			if !temporaryRejected || !oldPingAccepted || provisions != 1 {
				t.Fatalf("temporary_rejected=%t old_ping_accepted=%t provisions=%d", temporaryRejected, oldPingAccepted, provisions)
			}
			query := "set -eu\nexport MYSQL_PWD=\"$MARIADB_ROOT_PASSWORD\"\nmariadb --protocol=socket -uroot --batch --skip-column-names -e \"SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME IN ('billing','db') ORDER BY SCHEMA_NAME\"\n"
			verified, verifyErr := runNativeMySQLReadinessCommand(ctx, query, "docker", "exec", "-i", containerID, "sh", "-s")
			if verifyErr != nil || !verified.OK() || strings.TrimSpace(verified.Stdout) != "billing\ndb" {
				t.Fatalf("verify provisioned databases: %v, stdout=%q stderr=%s", verifyErr, verified.Stdout, verified.Stderr)
			}
			t.Logf("image=%s temporary_server_rejected=true old_ping_false_positive=true authenticated_ready=true probes=%d provisioning_calls=%d databases=billing,db elapsed=%s", nativeMySQLReadinessImage, probes, provisions, time.Since(started))
		})
	}
}

// runNativeMySQLReadinessCommand keeps fixture passwords in one subprocess environment and returns native exit status.
func runNativeMySQLReadinessCommand(ctx context.Context, input, executable string, arguments ...string) (execx.Result, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Env = append(os.Environ(), "MARIADB_ROOT_PASSWORD=fixture-root-password", "MARIADB_USER=fixture_user", "MARIADB_PASSWORD=fixture-user-password", "MARIADB_DATABASE=db")
	command.Stdin = strings.NewReader(input)
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

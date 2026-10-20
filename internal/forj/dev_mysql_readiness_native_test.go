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

// nativeMySQLReadinessFixture pins each server generation and its official image conventions.
type nativeMySQLReadinessFixture struct {
	vendor, version, image, client, environment string
}

// TestNativeMySQLAuthenticatedReadiness exercises legacy and current servers through fresh, concurrent initialization.
func TestNativeMySQLAuthenticatedReadiness(t *testing.T) {
	if os.Getenv("GOFORJ_NATIVE_MYSQL_READINESS") != "1" {
		t.Skip("set GOFORJ_NATIVE_MYSQL_READINESS=1 on a disposable Docker worker")
	}
	fixtures := []nativeMySQLReadinessFixture{
		{"mariadb", "10.5", "mariadb:10.5@sha256:a530aeeefd82f4fa5150f391b6c75462140904780338766f6b03acecb1cca3ce", "mariadb", "MYSQL"},
		{"mariadb", "10.6", "mariadb:10.6.28@sha256:88ac718dafee1e8fb415233c04f314f3c3866af99177ce25d436106c62090461", "mariadb", "MARIADB"},
		{"mariadb", "10.11", "mariadb:10.11.19@sha256:0d0945c57f75b3a3b9ebe71aedaaad005fa5305054e8fca3d2e42a8c92b43b5e", "mariadb", "MARIADB"},
		{"mariadb", "11.4", "mariadb:11.4.13@sha256:250e1609e93c0e6aecf64bfbab2d84d5e89d2481540fe77447ff6c855c3b1873", "mariadb", "MARIADB"},
		{"mariadb", "11.8", "mariadb:11.8.9@sha256:f16d7a344413e519dff518f27bdb221388f84b0d274c97c4fb5f2cb0dc1b34fa", "mariadb", "MARIADB"},
		{"mariadb", "12.3", "mariadb:12.3.3@sha256:dd9b303aed4f4890ed09f766d8ca9ddfd176c0c6f6267feff53b3192ec65a979", "mariadb", "MARIADB"},
		{"mariadb", "13.0", "mariadb:13.0.2@sha256:0e4bb0f339feba125d1154fbacd36c16ec8518f52e44e1f82ecec7d6771caf39", "mariadb", "MARIADB"},
		{"mysql", "5.7", "mysql:5.7@sha256:4bc6bc963e6d8443453676cae56536f4b8156d78bae03c0145cbe47c2aad73bb", "mysql", "MYSQL"},
		{"mysql", "8.0", "mysql:8.0@sha256:7dcddc01f13bab2f15cde676d44d01f61fc9f99fe7785e86196dfc07d358ae2b", "mysql", "MYSQL"},
		{"mysql", "8.4", "mysql:8.4.11@sha256:0f51e63e537a1f176423fa03ef559aabe6fa4372a53160a44bc7e75da7d15364", "mysql", "MYSQL"},
		{"mysql", "9.7", "mysql:9.7.2@sha256:e2fa358a74ac81148e7e27d9433e4c0ab62133de8ea4428b4f39a51a4ba7d07f", "mysql", "MYSQL"},
		{"mysql", "26.7", "mysql:26.7.0@sha256:625f74ddf93652d9d0f8d4ea7d741b9189554a5e96d27bb770abd0902d89c6e3", "mysql", "MYSQL"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.vendor+"-"+fixture.version, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			// Pulls are outside the production readiness budget, just as Compose pulls before preparing databases.
			pulled, err := runNativeMySQLReadinessCommand(ctx, "", "docker", "pull", fixture.image)
			if err != nil || !pulled.OK() {
				t.Fatalf("pull pinned database image: %v, %s", err, pulled.Stderr)
			}
			directory := t.TempDir()
			if err := os.Chmod(directory, 0o755); err != nil {
				t.Fatal(err)
			}
			// Hold init scripts after root setup so temporary-server rejection is deterministic on every version.
			gate := "while [ ! -f /goforj-fixture/release ]; do sleep 0.1; done\n"
			if err := os.WriteFile(filepath.Join(directory, "hold.sh"), []byte(gate), 0o644); err != nil {
				t.Fatal(err)
			}
			name := fmt.Sprintf("goforj-mysql-readiness-%d-%d", os.Getpid(), time.Now().UnixNano())
			arguments := []string{"run", "--detach", "--name", name,
				"--label", "goforj.fixture=mysql-authenticated-readiness", "--tmpfs", "/var/lib/mysql:rw",
				"--mount", "type=bind,source=" + directory + ",target=/goforj-fixture,readonly",
				"--mount", "type=bind,source=" + filepath.Join(directory, "hold.sh") + ",target=/docker-entrypoint-initdb.d/99-goforj-readiness.sh,readonly"}
			for _, variable := range []string{"ROOT_PASSWORD", "USER", "PASSWORD", "DATABASE"} {
				arguments = append(arguments, "--env", fixture.environment+"_"+variable)
			}
			arguments = append(arguments, fixture.image)
			result, err := runNativeMySQLReadinessCommand(ctx, "", "docker", arguments...)
			if err != nil || !result.OK() {
				t.Fatalf("start fresh database: %v, %s", err, result.Stderr)
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
					t.Logf("owned database fixture logs:\n%s\n%s", logs.Stdout, logs.Stderr)
				}
				removed, removeErr := runNativeMySQLReadinessCommand(cleanupContext, "", "docker", "rm", "--force", "--volumes", containerID)
				if removeErr != nil || !removed.OK() {
					t.Errorf("remove exact database fixture: %v, %s", removeErr, removed.Stderr)
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
					admin := "mysqladmin"
					if fixture.client == "mariadb" {
						admin = "mariadb-admin"
					}
					ping, pingErr := runNativeMySQLReadinessCommand(ctx, "", "docker", "exec", containerID, admin, "ping", "--protocol=socket", "--connect-timeout=1")
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
			query := "set -eu\nexport MYSQL_PWD=\"$" + fixture.environment + "_ROOT_PASSWORD\"\n" + fixture.client + " --protocol=socket -uroot --batch --skip-column-names -e \"SELECT VERSION(); SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME IN ('billing','db') ORDER BY SCHEMA_NAME\"\n"
			verified, verifyErr := runNativeMySQLReadinessCommand(ctx, query, "docker", "exec", "-i", containerID, "sh", "-s")
			version, databases, _ := strings.Cut(strings.TrimSpace(verified.Stdout), "\n")
			if verifyErr != nil || !verified.OK() || !strings.HasPrefix(version, fixture.version+".") || databases != "billing\ndb" {
				t.Fatalf("verify server version and provisioned databases: %v, stdout=%q stderr=%s", verifyErr, verified.Stdout, verified.Stderr)
			}
			query = "set -eu\nexport MYSQL_PWD=\"$" + fixture.environment + "_PASSWORD\"\n" + fixture.client + " --protocol=socket -ufixture_user --batch --skip-column-names -e \"CREATE TABLE billing.probe (value VARCHAR(16)); INSERT INTO billing.probe VALUES ('verified'); SELECT value FROM billing.probe; CREATE TABLE db.probe (value VARCHAR(16)); INSERT INTO db.probe VALUES ('verified'); SELECT value FROM db.probe;\"\n"
			grants, grantErr := runNativeMySQLReadinessCommand(ctx, query, "docker", "exec", "-i", containerID, "sh", "-s")
			if grantErr != nil || !grants.OK() || strings.TrimSpace(grants.Stdout) != "verified\nverified" {
				t.Fatalf("verify application user can create, write and read both databases: %v, stdout=%q stderr=%s", grantErr, grants.Stdout, grants.Stderr)
			}
			t.Logf("version=%s image=%s temporary_server_rejected=true old_ping_false_positive=true authenticated_ready=true probes=%d provisioning_calls=%d databases=billing,db application_grants_verified=true elapsed=%s", version, fixture.image, probes, provisions, time.Since(started))
		})
	}
}

// runNativeMySQLReadinessCommand keeps fixture passwords in one subprocess environment and returns native exit status.
func runNativeMySQLReadinessCommand(ctx context.Context, input, executable string, arguments ...string) (execx.Result, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Env = os.Environ()
	for _, prefix := range []string{"MARIADB", "MYSQL"} {
		command.Env = append(command.Env, prefix+"_ROOT_PASSWORD=fixture-root-password", prefix+"_USER=fixture_user", prefix+"_PASSWORD=fixture-user-password", prefix+"_DATABASE=db")
	}
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

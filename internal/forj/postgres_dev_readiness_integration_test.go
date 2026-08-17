//go:build integration

package forj

import (
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestPostgresCreateDatabasesScriptAuthenticates verifies the generated pre-dev script can create a database over password-authenticated TCP.
func TestPostgresCreateDatabasesScriptAuthenticates(t *testing.T) {
	started, err := testkit.StartTestcontainer(
		t.Logf,
		testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "app",
				"POSTGRES_USER":     "postgres",
				"POSTGRES_PASSWORD": `postgres test '$dollar" password`,
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithStartupTimeout(60 * time.Second),
		},
		"5432/tcp",
		60*time.Second,
		"Postgres pre-dev readiness",
	)
	if err != nil {
		t.Fatalf("start Postgres: %v", err)
	}
	defer started.Stop()

	// The generated Compose service resolves as postgres; loopback exercises the same TCP password exchange inside this isolated container.
	script := strings.ReplaceAll(postgresCreateDatabasesScript([]string{"app", "reporting"}), `"postgres"`, `"127.0.0.1"`)
	if err := testkit.WaitForContainerExecSuccess(
		started.Container,
		[]string{"sh", "-lc", script},
		20*time.Second,
	); err != nil {
		t.Fatalf("run authenticated Postgres pre-dev script: %v\n%s", err, script)
	}
}

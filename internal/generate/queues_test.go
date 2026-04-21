package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateQueueFilesSupportsDefaultAndNamedAccessors(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRIVER", "sync")
	t.Setenv("QUEUE_CRITICAL_DEFAULT_QUEUE", "critical")

	root := mustTempGeneratedModuleRoot(t, ".tmp-queue-generation-*", filepath.Join("internal", "queues"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/queuegenerationtest",
		[]string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/str"},
		nil,
		queueLocalReplaces(t),
	))
	written, err := GenerateQueueFiles(root)
	if err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated queue files to be written")
	}

	queuesGen, err := os.ReadFile(filepath.Join(root, "internal", "queues", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	for _, snippet := range []string{
		"func (m *Manager) Critical()",
		`Name: "queue_critical"`,
		`func (m *Manager) ReadinessChecks() []ReadinessCheck`,
	} {
		if !strings.Contains(string(queuesGen), snippet) {
			t.Fatalf("expected generated accessors to contain %q", snippet)
		}
	}

	testSource := `package queues

import "testing"

func TestGeneratedAccessors(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRIVER", "sync")
	t.Setenv("QUEUE_CRITICAL_DEFAULT_QUEUE", "critical")

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if got := mgr.Default().Driver(); got != "null" {
		t.Fatalf("Default driver = %q, want %q", got, "null")
	}
	if got := mgr.Critical().Driver(); got != "sync" {
		t.Fatalf("Critical driver = %q, want %q", got, "sync")
	}

	checks := mgr.ReadinessChecks()
	if len(checks) != 2 {
		t.Fatalf("len(ReadinessChecks()) = %d, want 2", len(checks))
	}
	for _, check := range checks {
		if err := check.Check(t.Context()); err != nil {
			t.Fatalf("readiness check %s returned error: %v", check.Name, err)
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "queues", "generated_accessors_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/queues", "TestGeneratedAccessors", nil)
}

func TestGenerateQueueFilesUsesSupportedDriverImports(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "workerpool")
	t.Setenv("QUEUE_SUPPORTED_DRIVERS", "workerpool,redis")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "queues"), 0o755); err != nil {
		t.Fatalf("mkdir queue package: %v", err)
	}

	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/queuesupportedtest",
		[]string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/queue/driver/redisqueue", "github.com/goforj/str"},
		nil,
		queueLocalReplaces(t),
	))

	if _, err := GenerateQueueFiles(root); err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "queues", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source := string(managerGen)
	if !strings.Contains(source, `"github.com/goforj/queue/driver/redisqueue"`) {
		t.Fatal("expected generated queue manager to import redisqueue from QUEUE_SUPPORTED_DRIVERS")
	}
	if !strings.Contains(source, `case driverRedis:`) {
		t.Fatal("expected generated queue manager to include redis case from QUEUE_SUPPORTED_DRIVERS")
	}
}

func TestGenerateQueueFilesDerivesAccessorNamesFromQueueNames(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_WORKER_DRIVER", "sync")
	t.Setenv("QUEUE_BULK_IMPORT_DRIVER", "workerpool")

	root := mustTempGeneratedModuleRoot(t, ".tmp-queue-accessor-names-*", filepath.Join("internal", "queues"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/queueaccessornametest",
		[]string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/str"},
		nil,
		nil,
	))
	written, err := GenerateQueueFiles(root)
	if err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated queue files to be written")
	}

	queuesGen, err := os.ReadFile(filepath.Join(root, "internal", "queues", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	for _, snippet := range []string{
		"func (m *Manager) CriticalWorker()",
		"func (m *Manager) BulkImport()",
	} {
		if !strings.Contains(string(queuesGen), snippet) {
			t.Fatalf("expected generated accessor to contain %q", snippet)
		}
	}
}

func TestGenerateQueueFilesRejectsUnknownEnvVars(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRVIER", "redis")

	_, err := GenerateQueueFiles(t.TempDir())
	if err == nil {
		t.Fatal("expected GenerateQueueFiles to reject unknown queue env vars")
	}
	if !strings.Contains(err.Error(), "QUEUE_CRITICAL_DRVIER") {
		t.Fatalf("expected error to mention unknown env var, got: %v", err)
	}
}

func TestGenerateQueueFilesRejectsWrongDriverEnvVars(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRIVER", "sync")
	t.Setenv("QUEUE_CRITICAL_URL", "nats://127.0.0.1:4222")

	_, err := GenerateQueueFiles(t.TempDir())
	if err == nil {
		t.Fatal("expected GenerateQueueFiles to reject wrong-driver queue env vars")
	}
	if !strings.Contains(err.Error(), "QUEUE_CRITICAL_URL") {
		t.Fatalf("expected error to mention wrong-driver env var, got: %v", err)
	}
	if !strings.Contains(err.Error(), `driver "sync"`) {
		t.Fatalf("expected error to mention sync driver, got: %v", err)
	}
}

func TestGenerateQueueFilesAllowsShutdownTimeoutEnvVar(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "workerpool")
	t.Setenv("QUEUE_SHUTDOWN_TIMEOUT", "90s")

	if _, err := GenerateQueueFiles(t.TempDir()); err != nil {
		t.Fatalf("expected GenerateQueueFiles to allow QUEUE_SHUTDOWN_TIMEOUT, got %v", err)
	}
}

func TestGenerateQueueFilesAllowsInactiveRootDriverEnvVars(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_SERVER_LOG_LEVEL", "error")
	t.Setenv("QUEUE_QUEUES", "critical=6,default=3,low=1")

	if _, err := GenerateQueueFiles(t.TempDir()); err != nil {
		t.Fatalf("expected GenerateQueueFiles to allow documented inactive root queue env vars, got %v", err)
	}
}

func TestGenerateQueueFilesRedisIncludesShutdownTimeout(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "redis")
	t.Setenv("QUEUE_ADDR", "127.0.0.1:6379")
	t.Setenv("QUEUE_SHUTDOWN_TIMEOUT", "7s")

	root := t.TempDir()
	if _, err := GenerateQueueFiles(root); err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "queues", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}

	source := string(managerGen)
	if !strings.Contains(source, `ShutdownTimeout:`) || !strings.Contains(source, `scope.GetDuration("SHUTDOWN_TIMEOUT", "10s")`) {
		t.Fatalf("expected generated redis queue config to include shutdown timeout passthrough, got:\n%s", string(managerGen))
	}
}

func TestGenerateQueueFilesAlwaysIncludesNativeDrivers(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "redis")
	t.Setenv("QUEUE_ADDR", "127.0.0.1:6379")

	root := mustTempGeneratedModuleRoot(t, ".tmp-queue-native-drivers-*", filepath.Join("internal", "queues"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/queuenativedriverstest",
		[]string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/queue/driver/redisqueue", "github.com/goforj/str"},
		nil,
		nil,
	))

	if _, err := GenerateQueueFiles(root); err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "queues", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}

	for _, snippet := range []string{
		"case driverNull:",
		"case driverSync:",
		"case driverWorkerpool:",
		"queue.DriverNull",
		"queue.DriverSync",
		"queue.DriverWorkerpool",
	} {
		if !strings.Contains(string(managerGen), snippet) {
			t.Fatalf("expected generated queue manager to contain %q", snippet)
		}
	}
}

func TestGenerateQueueFilesAddsDriverImportsToGoMod(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_SYNC_DRIVER", "sync")
	t.Setenv("QUEUE_WORKERPOOL_DRIVER", "workerpool")
	t.Setenv("QUEUE_REDIS_DRIVER", "redis")
	t.Setenv("QUEUE_REDIS_DEFAULT_QUEUE", "critical")
	t.Setenv("QUEUE_REDIS_QUEUES", "critical=1")
	t.Setenv("QUEUE_NATS_DRIVER", "nats")
	t.Setenv("QUEUE_NATS_URL", "nats://127.0.0.1:4222")
	t.Setenv("QUEUE_SQS_DRIVER", "sqs")
	t.Setenv("QUEUE_SQS_REGION", "us-east-1")
	t.Setenv("QUEUE_SQS_ENDPOINT", "http://127.0.0.1:4566")
	t.Setenv("QUEUE_SQS_ACCESS_KEY", "test")
	t.Setenv("QUEUE_SQS_SECRET_KEY", "test")
	t.Setenv("QUEUE_RABBITMQ_DRIVER", "rabbitmq")
	t.Setenv("QUEUE_RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672/")
	t.Setenv("QUEUE_SQLITE_DRIVER", "sqlite")
	t.Setenv("QUEUE_SQLITE_DSN", "file::memory:?cache=shared")
	t.Setenv("QUEUE_POSTGRES_DRIVER", "postgres")
	t.Setenv("QUEUE_POSTGRES_DSN", "postgres://queue:queue@127.0.0.1:5432/queue?sslmode=disable")
	t.Setenv("QUEUE_MYSQL_DRIVER", "mysql")
	t.Setenv("QUEUE_MYSQL_DSN", "queue:queue@tcp(127.0.0.1:3306)/queue?parseTime=true")

	root := mustTempGeneratedModuleRoot(t, ".tmp-queue-driver-imports-*", filepath.Join("internal", "queues"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/queueimporttest",
		[]string{
			"github.com/goforj/env/v2",
			"github.com/goforj/queue",
			"github.com/goforj/queue/driver/mysqlqueue",
			"github.com/goforj/queue/driver/natsqueue",
			"github.com/goforj/queue/driver/postgresqueue",
			"github.com/goforj/queue/driver/rabbitmqqueue",
			"github.com/goforj/queue/driver/redisqueue",
			"github.com/goforj/queue/driver/sqlitequeue",
			"github.com/goforj/queue/driver/sqlqueuecore",
			"github.com/goforj/queue/driver/sqsqueue",
			"github.com/goforj/str",
		},
		nil,
		queueLocalReplaces(t),
	))
	written, err := GenerateQueueFiles(root)
	if err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated queue files to be written")
	}

	configGenPath := filepath.Join(root, "internal", "queues", "manager_gen.go")
	configGen, err := os.ReadFile(configGenPath)
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	for _, importPath := range []string{
		`"github.com/goforj/queue/driver/redisqueue"`,
		`"github.com/goforj/queue/driver/natsqueue"`,
		`"github.com/goforj/queue/driver/sqsqueue"`,
		`"github.com/goforj/queue/driver/rabbitmqqueue"`,
		`"github.com/goforj/queue/driver/sqlitequeue"`,
		`"github.com/goforj/queue/driver/postgresqueue"`,
		`"github.com/goforj/queue/driver/mysqlqueue"`,
	} {
		if !strings.Contains(string(configGen), importPath) {
			t.Fatalf("expected manager_gen.go to import %s", importPath)
		}
	}

	runFixtureGoModTidy(t, root, nil)
	for _, module := range []string{
		"github.com/goforj/queue/driver/redisqueue",
		"github.com/goforj/queue/driver/natsqueue",
		"github.com/goforj/queue/driver/sqsqueue",
		"github.com/goforj/queue/driver/rabbitmqqueue",
		"github.com/goforj/queue/driver/sqlitequeue",
		"github.com/goforj/queue/driver/postgresqueue",
		"github.com/goforj/queue/driver/mysqlqueue",
	} {
		assertFixtureGoModContains(t, root, module)
	}
	runFixtureGoTest(t, root, "./internal/queues", "TestDoesNotExist", nil)
}

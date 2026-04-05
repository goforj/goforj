package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateQueueFilesSupportsDefaultAndNamedAccessors(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRIVER", "sync")
	t.Setenv("QUEUE_CRITICAL_DEFAULT_QUEUE", "critical")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-queue-generation-*")
	if err != nil {
		t.Fatalf("mkdir temp generation root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "queues"), 0o755); err != nil {
		t.Fatalf("mkdir queue package: %v", err)
	}

	goMod := `module example.com/queuegenerationtest

go 1.24

require (
	github.com/goforj/env/v2 v2.3.1
	github.com/goforj/queue v0.1.6
	github.com/goforj/str v1.2.0
)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
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

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	tidy.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err := tidy.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	goTest := exec.Command("go", "test", "./internal/queues", "-run", "TestGeneratedAccessors", "-count=1")
	goTest.Dir = root
	goTest.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err = goTest.CombinedOutput()
	if err != nil {
		t.Fatalf("generated queue package test failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestGenerateQueueFilesUsesSupportedDriverImports(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "workerpool")
	t.Setenv("QUEUE_SUPPORTED_DRIVERS", "workerpool,redis")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "queues"), 0o755); err != nil {
		t.Fatalf("mkdir queue package: %v", err)
	}

	goMod := `module example.com/queuesupportedtest

go 1.24

require (
	github.com/goforj/env/v2 v2.3.1
	github.com/goforj/queue v0.1.6
	github.com/goforj/queue/driver/redisqueue v0.1.6
	github.com/goforj/str v1.2.0
)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

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

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-queue-accessor-names-*")
	if err != nil {
		t.Fatalf("mkdir temp generation root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "queues"), 0o755); err != nil {
		t.Fatalf("mkdir queue package: %v", err)
	}

	goMod := `module example.com/queueaccessornametest

go 1.24

require (
	github.com/goforj/env/v2 v2.3.1
	github.com/goforj/queue v0.1.6
	github.com/goforj/str v1.2.0
)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
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

func TestGenerateQueueFilesAlwaysIncludesNativeDrivers(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "redis")
	t.Setenv("QUEUE_ADDR", "127.0.0.1:6379")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-queue-native-drivers-*")
	if err != nil {
		t.Fatalf("mkdir temp module root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "queues"), 0o755); err != nil {
		t.Fatalf("mkdir queue package: %v", err)
	}

	goMod := `module example.com/queuenativedriverstest

go 1.24

require (
	github.com/goforj/env/v2 v2.3.1
	github.com/goforj/queue v0.1.6
	github.com/goforj/queue/driver/redisqueue v0.1.6
	github.com/goforj/str v1.2.0
)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

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

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-queue-driver-imports-*")
	if err != nil {
		t.Fatalf("mkdir temp module root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "queues"), 0o755); err != nil {
		t.Fatalf("mkdir queue package: %v", err)
	}

	goMod := `module example.com/queueimporttest

go 1.24

require (
	github.com/goforj/env/v2 v2.3.1
	github.com/goforj/queue v0.1.6
	github.com/goforj/queue/driver/mysqlqueue v0.1.6
	github.com/goforj/queue/driver/natsqueue v0.1.6
	github.com/goforj/queue/driver/postgresqueue v0.1.6
	github.com/goforj/queue/driver/rabbitmqqueue v0.1.6
	github.com/goforj/queue/driver/redisqueue v0.1.6
	github.com/goforj/queue/driver/sqlitequeue v0.1.6
	github.com/goforj/queue/driver/sqlqueuecore v0.1.6
	github.com/goforj/queue/driver/sqsqueue v0.1.6
	github.com/goforj/str v1.2.0
)
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
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

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	tidy.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err := tidy.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}

	goModAfter, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod after tidy: %v", err)
	}
	for _, module := range []string{
		"github.com/goforj/queue/driver/redisqueue",
		"github.com/goforj/queue/driver/natsqueue",
		"github.com/goforj/queue/driver/sqsqueue",
		"github.com/goforj/queue/driver/rabbitmqqueue",
		"github.com/goforj/queue/driver/sqlitequeue",
		"github.com/goforj/queue/driver/postgresqueue",
		"github.com/goforj/queue/driver/mysqlqueue",
	} {
		if !strings.Contains(string(goModAfter), module) {
			t.Fatalf("expected go.mod to contain %s after tidy", module)
		}
	}

	goTest := exec.Command("go", "test", "./internal/queues", "-run", "TestDoesNotExist", "-count=1")
	goTest.Dir = root
	goTest.Env = append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	output, err = goTest.CombinedOutput()
	if err != nil {
		t.Fatalf("generated queue package compile failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

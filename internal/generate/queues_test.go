package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeQueueRuntimeFixture(t *testing.T, root string) {
	t.Helper()
	runtimeDir := filepath.Join(root, "internal", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime package: %v", err)
	}
	const source = `package runtime

import (
	"context"
	"os"
	"strings"
)

type sourceKey struct{}

const SourceJobs = "jobs"

type AppInfo struct {
	Name string
}

func CurrentApp() AppInfo {
	name := strings.TrimSpace(os.Getenv("FORJ_APP"))
	if name == "" {
		name = "app"
	}
	return AppInfo{Name: name}
}

func WithSource(ctx context.Context, source string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sourceKey{}, source)
}
`
	if err := os.WriteFile(filepath.Join(runtimeDir, "source.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("write runtime source fixture: %v", err)
	}

	inspectsDir := filepath.Join(root, "internal", "inspects")
	if err := os.MkdirAll(inspectsDir, 0o755); err != nil {
		t.Fatalf("mkdir inspects package: %v", err)
	}
	const inspectsSource = `package inspects

import "context"

type Manager struct{}

type InspectEvent struct {
	Kind       string
	Name       string
	Message    string
	Attributes map[string]any
}

type Recorder interface {
	RecordEvent(InspectEvent)
}

func RecorderFromContext(context.Context) Recorder { return nil }

func (m *Manager) Begin(ctx context.Context, _ string, _ string, _ map[string]string) context.Context {
	return ctx
}

func (m *Manager) Finish(context.Context, string, error) {}
`
	if err := os.WriteFile(filepath.Join(inspectsDir, "manager.go"), []byte(inspectsSource), 0o644); err != nil {
		t.Fatalf("write inspects source fixture: %v", err)
	}
}

func writeQueueFixtureModule(t *testing.T, root, moduleName string, requires []string, replaces []fixtureReplace) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "internal", "queues"), 0o755); err != nil {
		t.Fatalf("mkdir queue package: %v", err)
	}
	writeFixtureGoMod(t, root, fixtureModuleSpec(moduleName, requires, nil, replaces))
	writeQueueRuntimeFixture(t, root)
}

func TestGenerateQueueFilesSupportsDefaultAndNamedAccessors(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRIVER", "sync")
	t.Setenv("QUEUE_CRITICAL_NAME", "critical")

	root := mustTempGeneratedModuleRoot(t, ".tmp-queue-generation-*", filepath.Join("internal", "queues"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/queuegenerationtest",
		[]string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/str"},
		nil,
		queueLocalReplaces(t),
	))
	writeQueueRuntimeFixture(t, root)
	written, err := GenerateQueueFiles(root)
	if err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated queue files to be written")
	}

	queuesGen, err := os.ReadFile(filepath.Join(root, "internal", "queues", "accessors_gen.go"))
	if err != nil {
		t.Fatalf("read accessors_gen.go: %v", err)
	}
	for _, snippet := range []string{
		"func (m *Manager) Critical()",
		"func (m *Manager) Default()",
		"func (m *Manager) Instances() []Instance",
	} {
		if !strings.Contains(string(queuesGen), snippet) {
			t.Fatalf("expected generated accessors to contain %q", snippet)
		}
	}
	for _, snippet := range []string{
		"func (m *Manager) Register(",
		"func (m *Manager) Dispatch(",
		"func (m *Manager) WithContext(",
		"func recordJobPayload(",
	} {
		if strings.Contains(string(queuesGen), snippet) {
			t.Fatalf("expected generated accessors not to contain %q", snippet)
		}
	}
	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "queues", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	for _, snippet := range []string{
		`Name: "queue_critical"`,
		`func (m *Manager) ReadinessChecks() []ReadinessCheck`,
		"func (m *Manager) Register(",
		"func (m *Manager) Dispatch(",
		"func (m *Manager) WithContext(",
		"func recordJobPayload(",
	} {
		if !strings.Contains(string(managerGen), snippet) {
			t.Fatalf("expected generated manager to contain %q", snippet)
		}
	}

	testSource := `package queues

import (
	"context"
	"os"
	"testing"

	"github.com/goforj/env/v2"
	"github.com/goforj/queue"
)

func TestGeneratedAccessors(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRIVER", "sync")
	t.Setenv("QUEUE_CRITICAL_NAME", "production-critical-jobs")

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
	if got := mgr.Instances()[1].queueName; got != "production-critical-jobs" {
		t.Fatalf("critical instance queue name = %q, want production-critical-jobs", got)
	}
	if _, err := mgr.Dispatch(queue.NewJob("jobs:smoke")); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	handledCritical := false
	mgr.Register("jobs:critical", func(context.Context, queue.Message) error {
		handledCritical = true
		return nil
	})
	if err := mgr.Critical().StartWorkers(t.Context()); err != nil {
		t.Fatalf("start critical workers: %v", err)
	}
	t.Cleanup(func() {
		if err := mgr.Critical().Shutdown(t.Context()); err != nil {
			t.Errorf("shutdown critical workers: %v", err)
		}
	})
	if _, err := mgr.Dispatch(queue.NewJob("jobs:critical").OnQueue("critical")); err != nil {
		t.Fatalf("named Dispatch returned error: %v", err)
	}
	if !handledCritical {
		t.Fatal("named Dispatch did not use the registered critical queue runtime")
	}
	if got := mgr.defaultQueueName; got != "default" {
		t.Fatalf("default queue name = %q, want default", got)
	}
	queueScope := env.WithPrefix("QUEUE")
	if got := queueDefaultQueue("critical", queueScope.Child("CRITICAL"), queueScope); got != "production-critical-jobs" {
		t.Fatalf("critical queue name = %q, want production-critical-jobs", got)
	}

	t.Setenv("QUEUE_NAME", "production-default-jobs")
	queueEvents := make(chan queue.Event, 8)
	observedManager, err := NewManagerWithObserver(queue.ChannelObserver{Events: queueEvents}, nil, nil)
	if err != nil {
		t.Fatalf("NewManagerWithObserver returned error: %v", err)
	}
	if _, err := observedManager.Dispatch(queue.NewJob("jobs:default").OnQueue("default")); err != nil {
		t.Fatalf("explicit default Dispatch returned error: %v", err)
	}
	select {
	case event := <-queueEvents:
		if event.Queue != "production-default-jobs" {
			t.Fatalf("explicit default dispatched to %q, want production-default-jobs", event.Queue)
		}
	default:
		t.Fatal("explicit default Dispatch emitted no queue event")
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

func TestGeneratedQueueNamesUseCurrentApp(t *testing.T) {
	t.Setenv("FORJ_APP", "billing")
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRIVER", "sync")
	t.Setenv("QUEUE_CRITICAL_NAME", "critical")

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if got := mgr.defaultQueueName; got != "billing_default" {
		t.Fatalf("default queue name = %q, want billing_default", got)
	}
	queueScope := env.WithPrefix("QUEUE")
	if got := queueDefaultQueue("critical", queueScope.Child("CRITICAL"), queueScope); got != "billing_critical" {
		t.Fatalf("critical queue name = %q, want billing_critical", got)
	}
}

func TestGeneratedAccessorsFallbackWithoutRuntimeEnv(t *testing.T) {
	for _, key := range []string{
		"FORJ_APP",
		"QUEUE_DRIVER",
		"QUEUE_CRITICAL_DRIVER",
		"QUEUE_CRITICAL_NAME",
	} {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if mgr.Default() == nil {
		t.Fatal("expected default queue fallback")
	}
	if mgr.Critical() == nil {
		t.Fatal("expected critical queue fallback")
	}
	if got := mgr.Default().Driver(); got != "workerpool" {
		t.Fatalf("Default driver = %q, want workerpool", got)
	}
	if got := mgr.Critical().Driver(); got != "workerpool" {
		t.Fatalf("Critical driver = %q, want workerpool", got)
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "queues", "generated_accessors_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/queues", "TestGeneratedAccessors", nil)
}

func TestGenerateQueueFilesAcceptsDefaultQueueCompatibilityAlias(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRIVER", "sync")
	t.Setenv("QUEUE_CRITICAL_DEFAULT_QUEUE", "critical")

	root := mustTempGeneratedModuleRoot(t, ".tmp-queue-legacy-name-*", filepath.Join("internal", "queues"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/queuelegacytest",
		[]string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/str"},
		nil,
		queueLocalReplaces(t),
	))
	writeQueueRuntimeFixture(t, root)
	if _, err := GenerateQueueFiles(root); err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}

	testSource := `package queues

import "testing"

func TestLegacyDefaultQueueAlias(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_CRITICAL_DRIVER", "sync")
	t.Setenv("QUEUE_CRITICAL_DEFAULT_QUEUE", "critical")

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if got := mgr.Critical().Driver(); got != "sync" {
		t.Fatalf("Critical driver = %q, want %q", got, "sync")
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "queues", "legacy_default_queue_alias_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/queues", "TestLegacyDefaultQueueAlias", nil)
}

func TestGenerateQueueFilesNamedQueuesInheritRootConfig(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_WORKERS", "9")
	t.Setenv("QUEUE_REPORTS_WORKERS", "2")

	root := mustTempGeneratedModuleRoot(t, ".tmp-queue-inheritance-*", filepath.Join("internal", "queues"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/queueinheritancetest",
		[]string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/str"},
		nil,
		queueLocalReplaces(t),
	))
	writeQueueRuntimeFixture(t, root)
	if _, err := GenerateQueueFiles(root); err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}

	testSource := `package queues

import "testing"

func TestNamedQueueInheritsRootConfig(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_WORKERS", "9")
	t.Setenv("QUEUE_REPORTS_WORKERS", "2")

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if got := mgr.Reports().Driver(); got != "null" {
		t.Fatalf("Reports driver = %q, want %q", got, "null")
	}

	instances := mgr.Instances()
	if len(instances) != 2 {
		t.Fatalf("len(Instances()) = %d, want 2", len(instances))
	}
	if got := instances[1].Name; got != "reports" {
		t.Fatalf("named queue = %q, want %q", got, "reports")
	}
	if got := instances[0].Workers; got != 9 {
		t.Fatalf("default workers = %d, want 9", got)
	}
	if got := instances[1].Workers; got != 2 {
		t.Fatalf("reports workers = %d, want 2", got)
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "queues", "generated_inheritance_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/queues", "TestNamedQueueInheritsRootConfig", nil)
}

func TestGenerateQueueFilesUsesSupportedDriverImports(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "redis")
	t.Setenv("QUEUE_SUPPORTED_DRIVERS", "redis")

	root := t.TempDir()
	writeQueueFixtureModule(t, root,
		"example.com/queuesupportedtest",
		[]string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/queue/driver/redisqueue", "github.com/goforj/str"},
		queueLocalReplaces(t),
	)

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
	if !strings.Contains(source, `case driverWorkerpool:`) {
		t.Fatal("expected generated queue manager to keep workerpool as the no-env fallback")
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
	writeQueueRuntimeFixture(t, root)
	written, err := GenerateQueueFiles(root)
	if err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated queue files to be written")
	}

	queuesGen, err := os.ReadFile(filepath.Join(root, "internal", "queues", "accessors_gen.go"))
	if err != nil {
		t.Fatalf("read accessors_gen.go: %v", err)
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

	root := t.TempDir()
	writeQueueFixtureModule(t, root, "example.com/queueshutdowntest", []string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/str"}, nil)
	if _, err := GenerateQueueFiles(root); err != nil {
		t.Fatalf("expected GenerateQueueFiles to allow QUEUE_SHUTDOWN_TIMEOUT, got %v", err)
	}
}

func TestGenerateQueueFilesAllowsInactiveRootDriverEnvVars(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "null")
	t.Setenv("QUEUE_SERVER_LOG_LEVEL", "error")
	t.Setenv("QUEUE_QUEUES", "critical=6,default=3,low=1")

	root := t.TempDir()
	writeQueueFixtureModule(t, root, "example.com/queueinactiveroottest", []string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/str"}, nil)
	if _, err := GenerateQueueFiles(root); err != nil {
		t.Fatalf("expected GenerateQueueFiles to allow documented inactive root queue env vars, got %v", err)
	}
}

func TestGenerateQueueFilesRedisIncludesShutdownTimeout(t *testing.T) {
	t.Setenv("QUEUE_DRIVER", "redis")
	t.Setenv("QUEUE_ADDR", "127.0.0.1:6379")
	t.Setenv("QUEUE_SHUTDOWN_TIMEOUT", "7s")

	root := t.TempDir()
	writeQueueFixtureModule(t, root, "example.com/queueredistimeouttest", []string{"github.com/goforj/env/v2", "github.com/goforj/queue", "github.com/goforj/queue/driver/redisqueue", "github.com/goforj/str"}, queueLocalReplaces(t))
	if _, err := GenerateQueueFiles(root); err != nil {
		t.Fatalf("GenerateQueueFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "queues", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}

	source := string(managerGen)
	if !strings.Contains(source, `ShutdownTimeout:`) || !strings.Contains(source, `queueDuration(scope, rootScope, "SHUTDOWN_TIMEOUT", "10s")`) {
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
	writeQueueRuntimeFixture(t, root)

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
	t.Setenv("QUEUE_REDIS_NAME", "critical")
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
	writeQueueRuntimeFixture(t, root)
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

package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/goforj/env/v2"
	"github.com/goforj/str"
)

type queueAccessorTemplateData struct {
	GoModuleName string
	Names        []queueAccessorName
}

type queueAccessorName struct {
	Method string
	Queue  string
}

type queueConfigTemplateData struct {
	GoModuleName string
	Drivers      []queueDriverSpec
	HasOptional  bool
	HasRedis     bool
	HasSQL       bool
	HasSQS       bool
	HasURLBased  bool
	Names        []queueAccessorName
}

type queueDriverSpec struct {
	ConstName     string
	Constructor   string
	ImportPath    string
	ConfigType    string
	IsNative      bool
	Setup         []string
	Fields        []queueConfigField
	DriverLiteral string
}

type queueConfigField struct {
	Name  string
	Value string
}

var queueDriverSpecs = map[string]queueDriverSpec{
	"null": {
		ConstName:     "driverNull",
		IsNative:      true,
		DriverLiteral: "queue.DriverNull",
	},
	"sync": {
		ConstName:     "driverSync",
		IsNative:      true,
		DriverLiteral: "queue.DriverSync",
	},
	"workerpool": {
		ConstName:     "driverWorkerpool",
		IsNative:      true,
		DriverLiteral: "queue.DriverWorkerpool",
	},
	"redis": {
		ConstName:   "driverRedis",
		Constructor: "redisqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/redisqueue",
		ConfigType:  "redisqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "Addr", Value: "queueRedisAddr(scope, rootScope)"},
			{Name: "Password", Value: `queueString(scope, rootScope, "PASSWORD", env.Get("REDIS_PASSWORD", ""))`},
			{Name: "DB", Value: `queueInt(scope, rootScope, "DB", env.Get("REDIS_DB", "0"))`},
			{Name: "Queues", Value: "queueRedisWeights(scope, defaultQueue)"},
			{Name: "ServerLogLevel", Value: "queueRedisLogLevel(scope, rootScope)"},
			{Name: "ShutdownTimeout", Value: `queueDuration(scope, rootScope, "SHUTDOWN_TIMEOUT", "10s")`},
		},
	},
	"nats": {
		ConstName:   "driverNATS",
		Constructor: "natsqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/natsqueue",
		ConfigType:  "natsqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "URL", Value: "queueURL(scope, rootScope, \"nats://127.0.0.1:4222\")"},
		},
	},
	"sqs": {
		ConstName:   "driverSQS",
		Constructor: "sqsqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/sqsqueue",
		ConfigType:  "sqsqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "Region", Value: `queueString(scope, rootScope, "REGION", "us-east-1")`},
			{Name: "Endpoint", Value: `queueString(scope, rootScope, "ENDPOINT", "")`},
			{Name: "AccessKey", Value: `queueString(scope, rootScope, "ACCESS_KEY", "")`},
			{Name: "SecretKey", Value: `queueString(scope, rootScope, "SECRET_KEY", "")`},
		},
	},
	"rabbitmq": {
		ConstName:   "driverRabbitMQ",
		Constructor: "rabbitmqqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/rabbitmqqueue",
		ConfigType:  "rabbitmqqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "URL", Value: "queueURL(scope, rootScope, \"amqp://guest:guest@127.0.0.1:5672/\")"},
		},
	},
	"sqlite": {
		ConstName:   "driverSQLite",
		Constructor: "sqlitequeue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/sqlitequeue",
		ConfigType:  "sqlitequeue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "DSN", Value: `queueString(scope, rootScope, "DSN", queueSQLiteDSN(name))`},
			{Name: "ProcessingRecoveryGrace", Value: `queueDurationSeconds(scope, rootScope, "PROCESSING_RECOVERY_GRACE_SECONDS", 2)`},
			{Name: "ProcessingLeaseNoTimeout", Value: `queueDurationSeconds(scope, rootScope, "PROCESSING_LEASE_NO_TIMEOUT_SECONDS", 300)`},
		},
	},
	"postgres": {
		ConstName:   "driverPostgres",
		Constructor: "postgresqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/postgresqueue",
		ConfigType:  "postgresqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "DSN", Value: `queueString(scope, rootScope, "DSN", "")`},
			{Name: "ProcessingRecoveryGrace", Value: `queueDurationSeconds(scope, rootScope, "PROCESSING_RECOVERY_GRACE_SECONDS", 2)`},
			{Name: "ProcessingLeaseNoTimeout", Value: `queueDurationSeconds(scope, rootScope, "PROCESSING_LEASE_NO_TIMEOUT_SECONDS", 300)`},
		},
	},
	"mysql": {
		ConstName:   "driverMySQL",
		Constructor: "mysqlqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/mysqlqueue",
		ConfigType:  "mysqlqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "DSN", Value: `queueString(scope, rootScope, "DSN", "")`},
			{Name: "ProcessingRecoveryGrace", Value: `queueDurationSeconds(scope, rootScope, "PROCESSING_RECOVERY_GRACE_SECONDS", 2)`},
			{Name: "ProcessingLeaseNoTimeout", Value: `queueDurationSeconds(scope, rootScope, "PROCESSING_LEASE_NO_TIMEOUT_SECONDS", 300)`},
		},
	},
}

var queueRootKeys = []string{
	"DRIVER",
	"WORKERS",
	"NAME",
	"DEFAULT_QUEUE",
	"SHUTDOWN_TIMEOUT",
	"ADDR",
	"PASSWORD",
	"DB",
	"QUEUES",
	"SERVER_LOG_LEVEL",
	"URL",
	"REGION",
	"ENDPOINT",
	"ACCESS_KEY",
	"SECRET_KEY",
	"DSN",
	"WORKERPOOL_WORKERS",
	"WORKERPOOL_BUFFER",
	"WORKERPOOL_TASK_TIMEOUT_SECONDS",
	"PROCESSING_RECOVERY_GRACE_SECONDS",
	"PROCESSING_LEASE_NO_TIMEOUT_SECONDS",
}

var queueCommonKeys = makeSet(
	"DRIVER",
	"WORKERS",
	"NAME",
	"DEFAULT_QUEUE",
	"SHUTDOWN_TIMEOUT",
)

var queueDriverKeys = map[string]map[string]struct{}{
	"null":       makeSet(),
	"sync":       makeSet(),
	"workerpool": makeSet("WORKERPOOL_WORKERS", "WORKERPOOL_BUFFER", "WORKERPOOL_TASK_TIMEOUT_SECONDS"),
	"redis":      makeSet("ADDR", "PASSWORD", "DB", "QUEUES", "SERVER_LOG_LEVEL"),
	"nats":       makeSet("URL"),
	"sqs":        makeSet("REGION", "ENDPOINT", "ACCESS_KEY", "SECRET_KEY"),
	"rabbitmq":   makeSet("URL"),
	"sqlite":     makeSet("DSN", "PROCESSING_RECOVERY_GRACE_SECONDS", "PROCESSING_LEASE_NO_TIMEOUT_SECONDS"),
	"postgres":   makeSet("DSN", "PROCESSING_RECOVERY_GRACE_SECONDS", "PROCESSING_LEASE_NO_TIMEOUT_SECONDS"),
	"mysql":      makeSet("DSN", "PROCESSING_RECOVERY_GRACE_SECONDS", "PROCESSING_LEASE_NO_TIMEOUT_SECONDS"),
}

func GenerateQueueFiles(projectDir string) (int, error) {
	if err := validatePrimitiveEnv(primitiveEnvContract{
		Prefix:        "QUEUE",
		DefaultDriver: "workerpool",
		RootKeys:      queueRootKeys,
		CommonKeys:    queueCommonKeys,
		DriverKeys:    queueDriverKeys,
		ChildNames: func(scope env.Scope) []string {
			return scope.ChildNames(queueRootKeys)
		},
		AllowInactiveRootKeys: true,
		InheritRootDriver:     true,
	}); err != nil {
		return 0, err
	}
	manager, err := renderQueueConfig(projectDir)
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated queue manager: %w", err)
	}
	accessors, err := renderQueueAccessors(projectDir, discoverQueueNames())
	if err != nil {
		return 0, err
	}
	formattedAccessors, err := format.Source(accessors)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated queue accessors: %w", err)
	}
	written := 0
	changed, err := writeGeneratedSource(filepath.Join(projectDir, "internal", "queues", "manager_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	changed, err = writeGeneratedSource(filepath.Join(projectDir, "internal", "queues", "accessors_gen.go"), formattedAccessors)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	_ = os.Remove(filepath.Join(projectDir, "internal", "queues", "runtime.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "queues", "manager.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "queues", "queues_gen.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "queues", "config_gen.go"))
	return written, nil
}

func discoverQueueNames() []string {
	names := env.WithPrefix("QUEUE").ChildNames(queueRootKeys)
	for i := range names {
		names[i] = str.Of(names[i]).TrimSpace().ToLower().String()
	}
	sort.Strings(names)
	return names
}

func renderQueueAccessors(projectDir string, names []string) ([]byte, error) {
	moduleName, err := readModuleName(projectDir)
	if err != nil {
		return nil, err
	}
	data := queueAccessorTemplateData{
		GoModuleName: moduleName,
		Names:        make([]queueAccessorName, 0, len(names)),
	}
	for _, name := range names {
		data.Names = append(data.Names, queueAccessorName{
			Method: str.Of(name).Pascal().String(),
			Queue:  name,
		})
	}
	var b bytes.Buffer
	tmpl, err := template.New("queue-accessors").Parse(queueAccessorsSourceTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func readModuleName(projectDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module name not found in go.mod")
}

func renderQueueConfig(projectDir string) ([]byte, error) {
	names := discoverQueueNames()
	moduleName, err := readModuleName(projectDir)
	if err != nil {
		return nil, err
	}
	driverSet := map[string]struct{}{
		"null":       {},
		"sync":       {},
		"workerpool": {},
	}
	defaultDriver := str.Of(env.Get("QUEUE_DRIVER", "workerpool")).TrimSpace().ToLower().String()
	if defaultDriver != "" {
		driverSet[defaultDriver] = struct{}{}
	}
	for _, child := range env.WithPrefix("QUEUE").ChildNames(queueRootKeys) {
		driver := str.Of(env.Get("QUEUE_"+child+"_DRIVER", "")).TrimSpace().ToLower().String()
		if driver != "" {
			driverSet[driver] = struct{}{}
		}
	}
	drivers, err := supportedDrivers("QUEUE", queueDriverKeys, sortStrings(driverSet))
	if err != nil {
		return nil, err
	}

	data := queueConfigTemplateData{
		GoModuleName: moduleName,
		Drivers:      make([]queueDriverSpec, 0, len(drivers)),
		Names:        make([]queueAccessorName, 0, len(names)),
	}
	for _, name := range names {
		data.Names = append(data.Names, queueAccessorName{
			Method: str.Of(name).Pascal().String(),
			Queue:  name,
		})
	}
	for _, driver := range drivers {
		spec, ok := queueDriverSpecs[driver]
		if !ok {
			continue
		}
		switch driver {
		case "redis":
			data.HasOptional = true
			data.HasRedis = true
		case "sqlite", "postgres", "mysql":
			data.HasOptional = true
			data.HasSQL = true
		case "sqs":
			data.HasOptional = true
			data.HasSQS = true
		case "nats", "rabbitmq":
			data.HasOptional = true
			data.HasURLBased = true
		}
		if !spec.IsNative {
			data.HasOptional = true
		}
		data.Drivers = append(data.Drivers, spec)
	}

	var b bytes.Buffer
	tmpl, err := template.New("queue-config").Parse(queueConfigSourceTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

const queueAccessorsSourceTemplate = `// Code generated by forj generate --queue. DO NOT EDIT.
// Run: forj generate --queue
//
// These queue manager accessors are derived from the current .env file
// and environment variables available when generation ran.
// Named accessors are generated from QUEUE_<NAME>_<KEY> environment variables.
package queues

import "github.com/goforj/queue"

// Default returns the default queue instance derived from QUEUE_* configuration.
func (m *Manager) Default() *queue.Queue {
	return m.defaultQueue
}

{{ range .Names }}
// {{ .Method }} returns the "{{ .Queue }}" queue instance.
func (m *Manager) {{ .Method }}() *queue.Queue {
	return m.{{ .Queue }}
}
{{ end }}

// Instances returns the generated queue instances derived from QUEUE_* configuration.
func (m *Manager) Instances() []Instance {
	instances := []Instance{
		{Name: "default", Queue: m.defaultQueue, Workers: m.defaultWorkers, IsDefault: true},
	}
{{- range .Names }}
	instances = append(instances, Instance{Name: "{{ .Queue }}", Queue: m.{{ .Queue }}, Workers: m.{{ .Queue }}Workers})
{{- end }}
	return instances
}`

const queueConfigSourceTemplate = `// Code generated by forj generate --queue. DO NOT EDIT.
// Run: forj generate --queue
package queues

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
{{- if .HasSQL }}
	"os"
	"path/filepath"
{{- end }}

	"{{ .GoModuleName }}/internal/runtime"
	"github.com/goforj/env/v2"
	"github.com/goforj/queue"
	{{- range .Drivers }}
{{- if not .IsNative }}
	"{{ .ImportPath }}"
{{- end }}
{{- end }}
{{- if .HasOptional }}
	"github.com/goforj/queue/queueconfig"
	{{- end }}
	"github.com/goforj/str"
	"{{ .GoModuleName }}/internal/inspects"
)

const defaultQueueName = "default"

const (
	driverMySQL      = "mysql"
	driverNATS       = "nats"
	driverNull       = "null"
	driverPostgres   = "postgres"
	driverRabbitMQ   = "rabbitmq"
	driverRedis      = "redis"
	driverSQLite     = "sqlite"
	driverSQS        = "sqs"
	driverSync       = "sync"
	driverWorkerpool = "workerpool"
)

var queueRootKeys = []string{
	"DRIVER",
	"WORKERS",
	"NAME",
	"DEFAULT_QUEUE",
	"SHUTDOWN_TIMEOUT",
	"ADDR",
	"PASSWORD",
	"DB",
	"QUEUES",
	"SERVER_LOG_LEVEL",
	"URL",
	"REGION",
	"ENDPOINT",
	"ACCESS_KEY",
	"SECRET_KEY",
	"DSN",
	"WORKERPOOL_WORKERS",
	"WORKERPOOL_BUFFER",
	"WORKERPOOL_TASK_TIMEOUT_SECONDS",
	"PROCESSING_RECOVERY_GRACE_SECONDS",
	"PROCESSING_LEASE_NO_TIMEOUT_SECONDS",
}

type Manager struct {
	defaultQueue *queue.Queue
	defaultQueueName string
	defaultWorkers int
	ctx context.Context
	inspects *inspects.Manager
{{- range .Names }}
	{{ .Queue }} *queue.Queue
	{{ .Queue }}Workers int
{{- end }}
}

type Instance struct {
	Name      string
	Queue     *queue.Queue
	Workers   int
	IsDefault bool
}

type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

const inspectJobPayloadMaxBytes = 64 * 1024

// recordJobPayload captures a bounded job payload snapshot for inspect traces.
func recordJobPayload(ctx context.Context, msg queue.Message) {
	recorder := inspects.RecorderFromContext(ctx)
	if recorder == nil {
		return
	}
	payloadBytes := msg.PayloadBytes()
	if len(payloadBytes) == 0 {
		return
	}
	payloadKind := "text"
	if json.Valid(payloadBytes) {
		payloadKind = "json"
	}
	truncated := false
	if len(payloadBytes) > inspectJobPayloadMaxBytes {
		payloadBytes = payloadBytes[:inspectJobPayloadMaxBytes]
		truncated = true
	}
	recorder.RecordEvent(inspects.InspectEvent{
		Kind:    "annotation",
		Name:    "job_payload",
		Message: "job payload captured",
		Attributes: map[string]any{
			"payload":           string(payloadBytes),
			"payload_kind":      payloadKind,
			"payload_bytes":     len(msg.PayloadBytes()),
			"payload_truncated": truncated,
		},
	})
}

// recordQueuedJobPayload captures a bounded queued job payload snapshot for inspect traces.
func recordQueuedJobPayload(ctx context.Context, job queue.Job) {
	recorder := inspects.RecorderFromContext(ctx)
	if recorder == nil {
		return
	}
	payloadBytes := job.PayloadBytes()
	if len(payloadBytes) == 0 {
		return
	}
	payloadKind := "text"
	if json.Valid(payloadBytes) {
		payloadKind = "json"
	}
	truncated := false
	if len(payloadBytes) > inspectJobPayloadMaxBytes {
		payloadBytes = payloadBytes[:inspectJobPayloadMaxBytes]
		truncated = true
	}
	queueName := queue.DriverOptions(job).QueueName
	if queueName == "" {
		queueName = "default"
	}
	recorder.RecordEvent(inspects.InspectEvent{
		Kind:    "annotation",
		Name:    "queued_job_payload",
		Message: "queued job payload captured",
		Attributes: map[string]any{
			"job_name":          job.Type,
			"queue":             queueName,
			"payload":           string(payloadBytes),
			"payload_kind":      payloadKind,
			"payload_bytes":     len(job.PayloadBytes()),
			"payload_truncated": truncated,
		},
	})
}

func NewManager() (*Manager, error) {
	return NewManagerWithObserver(nil, nil, nil)
}

func NewManagerWithObserver(observer queue.Observer, logger queue.Logger, inspectManager *inspects.Manager) (*Manager, error) {
	return newManagerFromEnv(env.WithPrefix("QUEUE"), observer, logger, inspectManager)
}

func (m *Manager) ReadinessChecks() []ReadinessCheck {
	if m == nil {
		return nil
	}
	checks := []ReadinessCheck{
		{
			Name: "queue_default",
			Check: func(ctx context.Context) error {
				if m.defaultQueue == nil {
					return nil
				}
				return m.defaultQueue.Ready(ctx)
			},
		},
{{- range .Names }}
		{
			Name: "queue_{{ .Queue }}",
			Check: func(ctx context.Context) error {
				if m.{{ .Queue }} == nil {
					return nil
				}
				return m.{{ .Queue }}.Ready(ctx)
			},
		},
{{- end }}
	}
	return checks
}

// Register registers a handler on the default queue and stamps jobs source
// context at the queue execution boundary.
func (m *Manager) Register(jobType string, fn func(context.Context, queue.Message) error) {
	m.defaultQueue.Register(jobType, func(ctx context.Context, msg queue.Message) error {
		ctx = runtime.WithSource(ctx, runtime.SourceJobs)
		if m != nil && m.inspects != nil {
			ctx = m.inspects.Begin(ctx, runtime.SourceJobs, jobType, map[string]string{
				"job_name": jobType,
			})
			recordJobPayload(ctx, msg)
			defer m.inspects.Finish(ctx, "", nil)
		}
		err := fn(ctx, msg)
		if m != nil && m.inspects != nil && err != nil {
			m.inspects.Finish(ctx, "error", err)
		}
		return err
	})
}

// Dispatch enqueues work on the default queue with background context.
func (m *Manager) Dispatch(job queue.Job) (queue.DispatchResult, error) {
	if queue.DriverOptions(job).QueueName == "" {
		queueName := strings.TrimSpace(m.defaultQueueName)
		if queueName == "" {
			queueName = defaultQueueName
		}
		job = job.OnQueue(queueName)
	}
	recordQueuedJobPayload(m.ctx, job)
	return m.defaultQueue.Dispatch(job)
}

// WithContext returns a queue manager bound to the provided context.
func (m *Manager) WithContext(ctx context.Context) *Manager {
	clone := *m
	clone.ctx = ctx
	if m.defaultQueue != nil {
		clone.defaultQueue = m.defaultQueue.WithContext(ctx)
	}
{{- range .Names }}
	if m.{{ .Queue }} != nil {
		clone.{{ .Queue }} = m.{{ .Queue }}.WithContext(ctx)
	}
{{- end }}
	return &clone
}

func newManagerFromEnv(queueScope env.Scope, observer queue.Observer, logger queue.Logger, inspectManager *inspects.Manager) (*Manager, error) {
	defaultQueue, err := buildQueue(string(defaultQueueName), queueScope, queueScope, observer, logger)
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		defaultQueue: defaultQueue,
		defaultQueueName: queueDefaultQueue(string(defaultQueueName), queueScope, queueScope),
		defaultWorkers: queueWorkerCount(queueScope, queueScope),
		inspects: inspectManager,
	}

{{- if .Names }}
	for _, child := range queueScope.ChildNames(queueRootKeys) {
		name := str.Of(child).TrimSpace().ToLower().String()
		if name == "" {
			continue
		}
		queueInstance, err := buildQueue(name, queueScope.Child(child), queueScope, observer, logger)
		if err != nil {
			return nil, err
		}
		switch name {
{{- range .Names }}
		case "{{ .Queue }}":
			manager.{{ .Queue }} = queueInstance
			manager.{{ .Queue }}Workers = queueWorkerCount(queueScope.Child(child), queueScope)
{{- end }}
		}
	}
{{- end }}

	return manager, nil
}

func buildQueue(name string, scope env.Scope, rootScope env.Scope, observer queue.Observer, logger queue.Logger) (*queue.Queue, error) {
	driver := str.Of(queueString(scope, rootScope, "DRIVER", driverWorkerpool)).TrimSpace().ToLower().String()
	if driver == "" {
		driver = driverWorkerpool
	}

	defaultQueue := queueDefaultQueue(name, scope, rootScope)
	workerCount := queueWorkerCount(scope, rootScope)
{{- if .HasOptional }}
	baseConfig := queueconfig.DriverBaseConfig{
		DefaultQueue: defaultQueue,
		Observer:     observer,
		Logger:       logger,
	}
{{- end }}
	options := []queue.Option{
		queue.WithWorkers(workerCount),
		queue.WithHandlerContextDecorator(func(ctx context.Context) context.Context {
			return runtime.WithSource(ctx, runtime.SourceJobs)
		}),
	}

	switch driver {
{{- range .Drivers }}
	case {{ .ConstName }}:
{{- if .IsNative }}
		return queue.New(queue.Config{
			Driver: {{ .DriverLiteral }},
			DefaultQueue: defaultQueue,
			Observer: observer,
			Logger: logger,
		}, options...)
{{- else }}
		return {{ .Constructor }}({{ .ConfigType }}{
{{- range .Fields }}
			{{ .Name }}: {{ .Value }},
{{- end }}
		}, options...)
{{- end }}
{{- end }}
	default:
		return nil, fmt.Errorf("queue: unsupported driver %q", driver)
	}
}

func queueWorkerCount(scope env.Scope, rootScope env.Scope) int {
	workers := queueInt(scope, rootScope, "WORKERS", "30")
	if workers <= 0 {
		workers = queueInt(scope, rootScope, "WORKERPOOL_WORKERS", "30")
	}
	if workers <= 0 {
		return 30
	}
	return workers
}

func queueDefaultQueue(name string, scope env.Scope, rootScope env.Scope) string {
	value := strings.TrimSpace(scope.Get("NAME", ""))
	if value != "" {
		return queuePhysicalName(value)
	}
	value = strings.TrimSpace(scope.Get("DEFAULT_QUEUE", ""))
	if value != "" {
		return queuePhysicalName(value)
	}
	name = strings.TrimSpace(name)
	if name != "" && name != string(defaultQueueName) {
		return queuePhysicalName(name)
	}
	value = strings.TrimSpace(rootScope.Get("NAME", ""))
	if value != "" {
		return queuePhysicalName(value)
	}
	value = strings.TrimSpace(rootScope.Get("DEFAULT_QUEUE", "default"))
	if value == "" {
		return queuePhysicalName("default")
	}
	return queuePhysicalName(value)
}

func queuePhysicalName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = string(defaultQueueName)
	}
	target := strings.TrimSpace(runtime.CurrentAppTarget().Name)
	if target == "" || target == "app" {
		return name
	}
	prefix := target + "_"
	if strings.HasPrefix(name, prefix) {
		return name
	}
	return prefix + name
}

func queueString(scope env.Scope, rootScope env.Scope, key string, fallback string) string {
	if value := strings.TrimSpace(scope.Get(key, "")); value != "" {
		return value
	}
	if value := strings.TrimSpace(rootScope.Get(key, "")); value != "" {
		return value
	}
	return fallback
}

func queueInt(scope env.Scope, rootScope env.Scope, key string, fallback string) int {
	value := queueString(scope, rootScope, key, fallback)
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil {
		return parsed
	}
	parsed, _ = strconv.Atoi(strings.TrimSpace(fallback))
	return parsed
}

func queueDuration(scope env.Scope, rootScope env.Scope, key string, fallback string) time.Duration {
	value := queueString(scope, rootScope, key, fallback)
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err == nil {
		return parsed
	}
	parsed, _ = time.ParseDuration(strings.TrimSpace(fallback))
	return parsed
}

{{- if .HasRedis }}
func queueRedisAddr(scope env.Scope, rootScope env.Scope) string {
	addr := strings.TrimSpace(queueString(scope, rootScope, "ADDR", ""))
	if addr != "" {
		return addr
	}
	return fmt.Sprintf("%s:%s", env.Get("REDIS_HOST", "redis"), env.Get("REDIS_PORT", "6379"))
}

func queueRedisWeights(scope env.Scope, defaultQueue string) map[string]int {
	raw := strings.TrimSpace(scope.Get("QUEUES", ""))
	if raw == "" {
		raw = strings.TrimSpace(scope.Get("REDIS_QUEUES", ""))
	}
	if raw == "" {
		return map[string]int{defaultQueue: 1}
	}
	weights := make(map[string]int)
	for _, pair := range strings.Split(raw, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		weight, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			weight = 1
		}
		if name == "" || weight <= 0 {
			continue
		}
		weights[name] = weight
	}
	if len(weights) == 0 {
		return map[string]int{defaultQueue: 1}
	}
	return weights
}

func queueRedisLogLevel(scope env.Scope, rootScope env.Scope) redisqueue.ServerLogLevel {
	raw := strings.TrimSpace(queueString(scope, rootScope, "SERVER_LOG_LEVEL", ""))
	if raw == "" {
		raw = strings.TrimSpace(scope.Get("REDIS_LOG_LEVEL", ""))
	}
	switch strings.ToLower(raw) {
	case "debug":
		return redisqueue.ServerLogLevelDebug
	case "info":
		return redisqueue.ServerLogLevelInfo
	case "warn":
		return redisqueue.ServerLogLevelWarn
	case "error":
		return redisqueue.ServerLogLevelError
	case "fatal":
		return redisqueue.ServerLogLevelFatal
	default:
		return redisqueue.ServerLogLevelDefault
	}
}
{{- end }}

{{- if .HasURLBased }}
func queueURL(scope env.Scope, rootScope env.Scope, fallback string) string {
	value := strings.TrimSpace(queueString(scope, rootScope, "URL", fallback))
	if value == "" {
		return fallback
	}
	return value
}
{{- end }}

{{- if .HasSQL }}
func queueDurationSeconds(scope env.Scope, rootScope env.Scope, key string, fallbackSeconds int) time.Duration {
	seconds := queueInt(scope, rootScope, key, fmt.Sprintf("%d", fallbackSeconds))
	if seconds <= 0 {
		seconds = fallbackSeconds
	}
	return time.Duration(seconds) * time.Second
}

func queueSQLiteDSN(name string) string {
	base := filepath.Join(os.TempDir(), "queue-sqlite")
	if name == "" || name == string(defaultQueueName) {
		return filepath.Join(base, "default.db")
	}
	return filepath.Join(base, name+".db")
}
{{- end }}`

package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/goforj/str/v2"
)

// queueAccessorTemplateData carries the named queue methods emitted for one project snapshot.
type queueAccessorTemplateData struct {
	GoModuleName string
	Names        []queueAccessorName
}

// queueAccessorName binds an environment queue name to its generated Go method.
type queueAccessorName struct {
	Method string
	Queue  string
}

// queueConfigTemplateData keeps compiled transports, worker wiring, and named queues aligned during rendering.
type queueConfigTemplateData struct {
	CompiledDrivers []string
	GoModuleName    string
	Drivers         []queueDriverSpec
	HasOptional     bool
	HasRedis        bool
	HasSQL          bool
	HasSQS          bool
	HasURLBased     bool
	Names           []queueAccessorName
}

// queueDriverSpec captures the import and constructor metadata needed to emit one queue transport branch.
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

// queueConfigField binds a transport configuration field to its generated value expression.
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

// GenerateQueueFiles writes queue accessors whose selectable transports are fixed by the generation snapshot.
func GenerateQueueFiles(projectDir string) (int, error) {
	return generateQueueFiles(ambientGenerationInput(projectDir))
}

// generateQueueFiles uses one captured environment for validation, rendering, and named-resource discovery.
func generateQueueFiles(input generationInput) (int, error) {
	if err := validatePrimitiveEnv(input, primitiveEnvContract{
		Prefix:        "QUEUE",
		DefaultDriver: "workerpool",
		RootKeys:      queueRootKeys,
		CommonKeys:    queueCommonKeys,
		DriverKeys:    queueDriverKeys,
		ChildNames: func(environment generationEnvironment) []string {
			return exactScopedChildNames(environment, "QUEUE", queueRootKeys)
		},
		AllowInactiveRootKeys: true,
		InheritRootDriver:     true,
	}); err != nil {
		return 0, err
	}
	manager, err := renderQueueConfig(input)
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated queue manager: %w", err)
	}
	accessors, err := renderQueueAccessors(input.projectDir, discoverQueueNames(input))
	if err != nil {
		return 0, err
	}
	formattedAccessors, err := format.Source(accessors)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated queue accessors: %w", err)
	}
	written := 0
	changed, err := writeGeneratedSource(filepath.Join(input.projectDir, "internal", "queues", "manager_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	changed, err = writeGeneratedSource(filepath.Join(input.projectDir, "internal", "queues", "accessors_gen.go"), formattedAccessors)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	for _, name := range queueLegacyGeneratedFiles {
		changed, err = removeGeneratedFileIfExists(filepath.Join(input.projectDir, "internal", "queues", name))
		if err != nil {
			return written, err
		}
		if changed {
			written++
		}
	}
	return written, nil
}

// discoverQueueNames includes queues declared only through a configured App overlay.
func discoverQueueNames(input generationInput) []string {
	names := discoverPrimitiveChildNames(input, "QUEUE", queueRootKeys)
	for i := range names {
		names[i] = str.Of(names[i]).Trim().ToLower().String()
	}
	sort.Strings(names)
	return names
}

// renderQueueAccessors keeps named queue methods aligned with the same project snapshot used by the manager.
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

// readModuleName resolves the generated runtime import path from the target project's module declaration.
func readModuleName(projectDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return str.Of(line).TrimPrefix("module ").Trim().String(), nil
		}
	}
	return "", fmt.Errorf("module name not found in go.mod")
}

// renderQueueConfig retains native worker execution code without widening the authoritative compiled manifest.
func renderQueueConfig(input generationInput) ([]byte, error) {
	names := discoverQueueNames(input)
	moduleName, err := readModuleName(input.projectDir)
	if err != nil {
		return nil, err
	}
	driverSet := map[string]struct{}{
		"null":       {},
		"sync":       {},
		"workerpool": {},
	}
	defaultDriver := effectivePrimitiveDriver(input.environment.Get("QUEUE_DRIVER", "workerpool"), "workerpool")
	driverSet[defaultDriver] = struct{}{}
	for _, child := range exactScopedChildNames(input.environment, "QUEUE", queueRootKeys) {
		driver := str.Of(input.environment.Get("QUEUE_"+child+"_DRIVER", "")).Trim().ToLower().String()
		if driver != "" {
			driverSet[driver] = struct{}{}
		}
	}
	for _, active := range appPrefixedActiveDrivers(input, "QUEUE", "workerpool", true) {
		driverSet[active.driver] = struct{}{}
	}
	drivers, err := supportedDrivers(input.environment, "QUEUE", queueDriverKeys, sortStrings(driverSet))
	if err != nil {
		return nil, err
	}
	compiledDrivers := slices.Clone(drivers)
	drivers = appendMissingString(drivers, "workerpool")

	data := queueConfigTemplateData{
		CompiledDrivers: compiledDrivers,
		GoModuleName:    moduleName,
		Drivers:         make([]queueDriverSpec, 0, len(drivers)),
		Names:           make([]queueAccessorName, 0, len(names)),
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
		{Name: "default", queueName: m.defaultQueueName, Queue: m.defaultQueue, Workers: m.defaultWorkers, IsDefault: true},
	}
{{- range .Names }}
	instances = append(instances, Instance{Name: "{{ .Queue }}", queueName: m.{{ .Queue }}QueueName, Queue: m.{{ .Queue }}, Workers: m.{{ .Queue }}Workers})
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
	"github.com/goforj/str/v2"
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

var compiledQueueDrivers = []string{
{{- range .CompiledDrivers }}
	"{{ . }}",
{{- end }}
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

// Manager owns the queues and worker settings generated from the project's build contract.
type Manager struct {
	defaultQueue *queue.Queue
	defaultQueueName string
	defaultWorkers int
	ctx context.Context
	inspects *inspects.Manager
{{- range .Names }}
	{{ .Queue }} *queue.Queue
	{{ .Queue }}QueueName string
	{{ .Queue }}Workers int
{{- end }}
}

// Instance gives tooling a uniform view of each generated queue and its worker count.
type Instance struct {
	Name      string
	queueName string
	Queue     *queue.Queue
	Workers   int
	IsDefault bool
}

// ReadinessCheck pairs a stable queue name with its health probe.
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

// NewManager builds configured queues without optional runtime instrumentation.
func NewManager() (*Manager, error) {
	return NewManagerWithObserver(nil, nil, nil)
}

// NewManagerWithObserver centralizes queue construction when runtime instrumentation is available.
func NewManagerWithObserver(observer queue.Observer, logger queue.Logger, inspectManager *inspects.Manager) (*Manager, error) {
	return newManagerFromEnv(env.WithPrefix("QUEUE"), observer, logger, inspectManager)
}

// ReadinessChecks exposes an independently named health probe for every generated queue.
func (m *Manager) ReadinessChecks() []ReadinessCheck {
	checks := []ReadinessCheck{
		{
			Name: "queue_default",
			Check: func(ctx context.Context) error {
				return m.defaultQueue.Ready(ctx)
			},
		},
{{- range .Names }}
		{
			Name: "queue_{{ .Queue }}",
			Check: func(ctx context.Context) error {
				return m.{{ .Queue }}.Ready(ctx)
			},
		},
{{- end }}
	}
	return checks
}

// Register binds a job handler to every generated queue so workers can consume it from any configured lane.
func (m *Manager) Register(jobType string, fn func(context.Context, queue.Message) error) {
	for _, instance := range m.Instances() {
		instance.Queue.Register(jobType, m.instrumentJobHandler(jobType, fn))
	}
}

// instrumentJobHandler applies the shared source and inspect contract at every queue execution boundary.
func (m *Manager) instrumentJobHandler(jobType string, fn func(context.Context, queue.Message) error) func(context.Context, queue.Message) error {
	return func(ctx context.Context, msg queue.Message) (handlerErr error) {
		ctx = runtime.WithSource(ctx, runtime.SourceJobs)
		if m.inspects != nil {
			ctx = m.inspects.Begin(ctx, runtime.SourceJobs, jobType, map[string]string{
				"job_name": jobType,
			})
			recordJobPayload(ctx, msg)
			defer func() {
				status := ""
				if handlerErr != nil {
					status = "error"
				}
				m.inspects.Finish(ctx, status, handlerErr)
			}()
		}
		return fn(ctx, msg)
	}
}

// Dispatch routes generated named resources through their own driver and physical queue configuration.
func (m *Manager) Dispatch(job queue.Job) (queue.DispatchResult, error) {
	queueName := strings.TrimSpace(queue.DriverOptions(job).QueueName)
	if queueName == "" || strings.EqualFold(queueName, defaultQueueName) {
		queueName = strings.TrimSpace(m.defaultQueueName)
		if queueName == "" {
			queueName = defaultQueueName
		}
		job = job.OnQueue(queueName)
		recordQueuedJobPayload(m.ctx, job)
		return m.defaultQueue.Dispatch(job)
	}
	recordQueuedJobPayload(m.ctx, job)
	for _, instance := range m.Instances() {
		if !instance.IsDefault && strings.EqualFold(instance.Name, queueName) {
			return instance.Queue.Dispatch(job.OnQueue(instance.queueName))
		}
	}
	return m.defaultQueue.Dispatch(job)
}

// WithContext clones the manager so per-call context binding cannot mutate shared queue handles.
func (m *Manager) WithContext(ctx context.Context) *Manager {
	clone := *m
	clone.ctx = ctx
	clone.defaultQueue = m.defaultQueue.WithContext(ctx)
{{- range .Names }}
	clone.{{ .Queue }} = m.{{ .Queue }}.WithContext(ctx)
{{- end }}
	return &clone
}

// newManagerFromEnv keeps default and named queues on the same scoped configuration path.
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

{{- range .Names }}
	queue{{ .Method }}, err := buildQueue("{{ .Queue }}", queueScope.Child(str.Of("{{ .Queue }}").Snake().ToUpper().String()), queueScope, observer, logger)
	if err != nil {
		return nil, err
	}
	manager.{{ .Queue }} = queue{{ .Method }}
	manager.{{ .Queue }}QueueName = queueDefaultQueue("{{ .Queue }}", queueScope.Child(str.Of("{{ .Queue }}").Snake().ToUpper().String()), queueScope)
	manager.{{ .Queue }}Workers = queueWorkerCount(queueScope.Child(str.Of("{{ .Queue }}").Snake().ToUpper().String()), queueScope)
{{- end }}

	return manager, nil
}

// buildQueue rejects transports outside the artifact manifest before workers or infrastructure are initialized.
func buildQueue(name string, scope env.Scope, rootScope env.Scope, observer queue.Observer, logger queue.Logger) (*queue.Queue, error) {
	driver := str.Of(queueString(scope, rootScope, "DRIVER", driverWorkerpool)).Trim().ToLower().String()
	if driver == "" {
		driver = driverWorkerpool
	}
	if !queueDriverCompiled(driver) {
		return nil, fmt.Errorf("queue: active driver %q is not built in; compiled choices: %s; run forj generate --queue after updating QUEUE_SUPPORTED_DRIVERS", driver, strings.Join(compiledQueueDrivers, ", "))
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

// queueDriverCompiled reports whether driver is selectable in this generated artifact.
func queueDriverCompiled(driver string) bool {
	for _, compiled := range compiledQueueDrivers {
		if driver == compiled {
			return true
		}
	}
	return false
}

// queueWorkerCount preserves a usable worker pool when scoped configuration is missing or non-positive.
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

// queueDefaultQueue applies resource-specific naming before falling back to root queue configuration.
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

// queuePhysicalName namespaces implicit queue names so multiple Apps can share one transport safely.
func queuePhysicalName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = string(defaultQueueName)
	}
	target := strings.TrimSpace(runtime.CurrentApp().Name)
	if target == "" || target == "app" {
		return name
	}
	prefix := target + "_"
	if strings.HasPrefix(name, prefix) {
		return name
	}
	return prefix + name
}

// queueString gives named queue settings precedence while retaining root configuration as a shared fallback.
func queueString(scope env.Scope, rootScope env.Scope, key string, fallback string) string {
	if value := strings.TrimSpace(scope.Get(key, "")); value != "" {
		return value
	}
	if value := strings.TrimSpace(rootScope.Get(key, "")); value != "" {
		return value
	}
	return fallback
}

// queueInt falls back deterministically when an environment value is not a valid integer.
func queueInt(scope env.Scope, rootScope env.Scope, key string, fallback string) int {
	value := queueString(scope, rootScope, key, fallback)
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil {
		return parsed
	}
	parsed, _ = strconv.Atoi(strings.TrimSpace(fallback))
	return parsed
}

// queueDuration falls back deterministically when an environment value is not a valid duration.
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
// queueRedisAddr preserves queue-specific endpoints while sharing the global Redis fallback contract.
func queueRedisAddr(scope env.Scope, rootScope env.Scope) string {
	addr := strings.TrimSpace(queueString(scope, rootScope, "ADDR", ""))
	if addr != "" {
		return addr
	}
	return fmt.Sprintf("%s:%s", env.Get("REDIS_HOST", "redis"), env.Get("REDIS_PORT", "6379"))
}

// queueRedisWeights guarantees that Redis workers retain the configured default queue when weights are absent.
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

// queueRedisLogLevel translates environment text into the Redis driver's constrained log-level enumeration.
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
// queueURL lets named queues override a shared transport URL without losing the driver's fallback.
func queueURL(scope env.Scope, rootScope env.Scope, fallback string) string {
	value := strings.TrimSpace(queueString(scope, rootScope, "URL", fallback))
	if value == "" {
		return fallback
	}
	return value
}
{{- end }}

{{- if .HasSQL }}
// queueDurationSeconds prevents non-positive SQL lease settings from disabling recovery behavior accidentally.
func queueDurationSeconds(scope env.Scope, rootScope env.Scope, key string, fallbackSeconds int) time.Duration {
	seconds := queueInt(scope, rootScope, key, fmt.Sprintf("%d", fallbackSeconds))
	if seconds <= 0 {
		seconds = fallbackSeconds
	}
	return time.Duration(seconds) * time.Second
}

// queueSQLiteDSN isolates named queues in resource-specific local database files.
func queueSQLiteDSN(name string) string {
	base := filepath.Join(os.TempDir(), "queue-sqlite")
	if name == "" || name == string(defaultQueueName) {
		return filepath.Join(base, "default.db")
	}
	return filepath.Join(base, name+".db")
}
{{- end }}`

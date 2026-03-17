package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"text/template"

	"github.com/goforj/env/v2"
	"github.com/goforj/str"
)

type queueAccessorTemplateData struct {
	Names []queueAccessorName
}

type queueAccessorName struct {
	Method string
	Queue  string
}

type queueConfigTemplateData struct {
	Drivers     []queueDriverSpec
	HasOptional bool
	HasRedis    bool
	HasSQL      bool
	HasSQS      bool
	HasURLBased bool
	Names       []queueAccessorName
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
		DriverLiteral: "goforjqueue.DriverNull",
	},
	"sync": {
		ConstName:     "driverSync",
		IsNative:      true,
		DriverLiteral: "goforjqueue.DriverSync",
	},
	"workerpool": {
		ConstName:     "driverWorkerpool",
		IsNative:      true,
		DriverLiteral: "goforjqueue.DriverWorkerpool",
	},
	"redis": {
		ConstName:   "driverRedis",
		Constructor: "redisqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/redisqueue",
		ConfigType:  "redisqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "Addr", Value: "queueRedisAddr(scope)"},
			{Name: "Password", Value: `scope.Get("PASSWORD", env.Get("REDIS_PASSWORD", ""))`},
			{Name: "DB", Value: `scope.GetInt("DB", env.Get("REDIS_DB", "0"))`},
			{Name: "Queues", Value: "queueRedisWeights(scope, defaultQueue)"},
			{Name: "ServerLogLevel", Value: "queueRedisLogLevel(scope)"},
		},
	},
	"nats": {
		ConstName:   "driverNATS",
		Constructor: "natsqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/natsqueue",
		ConfigType:  "natsqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "URL", Value: "queueURL(scope, \"nats://127.0.0.1:4222\")"},
		},
	},
	"sqs": {
		ConstName:   "driverSQS",
		Constructor: "sqsqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/sqsqueue",
		ConfigType:  "sqsqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "Region", Value: `scope.Get("REGION", "us-east-1")`},
			{Name: "Endpoint", Value: `scope.Get("ENDPOINT", "")`},
			{Name: "AccessKey", Value: `scope.Get("ACCESS_KEY", "")`},
			{Name: "SecretKey", Value: `scope.Get("SECRET_KEY", "")`},
		},
	},
	"rabbitmq": {
		ConstName:   "driverRabbitMQ",
		Constructor: "rabbitmqqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/rabbitmqqueue",
		ConfigType:  "rabbitmqqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "URL", Value: "queueURL(scope, \"amqp://guest:guest@127.0.0.1:5672/\")"},
		},
	},
	"sqlite": {
		ConstName:   "driverSQLite",
		Constructor: "sqlitequeue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/sqlitequeue",
		ConfigType:  "sqlitequeue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "DSN", Value: `scope.Get("DSN", queueSQLiteDSN(name))`},
			{Name: "ProcessingRecoveryGrace", Value: `queueDurationSeconds(scope, "PROCESSING_RECOVERY_GRACE_SECONDS", 2)`},
			{Name: "ProcessingLeaseNoTimeout", Value: `queueDurationSeconds(scope, "PROCESSING_LEASE_NO_TIMEOUT_SECONDS", 300)`},
		},
	},
	"postgres": {
		ConstName:   "driverPostgres",
		Constructor: "postgresqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/postgresqueue",
		ConfigType:  "postgresqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "DSN", Value: `scope.Get("DSN", "")`},
			{Name: "ProcessingRecoveryGrace", Value: `queueDurationSeconds(scope, "PROCESSING_RECOVERY_GRACE_SECONDS", 2)`},
			{Name: "ProcessingLeaseNoTimeout", Value: `queueDurationSeconds(scope, "PROCESSING_LEASE_NO_TIMEOUT_SECONDS", 300)`},
		},
	},
	"mysql": {
		ConstName:   "driverMySQL",
		Constructor: "mysqlqueue.NewWithConfig",
		ImportPath:  "github.com/goforj/queue/driver/mysqlqueue",
		ConfigType:  "mysqlqueue.Config",
		Fields: []queueConfigField{
			{Name: "DriverBaseConfig", Value: "baseConfig"},
			{Name: "DSN", Value: `scope.Get("DSN", "")`},
			{Name: "ProcessingRecoveryGrace", Value: `queueDurationSeconds(scope, "PROCESSING_RECOVERY_GRACE_SECONDS", 2)`},
			{Name: "ProcessingLeaseNoTimeout", Value: `queueDurationSeconds(scope, "PROCESSING_LEASE_NO_TIMEOUT_SECONDS", 300)`},
		},
	},
}

var queueRootKeys = []string{
	"DRIVER",
	"WORKERS",
	"DEFAULT_QUEUE",
	"SHUTDOWN_TIMEOUT_SECONDS",
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
	"DEFAULT_QUEUE",
	"SHUTDOWN_TIMEOUT_SECONDS",
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
	}); err != nil {
		return 0, err
	}
	manager, err := renderQueueConfig()
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated queue manager: %w", err)
	}
	written := 0
	changed, err := writeGeneratedSource(filepath.Join(projectDir, "internal", "queue", "manager_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	_ = os.Remove(filepath.Join(projectDir, "internal", "queue", "runtime.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "queue", "manager.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "queue", "queues_gen.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "queue", "config_gen.go"))
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

func renderQueueAccessors(names []string) ([]byte, error) {
	data := queueAccessorTemplateData{
		Names: make([]queueAccessorName, 0, len(names)),
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

func renderQueueConfig() ([]byte, error) {
	names := discoverQueueNames()
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
	drivers := make([]string, 0, len(driverSet))
	for driver := range driverSet {
		drivers = append(drivers, driver)
	}
	sort.Strings(drivers)

	data := queueConfigTemplateData{
		Drivers: make([]queueDriverSpec, 0, len(drivers)),
		Names:   make([]queueAccessorName, 0, len(names)),
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
// The default queue is exposed by Manager.Default().
// This file contains named queue accessors generated from
// QUEUE_<NAME>_<KEY> environment variables.
package queue

{{- if .Names }}
import goforjqueue "github.com/goforj/queue"
type namedQueues struct {
{{- range .Names }}
	{{ .Queue }} *goforjqueue.Queue
{{- end }}
}

{{ range .Names }}
// {{ .Method }} returns the "{{ .Queue }}" queue instance.
func (m *Manager) {{ .Method }}() *goforjqueue.Queue {
	return m.named.{{ .Queue }}
}
{{ end }}
{{- else }}
type namedQueues struct{}
{{- end }}`

const queueConfigSourceTemplate = `// Code generated by forj generate --queue. DO NOT EDIT.
// Run: forj generate --queue
package queue

import (
	"fmt"
{{- if .HasRedis }}
	"strconv"
{{- end }}
	"strings"
{{- if .HasSQL }}
	"os"
	"path/filepath"
	"time"
{{- end }}

	"github.com/goforj/env/v2"
	goforjqueue "github.com/goforj/queue"
{{- range .Drivers }}
{{- if not .IsNative }}
	"{{ .ImportPath }}"
{{- end }}
{{- end }}
{{- if .HasOptional }}
	"github.com/goforj/queue/queueconfig"
{{- end }}
	"github.com/goforj/str"
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
	"DEFAULT_QUEUE",
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
	defaultQueue *goforjqueue.Queue
{{- range .Names }}
	{{ .Queue }} *goforjqueue.Queue
{{- end }}
}

func NewManager() (*Manager, error) {
	return NewManagerWithObserver(nil, nil)
}

func NewManagerWithObserver(observer goforjqueue.Observer, logger goforjqueue.Logger) (*Manager, error) {
	return newManagerFromEnv(env.WithPrefix("QUEUE"), observer, logger)
}

func (m *Manager) Default() *goforjqueue.Queue {
	return m.defaultQueue
}

{{- range .Names }}
func (m *Manager) {{ .Method }}() *goforjqueue.Queue {
	return m.{{ .Queue }}
}

{{- end }}
func newManagerFromEnv(queueScope env.Scope, observer goforjqueue.Observer, logger goforjqueue.Logger) (*Manager, error) {
	defaultQueue, err := buildQueue(string(defaultQueueName), queueScope, observer, logger)
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		defaultQueue: defaultQueue,
	}

{{- if .Names }}
	for _, child := range queueScope.ChildNames(queueRootKeys) {
		name := str.Of(child).TrimSpace().ToLower().String()
		if name == "" {
			continue
		}
		queueInstance, err := buildQueue(name, queueScope.Child(child), observer, logger)
		if err != nil {
			return nil, err
		}
		switch name {
{{- range .Names }}
		case "{{ .Queue }}":
			manager.{{ .Queue }} = queueInstance
{{- end }}
		}
	}
{{- end }}

	return manager, nil
}

func buildQueue(name string, scope env.Scope, observer goforjqueue.Observer, logger goforjqueue.Logger) (*goforjqueue.Queue, error) {
	driver := str.Of(scope.Get("DRIVER", driverWorkerpool)).TrimSpace().ToLower().String()
	if driver == "" {
		driver = driverWorkerpool
	}

	defaultQueue := queueDefaultQueue(scope)
	workerCount := queueWorkerCount(scope)
{{- if .HasOptional }}
	baseConfig := queueconfig.DriverBaseConfig{
		DefaultQueue: defaultQueue,
		Observer:     observer,
		Logger:       logger,
	}
{{- end }}
	options := []goforjqueue.Option{
		goforjqueue.WithWorkers(workerCount),
	}

	switch driver {
{{- range .Drivers }}
	case {{ .ConstName }}:
{{- if .IsNative }}
		return goforjqueue.New(goforjqueue.Config{
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

func queueWorkerCount(scope env.Scope) int {
	workers := scope.GetInt("WORKERS", "30")
	if workers <= 0 {
		workers = scope.GetInt("WORKERPOOL_WORKERS", "30")
	}
	if workers <= 0 {
		return 30
	}
	return workers
}

func queueDefaultQueue(scope env.Scope) string {
	value := strings.TrimSpace(scope.Get("DEFAULT_QUEUE", "default"))
	if value == "" {
		return "default"
	}
	return value
}

{{- if .HasRedis }}
func queueRedisAddr(scope env.Scope) string {
	addr := strings.TrimSpace(scope.Get("ADDR", ""))
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

func queueRedisLogLevel(scope env.Scope) redisqueue.ServerLogLevel {
	raw := strings.TrimSpace(scope.Get("SERVER_LOG_LEVEL", ""))
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
func queueURL(scope env.Scope, fallback string) string {
	value := strings.TrimSpace(scope.Get("URL", fallback))
	if value == "" {
		return fallback
	}
	return value
}
{{- end }}

{{- if .HasSQL }}
func queueDurationSeconds(scope env.Scope, key string, fallbackSeconds int) time.Duration {
	seconds := scope.GetInt(key, fmt.Sprintf("%d", fallbackSeconds))
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

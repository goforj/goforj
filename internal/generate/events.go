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

type eventConfigTemplateData struct {
	Drivers          []eventDriverSpec
	HasNATSJetStream bool
}

type eventDriverSpec struct {
	CaseName     string
	DriverName   string
	ImportPath   string
	PackageName  string
	ConfigType   string
	Constructor  string
	NeedsContext bool
	Fields       []eventConfigField
}

type eventConfigField struct {
	Name  string
	Value string
}

var eventDriverSpecs = map[string]eventDriverSpec{
	"redis": {
		CaseName:    "DriverRedis",
		DriverName:  "redis",
		ImportPath:  "github.com/goforj/events/driver/redisevents",
		PackageName: "redisevents",
		ConfigType:  "redisevents.Config",
		Constructor: "redisevents.New",
		Fields: []eventConfigField{
			{Name: "Addr", Value: `eventsRedisAddr()`},
		},
	},
	"nats": {
		CaseName:    "DriverNATS",
		DriverName:  "nats",
		ImportPath:  "github.com/goforj/events/driver/natsevents",
		PackageName: "natsevents",
		ConfigType:  "natsevents.Config",
		Constructor: "natsevents.New",
		Fields: []eventConfigField{
			{Name: "URL", Value: `env.Get("EVENTS_URL", "nats://127.0.0.1:4222")`},
		},
	},
	"natsjetstream": {
		CaseName:    "DriverNATSJetStream",
		DriverName:  "natsjetstream",
		ImportPath:  "github.com/goforj/events/driver/natsjetstreamevents",
		PackageName: "natsjetstreamevents",
		ConfigType:  "natsjetstreamevents.Config",
		Constructor: "natsjetstreamevents.New",
		Fields: []eventConfigField{
			{Name: "URL", Value: `env.Get("EVENTS_URL", "nats://127.0.0.1:4222")`},
			{Name: "SubjectPrefix", Value: `env.Get("EVENTS_SUBJECT_PREFIX", "events.")`},
			{Name: "StreamNamePrefix", Value: `env.Get("EVENTS_STREAM_NAME_PREFIX", "EVENTS_")`},
			{Name: "InactiveThreshold", Value: `eventsDurationSeconds("EVENTS_INACTIVE_THRESHOLD_SECONDS", 30)`},
			{Name: "AckWait", Value: `eventsDurationSeconds("EVENTS_ACK_WAIT_SECONDS", 30)`},
			{Name: "FetchMaxWait", Value: `eventsDurationMilliseconds("EVENTS_FETCH_MAX_WAIT_MS", 250)`},
			{Name: "Storage", Value: `eventsJetStreamStorage(env.Get("EVENTS_STORAGE", "memory"))`},
		},
	},
	"kafka": {
		CaseName:    "DriverKafka",
		DriverName:  "kafka",
		ImportPath:  "github.com/goforj/events/driver/kafkaevents",
		PackageName: "kafkaevents",
		ConfigType:  "kafkaevents.Config",
		Constructor: "kafkaevents.New",
		Fields: []eventConfigField{
			{Name: "Brokers", Value: `eventsCSV("EVENTS_BROKERS", "127.0.0.1:9092")`},
		},
	},
	"gcppubsub": {
		CaseName:     "DriverGCPPubSub",
		DriverName:   "gcppubsub",
		ImportPath:   "github.com/goforj/events/driver/gcppubsubevents",
		PackageName:  "gcppubsubevents",
		ConfigType:   "gcppubsubevents.Config",
		Constructor:  "gcppubsubevents.New",
		NeedsContext: true,
		Fields: []eventConfigField{
			{Name: "ProjectID", Value: `env.Get("EVENTS_PROJECT_ID", "")`},
			{Name: "URI", Value: `env.Get("EVENTS_URI", "")`},
		},
	},
	"sns": {
		CaseName:    "DriverSNS",
		DriverName:  "sns",
		ImportPath:  "github.com/goforj/events/driver/snsevents",
		PackageName: "snsevents",
		ConfigType:  "snsevents.Config",
		Constructor: "snsevents.New",
		Fields: []eventConfigField{
			{Name: "Region", Value: `env.Get("EVENTS_REGION", "us-east-1")`},
			{Name: "Endpoint", Value: `env.Get("EVENTS_ENDPOINT", "")`},
			{Name: "TopicNamePrefix", Value: `env.Get("EVENTS_TOPIC_NAME_PREFIX", "")`},
			{Name: "QueueNamePrefix", Value: `env.Get("EVENTS_QUEUE_NAME_PREFIX", "")`},
			{Name: "WaitTimeSeconds", Value: `int32(env.GetInt("EVENTS_WAIT_TIME_SECONDS", "1"))`},
			{Name: "VisibilityTimeout", Value: `int32(env.GetInt("EVENTS_VISIBILITY_TIMEOUT_SECONDS", "30"))`},
		},
	},
}

var eventRootKeys = []string{
	"DRIVER",
	"ADDR",
	"REDIS_CHANNEL_PREFIX",
	"URL",
	"BROKERS",
	"PROJECT_ID",
	"URI",
	"REGION",
	"ENDPOINT",
	"TOPIC_NAME_PREFIX",
	"QUEUE_NAME_PREFIX",
	"WAIT_TIME_SECONDS",
	"VISIBILITY_TIMEOUT_SECONDS",
	"SUBJECT_PREFIX",
	"STREAM_NAME_PREFIX",
	"INACTIVE_THRESHOLD_SECONDS",
	"ACK_WAIT_SECONDS",
	"FETCH_MAX_WAIT_MS",
	"STORAGE",
	"INPROC_WORKERS",
	"INPROC_BUFFER",
}

var eventCommonKeys = makeSet("DRIVER", "REDIS_CHANNEL_PREFIX")

var eventDriverKeys = map[string]map[string]struct{}{
	"inproc":        makeSet("INPROC_WORKERS", "INPROC_BUFFER"),
	"null":          makeSet(),
	"redis":         makeSet("ADDR", "REDIS_CHANNEL_PREFIX"),
	"nats":          makeSet("URL"),
	"natsjetstream": makeSet("URL", "SUBJECT_PREFIX", "STREAM_NAME_PREFIX", "INACTIVE_THRESHOLD_SECONDS", "ACK_WAIT_SECONDS", "FETCH_MAX_WAIT_MS", "STORAGE"),
	"kafka":         makeSet("BROKERS"),
	"gcppubsub":     makeSet("PROJECT_ID", "URI"),
	"sns":           makeSet("REGION", "ENDPOINT", "TOPIC_NAME_PREFIX", "QUEUE_NAME_PREFIX", "WAIT_TIME_SECONDS", "VISIBILITY_TIMEOUT_SECONDS"),
}

func GenerateEventFiles(projectDir string) (int, error) {
	if err := validatePrimitiveEnv(primitiveEnvContract{
		Prefix:        "EVENTS",
		DefaultDriver: "inproc",
		RootKeys:      eventRootKeys,
		CommonKeys:    eventCommonKeys,
		DriverKeys:    eventDriverKeys,
		ChildNames: func(scope env.Scope) []string {
			return nil
		},
		AllowInactiveRootKeys: true,
	}); err != nil {
		return 0, err
	}
	manager, err := renderEventConfig()
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated events driver config: %w", err)
	}
	written := 0
	changed, err := writeGeneratedSource(filepath.Join(projectDir, "internal", "events", "driver_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "driver.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "factory.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "bus_redis.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "bus_inproc.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "helpers.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "driver_test.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "factory_test.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "bus_redis_test.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "bus_inproc_test.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "helpers_test.go"))
	return written, nil
}

func renderEventConfig() ([]byte, error) {
	selectedDrivers := uniqueEventDrivers()
	drivers := make([]eventDriverSpec, 0, len(selectedDrivers))
	for _, name := range selectedDrivers {
		spec, ok := eventDriverSpecs[name]
		if !ok {
			continue
		}
		drivers = append(drivers, spec)
	}
	data := eventConfigTemplateData{Drivers: drivers}
	for _, driver := range drivers {
		if driver.DriverName == "natsjetstream" {
			data.HasNATSJetStream = true
		}
	}

	tmpl, err := template.New("eventsConfig").Parse(eventsConfigTemplate)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func uniqueEventDrivers() []string {
	seen := map[string]struct{}{}
	scope := env.WithPrefix("EVENTS")
	driver := str.Of(scope.Get("DRIVER", "inproc")).TrimSpace().ToLower().String()
	if driver == "" {
		driver = "inproc"
	}
	seen[driver] = struct{}{}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

const eventsConfigTemplate = `package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	goforjevents "github.com/goforj/events"
	eventscore "github.com/goforj/events/eventscore"
	"github.com/goforj/env/v2"
	"github.com/goforj/str"
{{- range .Drivers }}
	"{{ .ImportPath }}"
{{- end }}
{{- if .HasNATSJetStream }}
	"github.com/nats-io/nats.go/jetstream"
{{- end }}
)

func ActiveDriver() Driver {
	value := str.Of(env.Get("EVENTS_DRIVER", "inproc")).
		TrimSpace().
		ToLower().
		String()

	switch value {
	case "null":
		return DriverNull
	case "redis":
		return DriverRedis
	case "nats":
		return DriverNATS
	case "natsjetstream":
		return DriverNATSJetStream
	case "kafka":
		return DriverKafka
	case "gcppubsub":
		return DriverGCPPubSub
	case "sns":
		return DriverSNS
	default:
		return DriverInproc
	}
}

func NewBus(ctx context.Context) Bus {
	switch ActiveDriver() {
{{- range .Drivers }}
	case {{ .CaseName }}:
{{- if .NeedsContext }}
		driver, err := {{ .Constructor }}(normalizeEventsContext(ctx), {{ .ConfigType }}{
{{- else }}
		driver, err := {{ .Constructor }}({{ .ConfigType }}{
{{- end }}
{{- range .Fields }}
			{{ .Name }}: {{ .Value }},
{{- end }}
		})
		if err != nil {
			return newErrorBus({{ .CaseName }}, err)
		}
		api, err := goforjevents.New(goforjevents.Config{
			Driver: driverKind({{ .CaseName }}),
			Transport: driver,
		})
		if err != nil {
			return newErrorBus({{ .CaseName }}, err)
		}
		return newManagedBus(api, driver.Ready, func(context.Context) error {
			if closer, ok := any(driver).(interface{ Close() error }); ok {
				return closer.Close()
			}
			return nil
		})
{{- end }}
	case DriverNull:
		api, err := goforjevents.NewNull()
		if err != nil {
			return newErrorBus(DriverNull, err)
		}
		return newManagedBus(api, nil, nil)
	default:
		if ActiveDriver() == DriverInproc {
			api, err := goforjevents.NewSync()
			if err != nil {
				return newErrorBus(DriverInproc, err)
			}
			return newManagedBus(api, nil, nil)
		}
		return newErrorBus(ActiveDriver(), fmt.Errorf("events driver %q was not generated", ActiveDriver()))
	}
}

func eventsRedisAddr() string {
	addr := str.Of(env.Get("EVENTS_ADDR", "")).TrimSpace().String()
	if addr != "" {
		return addr
	}
	return fmt.Sprintf("%s:%s", env.Get("REDIS_HOST", "redis"), env.Get("REDIS_PORT", "6379"))
}

func eventsCSV(key string, fallback string) []string {
	raw := str.Of(env.Get(key, fallback)).TrimSpace().String()
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := str.Of(part).TrimSpace().String()
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func eventsDurationSeconds(key string, fallback int) time.Duration {
	return time.Duration(env.GetInt(key, fmt.Sprintf("%d", fallback))) * time.Second
}

func eventsDurationMilliseconds(key string, fallback int) time.Duration {
	return time.Duration(env.GetInt(key, fmt.Sprintf("%d", fallback))) * time.Millisecond
}

{{- if .HasNATSJetStream }}
func eventsJetStreamStorage(value string) jetstream.StorageType {
	switch str.Of(value).TrimSpace().ToLower().String() {
	case "file":
		return jetstream.FileStorage
	default:
		return jetstream.MemoryStorage
	}
}
{{- end }}

func driverKind(value Driver) eventscore.Driver {
	switch value {
	case DriverNull:
		return eventscore.DriverNull
	case DriverRedis:
		return eventscore.DriverRedis
	case DriverNATS:
		return eventscore.DriverNATS
	case DriverNATSJetStream:
		return eventscore.DriverNATSJetStream
	case DriverKafka:
		return eventscore.DriverKafka
	case DriverGCPPubSub:
		return eventscore.DriverGCPPubSub
	case DriverSNS:
		return eventscore.DriverSNS
	default:
		return eventscore.DriverSync
	}
}
`

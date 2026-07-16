package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"
	"sort"
	"text/template"

	"github.com/goforj/str"
)

// eventConfigTemplateData keeps compiled transports and named buses aligned while rendering the manager.
type eventConfigTemplateData struct {
	CompiledDrivers  []string
	Drivers          []eventDriverSpec
	HasRedis         bool
	HasNATSJetStream bool
	Names            []eventAccessorName
}

// eventAccessorTemplateData carries the named bus methods emitted for one project snapshot.
type eventAccessorTemplateData struct {
	Names []eventAccessorName
}

// eventAccessorName binds an environment bus name to its generated field and Go method.
type eventAccessorName struct {
	Method string
	Field  string
	Bus    string
}

// eventDriverSpec captures the imports and capabilities needed to emit one event transport branch.
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

// eventConfigField binds a transport configuration field to its generated value expression.
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
			{Name: "Client", Value: `eventsRedisClient(scope)`},
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
			{Name: "URL", Value: `scope.Get("URL", "nats://127.0.0.1:4222")`},
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
			{Name: "URL", Value: `scope.Get("URL", "nats://127.0.0.1:4222")`},
			{Name: "SubjectPrefix", Value: `scope.Get("SUBJECT_PREFIX", "events.")`},
			{Name: "StreamNamePrefix", Value: `scope.Get("STREAM_NAME_PREFIX", "EVENTS_")`},
			{Name: "InactiveThreshold", Value: `eventsDurationSeconds(scope, "INACTIVE_THRESHOLD_SECONDS", 30)`},
			{Name: "AckWait", Value: `eventsDurationSeconds(scope, "ACK_WAIT_SECONDS", 30)`},
			{Name: "FetchMaxWait", Value: `eventsDurationMilliseconds(scope, "FETCH_MAX_WAIT_MS", 250)`},
			{Name: "Storage", Value: `eventsJetStreamStorage(scope.Get("STORAGE", "memory"))`},
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
			{Name: "Brokers", Value: `eventsCSV(scope, "BROKERS", "127.0.0.1:9092")`},
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
			{Name: "ProjectID", Value: `scope.Get("PROJECT_ID", "")`},
			{Name: "URI", Value: `scope.Get("URI", "")`},
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
			{Name: "Region", Value: `scope.Get("REGION", "us-east-1")`},
			{Name: "Endpoint", Value: `scope.Get("ENDPOINT", "")`},
			{Name: "TopicNamePrefix", Value: `scope.Get("TOPIC_NAME_PREFIX", "")`},
			{Name: "QueueNamePrefix", Value: `scope.Get("QUEUE_NAME_PREFIX", "")`},
			{Name: "WaitTimeSeconds", Value: `int32(scope.GetInt("WAIT_TIME_SECONDS", "1"))`},
			{Name: "VisibilityTimeout", Value: `int32(scope.GetInt("VISIBILITY_TIMEOUT_SECONDS", "30"))`},
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

// GenerateEventFiles writes event accessors whose runtime choices are bounded by the generated manifest.
func GenerateEventFiles(projectDir string) (int, error) {
	return generateEventFiles(ambientGenerationInput(projectDir))
}

// generateEventFiles uses one captured environment for validation, rendering, and named-resource discovery.
func generateEventFiles(input generationInput) (int, error) {
	if err := validatePrimitiveEnv(input, primitiveEnvContract{
		Prefix:        "EVENTS",
		DefaultDriver: "inproc",
		RootKeys:      eventRootKeys,
		CommonKeys:    eventCommonKeys,
		DriverKeys:    eventDriverKeys,
		ChildNames: func(environment generationEnvironment) []string {
			return exactScopedChildNames(environment, "EVENTS", eventRootKeys)
		},
		AllowInactiveRootKeys: true,
		EagerNamedResources:   true,
	}); err != nil {
		return 0, err
	}
	manager, err := renderEventConfig(input)
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated events driver config: %w", err)
	}
	accessors, err := renderEventAccessors(input)
	if err != nil {
		return 0, err
	}
	formattedAccessors, err := format.Source(accessors)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated events accessors: %w", err)
	}
	written := 0
	changed, err := writeGeneratedSource(filepath.Join(input.projectDir, "internal", "events", "manager_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	changed, err = writeGeneratedSource(filepath.Join(input.projectDir, "internal", "events", "accessors_gen.go"), formattedAccessors)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	for _, name := range eventLegacyGeneratedFiles {
		changed, err = removeGeneratedFileIfExists(filepath.Join(input.projectDir, "internal", "events", name))
		if err != nil {
			return written, err
		}
		if changed {
			written++
		}
	}
	return written, nil
}

// renderEventConfig derives imports and compiled choices from the same validated event-driver snapshot.
func renderEventConfig(input generationInput) ([]byte, error) {
	names := discoverEventNames(input)
	selectedDrivers, err := uniqueEventDrivers(input)
	if err != nil {
		return nil, err
	}
	drivers := make([]eventDriverSpec, 0, len(selectedDrivers))
	hasRedis := false
	for _, name := range selectedDrivers {
		spec, ok := eventDriverSpecs[name]
		if !ok {
			continue
		}
		if name == "redis" {
			hasRedis = true
		}
		drivers = append(drivers, spec)
	}
	data := eventConfigTemplateData{
		CompiledDrivers: selectedDrivers,
		Drivers:         drivers,
		HasRedis:        hasRedis,
		Names:           make([]eventAccessorName, 0, len(names)),
	}
	for _, name := range names {
		data.Names = append(data.Names, eventAccessorName{
			Method: str.Of(name).Pascal().String(),
			Field:  str.Of(name).Camel().String(),
			Bus:    name,
		})
	}
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

// renderEventAccessors uses the project snapshot so App-only buses receive the same generated surface as root buses.
func renderEventAccessors(input generationInput) ([]byte, error) {
	names := discoverEventNames(input)
	data := eventAccessorTemplateData{
		Names: make([]eventAccessorName, 0, len(names)),
	}
	for _, name := range names {
		data.Names = append(data.Names, eventAccessorName{
			Method: str.Of(name).Pascal().String(),
			Field:  str.Of(name).Camel().String(),
			Bus:    name,
		})
	}

	tmpl, err := template.New("eventsAccessors").Parse(eventsAccessorsTemplate)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// uniqueEventDrivers resolves the complete event build contract without allowing active App overlays to be omitted.
func uniqueEventDrivers(input generationInput) ([]string, error) {
	seen := map[string]struct{}{}
	driver := effectivePrimitiveDriver(input.environment.Get("EVENTS_DRIVER", "inproc"), "inproc")
	seen[driver] = struct{}{}
	for _, child := range exactScopedChildNames(input.environment, "EVENTS", eventRootKeys) {
		driver := effectivePrimitiveDriver(input.environment.Get("EVENTS_"+child+"_DRIVER", ""), "inproc")
		seen[driver] = struct{}{}
	}
	for _, child := range discoverEventNames(input) {
		driver := effectivePrimitiveDriver(input.environment.Get("EVENTS_"+str.Of(child).Snake("_").ToUpper().String()+"_DRIVER", ""), "inproc")
		seen[driver] = struct{}{}
	}
	for _, active := range appPrefixedActiveDrivers(input, "EVENTS", "inproc", false) {
		seen[active.driver] = struct{}{}
	}
	return supportedDrivers(input.environment, "EVENTS", eventDriverKeys, sortStrings(seen))
}

// discoverEventNames includes event buses declared only through a configured App overlay.
func discoverEventNames(input generationInput) []string {
	names := discoverPrimitiveChildNames(input, "EVENTS", eventRootKeys)
	for i := range names {
		names[i] = str.Of(names[i]).TrimSpace().ToLower().String()
	}
	sort.Strings(names)
	return names
}

const eventsConfigTemplate = `package events

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"strings"
	"time"

	goforjevents "github.com/goforj/events"
	eventscore "github.com/goforj/events/eventscore"
	"github.com/goforj/env/v2"
	"github.com/goforj/str"
{{- range .Drivers }}
	"{{ .ImportPath }}"
{{- end }}
{{- if .HasRedis }}
	"github.com/redis/go-redis/v9"
{{- end }}
{{- if .HasNATSJetStream }}
	"github.com/nats-io/nats.go/jetstream"
{{- end }}
)

const defaultBusName = "default"

var compiledEventDrivers = []string{
{{- range .CompiledDrivers }}
	"{{ . }}",
{{- end }}
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

// Manager owns the event buses generated from the project's build contract.
type Manager struct {
	defaultBus Bus
	observer Observer
{{- range .Names }}
	{{ .Field }} Bus
{{- end }}
}

// Instance gives tooling a uniform view of each generated event bus.
type Instance struct {
	Name      string
	Bus       Bus
	IsDefault bool
}

// ReadinessCheck pairs a stable event-bus name with its health probe.
type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

// Observer decouples generated event instrumentation from its metrics and tracing consumers.
type Observer interface {
	// OnEventPublish observes the outcome of each publish attempt across generated buses.
	OnEventPublish(ctx context.Context, event EventPublishEvent)
	// OnEventSubscribe observes successful and failed handler registrations across generated buses.
	OnEventSubscribe(ctx context.Context, event EventSubscriptionEvent)
	// OnEventUnsubscribe observes subscription shutdown without coupling callers to a transport.
	OnEventUnsubscribe(ctx context.Context, event EventSubscriptionEvent)
	// OnEventDeliveryStart marks the beginning of an instrumented handler invocation.
	OnEventDeliveryStart(ctx context.Context, event EventDeliveryEvent)
	// OnEventDeliveryFinish reports the result and duration of an instrumented handler invocation.
	OnEventDeliveryFinish(ctx context.Context, event EventDeliveryEvent)
}

// ObserverFunc lets publish-only callbacks participate in the broader event observer contract.
type ObserverFunc func(ctx context.Context, event EventPublishEvent)

// OnEventPublish lets a publish-only callback satisfy the broader Observer contract.
func (fn ObserverFunc) OnEventPublish(ctx context.Context, event EventPublishEvent) {
	if fn != nil {
		fn(ctx, event)
	}
}

// OnEventSubscribe intentionally ignores subscription notifications for publish-only callbacks.
func (ObserverFunc) OnEventSubscribe(context.Context, EventSubscriptionEvent) {}

// OnEventUnsubscribe intentionally ignores unsubscribe notifications for publish-only callbacks.
func (ObserverFunc) OnEventUnsubscribe(context.Context, EventSubscriptionEvent) {}

// OnEventDeliveryStart intentionally ignores delivery-start notifications for publish-only callbacks.
func (ObserverFunc) OnEventDeliveryStart(context.Context, EventDeliveryEvent) {}

// OnEventDeliveryFinish intentionally ignores delivery-finish notifications for publish-only callbacks.
func (ObserverFunc) OnEventDeliveryFinish(context.Context, EventDeliveryEvent) {}

// observerChain retains multiple event observers without exposing composition to callers.
type observerChain []Observer

// OnEventPublish preserves registration order while fanning publish notifications out across observers.
func (c observerChain) OnEventPublish(ctx context.Context, event EventPublishEvent) {
	for _, observer := range c {
		if observer == nil {
			continue
		}
		observer.OnEventPublish(ctx, event)
	}
}

// OnEventSubscribe preserves registration order while fanning subscription notifications out across observers.
func (c observerChain) OnEventSubscribe(ctx context.Context, event EventSubscriptionEvent) {
	for _, observer := range c {
		if observer == nil {
			continue
		}
		observer.OnEventSubscribe(ctx, event)
	}
}

// OnEventUnsubscribe preserves registration order while fanning unsubscribe notifications out across observers.
func (c observerChain) OnEventUnsubscribe(ctx context.Context, event EventSubscriptionEvent) {
	for _, observer := range c {
		if observer == nil {
			continue
		}
		observer.OnEventUnsubscribe(ctx, event)
	}
}

// OnEventDeliveryStart preserves registration order while fanning delivery-start notifications out across observers.
func (c observerChain) OnEventDeliveryStart(ctx context.Context, event EventDeliveryEvent) {
	for _, observer := range c {
		if observer == nil {
			continue
		}
		observer.OnEventDeliveryStart(ctx, event)
	}
}

// OnEventDeliveryFinish preserves registration order while fanning delivery-finish notifications out across observers.
func (c observerChain) OnEventDeliveryFinish(ctx context.Context, event EventDeliveryEvent) {
	for _, observer := range c {
		if observer == nil {
			continue
		}
		observer.OnEventDeliveryFinish(ctx, event)
	}
}

// NewManager builds the configured event buses with a background lifecycle context.
func NewManager() (*Manager, error) {
	return NewManagerWithContext(context.Background())
}

// NewManagerWithContext builds the configured event buses with caller-controlled initialization context.
func NewManagerWithContext(ctx context.Context) (*Manager, error) {
	return newManagerFromEnv(normalizeEventsContext(ctx), env.WithPrefix("EVENTS"))
}

// NewBus preserves single-bus construction while surfacing initialization failures through an error bus.
func NewBus(ctx context.Context) Bus {
	manager, err := NewManagerWithContext(ctx)
	if err != nil {
		return newErrorBus(ActiveDriver(), err)
	}
	return manager.Default()
}

// WithObserver instruments every managed bus without replacing observers already attached to the manager.
func (m *Manager) WithObserver(observer Observer) *Manager {
	if observer == nil {
		return m
	}
	if m.observer == nil {
		m.observer = observer
	} else {
		switch existing := m.observer.(type) {
		case observerChain:
			m.observer = append(existing, observer)
		default:
			m.observer = observerChain{existing, observer}
		}
	}
	combined := m.observer
	m.defaultBus = wrapObservedBus("default", m.defaultBus, combined)
{{- range .Names }}
	m.{{ .Field }} = wrapObservedBus("{{ .Bus }}", m.{{ .Field }}, combined)
{{- end }}
	return m
}

// ReadinessChecks exposes an independently named health probe for every generated event bus.
func (m *Manager) ReadinessChecks() []ReadinessCheck {
	checks := []ReadinessCheck{
		{
			Name: "events_default",
			Check: func(ctx context.Context) error {
				return eventsReadinessCheck(ctx, m.defaultBus)
			},
		},
	}
{{- range .Names }}
	checks = append(checks, ReadinessCheck{
		Name: "events_{{ .Bus }}",
		Check: func(ctx context.Context) error {
			return eventsReadinessCheck(ctx, m.{{ .Field }})
		},
	})
{{- end }}
	return checks
}

// ActiveDriver reports the root event driver selected by EVENTS_* configuration.
func ActiveDriver() Driver {
	return activeDriverForScope(env.WithPrefix("EVENTS"))
}

// activeDriverNameForScope preserves invalid runtime values so startup can report the actual selection.
func activeDriverNameForScope(scope env.Scope) string {
	value := str.Of(scope.Get("DRIVER", "inproc")).TrimSpace().ToLower().String()
	if value == "" {
		return "inproc"
	}
	return value
}

// activeDriverForScope maps validated environment names onto the runtime driver enumeration.
func activeDriverForScope(scope env.Scope) Driver {
	value := activeDriverNameForScope(scope)
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

// newManagerFromEnv keeps default and named buses on the same scoped configuration path.
func newManagerFromEnv(ctx context.Context, eventsScope env.Scope) (*Manager, error) {
	defaultBus, err := buildBus(ctx, eventsScope)
	if err != nil {
		return nil, err
	}
	manager := &Manager{defaultBus: defaultBus}
{{- range .Names }}
	bus{{ .Method }}, err := buildBus(ctx, eventsScope.Child(str.Of("{{ .Bus }}").Snake("_").ToUpper().String()))
	if err != nil {
		return nil, err
	}
	manager.{{ .Field }} = bus{{ .Method }}
{{- end }}
	return manager, nil
}

// buildBus rejects drivers outside the artifact manifest before any infrastructure is initialized.
func buildBus(ctx context.Context, scope env.Scope) (Bus, error) {
	driverName := activeDriverNameForScope(scope)
	if !eventDriverCompiled(driverName) {
		return nil, fmt.Errorf("events: active driver %q is not built in; compiled choices: %s; run forj generate --events after updating EVENTS_SUPPORTED_DRIVERS", driverName, strings.Join(compiledEventDrivers, ", "))
	}
	switch activeDriverForScope(scope) {
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
			return nil, err
		}
		api, err := goforjevents.New(goforjevents.Config{
			Driver: driverKind({{ .CaseName }}),
			Transport: driver,
		})
		if err != nil {
			return nil, err
		}
		return newManagedBus(api, driver.Ready, func(context.Context) error {
			if closer, ok := any(driver).(interface{ Close() error }); ok {
				return closer.Close()
			}
			return nil
		}), nil
{{- end }}
	case DriverNull:
		api, err := goforjevents.NewNull()
		if err != nil {
			return nil, err
		}
		return newManagedBus(api, nil, nil), nil
	default:
		if activeDriverForScope(scope) == DriverInproc {
			api, err := goforjevents.NewSync()
			if err != nil {
				return nil, err
			}
			return newManagedBus(api, nil, nil), nil
		}
		return nil, fmt.Errorf("events driver %q was not generated", activeDriverForScope(scope))
	}
}

// eventDriverCompiled reports whether driver is selectable in this generated artifact.
func eventDriverCompiled(driver string) bool {
	for _, compiled := range compiledEventDrivers {
		if driver == compiled {
			return true
		}
	}
	return false
}

// eventsRedisAddr preserves resource-specific endpoints while sharing the global Redis fallback contract.
func eventsRedisAddr(scope env.Scope) string {
	addr := str.Of(scope.Get("ADDR", "")).TrimSpace().String()
	if addr != "" {
		return addr
	}
	return fmt.Sprintf("%s:%s", env.Get("REDIS_HOST", "redis"), env.Get("REDIS_PORT", "6379"))
}

{{- if .HasRedis }}
// eventsRedisClient keeps events on the same authenticated Redis connection contract as other primitives.
func eventsRedisClient(scope env.Scope) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     eventsRedisAddr(scope),
		Password: env.Get("REDIS_PASSWORD", ""),
		DB:       env.GetInt("REDIS_DB", "0"),
	})
}
{{- end }}

// eventsCSV normalizes list-valued settings so blank entries never reach a driver.
func eventsCSV(scope env.Scope, key string, fallback string) []string {
	raw := str.Of(scope.Get(key, fallback)).TrimSpace().String()
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

// eventsDurationSeconds keeps second-based driver settings typed at their configuration boundary.
func eventsDurationSeconds(scope env.Scope, key string, fallback int) time.Duration {
	return time.Duration(scope.GetInt(key, fmt.Sprintf("%d", fallback))) * time.Second
}

// eventsDurationMilliseconds keeps millisecond-based driver settings typed at their configuration boundary.
func eventsDurationMilliseconds(scope env.Scope, key string, fallback int) time.Duration {
	return time.Duration(scope.GetInt(key, fmt.Sprintf("%d", fallback))) * time.Millisecond
}

{{- if .HasNATSJetStream }}
// eventsJetStreamStorage defaults unknown values to memory so local event buses remain self-contained.
func eventsJetStreamStorage(value string) jetstream.StorageType {
	switch str.Of(value).TrimSpace().ToLower().String() {
	case "file":
		return jetstream.FileStorage
	default:
		return jetstream.MemoryStorage
	}
}
{{- end }}

// driverKind translates generated driver choices into the eventscore API's stable enumeration.
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

// eventsReadinessCheck binds readiness work to the caller's context before probing the transport.
func eventsReadinessCheck(ctx context.Context, bus Bus) error {
	return bus.WithContext(normalizeEventsContext(ctx)).Ready()
}

// observedBus decorates any event transport with context-aware lifecycle telemetry.
type observedBus struct {
	name     string
	inner    Bus
	observer Observer
	ctx      context.Context
}

// observedSubscription emits one unsubscribe event even when callers close repeatedly.
type observedSubscription struct {
	inner    Subscription
	observer Observer
	ctx      context.Context
	name     string
	topic    string
	handler  string
	driver   Driver
	once     sync.Once
}

// wrapObservedBus adds instrumentation once while allowing an existing wrapper's observer chain to evolve.
func wrapObservedBus(name string, bus Bus, observer Observer) Bus {
	if observer == nil {
		return bus
	}
	if wrapped, ok := bus.(*observedBus); ok {
		wrapped.name = name
		wrapped.observer = observer
		return wrapped
	}
	return &observedBus{
		name:     name,
		inner:    bus,
		observer: observer,
	}
}

// Driver preserves the underlying transport identity through the observer wrapper.
func (b *observedBus) Driver() Driver {
	return b.inner.Driver()
}

// Ready probes the underlying bus with the context bound to this wrapper.
func (b *observedBus) Ready() error {
	return b.inner.WithContext(b.context()).Ready()
}

// WithContext clones the wrapper so per-call context binding cannot mutate a shared bus.
func (b *observedBus) WithContext(ctx context.Context) API {
	clone := *b
	clone.ctx = normalizeEventsContext(ctx)
	return &clone
}

// Publish reports the delegated publish outcome and duration to the configured observer.
func (b *observedBus) Publish(event any) error {
	startedAt := time.Now()
	ctx := b.context()
	err := b.inner.WithContext(ctx).Publish(event)
	b.observer.OnEventPublish(ctx, EventPublishEvent{
		Bus:      eventBusLabel(b.name),
		Topic:    eventTopicLabel(event),
		Err:      err,
		Duration: time.Since(startedAt),
		Driver:   b.inner.Driver(),
	})
	return err
}

// Subscribe instruments delivery and lifecycle notifications while preserving the handler's original signature.
func (b *observedBus) Subscribe(handler any) (Subscription, error) {
	ctx := b.context()
	wrappedHandler, topic, handlerName, err := wrapObservedHandler(handler, b.observer, b.name, b.inner.Driver())
	if err != nil {
		b.observer.OnEventSubscribe(ctx, EventSubscriptionEvent{
			Bus:     eventBusLabel(b.name),
			Topic:   topic,
			Handler: handlerName,
			Err:     err,
			Driver:  b.inner.Driver(),
		})
		return nil, err
	}
	sub, err := b.inner.WithContext(ctx).Subscribe(wrappedHandler)
	b.observer.OnEventSubscribe(ctx, EventSubscriptionEvent{
		Bus:     eventBusLabel(b.name),
		Topic:   topic,
		Handler: handlerName,
		Err:     err,
		Driver:  b.inner.Driver(),
	})
	if err != nil {
		return nil, err
	}
	return &observedSubscription{
		inner:    sub,
		observer: b.observer,
		ctx:      ctx,
		name:     eventBusLabel(b.name),
		topic:    topic,
		handler:  handlerName,
		driver:   b.inner.Driver(),
	}, nil
}

// context guarantees that observer callbacks and delegated calls always receive a usable context.
func (b *observedBus) context() context.Context {
	if b.ctx == nil {
		return context.Background()
	}
	return b.ctx
}

// Start preserves the managed lifecycle behavior of the wrapped bus.
func (b *observedBus) Start(ctx context.Context) error {
	return b.inner.Start(ctx)
}

// Close preserves the managed shutdown behavior of the wrapped bus.
func (b *observedBus) Close(ctx context.Context) error {
	return b.inner.Close(ctx)
}

// Close emits at most one unsubscribe notification after the underlying subscription closes successfully.
func (s *observedSubscription) Close() error {
	var err error
	s.once.Do(func() {
		err = s.inner.Close()
		if err == nil {
			s.observer.OnEventUnsubscribe(s.ctx, EventSubscriptionEvent{
				Bus:     s.name,
				Topic:   s.topic,
				Handler: s.handler,
				Driver:  s.driver,
			})
		}
	})
	return err
}

// eventBusLabel keeps observer labels stable for the unnamed default bus.
func eventBusLabel(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultBusName
	}
	return name
}

// eventTopicLabel derives a stable observer topic when an event does not provide one explicitly.
func eventTopicLabel(event any) string {
	if event == nil {
		return "unknown"
	}
	if topicEvent, ok := event.(interface{ Topic() string }); ok {
		if topic := strings.TrimSpace(topicEvent.Topic()); topic != "" {
			return topic
		}
	}
	typ := reflect.TypeOf(event)
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil {
		return "unknown"
	}
	name := strings.TrimSpace(typ.Name())
	if name == "" {
		return "unknown"
	}
	return strings.ReplaceAll(str.Of(name).Snake("_").String(), "_", ".")
}

// wrapObservedHandler validates supported handler shapes before adding reflected delivery instrumentation.
func wrapObservedHandler(handler any, observer Observer, busName string, driver Driver) (any, string, string, error) {
	if handler == nil {
		return nil, "unknown", "unknown", goforjevents.ErrInvalidHandler
	}
	fn := reflect.ValueOf(handler)
	if fn.Kind() != reflect.Func {
		return nil, "unknown", eventHandlerLabel(fn), fmt.Errorf("%w: handler must be a function", goforjevents.ErrInvalidHandler)
	}
	typ := fn.Type()
	if typ.NumIn() < 1 || typ.NumIn() > 2 {
		return nil, "unknown", eventHandlerLabel(fn), fmt.Errorf("%w: handler must accept 1 or 2 arguments", goforjevents.ErrInvalidHandler)
	}
	if typ.NumOut() > 1 {
		return nil, "unknown", eventHandlerLabel(fn), fmt.Errorf("%w: handler must return zero or one value", goforjevents.ErrInvalidHandler)
	}
	eventIndex := 0
	if typ.NumIn() == 2 {
		if !typ.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
			return nil, "unknown", eventHandlerLabel(fn), fmt.Errorf("%w: first argument must implement context.Context", goforjevents.ErrInvalidHandler)
		}
		eventIndex = 1
	}
	eventType := typ.In(eventIndex)
	if eventType.Kind() == reflect.Interface {
		return nil, "unknown", eventHandlerLabel(fn), fmt.Errorf("%w: event argument must be a concrete type", goforjevents.ErrInvalidHandler)
	}
	if typ.NumOut() == 1 && !typ.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return nil, "unknown", eventHandlerLabel(fn), fmt.Errorf("%w: return value must be error", goforjevents.ErrInvalidHandler)
	}
	sample := sampleEventValue(eventType)
	topic := eventTopicLabel(sample.Interface())
	handlerName := eventHandlerLabel(fn)
	wrapped := reflect.MakeFunc(typ, func(args []reflect.Value) []reflect.Value {
		callCtx := context.Background()
		if typ.NumIn() == 2 && len(args) > 0 && !args[0].IsNil() {
			if ctxArg, ok := args[0].Interface().(context.Context); ok && ctxArg != nil {
				callCtx = ctxArg
			}
		}
		if observer != nil {
			observer.OnEventDeliveryStart(callCtx, EventDeliveryEvent{
				Bus:     eventBusLabel(busName),
				Topic:   topic,
				Handler: handlerName,
				Driver:  driver,
			})
		}
		startedAt := time.Now()
		out := fn.Call(args)
		var callErr error
		if typ.NumOut() == 1 && len(out) == 1 && !out[0].IsNil() {
			callErr = out[0].Interface().(error)
		}
		if observer != nil {
			observer.OnEventDeliveryFinish(callCtx, EventDeliveryEvent{
				Bus:      eventBusLabel(busName),
				Topic:    topic,
				Handler:  handlerName,
				Err:      callErr,
				Duration: time.Since(startedAt),
				Driver:   driver,
			})
		}
		return out
	})
	return wrapped.Interface(), topic, handlerName, nil
}

// sampleEventValue creates a representative value so topic labels can be derived before delivery.
func sampleEventValue(typ reflect.Type) reflect.Value {
	if typ.Kind() == reflect.Pointer {
		return reflect.New(indirectType(typ))
	}
	return reflect.Zero(typ)
}

// indirectType resolves pointer layers before constructing a representative event value.
func indirectType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

// eventHandlerLabel favors a stable runtime function name while safely handling invalid reflected values.
func eventHandlerLabel(fn reflect.Value) string {
	if !fn.IsValid() || fn.Kind() != reflect.Func {
		return "unknown"
	}
	runtimeFn := runtime.FuncForPC(fn.Pointer())
	if runtimeFn == nil {
		return "unknown"
	}
	name := strings.TrimSpace(runtimeFn.Name())
	if name == "" {
		return "unknown"
	}
	if slash := strings.LastIndex(name, "/"); slash >= 0 && slash+1 < len(name) {
		name = name[slash+1:]
	}
	return name
}
`

const eventsAccessorsTemplate = `// Code generated by forj generate --events. DO NOT EDIT.
// Run: forj generate --events
//
// These event manager accessors are derived from the current .env file
// and environment variables available when generation ran.
// Named accessors are generated from EVENTS_<NAME>_<KEY> environment variables.
package events

import "strings"

// Default returns the default event bus instance derived from EVENTS_* configuration.
func (m *Manager) Default() Bus {
	return m.defaultBus
}

// Named returns the event bus instance generated for a configured bus name.
func (m *Manager) Named(name string) Bus {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "default":
		return m.defaultBus
{{- range .Names }}
	case "{{ .Bus }}":
		return m.{{ .Field }}
{{- end }}
	default:
		return nil
	}
}

{{- range .Names }}
// {{ .Method }} returns the "{{ .Bus }}" event bus instance.
func (m *Manager) {{ .Method }}() Bus {
	return m.{{ .Field }}
}

{{- end }}
// Instances returns the generated event bus instances derived from EVENTS_* configuration.
func (m *Manager) Instances() []Instance {
	instances := []Instance{
		{Name: "default", Bus: m.defaultBus, IsDefault: true},
	}
{{- range .Names }}
	instances = append(instances, Instance{Name: "{{ .Bus }}", Bus: m.{{ .Field }}})
{{- end }}
	return instances
}
`

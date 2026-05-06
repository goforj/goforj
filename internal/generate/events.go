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
	Names            []eventAccessorName
}

type eventAccessorTemplateData struct {
	Names []eventAccessorName
}

type eventAccessorName struct {
	Method string
	Field  string
	Bus    string
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
			{Name: "Addr", Value: `eventsRedisAddr(scope)`},
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

func GenerateEventFiles(projectDir string) (int, error) {
	if err := validatePrimitiveEnv(primitiveEnvContract{
		Prefix:        "EVENTS",
		DefaultDriver: "inproc",
		RootKeys:      eventRootKeys,
		CommonKeys:    eventCommonKeys,
		DriverKeys:    eventDriverKeys,
		ChildNames: func(scope env.Scope) []string {
			return scope.ChildNames(eventRootKeys)
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
	accessors, err := renderEventAccessors()
	if err != nil {
		return 0, err
	}
	formattedAccessors, err := format.Source(accessors)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated events accessors: %w", err)
	}
	written := 0
	changed, err := writeGeneratedSource(filepath.Join(projectDir, "internal", "events", "manager_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	changed, err = writeGeneratedSource(filepath.Join(projectDir, "internal", "events", "accessors_gen.go"), formattedAccessors)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "driver.go"))
	_ = os.Remove(filepath.Join(projectDir, "internal", "events", "driver_gen.go"))
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
	names := discoverEventNames()
	selectedDrivers, err := uniqueEventDrivers()
	if err != nil {
		return nil, err
	}
	drivers := make([]eventDriverSpec, 0, len(selectedDrivers))
	for _, name := range selectedDrivers {
		spec, ok := eventDriverSpecs[name]
		if !ok {
			continue
		}
		drivers = append(drivers, spec)
	}
	data := eventConfigTemplateData{
		Drivers: drivers,
		Names:   make([]eventAccessorName, 0, len(names)),
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

func renderEventAccessors() ([]byte, error) {
	names := discoverEventNames()
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

func uniqueEventDrivers() ([]string, error) {
	seen := map[string]struct{}{}
	scope := env.WithPrefix("EVENTS")
	driver := str.Of(scope.Get("DRIVER", "inproc")).TrimSpace().ToLower().String()
	if driver == "" {
		driver = "inproc"
	}
	seen[driver] = struct{}{}
	for _, child := range scope.ChildNames(eventRootKeys) {
		driver := str.Of(scope.Child(child).Get("DRIVER", "")).TrimSpace().ToLower().String()
		if driver == "" {
			continue
		}
		seen[driver] = struct{}{}
	}
	return supportedDrivers("EVENTS", eventDriverKeys, sortStrings(seen))
}

func discoverEventNames() []string {
	names := env.WithPrefix("EVENTS").ChildNames(eventRootKeys)
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
{{- if .HasNATSJetStream }}
	"github.com/nats-io/nats.go/jetstream"
{{- end }}
)

const defaultBusName = "default"

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

type Manager struct {
	defaultBus Bus
{{- range .Names }}
	{{ .Field }} Bus
{{- end }}
}

type Instance struct {
	Name      string
	Bus       Bus
	IsDefault bool
}

type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

type Observer interface {
	OnEventPublish(ctx context.Context, name string, topic string, err error, dur time.Duration, driver Driver)
	OnEventSubscribe(ctx context.Context, name string, topic string, handler string, err error, driver Driver)
	OnEventUnsubscribe(ctx context.Context, name string, topic string, handler string, driver Driver)
	OnEventDeliveryStart(ctx context.Context, name string, topic string, handler string, driver Driver)
	OnEventDeliveryFinish(ctx context.Context, name string, topic string, handler string, err error, dur time.Duration, driver Driver)
}

type ObserverFunc func(ctx context.Context, name string, topic string, err error, dur time.Duration, driver Driver)

func (fn ObserverFunc) OnEventPublish(ctx context.Context, name string, topic string, err error, dur time.Duration, driver Driver) {
	if fn != nil {
		fn(ctx, name, topic, err, dur, driver)
	}
}

func (ObserverFunc) OnEventSubscribe(context.Context, string, string, string, error, Driver) {}

func (ObserverFunc) OnEventUnsubscribe(context.Context, string, string, string, Driver) {}

func (ObserverFunc) OnEventDeliveryStart(context.Context, string, string, string, Driver) {}

func (ObserverFunc) OnEventDeliveryFinish(context.Context, string, string, string, error, time.Duration, Driver) {}

func NewManager() (*Manager, error) {
	return NewManagerWithContext(context.Background())
}

func NewManagerWithContext(ctx context.Context) (*Manager, error) {
	return newManagerFromEnv(normalizeEventsContext(ctx), env.WithPrefix("EVENTS"))
}

func NewBus(ctx context.Context) Bus {
	manager, err := NewManagerWithContext(ctx)
	if err != nil {
		return newErrorBus(ActiveDriver(), err)
	}
	return manager.Default()
}

func (m *Manager) WithObserver(observer Observer) *Manager {
	if m == nil || observer == nil {
		return m
	}
	m.defaultBus = wrapObservedBus("default", m.defaultBus, observer)
{{- range .Names }}
	m.{{ .Field }} = wrapObservedBus("{{ .Bus }}", m.{{ .Field }}, observer)
{{- end }}
	return m
}

func (m *Manager) ReadinessChecks() []ReadinessCheck {
	if m == nil {
		return nil
	}
	checks := []ReadinessCheck{
		{
			Name: "events_default",
			Check: func(ctx context.Context) error {
				return eventsReadinessCheck(ctx, m.defaultBus)
			},
		},
	}
{{- range .Names }}
	if m.{{ .Field }} != nil {
		checks = append(checks, ReadinessCheck{
			Name: "events_{{ .Bus }}",
			Check: func(ctx context.Context) error {
				return eventsReadinessCheck(ctx, m.{{ .Field }})
			},
		})
	}
{{- end }}
	return checks
}

func ActiveDriver() Driver {
	return activeDriverForScope(env.WithPrefix("EVENTS"))
}

func activeDriverForScope(scope env.Scope) Driver {
	value := str.Of(scope.Get("DRIVER", "inproc")).TrimSpace().ToLower().String()
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

func newManagerFromEnv(ctx context.Context, eventsScope env.Scope) (*Manager, error) {
	defaultBus, err := buildBus(ctx, eventsScope)
	if err != nil {
		return nil, err
	}
	manager := &Manager{defaultBus: defaultBus}
{{- if .Names }}
	for _, child := range eventsScope.ChildNames(eventRootKeys) {
		name := str.Of(child).TrimSpace().ToLower().String()
		if name == "" {
			continue
		}
		bus, err := buildBus(ctx, eventsScope.Child(child))
		if err != nil {
			return nil, err
		}
		switch name {
{{- range .Names }}
		case "{{ .Bus }}":
			manager.{{ .Field }} = bus
{{- end }}
		}
	}
{{- end }}
	return manager, nil
}

func buildBus(ctx context.Context, scope env.Scope) (Bus, error) {
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

func eventsRedisAddr(scope env.Scope) string {
	addr := str.Of(scope.Get("ADDR", "")).TrimSpace().String()
	if addr != "" {
		return addr
	}
	return fmt.Sprintf("%s:%s", env.Get("REDIS_HOST", "redis"), env.Get("REDIS_PORT", "6379"))
}

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

func eventsDurationSeconds(scope env.Scope, key string, fallback int) time.Duration {
	return time.Duration(scope.GetInt(key, fmt.Sprintf("%d", fallback))) * time.Second
}

func eventsDurationMilliseconds(scope env.Scope, key string, fallback int) time.Duration {
	return time.Duration(scope.GetInt(key, fmt.Sprintf("%d", fallback))) * time.Millisecond
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

func eventsReadinessCheck(ctx context.Context, bus Bus) error {
	if bus == nil {
		return nil
	}
	return bus.WithContext(normalizeEventsContext(ctx)).Ready()
}

type observedBus struct {
	name     string
	inner    Bus
	observer Observer
	ctx      context.Context
}

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

func wrapObservedBus(name string, bus Bus, observer Observer) Bus {
	if bus == nil || observer == nil {
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

func (b *observedBus) Driver() Driver {
	return b.inner.Driver()
}

func (b *observedBus) Ready() error {
	return b.inner.WithContext(b.context()).Ready()
}

func (b *observedBus) WithContext(ctx context.Context) API {
	clone := *b
	clone.ctx = normalizeEventsContext(ctx)
	return &clone
}

func (b *observedBus) Publish(event any) error {
	startedAt := time.Now()
	ctx := b.context()
	err := b.inner.WithContext(ctx).Publish(event)
	b.observer.OnEventPublish(ctx, eventBusLabel(b.name), eventTopicLabel(event), err, time.Since(startedAt), b.inner.Driver())
	return err
}

func (b *observedBus) Subscribe(handler any) (Subscription, error) {
	ctx := b.context()
	wrappedHandler, topic, handlerName, err := wrapObservedHandler(handler, b.observer, b.name, b.inner.Driver())
	if err != nil {
		b.observer.OnEventSubscribe(ctx, eventBusLabel(b.name), topic, handlerName, err, b.inner.Driver())
		return nil, err
	}
	sub, err := b.inner.WithContext(ctx).Subscribe(wrappedHandler)
	b.observer.OnEventSubscribe(ctx, eventBusLabel(b.name), topic, handlerName, err, b.inner.Driver())
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

func (b *observedBus) context() context.Context {
	if b == nil || b.ctx == nil {
		return context.Background()
	}
	return b.ctx
}

func (b *observedBus) Start(ctx context.Context) error {
	return b.inner.Start(ctx)
}

func (b *observedBus) Close(ctx context.Context) error {
	return b.inner.Close(ctx)
}

func (s *observedSubscription) Close() error {
	var err error
	s.once.Do(func() {
		err = s.inner.Close()
		if err == nil && s.observer != nil {
			s.observer.OnEventUnsubscribe(s.ctx, s.name, s.topic, s.handler, s.driver)
		}
	})
	return err
}

func eventBusLabel(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultBusName
	}
	return name
}

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
			observer.OnEventDeliveryStart(callCtx, eventBusLabel(busName), topic, handlerName, driver)
		}
		startedAt := time.Now()
		out := fn.Call(args)
		var callErr error
		if typ.NumOut() == 1 && len(out) == 1 && !out[0].IsNil() {
			callErr = out[0].Interface().(error)
		}
		if observer != nil {
			observer.OnEventDeliveryFinish(callCtx, eventBusLabel(busName), topic, handlerName, callErr, time.Since(startedAt), driver)
		}
		return out
	})
	return wrapped.Interface(), topic, handlerName, nil
}

func sampleEventValue(typ reflect.Type) reflect.Value {
	if typ.Kind() == reflect.Pointer {
		return reflect.New(indirectType(typ))
	}
	return reflect.Zero(typ)
}

func indirectType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

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

// Default returns the default event bus instance derived from EVENTS_* configuration.
func (m *Manager) Default() Bus {
	return m.defaultBus
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
	if m.{{ .Field }} != nil {
		instances = append(instances, Instance{Name: "{{ .Bus }}", Bus: m.{{ .Field }}})
	}
{{- end }}
	return instances
}
`

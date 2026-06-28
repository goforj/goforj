package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateEventFilesUsesSelectedDriverImports(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "redis")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "events"), 0o755); err != nil {
		t.Fatalf("mkdir events package: %v", err)
	}

	written, err := GenerateEventFiles(root)
	if err != nil {
		t.Fatalf("GenerateEventFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated event files to be written")
	}

	driverGen, err := os.ReadFile(filepath.Join(root, "internal", "events", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source := string(driverGen)

	if !strings.Contains(source, `"github.com/goforj/events/driver/redisevents"`) {
		t.Fatal("expected generated events config to import redisevents")
	}
	if strings.Contains(source, `"github.com/goforj/events/driver/natsevents"`) {
		t.Fatal("did not expect generated events config to import natsevents")
	}
	if !strings.Contains(source, `goforjevents.New(goforjevents.Config{`) {
		t.Fatal("expected generated events manager to construct root events API")
	}
	if !strings.Contains(source, `return newManagedBus(api, driver.Ready`) {
		t.Fatal("expected generated events manager to wrap redis driver in managed bus")
	}
	if !strings.Contains(source, `type Manager struct`) {
		t.Fatal("expected generated events manager type")
	}
	if !strings.Contains(source, `func NewManager() (*Manager, error)`) {
		t.Fatal("expected generated events manager constructor")
	}
}

func TestGenerateEventFilesUsesSupportedDriverImports(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "inproc")
	t.Setenv("EVENTS_SUPPORTED_DRIVERS", "inproc,redis")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "events"), 0o755); err != nil {
		t.Fatalf("mkdir events package: %v", err)
	}

	if _, err := GenerateEventFiles(root); err != nil {
		t.Fatalf("GenerateEventFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "events", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source := string(managerGen)
	if !strings.Contains(source, `"github.com/goforj/events/driver/redisevents"`) {
		t.Fatal("expected generated events manager to import redisevents from EVENTS_SUPPORTED_DRIVERS")
	}
}

func TestGenerateEventFilesBuildsNamedAccessors(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "inproc")
	t.Setenv("EVENTS_AUDIT_DRIVER", "null")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "events"), 0o755); err != nil {
		t.Fatalf("mkdir events package: %v", err)
	}

	if _, err := GenerateEventFiles(root); err != nil {
		t.Fatalf("GenerateEventFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "events", "accessors_gen.go"))
	if err != nil {
		t.Fatalf("read accessors_gen.go: %v", err)
	}
	source := string(managerGen)
	if !strings.Contains(source, `func (m *Manager) Audit() Bus`) {
		t.Fatal("expected generated events manager accessor for named audit bus")
	}
	if !strings.Contains(source, `func (m *Manager) Named(name string) Bus`) {
		t.Fatal("expected generated events manager named lookup")
	}
	if !strings.Contains(source, `case "audit":`) {
		t.Fatal("expected generated events manager named lookup case for audit bus")
	}
}

func TestGenerateEventFilesChainsMultipleObservers(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "inproc")
	t.Setenv("EVENTS_AUDIT_DRIVER", "null")

	root := mustTempGeneratedModuleRoot(t, ".tmp-events-observer-chain-*", filepath.Join("internal", "events"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/eventsobserverchaintest",
		[]string{
			"github.com/goforj/env/v2",
			"github.com/goforj/events",
			"github.com/goforj/events/eventscore",
			"github.com/goforj/str",
		},
		nil,
		eventsLocalReplaces(t),
	))

	if _, err := GenerateEventFiles(root); err != nil {
		t.Fatalf("GenerateEventFiles returned error: %v", err)
	}

	supportSource := `package events

import (
	"context"
	"time"

	goforjevents "github.com/goforj/events"
	"github.com/goforj/events/eventscore"
)

type API = goforjevents.API
type Driver = eventscore.Driver
type Subscription = goforjevents.Subscription

type EventPublishEvent struct {
	Bus      string
	Topic    string
	Err      error
	Duration time.Duration
	Driver   Driver
}

type EventSubscriptionEvent struct {
	Bus     string
	Topic   string
	Handler string
	Err     error
	Driver  Driver
}

type EventDeliveryEvent struct {
	Bus      string
	Topic    string
	Handler  string
	Err      error
	Duration time.Duration
	Driver   Driver
}

const (
	DriverInproc        Driver = eventscore.DriverSync
	DriverNull          Driver = eventscore.DriverNull
	DriverRedis         Driver = eventscore.DriverRedis
	DriverNATS          Driver = eventscore.DriverNATS
	DriverNATSJetStream Driver = eventscore.DriverNATSJetStream
	DriverKafka         Driver = eventscore.DriverKafka
	DriverGCPPubSub     Driver = eventscore.DriverGCPPubSub
	DriverSNS           Driver = eventscore.DriverSNS
)

type Bus interface {
	API
	Start(ctx context.Context) error
	Close(ctx context.Context) error
}

type managedBus struct {
	api   goforjevents.API
	start func(context.Context) error
	close func(context.Context) error
}

type observerRecorder struct {
	onPublish        func(context.Context, EventPublishEvent)
	onSubscribe      func(context.Context, EventSubscriptionEvent)
	onUnsubscribe    func(context.Context, EventSubscriptionEvent)
	onDeliveryStart  func(context.Context, EventDeliveryEvent)
	onDeliveryFinish func(context.Context, EventDeliveryEvent)
}

type errorBus struct {
	driver Driver
	err    error
}

func newManagedBus(api goforjevents.API, start func(context.Context) error, close func(context.Context) error) Bus {
	return &managedBus{api: api, start: start, close: close}
}

func newErrorBus(driver Driver, err error) Bus {
	return &errorBus{driver: driver, err: err}
}

func (o observerRecorder) OnEventPublish(ctx context.Context, event EventPublishEvent) {
	if o.onPublish != nil {
		o.onPublish(ctx, event)
	}
}

func (o observerRecorder) OnEventSubscribe(ctx context.Context, event EventSubscriptionEvent) {
	if o.onSubscribe != nil {
		o.onSubscribe(ctx, event)
	}
}

func (o observerRecorder) OnEventUnsubscribe(ctx context.Context, event EventSubscriptionEvent) {
	if o.onUnsubscribe != nil {
		o.onUnsubscribe(ctx, event)
	}
}

func (o observerRecorder) OnEventDeliveryStart(ctx context.Context, event EventDeliveryEvent) {
	if o.onDeliveryStart != nil {
		o.onDeliveryStart(ctx, event)
	}
}

func (o observerRecorder) OnEventDeliveryFinish(ctx context.Context, event EventDeliveryEvent) {
	if o.onDeliveryFinish != nil {
		o.onDeliveryFinish(ctx, event)
	}
}

func (b *managedBus) Driver() eventscore.Driver                   { return b.api.Driver() }
func (b *managedBus) Ready() error                                { return b.api.Ready() }
func (b *managedBus) WithContext(ctx context.Context) API         { return b.api.WithContext(normalizeEventsContext(ctx)) }
func (b *managedBus) Publish(event any) error                     { return b.api.Publish(event) }
func (b *managedBus) Subscribe(handler any) (Subscription, error) { return b.api.Subscribe(handler) }
func (b *managedBus) Start(ctx context.Context) error {
	if b.start != nil {
		return b.start(normalizeEventsContext(ctx))
	}
	return b.api.WithContext(normalizeEventsContext(ctx)).Ready()
}
func (b *managedBus) Close(ctx context.Context) error {
	if b.close != nil {
		return b.close(normalizeEventsContext(ctx))
	}
	return nil
}

func (b *errorBus) Driver() Driver                                   { return b.driver }
func (b *errorBus) Start(context.Context) error                      { return b.err }
func (b *errorBus) Close(context.Context) error                      { return nil }
func (b *errorBus) Ready() error                                     { return b.err }
func (b *errorBus) WithContext(context.Context) API                  { return b }
func (b *errorBus) Publish(any) error                                { return b.err }
func (b *errorBus) Subscribe(any) (Subscription, error)              { return nil, b.err }

func normalizeEventsContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "events", "support_testtypes.go"), []byte(supportSource), 0o644); err != nil {
		t.Fatalf("write support source: %v", err)
	}

	testSource := `package events

import (
	"context"
	"os"
	"testing"
)

type userCreated struct {
	ID string
}

func (userCreated) Topic() string { return "users.created" }

func TestObserverChain(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	var metricsPublishes int
	var inspectPublishes int
	mgr = mgr.WithObserver(observerRecorder{
		onPublish: func(_ context.Context, event EventPublishEvent) {
			if event.Err != nil {
				t.Fatalf("metrics observer saw error: %v", event.Err)
			}
			if event.Bus == "audit" && event.Topic == "users.created" && event.Driver == DriverNull {
				metricsPublishes++
			}
		},
	})
	mgr = mgr.WithObserver(observerRecorder{
		onPublish: func(_ context.Context, event EventPublishEvent) {
			if event.Err != nil {
				t.Fatalf("inspect observer saw error: %v", event.Err)
			}
			if event.Bus == "audit" && event.Topic == "users.created" && event.Driver == DriverNull {
				inspectPublishes++
			}
		},
	})

	if err := mgr.Audit().Publish(userCreated{ID: "123"}); err != nil {
		t.Fatalf("audit Publish returned error: %v", err)
	}
	if metricsPublishes != 1 {
		t.Fatalf("metrics observer count = %d, want 1", metricsPublishes)
	}
	if inspectPublishes != 1 {
		t.Fatalf("inspect observer count = %d, want 1", inspectPublishes)
	}
}

func TestGeneratedAccessorsFallbackWithoutRuntimeEnv(t *testing.T) {
	for _, key := range []string{
		"EVENTS_DRIVER",
		"EVENTS_AUDIT_DRIVER",
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
		t.Fatal("expected default event bus fallback")
	}
	if mgr.Audit() == nil {
		t.Fatal("expected audit event bus fallback")
	}
	if got := mgr.Audit().Driver(); got != DriverInproc {
		t.Fatalf("Audit driver = %q, want %q", got, DriverInproc)
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "events", "observer_chain_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/events", "TestObserverChain", nil)
}

func TestGenerateEventFilesSupportsObserverWrapping(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "inproc")
	t.Setenv("EVENTS_AUDIT_DRIVER", "null")

	root := mustTempGeneratedModuleRoot(t, ".tmp-events-generation-*", filepath.Join("internal", "events"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/eventsobservertest",
		[]string{
			"github.com/goforj/env/v2",
			"github.com/goforj/events",
			"github.com/goforj/events/eventscore",
			"github.com/goforj/str",
		},
		nil,
		eventsLocalReplaces(t),
	))

	if _, err := GenerateEventFiles(root); err != nil {
		t.Fatalf("GenerateEventFiles returned error: %v", err)
	}

	supportSource := `package events

import (
	"context"
	"time"

	goforjevents "github.com/goforj/events"
	"github.com/goforj/events/eventscore"
)

type API = goforjevents.API
type Driver = eventscore.Driver
type Subscription = goforjevents.Subscription

type EventPublishEvent struct {
	Bus      string
	Topic    string
	Err      error
	Duration time.Duration
	Driver   Driver
}

type EventSubscriptionEvent struct {
	Bus     string
	Topic   string
	Handler string
	Err     error
	Driver  Driver
}

type EventDeliveryEvent struct {
	Bus      string
	Topic    string
	Handler  string
	Err      error
	Duration time.Duration
	Driver   Driver
}

const (
	DriverInproc        Driver = eventscore.DriverSync
	DriverNull          Driver = eventscore.DriverNull
	DriverRedis         Driver = eventscore.DriverRedis
	DriverNATS          Driver = eventscore.DriverNATS
	DriverNATSJetStream Driver = eventscore.DriverNATSJetStream
	DriverKafka         Driver = eventscore.DriverKafka
	DriverGCPPubSub     Driver = eventscore.DriverGCPPubSub
	DriverSNS           Driver = eventscore.DriverSNS
)

type Bus interface {
	API
	Start(ctx context.Context) error
	Close(ctx context.Context) error
}

type managedBus struct {
	api   goforjevents.API
	start func(context.Context) error
	close func(context.Context) error
}

type observerRecorder struct {
	onPublish        func(context.Context, EventPublishEvent)
	onSubscribe      func(context.Context, EventSubscriptionEvent)
	onUnsubscribe    func(context.Context, EventSubscriptionEvent)
	onDeliveryStart  func(context.Context, EventDeliveryEvent)
	onDeliveryFinish func(context.Context, EventDeliveryEvent)
}

type errorBus struct {
	driver Driver
	err    error
}

func newManagedBus(api goforjevents.API, start func(context.Context) error, close func(context.Context) error) Bus {
	return &managedBus{api: api, start: start, close: close}
}

func newErrorBus(driver Driver, err error) Bus {
	return &errorBus{driver: driver, err: err}
}

func (o observerRecorder) OnEventPublish(ctx context.Context, event EventPublishEvent) {
	if o.onPublish != nil {
		o.onPublish(ctx, event)
	}
}

func (o observerRecorder) OnEventSubscribe(ctx context.Context, event EventSubscriptionEvent) {
	if o.onSubscribe != nil {
		o.onSubscribe(ctx, event)
	}
}

func (o observerRecorder) OnEventUnsubscribe(ctx context.Context, event EventSubscriptionEvent) {
	if o.onUnsubscribe != nil {
		o.onUnsubscribe(ctx, event)
	}
}

func (o observerRecorder) OnEventDeliveryStart(ctx context.Context, event EventDeliveryEvent) {
	if o.onDeliveryStart != nil {
		o.onDeliveryStart(ctx, event)
	}
}

func (o observerRecorder) OnEventDeliveryFinish(ctx context.Context, event EventDeliveryEvent) {
	if o.onDeliveryFinish != nil {
		o.onDeliveryFinish(ctx, event)
	}
}

func (b *managedBus) Driver() eventscore.Driver                   { return b.api.Driver() }
func (b *managedBus) Ready() error                                { return b.api.Ready() }
func (b *managedBus) WithContext(ctx context.Context) API         { return b.api.WithContext(normalizeEventsContext(ctx)) }
func (b *managedBus) Publish(event any) error                     { return b.api.Publish(event) }
func (b *managedBus) Subscribe(handler any) (Subscription, error) { return b.api.Subscribe(handler) }
func (b *managedBus) Start(ctx context.Context) error {
	if b.start != nil {
		return b.start(normalizeEventsContext(ctx))
	}
	return b.api.WithContext(normalizeEventsContext(ctx)).Ready()
}
func (b *managedBus) Close(ctx context.Context) error {
	if b.close != nil {
		return b.close(normalizeEventsContext(ctx))
	}
	return nil
}

func (b *errorBus) Driver() Driver                                   { return b.driver }
func (b *errorBus) Start(context.Context) error                      { return b.err }
func (b *errorBus) Close(context.Context) error                      { return nil }
func (b *errorBus) Ready() error                                     { return b.err }
func (b *errorBus) WithContext(context.Context) API                  { return b }
func (b *errorBus) Publish(any) error                                { return b.err }
func (b *errorBus) Subscribe(any) (Subscription, error)              { return nil, b.err }

func normalizeEventsContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "events", "support_testtypes.go"), []byte(supportSource), 0o644); err != nil {
		t.Fatalf("write support source: %v", err)
	}

	testSource := `package events

import (
	"context"
	"strings"
	"testing"
)

type userCreated struct {
	ID string
}

func (userCreated) Topic() string { return "users.created" }

func TestGeneratedObserver(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	var publishes []string
	var lifecycle []string
	mgr = mgr.WithObserver(observerRecorder{
		onPublish: func(_ context.Context, event EventPublishEvent) {
			if event.Err != nil {
				t.Fatalf("observer saw publish error: %v", event.Err)
			}
			publishes = append(publishes, event.Bus+":"+event.Topic+":"+string(event.Driver))
		},
		onSubscribe: func(_ context.Context, event EventSubscriptionEvent) {
			if event.Err != nil {
				t.Fatalf("observer saw subscribe error: %v", event.Err)
			}
			lifecycle = append(lifecycle, "subscribe:"+event.Bus+":"+event.Topic+":"+event.Handler+":"+string(event.Driver))
		},
		onUnsubscribe: func(_ context.Context, event EventSubscriptionEvent) {
			lifecycle = append(lifecycle, "unsubscribe:"+event.Bus+":"+event.Topic+":"+event.Handler+":"+string(event.Driver))
		},
		onDeliveryStart: func(_ context.Context, event EventDeliveryEvent) {
			lifecycle = append(lifecycle, "deliver_start:"+event.Bus+":"+event.Topic+":"+event.Handler+":"+string(event.Driver))
		},
		onDeliveryFinish: func(_ context.Context, event EventDeliveryEvent) {
			if event.Err != nil {
				t.Fatalf("observer saw delivery error: %v", event.Err)
			}
			lifecycle = append(lifecycle, "deliver_finish:"+event.Bus+":"+event.Topic+":"+event.Handler+":"+string(event.Driver))
		},
	})

	defaultSub, err := mgr.Default().Subscribe(func(_ context.Context, payload userCreated) error {
		if payload.ID == "" {
			t.Fatal("default handler received empty id")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("default Subscribe returned error: %v", err)
	}
	defer defaultSub.Close()

	auditSub, err := mgr.Audit().Subscribe(func(payload userCreated) {
		if payload.ID == "" {
			t.Fatal("audit handler received empty id")
		}
	})
	if err != nil {
		t.Fatalf("audit Subscribe returned error: %v", err)
	}
	defer auditSub.Close()

	if err := mgr.Default().Publish(userCreated{ID: "123"}); err != nil {
		t.Fatalf("default Publish returned error: %v", err)
	}
	if err := mgr.Audit().Publish(userCreated{ID: "456"}); err != nil {
		t.Fatalf("audit Publish returned error: %v", err)
	}
	if err := defaultSub.Close(); err != nil {
		t.Fatalf("default Close returned error: %v", err)
	}
	if err := auditSub.Close(); err != nil {
		t.Fatalf("audit Close returned error: %v", err)
	}
	if len(publishes) != 2 {
		t.Fatalf("len(publishes) = %d, want 2", len(publishes))
	}
	if publishes[0] != "default:users.created:sync" {
		t.Fatalf("default observed = %q", publishes[0])
	}
	if publishes[1] != "audit:users.created:null" {
		t.Fatalf("audit observed = %q", publishes[1])
	}
	if len(lifecycle) != 6 {
		t.Fatalf("len(lifecycle) = %d, want 6", len(lifecycle))
	}
	if !strings.Contains(lifecycle[0], "subscribe:default:users.created:") {
		t.Fatalf("default subscribe lifecycle = %q", lifecycle[0])
	}
	if !strings.Contains(lifecycle[1], "subscribe:audit:users.created:") {
		t.Fatalf("audit subscribe lifecycle = %q", lifecycle[1])
	}
	if !strings.Contains(lifecycle[2], "deliver_start:default:users.created:") {
		t.Fatalf("default deliver start lifecycle = %q", lifecycle[2])
	}
	if !strings.Contains(lifecycle[3], "deliver_finish:default:users.created:") {
		t.Fatalf("default deliver finish lifecycle = %q", lifecycle[3])
	}
	if !strings.Contains(lifecycle[4], "unsubscribe:default:users.created:") {
		t.Fatalf("default unsubscribe lifecycle = %q", lifecycle[4])
	}
	if !strings.Contains(lifecycle[5], "unsubscribe:audit:users.created:") {
		t.Fatalf("audit unsubscribe lifecycle = %q", lifecycle[5])
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "events", "generated_observer_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/events", "TestGeneratedObserver", nil)
}

func TestGenerateEventFilesRejectsUnknownEnvVars(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "redis")
	t.Setenv("EVENTS_UNKNOWN_VALUE", "1")

	_, err := GenerateEventFiles(t.TempDir())
	if err == nil {
		t.Fatal("expected GenerateEventFiles to reject unknown events env vars")
	}
	if !strings.Contains(err.Error(), "EVENTS_UNKNOWN_VALUE") {
		t.Fatalf("expected error to mention unknown env var, got: %v", err)
	}
}

func TestGenerateEventFilesAllowsInactiveRootDriverEnvVars(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "inproc")
	t.Setenv("EVENTS_ADDR", "127.0.0.1:6379")
	t.Setenv("EVENTS_REDIS_CHANNEL_PREFIX", "events")

	if _, err := GenerateEventFiles(t.TempDir()); err != nil {
		t.Fatalf("expected GenerateEventFiles to allow documented inactive root events env vars, got %v", err)
	}
}

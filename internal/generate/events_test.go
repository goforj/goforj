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

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "events", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source := string(managerGen)
	if !strings.Contains(source, `func (m *Manager) Audit() Bus`) {
		t.Fatal("expected generated events manager accessor for named audit bus")
	}
	if !strings.Contains(source, `case "audit":`) {
		t.Fatal("expected generated events manager to support Named(\"audit\")")
	}
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

	goforjevents "github.com/goforj/events"
	"github.com/goforj/events/eventscore"
)

type API = goforjevents.API
type Driver = eventscore.Driver
type Subscription = goforjevents.Subscription

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

func (b *managedBus) Driver() eventscore.Driver                         { return b.api.Driver() }
func (b *managedBus) Ready() error                                      { return b.api.Ready() }
func (b *managedBus) ReadyContext(ctx context.Context) error            { return b.api.ReadyContext(normalizeEventsContext(ctx)) }
func (b *managedBus) Publish(event any) error                           { return b.api.Publish(event) }
func (b *managedBus) PublishContext(ctx context.Context, event any) error { return b.api.PublishContext(normalizeEventsContext(ctx), event) }
func (b *managedBus) Subscribe(handler any) (Subscription, error)       { return b.api.Subscribe(handler) }
func (b *managedBus) SubscribeContext(ctx context.Context, handler any) (Subscription, error) {
	return b.api.SubscribeContext(normalizeEventsContext(ctx), handler)
}
func (b *managedBus) Start(ctx context.Context) error {
	if b.start != nil {
		return b.start(normalizeEventsContext(ctx))
	}
	return b.api.ReadyContext(normalizeEventsContext(ctx))
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
func (b *errorBus) ReadyContext(context.Context) error               { return b.err }
func (b *errorBus) Publish(any) error                                { return b.err }
func (b *errorBus) PublishContext(context.Context, any) error        { return b.err }
func (b *errorBus) Subscribe(any) (Subscription, error)              { return nil, b.err }
func (b *errorBus) SubscribeContext(context.Context, any) (Subscription, error) { return nil, b.err }

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
	"testing"
	"time"
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

	var observed []string
	mgr = mgr.WithObserver(ObserverFunc(func(_ context.Context, name string, topic string, err error, _ time.Duration, driver Driver) {
		if err != nil {
			t.Fatalf("observer saw error: %v", err)
		}
		observed = append(observed, name+":"+topic+":"+string(driver))
	}))

	if err := mgr.Default().Publish(userCreated{ID: "123"}); err != nil {
		t.Fatalf("default Publish returned error: %v", err)
	}
	if err := mgr.Audit().Publish(userCreated{ID: "456"}); err != nil {
		t.Fatalf("audit Publish returned error: %v", err)
	}
	if len(observed) != 2 {
		t.Fatalf("len(observed) = %d, want 2", len(observed))
	}
	if observed[0] != "default:users.created:sync" {
		t.Fatalf("default observed = %q", observed[0])
	}
	if observed[1] != "audit:users.created:null" {
		t.Fatalf("audit observed = %q", observed[1])
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

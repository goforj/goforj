package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNotificationGenerationRejectsUnrepresentableConfiguration keeps invalid manifests from publishing partial artifacts.
func TestNotificationGenerationRejectsUnrepresentableConfiguration(t *testing.T) {
	for _, values := range []map[string]string{
		{"NOTIFICATION_SUPPORTED_DRIVERS": "log,unknown"},
		{"NOTIFICATION_SUPPORTED_DRIVERS": "log", "NOTIFICATION_DRIVER": "webhook"},
		{"NOTIFICATION_DRIVER": "unknown"},
		{"NOTIFICATION_DEFAULT_DRIVER": "log"},
		{"NOTIFICATION_CLOSE_DRIVER": "log"},
		{"NOTIFICATION_123_DRIVER": "log"},
		{"NOTIFICATION_FOO_BAR_DRIVER": "log", "NOTIFICATION_FOO__BAR_DRIVER": "log"},
	} {
		t.Run(strings.Join(sortedEnvironmentKeys(values), ","), func(t *testing.T) {
			for key, value := range values {
				t.Setenv(key, value)
			}
			root := t.TempDir()
			if _, err := GenerateNotificationFiles(root); err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if _, err := os.Stat(filepath.Join(root, NotificationManagerPath)); !os.IsNotExist(err) {
				t.Fatal("partial artifacts published")
			}
		})
	}
}

// sortedEnvironmentKeys gives invalid-configuration cases reproducible subtest names.
func sortedEnvironmentKeys(values map[string]string) []string {
	keys := map[string]struct{}{}
	for key := range values {
		keys[key] = struct{}{}
	}
	return sortStrings(keys)
}

// TestNotificationGenerationDiscoversAppSendersAndRegeneratesCleanly protects scoped accessors and driver imports.
func TestNotificationGenerationDiscoversAppSendersAndRegeneratesCleanly(t *testing.T) {
	t.Setenv("NOTIFICATION_DRIVER", "log")
	t.Setenv("NOTIFICATION_SUPPORTED_DRIVERS", "log,webhook")
	t.Setenv("BILLING_NOTIFICATION_DRIVER", "webhook")
	t.Setenv("BILLING_NOTIFICATION_ALERTS_DRIVER", "log")
	root := t.TempDir()
	if written, err := GenerateNotificationFiles(root); err != nil || written != 2 {
		t.Fatalf("first generation = %d, %v", written, err)
	}
	if written, err := GenerateNotificationFiles(root); err != nil || written != 0 {
		t.Fatalf("second generation = %d, %v", written, err)
	}
	accessors, err := os.ReadFile(filepath.Join(root, NotificationAccessorsPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(accessors), "Alerts() notification.Sender") || strings.Contains(string(accessors), "Billing()") {
		t.Fatalf("App accessors = %s", accessors)
	}
	t.Setenv("NOTIFICATION_SUPPORTED_DRIVERS", "log")
	t.Setenv("BILLING_NOTIFICATION_DRIVER", "log")
	if _, err := GenerateNotificationFiles(root); err != nil {
		t.Fatal(err)
	}
	manager, err := os.ReadFile(filepath.Join(root, NotificationManagerPath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manager), "driver/webhook") {
		t.Fatal("unselected driver imported")
	}
}

// TestGeneratedNotificationManagerBehavior exercises the released library through generated accessors without external traffic.
func TestGeneratedNotificationManagerBehavior(t *testing.T) {
	t.Setenv("NOTIFICATION_DRIVER", "log")
	t.Setenv("NOTIFICATION_SUPPORTED_DRIVERS", "log,webhook")
	t.Setenv("NOTIFICATION_ALERTS_DRIVER", "log")
	root := mustTempGeneratedModuleRoot(t, "notification-generation-*", "internal/notifications")
	writeFixtureGoMod(t, root, fixtureModuleSpec("example.com/notificationtest", []string{"github.com/goforj/env/v2", "github.com/goforj/notification"}, nil, nil))
	if _, err := GenerateNotificationFiles(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal/notifications/manager_test.go"), []byte(notificationManagerBehaviorTest), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/notifications", "", nil)
}

const notificationManagerBehaviorTest = `package notifications

import (
    "context"
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/goforj/notification"
)

// closeRecorder reports ownership and failure propagation without a live provider.
type closeRecorder struct { calls int; err error }

// Close records each ownership release.
func (closer *closeRecorder) Close() error { closer.calls++; return closer.err }

// TestAccessorsPreserveSenderAndContext verifies local capture, injection, and named lookup.
func TestAccessorsPreserveSenderAndContext(t *testing.T) {
    t.Setenv("NOTIFICATION_DRIVER", "log")
    t.Setenv("NOTIFICATION_ALERTS_DRIVER", "log")
    manager, err := NewManager()
    if err != nil { t.Fatal(err) }
    defer manager.Close()
    message := notification.Message{Title: "Incident inc-73", Body: "critical: Payments unavailable"}
    if err := manager.Default().Send(context.Background(), message); err != nil { t.Fatal(err) }
    if manager.Alerts() != manager.Named(" ALERTS ") || manager.Default() != manager.Named("") || manager.Named("missing") != nil { t.Fatal("wrong named lookup") }
    if len(manager.Names()) != 2 || len(manager.Instances()) != 2 || !manager.Instances()[0].IsDefault { t.Fatal("wrong instance inventory") }
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    if err := manager.Alerts().Send(ctx, message); !errors.Is(err, context.Canceled) { t.Fatalf("cancellation = %v", err) }
    if err := manager.Default().Send(context.Background(), notification.Message{}); !errors.Is(err, notification.ErrInvalidMessage) { t.Fatalf("validation = %v", err) }
    expected := errors.New("recorded send failure")
    manager.defaultSender = notification.SenderFunc(func(got context.Context, gotMessage notification.Message) error {
        if got != ctx || gotMessage != message { t.Error("sender context or data lost") }
        return expected
    })
    if err := manager.Default().Send(ctx, message); !errors.Is(err, expected) { t.Fatal("sender error lost") }
}

// TestOwnedCloseRunsOnce retains every cleanup error without closing borrowed senders.
func TestOwnedCloseRunsOnce(t *testing.T) {
    expected := errors.New("close failed")
    first, second := &closeRecorder{err: expected}, &closeRecorder{}
    manager := &Manager{closers: []io.Closer{first, second}}
    for index := 0; index < 2; index++ { if !errors.Is(manager.Close(), expected) { t.Fatal("cleanup error lost") } }
    if first.calls != 1 || second.calls != 1 { t.Fatal("owned resource closed repeatedly") }
}

// TestWebhookAccessorsSendOnce records exact message data and classified rejection locally.
func TestWebhookAccessorsSendOnce(t *testing.T) {
    status, calls := http.StatusNoContent, 0
    server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
        calls++
        var message notification.TextPayload
        if err := json.NewDecoder(request.Body).Decode(&message); err != nil { t.Error(err) }
        if message.Title != "Incident inc-9" || message.Body != "warning: latency" { t.Errorf("message = %#v", message) }
        response.WriteHeader(status)
    }))
    defer server.Close()
    t.Setenv("NOTIFICATION_DRIVER", "webhook")
    t.Setenv("NOTIFICATION_WEBHOOK_URL", server.URL)
    t.Setenv("NOTIFICATION_WEBHOOK_ALLOW_PRIVATE", "true")
    t.Setenv("NOTIFICATION_ALERTS_DRIVER", "log")
    manager, err := NewManager()
    if err != nil { t.Fatal(err) }
    defer manager.Close()
    message := notification.Message{Title: "Incident inc-9", Body: "warning: latency"}
    if err := manager.Default().Send(context.Background(), message); err != nil { t.Fatal(err) }
    status = http.StatusBadRequest
    if err := manager.Default().Send(context.Background(), message); !errors.Is(err, notification.ErrPermanent) { t.Fatalf("rejection = %v", err) }
    if calls != 2 { t.Fatalf("calls = %d, want 2 with no retries", calls) }
    status = http.StatusNoContent
    t.Setenv("NOTIFICATION_DRIVER", "")
    t.Setenv("NOTIFICATION_ALERTS_DRIVER", "webhook")
    t.Setenv("NOTIFICATION_ALERTS_WEBHOOK_URL", server.URL)
    t.Setenv("NOTIFICATION_ALERTS_WEBHOOK_ALLOW_PRIVATE", "true")
    named, err := NewManager()
    if err != nil { t.Fatal(err) }
    defer named.Close()
    if err := named.Alerts().Send(context.Background(), message); err != nil { t.Fatal(err) }
    if calls != 3 { t.Fatalf("named calls = %d, want one additional attempt", calls) }
}

// TestInvalidRuntimeConfigurationFailsBeforeSending rejects unavailable drivers and malformed transport settings.
func TestInvalidRuntimeConfigurationFailsBeforeSending(t *testing.T) {
    for _, test := range []struct{ key, value string }{
        {"NOTIFICATION_DRIVER", "unknown"},
        {"NOTIFICATION_WEBHOOK_TIMEOUT", "invalid"},
        {"NOTIFICATION_WEBHOOK_TIMEOUT", "0s"},
        {"NOTIFICATION_WEBHOOK_ALLOW_PRIVATE", "invalid"},
        {"NOTIFICATION_WEBHOOK_URL", "://invalid"},
        {"NOTIFICATION_WEBHOOK_URL", ""},
        {"NOTIFICATION_WEBHOOK_URL", "   "},
        {"NOTIFICATION_ALERTS_DRIVER", "webhook"},
        {"NOTIFICATION_WEBHOOK_FORMAT", "unknown"},
        {"NOTIFICATION_ALERTS_DRIVER", "unknown"},
    } {
        t.Run(test.key+test.value, func(t *testing.T) {
            t.Setenv("NOTIFICATION_DRIVER", "webhook")
            t.Setenv("NOTIFICATION_WEBHOOK_URL", "https://example.test/notify")
            t.Setenv("NOTIFICATION_ALERTS_DRIVER", "log")
            t.Setenv(test.key, test.value)
            if manager, err := NewManager(); err == nil || manager != nil { t.Fatal("invalid configuration accepted") }
        })
    }
}
`

// TestNotificationManagerScopesAppOnlySenders prevents undeclared resources from silently becoming local capture in another App.
func TestNotificationManagerScopesAppOnlySenders(t *testing.T) {
	root := mustTempGeneratedModuleRoot(t, "notification-app-scope-*", "internal/notifications")
	writeFixtureGoMod(t, root, fixtureModuleSpec("example.com/notificationscope", []string{"github.com/goforj/env/v2", "github.com/goforj/notification"}, nil, nil))
	config := "project_name: scoped\nmodule_name: example.com/notificationscope\nrender:\n  components: [cli,notification]\napps:\n  admin:\n    components: [cli,notification]\n"
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"NOTIFICATION_DRIVER": "log", "NOTIFICATION_SUPPORTED_DRIVERS": "log,webhook",
		"NOTIFICATION_SHARED_DRIVER": "log", "ADMIN_NOTIFICATION_ALERTS_DRIVER": "webhook",
		"ADMIN_NOTIFICATION_ALERTS_WEBHOOK_URL": "https://example.test/notify",
	} {
		t.Setenv(key, value)
	}
	if _, err := GenerateNotificationFiles(root); err != nil {
		t.Fatal(err)
	}
	if written, err := GenerateNotificationFiles(root); err != nil || written != 0 {
		t.Fatalf("scope regeneration = %d, %v", written, err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal/notifications/scope_test.go"), []byte(notificationAppScopeBehaviorTest), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/notifications", "", nil)
}

// TestNotificationGenerationRejectsInvalidAppOwnership avoids publishing managers with an unknown App membership contract.
func TestNotificationGenerationRejectsInvalidAppOwnership(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("render: ["), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateNotificationFiles(root); err == nil {
		t.Fatal("invalid App configuration accepted")
	}
}

const notificationAppScopeBehaviorTest = `package notifications

import (
    "context"
    "net/http"
    "net/http/httptest"
    "sync/atomic"
    "testing"

    "github.com/goforj/notification"
)

// TestAppOnlySenderMembership keeps App-specific destinations out of default initialization and discovery.
func TestAppOnlySenderMembership(t *testing.T) {
    var calls atomic.Int64
    server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) { calls.Add(1); response.WriteHeader(http.StatusNoContent) }))
    defer server.Close()
    t.Setenv("NOTIFICATION_DRIVER", "log")
    t.Setenv("NOTIFICATION_SHARED_DRIVER", "log")
    t.Setenv("ADMIN_NOTIFICATION_ALERTS_DRIVER", "webhook")
    t.Setenv("ADMIN_NOTIFICATION_ALERTS_WEBHOOK_URL", "://invalid-admin-only")
    root, err := NewManager()
    if err != nil { t.Fatalf("default initialized another App's invalid resource: %v", err) }
    defer root.Close()
    if len(root.Names()) != 2 || len(root.Instances()) != 2 || root.Named("alerts") != nil { t.Fatal("default advertised an admin-only sender") }
    if root.Shared() != root.Named("shared") { t.Fatal("root-declared sender unavailable") }
    assertOtherAppAccessorPanics(t, root)
    if manager, err := NewManagerForApp("ADMIN"); err == nil || manager != nil { t.Fatal("owning App ignored invalid configuration") }
    for _, endpoint := range []string{"", "   "} {
        t.Setenv("ADMIN_NOTIFICATION_ALERTS_WEBHOOK_URL", endpoint)
        if manager, err := NewManagerForApp("ADMIN"); err == nil || manager != nil { t.Fatal("owning App accepted missing webhook URL") }
    }
    t.Setenv("ADMIN_NOTIFICATION_ALERTS_WEBHOOK_URL", server.URL)
    t.Setenv("ADMIN_NOTIFICATION_ALERTS_WEBHOOK_ALLOW_PRIVATE", "true")
    admin, err := NewManagerForApp(" admin ")
    if err != nil { t.Fatal(err) }
    defer admin.Close()
    if len(admin.Names()) != 3 || len(admin.Instances()) != 3 || admin.Alerts() != admin.Named("alerts") { t.Fatal("admin membership lost") }
    if err := admin.Shared().Send(context.Background(), notification.Message{Body: "shared capture"}); err != nil { t.Fatal(err) }
    if err := admin.Alerts().Send(context.Background(), notification.Message{Body: "admin webhook"}); err != nil { t.Fatal(err) }
    if calls.Load() != 1 { t.Fatalf("requests = %d, want only admin-specific webhook", calls.Load()) }
    if manager, err := NewManagerForApp("unconfigured"); err == nil || manager != nil { t.Fatal("unknown App prefix initialized resources") }
}

// assertOtherAppAccessorPanics proves a wiring mistake fails before it can silently discard a notification.
func assertOtherAppAccessorPanics(t *testing.T, manager *Manager) {
    t.Helper()
    defer func() { if recover() == nil { t.Error("wrong-App typed accessor did not fail fast") } }()
    _ = manager.Alerts()
}
`

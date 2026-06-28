package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateMailFilesUsesSupportedDriverImports(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "resend")
	t.Setenv("MAIL_SUPPORTED_DRIVERS", "resend")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "mail"), 0o755); err != nil {
		t.Fatalf("mkdir mail package: %v", err)
	}

	if _, err := GenerateMailFiles(root); err != nil {
		t.Fatalf("GenerateMailFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "mail", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source := string(managerGen)
	if !strings.Contains(source, `"github.com/goforj/mail/mailresend"`) {
		t.Fatal("expected generated mail manager to import mailresend from MAIL_SUPPORTED_DRIVERS")
	}
	if !strings.Contains(source, `"github.com/goforj/mail/maillog"`) {
		t.Fatal("expected generated mail manager to keep maillog as the no-env fallback")
	}
	if strings.Contains(source, `"github.com/goforj/mail/mailses"`) {
		t.Fatal("did not expect generated mail manager to import mailses when it is not supported")
	}
}

func TestGenerateMailFilesUsesActiveDriverImportsWhenSupportedUnset(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "smtp")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "mail"), 0o755); err != nil {
		t.Fatalf("mkdir mail package: %v", err)
	}

	if _, err := GenerateMailFiles(root); err != nil {
		t.Fatalf("GenerateMailFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "mail", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source := string(managerGen)
	if !strings.Contains(source, `"github.com/goforj/mail/mailsmtp"`) {
		t.Fatal("expected generated mail manager to import mailsmtp from MAIL_DRIVER")
	}
}

func TestGenerateMailFilesSupportsDefaultAndNamedMailers(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "log")
	t.Setenv("MAIL_FROM_ADDRESS", "default@example.com")
	t.Setenv("MAIL_FROM_NAME", "Default")
	t.Setenv("MAIL_TRANSACTIONAL_DRIVER", "resend")
	t.Setenv("MAIL_TRANSACTIONAL_FROM_ADDRESS", "tx@example.com")
	t.Setenv("MAIL_TRANSACTIONAL_FROM_NAME", "Transactional")
	t.Setenv("MAIL_TRANSACTIONAL_RESEND_API_KEY", "resend-key")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "mail"), 0o755); err != nil {
		t.Fatalf("mkdir mail package: %v", err)
	}

	if _, err := GenerateMailFiles(root); err != nil {
		t.Fatalf("GenerateMailFiles returned error: %v", err)
	}

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "mail", "accessors_gen.go"))
	if err != nil {
		t.Fatalf("read accessors_gen.go: %v", err)
	}
	source := string(managerGen)
	for _, snippet := range []string{
		`func (m *Manager) Transactional() *goforjmail.Mailer`,
		`func (m *Manager) Named(name string) *goforjmail.Mailer`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected generated mail manager to contain %q", snippet)
		}
	}
	managerGen, err = os.ReadFile(filepath.Join(root, "internal", "mail", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source = string(managerGen)
	for _, snippet := range []string{
		`transactional *goforjmail.Mailer`,
		`scopeTransactional := env.WithPrefix("MAIL").Child(str.Of("transactional").Snake("_").ToUpper().String())`,
		`manager.transactional = goforjmail.New(`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected generated mail manager to contain %q", snippet)
		}
	}
}

func TestGenerateMailFilesSupportsObserverWrapping(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "log")
	t.Setenv("MAIL_FROM_ADDRESS", "default@example.com")
	t.Setenv("MAIL_FROM_NAME", "Default")
	t.Setenv("MAIL_TRANSACTIONAL_DRIVER", "log")
	t.Setenv("MAIL_TRANSACTIONAL_FROM_ADDRESS", "tx@example.com")
	t.Setenv("MAIL_TRANSACTIONAL_FROM_NAME", "Transactional")

	root := mustTempGeneratedModuleRoot(t, ".tmp-mail-generation-*", filepath.Join("internal", "mail"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/mailobservertest",
		[]string{
			"github.com/goforj/env/v2",
			"github.com/goforj/mail",
			"github.com/goforj/str",
		},
		nil,
		mailLocalReplaces(t),
	))

	if _, err := GenerateMailFiles(root); err != nil {
		t.Fatalf("GenerateMailFiles returned error: %v", err)
	}

	testSource := `package mail

import (
	"context"
	"os"
	"testing"

	goforjmail "github.com/goforj/mail"
)

func TestGeneratedObserver(t *testing.T) {
	mgr, err := NewManagerWithObserver(ObserverFunc(func(_ context.Context, event MailSendEvent) {
		if event.Err != nil {
			t.Fatalf("observer saw error: %v", event.Err)
		}
		observed = append(observed, event.Name+":"+event.Driver)
	}))
	if err != nil {
		t.Fatalf("NewManagerWithObserver returned error: %v", err)
	}

	msg := goforjmail.Message{
		To:      []goforjmail.Recipient{{Email: "alice@example.com", Name: "Alice"}},
		Subject: "Welcome",
		Text:    "hello world",
	}

	if err := mgr.Default().Send(context.Background(), msg); err != nil {
		t.Fatalf("default Send returned error: %v", err)
	}
	if err := mgr.Transactional().Send(context.Background(), msg); err != nil {
		t.Fatalf("transactional Send returned error: %v", err)
	}

	if len(observed) != 2 {
		t.Fatalf("len(observed) = %d, want 2", len(observed))
	}
	if observed[0] != "default:log" {
		t.Fatalf("default observed = %q", observed[0])
	}
	if observed[1] != "transactional:log" {
		t.Fatalf("transactional observed = %q", observed[1])
	}
}

func TestGeneratedAccessorsFallbackWithoutRuntimeEnv(t *testing.T) {
	for _, key := range []string{
		"MAIL_DRIVER",
		"MAIL_FROM_ADDRESS",
		"MAIL_FROM_NAME",
		"MAIL_TRANSACTIONAL_DRIVER",
		"MAIL_TRANSACTIONAL_FROM_ADDRESS",
		"MAIL_TRANSACTIONAL_FROM_NAME",
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
		t.Fatal("expected default mailer fallback")
	}
	if mgr.Transactional() == nil {
		t.Fatal("expected transactional mailer fallback")
	}
	msg := goforjmail.Message{
		To:      []goforjmail.Recipient{{Email: "alice@example.com", Name: "Alice"}},
		Subject: "Welcome",
		Text:    "hello world",
	}
	if err := mgr.Transactional().Send(context.Background(), msg); err != nil {
		t.Fatalf("transactional fallback Send returned error: %v", err)
	}
}

var observed []string
`
	if err := os.WriteFile(filepath.Join(root, "internal", "mail", "generated_observer_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/mail", "TestGeneratedObserver", nil)
}

func TestGenerateMailFilesChainsMultipleObservers(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "log")
	t.Setenv("MAIL_FROM_ADDRESS", "default@example.com")
	t.Setenv("MAIL_FROM_NAME", "Default")
	t.Setenv("MAIL_TRANSACTIONAL_DRIVER", "log")
	t.Setenv("MAIL_TRANSACTIONAL_FROM_ADDRESS", "tx@example.com")
	t.Setenv("MAIL_TRANSACTIONAL_FROM_NAME", "Transactional")

	root := mustTempGeneratedModuleRoot(t, ".tmp-mail-observer-chain-*", filepath.Join("internal", "mail"))
	writeFixtureGoMod(t, root, fixtureModuleSpec(
		"example.com/mailobserverchaintest",
		[]string{
			"github.com/goforj/env/v2",
			"github.com/goforj/mail",
			"github.com/goforj/str",
		},
		nil,
		mailLocalReplaces(t),
	))

	if _, err := GenerateMailFiles(root); err != nil {
		t.Fatalf("GenerateMailFiles returned error: %v", err)
	}

	testSource := `package mail

import (
	"context"
	"testing"

	goforjmail "github.com/goforj/mail"
)

func TestObserverChain(t *testing.T) {
	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	var metricsOps int
	var inspectOps int
	mgr, err = mgr.WithObserver(ObserverFunc(func(_ context.Context, event MailSendEvent) {
		if event.Err != nil {
			t.Fatalf("metrics observer saw error: %v", event.Err)
		}
		if event.Name == "transactional" && event.Driver == "log" {
			metricsOps++
		}
	}))
	if err != nil {
		t.Fatalf("WithObserver metrics returned error: %v", err)
	}
	mgr, err = mgr.WithObserver(ObserverFunc(func(_ context.Context, event MailSendEvent) {
		if event.Err != nil {
			t.Fatalf("inspect observer saw error: %v", event.Err)
		}
		if event.Name == "transactional" && event.Driver == "log" {
			inspectOps++
		}
	}))
	if err != nil {
		t.Fatalf("WithObserver inspect returned error: %v", err)
	}

	msg := goforjmail.Message{
		To:      []goforjmail.Recipient{{Email: "alice@example.com", Name: "Alice"}},
		Subject: "Welcome",
		Text:    "hello world",
	}
	if err := mgr.Transactional().Send(context.Background(), msg); err != nil {
		t.Fatalf("transactional Send returned error: %v", err)
	}
	if metricsOps != 1 {
		t.Fatalf("metrics observer count = %d, want 1", metricsOps)
	}
	if inspectOps != 1 {
		t.Fatalf("inspect observer count = %d, want 1", inspectOps)
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "mail", "observer_chain_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write generated test: %v", err)
	}

	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/mail", "TestObserverChain", nil)
}

func TestGenerateMailFilesRejectsUnknownEnvVars(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "log")
	t.Setenv("MAIL_RESND_API_KEY", "bad")

	_, err := GenerateMailFiles(t.TempDir())
	if err == nil {
		t.Fatal("expected GenerateMailFiles to reject unknown mail env vars")
	}
	if !strings.Contains(err.Error(), "MAIL_RESND_API_KEY") {
		t.Fatalf("expected error to mention unknown env var, got: %v", err)
	}
}

func TestGenerateMailFilesAllowsInactiveRootDriverEnvVars(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "log")
	t.Setenv("MAIL_SMTP_HOST", "smtp.gmail.com")

	if _, err := GenerateMailFiles(t.TempDir()); err != nil {
		t.Fatalf("expected GenerateMailFiles to allow documented inactive root mail env vars, got %v", err)
	}
}

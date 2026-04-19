package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateMailFilesUsesSupportedDriverImports(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "log")
	t.Setenv("MAIL_SUPPORTED_DRIVERS", "log,resend")

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

	managerGen, err := os.ReadFile(filepath.Join(root, "internal", "mail", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read manager_gen.go: %v", err)
	}
	source := string(managerGen)
	for _, snippet := range []string{
		`transactional *goforjmail.Mailer`,
		`func (m *Manager) Transactional() *goforjmail.Mailer`,
		`func (m *Manager) Named(name string) *goforjmail.Mailer`,
		`case "transactional":`,
		`scope := env.WithPrefix("MAIL").Child(child)`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected generated mail manager to contain %q", snippet)
		}
	}
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

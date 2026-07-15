package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderEnvironmentExampleRedactsSecrets verifies committed examples retain the resource contract without copying owner credentials.
func TestRenderEnvironmentExampleRedactsSecrets(t *testing.T) {
	source := strings.Join([]string{
		"# App",
		"APP_KEY=generated-key",
		"APP_DIAG_TOKEN=diagnostic-token # generated per project",
		"API_JWT_SECRET_KEY=jwt-secret",
		"AUTH_ACCESS_TOKEN_TTL=15m",
		"FRONTEND_AUTH_PASSWORD_MIN_LENGTH=8",
		"FRONTEND_AUTH_PASSWORD_REQUIRE_UPPER=true",
		"AUTH_BOOTSTRAP_PASSWORD=admin",
		"export SERVICE_API_KEY = api-key # provisioned externally",
		"GOOGLE_APPLICATION_CREDENTIALS=/private/account.json",
		"AWS_ACCESS_KEY_ID=owner-access-key",
		"DATABASE_DSN='mysql://user:secret@db/app'",
		"DATABASE_URL=mysql://user:secret@db/app",
		"NATS_URL=nats://user:secret@nats.internal:4222",
		"CUSTOM_BROKER='amqp://token@broker.internal/vhost' # arbitrary key",
		"APP_URL=http://localhost:3000",
		"",
		"# Resources",
		"DB_DRIVER=mysql # options: sqlite, mysql, postgres",
		"DB_SUPPORTED_DRIVERS=sqlite,mysql",
		"CACHE_DRIVER=memory",
		"CACHE_SUPPORTED_DRIVERS=memory,redis",
		"QUEUE_DRIVER=workerpool",
		"QUEUE_SUPPORTED_DRIVERS=workerpool,redis",
		"EVENTS_DRIVER=inproc",
		"EVENTS_SUPPORTED_DRIVERS=inproc,redis",
		"# AUTH_OAUTH_GITHUB_CLIENT_SECRET=do-not-commit",
		"MAIL_SMTP_PASSWORD=",
		"",
	}, "\n")
	want := strings.Join([]string{
		"# App",
		"APP_KEY=",
		"APP_DIAG_TOKEN= # generated per project",
		"API_JWT_SECRET_KEY=",
		"AUTH_ACCESS_TOKEN_TTL=15m",
		"FRONTEND_AUTH_PASSWORD_MIN_LENGTH=8",
		"FRONTEND_AUTH_PASSWORD_REQUIRE_UPPER=true",
		"AUTH_BOOTSTRAP_PASSWORD=",
		"export SERVICE_API_KEY = # provisioned externally",
		"GOOGLE_APPLICATION_CREDENTIALS=",
		"AWS_ACCESS_KEY_ID=",
		"DATABASE_DSN=''",
		"DATABASE_URL=",
		"NATS_URL=",
		"CUSTOM_BROKER='' # arbitrary key",
		"APP_URL=http://localhost:3000",
		"",
		"# Resources",
		"DB_DRIVER=mysql # options: sqlite, mysql, postgres",
		"DB_SUPPORTED_DRIVERS=sqlite,mysql",
		"CACHE_DRIVER=memory",
		"CACHE_SUPPORTED_DRIVERS=memory,redis",
		"QUEUE_DRIVER=workerpool",
		"QUEUE_SUPPORTED_DRIVERS=workerpool,redis",
		"EVENTS_DRIVER=inproc",
		"EVENTS_SUPPORTED_DRIVERS=inproc,redis",
		"# AUTH_OAUTH_GITHUB_CLIENT_SECRET=",
		"MAIL_SMTP_PASSWORD=",
		"",
	}, "\n")

	got := string(RenderEnvironmentExample([]byte(source)))
	if got != want {
		t.Fatalf("RenderEnvironmentExample() mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
	if second := string(RenderEnvironmentExample([]byte(got))); second != got {
		t.Fatalf("RenderEnvironmentExample() is not idempotent\nfirst:\n%s\nsecond:\n%s", got, second)
	}
}

// TestRenderEnvironmentExamplePreservesLineConventions verifies redaction does not churn CRLF files or terminal-newline ownership.
func TestRenderEnvironmentExamplePreservesLineConventions(t *testing.T) {
	source := []byte("APP_KEY=secret\r\nCACHE_DRIVER=redis\r\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis")
	want := "APP_KEY=\r\nCACHE_DRIVER=redis\r\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis"
	if got := string(RenderEnvironmentExample(source)); got != want {
		t.Fatalf("RenderEnvironmentExample() = %q, want %q", got, want)
	}
}

// TestRenderEnvironmentExampleRedactsMultilineSecrets verifies quoted private material cannot escape through continuation lines.
func TestRenderEnvironmentExampleRedactsMultilineSecrets(t *testing.T) {
	source := "JWT_PRIVATE_KEY=\"-----BEGIN PRIVATE KEY-----\nowner-private-material\n-----END PRIVATE KEY-----\" # generated\nCACHE_DRIVER=memory\n"
	want := "JWT_PRIVATE_KEY=\"\"\n\n # generated\nCACHE_DRIVER=memory\n"
	if got := string(RenderEnvironmentExample([]byte(source))); got != want {
		t.Fatalf("RenderEnvironmentExample() = %q, want %q", got, want)
	}
}

// TestMergeEnvironmentExamplePreservesSafeOwnerContract verifies rerender updates framework keys without pruning app documentation.
func TestMergeEnvironmentExamplePreservesSafeOwnerContract(t *testing.T) {
	existing := []byte("# Owner integration\nCUSTOM_BROKER=nats://guest:secret@nats.internal:4222\nCACHE_DRIVER=file\n# CACHE_ENDPOINT=https://cache.example\n")
	source := []byte("CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\n")
	merged := string(MergeEnvironmentExample(existing, source))
	for _, want := range []string{
		"# Owner integration",
		"CUSTOM_BROKER=",
		"CACHE_DRIVER=memory",
		"CACHE_SUPPORTED_DRIVERS=memory,redis",
		"# CACHE_ENDPOINT=https://cache.example",
	} {
		if !strings.Contains(merged, want) {
			t.Fatalf("merged environment example omitted %q:\n%s", want, merged)
		}
	}
}

// TestWriteEnvironmentExampleAtomic verifies publication redacts first and preserves an owner-selected file mode.
func TestWriteEnvironmentExampleAtomic(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env.example")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("write existing example: %v", err)
	}
	source := []byte("APP_KEY=generated\nCACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\n")
	if err := WriteEnvironmentExampleAtomic(path, source, 0o644); err != nil {
		t.Fatalf("WriteEnvironmentExampleAtomic() error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	want := "APP_KEY=\nCACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\n"
	if string(data) != want {
		t.Fatalf("example = %q, want %q", data, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat example: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("example mode = %o, want 600", got)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".env.example-*"))
	if err != nil {
		t.Fatalf("glob replacement files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("atomic replacement left temporary files: %v", matches)
	}
}

// TestEnsureGitignoreEnvironmentRulesPreservesOwnerEntries verifies rerender adds the safe contract without replacing custom ignores.
func TestEnsureGitignoreEnvironmentRulesPreservesOwnerEntries(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, []byte("vendor/\n.env\n# owner rule\ncustom.tmp\n"), 0o600); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}
	if err := ensureGitignoreEnvironmentRules(path); err != nil {
		t.Fatalf("ensure environment rules: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read gitignore: %v", err)
	}
	text := string(data)
	for _, want := range []string{"vendor/", "# owner rule", "custom.tmp", ".env.host", ".env.local", "!.env.example"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated gitignore omitted %q:\n%s", want, text)
		}
	}
	if strings.Count(text, ".env\n") != 1 {
		t.Fatalf("existing .env rule was duplicated:\n%s", text)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("gitignore mode changed: info=%v err=%v", info, err)
	}
}

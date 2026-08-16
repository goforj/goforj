package envfile_test

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/envfile"
)

// TestRedactExampleRedactsSecrets verifies committed examples retain the resource contract without copying owner credentials.
func TestRedactExampleRedactsSecrets(t *testing.T) {
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
		"GITHUB_PAT=github-token",
		"DISCORD_WEBHOOK=https://discord.example/private-hook",
		"CUSTOM_TOKEN: yaml-style-token",
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
		"APP_DIAG_TOKEN=",
		"API_JWT_SECRET_KEY=",
		"AUTH_ACCESS_TOKEN_TTL=15m",
		"FRONTEND_AUTH_PASSWORD_MIN_LENGTH=8",
		"FRONTEND_AUTH_PASSWORD_REQUIRE_UPPER=true",
		"AUTH_BOOTSTRAP_PASSWORD=",
		"export SERVICE_API_KEY =",
		"GOOGLE_APPLICATION_CREDENTIALS=",
		"AWS_ACCESS_KEY_ID=",
		"GITHUB_PAT=",
		"DISCORD_WEBHOOK=",
		"CUSTOM_TOKEN:",
		"DATABASE_DSN=''",
		"DATABASE_URL=",
		"NATS_URL=",
		"CUSTOM_BROKER=''",
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

	got := string(envfile.RedactExample([]byte(source)))
	if got != want {
		t.Fatalf("RedactExample() mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
	if second := string(envfile.RedactExample([]byte(got))); second != got {
		t.Fatalf("RedactExample() is not idempotent\nfirst:\n%s\nsecond:\n%s", got, second)
	}
}

// TestRedactExamplePreservesLineConventions verifies redaction does not churn CRLF files or terminal-newline ownership.
func TestRedactExamplePreservesLineConventions(t *testing.T) {
	source := []byte("APP_KEY=secret\r\nCACHE_DRIVER=redis\r\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis")
	want := "APP_KEY=\r\nCACHE_DRIVER=redis\r\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis"
	if got := string(envfile.RedactExample(source)); got != want {
		t.Fatalf("RedactExample() = %q, want %q", got, want)
	}
}

// TestRedactExampleRedactsMultilineSecrets verifies quoted private material cannot escape through continuation lines.
func TestRedactExampleRedactsMultilineSecrets(t *testing.T) {
	source := "JWT_PRIVATE_KEY=\"-----BEGIN PRIVATE KEY-----\nowner-private-material\n-----END PRIVATE KEY-----\" # generated\nCACHE_DRIVER=memory\n"
	want := "JWT_PRIVATE_KEY=\"\"\n\n\nCACHE_DRIVER=memory\n"
	if got := string(envfile.RedactExample([]byte(source))); got != want {
		t.Fatalf("RedactExample() = %q, want %q", got, want)
	}
}

// TestMergeExamplePreservesSafeOwnerContract verifies rerender updates framework keys without pruning app documentation.
func TestMergeExamplePreservesSafeOwnerContract(t *testing.T) {
	existing := []byte("# Owner integration\nCUSTOM_BROKER=nats://guest:secret@nats.internal:4222\nCACHE_DRIVER=file\n# CACHE_ENDPOINT=https://cache.example\n")
	source := []byte("CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\n")
	merged := string(envfile.MergeExample(existing, source))
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

// TestMergeExampleFailsClosedForNewApplicationKeys verifies private values need an explicit committed example before publication.
func TestMergeExampleFailsClosedForNewApplicationKeys(t *testing.T) {
	merged := string(envfile.MergeExample(nil, []byte("FEATURE_FLAG=true\nSSH_KEY=private\nAPP_ENV=local\n")))
	for _, want := range []string{"FEATURE_FLAG=\n", "SSH_KEY=\n", "APP_ENV=local\n"} {
		if !strings.Contains(merged, want) {
			t.Fatalf("MergeExample() omitted %q:\n%s", want, merged)
		}
	}
	if strings.Contains(merged, "private") {
		t.Fatalf("MergeExample() published private material:\n%s", merged)
	}

	existing := []byte("FEATURE_FLAG=true\n")
	merged = string(envfile.MergeExample(existing, []byte("FEATURE_FLAG=false\n")))
	if !strings.Contains(merged, "FEATURE_FLAG=true") {
		t.Fatalf("MergeExample() replaced an explicitly committed app value:\n%s", merged)
	}
}

// TestRedactExampleClosesClassifierBypasses verifies metadata and decoded multiline values cannot retain credentials.
func TestRedactExampleClosesClassifierBypasses(t *testing.T) {
	source := "AUTH_PASSWORD_MIN_LENGTH_API_KEY=private # private\nSSH_KEY=ssh-private\nHMAC_KEY=hmac-private\nMULTILINE_ENDPOINT=\"mysql://user:\npassword@db/app\"\n"
	got := string(envfile.RedactExample([]byte(source)))
	for _, secret := range []string{"private", "ssh-private", "hmac-private", "password@db"} {
		if strings.Contains(got, secret) {
			t.Fatalf("RedactExample() retained %q:\n%s", secret, got)
		}
	}
}

package envfile_test

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/envfile"
)

// TestMergeTestingCreatesSafeRunnableProfile verifies framework credentials and database settings become deterministic test values.
func TestMergeTestingCreatesSafeRunnableProfile(t *testing.T) {
	example := []byte("# App\nAPP_ENV=local\nAPP_KEY=\nAPI_JWT_SECRET_KEY=\nDB_HOST=mysql\nDB_DATABASE=app\nDB_USERNAME=postgres\nDB_PASSWORD=\nMAIL_RESEND_API_KEY=\nFEATURE_FLAG=true\n")
	got := string(envfile.MergeTesting(nil, example))
	for _, want := range []string{
		"# Selected automatically by goforj/env when tests load the environment.",
		"APP_ENV=testing",
		"APP_KEY=base64:",
		"API_JWT_SECRET_KEY=goforj-public-testing-jwt-signing-key",
		"DB_HOST=127.0.0.1",
		"DB_DATABASE=app_testing",
		"DB_USERNAME=test",
		"DB_PASSWORD=test",
		"MAIL_RESEND_API_KEY=\n",
		"FEATURE_FLAG=true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("testing contract omitted %q:\n%s", want, got)
		}
	}
}

// TestMergeTestingSuppliesLocalServiceURLs keeps supported NATS and RabbitMQ test profiles runnable without publishing private credentials.
func TestMergeTestingSuppliesLocalServiceURLs(t *testing.T) {
	for _, test := range []struct {
		name    string
		example string
		want    string
	}{
		{name: "NATS", example: "QUEUE_DRIVER=nats\nQUEUE_URL=\n", want: "QUEUE_URL=nats://goforj:goforj@nats:4222"},
		{name: "RabbitMQ", example: "QUEUE_DRIVER=rabbitmq\nQUEUE_URL=\n", want: "QUEUE_URL=amqp://goforj:goforj@rabbitmq:5672/"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := string(envfile.MergeTesting(nil, []byte(test.example)))
			if !strings.Contains(got, test.want) {
				t.Fatalf("testing contract omitted %q:\n%s", test.want, got)
			}
		})
	}
}

// TestMergeTestingPreservesProjectValuesAndRedactsSecrets verifies synchronization owns framework keys without leaking custom credentials.
func TestMergeTestingPreservesProjectValuesAndRedactsSecrets(t *testing.T) {
	existing := []byte("# Owner\nFEATURE_FLAG=false\nCUSTOM_TOKEN=owner-secret\nTEST_FIXTURE=compact\nDB_PASSWORD=owner-password\nREDIS_HOST=old-service\n")
	example := []byte("FEATURE_FLAG=true\nCUSTOM_TOKEN=\nNEW_SETTING=enabled\nDB_PASSWORD=\n")
	got := string(envfile.MergeTesting(existing, example))
	for _, want := range []string{"FEATURE_FLAG=false", "CUSTOM_TOKEN=", "TEST_FIXTURE=compact", "NEW_SETTING=enabled", "DB_PASSWORD=test"} {
		if !strings.Contains(got, want) {
			t.Errorf("merged testing contract omitted %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "owner-secret") || strings.Contains(got, "owner-password") {
		t.Fatalf("merged testing contract exposed an owner secret:\n%s", got)
	}
	if strings.Contains(got, "REDIS_HOST=") {
		t.Fatalf("merged testing contract retained a removed framework key:\n%s", got)
	}
}

// TestMergeTestingIsStable verifies repeated synchronization does not churn committed test configuration.
func TestMergeTestingIsStable(t *testing.T) {
	example := []byte("APP_ENV=local\nDB_DATABASE=./_data/sqlite/app.db\nDB_PASSWORD=\n")
	first := envfile.MergeTesting(nil, example)
	second := envfile.MergeTesting(first, example)
	if string(second) != string(first) {
		t.Fatalf("testing contract changed on the second merge\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestMergeTestingDatabaseNames distinguishes logical database names from supported SQLite filenames.
func TestMergeTestingDatabaseNames(t *testing.T) {
	example := []byte("DB_DATABASE=my.app\nDB_SQLITE_DATABASE=./_data/sqlite/app.sqlite3\nBILLING_DB_HOST=mysql\nBILLING_DB_DATABASE=billing\nBILLING_DB_PASSWORD=\n")
	got := string(envfile.MergeTesting(nil, example))
	for _, want := range []string{
		"DB_DATABASE=my.app_testing",
		"DB_SQLITE_DATABASE=./_data/sqlite/app_testing.sqlite3",
		"BILLING_DB_HOST=127.0.0.1",
		"BILLING_DB_DATABASE=billing_testing",
		"BILLING_DB_PASSWORD=test",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("testing contract omitted %q:\n%s", want, got)
		}
	}
}

// TestMergeTestingRefreshesAndPrunesNamedAppValues verifies prefixed framework keys follow the same ownership policy as root keys.
func TestMergeTestingRefreshesAndPrunesNamedAppValues(t *testing.T) {
	existing := envfile.MergeTesting(nil, []byte("BILLING_DB_PASSWORD=\nREMOVED_DB_PASSWORD=\n"))
	example := []byte("BILLING_DB_PASSWORD=\n")
	got := string(envfile.MergeTesting(existing, example))
	if !strings.Contains(got, "BILLING_DB_PASSWORD=test") {
		t.Fatalf("named App password was not refreshed:\n%s", got)
	}
	if strings.Contains(got, "REMOVED_DB_PASSWORD=") {
		t.Fatalf("removed named App password was retained:\n%s", got)
	}
}

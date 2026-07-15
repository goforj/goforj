package forj

import (
	"go/format"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestSyncLegacyJobHandlerRegistrationMigratesKnownJobConstructors verifies old injectors gain registration without guessing at unrelated providers.
func TestSyncLegacyJobHandlerRegistrationMigratesKnownJobConstructors(t *testing.T) {
	const source = `package wire

import (
	"example.com/app/internal/imports"
	"example.com/app/internal/jobs"
	"example.com/app/internal/monitoring"
	"github.com/goforj/wire"
)

var appJobSet = wire.NewSet(
	imports.NewProcessOCRJob,
	jobs.NewExampleHelloJob,
	jobs.NewExampleHelloJobCmd,
	monitoring.NewCheckService,
	monitoring.NewMonitorCheckJob,
)
`

	updated, err := syncLegacyJobHandlerRegistration(source, "example.com/app", project.Components{Jobs: true})
	if err != nil {
		t.Fatalf("syncLegacyJobHandlerRegistration() error: %v", err)
	}
	formatted, err := format.Source([]byte(updated))
	if err != nil {
		t.Fatalf("format migrated injector: %v\n%s", err, updated)
	}
	text := string(formatted)
	for _, want := range []string{
		`"example.com/app/internal/queues"`,
		"registerJobHandlers,",
		"jobsExampleHelloJob *jobs.ExampleHelloJob,",
		"monitoringMonitorCheckJob *monitoring.MonitorCheckJob,",
		"queueManager.Register(jobs.ExampleHelloJobTypeName, jobsExampleHelloJob.HandleTask)",
		"queueManager.Register(monitoring.MonitorCheckJobTypeName, monitoringMonitorCheckJob.HandleTask)",
		"Register preserved provider imports.NewProcessOCRJob here because its handler contract cannot be inferred safely.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("migrated injector omitted %q:\n%s", want, text)
		}
	}
	for _, excluded := range []string{"ProcessOCRJob *", "ExampleHelloJobCmd *", "CheckService *"} {
		if strings.Contains(text, excluded) {
			t.Errorf("migrated injector treated unrelated provider as a job: %s", excluded)
		}
	}
	second, err := syncLegacyJobHandlerRegistration(text, "example.com/app", project.Components{Jobs: true})
	if err != nil {
		t.Fatalf("second syncLegacyJobHandlerRegistration() error: %v", err)
	}
	if second != text {
		t.Fatal("job handler migration was not idempotent")
	}
}

// TestSyncLegacyJobHandlerRegistrationPreservesQueueImportAliases verifies migration references an owner-selected import name.
func TestSyncLegacyJobHandlerRegistrationPreservesQueueImportAliases(t *testing.T) {
	const source = `package wire

import (
	"example.com/app/internal/jobs"
	queueRuntime "example.com/app/internal/queues"
	"github.com/goforj/wire"
)

var appJobSet = wire.NewSet(
	jobs.NewExampleHelloJob,
)
`
	updated, err := syncLegacyJobHandlerRegistration(source, "example.com/app", project.Components{Jobs: true})
	if err != nil {
		t.Fatalf("syncLegacyJobHandlerRegistration() error: %v", err)
	}
	formatted, err := format.Source([]byte(updated))
	if err != nil {
		t.Fatalf("format aliased injector: %v\n%s", err, updated)
	}
	text := string(formatted)
	if !strings.Contains(text, "queueManager *queueRuntime.Manager") {
		t.Fatalf("migration ignored the existing Queue import alias:\n%s", text)
	}
	if strings.Contains(text, "queueManager *queues.Manager") {
		t.Fatalf("migration introduced an undefined Queue import name:\n%s", text)
	}
}

// TestSyncLegacyJobHandlerRegistrationSupportsEmptyAppSets verifies a user may remove every sample job without breaking rerender.
func TestSyncLegacyJobHandlerRegistrationSupportsEmptyAppSets(t *testing.T) {
	const source = `package wire

import "github.com/goforj/wire"

var appJobSet = wire.NewSet(
)
`
	updated, err := syncLegacyJobHandlerRegistration(source, "example.com/app", project.Components{Jobs: true})
	if err != nil {
		t.Fatalf("syncLegacyJobHandlerRegistration() error: %v", err)
	}
	formatted, err := format.Source([]byte(updated))
	if err != nil {
		t.Fatalf("format migrated injector: %v\n%s", err, updated)
	}
	for _, want := range []string{"registerJobHandlers,", "queueManager *queues.Manager", "_ = queueManager"} {
		if !strings.Contains(string(formatted), want) {
			t.Errorf("empty App migration omitted %q:\n%s", want, formatted)
		}
	}
}

// TestSyncLegacyJobHandlerRegistrationPreservesBuildConstraints verifies import creation stays beneath the package declaration.
func TestSyncLegacyJobHandlerRegistrationPreservesBuildConstraints(t *testing.T) {
	const source = `//go:build custom

package wire

var appJobSet = wire.NewSet(
)
`
	updated, err := syncLegacyJobHandlerRegistration(source, "example.com/app", project.Components{Jobs: true})
	if err != nil {
		t.Fatalf("syncLegacyJobHandlerRegistration() error: %v", err)
	}
	formatted, err := format.Source([]byte(updated))
	if err != nil {
		t.Fatalf("format build-constrained injector: %v\n%s", err, updated)
	}
	text := string(formatted)
	if !strings.HasPrefix(text, "//go:build custom\n\npackage wire\n\nimport") {
		t.Fatalf("migration misplaced the generated import:\n%s", text)
	}
}

// TestSyncLegacyJobHandlerRegistrationLeavesDisabledAppsUntouched verifies project support never widens an App without Jobs.
func TestSyncLegacyJobHandlerRegistrationLeavesDisabledAppsUntouched(t *testing.T) {
	const source = "package wire\n"
	got, err := syncLegacyJobHandlerRegistration(source, "example.com/app", project.Components{})
	if err != nil {
		t.Fatalf("syncLegacyJobHandlerRegistration() error: %v", err)
	}
	if got != source {
		t.Fatalf("disabled App injector changed:\n%s", got)
	}
}

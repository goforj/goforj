//go:build integration

package forj

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"gorm.io/gorm"
)

func TestMakeFlowsIntegration(t *testing.T) {
	projectDir := t.TempDir()
	renderAppAtDir(t, projectDir)
	binPath := testkit.EnsureIntegrationForjBinary(t)
	_ = testkit.EnsureIntegrationToolsDir(t)

	runForj := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}

	runForjFailure := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err == nil {
			tb.Fatalf("forj %s unexpectedly succeeded\n%s", strings.Join(args, " "), out.String())
		}
		return out.String()
	}

	runForjWithEnv := func(tb testing.TB, env map[string]string, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, env)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}

	buildApp := func(tb testing.TB) {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, "build", "-o", "./bin/app")
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj build failed: %v\n%s", err, out.String())
		}
		if _, err := os.Stat(filepath.Join(projectDir, "bin", "app")); err != nil {
			tb.Fatalf("expected built app binary: %v", err)
		}
	}

	runApp := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(projectDir, "bin", "app"), args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("app %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}

	runForj(t, "make:controller", "Audit")
	runForj(t, "make:controller", "Audit")
	assertFileContains(t, filepath.Join(projectDir, "internal", "audit", "controller.go"), []string{
		"package audit",
		`web.NewRoute(http.MethodGet, "/audit", c.Get)`,
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "wire", "inject_http_controllers_app.go"), []string{
		`"example.com/testapp/internal/audit"`,
		"audit.NewController",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "routes.go"), []string{
		`"example.com/testapp/internal/audit"`,
		"auditController *audit.Controller",
		"auditController.Routes(),",
	})

	runForj(t, "make:controller", "Billing:Reports")
	assertFileContains(t, filepath.Join(projectDir, "internal", "billing", "reports", "controller.go"), []string{
		"package reports",
		`web.NewRoute(http.MethodGet, "/billing/reports", c.Get)`,
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "wire", "inject_http_controllers_app.go"), []string{
		`billingReports "example.com/testapp/internal/billing/reports"`,
		"billingReports.NewController",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "routes.go"), []string{
		`billingReports "example.com/testapp/internal/billing/reports"`,
		"billingReportsController *billingReports.Controller",
		"billingReportsController.Routes(),",
	})

	runForj(t, "make:event", "UserRegistered")
	assertFileContains(t, filepath.Join(projectDir, "internal", "events", "user_registered_event.go"), []string{
		"package events",
		"const UserRegisteredEventTopic",
		"type UserRegisteredEvent struct",
	})
	runForj(t, "make:event", "Billing:InvoicePaid")
	assertFileContains(t, filepath.Join(projectDir, "internal", "billing", "invoice_paid_event.go"), []string{
		"package billing",
		"const InvoicePaidEventTopic",
		"type InvoicePaidEvent struct",
	})
	runForj(t, "make:subscriber", "Billing:InvoicePaid")
	assertFileContains(t, filepath.Join(projectDir, "internal", "billing", "invoice_paid_subscriber.go"), []string{
		"package billing",
		"// Generated by `forj make:subscriber`. Edit this App-owned subscriber.",
		"type InvoicePaidSubscriber struct",
		"func NewInvoicePaidSubscriber() *InvoicePaidSubscriber",
		"func (s *InvoicePaidSubscriber) Handle(ctx context.Context, event InvoicePaidEvent) error",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "wire", "inject_subscribers_app.go"), []string{
		"// App-owned Wire injector. EDIT THIS FILE.",
		"// Add application event subscriber providers here, or use `forj make:subscriber`.",
		`"example.com/testapp/internal/billing"`,
		"billing.NewInvoicePaidSubscriber",
		`eventManager.Named("default")`,
		"billingInvoicePaidSubscriber.Handle",
	})

	writeMakeFlowFile(t, filepath.Join(projectDir, "app", "reporting", "wire", "inject_cmd_app.go"), `package wire

import "github.com/goforj/wire"

var appCommandSet = wire.NewSet(
)
`)
	writeMakeFlowFile(t, filepath.Join(projectDir, "app", "reporting", "commands.go"), `package reporting

type Commands struct {
}

func NewCommands(
) *Commands {
	return &Commands{
	}
}
`)
	runForjWithEnv(t, map[string]string{"FORJ_APP": "reporting"}, "make:command", "Reports:Sync")
	assertFileContains(t, filepath.Join(projectDir, "app", "reporting", "wire", "inject_cmd_app.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"reports.NewSyncCmd",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "reporting", "commands.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"ReportsSyncCmd reports.SyncCmd",
		"reportsSyncCmd *reports.SyncCmd",
	})
	assertFileNotContains(t, filepath.Join(projectDir, "app", "wire", "inject_cmd_app.go"), []string{
		"reports.NewSyncCmd",
	})
	assertFileNotContains(t, filepath.Join(projectDir, "app", "commands.go"), []string{
		"ReportsSyncCmd reports.SyncCmd",
	})

	runForj(t, "make:queue", "reports", "--workers", "2", "--name", "production-report-jobs")
	assertFileContains(t, filepath.Join(projectDir, ".env"), []string{
		"# Queue",
		"# Named queues can prioritize work by allocating workers:",
		"QUEUE_REPORTS_NAME=production-report-jobs",
		"QUEUE_REPORTS_WORKERS=2",
	})
	assertFileNotContains(t, filepath.Join(projectDir, ".env"), []string{
		"# QUEUE_REPORTS_NAME=reports",
		"# QUEUE_REPORTS_WORKERS=2",
		"QUEUE_QUEUES",
	})
	runForj(t, "make:queue", "reports", "--workers", "4")
	assertFileContains(t, filepath.Join(projectDir, ".env"), []string{
		"QUEUE_REPORTS_NAME=reports",
		"QUEUE_REPORTS_WORKERS=4",
	})

	runForj(t, "make:job", "SyncReports", "--queue", "reports")
	assertFileContains(t, filepath.Join(projectDir, "internal", "jobs", "sync_reports_job.go"), []string{
		"package jobs",
		"const SyncReportsJobTypeName",
		"type SyncReportsJob struct",
		`.OnQueue("reports")`,
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "wire", "inject_jobs_app.go"), []string{
		"jobs.NewSyncReportsJob",
	})
	runForj(t, "make:job", "Billing:SyncReports")
	assertFileContains(t, filepath.Join(projectDir, "internal", "billing", "sync_reports_job.go"), []string{
		"package billing",
		"const SyncReportsJobTypeName",
		"type SyncReportsJob struct",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "wire", "inject_jobs_app.go"), []string{
		`"example.com/testapp/internal/billing"`,
		"billing.NewSyncReportsJob",
	})

	runForj(t, "make:migration", "create_widgets")
	assertGlob(t, filepath.Join(projectDir, "migrations", "*create_widgets.up.sql"))
	assertGlob(t, filepath.Join(projectDir, "migrations", "*create_widgets.down.sql"))

	runForj(t, "make:schedule", "Reports:Daily", "--every", "24h")
	assertFileContains(t, filepath.Join(projectDir, "internal", "reports", "daily_schedule.go"), []string{
		"package reports",
		"// Generated by `forj make:schedule`. Edit this App-owned schedule.",
		`const DailyScheduleName = "reports:daily"`,
		`const DailyScheduleInterval = "24h"`,
		"type DailySchedule struct",
		"func (s *DailySchedule) Name() string",
		"func (s *DailySchedule) Interval() (time.Duration, error)",
	})
	assertFileNotContains(t, filepath.Join(projectDir, "internal", "reports", "daily_schedule.go"), []string{
		"schedulerx",
		"github.com/goforj/scheduler",
	})
	assertFileContains(t, filepath.Join(projectDir, "internal", "schedules", "scheduler.go"), []string{
		"// Code generated by GoForj CLI. DO NOT EDIT.",
		"// App-owned schedule registration belongs in app/schedules.go.",
		"registry ScheduleRegistry",
		"registry:",
	})
	assertFileNotContains(t, filepath.Join(projectDir, "internal", "schedules", "scheduler.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"dailySchedule *reports.DailySchedule",
	})
	assertFileContains(t, filepath.Join(projectDir, "internal", "schedules", "registration.go"), []string{
		"return s.registry.Register(s)",
		"func (s *Scheduler) registerJobObservers()",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "wire", "inject_schedules_app.go"), []string{
		"// App-owned Wire injector. EDIT THIS FILE.",
		"// Add app schedule providers here, or use `forj make:schedule`.",
		`"example.com/testapp/internal/reports"`,
		"reports.NewDailySchedule",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "schedules.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"dailySchedule *reports.DailySchedule",
		"dailySchedule: dailySchedule,",
		"if err := schedules.RegisterRecurring(s, r.dailySchedule); err != nil {",
	})

	legacyScheduleInjector := `package wire

import (
	"github.com/goforj/wire"
	compositionapp "example.com/testapp/app"
	"example.com/testapp/internal/reports"
	"example.com/testapp/internal/scheduler"
)

var appScheduleSet = wire.NewSet(
	compositionapp.NewScheduleRegistry,
	wire.Bind(new(scheduler.ScheduleRegistry), new(*compositionapp.ScheduleRegistry)),
	reports.NewDailySchedule,
)
`
	legacyScheduleRegistry := `package app

import (
	"example.com/testapp/internal/reports"
	"example.com/testapp/internal/scheduler"
)

type ScheduleRegistry struct {
	dailySchedule *reports.DailySchedule
}

func NewScheduleRegistry(
	dailySchedule *reports.DailySchedule,
) *ScheduleRegistry {
	return &ScheduleRegistry{
		dailySchedule: dailySchedule,
	}
}

func (r *ScheduleRegistry) Register(s *scheduler.Scheduler) error {
	if err := scheduler.RegisterRecurring(s, r.dailySchedule); err != nil {
		return err
	}
	return nil
}
`
	scheduleInjectorPath := filepath.Join(projectDir, "app", "wire", "inject_schedules_app.go")
	if err := os.WriteFile(scheduleInjectorPath, []byte(legacyScheduleInjector), 0o644); err != nil {
		t.Fatalf("write legacy schedule injector: %v", err)
	}
	scheduleRegistryPath := filepath.Join(projectDir, "app", "schedules.go")
	if err := os.WriteFile(scheduleRegistryPath, []byte(legacyScheduleRegistry), 0o644); err != nil {
		t.Fatalf("write legacy schedule registry: %v", err)
	}
	renderAppAtDir(t, projectDir)
	assertFileContains(t, scheduleInjectorPath, []string{
		`compositionapp "example.com/testapp/app"`,
		`"example.com/testapp/internal/schedules"`,
		`"example.com/testapp/internal/reports"`,
		"reports.NewDailySchedule",
		"compositionapp.NewScheduleRegistry",
		"wire.Bind(new(schedules.ScheduleRegistry), new(*compositionapp.ScheduleRegistry))",
	})
	assertFileContains(t, scheduleRegistryPath, []string{
		`"example.com/testapp/internal/schedules"`,
		`"example.com/testapp/internal/reports"`,
		"dailySchedule *reports.DailySchedule",
		"if err := schedules.RegisterRecurring(s, r.dailySchedule); err != nil {",
	})
	assertFileNotContains(t, scheduleInjectorPath, []string{
		`"example.com/testapp/internal/scheduler"`,
	})
	assertFileNotContains(t, scheduleRegistryPath, []string{
		`"example.com/testapp/internal/scheduler"`,
		"scheduler.RegisterRecurring",
	})

	scheduleHelp := runForj(t, "make:schedule", "--help")
	if !strings.Contains(scheduleHelp, "Examples") || !strings.Contains(scheduleHelp, "forj make:schedule reports:daily --every 24h") {
		t.Fatalf("expected make:schedule help to include example, got:\n%s", scheduleHelp)
	}

	missingScheduleNameOutput := runForjFailure(t, "make:schedule")
	if !strings.Contains(missingScheduleNameOutput, `expected "<name>"`) {
		t.Fatalf("expected missing schedule name error, got:\n%s", missingScheduleNameOutput)
	}
	if !strings.Contains(missingScheduleNameOutput, "example: forj make:schedule reports:daily --every 24h") {
		t.Fatalf("expected missing schedule name error to include example, got:\n%s", missingScheduleNameOutput)
	}
	if strings.Contains(missingScheduleNameOutput, "Error executing command") || strings.Contains(missingScheduleNameOutput, "System") {
		t.Fatalf("expected console error output without app logger noise, got:\n%s", missingScheduleNameOutput)
	}
	if strings.Contains(missingScheduleNameOutput, "exit status") || strings.Contains(missingScheduleNameOutput, "go run:") {
		t.Fatalf("expected app command parse error without process wrapper noise, got:\n%s", missingScheduleNameOutput)
	}

	queueHelp := runForj(t, "make:queue", "--help")
	if !strings.Contains(queueHelp, "Examples") || !strings.Contains(queueHelp, "forj make:queue emails --workers 6") {
		t.Fatalf("expected make:queue help to include example, got:\n%s", queueHelp)
	}

	subscriberHelp := runForj(t, "make:subscriber", "--help")
	if !strings.Contains(subscriberHelp, "Examples") || !strings.Contains(subscriberHelp, "forj make:subscriber billing:invoice-paid") {
		t.Fatalf("expected make:subscriber help to include example, got:\n%s", subscriberHelp)
	}

	missingQueueNameOutput := runForjFailure(t, "make:queue", "--workers", "6")
	if !strings.Contains(missingQueueNameOutput, "missing queue name") {
		t.Fatalf("expected missing queue name error, got:\n%s", missingQueueNameOutput)
	}
	if !strings.Contains(missingQueueNameOutput, "example: forj make:queue emails --workers 6") {
		t.Fatalf("expected missing queue name error to include example, got:\n%s", missingQueueNameOutput)
	}
	if strings.Contains(missingQueueNameOutput, "Error executing command") || strings.Contains(missingQueueNameOutput, "System") {
		t.Fatalf("expected console error output without app logger noise, got:\n%s", missingQueueNameOutput)
	}

	missingQueueWorkersOutput := runForjFailure(t, "make:queue", "emails")
	if !strings.Contains(missingQueueWorkersOutput, "missing workers") {
		t.Fatalf("expected missing queue workers error, got:\n%s", missingQueueWorkersOutput)
	}

	composedHelp := runForj(t, "--help")
	for _, want := range []string{"GoForj CLI", "make:command", "route:list", "make:job", "make:queue", "make:subscriber"} {
		if !strings.Contains(composedHelp, want) {
			t.Fatalf("expected composed help to include %s, got:\n%s", want, composedHelp)
		}
	}
	for _, unexpected := range []string{"Framework Commands", "Application Commands", "Generators", "Migrations", "Unknown commands are delegated to this app.", "────"} {
		if strings.Contains(composedHelp, unexpected) {
			t.Fatalf("expected composed help not to include %s, got:\n%s", unexpected, composedHelp)
		}
	}

	sourceRoutes := runForj(t, "route:list")
	if !strings.Contains(sourceRoutes, "/audit") {
		t.Fatalf("expected delegated forj route:list to include generated audit route, got:\n%s", sourceRoutes)
	}
	if !strings.Contains(sourceRoutes, "/billing/reports") {
		t.Fatalf("expected delegated forj route:list to include generated billing reports route, got:\n%s", sourceRoutes)
	}

	buildApp(t)
	routes := runApp(t, "route:list")
	if !strings.Contains(routes, "/audit") {
		t.Fatalf("expected route:list to include generated audit route, got:\n%s", routes)
	}
	if !strings.Contains(routes, "/billing/reports") {
		t.Fatalf("expected route:list to include generated billing reports route, got:\n%s", routes)
	}
}

func TestMakeFlowsAppIsolationIntegration(t *testing.T) {
	projectDir := t.TempDir()
	renderAppAtDir(t, projectDir)
	binPath := testkit.EnsureIntegrationForjBinary(t)
	_ = testkit.EnsureIntegrationToolsDir(t)

	runForj := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}

	buildApp := func(tb testing.TB, appName string) {
		tb.Helper()
		args := []string{"build", "-o", "./bin/" + appName}
		if appName != project.DefaultAppName {
			args = append([]string{appName}, args...)
		}
		runForj(tb, args...)
		if _, err := os.Stat(filepath.Join(projectDir, "bin", appName)); err != nil {
			tb.Fatalf("expected built %s binary: %v", appName, err)
		}
	}

	runForj(t, "make:app", "billing", "--components", "web-api,jobs,scheduler")

	runForj(t, "billing", "make:controller", "Billing:Invoices")
	runForj(t, "billing", "make:job", "Billing:SendInvoices", "--queue", "invoices")
	runForj(t, "billing", "make:schedule", "Billing:DailyClose", "--every", "1h")

	assertFileContains(t, filepath.Join(projectDir, "app", "billing", "routes.go"), []string{
		`billingInvoices "example.com/testapp/internal/billing/invoices"`,
		"billingInvoicesController *billingInvoices.Controller",
		"billingInvoicesController.Routes(),",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "billing", "wire", "inject_http_controllers_app.go"), []string{
		`billingInvoices "example.com/testapp/internal/billing/invoices"`,
		"billingInvoices.NewController",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "billing", "wire", "inject_jobs_app.go"), []string{
		`"example.com/testapp/internal/billing"`,
		"billing.NewSendInvoicesJob",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "billing", "wire", "inject_schedules_app.go"), []string{
		`"example.com/testapp/internal/billing"`,
		"billing.NewDailyCloseSchedule",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "billing", "schedules.go"), []string{
		"dailyCloseSchedule *billing.DailyCloseSchedule",
		"dailyCloseSchedule: dailyCloseSchedule,",
		"if err := schedules.RegisterRecurring(s, r.dailyCloseSchedule); err != nil {",
	})

	assertFileNotContains(t, filepath.Join(projectDir, "app", "routes.go"), []string{
		"billingInvoicesController",
		"billingInvoices.Controller",
		"billingInvoicesController.Routes(),",
	})
	assertFileNotContains(t, filepath.Join(projectDir, "app", "wire", "inject_jobs_app.go"), []string{
		"billing.NewSendInvoicesJob",
	})
	assertFileNotContains(t, filepath.Join(projectDir, "app", "wire", "inject_schedules_app.go"), []string{
		"billing.NewDailyCloseSchedule",
	})
	assertFileNotContains(t, filepath.Join(projectDir, "app", "schedules.go"), []string{
		"dailyCloseSchedule *billing.DailyCloseSchedule",
	})

	runForj(t, "make:controller", "Reports:Overview")
	runForj(t, "make:job", "Reports:BuildSummary", "--queue", "reports")
	runForj(t, "make:schedule", "Reports:Nightly", "--every", "24h")

	assertFileContains(t, filepath.Join(projectDir, "app", "routes.go"), []string{
		`reportsOverview "example.com/testapp/internal/reports/overview"`,
		"reportsOverviewController *reportsOverview.Controller",
		"reportsOverviewController.Routes(),",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "wire", "inject_jobs_app.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"reports.NewBuildSummaryJob",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "wire", "inject_schedules_app.go"), []string{
		`"example.com/testapp/internal/reports"`,
		"reports.NewNightlySchedule",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "schedules.go"), []string{
		"nightlySchedule *reports.NightlySchedule",
		"nightlySchedule: nightlySchedule,",
	})

	assertFileNotContains(t, filepath.Join(projectDir, "app", "billing", "routes.go"), []string{
		"reportsOverviewController",
		"reportsOverview.Controller",
	})
	assertFileNotContains(t, filepath.Join(projectDir, "app", "billing", "wire", "inject_jobs_app.go"), []string{
		"reports.NewBuildSummaryJob",
	})
	assertFileNotContains(t, filepath.Join(projectDir, "app", "billing", "wire", "inject_schedules_app.go"), []string{
		"reports.NewNightlySchedule",
	})
	assertFileNotContains(t, filepath.Join(projectDir, "app", "billing", "schedules.go"), []string{
		"nightlySchedule *reports.NightlySchedule",
	})

	buildApp(t, project.DefaultAppName)
	buildApp(t, "billing")
}

func TestMakeAppBuildsNamedAppAfterFullRender(t *testing.T) {
	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "TestApp",
			GoModuleName: "example.com/testapp",
			UpdatedAt:    "2026-06-07 00:00:00 UTC",
			Render: project.RenderConfig{
				QueueDriver: "redis",
				StarterKit:  project.StarterKitVue,
				Components: project.Components{
					CLI:           true,
					DemoApp:       true,
					Mail:          true,
					Auth:          true,
					OAuth:         true,
					WebAPI:        true,
					WebUI:         true,
					Metrics:       true,
					Observability: true,
					Grafana:       true,
					Docker:        true,
					DatabaseMySQL: true,
					Scheduler:     true,
					Jobs:          true,
				},
			},
		},
	})

	binPath := testkit.EnsureIntegrationForjBinary(t)
	runForj := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}

	runForj(t, "make:app", "billing")
	rootHelp := runForj(t, "--help")
	for _, want := range []string{"GoForj CLI", "testapp · available in all apps", "testapp · app"} {
		if !strings.Contains(rootHelp, want) {
			t.Fatalf("expected root help to include %q, got:\n%s", want, rootHelp)
		}
	}
	if strings.Contains(rootHelp, "App:") {
		t.Fatalf("expected root help to omit app labels, got:\n%s", rootHelp)
	}
	runForj(t, "build", "-o", "./bin/billing", "./cmd/billing")

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir project: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	t.Setenv("PATH", filepath.Dir(binPath)+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load project config: %v", err)
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	if err := runDevBuild(cfg, &out, &errOut); err != nil {
		t.Fatalf("runDevBuild failed: %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errOut.String())
	}
}

func TestMakeModelFlowIntegration(t *testing.T) {
	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "TestApp",
			GoModuleName: "example.com/testapp",
			UpdatedAt:    "2026-01-01 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					WebAPI:         true,
					DatabaseSQLite: true,
				},
			},
		},
	})

	dbPath := filepath.Join(projectDir, "_data", "sqlite", "app.db")
	seedSQLiteMakeModelTable(t, dbPath)

	binPath := testkit.EnsureIntegrationForjBinary(t)
	runForj := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}

	runForjFailure := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err == nil {
			tb.Fatalf("forj %s unexpectedly succeeded\n%s", strings.Join(args, " "), out.String())
		}
		return out.String()
	}

	missingModelNameOutput := runForjFailure(t, "make:model")
	if !strings.Contains(missingModelNameOutput, `expected "<table>"`) {
		t.Fatalf("expected missing model table error, got:\n%s", missingModelNameOutput)
	}
	if !strings.Contains(missingModelNameOutput, "example: forj make:model users") {
		t.Fatalf("expected missing model table error to include example, got:\n%s", missingModelNameOutput)
	}
	if strings.Contains(missingModelNameOutput, "initializing application") ||
		strings.Contains(missingModelNameOutput, "exit status") ||
		strings.Contains(missingModelNameOutput, "go run:") {
		t.Fatalf("expected make:model parse error without app boot or process wrapper noise, got:\n%s", missingModelNameOutput)
	}

	runForj(t, "make:model", "widgets")
	runForj(t, "make:model", "widgets")

	assertFileContains(t, filepath.Join(projectDir, "internal", "models", "widget.go"), []string{
		"type Widget struct",
		`gorm:"column:name`,
		`func (*Widget) TableName() string`,
		"type WidgetRepo struct",
		"func NewWidgetRepo(",
	})
	assertFileContains(t, filepath.Join(projectDir, "app", "wire", "inject_repositories_app.go"), []string{
		"models.NewWidgetRepo",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "build", "-o", "./bin/app")
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("forj build failed after make:model: %v\n%s", err, out.String())
	}
}

// seedSQLiteMakeModelTable creates a small schema that exercises make:model repository wiring.
func seedSQLiteMakeModelTable(t *testing.T, dbPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatalf("prepare sqlite dir: %v", err)
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	if err := db.Exec(`CREATE TABLE widgets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME NULL
	)`).Error; err != nil {
		t.Fatalf("create widgets table: %v", err)
	}
}

// assertGlob verifies generators created at least one timestamped file for the requested pattern.
func assertGlob(t *testing.T, pattern string) {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected at least one match for %s", pattern)
	}
}

// writeMakeFlowFile writes a fixture file for generator integration checks.
func writeMakeFlowFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// assertFileNotContains verifies forbidden snippets are absent from a file.
func assertFileNotContains(t *testing.T, path string, forbidden []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(data)
	for _, value := range forbidden {
		if strings.Contains(content, value) {
			t.Fatalf("unexpected %q in %s\n\n%s", value, path, content)
		}
	}
}

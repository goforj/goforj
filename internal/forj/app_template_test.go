package forj

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWireAppTemplateUsesSingularDefaultAndPluralManagers(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templatePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "wire", "app.go.tmpl")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read app.go template: %v", err)
	}
	source := string(content)

	for _, snippet := range []string{
		"func (a *App) Cache() *cache.Cache",
		"return a.cache.Default()",
		"func (a *App) Caches() *caches.Manager",
		"func (a *App) Storage() *storages.Manager",
		"func (a *App) Bus() eventcore.Bus",
		"return a.events.Default()",
		"func (a *App) Events() *eventcore.Manager",
		"func (a *App) Queue() *queue.Queue",
		"return a.queues.Default()",
		"func (a *App) Queues() *queues.Manager",
		"case queue.DriverWorkerpool:",
		"defaultQueue.StartWorkers",
		`appTimeouts.QueueShutdownTimeout()`,
		`app.NewLifecycle(appTimeouts)`,
		`logger.Debug().Msg("Shutting down database connections...")`,
		`func (a *App) appShutdownTimeout() time.Duration`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected wire app template to contain %q", snippet)
		}
	}
}

func TestAboutCommandTemplateIsWired(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	base := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "cmd")

	files := map[string][]string{
		filepath.Join(base, "about_cmd.go.tmpl"): {
			`name:"about" help:"Show environment and configured services for this app" goforj:"skip_boot"`,
			`type AboutCmd struct {`,
			`JSON`,
			`NoColor`,
			`func (c *AboutCmd) renderAboutSection(`,
			`appinfo "{{.GoModuleName}}/internal/app"`,
			`func (c *AboutCmd) aboutService() *appinfo.AboutService`,
		},
		filepath.Join(base, "health_cmd.go.tmpl"): {
			`name:"health" help:"Query a live app readiness or liveness endpoint" goforj:"skip_boot"`,
			`type HealthCmd struct {`,
			`Probe`,
			`TimeoutMs`,
			`github.com/goforj/httpx`,
			`func (c *HealthCmd) probeURL() (string, error)`,
			`writer.AppendHeader(table.Row{"Type", "Name", "Driver", "Status", "Details"})`,
			`return printJSON(map[string]any{`,
		},
		filepath.Join(base, "about_grid.go.tmpl"): {
			`func aboutSplitSections(`,
			`func aboutPrimitiveGridColumns(`,
			`func aboutRenderGrid(`,
		},
		filepath.Join(base, "skip_boot.go.tmpl"): {
			`var skipBootFactories = []skipBootFactory{`,
			`func() interface{} { return NewAboutCmd() },`,
			`{{- if or .Components.WebAPI .Components.WebUI }}`,
			`func() interface{} { return NewHealthCmd() },`,
			`func MaybeRunSkipBootCommand(args []string) (bool, error)`,
			`func skipBootCommandMetadata(command interface{}) (string, bool)`,
			`commandSignatureValue(signature, "goforj") == "skip_boot"`,
		},
		filepath.Join(filepath.Dir(base), "app", "about.go.tmpl"): {
			`package app`,
			`type AboutService struct{}`,
			`func (s *AboutService) Build() AboutReport`,
			`type AboutSectionData struct {`,
			`type AboutConnectionData struct {`,
			`func aboutDatabaseDetails(name string) []AboutField`,
		},
		filepath.Join(filepath.Dir(base), "app", "discovery.go.tmpl"): {
			`package app`,
			`type PrimitiveInstance struct {`,
			`func DiscoverCacheInstances() []PrimitiveInstance`,
			`func DiscoverQueueInstances() []PrimitiveInstance`,
			`func DiscoverStorageInstances() []PrimitiveInstance`,
			`func DiscoverEventInstances() []PrimitiveInstance`,
			`func DiscoverDatabaseInstances() []PrimitiveInstance`,
			`func QueueDefaultQueue(name string) string`,
		},
		filepath.Join(filepath.Dir(base), "http", "readiness_checks.go.tmpl"): {
			`func ProvideReadinessChecks(`,
			`for _, check := range caches.ReadinessChecks() {`,
			`for _, check := range storage.ReadinessChecks() {`,
			`for _, check := range events.ReadinessChecks() {`,
			`for _, check := range queues.ReadinessChecks() {`,
			`for _, check := range db.ReadinessChecks() {`,
			`Check: check.Check,`,
		},
		filepath.Join(base, "app_commands.go.tmpl"): {
			`AboutCmd AboutCmd ` + "`cmd:\"\"`",
			`{{- if or .Components.WebAPI .Components.WebUI }}`,
			`HealthCmd HealthCmd ` + "`cmd:\"\"`",
			`aboutCmd *AboutCmd,`,
			`healthCmd *HealthCmd,`,
			`AboutCmd: *aboutCmd,`,
			`HealthCmd: *healthCmd,`,
		},
		filepath.Join(base, "wire.go.tmpl"): {
			`NewAboutCmd,`,
			`{{- if or .Components.WebAPI .Components.WebUI }}`,
			`NewHealthCmd,`,
		},
	}

	for path, snippets := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read template %s: %v", path, err)
		}
		source := string(content)
		for _, snippet := range snippets {
			if !strings.Contains(source, snippet) {
				t.Fatalf("expected %s to contain %q", path, snippet)
			}
		}
	}
}

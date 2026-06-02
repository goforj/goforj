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
		"func (a *App) Topology() app.RuntimeTopology",
		"return a.topology.Normalized()",
		`Mode: app.NormalizeRuntimeMode(os.Getenv("RUNTIME_MODE"))`,
		"func (a *App) Caches() *caches.Manager",
		"func (a *App) Storage() *storages.Manager",
		"func (a *App) Bus() eventcore.Bus",
		"return a.events.Default()",
		"func (a *App) Events() *eventcore.Manager",
		"func (a *App) Queue() *queue.Queue",
		"return a.queues.Default()",
		"func (a *App) Queues() *queues.Manager",
		`app.NewLifecycle(appTimeouts)`,
		`appLogger.Debug().Msg("Shutting down database connections...")`,
		`func (a *App) appShutdownTimeout() time.Duration`,
		`cmd.CommandParseError(parser, command, err)`,
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
		filepath.Join(base, "db_shell_cmd.go.tmpl"): {
			`name:"db:shell" aliases:"db" help:"Open a shell for a configured database connection" goforj:"skip_boot"`,
			`type DBShellCmd struct {`,
			`RawArgs       []string`,
			`passthrough:""`,
			`Method        string`,
			`func (c *DBShellCmd) applyInlineWrapperFlags() error`,
			`func (c *DBShellCmd) parsedArgs() dbShellParsedArgs`,
			`func (*DBShellCmd) Help() string`,
			`forj db -- --batch -e \"select count(*) from users\"`,
			`func NewDBShellCmd() *DBShellCmd`,
			`func (c *DBShellCmd) resolveLaunch(conn dbShellConnection)`,
			`exec.Command(launch.Command, launch.Args...)`,
			`appinfo "{{.GoModuleName}}/internal/app"`,
		},
		filepath.Join(base, "about_grid.go.tmpl"): {
			`func aboutSplitSections(`,
			`func aboutPrimitiveGridColumns(`,
			`func aboutRenderGrid(`,
		},
		filepath.Join(base, "skip_boot.go.tmpl"): {
			`var skipBootFactories = []skipBootFactory{`,
			`func() interface{} { return NewAboutCmd() },`,
			`func() interface{} { return NewDBShellCmd() },`,
			`func() interface{} { return makecmd.NewCommandCmd() },`,
			`func() interface{} { return makecmd.NewControllerCmd() },`,
			`func() interface{} { return makecmd.NewMigrationCmd() },`,
			`{{- if or .Components.WebAPI .Components.WebUI }}`,
			`func() interface{} { return NewHealthCmd() },`,
			`func MaybeRunSkipBootCommand(args []string) (bool, error)`,
			`func() interface{} { return makecmd.NewQueueCmd() },`,
			`func skipBootCommandMetadata(command interface{}) (string, bool)`,
			`commandSignatureValue(signature, "goforj") == "skip_boot"`,
			`func applyStandaloneSkipBootSignature(node *kong.Node, command standaloneCommand)`,
		},
		filepath.Join(base, "default_launch.go.tmpl"): {
			`var DefaultLaunchCommand string`,
			`func EffectiveLaunchArgs(args []string) []string`,
			`if len(args) > 0 {`,
			`return []string{command}`,
		},
		filepath.Join(base, "env_defaults.go.tmpl"): {
			`var CompiledEnvDefaultsBase64 string`,
			`var CompiledEnvOverridesBase64 string`,
			`func ApplyCompiledEnvDefaults() error`,
			`func ApplyCompiledEnvOverrides() error`,
			`base64.StdEncoding.DecodeString`,
			`return applyCompiledEnvMap(strings.TrimSpace(CompiledEnvOverridesBase64), true)`,
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
			`DBShellCmd DBShellCmd ` + "`cmd:\"\"`",
			`{{- if or .Components.WebAPI .Components.WebUI }}`,
			`HealthCmd HealthCmd ` + "`cmd:\"\"`",
			`aboutCmd *AboutCmd,`,
			`dbShellCmd *DBShellCmd,`,
			`healthCmd *HealthCmd,`,
			`AboutCmd: *aboutCmd,`,
			`DBShellCmd: *dbShellCmd,`,
			`HealthCmd: *healthCmd,`,
		},
		filepath.Join(base, "wire.go.tmpl"): {
			`NewAboutCmd,`,
			`NewDBShellCmd,`,
			`makecmd.NewCommandCmd,`,
			`{{- if or .Components.WebAPI .Components.WebUI }}`,
			`NewHealthCmd,`,
			`makecmd.NewControllerCmd,`,
			`makecmd.NewMigrationCmd,`,
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

func TestRunCommandTemplateUsesRuntimeHost(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templatePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "cmd", "run_cmd.go.tmpl")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read run_cmd template: %v", err)
	}
	source := string(content)

	for _, snippet := range []string{
		`app.NewRuntimeHost(runtimes...).Run(ctx)`,
		`DisableMetricsEndpoint: true,`,
		`type RunCmd struct {`,
		`func NewRunCmd(`,
		`httpRuntime *http.Runtime`,
		`schedulerRuntime *schedules.Runtime`,
		`jobsRuntime *jobs.Runtime`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected run_cmd template to contain %q", snippet)
		}
	}
	for _, snippet := range []string{
		`exec.Command(`,
		`os.Executable()`,
		`FORJ_SUBPROCESS=1`,
	} {
		if strings.Contains(source, snippet) {
			t.Fatalf("did not expect run_cmd template to contain %q", snippet)
		}
	}
}

func TestSourcePropagationTemplates(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	root := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates")

	files := map[string][]string{
		filepath.Join(root, "wire", "app.go.tmpl"): {
			`ctx, stop := app.CLINotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)`,
			`parseCtx.BindTo(ctx, (*context.Context)(nil))`,
		},
		filepath.Join(root, "internal", "http", "server.go.tmpl"): {
			`router.Use(s.sourceContextMiddleware(app.SourceHTTP))`,
			`carrier.SetAppSourceName(sourceName)`,
		},
		filepath.Join(root, "internal", "schedules", "scheduler.go.tmpl"): {
			`WithTaskContextDecorator(func(ctx context.Context) context.Context {`,
			`return app.WithSource(ctx, app.SourceScheduler)`,
		},
		filepath.Join(root, "wire", "inject_app_services.go.tmpl"): {
			`setupCtx := app.BackgroundSourceContext(app.SourceStartup)`,
		},
		filepath.Join(root, "demo", "internal", "monitoring", "controller.go.tmpl"): {
			`startupCtx := app.BackgroundSourceContext(app.SourceStartup)`,
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

func TestMainTemplateUsesEffectiveLaunchArgs(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templatePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "main.go.tmpl")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read main.go template: %v", err)
	}
	source := string(content)

	for _, snippet := range []string{
		`args := cmd.EffectiveLaunchArgs(os.Args[1:])`,
		`"{{.GoModuleName}}/internal/console"`,
		`if err := cmd.LoadEnv(); err != nil {`,
		`console.Fatalf("%v", err)`,
		`cmd.MaybeRunSkipBootCommand(args)`,
		`app.Run(nil, args)`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected main template to contain %q", snippet)
		}
	}
}

func TestCommandMetadataLivesInSignatures(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templatePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "cmd", "root_cmd.go.tmpl")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read root_cmd template: %v", err)
	}
	source := string(content)

	for _, snippet := range []string{
		`MakeCommandCmd    makecmd.CommandCmd    ` + "`cmd:\"\"`",
		`MakeControllerCmd makecmd.ControllerCmd ` + "`cmd:\"\"`",
		`MakeEventCmd      makecmd.EventCmd      ` + "`cmd:\"\"`",
		`MakeMigrationCmd  makecmd.MigrationCmd  ` + "`cmd:\"\"`",
		`MakeSubscriberCmd makecmd.SubscriberCmd ` + "`cmd:\"\"`",
		`RunCmd            RunCmd                      ` + "`cmd:\"\"`",
		`HttpServeCmd       http.ServeCmd                ` + "`cmd:\"\"`",
		`SchedulerCmd       schedules.Cmd                ` + "`cmd:\"\"`",
		`QueueWorkerCmd     jobs.WorkerCmd               ` + "`cmd:\"\"`",
		`MakeQueueCmd       makecmd.QueueCmd             ` + "`cmd:\"\"`",
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected root command template to contain %q", snippet)
		}
	}
	for _, snippet := range []string{
		`name:"run" aliases:"app"`,
		`name:"http:serve" aliases:"api"`,
		`name:"schedule:run" aliases:"scheduler"`,
		`name:"queue:work" aliases:"worker"`,
	} {
		if strings.Contains(source, snippet) {
			t.Fatalf("expected root command template not to contain duplicated signature metadata %q", snippet)
		}
	}

	files := map[string][]string{
		filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "cmd", "run_cmd.go.tmpl"): {
			`name:"run" aliases:"app" help:"Run enabled app runtimes together"`,
		},
		filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "http", "serve_cmd.go.tmpl"): {
			`name:"http:serve" aliases:"api" help:"Start the HTTP server"`,
		},
		filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "schedules", "cmd.go.tmpl"): {
			`name:"schedule:run" aliases:"scheduler" help:"Runs the scheduler indefinitely"`,
		},
		filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "jobs", "worker_cmd.go.tmpl"): {
			`name:"queue:work" aliases:"worker" help:"Runs queue workers indefinitely"`,
		},
	}
	for file, snippets := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		source := string(content)
		for _, snippet := range snippets {
			if !strings.Contains(source, snippet) {
				t.Fatalf("expected %s to contain %q", file, snippet)
			}
		}
	}
}

func TestDatabaseRenderingIncludesDBShellCommand(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	rendererPath := filepath.Join(filepath.Dir(currentFile), "project_renderer.go")
	content, err := os.ReadFile(rendererPath)
	if err != nil {
		t.Fatalf("read project renderer: %v", err)
	}
	source := string(content)
	if !strings.Contains(source, `"internal/cmd/db_shell_cmd.go.tmpl"`) {
		t.Fatal("expected database rendering to include db shell command template")
	}
}

func TestWorkerTemplatesSupportNamedQueueSelection(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templateRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "jobs")

	files := map[string][]string{
		filepath.Join(templateRoot, "worker_cmd.go.tmpl"): {
			`Queues  []string ` + "`name:\"queue\" short:\"q\"",
			`Queues: c.Queues`,
		},
		filepath.Join(templateRoot, "runtime.go.tmpl"): {
			`Queues                 []string`,
			`return r.worker.StartWithContext(ctx, cfg.Queues...)`,
		},
		filepath.Join(templateRoot, "worker.go.tmpl"): {
			`func selectManagedQueues(manager *queues.Manager, queueNames ...string)`,
			`unknown queue`,
			`func managedQueueInstances(manager *queues.Manager) []queues.Instance`,
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

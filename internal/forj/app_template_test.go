package forj

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"text/template"

	"github.com/goforj/goforj/project"
)

// TestLighthouseProjectConfigTemplatesPreserveNativeDevConfig verifies that
// generated settings saves retain lifecycle fields older UIs do not expose.
func TestLighthouseProjectConfigTemplatesPreserveNativeDevConfig(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templatesRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates")
	files := map[string][]string{
		filepath.Join("project", "config.go.tmpl"): {
			`package project`,
			`CurrentComponentContractVersion`,
			`ComponentContractVersion int`,
			`Watch    any`,
			`Root     string`,
			`Roots    []string`,
			`Files    DevWatchMatchers`,
			`Dirs     DevWatchMatchers`,
			`Apps              map[string]any`,
			`Extra`,
			`ModuleReplaces`,
			`Observability`,
			`Cache`,
			`Events`,
			`Storage`,
			`func (c *ProjectConfig) UnmarshalYAML(`,
			`func migrateLegacyAppPrimitiveComponents(`,
			`func (c *DevConfig) SetApps(`,
		},
		filepath.Join("internal", "lighthouse", "project_config_patch.go.tmpl"): {
			`import "{{.GoModuleName}}/project"`,
			`*[]project.DevWatch`,
			`func applyDevConfigUpdate(`,
			`func mergeLighthouseDevWatches(`,
			`if _, scalar := existing.Watch.(string); scalar`,
		},
		filepath.Join("internal", "lighthouse", "server.go.tmpl"): {
			`"{{.GoModuleName}}/project"`,
			`Dev          *devConfigUpdate`,
			`Components   *project.Components`,
			`applyDevConfigUpdate(&current.Dev, *payload.Dev)`,
			`func loadProjectConfig() (*project.Config, error)`,
			`config.Render.ComponentContractVersion = project.CurrentComponentContractVersion`,
		},
	}
	for name, snippets := range files {
		content, err := os.ReadFile(filepath.Join(templatesRoot, name))
		if err != nil {
			t.Fatalf("read Lighthouse template %s: %v", name, err)
		}
		for _, snippet := range snippets {
			if !strings.Contains(string(content), snippet) {
				t.Fatalf("expected Lighthouse template %s to contain %q", name, snippet)
			}
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
			`name:"about" help:"Show environment and configured services for this app" goforj:"preboot"`,
			`type AboutCmd struct {`,
			`JSON`,
			`NoColor`,
			`aboutFGBrightWhite`,
			`aboutFGCyan`,
			`aboutFGGreen`,
			`aboutSectionMarker`,
			`aboutSectionMarker       = "♦"`,
			`aboutHealthyStatusMarker`,
			`func (c *AboutCmd) renderAboutSections(`,
			`func (c *AboutCmd) renderSectionHeader(`,
			`type aboutRenderContext struct {`,
			`func (c *AboutCmd) renderAboutSection(`,
			`func (c *AboutCmd) renderConnectionInventory(`,
			`func aboutConnectionInventoryRows(`,
			`func aboutComponentGridRows(`,
			`func aboutValueColor(`,
			`"{{.GoModuleName}}/internal/runtime"`,
			`func (c *AboutCmd) aboutService() *runtime.AboutService`,
		},
		filepath.Join(base, "about_cmd_test.go.tmpl"): {
			`func TestAboutConnectionInventoryRendersOneRowPerResource(`,
			`func TestAboutSectionRowsWrapLongValues(`,
			`func TestAboutSectionsShareVisualGrid(`,
			`func TestAboutFirstColumnUsesWhiteAcrossSections(`,
			`func TestAboutConnectionInventoryUsesRestrainedColorHierarchy(`,
			`report_processing  redis   report-processing`,
		},
		filepath.Join(base, "health_cmd.go.tmpl"): {
			`name:"health" help:"Query a live app readiness or liveness endpoint" goforj:"preboot"`,
			`type HealthCmd struct {`,
			`Probe`,
			`TimeoutMs`,
			`github.com/goforj/httpx`,
			`healthSectionMarker = "♦"`,
			`func (c *HealthCmd) probeURL() (string, error)`,
			`func (c *HealthCmd) renderHealthSectionHeader(`,
			`func healthReportRows(`,
			`func healthReportSections(`,
			`func computeHealthReportColumnWidths(`,
			`func healthSectionTitle(`,
			`return printJSON(map[string]any{`,
		},
		filepath.Join(base, "db_shell_cmd.go.tmpl"): {
			`name:"db:shell" aliases:"db" help:"Open a shell for a configured database connection" goforj:"preboot"`,
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
			`"{{.GoModuleName}}/internal/runtime"`,
		},
		filepath.Join(base, "cache_shell_cmd.go.tmpl"): {
			`name:"cache:shell" aliases:"cache" help:"Open a shell for a configured cache store" goforj:"preboot"`,
			`type CacheShellCmd struct {`,
			`RawArgs  []string`,
			`passthrough:""`,
			`Method   string`,
			`func (c *CacheShellCmd) applyInlineWrapperFlags() error`,
			`func (c *CacheShellCmd) parsedArgs() cacheShellParsedArgs`,
			`func (*CacheShellCmd) Help() string`,
			`forj cache sessions -- GET user:1`,
			`func NewCacheShellCmd() *CacheShellCmd`,
			`func (c *CacheShellCmd) resolveLaunch(store cacheShellStore)`,
			`exec.Command(launch.Command, launch.Args...)`,
			`"{{.GoModuleName}}/internal/runtime"`,
		},
		filepath.Join(base, "about_grid.go.tmpl"): {
			`func aboutTerminalWidth() int`,
			`term.GetSize`,
		},
		filepath.Join(base, "preboot.go.tmpl"): {
			`func DispatchPrebootCommand(args []string, root interface{}) (bool, error)`,
			`func rootHelpRequested(args []string) bool`,
			`func AppHelpName() string`,
			`os.Getenv("FORJ_MULTI_APP_HELP") == "1"`,
			`return name + " · " + appName`,
			`func printRootPrebootHelp(root interface{}) error`,
			`func findPrebootCommand(root interface{}, commandName string) interface{}`,
			`func findPrebootCommandValue(value reflect.Value, commandName string) interface{}`,
			`func prebootCommandMatches(command interface{}, commandName string) bool`,
			`marker != "preboot" && marker != "skip_boot"`,
			`func prebootCommandNameMatches(arg string, name string, aliases []string) bool`,
			`func applyStandalonePrebootSignature(node *kong.Node, command standaloneCommand)`,
		},
		filepath.Join(base, "preboot_test.go.tmpl"): {
			`func TestDispatchPrebootCommandHandlesRootHelpBeforeBoot(`,
			`func TestCommandHelpRequestedAllowsPositionalArgs(`,
			`args: []string{"Wow", "--help"}, want: true`,
			`args: []string{"Wow", "--", "--help"}, want: false`,
		},
		filepath.Join(base, "default_launch.go.tmpl"): {
			`func EffectiveLaunchArgs(args []string, hasRuntime bool) []string`,
			`if len(args) > 0 || !hasRuntime {`,
			`return []string{"run"}`,
		},
		filepath.Join(base, "default_launch_test.go.tmpl"): {
			`EffectiveLaunchArgs(nil, true)`,
			`EffectiveLaunchArgs(nil, false)`,
			`EffectiveLaunchArgs(args, true)`,
		},
		filepath.Join(base, "env_defaults.go.tmpl"): {
			`var CompiledEnvDefaultsBase64 string`,
			`var CompiledEnvOverridesBase64 string`,
			`func ApplyCompiledEnvDefaults() error`,
			`func ApplyCompiledEnvOverrides() error`,
			`base64.StdEncoding.DecodeString`,
			`return applyCompiledEnvMap(strings.TrimSpace(CompiledEnvOverridesBase64), true)`,
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

func TestWireInjectorTemplatesDeclareOwnership(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	base := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "wire")

	appOwned := []string{
		"inject_services_app.go.tmpl",
		"inject_cmd_app.go.tmpl",
		"inject_subscribers_app.go.tmpl",
		"inject_http_controllers_app.go.tmpl",
		"inject_jobs_app.go.tmpl",
		"inject_repositories_app.go.tmpl",
		"inject_schedules_app.go.tmpl",
	}
	for _, name := range appOwned {
		path := filepath.Join(base, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read template %s: %v", path, err)
		}
		source := string(content)
		if !strings.Contains(source, "// App-owned Wire injector. EDIT THIS FILE.") {
			t.Fatalf("expected %s to declare app-owned editability", path)
		}
		if strings.Contains(source, "DO NOT EDIT") {
			t.Fatalf("expected %s not to include DO NOT EDIT", path)
		}
	}

	frameworkOwned := []string{
		"inject_auth.go.tmpl",
		"inject_cmd.go.tmpl",
		"inject_db.go.tmpl",
		"inject_http.go.tmpl",
		"inject_jobs.go.tmpl",
		"inject_managers.go.tmpl",
		"inject_scheduler.go.tmpl",
	}
	for _, name := range frameworkOwned {
		path := filepath.Join(base, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read template %s: %v", path, err)
		}
		source := string(content)
		if !strings.Contains(source, "// Code generated by GoForj CLI. DO NOT EDIT.") {
			t.Fatalf("expected %s to declare framework ownership", path)
		}
	}

	wireHarnessPath := filepath.Join(base, "wire.go.tmpl")
	content, err := os.ReadFile(wireHarnessPath)
	if err != nil {
		t.Fatalf("read template %s: %v", wireHarnessPath, err)
	}
	source := string(content)
	for _, snippet := range []string{
		"// GoForj Wire harness. Edit this file when customizing root assembly.",
		"// Re-rendering can overwrite this file; review local changes before rendering over them.",
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected %s to contain %q", wireHarnessPath, snippet)
		}
	}
	if strings.Contains(source, "DO NOT EDIT") {
		t.Fatalf("expected %s not to include DO NOT EDIT", wireHarnessPath)
	}
}

func TestRepositoryInjectorTemplatesOnlyWireRepositories(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	root := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates")

	for _, path := range []string{
		filepath.Join(root, "wire", "inject_repositories_app.go.tmpl"),
		filepath.Join(root, "demo", "wire", "inject_repositories.go.tmpl"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read template %s: %v", path, err)
		}
		source := string(content)
		if strings.Contains(source, "NewRetentionService") || strings.Contains(source, "NewIncidentTransitionService") {
			t.Fatalf("expected %s to contain repository providers only", path)
		}
	}
}

func TestWireInjectorRendererOwnershipMatchesHeaders(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	root := filepath.Join(filepath.Dir(currentFile), "..", "..")
	rendererPath := filepath.Join(root, "internal", "forj", "project_renderer.go")
	content, err := os.ReadFile(rendererPath)
	if err != nil {
		t.Fatalf("read renderer: %v", err)
	}
	source := string(content)

	appOwned := []string{
		"wire/inject_cmd_app.go.tmpl",
		"wire/inject_http_controllers_app.go.tmpl",
		"wire/inject_jobs_app.go.tmpl",
		"wire/inject_repositories_app.go.tmpl",
		"wire/inject_schedules_app.go.tmpl",
		"wire/inject_services_app.go.tmpl",
		"wire/inject_subscribers_app.go.tmpl",
	}
	for _, tmpl := range appOwned {
		assertRendererTemplateOwnership(t, source, tmpl, true)
	}

	frameworkOwned := []string{
		"wire/app.go.tmpl",
		"wire/app_test.go.tmpl",
		"wire/inject_auth.go.tmpl",
		"wire/inject_cmd.go.tmpl",
		"wire/inject_db.go.tmpl",
		"wire/inject_http.go.tmpl",
		"wire/inject_jobs.go.tmpl",
		"wire/inject_managers.go.tmpl",
		"wire/inject_scheduler.go.tmpl",
		"wire/wire.go.tmpl",
	}
	for _, tmpl := range frameworkOwned {
		assertRendererTemplateOwnership(t, source, tmpl, false)
	}
}

// assertRendererTemplateOwnership verifies framework-owned templates are rerendered and app-owned templates are preserved.
func assertRendererTemplateOwnership(t *testing.T, source string, tmpl string, wantOnce bool) {
	t.Helper()
	searchFrom := 0
	found := false
	for {
		idx := strings.Index(source[searchFrom:], tmpl)
		if idx == -1 {
			break
		}
		found = true
		absoluteIdx := searchFrom + idx
		callName := nearestRendererWriteCall(source[:absoluteIdx])
		if wantOnce && !strings.Contains(callName, "Once") {
			t.Fatalf("expected %s to render once, got %s", tmpl, callName)
		}
		if !wantOnce && (callName == "" || strings.Contains(callName, "Once")) {
			t.Fatalf("expected %s to render overwrite, got %s", tmpl, callName)
		}
		searchFrom = absoluteIdx + len(tmpl)
	}
	if !found {
		t.Fatalf("expected renderer to map %s", tmpl)
	}
}

// nearestRendererWriteCall finds the render helper surrounding a template mapping assertion.
func nearestRendererWriteCall(prefix string) string {
	candidates := []string{
		"writeTemplateMappingsOnceForApp(",
		"writeTemplateMappingsForApp(",
		"writeTemplateMappingsOnce(",
		"writeTemplateMappings(",
	}
	lastName := ""
	lastIdx := -1
	for _, name := range candidates {
		idx := strings.LastIndex(prefix, name)
		if idx > lastIdx {
			lastIdx = idx
			lastName = strings.TrimSuffix(name, "(")
		}
	}
	frameworkMappingIdx := strings.LastIndex(prefix, "func (p *ProjectRenderer) appFrameworkMappings")
	appOwnedMappingIdx := strings.LastIndex(prefix, "func (p *ProjectRenderer) appOwnedMappings")
	if appOwnedMappingIdx > frameworkMappingIdx && appOwnedMappingIdx > lastIdx {
		return "writeTemplateMappingsOnceForApp"
	}
	if frameworkMappingIdx > lastIdx {
		return "writeTemplateMappingsForApp"
	}
	return lastName
}

func TestMakeControllerOpenHookTemplateIsWired(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	root := filepath.Join(filepath.Dir(currentFile), "..", "..")

	files := map[string][]string{
		filepath.Join(root, "templates", ".env.tmpl"): {
			`# Forj`,
			`FORJ_MAKE_OPEN=auto # options: auto, always, never`,
			`# Optional editor command for make commands; falls back to common GUI editors.`,
			`FORJ_EDITOR=`,
		},
		filepath.Join(root, "templates", "internal", "makecmd", "editor.go.tmpl"): {
			`Mode          string`,
			`EditorCommand string`,
			`func maybeOpenGeneratedFile(`,
			`func resolveGeneratedFileEditorCommand(`,
			`func generatedFileOpenProjectRoot(`,
			`"{project}"`,
			`func generatedFileTerminalEditorCandidates(`,
			`func generatedFileRunningProcessNames()`,
		},
		filepath.Join(root, "templates", "internal", "makecmd", "generator_helpers.go.tmpl"): {
			`func generatedPackageSegment(`,
			`func generatedPackagePathParts(`,
			`func generatedPackagePathPartsFromPath(`,
			`func generatedPackageName(`,
			`func generatedPackageRef(`,
			`func removeGeneratedFile(`,
			`func removeGoImportIfUnused(`,
			`Snake("_").ReplaceAll("_", "")`,
		},
		filepath.Join(root, "templates", "internal", "makecmd", "generator_helpers_test.go.tmpl"): {
			`func TestGeneratedPackageHelpersUseCompactLowercaseSegments(`,
		},
		filepath.Join(root, "templates", "internal", "makecmd", "make_command_cmd.go.tmpl"): {
			`func generatedCommandSignatureName(`,
			`return generatedCommandSignatureName(rawName)`,
			`func (c *CommandCmd) remove(`,
		},
		filepath.Join(root, "templates", "internal", "makecmd", "make_command_cmd_test.go.tmpl"): {
			`func TestCommandTargetUsesLowercaseSignatureName(`,
			`raw: "Wow", want: "wow"`,
			`func TestCommandCmdRemoveDeletesFileAndWiring(`,
		},
		filepath.Join(root, "templates", "internal", "makecmd", "make_migration_cmd_test.go.tmpl"): {
			`func TestMigrationCmdRemoveDeletesMatchingMigrationFiles(`,
		},
		filepath.Join(root, "templates", "internal", "makecmd", "make_model_cmd.go.tmpl"): {
			`generatedPackagePathPartsFromPath(pkg)`,
			`func (c *ModelCmd) remove(`,
		},
		filepath.Join(root, "templates", "internal", "makecmd", "make_controller_cmd.go.tmpl"): {
			`"Package":    generatedPackageName(filepath.Dir(path), "cmd"),`,
			`func (c *ControllerCmd) remove(`,
			`Open          bool   ` + "`short:\"o\" help:\"Open the generated controller in your editor.\"`",
			`NoOpen        bool   ` + "`name:\"no-open\" help:\"Do not open the generated controller, even when FORJ_MAKE_OPEN would.\"`",
			`MakeOpen      string ` + "`name:\"make-open\" env:\"FORJ_MAKE_OPEN\" default:\"auto\" hidden:\"\"`",
			`EditorCommand string ` + "`name:\"editor\" env:\"FORJ_EDITOR\" hidden:\"\"`",
			`validateGeneratedFileOpenFlags(c.Open, c.NoOpen)`,
			`maybeOpenGeneratedFile(generatedFileOpenOptions{`,
		},
		filepath.Join(root, "templates", "internal", "makecmd", "make_controller_cmd_test.go.tmpl"): {
			`func TestControllerCmdRemoveDeletesFileAndWiring(`,
		},
		filepath.Join(root, "templates", "internal", "makecmd", "make_subscriber_cmd_test.go.tmpl"): {
			`func TestSubscriberCmdRemoveDeletesFileAndWiring(`,
		},
		filepath.Join(root, "internal", "forj", "project_renderer.go"): {
			`"internal/makecmd/editor.go.tmpl"`,
			`"internal/makecmd/generator_helpers_test.go.tmpl"`,
			`"internal/makecmd/make_command_cmd_test.go.tmpl"`,
			`"internal/makecmd/make_controller_cmd_test.go.tmpl"`,
			`"internal/makecmd/make_migration_cmd_test.go.tmpl"`,
			`"internal/makecmd/make_subscriber_cmd_test.go.tmpl"`,
			`needsForjMakeOpen`,
			`needsForjEditor`,
		},
	}

	for path, snippets := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		source := string(content)
		for _, snippet := range snippets {
			if !strings.Contains(source, snippet) {
				t.Fatalf("expected %s to contain %q", path, snippet)
			}
		}
	}
}

func TestMakeFileGeneratorsExposeOpenHook(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	base := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "makecmd")

	for _, name := range []string{
		"make_command_cmd.go.tmpl",
		"make_controller_cmd.go.tmpl",
		"make_event_cmd.go.tmpl",
		"make_job_cmd.go.tmpl",
		"make_migration_cmd.go.tmpl",
		"make_model_cmd.go.tmpl",
		"make_schedule_cmd.go.tmpl",
		"make_subscriber_cmd.go.tmpl",
	} {
		path := filepath.Join(base, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		source := string(content)
		for _, snippet := range []string{
			`Open`,
			`bool`,
			`short:"o"`,
			`NoOpen`,
			`MakeOpen`,
			`env:"FORJ_MAKE_OPEN"`,
			`EditorCommand`,
			`env:"FORJ_EDITOR"`,
			`validateGeneratedFileOpenFlags(c.Open, c.NoOpen)`,
			`maybeOpenGeneratedFile(generatedFileOpenOptions{`,
		} {
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
		`runtime.NewRuntimeHost(runtimes...).Run(ctx)`,
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
			`ctx, stop := runtime.CLINotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)`,
			`parseCtx.BindTo(ctx, (*context.Context)(nil))`,
		},
		filepath.Join(root, "internal", "http", "server.go.tmpl"): {
			`router.Use(s.sourceContextMiddleware(runtime.SourceHTTP))`,
			`carrier.SetAppSourceName(sourceName)`,
		},
		filepath.Join(root, "internal", "schedules", "scheduler.go.tmpl"): {
			`WithTaskContextDecorator(func(ctx context.Context) context.Context {`,
			`return runtime.WithSource(ctx, runtime.SourceScheduler)`,
		},
		filepath.Join(root, "wire", "inject_services_app.go.tmpl"): {
			`setupCtx := runtime.BackgroundSourceContext(runtime.SourceStartup)`,
		},
		filepath.Join(root, "demo", "internal", "monitoring", "controller.go.tmpl"): {
			`startupCtx := runtime.BackgroundSourceContext(runtime.SourceStartup)`,
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
	templatePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "cmd", "app", "main.go.tmpl")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read main.go template: %v", err)
	}
	source := string(content)

	for _, snippet := range []string{
		`args := cmd.EffectiveLaunchArgs(os.Args[1:], {{.Components.HasRuntime}})`,
		`cmd.ApplyLaunchApp("{{.App.Name}}")`,
		`"{{.GoModuleName}}/{{.AppImportPath}}"`,
		`"{{.GoModuleName}}/{{.WireImportPath}}"`,
		`"{{.GoModuleName}}/internal/console"`,
		`if err := cmd.LoadEnv(); err != nil {`,
		`console.Fatalf("%v", err)`,
		`handled, err := cmd.DispatchPrebootCommand(args, &{{.AppPackageName}}.RootCmd{})`,
		`application, err := wire.InitializeApplication()`,
		`application.Run(nil, args)`,
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected main template to contain %q", snippet)
		}
	}
}

// TestDefaultLaunchTemplateDoesNotDependOnBuildState verifies launch identity cannot be changed through linker injection.
func TestDefaultLaunchTemplateDoesNotDependOnBuildState(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templatePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "cmd", "default_launch.go.tmpl")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read default launch template: %v", err)
	}
	if strings.Contains(string(content), "DefaultLaunchCommand") {
		t.Fatal("expected default launch template not to expose linker-populated command state")
	}
}

// TestMainTemplateRendersRuntimeCapabilityPerEntrypoint verifies mixed projects cannot leak one app's launch behavior into another.
func TestMainTemplateRendersRuntimeCapabilityPerEntrypoint(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templatePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "cmd", "app", "main.go.tmpl")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read main.go template: %v", err)
	}
	mainTemplate, err := template.New("main.go").Parse(string(content))
	if err != nil {
		t.Fatalf("parse main.go template: %v", err)
	}

	tests := []struct {
		name       string
		components project.Components
		want       string
	}{
		{name: "runtime app", components: project.Components{Jobs: true}, want: `cmd.EffectiveLaunchArgs(os.Args[1:], true)`},
		{name: "cli app", components: project.Components{CLI: true}, want: `cmd.EffectiveLaunchArgs(os.Args[1:], false)`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var rendered bytes.Buffer
			data := templateRenderConfig{
				Config:         &project.Config{GoModuleName: "example.com/test"},
				Components:     test.components,
				App:            project.DefaultApp(),
				AppPackageName: "app",
				AppImportPath:  "app",
				WireImportPath: "app/wire",
			}
			if err := mainTemplate.Execute(&rendered, data); err != nil {
				t.Fatalf("render main.go template: %v", err)
			}
			if !strings.Contains(rendered.String(), test.want) {
				t.Fatalf("expected rendered entrypoint to contain %q, got:\n%s", test.want, rendered.String())
			}
		})
	}
}

// TestRunCommandTemplatesUseSharedRuntimeCapability verifies command exposure cannot drift from default launch classification.
func TestRunCommandTemplatesUseSharedRuntimeCapability(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	root := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates")
	oldPredicate := `or .Components.WebAPI .Components.WebUI .Components.Scheduler .Components.Jobs`
	for _, relativePath := range []string{"app/root_cmd.go.tmpl", "wire/inject_cmd.go.tmpl"} {
		content, err := os.ReadFile(filepath.Join(root, relativePath))
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		source := string(content)
		if !strings.Contains(source, ".Components.HasRuntime") {
			t.Fatalf("expected %s to use Components.HasRuntime", relativePath)
		}
		if strings.Contains(source, oldPredicate) {
			t.Fatalf("expected %s not to duplicate the runtime component predicate", relativePath)
		}
	}
}

// TestCommandMetadataLivesInSignatures verifies generated Kong command fields do not duplicate help metadata owned by command signatures.
func TestCommandMetadataLivesInSignatures(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templatePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "app", "root_cmd.go.tmpl")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read root_cmd template: %v", err)
	}
	source := string(content)

	for _, snippet := range []string{
		`MakeCommandCmd    makecmd.CommandCmd    ` + "`cmd:\"\"`",
		`MakeControllerCmd makecmd.ControllerCmd ` + "`cmd:\"\"`",
		`MakeJobCmd       makecmd.JobCmd       ` + "`cmd:\"\"`",
		`MakeMigrationCmd makecmd.MigrationCmd ` + "`cmd:\"\"`",
		`MakeQueueCmd makecmd.QueueCmd ` + "`cmd:\"\"`",
		`MakeScheduleCmd makecmd.ScheduleCmd ` + "`cmd:\"\"`",
		`BenchmarkRunCmd    jobs.BenchmarkRunCmd    ` + "`cmd:\"\"`",
		`ExampleHelloJobCmd jobs.ExampleHelloJobCmd ` + "`cmd:\"\"`",
		`HttpServeCmd http.ServeCmd ` + "`cmd:\"\"`",
		`QueueWorkerCmd jobs.WorkerCmd ` + "`cmd:\"\"`",
		`RoutesListCmd http.RouteListCmd ` + "`cmd:\"\"`",
		`RunCmd cmd.RunCmd ` + "`cmd:\"\"`",
		`SchedulerCmd schedules.Cmd ` + "`cmd:\"\"`",
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected root command template to contain %q", snippet)
		}
	}

	eventTemplatePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "app", "event_commands.go.tmpl")
	eventContent, err := os.ReadFile(eventTemplatePath)
	if err != nil {
		t.Fatalf("read event command template: %v", err)
	}
	eventSource := string(eventContent)
	for _, snippet := range []string{
		`MakeEventCmd      makecmd.EventCmd      ` + "`cmd:\"\"`",
		`MakeSubscriberCmd makecmd.SubscriberCmd ` + "`cmd:\"\"`",
		`TestEventPipelineCmd cmd.TestEventPipelineCmd ` + "`cmd:\"\"`",
	} {
		if !strings.Contains(eventSource, snippet) {
			t.Fatalf("expected event command template to contain %q", snippet)
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
		filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "http", "routes_list_cmd.go.tmpl"): {
			`fmt.Printf("App: %s\n\n", routeListApp())`,
			`func routeListApp() string`,
		},
		filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "http", "swagger.go.tmpl"): {
			`func defaultOpenAPISpecPathForApp() string`,
			`func isSafeOpenAPIAppName(name string) bool`,
			`filepath.Join("build", app, "openapi.json")`,
		},
		filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "internal", "cmd", "app_identity.go.tmpl"): {
			`func ApplyLaunchApp(app string)`,
			`os.Setenv("FORJ_APP", app)`,
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

// TestSwaggerTemplatesRenderPinnedAndAppScoped guards the generated UI and serving contract at the renderer boundary.
func TestSwaggerTemplatesRenderPinnedAndAppScoped(t *testing.T) {
	root := t.TempDir()
	renderer := &ProjectRenderer{
		stats:     &renderStats{},
		workspace: currentProjectRenderWorkspace(t),
	}
	config := &project.Config{
		GoModuleName: "example.com/swaggerfixture",
		Render: project.RenderConfig{
			Components: project.Components{WebAPI: true},
		},
	}
	files := map[string][]string{
		"swagger.go": {
			`const scalarAPIReferenceURL = "https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.62.5"`,
			`Scalar.createApiReference('#app'`,
			`url: '/swagger/doc.json'`,
			`func defaultOpenAPISpecPathForApp() string`,
			`func isSafeOpenAPIAppName(name string) bool`,
			`filepath.Join("build", app, "openapi.json")`,
			`"error": "OpenAPI document is missing"`,
			`func apiIndexBuildCommandForApp() string`,
			`return "forj " + app + " build:api-index"`,
		},
		"swagger_test.go": {
			`func TestSwaggerUIAndSpecRoutes(`,
			`func TestSwaggerSpecUsesAppDefaultPath(`,
			`func TestSwaggerSpecPreservesPathOverride(`,
			`func TestSwaggerSpecMissingDoesNotFallBack(`,
			`func TestSwaggerSpecRejectsUnsafeImplicitAppPath(`,
			`/wrong-default`,
		},
	}

	for name, snippets := range files {
		templatePath := filepath.Join("internal", "http", name+".tmpl")
		destination := filepath.Join(root, name)
		if err := renderer.renderTemplateFile(destination, templatePath, config); err != nil {
			t.Fatalf("render %s: %v", templatePath, err)
		}
		content, err := os.ReadFile(destination)
		if err != nil {
			t.Fatalf("read rendered %s: %v", name, err)
		}
		source := string(content)
		for _, snippet := range snippets {
			if !strings.Contains(source, snippet) {
				t.Fatalf("expected rendered %s to contain %q:\n%s", name, snippet, source)
			}
		}
		if strings.Contains(source, `src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"`) {
			t.Fatalf("expected rendered %s not to use a floating Scalar CDN URL:\n%s", name, source)
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

func TestCommonRenderingIncludesCacheShellCommand(t *testing.T) {
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
	if !strings.Contains(source, `"internal/cmd/cache_shell_cmd.go.tmpl"`) {
		t.Fatal("expected common rendering to include cache shell command template")
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

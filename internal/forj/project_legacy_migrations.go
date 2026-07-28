package forj

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/goforj/str/v2"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

// cleanupLegacyGeneratedFiles removes obsolete framework-owned artifacts while preserving App-owned source.
func (p *ProjectRenderer) cleanupLegacyGeneratedFiles() error {
	discovery, err := projectlayout.Discover(p.workspace.discoveryRoot())
	if err != nil {
		return fmt.Errorf("discover Apps for legacy cleanup: %w", p.workspace.logicalError(err))
	}
	legacyPaths := []string{
		filepath.Join("internal", "cmd", "generate_all_cmd.go"),
		filepath.Join("internal", "cmd", "generate_cmd.go"),
		"main.go",
		filepath.Join("app", "providers.go"),
		filepath.Join("internal", "storage", "generate_cmd.go"),
		filepath.Join("internal", "database", "generate_cmd.go"),
		filepath.Join("internal", "database", "generate_cmd_test.go"),
		filepath.Join("internal", "lighthouse", "project_config.go"),
		filepath.Join("internal", "cmd", "demo_push_monitor_trigger_cmd.go"),
		filepath.Join("internal", "cmd", "monitor_seed_cmd.go"),
		filepath.Join("internal", "cmd", "monitor_reset_cmd.go"),
		filepath.Join("internal", "cmd", "monitor_retention_cmd.go"),
		filepath.Join("internal", "cmd", "monitor_poll_cmd.go"),
		filepath.Join("internal", "cmd", "push_monitor_trigger_cmd.go"),
		filepath.Join("internal", "cmd", "test_monitor_poll_loop_cmd.go"),
		filepath.Join("internal", "cmd", "lifecycle_hooks.go"),
		filepath.Join("internal", "cmd", "about_service.go"),
		filepath.Join("internal", "cmd", "standalone.go"),
		filepath.Join("internal", "cmd", "signatures.go"),
		filepath.Join("internal", "cmd", "app_commands.go"),
		filepath.Join("internal", "cmd", "root_cmd.go"),
		filepath.Join("internal", "cmd", "wire.go"),
		filepath.Join("internal", "cmd", "skip_boot.go"),
		filepath.Join("internal", "cmd", "skip_boot_test.go"),
		// Old generated files from before app terminology replaced the app-target model.
		filepath.Join("internal", "cmd", "target_identity.go"),
		filepath.Join("internal", "runtime", "targets.go"),
		filepath.Join("internal", "runtime", "targets_test.go"),
		filepath.Join("internal", "observability", "observers.go"),
		filepath.Join("internal", "observability", "observers_test.go"),
		filepath.Join("internal", "router", "routes_registry.go"),
		filepath.Join("internal", "http", "devconsole.go"),
		filepath.Join("internal", "http", "route.go"),
		filepath.Join("internal", "http", "middleware_non_200.go"),
		filepath.Join("internal", "http", "routes_list.go"),
		filepath.Join("internal", "http", "routes_list_test.go"),
		filepath.Join("internal", "lifecycle", "README.md"),
		filepath.Join("internal", "lifecycle", "manager.go"),
		filepath.Join("internal", "lifecycle", "manager_test.go"),
		filepath.Join("internal", "lifecycle", "settings.go"),
		filepath.Join("internal", "lifecycle", "lifecycle_registry.go"),
		filepath.Join("internal", "app", "README.md"),
		filepath.Join("internal", "app", "about.go"),
		filepath.Join("internal", "app", "discovery.go"),
		filepath.Join("internal", "app", "lifecycle.go"),
		filepath.Join("internal", "app", "lifecycle_registry.go"),
		filepath.Join("internal", "app", "lifecycle_test.go"),
		filepath.Join("internal", "app", "manager.go"),
		filepath.Join("internal", "app", "manager_test.go"),
		filepath.Join("internal", "app", "registry.go"),
		filepath.Join("internal", "app", "runtime.go"),
		filepath.Join("internal", "app", "runtime_host.go"),
		filepath.Join("internal", "app", "runtime_host_test.go"),
		filepath.Join("internal", "app", "source.go"),
		filepath.Join("internal", "app", "timeouts.go"),
		filepath.Join("internal", "scheduler", "devconsole.go"),
		filepath.Join("internal", "scheduler", "app_schedules.go"),
		filepath.Join("internal", "scheduler", "cmd.go"),
		filepath.Join("internal", "scheduler", "lighthouse.go"),
		filepath.Join("internal", "scheduler", "runtime.go"),
		filepath.Join("internal", "scheduler", "scheduler.go"),
		filepath.Join("internal", "scheduler", "scheduler_registry.go"),
		filepath.Join("internal", "schedules", "scheduler_registry.go"),
		filepath.Join("migrations", "2025_04_25_235625_new_user_table.up.sql"),
		filepath.Join("migrations", "2025_04_25_235625_new_user_table.down.sql"),
		filepath.Join("wire", "app.go"),
		filepath.Join("wire", "app_test.go"),
		filepath.Join("wire", "inject_app_services.go"),
		filepath.Join("wire", "inject_services_app.go"),
		filepath.Join("wire", "inject_auth.go"),
		filepath.Join("wire", "inject_cache.go"),
		filepath.Join("wire", "inject_cmd.go"),
		filepath.Join("wire", "inject_db.go"),
		filepath.Join("wire", "inject_http.go"),
		filepath.Join("wire", "inject_http_controllers.go"),
		filepath.Join("wire", "inject_controllers_app.go"),
		filepath.Join("wire", "inject_http_controllers_app.go"),
		filepath.Join("wire", "inject_jobs.go"),
		filepath.Join("wire", "inject_mail.go"),
		filepath.Join("wire", "inject_queue.go"),
		filepath.Join("wire", "inject_repositories.go"),
		filepath.Join("wire", "inject_repositories_app.go"),
		filepath.Join("wire", "inject_scheduler.go"),
		filepath.Join("wire", "inject_scheduler_schedules.go"),
		filepath.Join("wire", "inject_schedules_app.go"),
		filepath.Join("wire", "inject_storage.go"),
		filepath.Join("wire", "wire.go"),
		filepath.Join("wire", "wire_gen.go"),
		filepath.Join("app", "wire", "inject_app_services.go"),
		filepath.Join("app", "wire", "inject_cache.go"),
		filepath.Join("app", "wire", "inject_events.go"),
		filepath.Join("app", "wire", "inject_http_controllers.go"),
		filepath.Join("app", "wire", "inject_inspects.go"),
		filepath.Join("app", "wire", "inject_mail.go"),
		filepath.Join("app", "wire", "inject_queue.go"),
		filepath.Join("app", "wire", "inject_repositories.go"),
		filepath.Join("app", "wire", "inject_scheduler_schedules.go"),
		filepath.Join("app", "wire", "inject_storage.go"),
	}
	legacyPaths = append(legacyPaths, legacyJobsFrameworkPaths()...)
	for _, path := range legacyPaths {
		if _, err := p.workspace.removeFileIfExists(path); err != nil {
			return err
		}
	}
	if err := p.workspace.removeTree("internal", "devconsole"); err != nil {
		return err
	}
	if err := p.workspace.removeTree("internal", "lifecycle"); err != nil {
		return err
	}
	if _, err := p.workspace.removeEmptyDir("wire"); err != nil {
		return err
	}
	if err := p.syncLegacyGeneratedTemplates(); err != nil {
		return err
	}

	for _, app := range discovery.ConventionalApps() {
		components := appRenderComponents(p.config, app)
		for _, path := range appOwnedWirePathsForApp(app) {
			if data, err := p.workspace.readFile(path); err == nil {
				updated := syncAppOwnedWireSetNames(string(data))
				switch filepath.Base(path) {
				case "inject_jobs_app.go":
					updated = syncDemoAppJobInjector(updated, p.config.GoModuleName, components)
					updated, err = syncLegacyJobHandlerRegistration(updated, p.config.GoModuleName, components)
					if err != nil {
						return fmt.Errorf("migrate %s: %w", path, err)
					}
				case "inject_repositories_app.go":
					updated = syncDemoAppRepositoryInjector(updated, p.config.GoModuleName, components)
				case "inject_services_app.go":
					updated = syncLegacyAppServiceInjector(updated, p.config.GoModuleName, filepath.ToSlash(projectlayout.AppDir(".", app)))
					updated = syncDemoAppServiceInjector(updated, p.config.GoModuleName, components)
				case "inject_schedules_app.go":
					updated = syncLegacyScheduleInjector(updated, p.config.GoModuleName, filepath.ToSlash(projectlayout.AppDir(".", app)))
				}
				if updated != string(data) {
					formatted, err := format.Source([]byte(updated))
					if err != nil {
						return fmt.Errorf("gofmt %s: %w", path, err)
					}
					if err := p.workspace.writeFile(path, formatted, 0o644); err != nil {
						return err
					}
				}
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}

	appServiceInjectorPath := filepath.Join("app", "wire", "inject_services_app.go")
	if data, err := p.workspace.readFile(appServiceInjectorPath); err == nil {
		updated := syncLegacyAppServiceInjector(string(data), p.config.GoModuleName, "app")
		if updated != string(data) {
			formatted, err := format.Source([]byte(updated))
			if err != nil {
				return fmt.Errorf("gofmt %s: %w", appServiceInjectorPath, err)
			}
			if err := p.workspace.writeFile(appServiceInjectorPath, formatted, 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	appLifecyclePath := filepath.Join("app", "lifecycle.go")
	if data, err := p.workspace.readFile(appLifecyclePath); err == nil {
		updated := syncLegacyAppLifecycleRegistry(string(data), p.config.GoModuleName)
		if updated != string(data) {
			formatted, err := format.Source([]byte(updated))
			if err != nil {
				return fmt.Errorf("gofmt %s: %w", appLifecyclePath, err)
			}
			if err := p.workspace.writeFile(appLifecyclePath, formatted, 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	scheduleInjectorPath := filepath.Join("app", "wire", "inject_schedules_app.go")
	if data, err := p.workspace.readFile(scheduleInjectorPath); err == nil {
		updated := syncLegacyScheduleInjector(string(data), p.config.GoModuleName, "app")
		if updated != string(data) {
			formatted, err := format.Source([]byte(updated))
			if err != nil {
				return fmt.Errorf("gofmt %s: %w", scheduleInjectorPath, err)
			}
			if err := p.workspace.writeFile(scheduleInjectorPath, formatted, 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Migrate legacy scheduler command names in preserved app-owned schedules.
	appSchedulesPath := filepath.Join("app", "schedules.go")
	if data, err := p.workspace.readFile(appSchedulesPath); err == nil {
		updated := str.Of(syncLegacyScheduleInjectorPackage(string(data))).
			ReplaceAll("demo:push-monitor-trigger", "monitor:push-test-trigger").
			ReplaceAll("push-monitor-trigger", "monitor:push-test-trigger").
			String()
		if updated != string(data) {
			formatted, err := format.Source([]byte(updated))
			if err != nil {
				return fmt.Errorf("gofmt %s: %w", appSchedulesPath, err)
			}
			if err := p.workspace.writeFile(appSchedulesPath, formatted, 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	defaultHealthEnabled := p.config.Render.Components.WebAPI || p.config.Render.Components.WebUI
	projectComponents := project.ProjectComponents(p.config)
	projectHealthEnabled := projectComponents.WebAPI || projectComponents.WebUI
	appCommandsPath := filepath.Join("app", "commands.go")
	if data, err := p.workspace.readFile(appCommandsPath); err == nil {
		updated := syncCommandsName(string(data))
		updated = syncHealthCommands(updated, defaultHealthEnabled)
		if updated != string(data) {
			formatted, err := format.Source([]byte(updated))
			if err != nil {
				return fmt.Errorf("gofmt %s: %w", appCommandsPath, err)
			}
			if err := p.workspace.writeFile(appCommandsPath, formatted, 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	cmdWirePath := filepath.Join("app", "wire", "inject_cmd.go")
	if data, err := p.workspace.readFile(cmdWirePath); err == nil {
		updated := syncHealthCommandWire(string(data), defaultHealthEnabled)
		if updated != string(data) {
			if err := p.workspace.writeFile(cmdWirePath, []byte(updated), 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	prebootPath := filepath.Join("internal", "cmd", "preboot.go")
	if data, err := p.workspace.readFile(prebootPath); err == nil {
		updated := syncHealthPreboot(string(data), projectHealthEnabled)
		if updated != string(data) {
			if err := p.workspace.writeFile(prebootPath, []byte(updated), 0o644); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if projectHealthEnabled {
		if err := p.renderTemplateFile(filepath.Join("internal", "cmd", "health_cmd.go"), "internal/cmd/health_cmd.go.tmpl", p.config); err != nil {
			return err
		}
		if err := p.renderTemplateFile(filepath.Join("internal", "cmd", "health_cmd_test.go"), "internal/cmd/health_cmd_test.go.tmpl", p.config); err != nil {
			return err
		}
	} else {
		if _, err := p.workspace.removeFileIfExists("internal", "cmd", "health_cmd.go"); err != nil {
			return err
		}
		if _, err := p.workspace.removeFileIfExists("internal", "cmd", "health_cmd_test.go"); err != nil {
			return err
		}
	}
	return nil
}

// syncLegacyGeneratedTemplates refreshes only framework-owned templates whose historical shapes are unambiguous.
func (p *ProjectRenderer) syncLegacyGeneratedTemplates() error {
	type templateSync struct {
		dest     string
		tmpl     string
		matches  []string
		requires []string
	}

	if err := p.workspace.validateLegacyAppServiceInjectorOwnership(project.DefaultApp()); err != nil {
		return err
	}

	syncs := []templateSync{
		{
			dest: "internal/lighthouse/server.go",
			tmpl: "internal/lighthouse/server.go.tmpl",
			matches: []string{
				`"/project"`,
				"project.DevConfig",
				"*DevConfig",
				"func loadProjectConfig() (*Config, error)",
				"var config Config",
				`group.GET("/*"`,
			},
			requires: []string{
				`"/auth/dev-session"`,
				`group.GET("/*"`,
				p.config.GoModuleName + "/project",
			},
		},
		{
			dest: filepath.Join("internal", "jobs", "worker.go"),
			tmpl: "internal/jobs/worker.go.tmpl",
			matches: []string{
				"internal/alerts",
				"internal/monitoring",
				"hello *ExampleHelloJob,",
				"monitorCheck *monitoring.MonitorCheckJob,",
				"alertDispatch *alerts.DispatchJob,",
			},
		},
	}

	for _, sync := range syncs {
		data, err := p.workspace.readFile(sync.dest)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		content := string(data)
		needsRewrite := false
		for _, match := range sync.matches {
			if strings.Contains(content, match) {
				needsRewrite = true
				break
			}
		}
		if !needsRewrite {
			for _, required := range sync.requires {
				if !strings.Contains(content, required) {
					needsRewrite = true
					break
				}
			}
		}
		if !needsRewrite {
			continue
		}
		if err := p.renderTemplateFile(sync.dest, sync.tmpl, p.config); err != nil {
			return err
		}
	}

	if err := p.renderTemplateIfMissing(
		filepath.Join("project", "config.go"),
		"project/config.go.tmpl",
		p.config,
	); err != nil {
		return err
	}
	if err := p.renderTemplateIfMissing(
		filepath.Join("internal", "lighthouse", "project_config_patch.go"),
		"internal/lighthouse/project_config_patch.go.tmpl",
		p.config,
	); err != nil {
		return err
	}
	if _, err := p.workspace.removeFileIfExists("internal", "lighthouse", "project_config.go"); err != nil {
		return err
	}

	return nil
}

// syncAppOwnedWireSetNames updates preserved app-owned injectors to the current set naming contract.
func syncAppOwnedWireSetNames(content string) string {
	return str.Of(content).
		ReplaceAll("cmdAppSet", "appCommandSet").
		ReplaceAll("httpAppControllerSet", "appHttpControllerSet").
		ReplaceAll("appControllerSet", "appHttpControllerSet").
		ReplaceAll("httpControllerSet provides all HTTP route controllers.", "appHttpControllerSet provides all HTTP route controllers.").
		ReplaceAll("jobAppSet", "appJobSet").
		ReplaceAll("schedulerScheduleSet", "appScheduleSet").
		String()
}

// syncDemoAppJobInjector repairs early app job injectors that omitted demo job providers.
func syncDemoAppJobInjector(content string, moduleName string, components project.Components) string {
	if !components.DemoApp || !components.Jobs || strings.TrimSpace(moduleName) == "" {
		return content
	}
	updated := content
	updated = ensureGoImport(updated, moduleName+"/internal/alerts", "")
	updated = ensureGoImport(updated, moduleName+"/internal/monitoring", "")
	for _, provider := range []string{
		"\talerts.NewDispatchJob,",
		"\tmonitoring.NewCheckService,",
		"\tmonitoring.NewMonitorCheckJob,",
	} {
		if !strings.Contains(updated, strings.TrimSpace(provider)) {
			updated = insertIntoWireSet(updated, "appJobSet", provider)
		}
	}
	return updated
}

// syncDemoAppRepositoryInjector repairs early app repository injectors that omitted demo repositories.
func syncDemoAppRepositoryInjector(content string, moduleName string, components project.Components) string {
	if !components.DemoApp || !components.HasDatabase() || strings.TrimSpace(moduleName) == "" {
		return content
	}
	updated := content
	for _, importPath := range []string{
		moduleName + "/internal/appsettings",
		moduleName + "/internal/models",
		moduleName + "/internal/notification",
	} {
		updated = ensureGoImport(updated, importPath, "")
	}
	if strings.Contains(updated, "wire.Value(repositorySetPlaceholder{})") {
		updated = strings.Replace(updated, "\twire.Value(repositorySetPlaceholder{}),\n", "", 1)
	}
	for _, provider := range []string{
		"\tappsettings.NewAppSettingRepo,",
		"\tmodels.NewAlertDispatchEventRepo,",
		"\tmodels.NewIncidentRepo,",
		"\tmodels.NewMonitorCheckRepo,",
		"\tmodels.NewMonitorCheckRollupRepo,",
		"\tmodels.NewMonitorRepo,",
		"\tnotification.NewChannelRepo,",
	} {
		if !strings.Contains(updated, strings.TrimSpace(provider)) {
			updated = insertIntoWireSet(updated, "repositorySet", provider)
		}
	}
	return updated
}

// syncDemoAppServiceInjector repairs early app service injectors that omitted demo providers.
func syncDemoAppServiceInjector(content string, moduleName string, components project.Components) string {
	if !components.DemoApp || !components.HasDatabase() || strings.TrimSpace(moduleName) == "" {
		return content
	}
	updated := content
	for _, importPath := range []string{
		moduleName + "/internal/appsettings",
		moduleName + "/internal/logger",
		moduleName + "/internal/monitoring",
		moduleName + "/internal/notification",
	} {
		updated = ensureGoImport(updated, importPath, "")
	}
	for _, provider := range []string{
		"\tmonitoring.NewIncidentTransitionService,",
		"\tmonitoring.NewRetentionService,",
		"\tnotification.NewManager,",
		"\tpreseedDemoDefaults,",
	} {
		if !strings.Contains(updated, strings.TrimSpace(provider)) {
			updated = insertIntoWireSet(updated, "appSet", provider)
		}
	}
	if !strings.Contains(updated, "type demoPreseedReady struct{}") {
		updated = strings.TrimRight(updated, "\n") + demoPreseedReadyBlock() + "\n"
	}
	return updated
}

// demoPreseedReadyBlock restores demo preseed wiring for apps rendered before the provider split.
func demoPreseedReadyBlock() string {
	return `

type demoPreseedReady struct{}

func preseedDemoDefaults(
	logger *logger.AppLogger,
	settingRepo *appsettings.AppSettingRepo,
	channelRepo *notification.ChannelRepo,
) (*demoPreseedReady, error) {
	setupCtx := runtime.BackgroundSourceContext(runtime.SourceStartup)
	if inserted, err := settingRepo.PreseedDefaults(setupCtx); err != nil {
		return nil, err
	} else if inserted > 0 {
		logger.Info().Int("inserted", inserted).Msg("app settings preseeded")
	}
	if inserted, err := channelRepo.PreseedDefaults(setupCtx); err != nil {
		return nil, err
	} else if inserted > 0 {
		logger.Info().Int("inserted", inserted).Msg("notification channels preseeded")
	}
	return &demoPreseedReady{}, nil
}
`
}

// syncLegacyScheduleInjectorPackage updates preserved schedule wiring after the scheduler package rename.
func syncLegacyScheduleInjectorPackage(content string) string {
	return str.Of(content).
		ReplaceAll("/internal/scheduler", "/internal/schedules").
		ReplaceAll("scheduler.AppSchedules", "schedules.AppSchedules").
		ReplaceAll("scheduler.NewAppSchedules", "schedules.NewAppSchedules").
		ReplaceAll("scheduler.ScheduleRegistry", "schedules.ScheduleRegistry").
		ReplaceAll("scheduler.RegisterRecurring", "schedules.RegisterRecurring").
		ReplaceAll("*scheduler.Scheduler", "*schedules.Scheduler").
		String()
}

// syncLegacyScheduleInjector updates preserved app schedule wiring after schedule registration moved to app/.
func syncLegacyScheduleInjector(content string, moduleName string, appImportPath string) string {
	moduleName = strings.TrimSpace(moduleName)
	if moduleName == "" {
		return syncLegacyScheduleInjectorPackage(content)
	}

	updated := syncLegacyScheduleInjectorPackage(content)
	appImportPath = strings.Trim(filepath.ToSlash(strings.TrimSpace(appImportPath)), "/")
	if appImportPath == "" {
		appImportPath = "app"
	}
	schedulesPath := moduleName + "/internal/schedules"
	compositionAppPath := moduleName + "/" + appImportPath
	appPackageName := project.AppPackageName(filepath.Base(appImportPath))
	updated = ensureGoImport(updated, schedulesPath, "")
	updated = replaceGoImportPath(updated, compositionAppPath, compositionAppPath, "")
	// targetapp was emitted by early multi-app renders; keep this as a legacy migration shim only.
	updated = replaceQualifiedIdentifier(updated, "targetapp.NewScheduleRegistry", appPackageName+".NewScheduleRegistry")
	updated = replaceQualifiedIdentifier(updated, "targetapp.ScheduleRegistry", appPackageName+".ScheduleRegistry")
	updated = replaceQualifiedIdentifier(updated, "compositionapp.NewScheduleRegistry", appPackageName+".NewScheduleRegistry")
	updated = replaceQualifiedIdentifier(updated, "compositionapp.ScheduleRegistry", appPackageName+".ScheduleRegistry")
	updated = ensureGoImport(updated, compositionAppPath, "")
	updated = strings.ReplaceAll(updated, "\tschedules.NewAppSchedules,", "\tProvideAppSchedules,")

	if !strings.Contains(updated, "func ProvideAppSchedules(") {
		updated = appendProvideAppSchedules(updated)
	}
	if !strings.Contains(updated, appPackageName+".NewScheduleRegistry") {
		updated = insertIntoWireSet(updated, "appScheduleSet", "\t"+appPackageName+".NewScheduleRegistry,")
	}
	scheduleBinding := "wire.Bind(new(schedules.ScheduleRegistry), new(*" + appPackageName + ".ScheduleRegistry))"
	if !strings.Contains(updated, scheduleBinding) {
		updated = insertIntoWireSet(updated, "appScheduleSet", "\t"+scheduleBinding+",")
	}
	return dedupeWireSetProviders(updated, "appScheduleSet")
}

// appendProvideAppSchedules adds the explicit zero-arg provider Wire needs for an empty schedule collection.
func appendProvideAppSchedules(content string) string {
	content = strings.TrimRight(content, "\n")
	return content + `

// ProvideAppSchedules collects application-owned schedules.
func ProvideAppSchedules() *schedules.AppSchedules {
	return schedules.NewAppSchedules()
}
`
}

// syncLegacyAppServiceInjector updates preserved app service wiring after lifecycle moved to app/.
func syncLegacyAppServiceInjector(content string, moduleName string, appImportPath string) string {
	moduleName = strings.TrimSpace(moduleName)
	if moduleName == "" {
		return content
	}

	updated := content
	updated = replaceQualifiedIdentifier(updated, "app.NewTimeouts", "runtime.NewTimeouts")
	updated = replaceQualifiedIdentifier(updated, "app.BackgroundSourceContext", "runtime.BackgroundSourceContext")
	updated = replaceQualifiedIdentifier(updated, "app.SourceStartup", "runtime.SourceStartup")
	updated = replaceQualifiedIdentifier(updated, "runtimeapp.NewTimeouts", "runtime.NewTimeouts")
	updated = replaceQualifiedIdentifier(updated, "runtimeapp.BackgroundSourceContext", "runtime.BackgroundSourceContext")
	updated = replaceQualifiedIdentifier(updated, "runtimeapp.SourceStartup", "runtime.SourceStartup")

	legacyRuntimePath := moduleName + "/internal/app"
	runtimePath := moduleName + "/internal/runtime"
	appImportPath = strings.Trim(filepath.ToSlash(strings.TrimSpace(appImportPath)), "/")
	if appImportPath == "" {
		appImportPath = "app"
	}
	compositionAppPath := moduleName + "/" + appImportPath
	appPackageName := project.AppPackageName(filepath.Base(appImportPath))
	updated = replaceQualifiedIdentifier(updated, "app.NewLifecycleRegistry", appPackageName+".NewLifecycleRegistry")
	// targetapp was emitted by early multi-app renders; keep this as a legacy migration shim only.
	updated = replaceQualifiedIdentifier(updated, "targetapp.NewLifecycleRegistry", appPackageName+".NewLifecycleRegistry")
	updated = replaceQualifiedIdentifier(updated, "compositionapp.NewLifecycleRegistry", appPackageName+".NewLifecycleRegistry")
	updated = replaceGoImportPath(updated, legacyRuntimePath, runtimePath, "")
	updated = replaceGoImportPath(updated, runtimePath, runtimePath, "")
	updated = replaceGoImportPath(updated, compositionAppPath, compositionAppPath, "")
	updated = ensureGoImport(updated, compositionAppPath, "")
	updated = removeFrameworkMetricsProviderFromAppServiceInjector(updated, moduleName)
	updated = removeFrameworkEventCommandProvidersFromAppServiceInjector(updated, moduleName)
	return updated
}

// removeFrameworkMetricsProviderFromAppServiceInjector moves metrics manager ownership back to framework-managed wiring.
func removeFrameworkMetricsProviderFromAppServiceInjector(content string, moduleName string) string {
	return str.Of(content).
		ReplaceFirst("\tmetrics.NewManager,\n", "").
		ReplaceFirst("\t"+strconv.Quote(moduleName+"/internal/metrics")+"\n", "").
		String()
}

// removeFrameworkEventCommandProvidersFromAppServiceInjector leaves owner services intact while command wiring returns to the framework set.
func removeFrameworkEventCommandProvidersFromAppServiceInjector(content string, moduleName string) string {
	providerPattern := regexp.MustCompile(`(?m)^[ \t]*makecmd\.New(?:Event|Subscriber)Cmd,[ \t\r]*\n`)
	updated := providerPattern.ReplaceAllString(content, "")
	if updated == content || sourceUsesPackageSelector(updated, "makecmd") {
		return updated
	}
	importPattern := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(strconv.Quote(strings.TrimSpace(moduleName)+"/internal/makecmd")) + `[ \t\r]*\n`)
	return importPattern.ReplaceAllString(updated, "")
}

// syncLegacyAppLifecycleRegistry updates preserved app lifecycle registration imports after the runtime package rename.
func syncLegacyAppLifecycleRegistry(content string, moduleName string) string {
	moduleName = strings.TrimSpace(moduleName)
	if moduleName == "" {
		return content
	}

	updated := content
	for _, name := range []string{
		"Lifecycle",
		"BeforeStartup",
		"Startup",
		"AfterStartup",
		"BeforeShutdown",
		"Shutdown",
		"AfterShutdown",
	} {
		updated = replaceQualifiedIdentifier(updated, "runtimeapp."+name, "runtime."+name)
	}

	legacyRuntimePath := moduleName + "/internal/app"
	runtimePath := moduleName + "/internal/runtime"
	updated = replaceGoImportPath(updated, legacyRuntimePath, runtimePath, "")
	updated = replaceGoImportPath(updated, runtimePath, runtimePath, "")
	return updated
}

// replaceQualifiedIdentifier rewrites an old qualifier without touching longer aliases.
func replaceQualifiedIdentifier(content string, old string, replacement string) string {
	re := regexp.MustCompile(`(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(old))
	return re.ReplaceAllString(content, `${1}`+replacement)
}

// replaceGoImportPath rewrites an existing import to a new path and optional alias.
func replaceGoImportPath(content string, oldPath string, newPath string, alias string) string {
	oldQuotedPath := strconv.Quote(oldPath)
	newQuotedPath := strconv.Quote(newPath)
	replacement := "\t" + newQuotedPath
	if alias != "" {
		replacement = "\t" + alias + " " + newQuotedPath
	}
	replacements := map[string]string{
		"\t" + oldQuotedPath:                replacement,
		"\tapp " + oldQuotedPath:            replacement,
		"\tcompositionapp " + oldQuotedPath: replacement,
		"\truntimeapp " + oldQuotedPath:     replacement,
		"\truntime " + oldQuotedPath:        replacement,
		// targetapp is legacy-only compatibility for preserved render-once files.
		"\ttargetapp " + oldQuotedPath: replacement,
	}
	for from, to := range replacements {
		content = strings.ReplaceAll(content, from, to)
	}
	return content
}

// ensureGoImport preserves the existing declaration shape while adding one migration dependency.
func ensureGoImport(content string, importPath string, alias string) string {
	if strings.Contains(content, strconv.Quote(importPath)) {
		if alias != "" {
			return replaceGoImportPath(content, importPath, importPath, alias)
		}
		return content
	}
	importSpec := strconv.Quote(importPath)
	if alias != "" {
		importSpec = alias + " " + importSpec
	}
	importStart := strings.Index(content, "import (\n")
	if importStart == -1 {
		singleImport := regexp.MustCompile(`(?m)^import[ \t]+([^\r\n]+)$`)
		if match := singleImport.FindStringSubmatchIndex(content); match != nil {
			existingSpec := strings.TrimSpace(content[match[2]:match[3]])
			block := "import (\n\t" + importSpec + "\n\t" + existingSpec + "\n)"
			return content[:match[0]] + block + content[match[1]:]
		}
		packageDeclaration := regexp.MustCompile(`(?m)^package[ \t]+[A-Za-z_][A-Za-z0-9_]*[ \t]*\r?\n`)
		match := packageDeclaration.FindStringIndex(content)
		if match == nil {
			return content
		}
		return content[:match[1]] + "\nimport " + importSpec + "\n" + content[match[1]:]
	}
	insertAt := importStart + len("import (\n")
	importLine := "\t" + importSpec + "\n"
	return content[:insertAt] + importLine + content[insertAt:]
}

// insertIntoWireSet inserts a provider before the named wire set closes.
func insertIntoWireSet(content string, setName string, provider string) string {
	lines := strings.Split(content, "\n")
	inSet := false
	for i, line := range lines {
		if !inSet && strings.Contains(line, "var "+setName+" = wire.NewSet(") {
			inSet = true
			continue
		}
		if inSet && strings.TrimSpace(line) == ")" {
			lines = append(lines[:i], append([]string{provider}, lines[i:]...)...)
			return strings.Join(lines, "\n")
		}
	}
	return content
}

// dedupeWireSetProviders removes duplicate provider entries left by older compatibility migrations.
func dedupeWireSetProviders(content string, setName string) string {
	lines := strings.Split(content, "\n")
	inSet := false
	seen := map[string]bool{}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if !inSet && strings.Contains(line, "var "+setName+" = wire.NewSet(") {
			inSet = true
			out = append(out, line)
			continue
		}
		if inSet {
			trimmed := strings.TrimSpace(line)
			if trimmed == ")" {
				inSet = false
				out = append(out, line)
				continue
			}
			if trimmed != "" {
				if seen[trimmed] {
					continue
				}
				seen[trimmed] = true
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// syncCommandsName migrates preserved app command registration away from the legacy AppCommands name.
func syncCommandsName(content string) string {
	return str.Of(content).
		ReplaceAll("// AppCommands wires application-specific commands into the CLI.", "// Commands wires application-specific commands into the CLI.").
		ReplaceAll("type AppCommands struct {", "type Commands struct {").
		ReplaceAll("// NewAppCommands creates a new AppCommands instance with the given commands.", "// NewCommands creates a new Commands instance with the given commands.").
		ReplaceAll("func NewAppCommands(", "func NewCommands(").
		ReplaceAll(") *AppCommands {", ") *Commands {").
		ReplaceAll("return &AppCommands{", "return &Commands{").
		String()
}

// syncHealthCommandWire keeps preserved command providers aligned with HTTP component availability.
func syncHealthCommandWire(content string, enabled bool) string {
	const healthLine = "\tcmd.NewHealthCmd,\n"
	if !enabled {
		return strings.Replace(content, healthLine, "", 1)
	}
	if strings.Contains(content, healthLine) {
		return content
	}
	anchor := "\tcmd.NewAboutCmd,\n"
	if strings.Contains(content, anchor) {
		return strings.Replace(content, anchor, anchor+healthLine, 1)
	}
	return content
}

// syncHealthPreboot keeps preserved preboot command dispatch aligned with HTTP component availability.
func syncHealthPreboot(content string, enabled bool) string {
	const healthLine = "\tfunc() interface{} { return NewHealthCmd() },\n"
	if !enabled {
		return strings.Replace(content, healthLine, "", 1)
	}
	if strings.Contains(content, healthLine) {
		return content
	}
	anchor := "\tfunc() interface{} { return NewAboutCmd() },\n"
	if strings.Contains(content, anchor) {
		return strings.Replace(content, anchor, anchor+healthLine, 1)
	}
	return content
}

// syncHealthCommands keeps preserved command registration aligned with HTTP component availability.
func syncHealthCommands(content string, enabled bool) string {
	const fieldLine = "\tHealthCmd cmd.HealthCmd `cmd:\"\"`\n"
	const paramLine = "\thealthCmd *cmd.HealthCmd,\n"
	const assignLine = "\t\tHealthCmd: *healthCmd,\n"
	if !enabled {
		return removeLinesContaining(content, "HealthCmd", "healthCmd")
	}
	updated := content
	if !strings.Contains(updated, "HealthCmd") {
		updated = insertAfterLineContaining(updated, "AboutCmd", fieldLine)
	}
	if !strings.Contains(updated, "healthCmd") {
		updated = insertAfterLineContaining(updated, "aboutCmd", paramLine)
	}
	if !strings.Contains(updated, "HealthCmd:") {
		updated = insertAfterLineContaining(updated, "AboutCmd:", assignLine)
	}
	return updated
}

// insertAfterLineContaining performs narrow render-once migrations without reformatting whole files.
func insertAfterLineContaining(content string, fragment string, insertedLine string) string {
	lines := strings.SplitAfter(content, "\n")
	for i, line := range lines {
		if !strings.Contains(line, fragment) {
			continue
		}
		inserted := []string{insertedLine}
		lines = append(lines[:i+1], append(inserted, lines[i+1:]...)...)
		return strings.Join(lines, "")
	}
	return content
}

// removeLinesContaining removes component-specific lines from preserved files when a component is disabled.
func removeLinesContaining(content string, fragments ...string) string {
	var builder strings.Builder
	for _, line := range strings.SplitAfter(content, "\n") {
		remove := false
		for _, fragment := range fragments {
			if strings.Contains(line, fragment) {
				remove = true
				break
			}
		}
		if !remove {
			builder.WriteString(line)
		}
	}
	return builder.String()
}

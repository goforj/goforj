package rendercheck

import (
	"fmt"
	"strings"

	"github.com/goforj/goforj/project"
)

// renderCombo describes a single component combination to render.
type renderCombo struct {
	id                  string
	components          project.Components
	starterKit          project.StarterKit
	apps                map[string]project.AppConfig
	enabled             []string
	legacyConfig        bool
	validateIdempotence bool
}

// featureCombo captures toggles for non-database components.
type featureCombo struct {
	auth      bool
	webAPI    bool
	webUI     bool
	scheduler bool
	jobs      bool
}

// featureID returns a stable, readable id for the feature set.
func featureID(feature featureCombo) string {
	parts := []string{"base"}
	if feature.auth {
		parts = append(parts, "auth")
	}
	if feature.webAPI {
		parts = append(parts, "webapi")
	}
	if feature.webUI {
		parts = append(parts, "webui")
	}
	if feature.scheduler {
		parts = append(parts, "scheduler")
	}
	if feature.jobs {
		parts = append(parts, "jobs")
	}
	return strings.Join(parts, "_")
}

const (
	renderProfileSmoke = "smoke"
	renderProfilePR    = "pr"
	renderProfileFull  = "full"
)

// selectedRenderProfile resolves legacy flags while rejecting profile typos that could silently reduce render coverage.
func selectedRenderProfile(profile string, full bool) (string, error) {
	if full {
		return renderProfileFull, nil
	}
	trimmed := strings.TrimSpace(profile)
	switch trimmed {
	case renderProfileSmoke, renderProfilePR, renderProfileFull:
		return trimmed, nil
	case "":
		return renderProfilePR, nil
	default:
		return "", fmt.Errorf("unknown render profile %q; valid profiles: %s, %s, %s", trimmed, renderProfileSmoke, renderProfilePR, renderProfileFull)
	}
}

// buildRenderCombos builds the render matrix for the run.
func buildRenderCombos(profile string) []renderCombo {
	switch profile {
	case renderProfileSmoke:
		return buildSmokeRenderCombos()
	case renderProfileFull:
		return buildFullRenderCombos()
	default:
		return buildCuratedRenderCombos()
	}
}

// buildFullRenderCombos returns the full component matrix.
func buildFullRenderCombos() []renderCombo {
	const numCombos = 1 << 8
	combos := make([]renderCombo, 0, numCombos)
	for i := 0; i < numCombos; i++ {
		cfg := project.Components{
			CLI:              true,
			Docker:           true,
			Auth:             i&(1<<0) != 0,
			WebAPI:           i&(1<<1) != 0,
			WebUI:            i&(1<<2) != 0,
			DatabaseMySQL:    i&(1<<3) != 0,
			DatabasePostgres: i&(1<<4) != 0,
			DatabaseSQLite:   i&(1<<5) != 0,
			Scheduler:        i&(1<<6) != 0,
			Jobs:             i&(1<<7) != 0,
		}
		cfg.ResolveDependencies()

		if cfg.DatabaseSQLite {
			cfg.DatabaseMySQL = false
			cfg.DatabasePostgres = false
		}
		if cfg.DatabasePostgres {
			cfg.DatabaseMySQL = false
		}
		if err := cfg.ValidateRenderContract(); err != nil {
			continue
		}

		combos = append(combos, renderCombo{
			id:         fmt.Sprintf("%v", i),
			components: cfg,
			starterKit: project.StarterKitNone,
			enabled:    componentLabels(cfg),
		})
	}
	combos = append(combos, prSentinelRenderCombos()...)
	return combos
}

// buildSmokeRenderCombos returns a small set that exercises the major render surfaces.
func buildSmokeRenderCombos() []renderCombo {
	cases := []struct {
		id  string
		cfg project.Components
	}{
		{id: "base", cfg: project.Components{CLI: true, Docker: true}},
		{id: "webapi", cfg: project.Components{CLI: true, Docker: true, WebAPI: true}},
		{id: "webui", cfg: project.Components{CLI: true, Docker: true, WebUI: true}},
		{id: "mysql", cfg: project.Components{CLI: true, Docker: true, DatabaseMySQL: true}},
		{id: "auth_mysql", cfg: project.Components{CLI: true, Docker: true, Auth: true, WebAPI: true, DatabaseMySQL: true}},
		{id: "jobs", cfg: project.Components{CLI: true, Docker: true, Jobs: true}},
		{id: "scheduler_jobs", cfg: project.Components{CLI: true, Docker: true, Scheduler: true, Jobs: true}},
		{id: "sqlite_webapi", cfg: project.Components{CLI: true, Docker: true, WebAPI: true, DatabaseSQLite: true}},
	}

	combos := make([]renderCombo, 0, len(cases))
	for _, tc := range cases {
		cfg := tc.cfg
		cfg.ResolveDependencies()
		if err := cfg.ValidateRenderContract(); err != nil {
			continue
		}
		combos = append(combos, renderCombo{
			id:         tc.id,
			components: cfg,
			starterKit: project.StarterKitNone,
			enabled:    componentLabels(cfg),
		})
	}
	combos = append(combos, starterKitRenderCombos()...)
	return combos
}

// prSentinelRenderCombos returns high-signal combos that cover cross-cutting render surfaces.
func prSentinelRenderCombos() []renderCombo {
	cases := []struct {
		id  string
		cfg project.Components
	}{
		{
			id:  "sentinel_recommended_default",
			cfg: project.DefaultSelectedComponents(),
		},
		{
			id: "sentinel_primitives_all_on",
			cfg: project.Components{
				CLI: true, Docker: true, Cache: true, Events: true, Storage: true, Jobs: true,
			},
		},
		{
			id:  "sentinel_cache_only",
			cfg: project.Components{CLI: true, Docker: true, Cache: true},
		},
		{
			id:  "sentinel_events_only",
			cfg: project.Components{CLI: true, Docker: true, Events: true},
		},
		{
			id:  "sentinel_storage_only",
			cfg: project.Components{CLI: true, Docker: true, Storage: true},
		},
		{
			id: "sentinel_web_metrics_grafana_without_primitives",
			cfg: project.Components{
				CLI: true, WebAPI: true, Metrics: true, Observability: true, Grafana: true, Docker: true,
			},
		},
		{
			id: "sentinel_max_mysql",
			cfg: project.Components{
				CLI: true, DemoApp: true, Mail: true, Auth: true, OAuth: true, WebAPI: true, WebUI: true,
				Metrics: true, Observability: true, Grafana: true, Docker: true, DatabaseMySQL: true,
				Scheduler: true, Cache: true, Events: true, Storage: true, Jobs: true,
			},
		},
		{
			id: "sentinel_max_postgres",
			cfg: project.Components{
				CLI: true, DemoApp: true, Mail: true, Auth: true, OAuth: true, WebAPI: true, WebUI: true,
				Metrics: true, Observability: true, Grafana: true, Docker: true, DatabasePostgres: true,
				Scheduler: true, Cache: true, Events: true, Storage: true, Jobs: true,
			},
		},
		{
			id: "sentinel_sqlite_webapi_jobs",
			cfg: project.Components{
				CLI: true, WebAPI: true, Metrics: true, Docker: true, DatabaseSQLite: true, Jobs: true,
			},
		},
		{
			id: "sentinel_auth_scheduler_jobs",
			cfg: project.Components{
				CLI: true, Mail: true, Auth: true, OAuth: true, WebAPI: true, Metrics: true, Docker: true,
				DatabaseMySQL: true, Scheduler: true, Jobs: true,
			},
		},
		{
			id: "sentinel_observability_grafana",
			cfg: project.Components{
				CLI: true, WebAPI: true, Metrics: true, Observability: true, Grafana: true, Docker: true,
				Scheduler: true, Jobs: true,
			},
		},
	}

	combos := make([]renderCombo, 0, len(cases))
	for _, tc := range cases {
		cfg := tc.cfg
		cfg.ResolveDependencies()
		if err := cfg.ValidateRenderContract(); err != nil {
			continue
		}
		combos = append(combos, renderCombo{
			id:                  tc.id,
			components:          cfg,
			starterKit:          project.StarterKitNone,
			enabled:             componentLabels(cfg),
			validateIdempotence: tc.id == "sentinel_primitives_all_on" || tc.id == "sentinel_web_metrics_grafana_without_primitives",
		})
	}
	combos = append(combos, legacyComponentMappingRenderCombo())

	mixedDefault := project.Components{
		CLI: true, WebAPI: true, Metrics: true, Observability: true, Grafana: true, Docker: true,
	}
	mixedDefault.ResolveDependencies()
	for _, sentinel := range []struct {
		id      string
		appName string
		key     project.ComponentKey
	}{
		{id: "sentinel_named_app_storage_only", appName: "storage-worker", key: project.ComponentStorage},
		{id: "sentinel_named_app_mail_only", appName: "mailer", key: project.ComponentMail},
		{id: "sentinel_named_app_database_only", appName: "database-worker", key: project.ComponentDatabaseSQLite},
		{id: "sentinel_named_app_auth_only", appName: "auth-api", key: project.ComponentAuth},
		{id: "sentinel_named_app_scheduler_only", appName: "scheduler-worker", key: project.ComponentScheduler},
	} {
		combos = append(combos, namedComponentRenderCombo(mixedDefault, sentinel.id, sentinel.appName, sentinel.key))
	}
	mixedEvents := project.Components{CLI: true, Events: true}
	mixedEvents.ResolveDependencies()
	combos = append(combos, renderCombo{
		id:         "sentinel_named_app_events_only",
		components: mixedDefault,
		starterKit: project.StarterKitNone,
		apps: map[string]project.AppConfig{
			"events-worker": {Components: mixedEvents},
		},
		enabled: append(componentLabels(mixedDefault), "App:events-worker(Events)"),
	})
	mixedCache := project.Components{CLI: true, Cache: true}
	mixedCache.ResolveDependencies()
	combos = append(combos, renderCombo{
		id:         "sentinel_named_app_cache_only",
		components: mixedDefault,
		starterKit: project.StarterKitNone,
		apps: map[string]project.AppConfig{
			"cache-worker": {Components: mixedCache},
		},
		enabled: append(componentLabels(mixedDefault), "App:cache-worker(Cache)"),
	})
	defaultEvents := project.Components{CLI: true, WebAPI: true, Metrics: true, Events: true, Docker: true}
	defaultEvents.ResolveDependencies()
	namedWithoutEvents := project.Components{CLI: true, WebAPI: true, Metrics: true}
	namedWithoutEvents.ResolveDependencies()
	combos = append(combos, renderCombo{
		id:         "sentinel_default_events_named_app_off",
		components: defaultEvents,
		starterKit: project.StarterKitNone,
		apps: map[string]project.AppConfig{
			"api": {Components: namedWithoutEvents},
		},
		enabled: append(componentLabels(defaultEvents), "App:api(WebAPI,Metrics;Events-off)"),
	})
	namedJobsDefault := project.Components{CLI: true, WebAPI: true, DatabaseSQLite: true, Storage: true}
	namedJobsDefault.ResolveDependencies()
	namedJobsWorker := project.Components{CLI: true, Jobs: true}
	namedJobsWorker.ResolveDependencies()
	namedJobsMetricsAPI := project.Components{CLI: true, WebAPI: true, Metrics: true}
	namedJobsMetricsAPI.ResolveDependencies()
	combos = append(combos, renderCombo{
		id:         "sentinel_named_app_jobs_only",
		components: namedJobsDefault,
		starterKit: project.StarterKitNone,
		apps: map[string]project.AppConfig{
			"metrics-api": {Components: namedJobsMetricsAPI},
			"worker":      {Components: namedJobsWorker},
		},
		enabled: append(componentLabels(namedJobsDefault), "App:metrics-api(WebAPI,Metrics;Jobs-off)", "App:worker(Jobs)"),
	})
	defaultJobs := project.Components{
		CLI: true, WebAPI: true, Metrics: true, DatabaseSQLite: true, Cache: true, Jobs: true,
	}
	defaultJobs.ResolveDependencies()
	namedJobsOff := project.Components{CLI: true, WebAPI: true, Metrics: true, DatabaseSQLite: true, Cache: true}
	namedJobsOff.ResolveDependencies()
	combos = append(combos, renderCombo{
		id:         "sentinel_default_jobs_named_app_off",
		components: defaultJobs,
		starterKit: project.StarterKitNone,
		apps: map[string]project.AppConfig{
			"api": {Components: namedJobsOff},
		},
		enabled: append(componentLabels(defaultJobs), "App:api(WebAPI,Metrics,SQLite,Cache;Jobs-off)"),
	})
	return combos
}

// legacyComponentMappingRenderCombo proves historical omission defaults migrate at both App scopes without widening later runs.
func legacyComponentMappingRenderCombo() renderCombo {
	defaultComponents := project.Components{
		CLI: true, WebAPI: true, Docker: true, Cache: true, Events: true, Storage: true,
	}
	defaultComponents.ResolveDependencies()
	workerComponents := project.Components{
		CLI: true, Cache: true, Events: true, Storage: true, Jobs: true,
	}
	workerComponents.ResolveDependencies()
	return renderCombo{
		id:         "sentinel_legacy_component_mappings",
		components: defaultComponents,
		starterKit: project.StarterKitNone,
		apps: map[string]project.AppConfig{
			"worker": {Components: workerComponents},
		},
		enabled: append(
			componentLabels(defaultComponents),
			"App:worker("+strings.Join(componentLabels(workerComponents), ",")+";legacy-mapping)",
		),
		legacyConfig:        true,
		validateIdempotence: true,
	}
}

// namedComponentRenderCombo keeps the default App lean while compiling one component through a named App.
func namedComponentRenderCombo(defaultComponents project.Components, id string, appName string, key project.ComponentKey) renderCombo {
	namedComponents := project.Components{CLI: true}
	namedComponents.SetEnabled(key, true)
	namedComponents = project.NormalizeAppComponents(defaultComponents, namedComponents)
	return renderCombo{
		id:         id,
		components: defaultComponents,
		starterKit: project.StarterKitNone,
		apps: map[string]project.AppConfig{
			appName: {Components: namedComponents},
		},
		enabled: append(componentLabels(defaultComponents), "App:"+appName+"("+strings.Join(componentLabels(namedComponents), ",")+")"),
	}
}

// starterKitRenderCombos keeps frontend integrations represented without multiplying the core component matrix.
func starterKitRenderCombos() []renderCombo {
	cfg := project.Components{CLI: true, Docker: true, Auth: true, WebAPI: true, WebUI: true, DatabaseSQLite: true}
	cfg.ResolveDependencies()
	return []renderCombo{
		{
			id:         "starter_react_auth_sqlite",
			components: cfg,
			starterKit: project.StarterKitReact,
			enabled:    append(componentLabels(cfg), "StarterKit:React"),
		},
		{
			id:         "starter_templ_htmx_auth_sqlite",
			components: cfg,
			starterKit: project.StarterKitTemplHTMX,
			enabled:    append(componentLabels(cfg), "StarterKit:templ_htmx"),
		},
	}
}

// buildCuratedRenderCombos returns a curated pairwise set of combos.
func buildCuratedRenderCombos() []renderCombo {
	features := []featureCombo{
		{},
		{auth: true},
		{webAPI: true},
		{webUI: true},
		{scheduler: true},
		{jobs: true},
		{auth: true, webAPI: true},
		{auth: true, webUI: true},
		{webAPI: true, webUI: true},
		{webAPI: true, scheduler: true},
		{webAPI: true, jobs: true},
		{webUI: true, scheduler: true},
		{webUI: true, jobs: true},
		{scheduler: true, jobs: true},
	}

	dbVariants := []struct {
		name  string
		apply func(*project.Components)
	}{
		{name: "mysql", apply: func(c *project.Components) { c.DatabaseMySQL = true }},
		{name: "postgres", apply: func(c *project.Components) { c.DatabasePostgres = true }},
		{name: "sqlite", apply: func(c *project.Components) { c.DatabaseSQLite = true }},
	}

	var combos []renderCombo
	for _, feature := range features {
		cfg := project.Components{
			CLI:       true,
			Docker:    true,
			Auth:      feature.auth,
			WebAPI:    feature.webAPI,
			WebUI:     feature.webUI,
			Scheduler: feature.scheduler,
			Jobs:      feature.jobs,
		}
		cfg.ResolveDependencies()
		if err := cfg.ValidateRenderContract(); err != nil {
			continue
		}
		combos = append(combos, renderCombo{
			id:         featureID(feature),
			components: cfg,
			starterKit: project.StarterKitNone,
			enabled:    componentLabels(cfg),
		})
	}

	for _, variant := range dbVariants {
		for idx, feature := range features {
			cfg := project.Components{
				CLI:       true,
				Docker:    true,
				Auth:      feature.auth,
				WebAPI:    feature.webAPI,
				WebUI:     feature.webUI,
				Scheduler: feature.scheduler,
				Jobs:      feature.jobs,
			}
			variant.apply(&cfg)
			cfg.ResolveDependencies()
			if err := cfg.ValidateRenderContract(); err != nil {
				continue
			}
			combos = append(combos, renderCombo{
				id:         fmt.Sprintf("%s_%02d", variant.name, idx),
				components: cfg,
				starterKit: project.StarterKitNone,
				enabled:    componentLabels(cfg),
			})
		}
	}

	combos = append(combos, prSentinelRenderCombos()...)
	combos = append(combos, starterKitRenderCombos()...)
	return combos
}

// componentLabels returns the human-friendly component labels for logging.
func componentLabels(cfg project.Components) []string {
	enabled := make([]string, 0)
	for _, definition := range project.ComponentCatalog() {
		if cfg.Enabled(definition.Key) {
			enabled = append(enabled, definition.Label)
		}
	}
	return enabled
}

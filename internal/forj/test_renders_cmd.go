package forj

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
)

func formatCommandFailure(command string, err error, stdout, stderr string) error {
	var parts []string
	parts = append(parts, fmt.Sprintf("%s: %v", command, err))
	if trimmed := strings.TrimSpace(stdout); trimmed != "" {
		parts = append(parts, fmt.Sprintf("stdout:\n%s", trimmed))
	}
	if trimmed := strings.TrimSpace(stderr); trimmed != "" {
		parts = append(parts, fmt.Sprintf("stderr:\n%s", trimmed))
	}
	return fmt.Errorf("%s", strings.Join(parts, "\n\n"))
}

// TestRendersCmd exercises generated project render combinations.
type TestRendersCmd struct {
	logger *logger.AppLogger

	// Profile selects the render coverage strategy.
	Profile string `help:"Render profile to run" enum:"smoke,pr,full" default:"pr"`

	// Full runs the exhaustive core matrix plus sentinel combinations. Prefer --profile=full for new usage.
	Full bool `help:"Run the full component matrix"`

	// RunTests executes rendered Go test packages after render/build validation.
	RunTests bool `help:"Run rendered Go test packages after render/build" short:"t"`

	// List prints the selected combinations without rendering them.
	List bool `help:"List selected render combinations without running them"`
}

// Signature exposes the render matrix command for framework validation.
func (*TestRendersCmd) Signature() string {
	return `name:"test:renders" help:"Runs all combinations of project configurations to test rendering" hidden:""`
}

type stepTimer struct {
	start time.Time
	parts map[string]time.Duration
}

func newStepTimer() *stepTimer {
	return &stepTimer{
		start: time.Now(),
		parts: make(map[string]time.Duration),
	}
}

func (t *stepTimer) Track(name string, fn func() error) error {
	s := time.Now()
	err := fn()
	t.parts[name] = time.Since(s)
	return err
}

func (t *stepTimer) Report(label string) {
	total := time.Since(t.start)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n⏱ Timing breakdown for %s\n", label))
	b.WriteString("---------------------------------------------------\n")

	for k, v := range t.parts {
		b.WriteString(fmt.Sprintf("%-15s %s\n", k+":", v))
	}

	b.WriteString("---------------------------------------------------\n")
	b.WriteString(fmt.Sprintf("%-15s %s\n\n", "total:", total))

	fmt.Print(b.String())
}

// NewTestRendersCmd creates a render matrix command.
func NewTestRendersCmd(logger *logger.AppLogger) *TestRendersCmd {
	return &TestRendersCmd{logger: logger}
}

// Run executes the selected render matrix profile.
func (cmd *TestRendersCmd) Run() error {
	profile := selectedRenderProfile(cmd.Profile, cmd.Full)
	combos := buildRenderCombos(profile)
	totalCombos := len(combos)
	combos, shardLabel, err := shardRenderCombos(combos)
	if err != nil {
		return err
	}
	if cmd.List {
		listRenderCombos(profile, combos, shardLabel)
		return nil
	}
	console.Infof("Testing %d component combinations with %s profile%s", len(combos), profile, shardLabel)
	if len(combos) == 0 {
		console.Warnf("No render combinations selected%s", shardLabel)
		return nil
	}

	modCache, buildCache := testkit.GoCachePaths()
	workspaceRoot := testRenderWorkspaceRoot()
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		return fmt.Errorf("create test render workspace: %w", err)
	}

	workerCount := testRenderWorkerCount()
	console.Infof("Render workers: %d", workerCount)
	forjExec, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}

	jobs := make(chan renderCombo)
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		workerRoot := filepath.Join(workspaceRoot, fmt.Sprintf("worker-%02d", i))
		go func(root string) {
			defer wg.Done()
			for combo := range jobs {
				_ = os.RemoveAll(root)
				if err := os.MkdirAll(root, 0755); err != nil {
					cmd.fail("workspace dir failed", combo.id, nil, err)
					continue
				}

				if err := initModule(root, modCache, buildCache); err != nil {
					_ = os.RemoveAll(root)
					cmd.fail("go mod init failed", combo.id, nil, err)
					continue
				}

				cmd.runCombo(root, modCache, buildCache, forjExec, combo)
				_ = os.RemoveAll(root)
			}
		}(workerRoot)
	}

	for _, combo := range combos {
		jobs <- combo
	}
	close(jobs)
	wg.Wait()
	console.Successf("Rendered %d combinations%s", len(combos), shardLabel)
	if totalCombos != len(combos) {
		console.Infof("Shard completed %d/%d combinations", len(combos), totalCombos)
	}
	return nil
}

// listRenderCombos prints the selected render combinations in a review-friendly table.
func listRenderCombos(profile string, combos []renderCombo, shardLabel string) {
	fmt.Printf("profile: %s\n", profile)
	fmt.Printf("combinations: %d%s\n\n", len(combos), shardLabel)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tComponents")
	for _, combo := range combos {
		fmt.Fprintf(w, "%s\t%s\n", combo.id, strings.Join(combo.enabled, ", "))
	}
	_ = w.Flush()
}

func shardRenderCombos(combos []renderCombo) ([]renderCombo, string, error) {
	count := 1
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_SHARD_COUNT")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, "", fmt.Errorf("invalid FORJ_TEST_RENDERS_SHARD_COUNT=%q (must be integer >= 1)", v)
		}
		count = n
	}
	if count == 1 {
		return combos, "", nil
	}

	index := 0
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_SHARD_INDEX")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, "", fmt.Errorf("invalid FORJ_TEST_RENDERS_SHARD_INDEX=%q (must be integer >= 0)", v)
		}
		index = n
	}
	if index >= count {
		return nil, "", fmt.Errorf(
			"invalid shard config: FORJ_TEST_RENDERS_SHARD_INDEX=%d must be < FORJ_TEST_RENDERS_SHARD_COUNT=%d",
			index,
			count,
		)
	}

	filtered := make([]renderCombo, 0, len(combos)/count+1)
	for i, combo := range combos {
		if i%count == index {
			filtered = append(filtered, combo)
		}
	}
	label := fmt.Sprintf(" (shard %d/%d · total %d)", index+1, count, len(combos))
	return filtered, label, nil
}

func testRenderWorkspaceRoot() string {
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_DIR")); v != "" {
		return v
	}
	return filepath.Join(os.TempDir(), "forj_test_renders")
}

func testRenderWorkerCount() int {
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_WORKERS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	workerCount := runtime.NumCPU()
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > 12 {
		workerCount = 12
	}
	return workerCount
}

// renderCombo describes a single component combination to render.
type renderCombo struct {
	id         string
	components project.Components
	starterKit project.StarterKit
	apps       map[string]project.AppConfig
	enabled    []string
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

// selectedRenderProfile resolves legacy flags and the named render profile.
func selectedRenderProfile(profile string, full bool) string {
	if full {
		return renderProfileFull
	}
	trimmed := strings.TrimSpace(profile)
	switch trimmed {
	case renderProfileSmoke, renderProfilePR, renderProfileFull:
		return trimmed
	case "":
		return renderProfilePR
	default:
		return renderProfilePR
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
			id:  "sentinel_primitives_all_off",
			cfg: project.Components{CLI: true, Docker: true},
		},
		{
			id: "sentinel_primitives_all_on",
			cfg: project.Components{
				CLI: true, Docker: true, Cache: true, Events: true, Storage: true, Jobs: true,
			},
		},
		{
			id:  "sentinel_events_only",
			cfg: project.Components{CLI: true, Docker: true, Events: true},
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
			id:         tc.id,
			components: cfg,
			starterKit: project.StarterKitNone,
			enabled:    componentLabels(cfg),
		})
	}

	mixedDefault := project.Components{
		CLI: true, WebAPI: true, Metrics: true, Observability: true, Grafana: true, Docker: true,
	}
	mixedDefault.ResolveDependencies()
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
	enabled := []string{"CLI", "Docker"}
	if cfg.DemoApp {
		enabled = append(enabled, "Demo App")
	}
	if cfg.Mail {
		enabled = append(enabled, "Mail")
	}
	if cfg.Auth {
		enabled = append(enabled, "Auth")
	}
	if cfg.OAuth {
		enabled = append(enabled, "OAuth")
	}
	if cfg.WebAPI {
		enabled = append(enabled, "WebAPI")
	}
	if cfg.WebUI {
		enabled = append(enabled, "WebUI")
	}
	if cfg.Metrics {
		enabled = append(enabled, "Metrics")
	}
	if cfg.Observability {
		enabled = append(enabled, "Observability")
	}
	if cfg.Grafana {
		enabled = append(enabled, "Grafana")
	}
	if cfg.DatabaseMySQL {
		enabled = append(enabled, "Database (MySQL)")
	}
	if cfg.DatabasePostgres {
		enabled = append(enabled, "Database (Postgres)")
	}
	if cfg.DatabaseSQLite {
		enabled = append(enabled, "Database (SQLite)")
	}
	if cfg.Scheduler {
		enabled = append(enabled, "Scheduler")
	}
	if cfg.Cache {
		enabled = append(enabled, "Cache")
	}
	if cfg.Events {
		enabled = append(enabled, "Events")
	}
	if cfg.Storage {
		enabled = append(enabled, "Storage")
	}
	if cfg.Jobs {
		enabled = append(enabled, "Jobs")
	}
	return enabled
}

// initModule creates the go.mod once for the shared render directory.
func initModule(dir, modCache, buildCache string) error {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return nil
	}
	goMod := exec.Command("go", "mod", "init", "github.com/test/project")
	goMod.Dir = dir
	goMod.Env = append(os.Environ(),
		"GOMODCACHE="+modCache,
		"GOCACHE="+buildCache,
	)
	if err := goMod.Run(); err != nil {
		return err
	}
	return nil
}

// runCombo renders and validates a single combo using a shared directory.
func (cmd *TestRendersCmd) runCombo(dir, modCache, buildCache, forjExec string, combo renderCombo) {
	comboID := combo.id
	apps, err := renderComboApps(combo)
	if err != nil {
		cmd.fail("invalid configured App", comboID, nil, err)
		return
	}
	cfg := project.Config{
		ProjectName:  fmt.Sprintf("TestProject%s", comboID),
		GoModuleName: "github.com/test/project",
		UpdatedAt:    time.Now().Format(time.RFC3339),
		Dev:          project.DevConfig{},
		Apps:         combo.apps,
		Render: project.RenderConfig{
			Components: combo.components,
			StarterKit: combo.starterKit,
		},
	}
	if repoRoot, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			cfg.Render.ModuleReplaces = map[string]string{"github.com/goforj/goforj": repoRoot}
		}
	}

	console.Actionf("Rendering components %s", strings.Join(combo.enabled, ", "))

	timer := newStepTimer()
	ymlPath := filepath.Join(dir, ".goforj.yml")

	if err := timer.Track("write_yaml", func() error {
		return WriteYAML(ymlPath, cfg)
	}); err != nil {
		cmd.fail("failed to write config", comboID, &cfg, err)
		return
	}
	if err := timer.Track("seed_apps", func() error {
		return seedRenderComboApps(dir, apps)
	}); err != nil {
		cmd.fail("failed to seed configured Apps", comboID, &cfg, err)
		return
	}

	if err := timer.Track("forj_render", func() error {
		render := exec.Command(forjExec, "render")
		render.Dir = dir
		render.Env = append(os.Environ(),
			"GOMODCACHE="+modCache,
			"GOCACHE="+buildCache,
		)

		var stdout, stderr strings.Builder
		render.Stdout = &stdout
		render.Stderr = &stderr

		if err := render.Run(); err != nil {
			return formatCommandFailure("forj render", err, stdout.String(), stderr.String())
		}
		if renderDebugEnabled() {
			if stdout.Len() > 0 {
				fmt.Printf("%s\n", strings.TrimSpace(stdout.String()))
			}
			if stderr.Len() > 0 {
				fmt.Printf("%s\n", strings.TrimSpace(stderr.String()))
			}
		}
		return nil
	}); err != nil {
		cmd.fail("render failed", comboID, &cfg, err)
		return
	}

	if combo.starterKit == project.StarterKitTemplHTMX {
		if err := timer.Track("templ_generate", func() error {
			templCmd := exec.Command("go", "run", "github.com/a-h/templ/cmd/templ@v0.3.1020", "generate")
			templCmd.Dir = dir
			templCmd.Env = append(os.Environ(),
				"GOMODCACHE="+modCache,
				"GOCACHE="+buildCache,
			)
			output, err := templCmd.CombinedOutput()
			if err != nil {
				return formatCommandFailure("templ generate", err, string(output), "")
			}
			return nil
		}); err != nil {
			cmd.fail("templ generate failed", comboID, &cfg, err)
			return
		}
	}

	if err := timer.Track("wire_gen", func() error {
		for _, app := range apps {
			wireCmd := exec.Command("wire")
			wireCmd.Dir = filepath.Join(dir, app.WireDir)
			wireCmd.Env = append(os.Environ(),
				"GOMODCACHE="+modCache,
				"GOCACHE="+buildCache,
			)
			if output, err := wireCmd.CombinedOutput(); err != nil {
				return formatCommandFailure("wire generate "+app.Name, err, string(output), "")
			}
		}
		return nil
	}); err != nil {
		cmd.fail("wire generate failed", comboID, &cfg, err)
		return
	}

	if err := timer.Track("go_build", func() error {
		binDir := filepath.Join(dir, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return fmt.Errorf("create bin dir: %w", err)
		}
		for _, app := range apps {
			args := []string{"build"}
			if renderBuildTraceEnabled() {
				args = append(args, "-x")
			}
			target := "./" + filepath.ToSlash(filepath.Dir(app.Entrypoint))
			args = append(args, "-o", filepath.Join(binDir, app.Name), target)
			build := exec.Command("go", args...)
			build.Dir = dir
			build.Env = append(os.Environ(),
				"GOMODCACHE="+modCache,
				"GOCACHE="+buildCache,
			)
			output, err := build.CombinedOutput()
			if err != nil {
				return formatCommandFailure("go build "+app.Name, err, string(output), "")
			}
			if renderBuildTraceEnabled() {
				console.Infof("go build trace for combo %s App %s:\n%s", comboID, app.Name, strings.TrimSpace(string(output)))
			}
		}
		return nil
	}); err != nil {
		cmd.fail("go build failed", comboID, &cfg, err)
		return
	}

	if cmd.RunTests {
		if err := timer.Track("go_test", func() error {
			return runRenderedGoTests(dir, modCache, buildCache)
		}); err != nil {
			cmd.fail("go test failed", comboID, &cfg, err)
			return
		}
	}

	timer.Report(fmt.Sprintf("combo %s (%s)", comboID, strings.Join(combo.enabled, ", ")))

	console.Successf("Passed")
}

// renderComboApps returns every executable projection a render combo must compile.
func renderComboApps(combo renderCombo) ([]project.App, error) {
	names := make([]string, 0, len(combo.apps))
	for name := range combo.apps {
		if name == project.DefaultAppName {
			continue
		}
		if !project.IsSafeAppName(name) || project.IsReservedAppName(name) {
			return nil, fmt.Errorf("unsafe App name %q", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	apps := []project.App{project.DefaultApp()}
	for _, name := range names {
		apps = append(apps, project.DefaultNamedApp(name))
	}
	return apps, nil
}

// seedRenderComboApps makes configured named Apps discoverable before the first clean render replaces their markers.
func seedRenderComboApps(root string, apps []project.App) error {
	for _, app := range apps {
		if app.Name == project.DefaultAppName {
			continue
		}
		entrypoint := filepath.Join(root, app.Entrypoint)
		if err := os.MkdirAll(filepath.Dir(entrypoint), 0o755); err != nil {
			return fmt.Errorf("create App %s entrypoint directory: %w", app.Name, err)
		}
		if err := os.WriteFile(entrypoint, []byte("package main\n"), 0o644); err != nil {
			return fmt.Errorf("seed App %s entrypoint: %w", app.Name, err)
		}
	}
	return nil
}

func runRenderedGoTests(dir, modCache, buildCache string) error {
	args := []string{"test", "-count=1", "./..."}
	goTest := exec.Command("go", args...)
	goTest.Dir = dir
	goTest.Env = append(os.Environ(),
		"GOMODCACHE="+modCache,
		"GOCACHE="+buildCache,
		"GOFLAGS=",
		"GOWORK=off",
	)
	output, err := goTest.CombinedOutput()
	if err != nil {
		return formatCommandFailure("go test", err, string(output), "")
	}
	if renderDebugEnabled() {
		console.Infof("go test packages: ./...")
	}
	return nil
}

// WriteYAML writes a project config while preserving raw component selections.
func WriteYAML(path string, cfg project.Config) error {
	cfg.Render.StarterKit = project.NormalizeStarterKit(cfg.Render.StarterKit)
	cfg.Render.ComponentContractVersion = project.CurrentComponentContractVersion
	if strings.TrimSpace(cfg.Render.GoForjVersion) == "" {
		cfg.Render.GoForjVersion = version.Semver()
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (cmd *TestRendersCmd) fail(reason, comboID string, cfg *project.Config, err error) {
	console.Errorf("Failure")
	console.Infof("reason: %s", reason)
	console.Infof("combo: %s", comboID)
	if err != nil {
		console.Infof("error: %v", err)
	}
	if cfg != nil {
		if yamlDump, yerr := yaml.Marshal(cfg); yerr == nil {
			console.Infof("config:\n%s", string(yamlDump))
		}
	}
	os.Exit(1)
}

func renderBuildTraceEnabled() bool {
	if !renderDebugEnabled() {
		return false
	}
	for _, key := range []string{"FORJ_RENDER_BUILD_TRACE", "FORJ_RENDER_GO_BUILD_TRACE"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

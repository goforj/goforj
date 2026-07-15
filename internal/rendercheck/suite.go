package rendercheck

import (
	"fmt"
	"io"
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
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// formatCommandFailure retains tool output because generated-project failures are otherwise difficult to reproduce from CI logs.
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

// stepTimer keeps per-combination diagnostics useful without coupling render execution to a profiling package.
type stepTimer struct {
	start time.Time
	parts map[string]time.Duration
}

// renderComboFailure retains the context needed to report one failed render without terminating its worker.
type renderComboFailure struct {
	reason  string
	comboID string
	config  *project.Config
	err     error
}

// Error identifies the failed combination while preserving its underlying cause.
func (failure renderComboFailure) Error() string {
	return fmt.Sprintf("%s for combo %s: %v", failure.reason, failure.comboID, failure.err)
}

// Unwrap exposes the command or filesystem error that caused this combination to fail.
func (failure renderComboFailure) Unwrap() error {
	return failure.err
}

// renderComboFailures keeps every worker failure available while presenting one command-level summary.
type renderComboFailures struct {
	failures   []*renderComboFailure
	total      int
	shardLabel string
}

// renderComboWorker owns the filesystem and toolchain dependencies reused by one render worker.
type renderComboWorker struct {
	workspaceRoot  string
	moduleCache    string
	buildCache     string
	forjExecutable string
	runTests       bool
}

// Error summarizes the failed combinations for the CLI boundary after their detailed reports are printed.
func (failures renderComboFailures) Error() string {
	return fmt.Sprintf("%d of %d render combinations failed%s", len(failures.failures), failures.total, failures.shardLabel)
}

// Unwrap exposes every combination failure to callers that need to inspect the aggregate.
func (failures renderComboFailures) Unwrap() []error {
	causes := make([]error, len(failures.failures))
	for index := range failures.failures {
		causes[index] = failures.failures[index]
	}
	return causes
}

// newStepTimer starts one timing report so failed and slow combinations can be diagnosed independently.
func newStepTimer() *stepTimer {
	return &stepTimer{
		start: time.Now(),
		parts: make(map[string]time.Duration),
	}
}

// Track records a named phase even when that phase returns an error.
func (t *stepTimer) Track(name string, fn func() error) error {
	s := time.Now()
	err := fn()
	t.parts[name] = time.Since(s)
	return err
}

// Report prints phase timings together so parallel render output still identifies its combination.
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

// Suite owns the render combinations selected by one coverage profile.
type Suite struct {
	profile string
	combos  []renderCombo
}

// NewSuite selects a stable render matrix while retaining the legacy full-flag precedence.
func NewSuite(profile string, full bool) *Suite {
	selectedProfile := selectedRenderProfile(profile, full)
	return &Suite{
		profile: selectedProfile,
		combos:  buildRenderCombos(selectedProfile),
	}
}

// List prints the selected shard without performing filesystem or toolchain work.
func (suite *Suite) List(writer io.Writer) error {
	combos, shardLabel, err := shardRenderCombos(suite.combos)
	if err != nil {
		return err
	}
	listRenderCombos(writer, suite.profile, combos, shardLabel)
	return nil
}

// Run renders and compiles every combination in the selected shard before returning their aggregate result.
func (suite *Suite) Run(runTests bool) error {
	combos := suite.combos
	totalCombos := len(combos)
	combos, shardLabel, err := shardRenderCombos(combos)
	if err != nil {
		return err
	}
	console.Infof("Testing %d component combinations with %s profile%s", len(combos), suite.profile, shardLabel)
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
	failureResults := make(chan *renderComboFailure, len(combos))
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		workerRoot := filepath.Join(workspaceRoot, fmt.Sprintf("worker-%02d", i))
		go func(root string) {
			defer wg.Done()
			worker := renderComboWorker{
				workspaceRoot:  root,
				moduleCache:    modCache,
				buildCache:     buildCache,
				forjExecutable: forjExec,
				runTests:       runTests,
			}
			for combo := range jobs {
				if failure := worker.run(combo); failure != nil {
					failureResults <- failure
				}
			}
		}(workerRoot)
	}

	for _, combo := range combos {
		jobs <- combo
	}
	close(jobs)
	wg.Wait()
	close(failureResults)

	failures := make([]*renderComboFailure, 0, len(failureResults))
	for failure := range failureResults {
		failures = append(failures, failure)
	}
	if len(failures) > 0 {
		aggregate := aggregateRenderComboFailures(failures, len(combos), shardLabel)
		for _, failure := range aggregate.failures {
			reportRenderComboFailure(failure)
		}
		return aggregate
	}

	console.Successf("Rendered %d combinations%s", len(combos), shardLabel)
	if totalCombos != len(combos) {
		console.Infof("Shard completed %d/%d combinations", len(combos), totalCombos)
	}
	return nil
}

// listRenderCombos keeps profile review output stable across local and CI invocations.
func listRenderCombos(writer io.Writer, profile string, combos []renderCombo, shardLabel string) {
	fmt.Fprintf(writer, "profile: %s\n", profile)
	fmt.Fprintf(writer, "combinations: %d%s\n\n", len(combos), shardLabel)

	w := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tComponents")
	for _, combo := range combos {
		fmt.Fprintf(w, "%s\t%s\n", combo.id, strings.Join(combo.enabled, ", "))
	}
	_ = w.Flush()
}

// shardRenderCombos partitions by stable matrix order so CI shards remain deterministic.
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

// testRenderWorkspaceRoot keeps generated projects outside the source repository by default.
func testRenderWorkspaceRoot() string {
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_DIR")); v != "" {
		return v
	}
	return filepath.Join(os.TempDir(), "forj_test_renders")
}

// testRenderWorkerCount caps parallel toolchains to avoid exhausting CI hosts with many logical CPUs.
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

// run prepares a clean workspace and returns any render failure to the coordinator for aggregation.
func (worker renderComboWorker) run(combo renderCombo) *renderComboFailure {
	comboID := combo.id
	apps, err := renderComboApps(combo)
	if err != nil {
		return newRenderComboFailure("invalid configured App", comboID, nil, err)
	}

	_ = os.RemoveAll(worker.workspaceRoot)
	if err := os.MkdirAll(worker.workspaceRoot, 0o755); err != nil {
		return newRenderComboFailure("workspace dir failed", comboID, nil, err)
	}
	defer func() {
		_ = os.RemoveAll(worker.workspaceRoot)
	}()

	if err := initModule(worker.workspaceRoot, worker.moduleCache, worker.buildCache); err != nil {
		return newRenderComboFailure("go mod init failed", comboID, nil, err)
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
	ymlPath := filepath.Join(worker.workspaceRoot, ".goforj.yml")

	if err := timer.Track("write_yaml", func() error {
		return testkit.WriteProjectConfig(ymlPath, cfg)
	}); err != nil {
		return newRenderComboFailure("failed to write config", comboID, &cfg, err)
	}
	if err := timer.Track("seed_apps", func() error {
		return seedRenderComboApps(worker.workspaceRoot, apps)
	}); err != nil {
		return newRenderComboFailure("failed to seed configured Apps", comboID, &cfg, err)
	}

	if err := timer.Track("forj_render", func() error {
		render := exec.Command(worker.forjExecutable, "render")
		render.Dir = worker.workspaceRoot
		render.Env = append(os.Environ(),
			"GOMODCACHE="+worker.moduleCache,
			"GOCACHE="+worker.buildCache,
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
		return newRenderComboFailure("render failed", comboID, &cfg, err)
	}

	if combo.starterKit == project.StarterKitTemplHTMX {
		if err := timer.Track("templ_generate", func() error {
			templCmd := exec.Command("go", "run", "github.com/a-h/templ/cmd/templ@v0.3.1020", "generate")
			templCmd.Dir = worker.workspaceRoot
			templCmd.Env = append(os.Environ(),
				"GOMODCACHE="+worker.moduleCache,
				"GOCACHE="+worker.buildCache,
			)
			output, err := templCmd.CombinedOutput()
			if err != nil {
				return formatCommandFailure("templ generate", err, string(output), "")
			}
			return nil
		}); err != nil {
			return newRenderComboFailure("templ generate failed", comboID, &cfg, err)
		}
	}

	if err := timer.Track("wire_gen", func() error {
		for _, app := range apps {
			wireCmd := exec.Command("wire")
			wireCmd.Dir = filepath.Join(worker.workspaceRoot, app.WireDir)
			wireCmd.Env = append(os.Environ(),
				"GOMODCACHE="+worker.moduleCache,
				"GOCACHE="+worker.buildCache,
			)
			if output, err := wireCmd.CombinedOutput(); err != nil {
				return formatCommandFailure("wire generate "+app.Name, err, string(output), "")
			}
		}
		return nil
	}); err != nil {
		return newRenderComboFailure("wire generate failed", comboID, &cfg, err)
	}

	if err := timer.Track("go_build", func() error {
		binDir := filepath.Join(worker.workspaceRoot, "bin")
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
			build.Dir = worker.workspaceRoot
			build.Env = append(os.Environ(),
				"GOMODCACHE="+worker.moduleCache,
				"GOCACHE="+worker.buildCache,
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
		return newRenderComboFailure("go build failed", comboID, &cfg, err)
	}

	if worker.runTests {
		if err := timer.Track("go_test", func() error {
			return runRenderedGoTests(worker.workspaceRoot, worker.moduleCache, worker.buildCache)
		}); err != nil {
			return newRenderComboFailure("go test failed", comboID, &cfg, err)
		}
	}

	timer.Report(fmt.Sprintf("combo %s (%s)", comboID, strings.Join(combo.enabled, ", ")))

	console.Successf("Passed")
	return nil
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

// runRenderedGoTests clears inherited workspace flags so generated projects are tested as independent modules.
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

// newRenderComboFailure packages a worker failure without reporting it concurrently with other combinations.
func newRenderComboFailure(reason, comboID string, cfg *project.Config, err error) *renderComboFailure {
	return &renderComboFailure{
		reason:  reason,
		comboID: comboID,
		config:  cfg,
		err:     err,
	}
}

// aggregateRenderComboFailures orders concurrent results so repeated runs report the same diagnostic sequence.
func aggregateRenderComboFailures(failures []*renderComboFailure, total int, shardLabel string) renderComboFailures {
	sort.SliceStable(failures, func(left, right int) bool {
		return failures[left].comboID < failures[right].comboID
	})
	return renderComboFailures{
		failures:   failures,
		total:      total,
		shardLabel: shardLabel,
	}
}

// reportRenderComboFailure prints the same detailed diagnostics once workers have finished.
func reportRenderComboFailure(failure *renderComboFailure) {
	console.Errorf("Failure")
	console.Infof("reason: %s", failure.reason)
	console.Infof("combo: %s", failure.comboID)
	if failure.err != nil {
		console.Infof("error: %v", failure.err)
	}
	if failure.config != nil {
		if yamlDump, yerr := yaml.Marshal(failure.config); yerr == nil {
			console.Infof("config:\n%s", string(yamlDump))
		}
	}
}

// renderBuildTraceEnabled requires general debug output before emitting the especially noisy Go build trace.
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

// renderDebugEnabled follows the renderer's established environment contract for diagnostic output.
func renderDebugEnabled() bool {
	for _, key := range []string{"FORJ_DEBUG", "DEBUG"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

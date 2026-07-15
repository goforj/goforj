package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testexec"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// MetricsOverheadMeasureCmd renders a temp app and compares telemetry overhead with metrics on vs off.
type MetricsOverheadMeasureCmd struct {
	logger *logger.AppLogger

	Iterations     int  `help:"Fixed iterations for non-auth telemetry surfaces" default:"5000"`
	AuthIterations int  `help:"Fixed iterations for auth telemetry surface" default:"500"`
	Rounds         int  `help:"Comparison rounds to run and summarize with medians" default:"5"`
	Keep           bool `help:"Keep the rendered temp project after completion" short:"k"`
	Silent         bool `help:"Suppress command progress output" short:"s"`
}

type metricsOverheadProbeOutput struct {
	Mode     string                       `json:"mode"`
	Results  []metricsOverheadProbeResult `json:"results"`
	Warnings []string                     `json:"warnings,omitempty"`
}

type metricsOverheadProbeResult struct {
	Surface     string  `json:"surface"`
	Iterations  int     `json:"iterations"`
	Errors      int     `json:"errors"`
	ElapsedNS   int64   `json:"elapsed_ns"`
	NSPerOp     float64 `json:"ns_per_op"`
	Allocs      uint64  `json:"allocs"`
	Bytes       uint64  `json:"bytes"`
	AllocsPerOp float64 `json:"allocs_per_op"`
	BytesPerOp  float64 `json:"bytes_per_op"`
}

func (*MetricsOverheadMeasureCmd) Signature() string {
	return `name:"bench:metrics-overhead" help:"Measure generated telemetry overhead across observed surfaces" hidden:""`
}

func NewMetricsOverheadMeasureCmd(logger *logger.AppLogger) *MetricsOverheadMeasureCmd {
	return &MetricsOverheadMeasureCmd{logger: logger}
}

func (cmd *MetricsOverheadMeasureCmd) Run() error {
	if cmd.Iterations <= 0 {
		return fmt.Errorf("iterations must be greater than zero")
	}
	if cmd.AuthIterations <= 0 {
		return fmt.Errorf("auth-iterations must be greater than zero")
	}
	if cmd.Rounds <= 0 {
		return fmt.Errorf("rounds must be greater than zero")
	}

	modCache, buildCache := testkit.GoCachePaths()
	dir, err := os.MkdirTemp("", "forj_metrics_overhead_")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	if !cmd.Keep {
		defer os.RemoveAll(dir)
	}
	caches := testexec.GoCaches{ModulePath: modCache, BuildPath: buildCache}
	workspace := testexec.NewWorkspace(cmd.logger, cmd.Silent, dir, caches)

	if !cmd.Silent {
		testkit.PrintSection("Metrics Overhead")
		console.Actionf("Rendering fixed telemetry probe app")
	}

	cfg := project.Config{
		ProjectName:  "Metrics Overhead",
		GoModuleName: "example.com/metricsoverheadapp",
		UpdatedAt:    "2026-05-02 00:00:00 UTC",
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:            true,
				Cache:          true,
				Events:         true,
				WebAPI:         true,
				Metrics:        true,
				Auth:           true,
				Mail:           true,
				Jobs:           true,
				Scheduler:      true,
				Storage:        true,
				DatabaseSQLite: true,
			},
		},
	}

	if err := testkit.WriteProjectConfig(filepath.Join(dir, ".goforj.yml"), cfg); err != nil {
		return err
	}

	builtForj, err := testkit.BuildForjBinary(modCache, buildCache)
	if err != nil {
		return err
	}
	defer builtForj.Cleanup()
	forjExec := builtForj.Path

	if err := workspace.Run("render", forjExec, "render"); err != nil {
		return err
	}
	queueEnv := map[string]string{
		"QUEUE_DRIVER":            "workerpool",
		"QUEUE_SUPPORTED_DRIVERS": "workerpool",
	}
	if err := testkit.ReplaceOrAppendEnvValues([]string{filepath.Join(dir, ".env")}, queueEnv); err != nil {
		return fmt.Errorf("set metrics benchmark queue driver: %w", err)
	}
	if err := workspace.Run("generate queue", forjExec, "generate", "--queue"); err != nil {
		return err
	}

	if err := cmd.writeProbe(dir, cfg.GoModuleName); err != nil {
		return err
	}

	rounds := make([]metricsOverheadRound, 0, cmd.Rounds)
	for round := 1; round <= cmd.Rounds; round++ {
		off, err := cmd.runProbe(dir, modCache, buildCache, "off", round)
		if err != nil {
			return err
		}
		on, err := cmd.runProbe(dir, modCache, buildCache, "on", round)
		if err != nil {
			return err
		}
		rounds = append(rounds, metricsOverheadRound{Index: round, Off: off, On: on})
	}

	if !cmd.Silent {
		cmd.printComparison(rounds)
		if hasWarnings(rounds) {
			console.Warnf("Probe warnings detected; see table inputs or rerun with --keep to inspect the temp app")
		}
		if cmd.Keep {
			cmd.logger.Info().Str("path", dir).Msg("Kept rendered telemetry probe project")
		}
	}

	return nil
}

type metricsOverheadRound struct {
	Index int
	Off   *metricsOverheadProbeOutput
	On    *metricsOverheadProbeOutput
}

func (cmd *MetricsOverheadMeasureCmd) writeProbe(dir, module string) error {
	source := strings.ReplaceAll(metricsOverheadProbeSource, "__MODULE__", module)
	probeDir := filepath.Join(dir, "cmd", "metricsprobe")
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		return fmt.Errorf("create probe dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(probeDir, "main.go"), []byte(source), 0o644); err != nil {
		return fmt.Errorf("write probe source: %w", err)
	}
	return nil
}

func (cmd *MetricsOverheadMeasureCmd) runProbe(dir, modCache, buildCache, mode string, round int) (*metricsOverheadProbeOutput, error) {
	if !cmd.Silent {
		console.Actionf("Running telemetry probe (%s round %d/%d)", mode, round, cmd.Rounds)
	}

	envMap := map[string]string{
		"APP_ENV":                 "local",
		"APP_URL":                 "http://localhost:3000",
		"DB_DRIVER":               "sqlite",
		"DB_DATABASE":             "./_data/sqlite/app.db",
		"CACHE_DRIVER":            "memory",
		"STORAGE_DRIVER":          "memory",
		"EVENTS_DRIVER":           "inproc",
		"QUEUE_DRIVER":            "sync",
		"MAIL_DRIVER":             "log",
		"MAIL_SUPPORTED_DRIVERS":  "log",
		"HTTP_ACCESS_LOG_ENABLED": "false",
		"AUTH_REGISTER_REQUIRES_EMAIL_VERIFICATION": "false",
	}
	for _, key := range []string{
		"METRICS_HTTP_ENABLED",
		"METRICS_CACHE_ENABLED",
		"METRICS_STORAGE_ENABLED",
		"METRICS_EVENTS_ENABLED",
		"METRICS_MAIL_ENABLED",
		"METRICS_QUEUE_ENABLED",
		"METRICS_DATABASE_ENABLED",
		"METRICS_AUTH_ENABLED",
		"METRICS_SCHEDULER_ENABLED",
	} {
		envMap[key] = boolString(mode == "on")
	}

	runCtx := context.Background()
	execCmd := exec.CommandContext(
		runCtx,
		"go",
		"run",
		"./cmd/metricsprobe",
		"-iterations",
		fmt.Sprintf("%d", cmd.Iterations),
		"-auth-iterations",
		fmt.Sprintf("%d", cmd.AuthIterations),
		"-mode",
		mode,
	)
	execCmd.Dir = dir
	execCmd.Env = testkit.WithEnvOverrides(testkit.ProcessGoEnv("", nil), envMap)
	execCmd.Env = append(execCmd.Env, "GOCACHE="+buildCache, "GOMODCACHE="+modCache)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr
	if err := execCmd.Run(); err != nil {
		return nil, fmt.Errorf("run telemetry probe (%s): %w\nstdout:\n%s\nstderr:\n%s", mode, err, stdout.String(), stderr.String())
	}

	var out metricsOverheadProbeOutput
	jsonLine := lastJSONLine(stdout.String())
	if jsonLine == "" {
		return nil, fmt.Errorf("telemetry probe (%s) did not emit json\nstdout:\n%s\nstderr:\n%s", mode, stdout.String(), stderr.String())
	}
	if err := json.Unmarshal([]byte(jsonLine), &out); err != nil {
		return nil, fmt.Errorf("decode telemetry probe output (%s): %w\nstdout:\n%s\nstderr:\n%s", mode, err, stdout.String(), stderr.String())
	}
	return &out, nil
}

func (cmd *MetricsOverheadMeasureCmd) printComparison(rounds []metricsOverheadRound) {
	offMap := make(map[string][]metricsOverheadProbeResult)
	onMap := make(map[string][]metricsOverheadProbeResult)
	for _, round := range rounds {
		for _, result := range round.Off.Results {
			offMap[result.Surface] = append(offMap[result.Surface], result)
		}
		for _, result := range round.On.Results {
			onMap[result.Surface] = append(onMap[result.Surface], result)
		}
	}

	surfaces := make([]string, 0, len(offMap))
	for surface := range offMap {
		surfaces = append(surfaces, surface)
	}
	sort.Strings(surfaces)

	latencyRows := make([][]string, 0, len(surfaces)+1)
	latencyRows = append(latencyRows, []string{
		"Surface",
		"Iter",
		"Off ns/op",
		"On ns/op",
		"Delta %",
		"Errors",
		"Rounds",
	})
	memoryRows := make([][]string, 0, len(surfaces)+1)
	memoryRows = append(memoryRows, []string{
		"Surface",
		"Off alloc/op",
		"On alloc/op",
		"Alloc delta %",
		"Off B/op",
		"On B/op",
		"Bytes delta %",
	})
	for _, surface := range surfaces {
		offResult := medianProbeResult(offMap[surface])
		onResult := medianProbeResult(onMap[surface])
		latencyRows = append(latencyRows, []string{
			surface,
			fmt.Sprintf("%d", offResult.Iterations),
			fmt.Sprintf("%.1f", offResult.NSPerOp),
			fmt.Sprintf("%.1f", onResult.NSPerOp),
			formatPercentDelta(offResult.NSPerOp, onResult.NSPerOp),
			fmt.Sprintf("%d/%d", offResult.Errors, onResult.Errors),
			fmt.Sprintf("%d", len(offMap[surface])),
		})
		memoryRows = append(memoryRows, []string{
			surface,
			fmt.Sprintf("%.3f", offResult.AllocsPerOp),
			fmt.Sprintf("%.3f", onResult.AllocsPerOp),
			formatPercentDelta(offResult.AllocsPerOp, onResult.AllocsPerOp),
			fmt.Sprintf("%.1f", offResult.BytesPerOp),
			fmt.Sprintf("%.1f", onResult.BytesPerOp),
			formatPercentDelta(offResult.BytesPerOp, onResult.BytesPerOp),
		})
	}
	fmt.Fprintln(os.Stdout, "Latency")
	printASCIITable(os.Stdout, latencyRows)
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Memory")
	printASCIITable(os.Stdout, memoryRows)
}

func formatPercentDelta(base, current float64) string {
	if base == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%+.1f%%", ((current-base)/base)*100)
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func hasWarnings(rounds []metricsOverheadRound) bool {
	for _, round := range rounds {
		if len(round.Off.Warnings) > 0 || len(round.On.Warnings) > 0 {
			return true
		}
	}
	return false
}

func lastJSONLine(stdout string) string {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") {
			return line
		}
	}
	return ""
}

func medianProbeResult(results []metricsOverheadProbeResult) metricsOverheadProbeResult {
	if len(results) == 0 {
		return metricsOverheadProbeResult{}
	}
	base := results[0]
	nsValues := make([]float64, 0, len(results))
	allocValues := make([]float64, 0, len(results))
	byteValues := make([]float64, 0, len(results))
	errorValues := make([]float64, 0, len(results))
	for _, result := range results {
		nsValues = append(nsValues, result.NSPerOp)
		allocValues = append(allocValues, result.AllocsPerOp)
		byteValues = append(byteValues, result.BytesPerOp)
		errorValues = append(errorValues, float64(result.Errors))
	}
	base.NSPerOp = medianFloat64(nsValues)
	base.AllocsPerOp = medianFloat64(allocValues)
	base.BytesPerOp = medianFloat64(byteValues)
	base.Errors = int(math.Round(medianFloat64(errorValues)))
	return base
}

func medianFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64{}, values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func printASCIITable(out *os.File, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	border := func() {
		fmt.Fprint(out, "+")
		for _, width := range widths {
			fmt.Fprint(out, strings.Repeat("-", width+2))
			fmt.Fprint(out, "+")
		}
		fmt.Fprintln(out)
	}

	printRow := func(row []string) {
		fmt.Fprint(out, "|")
		for i, cell := range row {
			fmt.Fprintf(out, " %-*s |", widths[i], cell)
		}
		fmt.Fprintln(out)
	}

	border()
	printRow(rows[0])
	border()
	for _, row := range rows[1:] {
		printRow(row)
	}
	border()
}

const metricsOverheadProbeSource = `package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	cacheapi "github.com/goforj/cache"
	"github.com/goforj/cache/cachecore"
	"github.com/goforj/env/v2"
	goforjmail "github.com/goforj/mail"
	"github.com/goforj/queue"
	"github.com/goforj/scheduler/v2"
	"github.com/goforj/web/webmiddleware"
	"github.com/goforj/web/webtest"

	"__MODULE__/internal/auth"
	"__MODULE__/internal/caches"
	"__MODULE__/internal/database"
	"__MODULE__/internal/events"
	"__MODULE__/internal/logger"
	"__MODULE__/internal/mail"
	"__MODULE__/internal/metrics"
	"__MODULE__/migrations"
	"__MODULE__/internal/storages"
)

type probeOutput struct {
	Mode     string        ` + "`json:\"mode\"`" + `
	Results  []probeResult ` + "`json:\"results\"`" + `
	Warnings []string      ` + "`json:\"warnings,omitempty\"`" + `
}

type probeResult struct {
	Surface      string  ` + "`json:\"surface\"`" + `
	Iterations   int     ` + "`json:\"iterations\"`" + `
	Errors       int     ` + "`json:\"errors\"`" + `
	ElapsedNS    int64   ` + "`json:\"elapsed_ns\"`" + `
	NSPerOp      float64 ` + "`json:\"ns_per_op\"`" + `
	Allocs       uint64  ` + "`json:\"allocs\"`" + `
	Bytes        uint64  ` + "`json:\"bytes\"`" + `
	AllocsPerOp  float64 ` + "`json:\"allocs_per_op\"`" + `
	BytesPerOp   float64 ` + "`json:\"bytes_per_op\"`" + `
}

type probeEnv struct {
	metrics *metrics.Manager
	auth    *auth.Service
	cache   *caches.Manager
	storage *storages.Manager
	events  *events.Manager
	mail    *mail.Manager
	db      *database.Connections
}

type probeEvent struct {
	ID string
}

func (probeEvent) Topic() string { return "probe.event" }

func main() {
	mode := flag.String("mode", "off", "probe mode")
	iterations := flag.Int("iterations", 5000, "iterations per surface")
	authIterations := flag.Int("auth-iterations", 500, "iterations for auth surface")
	flag.Parse()

	if *iterations <= 0 {
		fatal(fmt.Errorf("iterations must be greater than zero"))
	}
	if *authIterations <= 0 {
		fatal(fmt.Errorf("auth-iterations must be greater than zero"))
	}
	if err := env.Load(); err != nil {
		fatal(err)
	}

	penv, warnings, err := newProbeEnv()
	if err != nil {
		fatal(err)
	}
	defer func() {
		if penv.db != nil {
			_ = penv.db.Close(context.Background())
		}
	}()

	results := make([]probeResult, 0, 9)
	results = append(results, measure("http", *iterations, func() error { return runHTTP(penv) }))
	results = append(results, measure("cache", *iterations, func() error { return runCache(penv) }))
	results = append(results, measure("storage", *iterations, func() error { return runStorage(penv) }))
	results = append(results, measure("events", *iterations, func() error { return runEvents(penv) }))
	results = append(results, measure("mail", *iterations, func() error { return runMail(penv) }))
	results = append(results, measure("database", *iterations, func() error { return runDatabase(penv) }))
	results = append(results, measure("auth", *authIterations, func() error { return runAuth(penv) }))
	results = append(results, measure("queue", *iterations, func() error { return runQueue(penv) }))
	results = append(results, measure("scheduler", *iterations, func() error { return runScheduler(penv) }))

	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(probeOutput{Mode: *mode, Results: results, Warnings: warnings}); err != nil {
		fatal(err)
	}
}

func newProbeEnv() (*probeEnv, []string, error) {
	log := logger.NewSilentLogger()
	metricsManager := metrics.NewManager()

	cacheManager, err := caches.NewManager()
	if err != nil {
		return nil, nil, fmt.Errorf("new cache manager: %w", err)
	}
	if metricsManager.CacheEnabled() {
		cacheManager = cacheManager.WithObserver(caches.ObserverFunc(func(ctx context.Context, event caches.CacheOpEvent) {
			metricsManager.RecordCacheOperation(ctx, event)
		}))
	}

	storageManager, err := storages.NewManager()
	if err != nil {
		return nil, nil, fmt.Errorf("new storage manager: %w", err)
	}
	if metricsManager.StorageEnabled() {
		storageManager = storageManager.WithObserver(storages.ObserverFunc(func(ctx context.Context, event storages.StorageOpEvent) {
			metricsManager.RecordStorageOperation(ctx, event)
		}))
	}

	eventManager, err := events.NewManager()
	if err != nil {
		return nil, nil, fmt.Errorf("new event manager: %w", err)
	}
	if metricsManager.EventsEnabled() {
		eventManager = eventManager.WithObserver(eventMetricsObserver{metrics: metricsManager})
	}
	if _, err := eventManager.Default().Subscribe(func(_ context.Context, payload probeEvent) error {
		if payload.ID == "" {
			return fmt.Errorf("empty probe event id")
		}
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("subscribe probe event: %w", err)
	}

	var mailManager *mail.Manager
	if metricsManager.MailEnabled() {
		mailManager, err = mail.NewManagerWithObserver(mail.ObserverFunc(func(ctx context.Context, event mail.MailSendEvent) {
			metricsManager.RecordMailSend(ctx, metrics.MailSendMetricEvent{
				Name:     event.Name,
				Driver:   event.Driver,
				Err:      event.Err,
				Duration: event.Duration,
			})
		}))
	} else {
		mailManager, err = mail.NewManager()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("new mail manager: %w", err)
	}

	conns := database.NewConnections(metrics)
	migrate := migrations.NewMigrateCmd(log, conns)
	if err := migrate.Run(); err != nil {
		return nil, nil, fmt.Errorf("migrate: %w", err)
	}
	db, err := conns.Default()
	if err != nil {
		return nil, nil, fmt.Errorf("open default db: %w", err)
	}
	if err := db.Exec("CREATE TABLE IF NOT EXISTS probe_metric_rows (id INTEGER PRIMARY KEY, name TEXT NOT NULL)").Error; err != nil {
		return nil, nil, fmt.Errorf("create probe table: %w", err)
	}
	if err := db.Exec("INSERT OR IGNORE INTO probe_metric_rows (id, name) VALUES (1, ?)", "probe").Error; err != nil {
		return nil, nil, fmt.Errorf("seed probe table row: %w", err)
	}

	service := auth.NewService(
		auth.NewUserRepo(conns, cacheManager.Sessions()),
		auth.NewAuthSessionRepo(conns, cacheManager.Sessions()),
		auth.NewEmailVerificationRepo(conns),
		auth.NewPasswordResetRepo(conns),
		auth.NewLoginAttemptRepo(conns),
		auth.NewLogDelivery(log),
		metrics,
	)
	if _, err := service.Register(context.Background(), auth.RegisterInput{
		Username:    "probe",
		DisplayName: "Probe User",
		Email:       "probe@example.com",
		Password:    "Secret-pass",
		UserAgent:   "probe-init",
		IPAddress:   "127.0.0.1",
	}); err != nil && !strings.Contains(err.Error(), "already in use") {
		return nil, nil, fmt.Errorf("seed auth user: %w", err)
	}

	return &probeEnv{
		metrics: metricsManager,
		auth:    service,
		cache:   cacheManager,
		storage: storageManager,
		events:  eventManager,
		mail:    mailManager,
		db:      conns,
	}, nil, nil
}

type eventMetricsObserver struct {
	metrics *metrics.Manager
}

func (o eventMetricsObserver) OnEventPublish(ctx context.Context, event events.EventPublishEvent) {
	if o.metrics != nil {
		o.metrics.RecordEventPublish(ctx, metrics.EventPublishMetricEvent{
			Bus:      event.Bus,
			Driver:   string(event.Driver),
			Topic:    event.Topic,
			Err:      event.Err,
			Duration: event.Duration,
		})
	}
}

func (o eventMetricsObserver) OnEventSubscribe(ctx context.Context, event events.EventSubscriptionEvent) {
	if o.metrics != nil {
		o.metrics.RecordEventSubscribe(ctx, metrics.EventSubscriptionMetricEvent{
			Bus:     event.Bus,
			Driver:  string(event.Driver),
			Topic:   event.Topic,
			Handler: event.Handler,
			Err:     event.Err,
		})
	}
}

func (o eventMetricsObserver) OnEventUnsubscribe(ctx context.Context, event events.EventSubscriptionEvent) {
	if o.metrics != nil {
		o.metrics.RecordEventUnsubscribe(ctx, metrics.EventSubscriptionMetricEvent{
			Bus:     event.Bus,
			Driver:  string(event.Driver),
			Topic:   event.Topic,
			Handler: event.Handler,
		})
	}
}

func (o eventMetricsObserver) OnEventDeliveryStart(ctx context.Context, event events.EventDeliveryEvent) {
	if o.metrics != nil {
		o.metrics.RecordEventDeliveryStart(ctx, metrics.EventDeliveryMetricEvent{
			Bus:     event.Bus,
			Driver:  string(event.Driver),
			Topic:   event.Topic,
			Handler: event.Handler,
		})
	}
}

func (o eventMetricsObserver) OnEventDeliveryFinish(ctx context.Context, event events.EventDeliveryEvent) {
	if o.metrics != nil {
		o.metrics.RecordEventDeliveryFinish(ctx, metrics.EventDeliveryMetricEvent{
			Bus:      event.Bus,
			Driver:   string(event.Driver),
			Topic:    event.Topic,
			Handler:  event.Handler,
			Err:      event.Err,
			Duration: event.Duration,
		})
	}
}

func measure(surface string, iterations int, fn func() error) probeResult {
	runtime.GC()
	var before runtime.MemStats
	var after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	errors := 0
	for i := 0; i < iterations; i++ {
		if err := fn(); err != nil {
			errors++
		}
	}
	elapsed := time.Since(started)
	runtime.ReadMemStats(&after)
	allocs := after.Mallocs - before.Mallocs
	bytes := after.TotalAlloc - before.TotalAlloc
	return probeResult{
		Surface:     surface,
		Iterations:  iterations,
		Errors:      errors,
		ElapsedNS:   elapsed.Nanoseconds(),
		NSPerOp:     float64(elapsed.Nanoseconds()) / float64(iterations),
		Allocs:      allocs,
		Bytes:       bytes,
		AllocsPerOp: float64(allocs) / float64(iterations),
		BytesPerOp:  float64(bytes) / float64(iterations),
	}
}

func runHTTP(p *probeEnv) error {
	req, err := http.NewRequest(http.MethodGet, "http://localhost/api/v1/hello", nil)
	if err != nil {
		return err
	}
	ctx := webtest.NewContext(req, nil, "/api/v1/hello", nil)
	p.metrics.RecordHTTPRequest(ctx, http.StatusOK, 2*time.Millisecond)
	_ = webmiddleware.RequestLoggerValues{Status: http.StatusOK, URI: "/api/v1/hello", Method: http.MethodGet, Latency: 2 * time.Millisecond}
	return nil
}

func runCache(p *probeEnv) error {
	store := p.cache.Default().WithContext(context.Background())
	if err := cacheapi.Set(store, "probe", map[string]string{"v": "x"}, time.Minute); err != nil {
		return err
	}
	_, _, err := cacheapi.Get[map[string]string](store, "probe")
	return err
}

func runStorage(p *probeEnv) error {
	const path = "probe.txt"
	data := []byte("hello world")
	if err := p.storage.Default().Put(path, data); err != nil {
		return err
	}
	if _, err := p.storage.Default().Get(path); err != nil {
		return err
	}
	return p.storage.Default().Delete(path)
}

func runEvents(p *probeEnv) error {
	return p.events.Default().Publish(probeEvent{ID: "123"})
}

func runMail(p *probeEnv) error {
	return p.mail.Default().Send(context.Background(), goforjmail.Message{
		To:      []goforjmail.Recipient{{Email: "probe@example.com", Name: "Probe"}},
		Subject: "Probe",
		Text:    "hello",
	})
}

func runDatabase(p *probeEnv) error {
	db, err := p.db.Default()
	if err != nil {
		return err
	}
	return db.Exec("UPDATE probe_metric_rows SET name = ? WHERE id = 1", "probe").Error
}

func runAuth(p *probeEnv) error {
	_, _, err := p.auth.Authenticate(context.Background(), auth.AuthenticateInput{
		Login:     "probe@example.com",
		Password:  "Secret-pass",
		UserAgent: "probe",
		IPAddress: "127.0.0.1",
	})
	return err
}

func runQueue(p *probeEnv) error {
	p.metrics.RecordQueueEvent(queue.Event{
		Kind:    queue.EventEnqueueAccepted,
		Driver:  queue.DriverSync,
		Queue:   "default",
		JobType: "probe.job",
		Time:    time.Now(),
	})
	p.metrics.RecordQueueEvent(queue.Event{
		Kind:    queue.EventProcessStarted,
		Driver:  queue.DriverSync,
		Queue:   "default",
		JobType: "probe.job",
		Time:    time.Now(),
	})
	p.metrics.RecordQueueEvent(queue.Event{
		Kind:     queue.EventProcessSucceeded,
		Driver:   queue.DriverSync,
		Queue:    "default",
		JobType:  "probe.job",
		Duration: 2 * time.Millisecond,
		Time:     time.Now(),
	})
	return nil
}

func runScheduler(p *probeEnv) error {
	p.metrics.RecordSchedulerJob(scheduler.JobEvent{
		Type:       scheduler.JobStarted,
		Name:       "probe:job",
		TargetKind: "callable",
	})
	p.metrics.RecordSchedulerJob(scheduler.JobEvent{
		Type:       scheduler.JobSucceeded,
		Name:       "probe:job",
		TargetKind: "callable",
		Duration:   2 * time.Millisecond,
	})
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
`

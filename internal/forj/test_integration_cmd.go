package forj

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/goforj/str/v2"

	"github.com/goforj/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testexec"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
)

// TestIntegrationCmd runs integration tests for the GoForj CLI.
type TestIntegrationCmd struct {
	logger *logger.AppLogger

	// Suite chooses which integration pipeline to run.
	Suite string `arg:"" optional:"" default:"all" enum:"framework,rendered,all" help:"Integration suite to run"`

	// Target narrows the package target within the selected suite.
	Target string `help:"Integration target to run" default:"all" enum:"all,auth,makecmd,modelgen,migrations,database"`

	// Variant selects the DB variant. Defaults to all rendered variants so no-arg runs exercise the full matrix.
	Variant string `help:"Database variant selection" default:"all" enum:"sqlite,mysql,postgres,all"`

	// Shard selects one one-based framework shard as N/M.
	Shard string `help:"Run one framework shard in N/M form (for example 1/6)"`

	// FrameworkProfile chooses the integration-only build-tag layer to discover and run.
	FrameworkProfile string `help:"Framework integration profile" default:"integration" enum:"integration,lighthouse,multiapp,verifiercalibration"`

	// Silent suppresses shadow-printed commands.
	Silent bool `help:"Suppress command output" short:"s"`

	// Verbose enables verbose test output.
	Verbose bool `help:"Enable verbose test output" short:"v"`
}

// integrationStep names one command so execution and failure reporting cannot drift apart.
type integrationStep struct {
	name string
	args []string
}

// integrationExecutor owns the output policy and Go caches shared by every command in one integration run.
type integrationExecutor struct {
	logger   *logger.AppLogger
	silent   bool
	verbose  bool
	caches   testexec.GoCaches
	forjExec string
}

// frameworkShard identifies one one-based partition of the discovered integration tests.
type frameworkShard struct {
	number int
	total  int
}

// goTestListEvent contains the fields emitted by go test -json while listing runnable tests.
type goTestListEvent struct {
	Action  string
	Package string
	Output  string
}

// integrationTestName identifies one top-level test without conflating equal names from separate packages.
type integrationTestName struct {
	packagePath string
	name        string
}

// integrationTestTarget groups one shard's selected tests into a package-specific command.
type integrationTestTarget struct {
	packagePath string
	testNames   []string
}

// dbIntegrationVariantSpec defines the component selection and runtime environment for one rendered database target.
type dbIntegrationVariantSpec struct {
	applyConfig func(*project.Components)
	testEnv     map[string]string
}

var dbIntegrationVariantSpecs = map[string]dbIntegrationVariantSpec{
	"mysql": {
		applyConfig: func(components *project.Components) {
			components.DatabaseMySQL = true
		},
		testEnv: map[string]string{
			"DB_DRIVER":               "mysql",
			"DB_HOST":                 "127.0.0.1",
			"DB_PORT":                 "3306",
			"DB_DATABASE":             "db",
			"DB_USERNAME":             "user",
			"DB_PASSWORD":             "password",
			"DB_HOST_INTEGRATION":     "127.0.0.1",
			"DB_PORT_INTEGRATION":     "3306",
			"DB_DATABASE_INTEGRATION": "db",
			"DB_USERNAME_INTEGRATION": "user",
			"DB_PASSWORD_INTEGRATION": "password",
			"TZ":                      "America/Los_Angeles",
		},
	},
	"postgres": {
		applyConfig: func(components *project.Components) {
			components.DatabasePostgres = true
		},
		testEnv: map[string]string{
			"DB_DRIVER":               "postgres",
			"DB_HOST":                 "127.0.0.1",
			"DB_PORT":                 "5432",
			"DB_DATABASE":             "app",
			"DB_USERNAME":             "postgres",
			"DB_PASSWORD":             "postgres",
			"DB_HOST_INTEGRATION":     "127.0.0.1",
			"DB_PORT_INTEGRATION":     "5432",
			"DB_DATABASE_INTEGRATION": "app",
			"DB_USERNAME_INTEGRATION": "postgres",
			"DB_PASSWORD_INTEGRATION": "postgres",
			"TZ":                      "America/Los_Angeles",
		},
	},
	"sqlite": {
		applyConfig: func(components *project.Components) {
			components.DatabaseSQLite = true
		},
		testEnv: map[string]string{
			"DB_DRIVER":   "sqlite",
			"DB_DATABASE": "./_data/sqlite/app.db",
		},
	},
}

// Signature exposes integration validation as a maintainer-only command.
func (*TestIntegrationCmd) Signature() string {
	return `name:"test:integration" help:"Run integration tests" hidden:""`
}

// NewTestIntegrationCmd creates a new TestIntegrationCmd instance.
func NewTestIntegrationCmd(logger *logger.AppLogger) *TestIntegrationCmd {
	return &TestIntegrationCmd{logger: logger}
}

// Run executes integration tests for the model generator.
func (cmd *TestIntegrationCmd) Run() error {
	modCache, buildCache := testkit.GoCachePaths()
	executor := integrationExecutor{
		logger:  cmd.logger,
		silent:  cmd.Silent,
		verbose: cmd.Verbose,
		caches: testexec.GoCaches{
			ModulePath: modCache,
			BuildPath:  buildCache,
		},
	}
	suite := str.Of(cmd.Suite).ToLower().Trim().String()
	target := str.Of(cmd.Target).ToLower().Trim().String()
	variant := str.Of(cmd.Variant).ToLower().Trim().String()
	frameworkProfile := str.Of(cmd.FrameworkProfile).ToLower().Trim().String()
	shard, err := parseFrameworkShard(cmd.Shard)
	if err != nil {
		return err
	}
	if err := validateShardSuite(suite, cmd.Shard); err != nil {
		return err
	}
	forjExec, cleanup, err := integrationForjBinary(executor.caches)
	if err != nil {
		return err
	}
	defer cleanup()
	executor.forjExec = forjExec

	if !cmd.Silent {
		testkit.PrintSection(fmt.Sprintf("Integration Suite: %s", suite))
	}

	switch suite {
	case "framework":
		return cmd.runFrameworkSuite(executor, target, frameworkProfile, shard)
	case "rendered":
		return cmd.runRenderedSuite(executor, target, variant)
	case "all":
		if err := cmd.runFrameworkSuite(executor, target, frameworkProfile, shard); err != nil {
			return err
		}
		return cmd.runRenderedSuite(executor, target, variant)
	default:
		return fmt.Errorf("unknown integration suite %q", suite)
	}
}

// frameworkProfileTags defines each profile as the test files added beyond a narrower build-tag baseline.
func frameworkProfileTags(profile string) (string, string, error) {
	switch profile {
	case "", "integration":
		return "", "integration", nil
	case "lighthouse":
		return "integration", "integration,lighthouse", nil
	case "multiapp":
		return "integration", "integration,multiapp", nil
	case "verifiercalibration":
		return "integration", "integration,verifiercalibration", nil
	default:
		return "", "", fmt.Errorf("unknown framework integration profile %q", profile)
	}
}

// parseFrameworkShard converts the one-based N/M command value into a validated shard.
func parseFrameworkShard(value string) (frameworkShard, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "1/1"
	}
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return frameworkShard{}, fmt.Errorf("invalid framework shard %q: expected N/M, for example 1/6", value)
	}
	if parts[0] == "" || parts[1] == "" || strings.Trim(parts[0], "0123456789") != "" || strings.Trim(parts[1], "0123456789") != "" {
		return frameworkShard{}, fmt.Errorf("invalid framework shard %q: N and M must be positive integers", value)
	}
	number, numberErr := strconv.Atoi(parts[0])
	total, totalErr := strconv.Atoi(parts[1])
	if numberErr != nil || totalErr != nil {
		return frameworkShard{}, fmt.Errorf("invalid framework shard %q: N and M must be integers", value)
	}
	if total < 1 {
		return frameworkShard{}, fmt.Errorf("invalid framework shard %q: total must be at least 1", value)
	}
	if number < 1 || number > total {
		return frameworkShard{}, fmt.Errorf("invalid framework shard %q: shard number must be between 1 and %d", value, total)
	}
	return frameworkShard{number: number, total: total}, nil
}

// validateShardSuite prevents an explicitly requested framework partition from being ignored by another suite.
func validateShardSuite(suite, shardValue string) error {
	if suite != "framework" && strings.TrimSpace(shardValue) != "" {
		return fmt.Errorf("--shard is only supported by the framework integration suite")
	}
	return nil
}

// integrationForjBinary reuses an explicitly validated binary or builds one snapshot for the entire command run.
func integrationForjBinary(caches testexec.GoCaches) (string, func(), error) {
	if provided := strings.TrimSpace(os.Getenv("FORJ_INTEGRATION_FORJ_PATH")); provided != "" {
		absolutePath, err := filepath.Abs(provided)
		if err != nil {
			return "", nil, fmt.Errorf("resolve FORJ_INTEGRATION_FORJ_PATH: %w", err)
		}
		info, err := os.Stat(absolutePath)
		if err != nil {
			return "", nil, fmt.Errorf("validate FORJ_INTEGRATION_FORJ_PATH: %w", err)
		}
		if info.IsDir() {
			return "", nil, fmt.Errorf("validate FORJ_INTEGRATION_FORJ_PATH: %s is a directory", absolutePath)
		}
		return absolutePath, func() {}, nil
	}
	builtForj, err := testkit.BuildForjBinary(caches.ModulePath, caches.BuildPath)
	if err != nil {
		return "", nil, err
	}
	return builtForj.Path, builtForj.Cleanup, nil
}

// runFrameworkSuite runs integration-tagged tests against this repository.
func (cmd *TestIntegrationCmd) runFrameworkSuite(executor integrationExecutor, target, profile string, shard frameworkShard) error {
	if target != "" && target != "all" {
		return fmt.Errorf("framework integration does not support target %q; use rendered targets for generated app package tests", target)
	}
	baselineTags, integrationTags, err := frameworkProfileTags(profile)
	if err != nil {
		return err
	}
	frameworkEnv := map[string]string{
		"FORJ_INTEGRATION_FORJ_PATH": executor.forjExec,
	}
	targets, selectedTests, err := executor.frameworkIntegrationTestTargets(baselineTags, integrationTags, shard)
	if err != nil {
		return err
	}
	if !executor.silent {
		console.Infof("framework %s shard %d/%d: %d integration tests", profile, shard.number, shard.total, selectedTests)
	}
	if !executor.silent {
		testkit.PrintSubsection("Framework integration tests")
	}
	for _, testTarget := range targets {
		args := []string{
			"go", "test", "-tags=" + integrationTags, testTarget.packagePath,
			"-run", makeIntegrationTestPattern(testTarget.testNames), "-count=1",
		}
		if executor.verbose {
			args = append(args, "-v")
		}
		stepName := "framework " + testTarget.packagePath
		if err := executor.runStep(".", integrationStep{name: stepName, args: args}, frameworkEnvironment(frameworkEnv)); err != nil {
			return err
		}
	}
	return nil
}

// frameworkIntegrationTestTargets discovers tests added by one tag layer and selects one deterministic shard.
func (executor integrationExecutor) frameworkIntegrationTestTargets(baselineTags, integrationTags string, shard frameworkShard) ([]integrationTestTarget, int, error) {
	baseline, err := executor.listFrameworkTests(baselineTags)
	if err != nil {
		return nil, 0, err
	}
	integration, err := executor.listFrameworkTests(integrationTags)
	if err != nil {
		return nil, 0, err
	}
	testNames, err := integrationOnlyTestNames(baseline, integration)
	if err != nil {
		return nil, 0, err
	}
	return selectIntegrationTestTargets(testNames, shard)
}

// selectIntegrationTestTargets partitions package-qualified test names and gives each top-level test an independent timeout budget.
func selectIntegrationTestTargets(testNames []integrationTestName, shard frameworkShard) ([]integrationTestTarget, int, error) {
	if shard.total > len(testNames) {
		return nil, 0, fmt.Errorf("framework shard count %d exceeds %d integration tests", shard.total, len(testNames))
	}
	targets := make([]integrationTestTarget, 0, (len(testNames)+shard.total-1)/shard.total)
	for index, testName := range testNames {
		if index%shard.total == shard.number-1 {
			targets = append(targets, integrationTestTarget{packagePath: testName.packagePath, testNames: []string{testName.name}})
		}
	}
	return targets, len(targets), nil
}

// makeIntegrationTestPattern produces an exact Go test filter for one package's selected top-level tests.
func makeIntegrationTestPattern(testNames []string) string {
	quoted := make([]string, 0, len(testNames))
	for _, testName := range testNames {
		quoted = append(quoted, regexp.QuoteMeta(testName))
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

// listFrameworkTests asks the Go tool which runnable tests belong to one framework build-tag selection.
func (executor integrationExecutor) listFrameworkTests(tags string) ([]integrationTestName, error) {
	repoRoot, err := testkit.RepoRoot()
	if err != nil {
		return nil, fmt.Errorf("resolve repository root for framework tests: %w", err)
	}
	return executor.listGoTests(repoRoot, "./internal/forj/...", tags)
}

// listGoTests returns the package-qualified names that go test itself considers runnable.
// Packages in scope must keep init and TestMain output free of test-shaped standalone lines because Go reports names as output events.
func (executor integrationExecutor) listGoTests(dir, packagePattern, tags string) ([]integrationTestName, error) {
	args := []string{"test", "-json", "-vet=off", "-list=^(Test|Fuzz|Example)"}
	if tags != "" {
		args = append(args, "-tags="+tags)
	}
	args = append(args, packagePattern)
	command := exec.Command("go", args...)
	command.Dir = dir
	command.Env = testkit.WithEnvOverrides(os.Environ(), map[string]string{
		"GOMODCACHE": executor.caches.ModulePath,
		"GOCACHE":    executor.caches.BuildPath,
		"GOFLAGS":    "",
		"GOWORK":     "off",
	})
	output, err := runFrameworkTestListCommand(command)
	if err != nil {
		return nil, fmt.Errorf("list Go tests for %s with tags %q: %w", packagePattern, tags, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	tests := make([]integrationTestName, 0)
	for {
		var event goTestListEvent
		if err := decoder.Decode(&event); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode Go test list for %s with tags %q: %w", packagePattern, tags, err)
		}
		if event.Action != "output" || event.Package == "" {
			continue
		}
		if name, ok := listedGoTestName(event.Output); ok {
			tests = append(tests, integrationTestName{packagePath: event.Package, name: name})
		}
	}
	sort.Slice(tests, func(left, right int) bool {
		if tests[left].packagePath == tests[right].packagePath {
			return tests[left].name < tests[right].name
		}
		return tests[left].packagePath < tests[right].packagePath
	})
	return tests, nil
}

// runFrameworkTestListCommand keeps Go diagnostics on stderr from corrupting the JSON stdout stream.
func runFrameworkTestListCommand(command *exec.Cmd) ([]byte, error) {
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if err == nil {
		return output, nil
	}
	details := make([]string, 0, 2)
	if stdout := strings.TrimSpace(string(output)); stdout != "" {
		details = append(details, stdout)
	}
	if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
		details = append(details, diagnostic)
	}
	if len(details) == 0 {
		return nil, err
	}
	return nil, fmt.Errorf("%w (%s)", err, strings.Join(details, "\n"))
}

// listedGoTestName recognizes the runnable-name lines emitted by go test -list.
func listedGoTestName(output string) (string, bool) {
	name := strings.TrimSpace(output)
	if name == "" || name == "TestMain" || strings.ContainsAny(name, " \t\r\n") {
		return "", false
	}
	for _, prefix := range []string{"Test", "Fuzz", "Example"} {
		if strings.HasPrefix(name, prefix) {
			return name, true
		}
	}
	return "", false
}

// integrationOnlyTestNames subtracts the runnable baseline while preserving tagged duplicate-name additions.
func integrationOnlyTestNames(baseline, integration []integrationTestName) ([]integrationTestName, error) {
	baselineCounts := make(map[integrationTestName]int, len(baseline))
	for _, name := range baseline {
		baselineCounts[name]++
	}
	integrationCounts := make(map[integrationTestName]int, len(integration))
	for _, name := range integration {
		integrationCounts[name]++
	}
	names := make([]integrationTestName, 0, len(integrationCounts))
	for name, count := range integrationCounts {
		if count > baselineCounts[name] {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("integration build tags did not add framework tests")
	}
	sort.Slice(names, func(left, right int) bool {
		if names[left].packagePath == names[right].packagePath {
			return names[left].name < names[right].name
		}
		return names[left].packagePath < names[right].packagePath
	})
	return names, nil
}

// runRenderedSuite runs generated application tests across the selected database variants.
func (cmd *TestIntegrationCmd) runRenderedSuite(executor integrationExecutor, target, variant string) error {
	var variants []string
	switch variant {
	case "", "all":
		variants = []string{"sqlite", "mysql", "postgres"}
	case "sqlite", "mysql", "postgres":
		variants = []string{variant}
	default:
		return fmt.Errorf("unsupported rendered integration variant %q", variant)
	}

	for _, dbVariant := range variants {
		if !executor.silent {
			testkit.PrintSubsection(fmt.Sprintf("Rendered integration variant: %s", dbVariant))
		}
		if err := cmd.runRenderedVariant(executor, dbVariant, target); err != nil {
			return err
		}
	}

	if !executor.silent {
		console.Successf("Integration tests completed")
	}
	return nil
}

// cloneStringMap prevents runtime container overrides from mutating the reusable variant catalog.
func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

// writeRenderedIntegrationConfig publishes the minimal generated App needed to exercise one database variant.
func (cmd *TestIntegrationCmd) writeRenderedIntegrationConfig(dir, variant string, spec dbIntegrationVariantSpec) error {
	cfg := project.Config{
		ProjectName:  "Integration" + strings.ToUpper(variant[:1]) + variant[1:],
		GoModuleName: "github.com/test/project",
		UpdatedAt:    "2026-01-01 00:00:00 UTC",
		Render: project.RenderConfig{
			GoForjVersion: version.Semver(),
			Components: project.Components{
				CLI:    true,
				WebAPI: true,
				Auth:   true,
				Docker: true,
			},
		},
	}
	if spec.applyConfig != nil {
		spec.applyConfig(&cfg.Render.Components)
	}
	return testkit.WriteProjectConfig(filepath.Join(dir, ".goforj.yml"), cfg)
}

// renderedIntegrationSteps maps a user target to deterministic generated-package test commands.
func renderedIntegrationSteps(tag, target string) ([]integrationStep, error) {
	all := []integrationStep{
		{name: "auth", args: []string{"go", "test", "./internal/auth", "-tags=integration," + tag}},
		{name: "makecmd", args: []string{"go", "test", "./internal/makecmd", "-tags=integration," + tag}},
		{name: "migrations", args: []string{"go", "test", "./migrations", "-tags=integration," + tag}},
		{name: "database", args: []string{"go", "test", "./internal/database", "-tags=integration," + tag}},
	}
	target = str.Of(target).ToLower().Trim().String()
	if target == "modelgen" {
		target = "makecmd"
	}
	if target == "" || target == "all" {
		return all, nil
	}
	for _, step := range all {
		if step.name == target {
			return []integrationStep{step}, nil
		}
	}
	return nil, fmt.Errorf("unknown rendered integration target %q", target)
}

// runRenderedTaggedTests runs each selected generated-package test through the shared integration environment.
func (executor integrationExecutor) runRenderedTaggedTests(dir, tag, target string, extraEnv map[string]string) error {
	steps, err := renderedIntegrationSteps(tag, target)
	if err != nil {
		return err
	}
	for _, step := range steps {
		step.args = append([]string{}, step.args...)
		if executor.verbose {
			step.args = append(step.args, "-v")
		}
		if !executor.silent {
			testkit.PrintSubsection(fmt.Sprintf("%s integration tests", step.name))
		}
		if err := executor.runStep(dir, step, extraEnv); err != nil {
			return err
		}
	}
	return nil
}

// runRenderedVariant renders and tests one database-specific application workspace.
func (cmd *TestIntegrationCmd) runRenderedVariant(executor integrationExecutor, variant, target string) error {
	spec, ok := dbIntegrationVariantSpecs[variant]
	if !ok {
		return fmt.Errorf("unknown variant %q (expected mysql, postgres, or sqlite)", variant)
	}

	tempRoot, err := testkit.TempRoot("FORJ_DB_INTEGRATION_TMPDIR")
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(tempRoot, "forj_db_integration_")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if !executor.silent {
		testkit.PrintSection(fmt.Sprintf("Rendered App Integration: %s", variant))
		console.Infof("workspace: %s", tempDir)
	}

	if err := cmd.writeRenderedIntegrationConfig(tempDir, variant, spec); err != nil {
		return err
	}
	if err := executor.workspace(tempDir).Run("render", executor.forjExec, "render"); err != nil {
		return err
	}

	testEnv := cloneStringMap(spec.testEnv)
	if err := applyRenderedIntegrationEnvironment(tempDir, testEnv); err != nil {
		return err
	}
	stack, err := testkit.StartRenderedComposeServices(tempDir, testkit.ConsoleLogf(executor.silent))
	if err != nil {
		return err
	}
	defer stack.Stop()
	if err := stack.ApplyHostEnvOverrides([]string{filepath.Join(tempDir, ".env")}); err != nil {
		return err
	}
	for key, value := range stack.EnvOverrides() {
		testEnv[key] = value
	}

	if err := executor.runRenderedTaggedTests(tempDir, variant, target, testEnv); err != nil {
		return err
	}

	if !executor.silent {
		console.Successf("DB integration tests completed (%s)", variant)
	}
	return nil
}

// applyRenderedIntegrationEnvironment publishes service-level test overrides before containers start.
func applyRenderedIntegrationEnvironment(projectDir string, testEnv map[string]string) error {
	timezone := testEnv["TZ"]
	if timezone == "" {
		return nil
	}
	// A non-UTC container environment keeps host assumptions visible while the
	// database assertions verify the generated session contract directly.
	return testkit.ReplaceOrAppendEnvValues(
		[]string{filepath.Join(projectDir, ".env")},
		map[string]string{"TZ": timezone},
	)
}

// runStep delegates streaming integration commands to the shared workspace execution policy.
func (executor integrationExecutor) runStep(dir string, step integrationStep, environment map[string]string) error {
	args := ensureGoTestVerbose(step.args)
	return executor.workspace(dir).RunStreaming(testexec.StreamingStep{
		Name:        step.name,
		Command:     args,
		Environment: environment,
	})
}

// ensureGoTestVerbose keeps long integration runs observable even when the command flag is omitted.
func ensureGoTestVerbose(args []string) []string {
	if len(args) < 2 || args[0] != "go" || args[1] != "test" {
		return args
	}
	for _, arg := range args[2:] {
		if arg == "-v" {
			return args
		}
	}
	updated := append([]string{}, args[:2]...)
	updated = append(updated, "-v")
	updated = append(updated, args[2:]...)
	return updated
}

// frameworkEnvironment isolates framework tests from caller Go flags and workspaces while preserving step-specific overrides.
func frameworkEnvironment(overrides map[string]string) map[string]string {
	environment := map[string]string{
		"GOFLAGS": "",
		"GOWORK":  "off",
	}
	for key, value := range overrides {
		environment[key] = value
	}
	return environment
}

// workspace binds integration commands to the executor's shared policy.
func (executor integrationExecutor) workspace(dir string) *testexec.Workspace {
	return testexec.NewWorkspace(executor.logger, executor.silent, dir, executor.caches)
}

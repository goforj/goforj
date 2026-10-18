package atlas

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/goforj/atlas/agents"
	"github.com/goforj/atlas/eval"
	"github.com/goforj/atlas/skills"
	"github.com/goforj/goforj/internal/forj/atlaseval"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
)

const (
	evaluationArtifactRootMarker      = ".atlas-evaluation-artifacts"
	evaluationArtifactRootIdentity    = "goforj-atlas-evaluation-artifacts/v1\n"
	evaluationWorkRootMarker          = ".atlas-evaluation-work"
	evaluationWorkRootIdentity        = "goforj-atlas-evaluation-work/v1\n"
	evaluationWorkRootLease           = ".lease"
	evaluationStaleWorkRootScanLimit  = 1024
	evaluationStaleWorkRootPruneLimit = 8
	evaluationStaleWorkQuarantine     = ".prune-"
	maxEvaluationCredentialSize       = 1 << 20
	maxEvaluationArtifactKeySize      = 64 << 10
	// Atlas bounds each candidate tree at 2 GiB. A treatment can retain its candidate,
	// sealed input, and one verifier clone simultaneously; private Go caches need a
	// separate allowance. Concurrent workers can materialize distinct command-owned
	// bases, while idle retention remains bounded independently.
	evaluationCandidateTreeBytes  = uint64(2 << 30)
	evaluationTreatmentTreeCopies = uint64(3)
	evaluationPrivateCacheBytes   = uint64(1 << 30)
	evaluationSharedCacheBytes    = uint64(1 << 30)
	evaluationPrivateCacheCopies  = uint64(3)
)

// EvalCmd groups opt-in live-agent evaluation commands.
type EvalCmd struct {
	Compare  EvalCompareCmd  `cmd:""`
	Coverage EvalCoverageCmd `cmd:""`
	List     EvalListCmd     `cmd:""`
	Report   EvalReportCmd   `cmd:""`
	Run      EvalRunCmd      `cmd:""`
	Suite    EvalSuiteCmd    `cmd:""`
}

// EvalCoverageCmd reports which framework capabilities have promoted evaluation coverage.
type EvalCoverageCmd struct {
	Format string `help:"Output format" enum:"table,markdown,json" default:"table"`
}

// Signature returns the Kong metadata for EvalCoverageCmd.
func (*EvalCoverageCmd) Signature() string {
	return `name:"coverage" help:"Report promoted evaluation coverage and planned gaps"`
}

// Run prints the versioned Atlas coverage catalog without starting an agent.
func (command *EvalCoverageCmd) Run() error {
	catalog, err := eval.LoadCoverageCatalog()
	if err != nil {
		return err
	}
	switch command.Format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(catalog)
	case "markdown":
		printEvaluationCoverageMarkdown(os.Stdout, catalog)
	default:
		printEvaluationCoverageTable(os.Stdout, catalog)
	}
	return nil
}

// printEvaluationCoverageTable renders a compact terminal inventory with planned gaps visible in place.
func printEvaluationCoverageTable(writer io.Writer, catalog eval.CoverageCatalog) {
	areaWidth := len("area")
	capabilityWidth := len("capability")
	covered := 0
	for _, capability := range catalog.Capabilities {
		areaWidth = max(areaWidth, len(capability.Area))
		capabilityWidth = max(capabilityWidth, len(capability.ID))
		if capability.Covered() {
			covered++
		}
	}
	for _, capability := range catalog.Capabilities {
		status := "planned"
		if capability.Covered() {
			status = "covered"
		}
		fmt.Fprintf(writer, "%-*s  %-*s  %-8s  %-8s  %s\n", areaWidth, capability.Area, capabilityWidth, capability.ID, capability.Tier, status, strings.Join(capability.Evaluations, ", "))
	}
	fmt.Fprintf(writer, "\nCoverage · %d/%d capabilities · %d planned gaps\n", covered, len(catalog.Capabilities), len(catalog.Capabilities)-covered)
}

// printEvaluationCoverageMarkdown renders a reviewable report suitable for checked-in benchmark notes and pull requests.
func printEvaluationCoverageMarkdown(writer io.Writer, catalog eval.CoverageCatalog) {
	fmt.Fprintln(writer, "| Area | Capability | Tier | Status | Evaluations |")
	fmt.Fprintln(writer, "| --- | --- | --- | --- | --- |")
	for _, capability := range catalog.Capabilities {
		status := "Planned"
		if capability.Covered() {
			status = "Covered"
		}
		fmt.Fprintf(writer, "| %s | `%s` | %s | %s | %s |\n", capability.Area, capability.ID, capability.Tier, status, strings.Join(capability.Evaluations, ", "))
	}
}

// Signature returns the Kong metadata for EvalCmd.
func (*EvalCmd) Signature() string {
	return `name:"atlas:eval" help:"Run opt-in Atlas agent evaluations"`
}

// EvalCompareCmd runs one explicit ordered guidance comparison.
type EvalCompareCmd struct {
	Evaluation      string `arg:"" help:"Promoted evaluation ID" default:"add-http-controller"`
	Model           string `help:"Exact Codex model identity" required:""`
	ModelProvider   string `name:"model-provider" help:"Codex model provider" default:"openai"`
	CodexExecutable string `name:"codex" help:"Codex executable or PATH name" default:"codex"`
	Credential      string `help:"Disposable, revocable Codex auth.json source for this unconfined diagnostic" type:"path" required:""`
	Artifacts       string `help:"Supervisor-owned artifact directory" type:"path"`
	ArtifactKey     string `name:"artifact-key" help:"External manifest authentication key; must be outside artifacts" type:"path" required:""`
	Trials          int    `help:"Independent paired trials" default:"1"`
	Control         string `help:"First guidance treatment" enum:"none,agents,agents-skills,atlas" default:"none"`
	Treatment       string `help:"Second guidance treatment" enum:"none,agents,agents-skills,atlas" default:"agents"`
}

// EvalRunCmd runs one promoted diagnostic treatment without paying for an unused comparison profile.
type EvalRunCmd struct {
	Evaluation      string `arg:"" help:"Promoted evaluation ID" default:"add-http-controller"`
	Guidance        string `help:"Guidance profile" enum:"none,agents,agents-skills,atlas" default:"agents"`
	Model           string `help:"Exact Codex model identity" required:""`
	ModelProvider   string `name:"model-provider" help:"Codex model provider" default:"openai"`
	CodexExecutable string `name:"codex" help:"Codex executable or PATH name" default:"codex"`
	Credential      string `help:"Disposable, revocable Codex auth.json source for this unconfined diagnostic" type:"path" required:""`
	Artifacts       string `help:"Supervisor-owned artifact directory" type:"path"`
	ArtifactKey     string `name:"artifact-key" help:"External manifest authentication key; must be outside artifacts" type:"path" required:""`
	Trials          int    `help:"Independent trials" default:"1"`
}

// EvalReportCmd authenticates and prints one retained attempt summary.
type EvalReportCmd struct {
	Directory   string `arg:"" help:"Retained attempt artifact directory" type:"path"`
	ArtifactKey string `name:"artifact-key" help:"External manifest authentication key" type:"path" required:""`
}

// EvalListCmd lists the promoted evaluation catalog without starting an agent.
type EvalListCmd struct{}

// Signature returns the Kong metadata for EvalListCmd.
func (*EvalListCmd) Signature() string {
	return `name:"list" help:"List promoted Atlas evaluations"`
}

// Run prints stable evaluation IDs with their suite and purpose.
func (*EvalListCmd) Run() error {
	ids, err := eval.PromotedEvaluationIDs("")
	if err != nil {
		return err
	}
	for _, id := range ids {
		definition, err := eval.LoadPromotedDefinition(id)
		if err != nil {
			return err
		}
		fmt.Printf("%-28s  %-10s  %-8s  %s\n", definition.ID, definition.TaskKind, definition.Suite, definition.Summary)
	}
	return nil
}

// Signature returns the Kong metadata for EvalRunCmd.
func (*EvalRunCmd) Signature() string {
	return `name:"run" help:"Run one promoted diagnostic treatment"`
}

// Run executes one selected guidance profile through a fresh agent session.
func (command *EvalRunCmd) Run() (runErr error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runEvaluationTreatments(ctx, evaluationInvocation{
		Model: command.Model, ModelProvider: command.ModelProvider, CodexExecutable: command.CodexExecutable,
		Credential: command.Credential, Artifacts: command.Artifacts, ArtifactKey: command.ArtifactKey,
	}, command.Evaluation, command.Guidance, command.Trials)
}

// Signature returns the Kong metadata for EvalReportCmd.
func (*EvalReportCmd) Signature() string {
	return `name:"report" help:"Authenticate and print one retained evaluation summary"`
}

// Run verifies the complete retained attempt before rendering its human summary.
func (command *EvalReportCmd) Run() error {
	directory, err := filepath.Abs(command.Directory)
	if err != nil {
		return err
	}
	key, err := readEvalArtifactKey(command.ArtifactKey, filepath.Dir(directory))
	if err != nil {
		return err
	}
	summary, _, err := eval.ReadVerifiedAttemptSummary(directory, key)
	if err != nil {
		return err
	}
	fmt.Print(escapeEvaluationTerminalText(summary))
	return nil
}

// escapeEvaluationTerminalText renders authenticated candidate-derived text without allowing terminal control or format sequences to execute.
func escapeEvaluationTerminalText(value string) string {
	var output strings.Builder
	for _, character := range value {
		if character == '\n' || character == '\t' || !unicode.IsControl(character) && !unicode.Is(unicode.Cf, character) {
			output.WriteRune(character)
			continue
		}
		fmt.Fprintf(&output, "\\u%04x", character)
	}
	return output.String()
}

// EvalSuiteCmd runs every promoted evaluation in one suite with frozen authority and bounded concurrent diagnostics.
type EvalSuiteCmd struct {
	Suite           string `arg:"" help:"Promoted evaluation suite" default:"core"`
	Kind            string `help:"Limit the suite to one measurement kind" enum:"all,scaffold,feature,repair,abstention" default:"all"`
	Tier            string `help:"Limit the suite to a cumulative capability tier" enum:"all,smoke,core,extended" default:"all"`
	Model           string `help:"Exact Codex model identity" required:""`
	ModelProvider   string `name:"model-provider" help:"Codex model provider" default:"openai"`
	CodexExecutable string `name:"codex" help:"Codex executable or PATH name" default:"codex"`
	Credential      string `help:"Disposable, revocable Codex auth.json source for this unconfined diagnostic" type:"path" required:""`
	Artifacts       string `help:"Supervisor-owned artifact directory" type:"path"`
	ArtifactKey     string `name:"artifact-key" help:"External manifest authentication key; must be outside artifacts" type:"path" required:""`
	Trials          int    `help:"Independent paired trials per evaluation" default:"1"`
	Workers         int    `help:"Concurrent diagnostic executions; does not provide security isolation" default:"1"`
	Control         string `help:"First guidance treatment" enum:"none,agents,agents-skills,atlas" default:"none"`
	Treatment       string `help:"Second guidance treatment" enum:"none,agents,agents-skills,atlas" default:"agents"`
}

// Signature returns the Kong metadata for EvalSuiteCmd.
func (*EvalSuiteCmd) Signature() string {
	return `name:"suite" help:"Run every promoted evaluation in a suite"`
}

// Run executes every suite member through one frozen credential, one command-owned immutable base cache, and worker-private writable state.
func (command *EvalSuiteCmd) Run() (runErr error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ids, err := eval.PromotedEvaluationIDsMatching(eval.EvaluationFilter{
		Suite:    command.Suite,
		TaskKind: evaluationTaskKind(command.Kind),
	})
	if err != nil {
		return err
	}
	ids, err = evaluationIDsForTier(ids, command.Tier)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("evaluation suite %q with kind %q is empty or unknown", command.Suite, command.Kind)
	}
	plan, err := buildEvaluationSuitePlan(ids, command.Trials)
	if err != nil {
		return err
	}
	printEvaluationSuitePlan(os.Stderr, plan)
	return runEvaluationComparisonsWithProfilesWorkers(ctx, evaluationInvocation{
		Model: command.Model, ModelProvider: command.ModelProvider, CodexExecutable: command.CodexExecutable,
		Credential: command.Credential, Artifacts: command.Artifacts, ArtifactKey: command.ArtifactKey, Progress: os.Stderr,
	}, ids, command.Trials, command.Workers, []string{command.Control, command.Treatment})
}

// evaluationIDsForTier intersects a promoted suite with the capability catalog's cumulative portfolio.
func evaluationIDsForTier(ids []string, tier string) ([]string, error) {
	if tier == "all" {
		return append([]string(nil), ids...), nil
	}
	catalog, err := eval.LoadCoverageCatalog()
	if err != nil {
		return nil, err
	}
	covered, err := catalog.EvaluationIDs(eval.CoverageTier(tier))
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(covered))
	for _, id := range covered {
		allowed[id] = true
	}
	selected := make([]string, 0, len(ids))
	for _, id := range ids {
		if allowed[id] {
			selected = append(selected, id)
		}
	}
	return selected, nil
}

// evaluationSuitePlan makes the cost-bearing scope of one explicit suite invocation visible before authority is loaded.
type evaluationSuitePlan struct {
	evaluations int
	attempts    int
	wallTime    time.Duration
}

// buildEvaluationSuitePlan totals the manifest budgets without inventing provider-specific cost estimates.
func buildEvaluationSuitePlan(evaluationIDs []string, trials int) (evaluationSuitePlan, error) {
	if err := validateEvaluationTrials(trials); err != nil {
		return evaluationSuitePlan{}, err
	}
	plan := evaluationSuitePlan{
		evaluations: len(evaluationIDs),
		attempts:    len(evaluationIDs) * trials * 2,
	}
	for _, evaluationID := range evaluationIDs {
		definition, err := eval.LoadPromotedDefinition(evaluationID)
		if err != nil {
			return evaluationSuitePlan{}, err
		}
		plan.wallTime += definition.Limits.WallTime * time.Duration(trials*2)
	}
	return plan, nil
}

// printEvaluationSuitePlan reports the bounded work without turning an explicit suite command into another confirmation workflow.
func printEvaluationSuitePlan(writer io.Writer, plan evaluationSuitePlan) {
	fmt.Fprintf(writer, "Evaluation suite · %d evaluations · %d agent sessions · up to %s\n", plan.evaluations, plan.attempts, plan.wallTime)
}

// evaluationTaskKind converts the CLI's explicit all selection into an unrestricted catalog filter.
func evaluationTaskKind(kind string) eval.EvaluationTaskKind {
	if kind == "all" {
		return ""
	}
	return eval.EvaluationTaskKind(kind)
}

// evaluationInvocation contains the authority and runtime options shared by compare and suite execution.
type evaluationInvocation struct {
	Model           string
	ModelProvider   string
	CodexExecutable string
	Credential      string
	Artifacts       string
	ArtifactKey     string
	Progress        io.Writer
	PairWorkers     int
}

// Signature returns the Kong metadata for EvalCompareCmd.
func (*EvalCompareCmd) Signature() string {
	return `name:"compare" help:"Compare two guidance profiles"`
}

// Run executes two fresh diagnostic sessions and prints their machine-readable results.
func (command *EvalCompareCmd) Run() (runErr error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return command.run(ctx)
}

// run executes the comparison with a caller-owned lifecycle so cancellation reaches every evaluation resource.
func (command *EvalCompareCmd) run(ctx context.Context) (runErr error) {
	return runEvaluationComparisonsWithProfilesWorkers(ctx, evaluationInvocation{
		Model: command.Model, ModelProvider: command.ModelProvider, CodexExecutable: command.CodexExecutable,
		Credential: command.Credential, Artifacts: command.Artifacts, ArtifactKey: command.ArtifactKey,
	}, []string{command.Evaluation}, command.Trials, 1, []string{command.Control, command.Treatment})
}

// runEvaluationComparisons shares frozen credentials, tool resolution, caches, and verifier infrastructure across one invocation.
func runEvaluationComparisons(ctx context.Context, invocation evaluationInvocation, evaluationIDs []string, trials int) (runErr error) {
	return runEvaluationComparisonsWithWorkers(ctx, invocation, evaluationIDs, trials, 1)
}

// runEvaluationComparisonsWithWorkers schedules complete paired trials across separate diagnostic executions.
func runEvaluationComparisonsWithWorkers(ctx context.Context, invocation evaluationInvocation, evaluationIDs []string, trials, workers int) (runErr error) {
	return runEvaluationComparisonsWithProfilesWorkers(ctx, invocation, evaluationIDs, trials, workers, []string{eval.GuidanceProfileNone, eval.GuidanceProfileAgents})
}

// runEvaluationComparisonsWithProfilesWorkers schedules explicit ordered treatment pairs across isolated workers.
func runEvaluationComparisonsWithProfilesWorkers(ctx context.Context, invocation evaluationInvocation, evaluationIDs []string, trials, workers int, profiles []string) (runErr error) {
	profiles, err := evaluationProfiles(profiles)
	if err != nil {
		return err
	}
	if len(profiles) != 2 || profiles[0] == profiles[1] {
		return fmt.Errorf("evaluation comparison requires two distinct guidance profiles")
	}
	if err := validateEvaluationTrials(trials); err != nil {
		return err
	}
	if err := validateEvaluationWorkers(workers); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateEvaluationIDs(evaluationIDs); err != nil {
		return err
	}
	jobs := evaluationComparisonJobs(evaluationIDs, trials)
	if len(jobs) == 0 {
		return fmt.Errorf("at least one evaluation is required")
	}
	workers = effectiveEvaluationWorkers(workers, len(jobs))
	invocation.PairWorkers = workers
	if err := validateEvaluationFileCreationMode(); err != nil {
		return err
	}
	if err := ensureEvaluationDiskCapacity(workers); err != nil {
		return err
	}
	printEvaluationScratchAdmission(invocation.Progress, workers)
	authority, err := newEvaluationAuthority(invocation)
	if err != nil {
		return err
	}
	executions := make([]*evaluationExecution, 0, workers)
	for range workers {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, closeEvaluationExecutions(executions), authority.Close())
		}
		execution, err := newEvaluationExecution(invocation, authority, profiles)
		if err != nil {
			return errors.Join(err, closeEvaluationExecutions(executions), authority.Close())
		}
		executions = append(executions, execution)
		if err := ctx.Err(); err != nil {
			return errors.Join(err, closeEvaluationExecutions(executions), authority.Close())
		}
	}
	defer func() {
		runErr = errors.Join(runErr, closeEvaluationExecutions(executions), authority.Close())
	}()
	results, attemptErrors := runEvaluationComparisonJobs(ctx, invocation.Progress, jobs, len(executions), trials, func(ctx context.Context, job evaluationComparisonJob, worker int) (eval.GuidanceDiagnosticResult, []error) {
		return runEvaluationComparisonJob(ctx, executions[worker], authority.tools, job, profiles)
	})
	if err := printEvaluationResults(results, authority.redactor, authority.artifactRoot); err != nil {
		return err
	}
	return errors.Join(attemptErrors...)
}

// effectiveEvaluationWorkers prevents resource planning from charging for workers that have no job to execute.
func effectiveEvaluationWorkers(requested, jobs int) int {
	return min(requested, jobs)
}

// validateEvaluationFileCreationMode prevents the invoking shell from changing candidate permissions and therefore the measured diff.
func validateEvaluationFileCreationMode() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.MkdirTemp("", "goforj-evaluation-mode-*")
	if err != nil {
		return fmt.Errorf("create evaluation file-mode probe directory: %w", err)
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "probe")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666)
	if err != nil {
		return fmt.Errorf("create evaluation file-mode probe: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close evaluation file-mode probe: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect evaluation file-mode probe: %w", err)
	}
	return validateObservedEvaluationFileMode(info.Mode().Perm())
}

// validateObservedEvaluationFileMode requires the conventional umask used by generated Project fixtures and framework commands.
func validateObservedEvaluationFileMode(mode os.FileMode) error {
	if mode == 0o644 {
		return nil
	}
	return fmt.Errorf("evaluation requires an effective umask of 022 so generated file permissions remain reproducible; observed mode %04o (run `umask 022` before retrying)", mode)
}

// validateEvaluationIDs resolves every promoted contract before capacity checks or command-owned filesystem mutation.
func validateEvaluationIDs(evaluationIDs []string) error {
	for _, evaluationID := range evaluationIDs {
		if _, err := evaluationJobRoot(".", evaluationID, 1); err != nil {
			return err
		}
	}
	return nil
}

// ensureEvaluationDiskCapacity fails before authority or agents start when the bounded candidate copies and private caches cannot fit safely.
func ensureEvaluationDiskCapacity(workers int) error {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("resolve evaluation work-state cache: %w", err)
	}
	return ensureEvaluationDiskCapacityAt(cacheRoot, workers, availableEvaluationDiskBytes)
}

// ensureEvaluationDiskCapacityAt recovers bounded abandoned command roots before measuring the volume that will hold new evaluation state.
func ensureEvaluationDiskCapacityAt(cacheRoot string, workers int, availableBytes func(string) (uint64, error)) error {
	workStateRoot, err := resolveEvaluationWorkStateRootAt(cacheRoot)
	if err != nil {
		return err
	}
	if err := recoverStaleEvaluationWorkRoots(workStateRoot); err != nil {
		return err
	}
	available, err := availableBytes(workStateRoot)
	if err != nil {
		return fmt.Errorf("inspect evaluation work-state capacity: %w", err)
	}
	return validateEvaluationDiskCapacity(workStateRoot, available, workers)
}

// evaluationScratchBudget estimates peak scratch from bounded prepared bases plus the none, agents, and verifier trees and caches each worker can retain at once.
func evaluationScratchBudget(workers int) uint64 {
	shared := evaluationPreparedBaseTreeCopies(workers)*evaluationCandidateTreeBytes + evaluationSharedCacheBytes
	perWorker := evaluationTreatmentTreeCopies*evaluationCandidateTreeBytes + evaluationPrivateCacheCopies*evaluationPrivateCacheBytes
	return shared + uint64(workers)*perWorker
}

// evaluationPreparedBaseTreeCopies charges one retained base per worker plus the single serialized base that may be materializing before eviction.
func evaluationPreparedBaseTreeCopies(workers int) uint64 {
	return uint64(workers + 1)
}

// printEvaluationScratchAdmission makes the conservative peak scratch estimate visible before workers begin; it is not a filesystem reservation.
func printEvaluationScratchAdmission(writer io.Writer, workers int) {
	if writer == nil {
		return
	}
	shared := evaluationPreparedBaseTreeCopies(workers)*evaluationCandidateTreeBytes + evaluationSharedCacheBytes
	perWorker := evaluationTreatmentTreeCopies*evaluationCandidateTreeBytes + evaluationPrivateCacheCopies*evaluationPrivateCacheBytes
	fmt.Fprintf(writer, "Evaluation scratch estimate · %d workers · up to %d GiB shared + %d GiB per worker = %d GiB peak\n", workers, shared>>30, perWorker>>30, evaluationScratchBudget(workers)>>30)
}

// validateEvaluationDiskCapacity applies the conservative aggregate scratch budget independently from platform filesystem calls.
func validateEvaluationDiskCapacity(cacheRoot string, available uint64, workers int) error {
	required := evaluationScratchBudget(workers)
	if available < required {
		return fmt.Errorf("evaluation scratch estimate requires at least %d GiB free in %s; %.2f GiB is available (reduce --workers or free disk space)", required>>30, cacheRoot, float64(available)/(1<<30))
	}
	return nil
}

// closeEvaluationExecutions releases every successfully constructed worker before returning a setup error.
func closeEvaluationExecutions(executions []*evaluationExecution) error {
	var closeErr error
	for _, execution := range executions {
		closeErr = errors.Join(closeErr, execution.Close())
	}
	return closeErr
}

// evaluationComparisonJob is one complete control-versus-guidance pairing, which must never be split between workers.
type evaluationComparisonJob struct {
	evaluationID string
	trial        int
}

// evaluationComparisonJobs retains catalog and trial order independently of worker completion order.
func evaluationComparisonJobs(evaluationIDs []string, trials int) []evaluationComparisonJob {
	jobs := make([]evaluationComparisonJob, 0, len(evaluationIDs)*trials)
	for _, evaluationID := range evaluationIDs {
		for trial := 1; trial <= trials; trial++ {
			jobs = append(jobs, evaluationComparisonJob{evaluationID: evaluationID, trial: trial})
		}
	}
	return jobs
}

// runEvaluationComparisonJobs stores each worker outcome at its planned position, keeping command output reproducible.
func runEvaluationComparisonJobs(ctx context.Context, progress io.Writer, jobs []evaluationComparisonJob, workerCount, trials int, run func(context.Context, evaluationComparisonJob, int) (eval.GuidanceDiagnosticResult, []error)) ([]eval.GuidanceDiagnosticResult, []error) {
	commandContext, stop := context.WithCancel(ctx)
	defer stop()
	results := make([]eval.GuidanceDiagnosticResult, len(jobs))
	completed := make([]bool, len(jobs))
	errorsByJob := make([][]error, len(jobs))
	reporter := newEvaluationProgress(progress, len(jobs), trials)
	runEvaluationComparisonSchedule(commandContext, jobs, workerCount, func(index int, job evaluationComparisonJob, worker int) {
		reporter.started(job)
		result, jobErrors := run(commandContext, job, worker)
		results[index] = result
		completed[index] = true
		errorsByJob[index] = jobErrors
		reporter.finished(job)
		if evaluationCommandFatal(jobErrors) {
			stop()
		}
	})
	orderedResults := make([]eval.GuidanceDiagnosticResult, 0, len(jobs))
	var attemptErrors []error
	for index, jobErrors := range errorsByJob {
		if completed[index] {
			orderedResults = append(orderedResults, results[index])
		}
		attemptErrors = append(attemptErrors, jobErrors...)
	}
	if ctx.Err() != nil {
		attemptErrors = append(attemptErrors, ctx.Err())
	}
	return orderedResults, attemptErrors
}

// evaluationCommandFatal reports failures that invalidate the shared execution surface or make further writes unsafe.
func evaluationCommandFatal(causes []error) bool {
	for _, err := range causes {
		var fatal evaluationCommandFatalError
		if errors.As(err, &fatal) || eval.IsResourceExhaustion(err) || errors.Is(err, atlaseval.ErrPreparationCachePoisoned) {
			return true
		}
	}
	return false
}

// evaluationCommandFatalCause marks resource exhaustion and known shared-authority failures command-wide while preserving their causal identities.
func evaluationCommandFatalCause(cause error) error {
	if cause == nil {
		return nil
	}
	var fatal evaluationCommandFatalError
	if errors.As(cause, &fatal) || (!eval.IsResourceExhaustion(cause) && !errors.Is(cause, atlaseval.ErrPreparationCachePoisoned)) {
		return cause
	}
	return evaluationCommandFatalError{cause: cause}
}

// runEvaluationComparisonJob owns one pair's writable state from preflight through cleanup and postflight integrity checks.
func runEvaluationComparisonJob(ctx context.Context, execution *evaluationExecution, tools evaluationTools, job evaluationComparisonJob, selected ...[]string) (eval.GuidanceDiagnosticResult, []error) {
	profiles := []string{eval.GuidanceProfileNone, eval.GuidanceProfileAgents}
	if len(selected) == 1 {
		profiles = selected[0]
	}
	if err := tools.Verify(); err != nil {
		return eval.GuidanceDiagnosticResult{}, []error{err}
	}
	jobRoot, err := evaluationJobRoot(execution.workRoot, job.evaluationID, job.trial)
	if err != nil {
		return eval.GuidanceDiagnosticResult{}, []error{err}
	}
	environments, err := newEvaluationJobEnvironments(jobRoot, tools, profiles)
	if err != nil {
		return eval.GuidanceDiagnosticResult{}, []error{evaluationCommandFatalCause(errors.Join(err, removeEvaluationWorkRoot(jobRoot)))}
	}
	if err := execution.verifierModuleProxyHealthy(ctx); err != nil {
		return eval.GuidanceDiagnosticResult{}, []error{evaluationCommandFatalCause(errors.Join(err, removeEvaluationWorkRoot(jobRoot)))}
	}
	result, diagnosticErr := execution.diagnostic.Run(ctx, eval.LocalGuidanceDiagnosticRequest{
		EvaluationID: job.evaluationID, DestinationRoot: filepath.Join(jobRoot, "projects"), Environments: environments,
		Profiles: profiles,
		TreatmentBoundary: func(boundaryContext context.Context) error {
			return errors.Join(tools.Verify(), execution.verifierModuleProxyHealthy(boundaryContext))
		},
	})
	diagnosticErr = evaluationCommandFatalCause(errors.Join(diagnosticErr, removeEvaluationWorkRoot(jobRoot), tools.Verify(), execution.verifierModuleProxyHealthy(context.Background())))
	return result, evaluationComparisonErrors(job, result, diagnosticErr, execution.redactor)
}

// evaluationJobRoot resolves only promoted IDs as one path component before any writable state or recursive cleanup exists.
func evaluationJobRoot(workRoot, evaluationID string, trial int) (string, error) {
	if filepath.Base(evaluationID) != evaluationID || !filepath.IsLocal(evaluationID) || evaluationID == "." {
		return "", fmt.Errorf("evaluation ID %q must be one local path component", evaluationID)
	}
	if _, err := eval.LoadPromotedDefinition(evaluationID); err != nil {
		return "", err
	}
	if trial < 1 {
		return "", fmt.Errorf("evaluation trial must be positive")
	}
	return filepath.Join(workRoot, "jobs", evaluationID, fmt.Sprintf("trial-%d", trial)), nil
}

// evaluationComparisonErrors redacts all paired-treatment and diagnostic failures before stable collation.
func evaluationComparisonErrors(job evaluationComparisonJob, result eval.GuidanceDiagnosticResult, diagnosticErr error, redactor eval.Redactor) []error {
	var attemptErrors []error
	for _, attempt := range result.Attempts {
		if attempt.Error != "" {
			cause := attempt.Cause
			if cause == nil {
				cause = errors.New(attempt.Error)
			}
			cause = evaluationCommandFatalCause(cause)
			attemptErrors = append(attemptErrors, evaluationDiagnosticError{
				message: fmt.Sprintf("%s/%s treatment: %s", job.evaluationID, attempt.Profile, redactor.Text(attempt.Error)),
				cause:   cause,
			})
		}
	}
	if diagnosticErr != nil {
		attemptErrors = append(attemptErrors, redactedEvaluationFailure(job.evaluationID, diagnosticErr, redactor))
	}
	return attemptErrors
}

// redactedEvaluationFailure keeps lifecycle identity without exposing the original diagnostic text.
func redactedEvaluationFailure(prefix string, cause error, redactor eval.Redactor) error {
	return evaluationDiagnosticError{message: fmt.Sprintf("%s: %s", prefix, redactor.Text(cause.Error())), cause: cause}
}

// evaluationDiagnosticError preserves cancellation identity while exposing only redacted text to command output.
type evaluationDiagnosticError struct {
	message string
	cause   error
}

// Error returns the terminal-safe diagnostic message.
func (failure evaluationDiagnosticError) Error() string {
	return failure.message
}

// Unwrap retains lifecycle sentinels for cancellation and timeout handling.
func (failure evaluationDiagnosticError) Unwrap() error {
	return failure.cause
}

// evaluationProgress serializes monotonic command progress while workers complete in arbitrary order.
type evaluationProgress struct {
	writer        io.Writer
	total         int
	trials        int
	mu            sync.Mutex
	startedCount  int
	finishedCount int
}

// newEvaluationProgress creates a no-op reporter when the command has no progress writer.
func newEvaluationProgress(writer io.Writer, total, trials int) *evaluationProgress {
	return &evaluationProgress{writer: writer, total: total, trials: trials}
}

// started records dispatch order rather than catalog position so counters remain monotonic with concurrent workers.
func (progress *evaluationProgress) started(job evaluationComparisonJob) {
	if progress.writer == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	progress.startedCount++
	fmt.Fprintf(progress.writer, "[%d/%d] %s · trial %d/%d\n", progress.startedCount, progress.total, job.evaluationID, job.trial, progress.trials)
}

// finished records completion order, which is intentionally independent from deterministic final result order.
func (progress *evaluationProgress) finished(job evaluationComparisonJob) {
	if progress.writer == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	progress.finishedCount++
	fmt.Fprintf(progress.writer, "[%d/%d] %s · finished\n", progress.finishedCount, progress.total, job.evaluationID)
}

// runEvaluationComparisonSchedule dispatches whole paired trials and stops assigning new work after cancellation.
func runEvaluationComparisonSchedule(ctx context.Context, jobs []evaluationComparisonJob, workerCount int, run func(index int, job evaluationComparisonJob, worker int)) {
	indexes := make(chan int)
	var workers sync.WaitGroup
	for worker := range workerCount {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for index := range indexes {
				if ctx.Err() == nil {
					run(index, jobs[index], worker)
				}
			}
		}(worker)
	}
	for index := range jobs {
		select {
		case <-ctx.Done():
			close(indexes)
			workers.Wait()
			return
		case indexes <- index:
		}
	}
	close(indexes)
	workers.Wait()
}

// runEvaluationTreatments executes one guidance profile while reusing immutable preparation across requested trials.
func runEvaluationTreatments(ctx context.Context, invocation evaluationInvocation, evaluationID, profile string, trials int) (runErr error) {
	if err := validateEvaluationTrials(trials); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateEvaluationIDs([]string{evaluationID}); err != nil {
		return err
	}
	if err := ensureEvaluationDiskCapacity(1); err != nil {
		return err
	}
	authority, err := newEvaluationAuthority(invocation)
	if err != nil {
		return err
	}
	execution, err := newEvaluationExecution(invocation, authority, []string{profile})
	if err != nil {
		return errors.Join(err, authority.Close())
	}
	defer func() {
		runErr = errors.Join(runErr, execution.Close(), authority.Close())
	}()
	results := make([]eval.GuidanceDiagnosticAttempt, 0, trials)
	var attemptErrors []error
	for range trials {
		if err := authority.tools.Verify(); err != nil {
			attemptErrors = append(attemptErrors, err)
			break
		}
		jobRoot, rootErr := evaluationJobRoot(execution.workRoot, evaluationID, len(results)+1)
		if rootErr != nil {
			attemptErrors = append(attemptErrors, rootErr)
			break
		}
		environment, environmentErr := newEvaluationTreatmentEnvironment(jobRoot, profile, authority.tools)
		if environmentErr != nil {
			attemptErrors = append(attemptErrors, evaluationCommandFatalCause(errors.Join(environmentErr, removeEvaluationWorkRoot(jobRoot))))
			break
		}
		if err := execution.verifierModuleProxyHealthy(ctx); err != nil {
			attemptErrors = append(attemptErrors, evaluationCommandFatalCause(errors.Join(err, removeEvaluationWorkRoot(jobRoot))))
			break
		}
		attempt, attemptErr := execution.diagnostic.RunTreatment(ctx, eval.LocalDiagnosticTreatmentRequest{
			EvaluationID:    evaluationID,
			GuidanceProfile: profile,
			DestinationRoot: filepath.Join(jobRoot, "projects"),
			Environment:     environment,
		})
		attemptErr = evaluationCommandFatalCause(errors.Join(attemptErr, removeEvaluationWorkRoot(jobRoot), authority.tools.Verify(), execution.verifierModuleProxyHealthy(context.Background())))
		results = append(results, attempt)
		if attemptErr != nil {
			attemptErrors = append(attemptErrors, redactedEvaluationFailure(evaluationID+"/"+profile+" treatment", attemptErr, execution.redactor))
			if evaluationCommandFatal([]error{attemptErr}) {
				break
			}
		}
		if ctx.Err() != nil {
			break
		}
	}
	if err := printEvaluationResults(results, authority.redactor, authority.artifactRoot); err != nil {
		return err
	}
	return errors.Join(attemptErrors...)
}

// evaluationAuthority freezes the retained artifact authority and Codex credential once for an invocation.
type evaluationAuthority struct {
	artifactKey  []byte
	artifactRoot string
	credential   eval.CodexCredential
	preparer     *atlaseval.Preparer
	tools        evaluationTools
	moduleProxy  *verifierModuleProxy
	redactor     eval.Redactor
	workRoot     string
	work         *evaluationWorkRoot
}

// newEvaluationAuthority resolves command-wide immutable inputs before workers create their private state.
func newEvaluationAuthority(invocation evaluationInvocation) (authority evaluationAuthority, runErr error) {
	forjExecutable, err := os.Executable()
	if err != nil {
		return evaluationAuthority{}, fmt.Errorf("resolve current Forj executable: %w", err)
	}
	credential, err := readEvalCredential(invocation.Credential)
	if err != nil {
		return evaluationAuthority{}, err
	}
	artifactRoot, err := resolveEvalArtifactRoot(invocation.Artifacts)
	if err != nil {
		return evaluationAuthority{}, err
	}
	artifactKey, err := readEvalArtifactKey(invocation.ArtifactKey, artifactRoot)
	if err != nil {
		return evaluationAuthority{}, err
	}
	work, err := newEvaluationWorkRoot()
	if err != nil {
		return evaluationAuthority{}, err
	}
	workRoot := work.path
	defer func() {
		if runErr != nil {
			runErr = errors.Join(runErr, work.Close())
		}
	}()
	tools, err := newEvaluationTools(filepath.Join(workRoot, "tools"), forjExecutable)
	if err != nil {
		return evaluationAuthority{}, err
	}
	baseEnvironment, err := evaluationEnvironmentWithTools(filepath.Join(workRoot, "base"), tools)
	if err != nil {
		return evaluationAuthority{}, err
	}
	moduleProxy, err := newVerifierModuleProxy(baseEnvironment)
	if err != nil {
		return evaluationAuthority{}, err
	}
	preparer := atlaseval.NewPreparerWithCapacity(filepath.Join(workRoot, "bases"), baseEnvironment, nil, materializeEvaluationGuidance, invocation.PairWorkers)
	return evaluationAuthority{
		artifactKey: artifactKey, artifactRoot: artifactRoot, credential: credential,
		preparer: preparer, tools: tools, moduleProxy: moduleProxy,
		redactor: credential.Redactor(eval.NewRedactor([]string{string(artifactKey)})), workRoot: workRoot, work: work,
	}, nil
}

// Close releases the command-wide immutable base cache after all worker diagnostics have stopped.
func (authority *evaluationAuthority) Close() error {
	if authority == nil {
		return nil
	}
	return errors.Join(authority.moduleProxy.Close(), authority.preparer.Close(context.Background()), authority.work.Close())
}

// evaluationExecution owns one worker's diagnostic, writable project clones, verifier state, home, temporary files, and Go caches.
type evaluationExecution struct {
	diagnostic   evaluationComparisonDiagnostic
	moduleProxy  *verifierModuleProxy
	workRoot     string
	artifactRoot string
	redactor     eval.Redactor
	work         *evaluationWorkRoot
}

// verifierModuleProxyHealthy keeps the shared verifier dependency surface live across treatment boundaries.
func (execution *evaluationExecution) verifierModuleProxyHealthy(ctx context.Context) error {
	if execution == nil {
		return errors.New("evaluation execution is required")
	}
	if execution.moduleProxy == nil {
		return nil
	}
	if err := execution.moduleProxy.Healthy(ctx); err != nil {
		return evaluationCommandFatalError{cause: err}
	}
	return nil
}

// evaluationComparisonDiagnostic keeps worker orchestration testable without weakening the concrete Atlas diagnostic used in production.
type evaluationComparisonDiagnostic interface {
	// Run preserves the diagnostic's treatment ordering when pair-level scheduling is unnecessary.
	Run(context.Context, eval.LocalGuidanceDiagnosticRequest) (eval.GuidanceDiagnosticResult, error)
	// RunTreatment lets workers schedule complete pairs while retaining independently sealed treatment evidence.
	RunTreatment(context.Context, eval.LocalDiagnosticTreatmentRequest) (eval.GuidanceDiagnosticAttempt, error)
}

// evaluationWorkRoot keeps the command workspace leased until every process and cleanup step has finished.
type evaluationWorkRoot struct {
	path  string
	lease *os.File
}

// Close releases the liveness lease before removing the command-owned workspace.
func (root *evaluationWorkRoot) Close() error {
	if root == nil {
		return nil
	}
	if root.lease == nil {
		return removeEvaluationWorkRoot(root.path)
	}
	unlockErr := unlockEvaluationLease(root.lease)
	closeErr := root.lease.Close()
	removeErr := removeEvaluationWorkRoot(root.path)
	root.lease = nil
	return errors.Join(unlockErr, closeErr, removeErr)
}

// newEvaluationExecution creates only one worker's private treatment environments from the invocation's frozen authority.
func newEvaluationExecution(invocation evaluationInvocation, authority evaluationAuthority, profiles []string) (execution *evaluationExecution, runErr error) {
	if _, err := evaluationProfiles(profiles); err != nil {
		return nil, err
	}
	work, err := newEvaluationWorkRootWithoutCleanup()
	if err != nil {
		return nil, err
	}
	workRoot := work.path
	defer func() {
		if runErr != nil {
			runErr = errors.Join(runErr, work.Close())
		}
	}()
	// Verifiers may read prepared module archives, so their seed must never come from agent-writable treatment state.
	runtime, err := evaluationRuntimeIdentity(authority.tools)
	if err != nil {
		return nil, err
	}
	diagnostic, err := eval.NewLocalGuidanceDiagnostic(eval.LocalGuidanceDiagnosticOptions{
		WorkRoot:            workRoot,
		ArtifactRoot:        authority.artifactRoot,
		ArtifactKey:         authority.artifactKey,
		Redactor:            authority.redactor,
		Preparer:            authority.preparer,
		Codex:               eval.CodexOptions{Executable: invocation.CodexExecutable, Model: invocation.Model, ModelProvider: invocation.ModelProvider, Credential: authority.credential},
		GoExecutable:        authority.tools.Executable("go"),
		ForjExecutable:      authority.tools.Executable("forj"),
		VerifierEnvironment: append([]string(nil), authority.preparer.BaseEnvironment...),
		VerifierModuleProxy: authority.moduleProxy.URL(),
		Runtime:             runtime,
	})
	if err != nil {
		return nil, err
	}
	return &evaluationExecution{
		diagnostic: diagnostic, workRoot: workRoot, artifactRoot: authority.artifactRoot,
		moduleProxy: authority.moduleProxy, redactor: authority.redactor, work: work,
	}, nil
}

// newEvaluationWorkRoot creates a disposable command workspace in the user cache, independent from retained artifacts and system temporary cleanup.
func newEvaluationWorkRoot() (*evaluationWorkRoot, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve evaluation work-state cache: %w", err)
	}
	return newEvaluationWorkRootAt(cacheRoot)
}

// newEvaluationWorkRootWithoutCleanup creates a leased worker root without repeating command-start recovery work.
func newEvaluationWorkRootWithoutCleanup() (*evaluationWorkRoot, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve evaluation work-state cache: %w", err)
	}
	return newEvaluationWorkRootAtWithoutCleanup(cacheRoot)
}

// newEvaluationWorkRootAt creates one disposable command workspace below an explicit cache root.
func newEvaluationWorkRootAt(cacheRoot string) (*evaluationWorkRoot, error) {
	workStateRoot, err := resolveEvaluationWorkStateRootAt(cacheRoot)
	if err != nil {
		return nil, err
	}
	if err := recoverStaleEvaluationWorkRoots(workStateRoot); err != nil {
		return nil, err
	}
	return newEvaluationWorkRootInStateRoot(workStateRoot)
}

// recoverStaleEvaluationWorkRoots performs bounded lease-protected crash recovery before new work consumes shared scratch capacity.
func recoverStaleEvaluationWorkRoots(workStateRoot string) error {
	cleanup, err := pruneStaleEvaluationWorkRoots(workStateRoot, time.Now())
	if err != nil {
		return err
	}
	if cleanup.deferred > 0 {
		fmt.Fprintf(os.Stderr, "Evaluation cleanup deferred %d stale work roots after removing %d; recovery will continue on the next command.\n", cleanup.deferred, cleanup.removed)
	}
	return nil
}

// newEvaluationWorkRootAtWithoutCleanup supports worker allocation after command-start recovery has established the shared state root.
func newEvaluationWorkRootAtWithoutCleanup(cacheRoot string) (*evaluationWorkRoot, error) {
	workStateRoot, err := resolveEvaluationWorkStateRootAt(cacheRoot)
	if err != nil {
		return nil, err
	}
	return newEvaluationWorkRootInStateRoot(workStateRoot)
}

// newEvaluationWorkRootInStateRoot publishes a leased, owned workspace below an already-resolved state root.
func newEvaluationWorkRootInStateRoot(workStateRoot string) (*evaluationWorkRoot, error) {
	root, err := os.MkdirTemp(workStateRoot, "command-")
	if err != nil {
		return nil, err
	}
	lease, err := openEvaluationWorkRootLease(root, true)
	if err != nil {
		return nil, errors.Join(err, removeEvaluationWorkRoot(root))
	}
	if err := lockEvaluationLease(lease); err != nil {
		return nil, errors.Join(err, lease.Close(), removeEvaluationWorkRoot(root))
	}
	// Publishing ownership only after the lease is held prevents a concurrent
	// crash-recovery scan from mistaking a newly created root for abandoned work.
	if err := writeEvaluationOwnershipMarker(root, evaluationWorkRootMarker, evaluationWorkRootIdentity); err != nil {
		return nil, errors.Join(err, unlockEvaluationLease(lease), lease.Close(), removeEvaluationWorkRoot(root))
	}
	return &evaluationWorkRoot{path: root, lease: lease}, nil
}

// openEvaluationWorkRootLease opens an owned regular lease file without following a final symlink.
func openEvaluationWorkRootLease(root string, create bool) (*os.File, error) {
	path := filepath.Join(root, evaluationWorkRootLease)
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE | os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open evaluation work-root lease: %w", err)
	}
	info, statErr := file.Stat()
	pathInfo, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !info.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) {
		_ = file.Close()
		return nil, fmt.Errorf("evaluation work-root lease must be one stable regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		_ = file.Close()
		return nil, fmt.Errorf("evaluation work-root lease must not be group or world accessible")
	}
	return file, nil
}

// resolveEvaluationWorkStateRootAt creates and verifies the private cache subtree below one explicit user-cache root.
func resolveEvaluationWorkStateRootAt(cacheRoot string) (string, error) {
	workStateRoot := filepath.Join(cacheRoot, "goforj", "atlas-evaluation-work")
	if err := os.MkdirAll(workStateRoot, 0o700); err != nil {
		return "", fmt.Errorf("create evaluation work-state root: %w", err)
	}
	info, err := os.Lstat(workStateRoot)
	if err != nil {
		return "", fmt.Errorf("inspect evaluation work-state root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("evaluation work-state root must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		return "", fmt.Errorf("evaluation work-state root permissions must be 0700")
	}
	return workStateRoot, nil
}

// staleEvaluationWorkCleanup reports bounded crash-recovery work and residue deferred to a later command.
type staleEvaluationWorkCleanup struct {
	removed  int
	deferred int
}

// pruneStaleEvaluationWorkRoots removes a bounded number of abandoned owned roots after an exclusive lease proves they are inactive.
func pruneStaleEvaluationWorkRoots(workStateRoot string, _ time.Time) (staleEvaluationWorkCleanup, error) {
	cleanup := staleEvaluationWorkCleanup{}
	directory, err := os.Open(workStateRoot)
	if err != nil {
		return cleanup, fmt.Errorf("open evaluation work-state root: %w", err)
	}
	entries, readErr := directory.Readdir(evaluationStaleWorkRootScanLimit + 1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return cleanup, errors.Join(fmt.Errorf("scan stale evaluation work roots: %w", readErr), closeErr)
	}
	if closeErr != nil {
		return cleanup, closeErr
	}
	if len(entries) > evaluationStaleWorkRootScanLimit {
		return cleanup, fmt.Errorf("evaluation work-state root exceeds %d entries; inspect and remove abandoned owned roots before retrying", evaluationStaleWorkRootScanLimit)
	}
	for _, entry := range entries {
		if !entry.IsDir() || (!strings.HasPrefix(entry.Name(), "command-") && !strings.HasPrefix(entry.Name(), evaluationStaleWorkQuarantine+"command-")) {
			continue
		}
		root := filepath.Join(workStateRoot, entry.Name())
		rootDirectory, err := os.Open(root)
		if err != nil {
			continue
		}
		rootIdentity, err := rootDirectory.Stat()
		if err != nil || !rootIdentity.IsDir() {
			_ = rootDirectory.Close()
			continue
		}
		markerPath := filepath.Join(root, evaluationWorkRootMarker)
		marker, err := os.Lstat(markerPath)
		if err != nil || !marker.Mode().IsRegular() || marker.Mode()&os.ModeSymlink != 0 {
			_ = rootDirectory.Close()
			continue
		}
		if err := verifyEvaluationOwnershipMarker(root, evaluationWorkRootMarker, evaluationWorkRootIdentity); err != nil {
			_ = rootDirectory.Close()
			continue
		}
		lease, err := openEvaluationWorkRootLease(root, false)
		if err != nil {
			_ = rootDirectory.Close()
			continue
		}
		locked, lockErr := tryLockEvaluationLease(lease)
		if lockErr != nil {
			_ = lease.Close()
			_ = rootDirectory.Close()
			return cleanup, fmt.Errorf("inspect stale evaluation work root %q lease: %w", entry.Name(), lockErr)
		}
		if !locked {
			_ = lease.Close()
			_ = rootDirectory.Close()
			continue
		}
		if cleanup.removed >= evaluationStaleWorkRootPruneLimit {
			releaseErr := errors.Join(unlockEvaluationLease(lease), lease.Close(), rootDirectory.Close())
			if releaseErr != nil {
				return cleanup, fmt.Errorf("release stale evaluation work root %q lease: %w", entry.Name(), releaseErr)
			}
			cleanup.deferred++
			continue
		}
		pathIdentity, pathErr := os.Lstat(root)
		if pathErr != nil || !os.SameFile(rootIdentity, pathIdentity) {
			_ = unlockEvaluationLease(lease)
			_ = lease.Close()
			_ = rootDirectory.Close()
			continue
		}
		quarantine := root
		if !strings.HasPrefix(entry.Name(), evaluationStaleWorkQuarantine) {
			quarantine = filepath.Join(workStateRoot, fmt.Sprintf("%s%s-%d", evaluationStaleWorkQuarantine, entry.Name(), time.Now().UnixNano()))
			if err := os.Rename(root, quarantine); err != nil {
				_ = unlockEvaluationLease(lease)
				_ = lease.Close()
				_ = rootDirectory.Close()
				return cleanup, fmt.Errorf("quarantine stale evaluation work root %q: %w", entry.Name(), err)
			}
		}
		quarantineIdentity, identityErr := os.Lstat(quarantine)
		releaseErr := errors.Join(unlockEvaluationLease(lease), lease.Close(), rootDirectory.Close())
		if releaseErr != nil {
			return cleanup, fmt.Errorf("release stale evaluation work root %q lease: %w", entry.Name(), releaseErr)
		}
		if identityErr != nil || !os.SameFile(rootIdentity, quarantineIdentity) {
			return cleanup, fmt.Errorf("quarantined evaluation work root %q changed identity", entry.Name())
		}
		if err := removeEvaluationWorkRoot(quarantine); err != nil {
			return cleanup, fmt.Errorf("remove stale evaluation work root %q: %w", entry.Name(), err)
		}
		cleanup.removed++
	}
	return cleanup, nil
}

// evaluationProfiles validates and de-duplicates the diagnostic treatment set before creating persistent artifacts.
func evaluationProfiles(profiles []string) ([]string, error) {
	selected := make([]string, 0, len(profiles))
	seen := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		if !eval.SupportedGuidanceProfile(profile) {
			return nil, fmt.Errorf("unsupported evaluation guidance profile %q", profile)
		}
		if !seen[profile] {
			seen[profile] = true
			selected = append(selected, profile)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("at least one evaluation guidance profile is required")
	}
	return selected, nil
}

// Close releases preparation caches before removing the command-owned workspace.
func (execution *evaluationExecution) Close() error {
	if execution == nil {
		return nil
	}
	return execution.work.Close()
}

// validateEvaluationTrials bounds accidental model spend while keeping repeated experiments explicit.
func validateEvaluationTrials(trials int) error {
	if trials < 1 || trials > 20 {
		return fmt.Errorf("evaluation trials must be between 1 and 20")
	}
	return nil
}

// validateEvaluationWorkers bounds concurrent live agent sessions and their private cache footprints.
func validateEvaluationWorkers(workers int) error {
	if workers < 1 || workers > 8 {
		return fmt.Errorf("evaluation workers must be between 1 and 8")
	}
	return nil
}

// printEvaluationResults preserves the existing single-result shape while emitting arrays for repeated runs.
func printEvaluationResults[T any](results []T, redactor eval.Redactor, artifactRoot string) error {
	var output any = results
	if len(results) == 1 {
		output = results[0]
	}
	body, err := marshalRedactedEvaluation(output, redactor)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	fmt.Printf("Artifacts: %s\n", artifactRoot)
	return nil
}

// evaluationRuntimeIdentity makes retained diagnostics reconstructable after command-owned binaries and workspaces are removed.
func evaluationRuntimeIdentity(tools evaluationTools) (eval.RuntimeIdentity, error) {
	if err := tools.Verify(); err != nil {
		return eval.RuntimeIdentity{}, err
	}
	if tools.goVersion == "" || tools.goDigest == "" || tools.goRootDigest == "" {
		return eval.RuntimeIdentity{}, fmt.Errorf("evaluation Go launcher provenance is incomplete")
	}
	frameworkVersion, frameworkCommit, frameworkDirty, err := version.EvaluationIdentity()
	if err != nil {
		return eval.RuntimeIdentity{}, err
	}
	return eval.RuntimeIdentity{
		Supervisor: atlasSoftwareIdentity(),
		Framework: eval.SoftwareIdentity{
			Module:  "github.com/goforj/goforj",
			Version: frameworkVersion,
			Commit:  frameworkCommit,
			Dirty:   frameworkDirty,
		},
		GoVersion:          tools.goVersion,
		GoExecutableDigest: tools.goDigest,
		GoRootDigest:       tools.goRootDigest,
		GOOS:               runtime.GOOS,
		GOARCH:             runtime.GOARCH,
	}, nil
}

// atlasSoftwareIdentity reports the exact dependency selected into the current GoForj binary.
func atlasSoftwareIdentity() eval.SoftwareIdentity {
	identity := eval.SoftwareIdentity{Module: "github.com/goforj/atlas", Version: "unknown"}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return identity
	}
	for _, dependency := range info.Deps {
		if dependency.Path != identity.Module {
			continue
		}
		identity.Version = dependency.Version
		if dependency.Replace != nil {
			replacement := dependency.Replace.Version
			if replacement == "" {
				replacement = "local"
			}
			identity.Version += " => " + replacement
		}
		return identity
	}
	return identity
}

// materializeEvaluationGuidance uses the production render setting and native guidance reconciliation path so evaluation treatments cannot drift from normal Projects.
func materializeEvaluationGuidance(ctx context.Context, prepared eval.PreparedProject, guidance eval.Guidance, selection project.AgentGuidance) (eval.Guidance, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return eval.Guidance{}, err
	}
	if prepared == nil || strings.TrimSpace(prepared.Result().ProjectRoot) == "" {
		return eval.Guidance{}, fmt.Errorf("prepared Project is required")
	}
	root := prepared.Result().ProjectRoot
	config, err := project.LoadProjectConfigAt(root)
	if err != nil {
		return eval.Guidance{}, fmt.Errorf("load prepared Project configuration: %w", err)
	}
	config.Render.AgentGuidance = selection
	if err := writeProjectGuidanceConfig(filepath.Join(root, ".goforj.yml"), config); err != nil {
		return eval.Guidance{}, fmt.Errorf("persist evaluation guidance selection: %w", err)
	}
	if _, err := ReconcileAgentGuidance(root, selection); err != nil {
		return eval.Guidance{}, fmt.Errorf("reconcile evaluation guidance: %w", err)
	}
	result := eval.Guidance{Profile: guidance.Profile, Skills: append([]string(nil), guidance.Skills...), MCP: append([]string(nil), guidance.MCP...), Files: map[string][]byte{}}
	if selection != project.AgentGuidanceBaseline {
		return result, nil
	}
	if err := recordEvaluationGuidanceFile(root, "AGENTS.md", result.Files); err != nil {
		return eval.Guidance{}, err
	}
	if len(guidance.Skills) > 0 {
		paths, err := skills.Write(skills.WriteOptions{Root: root, Agent: agents.Codex{}, Project: Project(root)})
		if err != nil {
			return eval.Guidance{}, fmt.Errorf("write evaluation skills: %w", err)
		}
		for _, path := range paths {
			if err := recordEvaluationGuidanceFile(root, path, result.Files); err != nil {
				return eval.Guidance{}, err
			}
		}
	}
	if len(guidance.MCP) > 0 {
		if len(guidance.MCP) != 1 || guidance.MCP[0] != "goforj-atlas" {
			return eval.Guidance{}, fmt.Errorf("unsupported evaluation MCP selection %v", guidance.MCP)
		}
		codex := agents.Codex{}
		server := agents.DefaultMCPServerConfig(root)
		server.Command = prepared.Result().ForjExecutable
		if strings.TrimSpace(server.Command) == "" {
			return eval.Guidance{}, fmt.Errorf("prepared GoForj executable is required for Atlas MCP")
		}
		if err := codex.WriteMCPConfig(ctx, root, server); err != nil {
			return eval.Guidance{}, fmt.Errorf("write evaluation MCP configuration: %w", err)
		}
		if err := recordEvaluationGuidanceFile(root, codex.MCPConfigPath(root), result.Files); err != nil {
			return eval.Guidance{}, err
		}
	}
	return result, nil
}

// recordEvaluationGuidanceFile captures one native projection relative to the prepared Project for treatment attribution.
func recordEvaluationGuidanceFile(root, path string, files map[string][]byte) error {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("evaluation guidance path %q escapes prepared Project", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read evaluation guidance %q: %w", relative, err)
	}
	files[filepath.Clean(relative)] = body
	return nil
}

// removeEvaluationWorkRoot restores traversal only inside the command-owned root so read-only module caches remain disposable.
func removeEvaluationWorkRoot(root string) error {
	if _, err := os.Lstat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return nil
	})
	return errors.Join(walkErr, os.RemoveAll(root))
}

// readEvalCredential freezes one explicit private credential so later path changes cannot substitute normal developer authority.
func readEvalCredential(candidate string) (eval.CodexCredential, error) {
	if strings.TrimSpace(candidate) == "" {
		return eval.CodexCredential{}, fmt.Errorf("Codex credential is required; pass a disposable, revocable auth.json with --credential")
	}
	file, err := readPrivateEvaluationFile(candidate, "Codex credential", maxEvaluationCredentialSize)
	if err != nil {
		return eval.CodexCredential{}, err
	}
	credential, err := eval.NewCodexCredential(file.body)
	if err != nil {
		return eval.CodexCredential{}, err
	}
	return credential, nil
}

// resolveEvalArtifactRoot selects a retained private directory outside every disposable agent Project.
func resolveEvalArtifactRoot(candidate string) (string, error) {
	if strings.TrimSpace(candidate) == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		candidate = filepath.Join(cache, "goforj", "atlas-evaluations")
	}
	path, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return "", fmt.Errorf("create evaluation artifact root: %w", err)
		}
		if err := writeEvaluationOwnershipMarker(path, evaluationArtifactRootMarker, evaluationArtifactRootIdentity); err != nil {
			return "", errors.Join(err, os.Remove(path))
		}
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect evaluation artifact root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("evaluation artifact root must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		return "", fmt.Errorf("evaluation artifact root permissions must be 0700")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("read evaluation artifact root: %w", err)
	}
	if len(entries) == 0 {
		if err := writeEvaluationOwnershipMarker(path, evaluationArtifactRootMarker, evaluationArtifactRootIdentity); err != nil {
			return "", err
		}
		return path, nil
	}
	if err := verifyEvaluationOwnershipMarker(path, evaluationArtifactRootMarker, evaluationArtifactRootIdentity); err != nil {
		return "", fmt.Errorf("non-empty evaluation artifact root is not owned by Atlas evaluation: %w", err)
	}
	return path, nil
}

// marshalRedactedEvaluation applies artifact-equivalent redaction before diagnostic values reach the terminal.
func marshalRedactedEvaluation(value any, redactor eval.Redactor) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	return json.MarshalIndent(redactor.JSONValue(decoded), "", "  ")
}

// readEvalArtifactKey freezes an external authentication key before an artifact operation begins.
func readEvalArtifactKey(path, artifactRoot string) ([]byte, error) {
	keyFile, err := readPrivateEvaluationFile(path, "evaluation artifact key", maxEvaluationArtifactKeySize)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(artifactRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve evaluation artifact root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve evaluation artifact root links: %w", err)
	}
	relative, err := filepath.Rel(resolvedRoot, keyFile.resolvedPath)
	if err != nil || relative == "." || relative == ".." || !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("evaluation artifact key must be outside the artifact root")
	}
	if len(keyFile.body) < 32 {
		return nil, fmt.Errorf("evaluation artifact key has invalid length")
	}
	return append([]byte(nil), keyFile.body...), nil
}

// privateEvaluationFile is one descriptor-verified supervisor input frozen before concurrent diagnostics begin.
type privateEvaluationFile struct {
	body         []byte
	resolvedPath string
}

// readPrivateEvaluationFile rejects mutable link endpoints and broad permissions before reading a bounded supervisor input once.
func readPrivateEvaluationFile(candidate, label string, limit int64) (privateEvaluationFile, error) {
	path, err := filepath.Abs(candidate)
	if err != nil {
		return privateEvaluationFile{}, fmt.Errorf("resolve %s: %w", label, err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return privateEvaluationFile{}, fmt.Errorf("resolve %s links: %w", label, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return privateEvaluationFile{}, fmt.Errorf("inspect %s: %w", label, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return privateEvaluationFile{}, fmt.Errorf("%s must be a regular file", label)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return privateEvaluationFile{}, fmt.Errorf("%s must not be group or world accessible", label)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return privateEvaluationFile{}, fmt.Errorf("open %s: %w", label, err)
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		return privateEvaluationFile{}, fmt.Errorf("%s changed while it was opened", label)
	}
	body, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return privateEvaluationFile{}, fmt.Errorf("read %s: %w", label, err)
	}
	if int64(len(body)) > limit {
		return privateEvaluationFile{}, fmt.Errorf("%s exceeds %d bytes", label, limit)
	}
	return privateEvaluationFile{body: body, resolvedPath: resolved}, nil
}

// writeEvaluationOwnershipMarker claims an otherwise empty supervisor-owned directory without storing authentication authority in it.
func writeEvaluationOwnershipMarker(root, name, identity string) error {
	path := filepath.Join(root, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create evaluation ownership marker: %w", err)
	}
	_, writeErr := io.WriteString(file, identity)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return errors.Join(err, os.Remove(path))
	}
	return nil
}

// verifyEvaluationOwnershipMarker distinguishes reusable Atlas directories from unrelated caller content.
func verifyEvaluationOwnershipMarker(root, name, identity string) error {
	path := filepath.Join(root, name)
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("ownership marker must be a regular file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(body) != identity {
		return fmt.Errorf("ownership marker identity is invalid")
	}
	return nil
}

// evaluationEnvironment gives both treatments identical private Go caches and a fixed developer tool surface.
func evaluationEnvironment(workRoot, forjExecutable string) ([]string, error) {
	tools, err := newEvaluationTools(filepath.Join(workRoot, "tools"), forjExecutable)
	if err != nil {
		return nil, err
	}
	return evaluationEnvironmentWithTools(workRoot, tools)
}

// evaluationTools is one command-owned immutable executable snapshot shared by job-private environments.
type evaluationTools struct {
	digest         string
	dir            string
	goDigest       string
	goVersion      string
	goRoot         string
	goRootDigest   string
	goRootIdentity os.FileInfo
	mu             *sync.Mutex
}

// Executable returns one command-owned verifier executable from the closed tool snapshot.
func (tools evaluationTools) Executable(name string) string {
	return filepath.Join(tools.dir, name)
}

// evaluationCommandFatalError identifies a failure that must stop dispatching independent evaluations.
type evaluationCommandFatalError struct {
	cause error
}

// Error returns the shared-resource failure without changing its causal identity.
func (failure evaluationCommandFatalError) Error() string {
	return failure.cause.Error()
}

// Unwrap retains the underlying filesystem or integrity failure for diagnostics and errors.Is.
func (failure evaluationCommandFatalError) Unwrap() error {
	return failure.cause
}

// newEvaluationTools snapshots the fixed command surface once before concurrent diagnostics begin.
func newEvaluationTools(directory, forjExecutable string) (evaluationTools, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return evaluationTools{}, err
	}
	digest, goDigest, goRoot, goRootIdentity, err := installEvaluationTools(directory, forjExecutable)
	if err != nil {
		return evaluationTools{}, err
	}
	if err := os.Chmod(directory, 0o500); err != nil {
		return evaluationTools{}, fmt.Errorf("seal evaluation tool snapshot: %w", err)
	}
	goVersion, err := evaluationGoVersion(filepath.Join(directory, evaluationToolFileName("go")), goRoot)
	if err != nil {
		return evaluationTools{}, err
	}
	return evaluationTools{dir: directory, digest: digest, goDigest: goDigest, goVersion: goVersion, goRoot: goRoot, goRootDigest: digestEvaluationGoRoot(goRoot), goRootIdentity: goRootIdentity, mu: &sync.Mutex{}}, nil
}

// Verify detects same-user mutations to the shared tool snapshot before they can contaminate another job.
func (tools evaluationTools) Verify() error {
	if tools.mu != nil {
		tools.mu.Lock()
		defer tools.mu.Unlock()
	}
	digest, err := digestEvaluationTools(tools.dir)
	if err != nil {
		return evaluationCommandFatalError{cause: fmt.Errorf("inspect evaluation tool snapshot: %w", err)}
	}
	if digest != tools.digest {
		return evaluationCommandFatalError{cause: fmt.Errorf("evaluation tool snapshot changed during diagnostic execution")}
	}
	goRootIdentity, err := os.Lstat(tools.goRoot)
	if err != nil || !goRootIdentity.IsDir() || goRootIdentity.Mode()&os.ModeSymlink != 0 || !os.SameFile(tools.goRootIdentity, goRootIdentity) {
		return evaluationCommandFatalError{cause: fmt.Errorf("evaluation Go runtime changed during diagnostic execution")}
	}
	return nil
}

// newEvaluationJobEnvironments creates the paired treatment environments owned by one evaluation/trial job.
func newEvaluationJobEnvironments(root string, tools evaluationTools, selected ...[]string) (map[string][]string, error) {
	profiles := []string{eval.GuidanceProfileNone, eval.GuidanceProfileAgents}
	if len(selected) == 1 {
		profiles = selected[0]
	}
	environments := make(map[string][]string, len(profiles))
	for _, profile := range profiles {
		environment, err := newEvaluationTreatmentEnvironment(root, profile, tools)
		if err != nil {
			return nil, err
		}
		environments[profile] = environment
	}
	return environments, nil
}

// newEvaluationTreatmentEnvironment gives one treatment its own writable process state below its job root.
func newEvaluationTreatmentEnvironment(root, profile string, tools evaluationTools) ([]string, error) {
	if !eval.SupportedGuidanceProfile(profile) {
		return nil, fmt.Errorf("unsupported evaluation guidance profile %q", profile)
	}
	return evaluationEnvironmentWithTools(filepath.Join(root, profile), tools)
}

// evaluationEnvironmentWithTools gives one treatment private writable state while reusing the command-owned tool snapshot.
func evaluationEnvironmentWithTools(workRoot string, tools evaluationTools) ([]string, error) {
	goCache := filepath.Join(workRoot, "gocache")
	moduleCache := filepath.Join(workRoot, "gomodcache")
	goPath := filepath.Join(workRoot, "gopath")
	home := filepath.Join(workRoot, "home")
	temporary := filepath.Join(workRoot, "tmp")
	if err := errors.Join(os.MkdirAll(goCache, 0o700), os.MkdirAll(moduleCache, 0o700), os.MkdirAll(goPath, 0o700), os.MkdirAll(home, 0o700), os.MkdirAll(temporary, 0o700)); err != nil {
		return nil, err
	}
	overrides := map[string]string{
		"GOCACHE":                 goCache,
		"GOENV":                   "off",
		"GOMODCACHE":              moduleCache,
		"GOPATH":                  goPath,
		"GOPROXY":                 "https://proxy.golang.org,direct",
		"GOSUMDB":                 "sum.golang.org",
		"GOTOOLCHAIN":             "local",
		"GOROOT":                  tools.goRoot,
		"GOWORK":                  "off",
		"HOME":                    home,
		"GOTMPDIR":                temporary,
		"PATH":                    tools.dir,
		"TEMP":                    temporary,
		"TMP":                     temporary,
		"TMPDIR":                  temporary,
		"ATLAS_EVAL_TOOLS_DIGEST": tools.digest,
	}
	base := baseEvaluationEnvironment()
	environment := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		environment = append(environment, entry)
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment, nil
}

// installEvaluationTools snapshots the small command surface agents need without exposing the supervisor's complete PATH.
func installEvaluationTools(toolsDir, forjExecutable string) (string, string, string, os.FileInfo, error) {
	hash := sha256.New()
	goExecutable, err := exec.LookPath("go")
	if err != nil {
		return "", "", "", nil, fmt.Errorf("resolve evaluation tool %q: %w", "go", err)
	}
	goRoot, goRootIdentity, err := resolveEvaluationGoRoot(goExecutable)
	if err != nil {
		return "", "", "", nil, err
	}
	goDigest, err := installEvaluationTool(toolsDir, "go", goExecutable)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("install evaluation tool %q: %w", "go", err)
	}
	fmt.Fprintf(hash, "go\x00%s\x00", goDigest)
	forjDigest, err := installEvaluationTool(toolsDir, "forj", forjExecutable)
	if err != nil {
		return "", "", "", nil, err
	}
	fmt.Fprintf(hash, "forj\x00%s\x00", forjDigest)
	for _, name := range evaluationSupportToolNames() {
		path, err := exec.LookPath(name)
		if err != nil {
			return "", "", "", nil, fmt.Errorf("resolve evaluation tool %q: %w", name, err)
		}
		digest, err := installEvaluationTool(toolsDir, name, path)
		if err != nil {
			return "", "", "", nil, fmt.Errorf("install evaluation tool %q: %w", name, err)
		}
		fmt.Fprintf(hash, "%s\x00%s\x00", name, digest)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), goDigest, goRoot, goRootIdentity, nil
}

// evaluationGoVersion returns the version reported by the exact copied launcher used by candidate and verifier commands.
func evaluationGoVersion(goExecutable, goRoot string) (string, error) {
	command := exec.Command(goExecutable, "version")
	command.Env = append(os.Environ(), "GOROOT="+goRoot, "GOTOOLCHAIN=local")
	body, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("inspect evaluation Go launcher version: %w", err)
	}
	version := strings.TrimSpace(string(body))
	if version == "" {
		return "", fmt.Errorf("evaluation Go launcher version is empty")
	}
	return version, nil
}

// digestEvaluationGoRoot retains a path-free identifier for the selected Go runtime tree.
func digestEvaluationGoRoot(goRoot string) string {
	hash := sha256.Sum256([]byte("goforj-evaluation-go-root/v1\x00" + goRoot))
	return fmt.Sprintf("sha256:%x", hash[:])
}

// resolveEvaluationGoRoot binds the copied Go launcher to the host runtime tree it requires after PATH is sealed.
func resolveEvaluationGoRoot(goExecutable string) (string, os.FileInfo, error) {
	command := exec.Command(goExecutable, "env", "GOROOT")
	command.Env = os.Environ()
	body, err := command.Output()
	if err != nil {
		return "", nil, fmt.Errorf("resolve evaluation Go runtime: %w", err)
	}
	configuredRoot := strings.TrimSpace(string(body))
	if configuredRoot == "" {
		return "", nil, fmt.Errorf("evaluation Go runtime path is empty")
	}
	root, err := filepath.Abs(configuredRoot)
	if err != nil {
		return "", nil, fmt.Errorf("resolve evaluation Go runtime path: %w", err)
	}
	identity, err := os.Lstat(root)
	if err != nil || !identity.IsDir() || identity.Mode()&os.ModeSymlink != 0 {
		if err != nil {
			return "", nil, fmt.Errorf("inspect evaluation Go runtime: %w", err)
		}
		return "", nil, fmt.Errorf("evaluation Go runtime must be a directory without symlinks")
	}
	return root, identity, nil
}

// digestEvaluationTools recomputes the command snapshot identity without copying its executables again.
func digestEvaluationTools(toolsDir string) (string, error) {
	directory, err := os.Lstat(toolsDir)
	if err != nil {
		return "", fmt.Errorf("inspect evaluation tool snapshot: %w", err)
	}
	if !directory.IsDir() || directory.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("evaluation tool snapshot must be a real directory")
	}
	if runtime.GOOS != "windows" && directory.Mode().Perm()&0o222 != 0 {
		return "", fmt.Errorf("evaluation tool snapshot directory became writable")
	}
	hash := sha256.New()
	want := append([]string{"go", "forj"}, evaluationSupportToolNames()...)
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return "", fmt.Errorf("list evaluation tool snapshot: %w", err)
	}
	if len(entries) != len(want) {
		return "", fmt.Errorf("evaluation tool snapshot contains unexpected entries")
	}
	wantedEntries := make(map[string]struct{}, len(want))
	for _, name := range want {
		wantedEntries[evaluationToolFileName(name)] = struct{}{}
	}
	for _, entry := range entries {
		if _, ok := wantedEntries[entry.Name()]; !ok {
			return "", fmt.Errorf("evaluation tool snapshot contains unexpected entry %q", entry.Name())
		}
	}
	for _, name := range want {
		path := filepath.Join(toolsDir, evaluationToolFileName(name))
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("inspect evaluation tool %q: %w", name, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("evaluation tool %q must be a regular file", name)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o222 != 0 {
			return "", fmt.Errorf("evaluation tool %q became writable", name)
		}
		input, err := os.Open(path)
		if err != nil {
			return "", err
		}
		fileHash := sha256.New()
		_, copyErr := io.Copy(fileHash, input)
		closeErr := input.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return "", err
		}
		fmt.Fprintf(hash, "%s\x00sha256:%x\x00", name, fileHash.Sum(nil))
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

// evaluationToolFileName returns the platform-specific filename used in a tool snapshot.
func evaluationToolFileName(name string) string {
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		return name + ".exe"
	}
	return name
}

// evaluationSupportToolNames keeps the diagnostic shell useful while making every available command an explicit harness decision.
func evaluationSupportToolNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"wire"}
	}
	return []string{"bash", "cat", "git", "head", "ls", "node", "sed", "sh", "wire"}
}

// baseEvaluationEnvironment retains only process settings needed by portable tools and excludes ambient credentials and user configuration.
func baseEvaluationEnvironment() []string {
	keys := []string{
		"PATH",
		"TMPDIR", "TMP", "TEMP",
		"SystemRoot", "WINDIR", "COMSPEC", "PATHEXT",
		"LANG", "LC_ALL", "LC_CTYPE", "TERM", "COLORTERM",
		"SSL_CERT_FILE", "SSL_CERT_DIR",
	}
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}

// installEvaluationTool gives the agent an attempt-private executable without sharing a mutable source binary.
func installEvaluationTool(toolsDir, name, source string) (string, error) {
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		extension := filepath.Ext(source)
		if extension == "" {
			extension = ".exe"
		}
		name += extension
	}
	destination := filepath.Join(toolsDir, name)
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o500)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	closeErr := output.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

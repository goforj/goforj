package atlas

import (
	"context"
	"crypto/rand"
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

	"github.com/goforj/atlas/eval"
	"github.com/goforj/goforj/internal/forj/atlaseval"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
)

// EvalCmd groups opt-in live-agent evaluation commands.
type EvalCmd struct {
	Compare EvalCompareCmd `cmd:""`
	List    EvalListCmd    `cmd:""`
	Report  EvalReportCmd  `cmd:""`
	Run     EvalRunCmd     `cmd:""`
	Suite   EvalSuiteCmd   `cmd:""`
}

// Signature returns the Kong metadata for EvalCmd.
func (*EvalCmd) Signature() string {
	return `name:"atlas:eval" help:"Run opt-in Atlas agent evaluations"`
}

// EvalCompareCmd runs the first diagnostic control-versus-AGENTS comparison.
type EvalCompareCmd struct {
	Evaluation      string `arg:"" help:"Promoted evaluation ID" default:"add-http-controller"`
	Model           string `help:"Exact Codex model identity" required:""`
	ModelProvider   string `name:"model-provider" help:"Codex model provider" default:"openai"`
	CodexExecutable string `name:"codex" help:"Codex executable or PATH name" default:"codex"`
	Credential      string `help:"Disposable, revocable Codex auth.json source for this unconfined diagnostic" type:"path" required:""`
	Artifacts       string `help:"Supervisor-owned artifact directory" type:"path"`
	Trials          int    `help:"Independent paired trials" default:"1"`
}

// EvalRunCmd runs one promoted diagnostic treatment without paying for an unused comparison profile.
type EvalRunCmd struct {
	Evaluation      string `arg:"" help:"Promoted evaluation ID" default:"add-http-controller"`
	Guidance        string `help:"Guidance profile" enum:"none,agents" default:"agents"`
	Model           string `help:"Exact Codex model identity" required:""`
	ModelProvider   string `name:"model-provider" help:"Codex model provider" default:"openai"`
	CodexExecutable string `name:"codex" help:"Codex executable or PATH name" default:"codex"`
	Credential      string `help:"Disposable, revocable Codex auth.json source for this unconfined diagnostic" type:"path" required:""`
	Artifacts       string `help:"Supervisor-owned artifact directory" type:"path"`
	Trials          int    `help:"Independent trials" default:"1"`
}

// EvalReportCmd authenticates and prints one retained attempt summary.
type EvalReportCmd struct {
	Directory string `arg:"" help:"Retained attempt artifact directory" type:"path"`
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
		Credential: command.Credential, Artifacts: command.Artifacts,
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
	key, err := readEvalArtifactKey(filepath.Dir(directory))
	if err != nil {
		return err
	}
	summary, _, err := eval.ReadVerifiedAttemptSummary(directory, key)
	if err != nil {
		return err
	}
	fmt.Print(summary)
	return nil
}

// EvalSuiteCmd runs every promoted evaluation in one suite with frozen authority and bounded concurrent diagnostics.
type EvalSuiteCmd struct {
	Suite           string `arg:"" help:"Promoted evaluation suite" default:"core"`
	Kind            string `help:"Limit the suite to one measurement kind" enum:"all,scaffold,feature,repair,abstention" default:"all"`
	Model           string `help:"Exact Codex model identity" required:""`
	ModelProvider   string `name:"model-provider" help:"Codex model provider" default:"openai"`
	CodexExecutable string `name:"codex" help:"Codex executable or PATH name" default:"codex"`
	Credential      string `help:"Disposable, revocable Codex auth.json source for this unconfined diagnostic" type:"path" required:""`
	Artifacts       string `help:"Supervisor-owned artifact directory" type:"path"`
	Trials          int    `help:"Independent paired trials per evaluation" default:"1"`
	Workers         int    `help:"Concurrent diagnostic executions; does not provide security isolation" default:"1"`
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
	if len(ids) == 0 {
		return fmt.Errorf("evaluation suite %q with kind %q is empty or unknown", command.Suite, command.Kind)
	}
	plan, err := buildEvaluationSuitePlan(ids, command.Trials)
	if err != nil {
		return err
	}
	printEvaluationSuitePlan(os.Stderr, plan)
	return runEvaluationComparisonsWithWorkers(ctx, evaluationInvocation{
		Model: command.Model, ModelProvider: command.ModelProvider, CodexExecutable: command.CodexExecutable,
		Credential: command.Credential, Artifacts: command.Artifacts, Progress: os.Stderr,
	}, ids, command.Trials, command.Workers)
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
	Progress        io.Writer
}

// Signature returns the Kong metadata for EvalCompareCmd.
func (*EvalCompareCmd) Signature() string {
	return `name:"compare" help:"Compare no guidance with canonical AGENTS.md guidance"`
}

// Run executes two fresh diagnostic sessions and prints their machine-readable results.
func (command *EvalCompareCmd) Run() (runErr error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return command.run(ctx)
}

// run executes the comparison with a caller-owned lifecycle so cancellation reaches every evaluation resource.
func (command *EvalCompareCmd) run(ctx context.Context) (runErr error) {
	return runEvaluationComparisons(ctx, evaluationInvocation{
		Model: command.Model, ModelProvider: command.ModelProvider, CodexExecutable: command.CodexExecutable,
		Credential: command.Credential, Artifacts: command.Artifacts,
	}, []string{command.Evaluation}, command.Trials)
}

// runEvaluationComparisons shares frozen credentials, tool resolution, caches, and verifier infrastructure across one invocation.
func runEvaluationComparisons(ctx context.Context, invocation evaluationInvocation, evaluationIDs []string, trials int) (runErr error) {
	return runEvaluationComparisonsWithWorkers(ctx, invocation, evaluationIDs, trials, 1)
}

// runEvaluationComparisonsWithWorkers schedules complete paired trials across separate diagnostic executions.
func runEvaluationComparisonsWithWorkers(ctx context.Context, invocation evaluationInvocation, evaluationIDs []string, trials, workers int) (runErr error) {
	if err := validateEvaluationTrials(trials); err != nil {
		return err
	}
	if err := validateEvaluationWorkers(workers); err != nil {
		return err
	}
	jobs := evaluationComparisonJobs(evaluationIDs, trials)
	if len(jobs) == 0 {
		return fmt.Errorf("at least one evaluation is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	authority, err := newEvaluationAuthority(invocation)
	if err != nil {
		return err
	}
	workers = min(workers, len(jobs))
	executions := make([]*evaluationExecution, 0, workers)
	for range workers {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, closeEvaluationExecutions(executions), authority.Close())
		}
		execution, err := newEvaluationExecution(invocation, authority, []string{eval.GuidanceProfileNone, eval.GuidanceProfileAgents})
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
		execution := executions[worker]
		result, diagnosticErr := execution.diagnostic.Run(ctx, eval.LocalGuidanceDiagnosticRequest{
			EvaluationID: job.evaluationID, DestinationRoot: filepath.Join(execution.workRoot, "projects"), Environments: execution.environments,
		})
		return result, evaluationComparisonErrors(job, result, diagnosticErr, execution.redactor)
	})
	if err := printEvaluationResults(results, authority.redactor, authority.artifactRoot); err != nil {
		return err
	}
	return errors.Join(attemptErrors...)
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
	results := make([]eval.GuidanceDiagnosticResult, len(jobs))
	completed := make([]bool, len(jobs))
	errorsByJob := make([][]error, len(jobs))
	reporter := newEvaluationProgress(progress, len(jobs), trials)
	runEvaluationComparisonSchedule(ctx, jobs, workerCount, func(index int, job evaluationComparisonJob, worker int) {
		reporter.started(job)
		result, jobErrors := run(ctx, job, worker)
		results[index] = result
		completed[index] = true
		errorsByJob[index] = jobErrors
		reporter.finished(job)
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

// evaluationComparisonErrors redacts all paired-treatment and diagnostic failures before stable collation.
func evaluationComparisonErrors(job evaluationComparisonJob, result eval.GuidanceDiagnosticResult, diagnosticErr error, redactor eval.Redactor) []error {
	var attemptErrors []error
	for _, attempt := range result.Attempts {
		if attempt.Error != "" {
			attemptErrors = append(attemptErrors, fmt.Errorf("%s/%s treatment: %s", job.evaluationID, attempt.Profile, redactor.Text(attempt.Error)))
		}
	}
	if diagnosticErr != nil {
		attemptErrors = append(attemptErrors, fmt.Errorf("%s: %s", job.evaluationID, redactor.Text(diagnosticErr.Error())))
	}
	return attemptErrors
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
		attempt, attemptErr := execution.diagnostic.RunTreatment(ctx, eval.LocalDiagnosticTreatmentRequest{
			EvaluationID:    evaluationID,
			GuidanceProfile: profile,
			DestinationRoot: filepath.Join(execution.workRoot, "projects"),
			Environment:     execution.environments[profile],
		})
		results = append(results, attempt)
		if attemptErr != nil {
			attemptErrors = append(attemptErrors, fmt.Errorf("%s/%s treatment: %s", evaluationID, profile, execution.redactor.Text(attemptErr.Error())))
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
	artifactKey    []byte
	artifactRoot   string
	credential     eval.CodexCredential
	forjExecutable string
	goExecutable   string
	preparer       *atlaseval.Preparer
	redactor       eval.Redactor
	workRoot       string
}

// newEvaluationAuthority resolves command-wide immutable inputs before workers create their private state.
func newEvaluationAuthority(invocation evaluationInvocation) (authority evaluationAuthority, runErr error) {
	forjExecutable, err := os.Executable()
	if err != nil {
		return evaluationAuthority{}, fmt.Errorf("resolve current Forj executable: %w", err)
	}
	goExecutable, err := exec.LookPath("go")
	if err != nil {
		return evaluationAuthority{}, fmt.Errorf("resolve Go executable: %w", err)
	}
	credentialPath, err := resolveEvalCredential(invocation.Credential)
	if err != nil {
		return evaluationAuthority{}, err
	}
	credential, err := eval.LoadCodexCredential(credentialPath)
	if err != nil {
		return evaluationAuthority{}, err
	}
	artifactRoot, err := resolveEvalArtifactRoot(invocation.Artifacts)
	if err != nil {
		return evaluationAuthority{}, err
	}
	artifactKey, err := loadOrCreateEvalArtifactKey(artifactRoot)
	if err != nil {
		return evaluationAuthority{}, err
	}
	workRoot, err := newEvaluationWorkRoot()
	if err != nil {
		return evaluationAuthority{}, err
	}
	defer func() {
		if runErr != nil {
			runErr = cleanupEvaluationSetupFailure(workRoot, runErr)
		}
	}()
	baseEnvironment, err := evaluationEnvironment(filepath.Join(workRoot, "base"), forjExecutable)
	if err != nil {
		return evaluationAuthority{}, err
	}
	preparer := atlaseval.NewPreparer(filepath.Join(workRoot, "bases"), baseEnvironment, nil, materializeEvaluationGuidance)
	return evaluationAuthority{
		artifactKey: artifactKey, artifactRoot: artifactRoot, credential: credential,
		forjExecutable: forjExecutable, goExecutable: goExecutable, preparer: preparer,
		redactor: credential.Redactor(eval.NewRedactor(nil)), workRoot: workRoot,
	}, nil
}

// Close releases the command-wide immutable base cache after all worker diagnostics have stopped.
func (authority *evaluationAuthority) Close() error {
	if authority == nil {
		return nil
	}
	return errors.Join(authority.preparer.Close(context.Background()), removeEvaluationWorkRoot(authority.workRoot))
}

// evaluationExecution owns one worker's diagnostic, writable project clones, verifier state, home, temporary files, and Go caches.
type evaluationExecution struct {
	diagnostic   *eval.LocalGuidanceDiagnostic
	workRoot     string
	artifactRoot string
	environments map[string][]string
	redactor     eval.Redactor
}

// newEvaluationExecution creates only one worker's private treatment environments from the invocation's frozen authority.
func newEvaluationExecution(invocation evaluationInvocation, authority evaluationAuthority, profiles []string) (execution *evaluationExecution, runErr error) {
	selectedProfiles, err := evaluationProfiles(profiles)
	if err != nil {
		return nil, err
	}
	workRoot, err := newEvaluationWorkRoot()
	if err != nil {
		return nil, err
	}
	defer func() {
		if runErr != nil {
			runErr = cleanupEvaluationSetupFailure(workRoot, runErr)
		}
	}()
	environments := make(map[string][]string, len(selectedProfiles))
	for _, profile := range selectedProfiles {
		environment, environmentErr := evaluationEnvironment(filepath.Join(workRoot, profile), authority.forjExecutable)
		if environmentErr != nil {
			return nil, environmentErr
		}
		environments[profile] = environment
	}
	// Verifiers may read prepared module archives, so their seed must never come from agent-writable treatment state.
	diagnostic, err := eval.NewLocalGuidanceDiagnostic(eval.LocalGuidanceDiagnosticOptions{
		WorkRoot:            workRoot,
		ArtifactRoot:        authority.artifactRoot,
		ArtifactKey:         authority.artifactKey,
		Redactor:            authority.redactor,
		Preparer:            authority.preparer,
		Codex:               eval.CodexOptions{Executable: invocation.CodexExecutable, Model: invocation.Model, ModelProvider: invocation.ModelProvider, Credential: authority.credential},
		GoExecutable:        authority.goExecutable,
		ForjExecutable:      authority.forjExecutable,
		VerifierEnvironment: append([]string(nil), authority.preparer.BaseEnvironment...),
		Runtime:             evaluationRuntimeIdentity(),
	})
	if err != nil {
		return nil, err
	}
	return &evaluationExecution{
		diagnostic: diagnostic, workRoot: workRoot, artifactRoot: authority.artifactRoot,
		environments: environments, redactor: authority.redactor,
	}, nil
}

// cleanupEvaluationSetupFailure preserves cleanup errors alongside the setup error that caused a disposable root to be abandoned.
func cleanupEvaluationSetupFailure(root string, setupErr error) error {
	return cleanupEvaluationSetupFailureWith(root, setupErr, removeEvaluationWorkRoot)
}

// cleanupEvaluationSetupFailureWith retains a setup failure and the result of removing its partially initialized root.
func cleanupEvaluationSetupFailureWith(root string, setupErr error, remove func(string) error) error {
	return errors.Join(setupErr, remove(root))
}

// newEvaluationWorkRoot creates a disposable command workspace in the user cache, independent from retained artifacts and system temporary cleanup.
func newEvaluationWorkRoot() (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve evaluation work-state cache: %w", err)
	}
	return newEvaluationWorkRootAt(cacheRoot)
}

// newEvaluationWorkRootAt creates one disposable command workspace below an explicit cache root.
func newEvaluationWorkRootAt(cacheRoot string) (string, error) {
	workStateRoot, err := resolveEvaluationWorkStateRootAt(cacheRoot)
	if err != nil {
		return "", err
	}
	return os.MkdirTemp(workStateRoot, "command-")
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

// evaluationProfiles validates and de-duplicates the diagnostic treatment set before creating persistent artifacts.
func evaluationProfiles(profiles []string) ([]string, error) {
	selected := make([]string, 0, len(profiles))
	seen := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		if profile != eval.GuidanceProfileNone && profile != eval.GuidanceProfileAgents {
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
	return removeEvaluationWorkRoot(execution.workRoot)
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
func evaluationRuntimeIdentity() eval.RuntimeIdentity {
	return eval.RuntimeIdentity{
		Supervisor: atlasSoftwareIdentity(),
		Framework: eval.SoftwareIdentity{
			Module:  "github.com/goforj/goforj",
			Version: version.Version,
			Commit:  version.Commit,
			Dirty:   version.Dirty,
		},
		GoVersion: runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}
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
func materializeEvaluationGuidance(_ context.Context, prepared eval.PreparedProject, guidance eval.Guidance, selection project.AgentGuidance) (eval.Guidance, error) {
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
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return eval.Guidance{}, fmt.Errorf("read managed evaluation guidance: %w", err)
	}
	result.Files["AGENTS.md"] = body
	return result, nil
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

// resolveEvalCredential requires an explicit disposable credential so diagnostics never silently inherit a developer's normal login.
func resolveEvalCredential(candidate string) (string, error) {
	if strings.TrimSpace(candidate) == "" {
		return "", fmt.Errorf("Codex credential is required; pass a disposable, revocable auth.json with --credential")
	}
	path, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("Codex credential %q is not a regular file", path)
	}
	return path, nil
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
	if len(entries) > 0 {
		marker, err := os.Lstat(filepath.Join(path, ".manifest-key"))
		if err != nil || !marker.Mode().IsRegular() {
			return "", fmt.Errorf("non-empty evaluation artifact root is not owned by Atlas evaluation")
		}
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

// loadOrCreateEvalArtifactKey keeps retained manifests verifiable across diagnostic commands.
func loadOrCreateEvalArtifactKey(root string) ([]byte, error) {
	path := filepath.Join(root, ".manifest-key")
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("evaluation artifact key has invalid length")
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			existing, readErr := os.ReadFile(path)
			if readErr == nil && len(existing) == 32 {
				return existing, nil
			}
		}
		return nil, err
	}
	_, writeErr := file.Write(key)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return nil, errors.Join(err, os.Remove(path))
	}
	return key, nil
}

// readEvalArtifactKey loads the existing root key without creating trust material while reporting retained evidence.
func readEvalArtifactKey(root string) ([]byte, error) {
	key, err := os.ReadFile(filepath.Join(root, ".manifest-key"))
	if err != nil {
		return nil, fmt.Errorf("read evaluation artifact key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("evaluation artifact key has invalid length")
	}
	return key, nil
}

// evaluationEnvironment gives both treatments identical private Go caches and a fixed developer tool surface.
func evaluationEnvironment(workRoot, forjExecutable string) ([]string, error) {
	goExecutable, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("resolve Go executable: %w", err)
	}
	goExecutable, err = filepath.Abs(goExecutable)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute Go executable: %w", err)
	}
	goCache := filepath.Join(workRoot, "gocache")
	moduleCache := filepath.Join(workRoot, "gomodcache")
	goPath := filepath.Join(workRoot, "gopath")
	home := filepath.Join(workRoot, "home")
	temporary := filepath.Join(workRoot, "tmp")
	toolsDir := filepath.Join(workRoot, "tools")
	if err := errors.Join(os.MkdirAll(goCache, 0o700), os.MkdirAll(moduleCache, 0o700), os.MkdirAll(goPath, 0o700), os.MkdirAll(home, 0o700), os.MkdirAll(temporary, 0o700), os.MkdirAll(toolsDir, 0o700)); err != nil {
		return nil, err
	}
	toolsDigest, err := installEvaluationTools(toolsDir, forjExecutable)
	if err != nil {
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
		"GOWORK":                  "off",
		"HOME":                    home,
		"GOTMPDIR":                temporary,
		"PATH":                    toolsDir + string(os.PathListSeparator) + filepath.Dir(goExecutable),
		"TEMP":                    temporary,
		"TMP":                     temporary,
		"TMPDIR":                  temporary,
		"ATLAS_EVAL_TOOLS_DIGEST": toolsDigest,
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
func installEvaluationTools(toolsDir, forjExecutable string) (string, error) {
	hash := sha256.New()
	forjDigest, err := installEvaluationTool(toolsDir, "forj", forjExecutable)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(hash, "forj\x00%s\x00", forjDigest)
	for _, name := range evaluationSupportToolNames() {
		path, err := exec.LookPath(name)
		if err != nil {
			return "", fmt.Errorf("resolve evaluation tool %q: %w", name, err)
		}
		digest, err := installEvaluationTool(toolsDir, name, path)
		if err != nil {
			return "", fmt.Errorf("install evaluation tool %q: %w", name, err)
		}
		fmt.Fprintf(hash, "%s\x00%s\x00", name, digest)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
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

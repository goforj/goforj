package atlas

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/goforj/atlas/eval"
	"github.com/goforj/goforj/project"
)

// TestLoadOrCreateEvalArtifactKeyReusesOnePrivateKey keeps retained manifests verifiable across invocations.
func TestLoadOrCreateEvalArtifactKeyReusesOnePrivateKey(t *testing.T) {
	root := t.TempDir()
	first, err := loadOrCreateEvalArtifactKey(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateEvalArtifactKey(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || !bytes.Equal(first, second) {
		t.Fatalf("artifact keys differ: %x != %x", first, second)
	}
	info, err := os.Stat(filepath.Join(root, ".manifest-key"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("artifact key mode = %o, want 600", info.Mode().Perm())
	}
}

// TestReadEvalArtifactKeyNeverCreatesAuthority keeps report-only commands from changing an artifact root.
func TestReadEvalArtifactKeyNeverCreatesAuthority(t *testing.T) {
	root := t.TempDir()
	if _, err := readEvalArtifactKey(root); err == nil {
		t.Fatal("readEvalArtifactKey() created or accepted a missing key")
	}
	if _, err := os.Stat(filepath.Join(root, ".manifest-key")); !os.IsNotExist(err) {
		t.Fatalf("report key appeared after read: %v", err)
	}
}

// TestValidateEvaluationTrialsBoundsModelSpend prevents accidental zero-work and unbounded diagnostic invocations.
func TestValidateEvaluationTrialsBoundsModelSpend(t *testing.T) {
	for _, trials := range []int{1, 20} {
		if err := validateEvaluationTrials(trials); err != nil {
			t.Fatalf("validateEvaluationTrials(%d): %v", trials, err)
		}
	}
	for _, trials := range []int{0, 21} {
		if err := validateEvaluationTrials(trials); err == nil {
			t.Fatalf("validateEvaluationTrials(%d) succeeded", trials)
		}
	}
}

// TestValidateEvaluationWorkersBoundsConcurrentAgentSessions keeps suite fan-out bounded and explicit.
func TestValidateEvaluationWorkersBoundsConcurrentAgentSessions(t *testing.T) {
	for _, workers := range []int{1, 8} {
		if err := validateEvaluationWorkers(workers); err != nil {
			t.Fatalf("validateEvaluationWorkers(%d): %v", workers, err)
		}
	}
	for _, workers := range []int{0, 9} {
		if err := validateEvaluationWorkers(workers); err == nil {
			t.Fatalf("validateEvaluationWorkers(%d) succeeded", workers)
		}
	}
}

// TestEvaluationComparisonScheduleRetainsPlannedOrderAfterOutOfOrderCompletion keeps worker timing out of result ordering.
func TestEvaluationComparisonScheduleRetainsPlannedOrderAfterOutOfOrderCompletion(t *testing.T) {
	jobs := evaluationComparisonJobs([]string{"catalog-a", "catalog-b"}, 2)
	results := make([]string, len(jobs))
	completion := make([]int, 0, len(jobs))
	var lock sync.Mutex
	release := make([]chan struct{}, len(jobs))
	finished := make([]chan struct{}, len(jobs))
	for index := range jobs {
		release[index] = make(chan struct{})
		finished[index] = make(chan struct{})
	}
	var ready sync.WaitGroup
	ready.Add(len(jobs))
	done := make(chan struct{})
	go func() {
		runEvaluationComparisonSchedule(context.Background(), jobs, len(jobs), func(index int, job evaluationComparisonJob, _ int) {
			ready.Done()
			<-release[index]
			results[index] = job.evaluationID + "/" + string(rune('0'+job.trial))
			lock.Lock()
			completion = append(completion, index)
			lock.Unlock()
			close(finished[index])
		})
		close(done)
	}()
	ready.Wait()
	for index := len(jobs) - 1; index >= 0; index-- {
		close(release[index])
		<-finished[index]
	}
	<-done
	if slices.Equal(completion, []int{0, 1, 2, 3}) {
		t.Fatalf("completion order = %v, want out-of-order completion", completion)
	}
	if got, want := results, []string{"catalog-a/1", "catalog-a/2", "catalog-b/1", "catalog-b/2"}; !slices.Equal(got, want) {
		t.Fatalf("stored result order = %v, want %v", got, want)
	}
}

// TestEvaluationComparisonScheduleStopsDispatchAfterCancellation keeps queued paired trials from starting after interruption.
func TestEvaluationComparisonScheduleStopsDispatchAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := evaluationComparisonJobs([]string{"catalog-a", "catalog-b"}, 2)
	var executed []int
	var lock sync.Mutex
	runEvaluationComparisonSchedule(ctx, jobs, 1, func(index int, _ evaluationComparisonJob, _ int) {
		lock.Lock()
		executed = append(executed, index)
		lock.Unlock()
		cancel()
	})
	if got, want := executed, []int{0}; !slices.Equal(got, want) {
		t.Fatalf("executed jobs = %v, want %v", got, want)
	}
}

// TestRunEvaluationComparisonJobsOrdersOutOfOrderWorkerResults keeps terminal output independent from completion timing.
func TestRunEvaluationComparisonJobsOrdersOutOfOrderWorkerResults(t *testing.T) {
	jobs := evaluationComparisonJobs([]string{"catalog-a", "catalog-b"}, 2)
	release := make([]chan struct{}, len(jobs))
	completed := make([]chan struct{}, len(jobs))
	started := make(chan int, len(jobs))
	for index := range jobs {
		release[index] = make(chan struct{})
		completed[index] = make(chan struct{})
	}
	type output struct {
		results []eval.GuidanceDiagnosticResult
		errors  []error
	}
	done := make(chan output, 1)
	go func() {
		results, runErrors := runEvaluationComparisonJobs(context.Background(), nil, jobs, len(jobs), 2, func(_ context.Context, job evaluationComparisonJob, _ int) (eval.GuidanceDiagnosticResult, []error) {
			index := (job.trial - 1) + map[string]int{"catalog-a": 0, "catalog-b": 2}[job.evaluationID]
			started <- index
			<-release[index]
			close(completed[index])
			return eval.GuidanceDiagnosticResult{LogicalTrialID: string(rune('a' + index))}, []error{fmt.Errorf("error-%d", index)}
		})
		done <- output{results: results, errors: runErrors}
	}()
	for range jobs {
		<-started
	}
	for index := len(jobs) - 1; index >= 0; index-- {
		close(release[index])
		<-completed[index]
	}
	got := <-done
	if trialIDs := []string{got.results[0].LogicalTrialID, got.results[1].LogicalTrialID, got.results[2].LogicalTrialID, got.results[3].LogicalTrialID}; !slices.Equal(trialIDs, []string{"a", "b", "c", "d"}) {
		t.Fatalf("result order = %v", trialIDs)
	}
	if messages := []string{got.errors[0].Error(), got.errors[1].Error(), got.errors[2].Error(), got.errors[3].Error()}; !slices.Equal(messages, []string{"error-0", "error-1", "error-2", "error-3"}) {
		t.Fatalf("error order = %v", messages)
	}
}

// TestRunEvaluationComparisonJobsOmitsUndispatchedResultsAfterCancellation keeps interrupted output free of zero-value slots.
func TestRunEvaluationComparisonJobsOmitsUndispatchedResultsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := evaluationComparisonJobs([]string{"catalog-a", "catalog-b"}, 2)
	results, runErrors := runEvaluationComparisonJobs(ctx, nil, jobs, 1, 2, func(_ context.Context, job evaluationComparisonJob, _ int) (eval.GuidanceDiagnosticResult, []error) {
		cancel()
		return eval.GuidanceDiagnosticResult{LogicalTrialID: job.evaluationID}, nil
	})
	if got, want := len(results), 1; got != want || results[0].LogicalTrialID != "catalog-a" {
		t.Fatalf("results = %#v", results)
	}
	if !errors.Is(errors.Join(runErrors...), context.Canceled) {
		t.Fatalf("run errors = %v, want context cancellation", runErrors)
	}
}

// TestRunEvaluationComparisonJobsNeverReusesActiveWorker keeps each worker execution single-threaded.
func TestRunEvaluationComparisonJobsNeverReusesActiveWorker(t *testing.T) {
	jobs := evaluationComparisonJobs([]string{"catalog-a", "catalog-b"}, 2)
	active := make([]bool, 2)
	started := make(chan struct{}, len(active))
	release := make(chan struct{})
	var lock sync.Mutex
	done := make(chan []error, 1)
	go func() {
		_, runErrors := runEvaluationComparisonJobs(context.Background(), nil, jobs, len(active), 2, func(_ context.Context, job evaluationComparisonJob, worker int) (eval.GuidanceDiagnosticResult, []error) {
			lock.Lock()
			if active[worker] {
				lock.Unlock()
				return eval.GuidanceDiagnosticResult{}, []error{fmt.Errorf("worker %d reused concurrently", worker)}
			}
			active[worker] = true
			lock.Unlock()
			started <- struct{}{}
			<-release
			lock.Lock()
			active[worker] = false
			lock.Unlock()
			return eval.GuidanceDiagnosticResult{LogicalTrialID: job.evaluationID}, nil
		})
		done <- runErrors
	}()
	for range active {
		<-started
	}
	close(release)
	if runErrors := <-done; len(runErrors) != 0 {
		t.Fatalf("run errors = %v", runErrors)
	}
}

// TestRunEvaluationComparisonsWithWorkersStopsBeforeAuthoritySetup keeps cancellation from loading credentials or creating artifacts.
func TestRunEvaluationComparisonsWithWorkersStopsBeforeAuthoritySetup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runEvaluationComparisonsWithWorkers(ctx, evaluationInvocation{}, []string{"catalog-a"}, 1, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runEvaluationComparisonsWithWorkers() error = %v, want context cancellation", err)
	}
}

// TestEvaluationProgressKeepsCountersMonotonicAfterOutOfOrderCompletion keeps concurrent status useful without promising catalog order.
func TestEvaluationProgressKeepsCountersMonotonicAfterOutOfOrderCompletion(t *testing.T) {
	var output bytes.Buffer
	progress := newEvaluationProgress(&output, 2, 1)
	first := evaluationComparisonJob{evaluationID: "catalog-a", trial: 1}
	second := evaluationComparisonJob{evaluationID: "catalog-b", trial: 1}
	progress.started(first)
	progress.started(second)
	progress.finished(second)
	progress.finished(first)
	if got, want := output.String(), "[1/2] catalog-a · trial 1/1\n[2/2] catalog-b · trial 1/1\n[1/2] catalog-b · finished\n[2/2] catalog-a · finished\n"; got != want {
		t.Fatalf("progress output = %q, want %q", got, want)
	}
}

// TestNewEvaluationAuthorityFreezesCredentialRedaction keeps later source mutations out of worker authority.
func TestNewEvaluationAuthorityFreezesCredentialRedaction(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	credential := filepath.Join(root, "auth.json")
	if err := os.WriteFile(credential, []byte(`{"access_token":"first-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := newEvaluationAuthority(evaluationInvocation{Credential: credential, Artifacts: filepath.Join(root, "artifacts")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := authority.Close(); err != nil {
			t.Errorf("close evaluation authority: %v", err)
		}
	})
	if err := os.WriteFile(credential, []byte(`{"access_token":"later-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	redacted := authority.redactor.Text("first-secret later-secret")
	if strings.Contains(redacted, "first-secret") || !strings.Contains(redacted, "later-secret") {
		t.Fatalf("frozen authority redaction = %q", redacted)
	}
}

// TestNewEvaluationWorkRootUsesUserCacheOutsideArtifacts keeps long-running evaluations out of system temporary and artifact-key storage.
func TestNewEvaluationWorkRootUsesUserCacheOutsideArtifacts(t *testing.T) {
	artifactRoot := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Mkdir(artifactRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	workRoot, err := newEvaluationWorkRootAt(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Dir(workRoot), filepath.Join(cacheRoot, "goforj", "atlas-evaluation-work"); got != want {
		t.Fatalf("work state root = %q, want %q", got, want)
	}
	if strings.HasPrefix(workRoot, artifactRoot+string(filepath.Separator)) {
		t.Fatalf("work root %q is inside retained artifacts", workRoot)
	}
	if err := removeEvaluationWorkRoot(workRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workRoot); !os.IsNotExist(err) {
		t.Fatalf("work root survived cleanup: %v", err)
	}
}

// TestResolveEvaluationWorkStateRootRejectsUnsafeExistingTargets keeps disposable state inside a private owned directory.
func TestResolveEvaluationWorkStateRootRejectsUnsafeExistingTargets(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, root, stateRoot string)
	}{
		{
			name: "regular file",
			setup: func(t *testing.T, _ string, stateRoot string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(stateRoot), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(stateRoot, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symbolic link",
			setup: func(t *testing.T, root, stateRoot string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(stateRoot), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(root, "target"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "target"), stateRoot); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "wrong permissions",
			setup: func(t *testing.T, _ string, stateRoot string) {
				t.Helper()
				if err := os.MkdirAll(stateRoot, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && test.name != "regular file" {
				t.Skip("POSIX symlink and permission checks do not apply on Windows")
			}
			root := t.TempDir()
			stateRoot := filepath.Join(root, "goforj", "atlas-evaluation-work")
			test.setup(t, root, stateRoot)
			if _, err := resolveEvaluationWorkStateRootAt(root); err == nil {
				t.Fatal("resolveEvaluationWorkStateRoot() accepted unsafe target")
			}
		})
	}
}

// TestCleanupEvaluationSetupFailureRetainsCleanupError keeps partial construction cleanup failures observable.
func TestCleanupEvaluationSetupFailureRetainsCleanupError(t *testing.T) {
	setupErr := errors.New("setup failed")
	cleanupErr := errors.New("cleanup failed")
	err := cleanupEvaluationSetupFailureWith("partial-root", setupErr, func(string) error { return cleanupErr })
	if !errors.Is(err, setupErr) {
		t.Fatalf("cleanup error = %v, want setup error", err)
	}
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("cleanup error = %v, want cleanup failure", err)
	}
}

// TestBuildEvaluationSuitePlanMakesTotalWorkVisible keeps suite scope tied to promoted manifest budgets.
func TestBuildEvaluationSuitePlanMakesTotalWorkVisible(t *testing.T) {
	plan, err := buildEvaluationSuitePlan([]string{"add-migration", "unknown-framework-shape"}, 2)
	if err != nil {
		t.Fatalf("buildEvaluationSuitePlan(): %v", err)
	}
	if plan.evaluations != 2 || plan.attempts != 8 || plan.wallTime.String() != "40m0s" {
		t.Fatalf("evaluation suite plan = %#v", plan)
	}
	var output bytes.Buffer
	printEvaluationSuitePlan(&output, plan)
	if got, want := output.String(), "Evaluation suite · 2 evaluations · 8 agent sessions · up to 40m0s\n"; got != want {
		t.Fatalf("evaluation suite plan output = %q, want %q", got, want)
	}
}

// TestEvaluationTaskKindPreservesExplicitFiltering keeps all distinct from the promoted task-kind values.
func TestEvaluationTaskKindPreservesExplicitFiltering(t *testing.T) {
	if got := evaluationTaskKind("all"); got != "" {
		t.Fatalf("evaluationTaskKind(all) = %q, want no filter", got)
	}
	if got := evaluationTaskKind("feature"); got != eval.TaskFeature {
		t.Fatalf("evaluationTaskKind(feature) = %q, want %q", got, eval.TaskFeature)
	}
}

// TestEvaluationProfilesRejectsUnsupportedTreatmentsBeforeSetup keeps invocation policy explicit and side-effect free.
func TestEvaluationProfilesRejectsUnsupportedTreatmentsBeforeSetup(t *testing.T) {
	profiles, err := evaluationProfiles([]string{eval.GuidanceProfileAgents, eval.GuidanceProfileAgents, eval.GuidanceProfileNone})
	if err != nil || !slices.Equal(profiles, []string{eval.GuidanceProfileAgents, eval.GuidanceProfileNone}) {
		t.Fatalf("evaluationProfiles() = %v, %v", profiles, err)
	}
	for _, profiles := range [][]string{nil, {"skills"}} {
		if _, err := evaluationProfiles(profiles); err == nil {
			t.Fatalf("evaluationProfiles(%v) succeeded", profiles)
		}
	}
}

// TestEvalReportCmdAuthenticatesSummary keeps retained output behind Atlas's manifest verification.
func TestEvalReportCmdAuthenticatesSummary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	if err := os.WriteFile(filepath.Join(root, ".manifest-key"), key, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := eval.NewArtifactStore(root, key, eval.NewRedactor(nil))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := store.Begin("attempt-report")
	if err != nil {
		t.Fatal(err)
	}
	if err := artifacts.WriteText("summary.txt", "verified summary\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := artifacts.Finalize("sha256:plan", "sha256:baseline", "sha256:final"); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "attempt-report")
	output := captureStdout(t, func() {
		if err := (&EvalReportCmd{Directory: directory}).Run(); err != nil {
			t.Fatalf("EvalReportCmd.Run(): %v", err)
		}
	})
	if output != "verified summary\n" {
		t.Fatalf("report output = %q", output)
	}
	if err := os.WriteFile(filepath.Join(directory, "summary.txt"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&EvalReportCmd{Directory: directory}).Run(); err == nil {
		t.Fatal("EvalReportCmd.Run() accepted tampered evidence")
	}
}

// TestMaterializeEvaluationGuidanceUsesProductionDurablePath keeps evaluation treatments on the same config and marker path used by rendering.
func TestMaterializeEvaluationGuidanceUsesProductionDurablePath(t *testing.T) {
	root := t.TempDir()
	if err := writeProjectGuidanceConfig(filepath.Join(root, ".goforj.yml"), &project.Config{ProjectName: "evaluation-test"}); err != nil {
		t.Fatal(err)
	}
	prepared := evaluationPreparedProject{result: eval.PreparationResult{ProjectRoot: root}}
	agents, err := materializeEvaluationGuidance(context.Background(), prepared, eval.Guidance{Profile: eval.GuidanceProfileAgents}, project.AgentGuidanceBaseline)
	if err != nil {
		t.Fatalf("materializeEvaluationGuidance(agents): %v", err)
	}
	if !strings.Contains(string(agents.Files["AGENTS.md"]), "<!-- goforj-atlas:start -->") {
		t.Fatalf("agents guidance = %q, want production marker", agents.Files["AGENTS.md"])
	}
	config, err := project.LoadProjectConfigAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.Render.AgentGuidance != project.AgentGuidanceBaseline {
		t.Fatalf("agent guidance = %q, want baseline", config.Render.AgentGuidance)
	}
	none, err := materializeEvaluationGuidance(context.Background(), prepared, eval.Guidance{Profile: eval.GuidanceProfileNone}, project.AgentGuidanceNone)
	if err != nil {
		t.Fatalf("materializeEvaluationGuidance(none): %v", err)
	}
	if len(none.Files) != 0 {
		t.Fatalf("none guidance files = %#v", none.Files)
	}
	if content, readErr := os.ReadFile(filepath.Join(root, "AGENTS.md")); readErr == nil && strings.Contains(string(content), "<!-- goforj-atlas:start -->") {
		t.Fatalf("none treatment retained managed guidance: %s", content)
	} else if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read none guidance: %v", readErr)
	}
}

// evaluationPreparedProject supplies the prepared root needed to test the production durable guidance callback.
type evaluationPreparedProject struct {
	result eval.PreparationResult
}

// Result returns the prepared Project identity.
func (project evaluationPreparedProject) Result() eval.PreparationResult {
	return project.result
}

// Close has no resources because this fixture only represents an existing Project root.
func (evaluationPreparedProject) Close(context.Context) error {
	return nil
}

// TestFrozenEvalCredentialRedactsExactAuthority keeps the credential used by both treatments aligned with terminal redaction.
func TestFrozenEvalCredentialRedactsExactAuthority(t *testing.T) {
	credential := filepath.Join(t.TempDir(), "auth.json")
	body := []byte(`{"auth":{"access_token":"access-token-value","token":"token-value"},"provider":{"refresh_token":"refresh-token-value","OPENAI_API_KEY":"api-key-value"}}`)
	if err := os.WriteFile(credential, body, 0o600); err != nil {
		t.Fatal(err)
	}
	frozen, err := eval.LoadCodexCredential(credential)
	if err != nil {
		t.Fatal(err)
	}
	redactor := frozen.Redactor(eval.NewRedactor(nil))
	for _, want := range []string{"access-token-value", "token-value", "refresh-token-value", "api-key-value"} {
		if strings.Contains(redactor.Text(string(body)), want) {
			t.Fatalf("frozen credential redaction retained %q", want)
		}
	}
}

// TestResolveEvalCredentialRequiresExplicitDisposableSource prevents silent use of a developer's normal Codex login.
func TestResolveEvalCredentialRequiresExplicitDisposableSource(t *testing.T) {
	if _, err := resolveEvalCredential(""); err == nil || !strings.Contains(err.Error(), "disposable") {
		t.Fatalf("resolveEvalCredential() error = %v, want explicit disposable credential requirement", err)
	}
}

// TestResolveEvalArtifactRootRejectsUnownedExistingContent prevents broad caller paths from becoming artifact stores.
func TestResolveEvalArtifactRootRejectsUnownedExistingContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveEvalArtifactRoot(root); err == nil || !strings.Contains(err.Error(), "not owned") {
		t.Fatalf("resolveEvalArtifactRoot() error = %v, want unowned-root rejection", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("artifact root permissions changed to %o", info.Mode().Perm())
	}
}

// TestMarshalRedactedEvaluationKeepsSecretsOutOfTerminalJSON applies the retained-artifact boundary to command output.
func TestMarshalRedactedEvaluationKeepsSecretsOutOfTerminalJSON(t *testing.T) {
	secret := "terminal-secret-value"
	body, err := marshalRedactedEvaluation(map[string]any{"error": "token=" + secret}, eval.NewRedactor([]string{secret}))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(secret)) || !bytes.Contains(body, []byte("[REDACTED]")) {
		t.Fatalf("redacted terminal JSON = %s", body)
	}
}

// TestEvaluationEnvironmentUsesPrivateCachesAndCopiedForj keeps host workspaces and the source executable outside agent ownership.
func TestEvaluationEnvironmentUsesPrivateCachesAndCopiedForj(t *testing.T) {
	root := t.TempDir()
	hostTools := filepath.Join(root, "host-tools")
	if err := os.Mkdir(hostTools, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{"go"}, evaluationSupportToolNames()...) {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(hostTools, name), []byte(name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", hostTools)
	t.Setenv("ATLAS_EVAL_SECRET", "must-not-leak")
	t.Setenv("GOENV", filepath.Join(root, "host-go-env"))
	t.Setenv("GOFLAGS", "-modfile=host.mod")
	t.Setenv("GOPROXY", "https://credential@private.example.invalid")
	t.Setenv("GOPRIVATE", "private.example.invalid")
	source := filepath.Join(root, "source-forj")
	if err := os.WriteFile(source, []byte("candidate"), 0o700); err != nil {
		t.Fatal(err)
	}
	environment, err := evaluationEnvironment(filepath.Join(root, "attempt"), source)
	if err != nil {
		t.Fatal(err)
	}
	values := environmentValues(environment)
	for key, suffix := range map[string]string{"GOCACHE": "gocache", "GOMODCACHE": "gomodcache", "GOPATH": "gopath", "HOME": "home", "GOTMPDIR": "tmp", "TEMP": "tmp", "TMP": "tmp", "TMPDIR": "tmp"} {
		if values[key] != filepath.Join(root, "attempt", suffix) {
			t.Fatalf("%s = %q", key, values[key])
		}
	}
	secondEnvironment, err := evaluationEnvironment(filepath.Join(root, "second-attempt"), source)
	if err != nil {
		t.Fatal(err)
	}
	secondValues := environmentValues(secondEnvironment)
	for _, key := range []string{"GOTMPDIR", "TEMP", "TMP", "TMPDIR"} {
		if values[key] == secondValues[key] {
			t.Fatalf("%s shared temporary state: %q", key, values[key])
		}
	}
	if values["GOWORK"] != "off" {
		t.Fatalf("GOWORK = %q", values["GOWORK"])
	}
	if !strings.HasPrefix(values["ATLAS_EVAL_TOOLS_DIGEST"], "sha256:") {
		t.Fatalf("ATLAS_EVAL_TOOLS_DIGEST = %q", values["ATLAS_EVAL_TOOLS_DIGEST"])
	}
	for key, want := range map[string]string{"GOENV": "off", "GOPROXY": "https://proxy.golang.org,direct", "GOSUMDB": "sum.golang.org", "GOTOOLCHAIN": "local"} {
		if values[key] != want {
			t.Fatalf("%s = %q, want %q", key, values[key], want)
		}
	}
	for _, key := range []string{"GOFLAGS", "GOPRIVATE"} {
		if _, leaked := values[key]; leaked {
			t.Fatalf("ambient %s reached the evaluation process", key)
		}
	}
	if _, leaked := values["ATLAS_EVAL_SECRET"]; leaked {
		t.Fatal("ambient credential-like environment reached the evaluation process")
	}
	toolsDir := filepath.Join(root, "attempt", "tools")
	if !strings.HasPrefix(values["PATH"], toolsDir+string(os.PathListSeparator)) {
		t.Fatalf("PATH = %q", values["PATH"])
	}
	name := "forj"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	copyPath := filepath.Join(toolsDir, name)
	if err := os.Chmod(copyPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath, []byte("mutated"), 0o500); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "candidate" {
		t.Fatalf("source executable changed through private copy: %q", body)
	}
	for _, tool := range evaluationSupportToolNames() {
		if runtime.GOOS == "windows" {
			tool += ".exe"
		}
		if _, err := os.Stat(filepath.Join(toolsDir, tool)); err != nil {
			t.Fatalf("snapshotted tool %q: %v", tool, err)
		}
	}
}

// TestEvaluationSupportToolsIncludeCodexInterpreter keeps script-based Codex launchers usable inside the fixed tool snapshot.
func TestEvaluationSupportToolsIncludeCodexInterpreter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Windows Codex launcher does not use an env-resolved Node interpreter")
	}
	if !slices.Contains(evaluationSupportToolNames(), "node") {
		t.Fatal("evaluation support tools omit the Codex Node interpreter")
	}
}

// TestEvaluationSupportToolsIncludePortableShell protects generated tests that invoke the POSIX shell by name.
func TestEvaluationSupportToolsIncludePortableShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("generated Windows tests do not invoke the POSIX shell")
	}
	if !slices.Contains(evaluationSupportToolNames(), "sh") {
		t.Fatal("evaluation support tools omit the POSIX shell")
	}
}

// TestRemoveEvaluationWorkRootHandlesReadOnlyModuleDirectories keeps successful diagnostics from failing during cache cleanup.
func TestRemoveEvaluationWorkRootHandlesReadOnlyModuleDirectories(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "owned")
	directory := filepath.Join(root, "gomodcache", "module")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.com/test\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(directory, 0o555); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeEvaluationWorkRoot(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("evaluation root survived cleanup: %v", err)
	}
	if err := removeEvaluationWorkRoot(root); err != nil {
		t.Fatalf("idempotent cleanup: %v", err)
	}
}

// TestEvaluationRuntimeIdentityRecordsReconstructableComponents keeps retained attempts independent from deleted command workspaces.
func TestEvaluationRuntimeIdentityRecordsReconstructableComponents(t *testing.T) {
	identity := evaluationRuntimeIdentity()
	if identity.Framework.Module != "github.com/goforj/goforj" || identity.Framework.Version == "" || identity.Supervisor.Module != "github.com/goforj/atlas" || identity.Supervisor.Version == "" {
		t.Fatalf("runtime identity is incomplete: %#v", identity)
	}
	if identity.GoVersion == "" || identity.GOOS != runtime.GOOS || identity.GOARCH != runtime.GOARCH {
		t.Fatalf("Go runtime identity is incomplete: %#v", identity)
	}
}

// environmentValues converts one process environment into its effective key/value map for focused assertions.
func environmentValues(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

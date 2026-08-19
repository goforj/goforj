//go:build integration

package atlaseval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/goforj/atlas/eval"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/scenarios"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestPreparerMaterializesEveryPromotedStartingState verifies every promoted fixture satisfies its declared pre-target contract without invoking target checks.
func TestPreparerMaterializesEveryPromotedStartingState(t *testing.T) {
	ids, err := eval.PromotedEvaluationIDs("")
	if err != nil {
		t.Fatalf("PromotedEvaluationIDs(): %v", err)
	}
	forjExecutable := testkit.EnsureIntegrationForjBinary(t)
	environment := testkit.ProcessGoEnv("", nil)
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			definition, err := eval.LoadPromotedDefinition(id)
			if err != nil {
				t.Fatalf("LoadPromotedDefinition(): %v", err)
			}
			request := eval.PreparationRequest{
				ScenarioID:      definition.ProjectScenario,
				DestinationRoot: filepath.Join(t.TempDir(), "project"),
				ForjExecutable:  forjExecutable,
				OrchestrationID: "integration-" + id,
				Environment:     environment,
			}
			preparer := Preparer{}
			plan, err := preparer.Resolve(context.Background(), request)
			if err != nil {
				t.Fatalf("Resolve(): %v", err)
			}
			if !plan.TargetOmitted {
				t.Fatalf("plan includes target work: %#v", plan)
			}
			project, err := preparer.Prepare(context.Background(), request, plan)
			if err != nil {
				t.Fatalf("Prepare(): %v", err)
			}
			t.Cleanup(func() { _ = project.Close(context.Background()) })
			if project.Result().BaselineTree == "" || project.Result().ForjDigest == "" {
				t.Fatalf("preparation result = %#v", project.Result())
			}
			assertPreparedGoSourcesCanonical(t, project.Result().ProjectRoot)
			if id == "add-http-controller" {
				controller := filepath.Join(project.Result().ProjectRoot, "internal", "invoices", "controller.go")
				if _, err := os.Stat(controller); !os.IsNotExist(err) {
					t.Fatalf("target controller leaked into preparation: %v", err)
				}
			}
		})
	}
}

// assertPreparedGoSourcesCanonical prevents agent formatting from appearing as task behavior in retained diffs.
func assertPreparedGoSourcesCanonical(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "_data", "bin", "build", "node_modules":
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		formatted, err := format.Source(body)
		if err != nil {
			return fmt.Errorf("format %s: %w", path, err)
		}
		if !bytes.Equal(body, formatted) {
			return fmt.Errorf("prepared Go source %s is not canonical", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestPreparerClonesOneIdenticalBaseForPairedTreatments protects the comparison's single-variable invariant.
func TestPreparerClonesOneIdenticalBaseForPairedTreatments(t *testing.T) {
	workRoot := t.TempDir()
	baseRoot := filepath.Join(workRoot, "bases")
	trialBuildCache := filepath.Join(workRoot, "trial-gocache")
	baseEnvironment := testkit.ProcessGoEnv("", nil)
	trialEnvironment := testkit.ProcessGoEnv("", map[string]string{"GOCACHE": trialBuildCache})
	preparer := NewPreparer(baseRoot, baseEnvironment, nil, nil)
	t.Cleanup(func() {
		if err := preparer.Close(context.Background()); err != nil {
			t.Fatalf("Close preparer: %v", err)
		}
		entries, err := os.ReadDir(baseRoot)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read base root after Close: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("prepared bases survived Close: %v", entries)
		}
	})
	request := eval.PreparationRequest{
		ScenarioID:      "invoice-http-route",
		DestinationRoot: filepath.Join(workRoot, "projects"),
		ForjExecutable:  testkit.EnsureIntegrationForjBinary(t),
		OrchestrationID: "paired-none",
		Environment:     trialEnvironment,
	}
	plan, err := preparer.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	none, err := preparer.Prepare(context.Background(), request, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = none.Close(context.Background()) })
	request.OrchestrationID = "paired-agents"
	plan, err = preparer.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	agents, err := preparer.Prepare(context.Background(), request, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agents.Close(context.Background()) })
	if none.Result().ProjectRoot == agents.Result().ProjectRoot {
		t.Fatal("paired treatments shared one mutable Project")
	}
	preparer.cache.mu.Lock()
	baseCount := len(preparer.cache.bases)
	preparer.cache.mu.Unlock()
	if baseCount != 1 {
		t.Fatalf("prepared base count = %d, want one command-local materialization", baseCount)
	}
	if none.Result().BaselineTree != agents.Result().BaselineTree {
		t.Fatalf("paired baseline trees differ: %s != %s", none.Result().BaselineTree, agents.Result().BaselineTree)
	}
	baseBuildCache := integrationEnvironmentValues(preparer.BaseEnvironment)["GOCACHE"]
	if entries, err := os.ReadDir(baseBuildCache); err != nil || len(entries) == 0 {
		t.Fatalf("base preparation did not use its private build cache: entries=%v error=%v", entries, err)
	}
	if entries, err := os.ReadDir(trialBuildCache); err == nil && len(entries) > 0 {
		t.Fatalf("preparation warmed the candidate build cache: %v", entries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read candidate build cache: %v", err)
	}
}

// TestPreparerDistinctPlansReuseTheCommandPreparationSeed proves serialized bases warm one trusted cache without touching treatment caches.
func TestPreparerDistinctPlansReuseTheCommandPreparationSeed(t *testing.T) {
	workRoot := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.WalkDir(workRoot, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0o700)
			}
			return err
		})
	})
	baseValues := map[string]string{}
	for _, key := range []string{"GOCACHE", "GOMODCACHE", "GOPATH", "HOME", "GOTMPDIR", "TMPDIR", "TMP", "TEMP"} {
		baseValues[key] = filepath.Join(workRoot, strings.ToLower(key))
		if err := os.MkdirAll(baseValues[key], 0o700); err != nil {
			t.Fatal(err)
		}
	}
	baseEnvironment := testkit.ProcessGoEnv("", baseValues)
	requests := []eval.PreparationRequest{
		{ScenarioID: "invoice-http-route", DestinationRoot: filepath.Join(workRoot, "http-route"), ForjExecutable: testkit.EnsureIntegrationForjBinary(t), OrchestrationID: "distinct-http", Environment: testkit.ProcessGoEnv("", nil)},
		{ScenarioID: "invoice-domain", DestinationRoot: filepath.Join(workRoot, "domain"), ForjExecutable: testkit.EnsureIntegrationForjBinary(t), OrchestrationID: "distinct-domain", Environment: testkit.ProcessGoEnv("", nil)},
	}
	preparer := NewPreparerWithCapacity(filepath.Join(workRoot, "bases"), baseEnvironment, nil, nil, len(requests))
	t.Cleanup(func() {
		if err := preparer.Close(context.Background()); err != nil {
			t.Errorf("close preparer: %v", err)
		}
	})
	plans := make([]eval.ResolvedPreparationPlan, len(requests))
	for index, request := range requests {
		plan, err := preparer.Resolve(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		plans[index] = plan
	}
	type preparedResult struct {
		project eval.PreparedProject
		err     error
	}
	results := make(chan preparedResult, len(requests))
	for index := range requests {
		go func(index int) {
			project, err := preparer.Prepare(context.Background(), requests[index], plans[index])
			results <- preparedResult{project: project, err: err}
		}(index)
	}
	for range requests {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		t.Cleanup(func() { _ = result.project.Close(context.Background()) })
	}
	preparer.cache.mu.Lock()
	baseCount := len(preparer.cache.bases)
	preparer.cache.mu.Unlock()
	if baseCount != len(requests) {
		t.Fatalf("prepared base count = %d, want %d distinct plans", baseCount, len(requests))
	}
	if entries, err := os.ReadDir(baseValues["GOCACHE"]); err != nil || len(entries) == 0 {
		t.Fatalf("distinct preparations did not warm the command build cache: entries=%v error=%v", entries, err)
	}
	for _, request := range requests {
		requestValues := integrationEnvironmentValues(request.Environment)
		if requestValues["GOCACHE"] == baseValues["GOCACHE"] {
			t.Fatal("treatment environment unexpectedly owns the trusted preparation cache")
		}
	}
}

// integrationEnvironmentValues converts process environment entries into a lookup map for integration assertions.
func integrationEnvironmentValues(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

// TestPreparerDelegatesDurableGuidanceTreatment proves the scenario adapter delegates native instruction ownership to its host.
func TestPreparerDelegatesDurableGuidanceTreatment(t *testing.T) {
	workRoot := t.TempDir()
	var selections []project.AgentGuidance
	preparer := NewPreparer(filepath.Join(workRoot, "bases"), testkit.ProcessGoEnv("", nil), nil, func(_ context.Context, prepared eval.PreparedProject, guidance eval.Guidance, selection project.AgentGuidance) (eval.Guidance, error) {
		selections = append(selections, selection)
		if prepared == nil || prepared.Result().ProjectRoot == "" {
			return eval.Guidance{}, errors.New("prepared Project is required")
		}
		result := guidance
		result.Files = map[string][]byte{}
		if selection == project.AgentGuidanceBaseline {
			result.Files["AGENTS.md"] = []byte("<!-- host-managed -->")
		}
		return result, nil
	})
	t.Cleanup(func() { _ = preparer.Close(context.Background()) })
	request := eval.PreparationRequest{
		ScenarioID:      "invoice-http-route",
		DestinationRoot: filepath.Join(workRoot, "project"),
		ForjExecutable:  testkit.EnsureIntegrationForjBinary(t),
		OrchestrationID: "guidance-treatment",
		Environment:     testkit.ProcessGoEnv("", nil),
	}
	plan, err := preparer.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := preparer.Prepare(context.Background(), request, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close(context.Background()) })
	before := prepared.Result().BaselineTree
	agents, err := preparer.MaterializeGuidance(context.Background(), prepared, eval.Guidance{Profile: eval.GuidanceProfileAgents})
	if err != nil {
		t.Fatalf("MaterializeGuidance(agents): %v", err)
	}
	if string(agents.Files["AGENTS.md"]) == "" || !strings.Contains(string(agents.Files["AGENTS.md"]), "<!-- host-managed -->") {
		t.Fatalf("agents guidance did not retain host materialization: %q", agents.Files["AGENTS.md"])
	}
	if prepared.Result().BaselineTree != before {
		t.Fatalf("guidance treatment changed pre-guidance preparation identity: %s != %s", prepared.Result().BaselineTree, before)
	}
	none, err := preparer.MaterializeGuidance(context.Background(), prepared, eval.Guidance{Profile: eval.GuidanceProfileNone})
	if err != nil {
		t.Fatalf("MaterializeGuidance(none): %v", err)
	}
	if len(none.Files) != 0 {
		t.Fatalf("none guidance files = %#v", none.Files)
	}
	if !slices.Equal(selections, []project.AgentGuidance{project.AgentGuidanceBaseline, project.AgentGuidanceNone}) {
		t.Fatalf("guidance selections = %v", selections)
	}
}

// invoiceHTTPVerifierFixture keeps the real Project, verifier, and materialization inputs together across independent calibration tests.
type invoiceHTTPVerifierFixture struct {
	projectRoot    string
	verifier       *eval.AddHTTPControllerVerifier
	forjExecutable string
	environment    []string
}

// TestInvoiceHTTPGoldenVerifierCalibration proves the diagnostic executes the hidden behavior probe for the generated controller.
func TestInvoiceHTTPGoldenVerifierCalibration(t *testing.T) {
	fixture := prepareInvoiceHTTPVerifierFixture(t)
	result, err := fixture.verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: fixture.projectRoot})
	if err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if result.FrameworkOutcome.Status != eval.EndpointPassed {
		t.Fatalf("framework outcome = %#v; checks = %#v", result.FrameworkOutcome, result.Checks)
	}
	if !evaluationCheckHasStatus(result.Checks, "invoice-behavior", eval.EndpointPassed) {
		t.Fatalf("hidden behavior probe did not complete: %#v", result.Checks)
	}
}

// TestInvoiceHTTPMutantVerifierCalibration proves the hidden behavior oracle rejects a plausible wrong response.
func TestInvoiceHTTPMutantVerifierCalibration(t *testing.T) {
	fixture := prepareInvoiceHTTPVerifierFixture(t)
	controllerPath := filepath.Join(fixture.projectRoot, "internal", "invoices", "controller.go")
	controller, err := os.ReadFile(controllerPath)
	if err != nil {
		t.Fatalf("read controller: %v", err)
	}
	mutant := strings.Replace(string(controller), "return request.JSON(http.StatusOK, invoice)", `return request.JSON(http.StatusOK, map[string]string{"id": "wrong"})`, 1)
	if mutant == string(controller) {
		t.Fatal("invoice behavior mutant did not apply")
	}
	if err := os.WriteFile(controllerPath, []byte(mutant), 0o644); err != nil {
		t.Fatalf("write controller mutant: %v", err)
	}
	result, err := fixture.verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: fixture.projectRoot})
	if err != nil {
		t.Fatalf("Verify(mutant): %v", err)
	}
	if !evaluationCheckHasStatus(result.Checks, "invoice-behavior", eval.EndpointFailed) {
		t.Fatalf("independent behavior oracle accepted wrong response: %#v", result.Checks)
	}
}

// TestInvoiceHTTPTransportVerifierCalibration proves the semantic contract accepts an independently valid transport package layout.
func TestInvoiceHTTPTransportVerifierCalibration(t *testing.T) {
	fixture := prepareInvoiceHTTPVerifierFixture(t)
	materializeTransportInvoiceController(t, fixture.projectRoot, fixture.forjExecutable, fixture.environment)
	result, err := fixture.verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: fixture.projectRoot})
	if err != nil {
		t.Fatalf("Verify(transport family): %v", err)
	}
	if result.FrameworkOutcome.Status != eval.EndpointPassed || !evaluationCheckHasStatus(result.Checks, "invoice-behavior", eval.EndpointPassed) {
		t.Fatalf("transport-package implementation family failed: outcome=%#v checks=%#v", result.FrameworkOutcome, result.Checks)
	}
}

// prepareInvoiceHTTPVerifierFixture builds one real scenario per top-level calibration so the integration runner can shard each family independently.
func prepareInvoiceHTTPVerifierFixture(t *testing.T) invoiceHTTPVerifierFixture {
	t.Helper()
	t.Parallel()
	workRoot := t.TempDir()
	forjExecutable := testkit.EnsureIntegrationForjBinary(t)
	environment := testkit.IntegrationGoProcessEnv(t, nil)
	if err := scenarios.Validate(scenarios.ValidateOptions{
		Logger:      logger.NewAppLogger(),
		WorkDir:     workRoot,
		Keep:        true,
		IDs:         []string{"invoice-http-route"},
		ForjExec:    forjExecutable,
		Environment: environment,
	}); err != nil {
		t.Fatalf("build invoice scenario: %v", err)
	}
	entries, err := os.ReadDir(workRoot)
	if err != nil {
		t.Fatalf("read scenario work root: %v", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("scenario workspaces = %#v, want one directory", entries)
	}
	projectRoot := filepath.Join(workRoot, entries[0].Name())
	verifier := eval.NewAddHTTPControllerVerifier(eval.VerifierCommands{
		WorkRoot:       t.TempDir(),
		ForjExecutable: forjExecutable,
		Environment:    environment,
	})
	return invoiceHTTPVerifierFixture{projectRoot: projectRoot, verifier: verifier, forjExecutable: forjExecutable, environment: environment}
}

// majorSurfaceVerifierCase binds one promoted contract to its executable golden Project and a targeted semantic defect.
type majorSurfaceVerifierCase struct {
	evaluation           string
	scenario             string
	path                 string
	old                  string
	mutant               string
	additional           []evaluationMutation
	behavior             *evaluationBehaviorMutation
	alternates           []evaluationFileMutation
	alternateRenames     []evaluationPathRename
	disconnectedProvider string
	wantFailed           string
	wantCompile          bool
}

// evaluationBehaviorMutation preserves structural evidence while violating the supervisor-owned runtime oracle.
type evaluationBehaviorMutation struct {
	path        string
	old         string
	mutant      string
	wantFailed  string
	wantCompile bool
}

// evaluationMutation completes a compiling semantic mutant when one replacement cannot preserve source validity.
type evaluationMutation struct {
	old    string
	mutant string
}

// evaluationFileMutation describes one reversible source change used to prove a verifier accepts an independently valid implementation family.
type evaluationFileMutation struct {
	path        string
	old         string
	replacement string
}

// evaluationPathRename moves a valid source without changing its package behavior.
type evaluationPathRename struct {
	from string
	to   string
}

// TestAddAppCommandVerifierCalibration proves the App command contract accepts its golden Project and rejects lost cancellation.
func TestAddAppCommandVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-app-command",
		scenario:   "invoice-app-command",
		path:       "internal/invoices/show_cmd.go",
		old:        "command.service.Find(ctx, command.ID)",
		mutant:     "command.service.Find(context.Background(), command.ID)",
		behavior: &evaluationBehaviorMutation{
			path:       "internal/invoices/show_cmd.go",
			old:        `fmt.Printf("%s %d\n", invoice.ID, invoice.TotalCents)`,
			mutant:     `_ = invoice; fmt.Printf("%s %d\n", command.ID, 12500)`,
			wantFailed: "command-behavior-variable",
		},
		wantFailed:  "command-shape",
		wantCompile: true,
	})
}

// TestAddJobVerifierCalibration proves the job contract requires its typed payload identity.
func TestAddJobVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-job",
		scenario:   "invoice-receipt-job",
		path:       "internal/invoices/receipt_job.go",
		old:        "InvoiceID string",
		mutant:     "Reference string",
		additional: []evaluationMutation{{old: "payload.InvoiceID", mutant: "payload.Reference"}},
		behavior: &evaluationBehaviorMutation{
			path:       "internal/invoices/receipt_job.go",
			old:        "job.service.Find(ctx, payload.InvoiceID)",
			mutant:     "func() (Invoice, error) { _ = payload.InvoiceID; return job.service.Find(ctx, \"missing\") }()",
			wantFailed: "receipt-job-behavior",
		},
		wantFailed:  "typed-job",
		wantCompile: true,
	})
}

// TestAddMigrationVerifierCalibration proves the migration contract accepts formatting variants while rejecting the wrong schema effect.
func TestAddMigrationVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-migration",
		scenario:   "invoice-status-migration",
		path:       "migrations/*_add_status_to_invoices.up.sql",
		old:        "status",
		mutant:     "state",
		alternates: []evaluationFileMutation{
			{path: "migrations/*_add_status_to_invoices.up.sql", old: "ALTER TABLE invoices ADD COLUMN status TEXT NOT NULL DEFAULT 'open';", replacement: "ALTER TABLE \"invoices\" ADD IF NOT EXISTS `status` TEXT;"},
			{path: "migrations/*_add_status_to_invoices.down.sql", old: "ALTER TABLE invoices DROP COLUMN status;", replacement: "ALTER TABLE [invoices] DROP status;"},
		},
		wantFailed: "migration-up",
	})
}

// TestAddScheduleVerifierCalibration proves the schedule contract rejects work detached from its runtime context.
func TestAddScheduleVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation:  "add-schedule",
		scenario:    "invoice-reconcile-schedule",
		path:        "internal/invoices/reconcile_schedule.go",
		old:         "schedule.service.Find(ctx, \"inv-42\")",
		mutant:      "schedule.service.Find(context.Background(), \"inv-42\")",
		wantFailed:  "schedule-shape",
		wantCompile: true,
		behavior: &evaluationBehaviorMutation{
			path:       "internal/invoices/reconcile_schedule.go",
			old:        `schedule.service.Find(ctx, "inv-42")`,
			mutant:     `schedule.service.Find(ctx, "missing")`,
			wantFailed: "reconcile-schedule-behavior",
		},
		alternateRenames: []evaluationPathRename{{from: "app/wire/inject_schedules_app.go", to: "app/wire/schedules.go"}},
	})
}

// TestAddEventSubscriberVerifierCalibration proves the subscriber contract rejects work detached from its event context.
func TestAddEventSubscriberVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation:  "add-event-subscriber",
		scenario:    "invoice-paid-subscriber",
		path:        "internal/invoices/paid_subscriber.go",
		old:         "subscriber.service.Find(ctx, event.InvoiceID)",
		mutant:      "subscriber.service.Find(context.Background(), event.InvoiceID)",
		wantFailed:  "subscriber-boundary",
		wantCompile: true,
		behavior: &evaluationBehaviorMutation{
			path:       "internal/invoices/paid_subscriber.go",
			old:        "subscriber.service.Find(ctx, event.InvoiceID)",
			mutant:     "func() (Invoice, error) { _ = event.InvoiceID; return subscriber.service.Find(ctx, \"inv-42\") }()",
			wantFailed: "paid-subscriber-behavior",
		},
	})
}

// TestCreateModelVerifierCalibration proves the model contract requires the database-derived field shape.
func TestCreateModelVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{evaluation: "create-model", scenario: "create-user-model", path: "internal/models/user.go", old: "Email", mutant: "EmailAddress", wantFailed: "model-shape"})
}

// TestModelRelationshipsVerifierCalibration proves the generated relationship remains attached to its schema key contract.
func TestModelRelationshipsVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{evaluation: "model-relationships", scenario: "user-post-relationships", path: ".db-relationships.yaml", old: "1-many id->posts:user_id", mutant: "1-many id->posts:id", wantFailed: "relationship-contract"})
}

// TestAddNamedAppRouteVerifierCalibration proves additional-App routing stays attached to the selected App.
func TestAddNamedAppRouteVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{evaluation: "add-named-app-route", scenario: "admin-audit-route", path: "internal/audits/controller.go", old: "/api/v1/audits", mutant: "/api/v1/wrong", wantFailed: "admin-route-visible"})
}

// TestAddNamedResourceVerifierCalibration proves named queues are resolved through their generated manager accessor.
func TestAddNamedResourceVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-named-resource",
		scenario:   "named-reports-queue",
		path:       "internal/invoices/report_dispatcher.go",
		old:        "manager.Reports()",
		mutant:     "manager.Default()",
		behavior: &evaluationBehaviorMutation{
			path:       "internal/invoices/report_dispatcher.go",
			old:        `queue.NewJob("reports:generate")`,
			mutant:     `queue.NewJob("reports:wrong")`,
			wantFailed: "named-queue-behavior",
		},
		disconnectedProvider: "invoices.NewService",
		wantFailed:           "named-queue-injection",
	})
}

// TestAddNamedCacheVerifierCalibration proves named caches are resolved through their generated manager accessor.
func TestAddNamedCacheVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-named-cache",
		scenario:   "named-profiles-cache",
		path:       "internal/invoices/profile_cache.go",
		old:        "manager.Profiles()",
		mutant:     "manager.Default()",
		behavior: &evaluationBehaviorMutation{
			path:       "internal/invoices/profile_cache.go",
			old:        "SetString(key, value, 0)",
			mutant:     `SetString("wrong:"+key, value, 0)`,
			wantFailed: "named-cache-behavior",
		},
		disconnectedProvider: "invoices.NewService",
		wantFailed:           "named-cache-injection",
	})
}

// TestAddNamedStorageVerifierCalibration proves named storage is resolved through its generated manager accessor.
func TestAddNamedStorageVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-named-storage",
		scenario:   "named-avatar-storage",
		path:       "internal/invoices/avatar_storage.go",
		old:        "manager.Avatars()",
		mutant:     "manager.Default()",
		behavior: &evaluationBehaviorMutation{
			path:       "internal/invoices/avatar_storage.go",
			old:        "Put(path, body)",
			mutant:     `Put("wrong/"+path, body)`,
			wantFailed: "named-storage-behavior",
		},
		disconnectedProvider: "invoices.NewService",
		wantFailed:           "named-storage-injection",
	})
}

// TestChooseStorageForFilesVerifierCalibration proves an inferred durable file category uses its purpose-named accessor.
func TestChooseStorageForFilesVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "choose-storage-for-files",
		scenario:   "invoice-attachments",
		path:       "internal/invoices/attachments.go",
		old:        "manager.Attachments()",
		mutant:     "manager.Default()",
		behavior: &evaluationBehaviorMutation{
			path:       "internal/invoices/attachments.go",
			old:        "return body, nil",
			mutant:     "return append(body, '!'), nil",
			wantFailed: "attachment-storage-behavior",
		},
		wantFailed: "attachment-service-boundary",
	})
}

// TestServeCacheableImageVerifierCalibration proves a successful image response retains conditional revalidation.
func TestServeCacheableImageVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "serve-cacheable-image",
		scenario:   "cacheable-avatar-response",
		path:       "internal/avatars/controller.go",
		old:        `"If-None-Match"`,
		mutant:     `"X-Ignored-Validator"`,
		behavior: &evaluationBehaviorMutation{
			path:       "internal/avatars/controller.go",
			old:        "return request.NoContent(http.StatusNotModified)",
			mutant:     "return request.NoContent(http.StatusOK)",
			wantFailed: "avatar-revalidation-behavior",
		},
		wantFailed: "avatar-revalidation",
	})
}

// TestRepairWireProviderVerifierCalibration proves a repaired service provider remains in the intended Wire set and generated output is not hand-edited.
func TestRepairWireProviderVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "repair-wire-provider",
		scenario:   "repair-report-wire-provider",
		path:       "app/wire/inject_services_app.go",
		old:        "reports.NewService",
		mutant:     "app.NewLifecycleRegistry",
		wantFailed: "provider-registration",
		behavior: &evaluationBehaviorMutation{
			path:       "app/wire/wire_gen.go",
			old:        "// Code generated by Wire. DO NOT EDIT.",
			mutant:     "// manually edited Wire output",
			wantFailed: "wire-output-parity",
		},
	})
}

// TestRuntimeObservabilityVerifierCalibration proves a repair enables the inspect manager used by Lighthouse.
func TestRuntimeObservabilityVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "runtime-observability",
		scenario:   "runtime-observability",
		path:       ".env.local",
		old:        "LIGHTHOUSE_INSPECT_ENABLED=true",
		mutant:     "LIGHTHOUSE_INSPECT_ENABLED=0",
		wantFailed: "local-inspect-capture-behavior",
	})
}

// TestBuildJSONAPIFeatureVerifierCalibration proves the complete API contract rejects lookup detached from request cancellation.
func TestBuildJSONAPIFeatureVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "build-json-api-feature",
		scenario:   "json-api-route",
		path:       "internal/users/controller.go",
		old:        "c.service.Find(ctx.Context(), ctx.Param(\"id\"))",
		mutant:     "c.service.Find(context.Background(), ctx.Param(\"id\"))",
		additional: []evaluationMutation{{old: `"errors"`, mutant: "\"context\"\n\t\"errors\""}},
		alternates: []evaluationFileMutation{
			{path: "internal/users/controller.go", old: "Show", replacement: "Get"},
		},
		behavior: &evaluationBehaviorMutation{
			path:       "internal/users/service.go",
			old:        `if id != "42"`,
			mutant:     `if id == ""`,
			wantFailed: "json-api-behavior",
		},
		wantFailed:  "users-application-boundary",
		wantCompile: true,
	})
}

// TestAddCachedRepositoryVerifierCalibration proves cache-aside lookup uses the requested named cache.
func TestAddCachedRepositoryVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-cached-repository",
		scenario:   "cached-user-profile",
		path:       "app/wire/inject_services_app.go",
		old:        "manager.Profiles()",
		mutant:     "manager.Default()",
		behavior: &evaluationBehaviorMutation{
			path:       "internal/users/repository.go",
			old:        "cache.Set(cacheForRequest, key, user, profileCacheTTL)",
			mutant:     `cache.Set(cacheForRequest, "wrong:"+key, user, profileCacheTTL)`,
			wantFailed: "cache-aside-behavior",
		},
		wantFailed: "profiles-cache-access",
	})
}

// TestAddUploadWorkflowVerifierCalibration proves upload storage resolves through the requested named disk.
func TestAddUploadWorkflowVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-upload-workflow",
		scenario:   "file-upload-storage",
		path:       "app/wire/inject_services_app.go",
		old:        "manager.Uploads()",
		mutant:     "manager.Default()",
		behavior: &evaluationBehaviorMutation{
			path:       "internal/uploads/service.go",
			old:        "s.disk.WithContext(ctx).Put(storedPath, body)",
			mutant:     "func() error { return nil }()",
			wantFailed: "upload-workflow-behavior",
		},
		alternates: []evaluationFileMutation{
			{path: "internal/uploads/service.go", old: "func (s *Service) Store", replacement: "func (service *Service) Store"},
			{path: "internal/uploads/service.go", old: "s.disk.WithContext", replacement: "service.disk.WithContext"},
		},
		wantFailed: "uploads-storage-registration",
	})
}

// TestPublishDomainEventVerifierCalibration proves the published fact retains its reviewed topic identity.
func TestPublishDomainEventVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "publish-domain-event",
		scenario:   "users-created-event",
		path:       "internal/events/user_created_event.go",
		old:        `"users.created"`,
		mutant:     `"users.wrong"`,
		behavior: &evaluationBehaviorMutation{
			path:       "internal/users/events.go",
			old:        "UserID: user.ID",
			mutant:     `UserID: ""`,
			wantFailed: "domain-event-behavior",
		},
		alternates: []evaluationFileMutation{
			{path: "internal/events/user_created_event.go", old: "UserCreated", replacement: "UserCreatedEvent"},
			{path: "internal/users/events.go", old: "events.UserCreated", replacement: "events.UserCreatedEvent"},
			{path: "internal/notifications/subscribers.go", old: "events.UserCreated", replacement: "events.UserCreatedEvent"},
			{path: "internal/notifications/subscribers.go", old: "func (s *Subscribers) Register", replacement: "func (subscribers *Subscribers) Register"},
			{path: "internal/notifications/subscribers.go", old: "s.handler.HandleUserCreated", replacement: "subscribers.handler.HandleUserCreated"},
		},
		wantFailed: "typed-user-event",
	})
}

// TestDispatchEventFollowupJobVerifierCalibration proves queued report work retains its typed routing identity.
func TestDispatchEventFollowupJobVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "dispatch-event-followup-job",
		scenario:   "reports-generate-job",
		path:       "internal/reports/generate_job.go",
		old:        "UserID string",
		mutant:     "Email string",
		additional: []evaluationMutation{
			{old: "UserID: userID", mutant: "Email: userID"},
			{old: "payload.UserID", mutant: "payload.Email"},
		},
		behavior: &evaluationBehaviorMutation{
			path:       "internal/reports/generate_job.go",
			old:        "j.service.GenerateForUser(ctx, payload.UserID)",
			mutant:     `j.service.GenerateForUser(ctx, "missing")`,
			wantFailed: "event-followup-job-behavior",
		},
		alternates: []evaluationFileMutation{
			{path: "internal/reports/generate_job.go", old: "func (j *GenerateJob)", replacement: "func (job *GenerateJob)"},
			{path: "internal/reports/generate_job.go", old: "j.queues", replacement: "job.queues"},
			{path: "internal/reports/generate_job.go", old: "j.service", replacement: "job.service"},
		},
		wantFailed:  "typed-report-job",
		wantCompile: true,
	})
}

// TestAddResilientJobVerifierCalibration proves retry-safe report work keeps explicit attempt and timeout policy at its queue boundary.
func TestAddResilientJobVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-resilient-job",
		scenario:   "reports-generate-job",
		path:       "internal/reports/generate_job.go",
		old:        "Retry(3).",
		mutant:     "",
		behavior: &evaluationBehaviorMutation{
			path:       "internal/reports/generate_job.go",
			old:        "j.service.GenerateForUser(ctx, payload.UserID)",
			mutant:     `j.service.GenerateForUser(ctx, "missing")`,
			wantFailed: "resilient-job-behavior",
		},
		wantFailed:  "retry-safe-report-job",
		wantCompile: true,
	})
}

// TestScheduleExistingJobVerifierCalibration proves scheduled dispatch remains attached to the caller's runtime context.
func TestScheduleExistingJobVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation:  "schedule-existing-job",
		scenario:    "reports-daily-schedule",
		path:        "internal/reports/daily_schedule.go",
		old:         "return s.runner.Run(ctx)",
		mutant:      "return s.runner.Run(context.Background())",
		wantFailed:  "scheduled-job-dispatch",
		wantCompile: true,
		behavior: &evaluationBehaviorMutation{
			path:       "internal/reports/daily.go",
			old:        "r.queue.Queue(ctx, userID)",
			mutant:     `r.queue.Queue(ctx, "wrong")`,
			wantFailed: "daily-schedule-behavior",
		},
		alternates: []evaluationFileMutation{
			{path: "internal/reports/daily.go", old: "func (r *DailyRunner) Run", replacement: "func (runner *DailyRunner) Run"},
			{path: "internal/reports/daily.go", old: "r.targets", replacement: "runner.targets"},
			{path: "internal/reports/daily.go", old: "r.queue", replacement: "runner.queue"},
		},
		alternateRenames: []evaluationPathRename{{from: "internal/reports/daily.go", to: "internal/reports/daily_runner.go"}},
	})
}

// TestCreateAdditionalAppVerifierCalibration proves the additional App remains declared in Project configuration.
func TestCreateAdditionalAppVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{evaluation: "create-additional-app", scenario: "create-statuspage-app", path: "cmd/statuspage/main.go", old: "wire.LaunchApplication()", mutant: "_ = wire.LaunchApplication", wantFailed: "statuspage-entrypoint", wantCompile: true})
}

// TestAddAppLifecycleHookVerifierCalibration proves readiness work preserves startup cancellation.
func TestAddAppLifecycleHookVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation:  "add-app-lifecycle-hook",
		scenario:    "invoice-readiness-hook",
		path:        "app/lifecycle.go",
		old:         "registry.invoices.Find(ctx, \"readiness\")",
		mutant:      "registry.invoices.Find(context.Background(), \"readiness\")",
		wantFailed:  "application-readiness-hook",
		wantCompile: true,
		behavior: &evaluationBehaviorMutation{
			path:       "app/lifecycle.go",
			old:        `return fmt.Errorf("check invoice readiness: %w", err)`,
			mutant:     `return nil`,
			wantFailed: "application-readiness-behavior",
		},
		alternates: []evaluationFileMutation{
			{path: "app/lifecycle.go", old: "invoices *invoices.Service", replacement: "invoiceService *invoices.Service"},
			{path: "app/lifecycle.go", old: "{invoices: invoices}", replacement: "{invoiceService: invoiceService}"},
			{path: "app/lifecycle.go", old: "registry.invoices.Find", replacement: "registry.invoiceService.Find"},
		},
	})
}

// TestAddOutboundHTTPIntegrationVerifierCalibration proves remote calls preserve caller cancellation.
func TestAddOutboundHTTPIntegrationVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation:  "add-outbound-http-integration",
		scenario:    "tax-rate-http-integration",
		path:        "internal/taxrates/client.go",
		old:         "httpx.GetCtx[Rate](client.http, ctx,",
		mutant:      "httpx.GetCtx[Rate](client.http, context.Background(),",
		wantFailed:  "typed-http-client",
		wantCompile: true,
		behavior: &evaluationBehaviorMutation{
			path:       "internal/taxrates/client.go",
			old:        "return rate, nil",
			mutant:     "return Rate{}, nil",
			wantFailed: "tax-rate-http-behavior",
		},
		alternates: []evaluationFileMutation{
			{path: "internal/taxrates/client.go", old: "http *httpx.Client", replacement: "transport *httpx.Client"},
			{path: "internal/taxrates/client.go", old: "{http: httpx.New", replacement: "{transport: httpx.New"},
			{path: "internal/taxrates/client.go", old: "client.http", replacement: "client.transport"},
		},
	})
}

// TestAddValidatedWriteEndpointVerifierCalibration proves valid invoice creation preserves request cancellation.
func TestAddValidatedWriteEndpointVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-validated-write-endpoint",
		scenario:   "create-invoice-validation",
		path:       "internal/invoices/controller.go",
		old:        "controller.service.Create(request.Context(), input)",
		mutant:     "controller.service.Create(context.Background(), input)",
		additional: []evaluationMutation{{old: `"errors"`, mutant: "\"context\"\n\t\"errors\""}},
		behavior: &evaluationBehaviorMutation{
			path:       "internal/invoices/controller.go",
			old:        "return request.JSON(http.StatusCreated, invoice)",
			mutant:     "return request.JSON(http.StatusOK, invoice)",
			wantFailed: "invoice-validation-behavior",
		},
		wantFailed:  "validated-request-boundary",
		wantCompile: true,
	})
}

// TestAddRouteMiddlewareVerifierCalibration proves middleware configuration uses the reviewed environment key.
func TestAddRouteMiddlewareVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "add-route-middleware",
		scenario:   "invoice-token-middleware",
		path:       "app/wire/inject_http_controllers_app.go",
		old:        `env.Get("INVOICE_HTTP_TOKEN", "")`,
		mutant:     `env.Get("INVOICE_HTTP_TOKEN", "invoice-secret")`,
		behavior: &evaluationBehaviorMutation{
			path:        "internal/invoices/middleware.go",
			old:         `Header.Get("X-Invoice-Token")`,
			mutant:      `Header.Get("X-Invalid-Invoice-Token")`,
			wantFailed:  "token-policy-behavior",
			wantCompile: true,
		},
		wantFailed:  "resolved-token-provider",
		wantCompile: true,
	})
}

// TestAddDatabaseTransactionVerifierCalibration proves the transaction retains the caller's cancellation boundary.
func TestAddDatabaseTransactionVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation:  "add-database-transaction",
		scenario:    "account-transfer-transaction",
		path:        "internal/accounts/service.go",
		old:         "service.accounts.WithTransaction(ctx,",
		mutant:      "service.accounts.WithTransaction(context.Background(),",
		wantFailed:  "atomic-transfer-service",
		wantCompile: true,
		behavior: &evaluationBehaviorMutation{
			path:       "internal/accounts/service.go",
			old:        "return accounts.AdjustBalance(ctx, toID, amountCents)",
			mutant:     "return accounts.AdjustBalance(ctx, fromID, amountCents)",
			wantFailed: "transaction-behavior",
		},
	})
}

// TestAddMailWorkflowVerifierCalibration proves delivery retains the caller's cancellation boundary.
func TestAddMailWorkflowVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation:  "add-mail-workflow",
		scenario:    "invoice-receipt-mail",
		path:        "internal/invoices/receipt_mailer.go",
		old:         "Send(ctx)",
		mutant:      "Send(context.Background())",
		wantFailed:  "receipt-mail-service",
		wantCompile: true,
		behavior: &evaluationBehaviorMutation{
			path:       "internal/invoices/receipt_mailer.go",
			old:        `To(email, "")`,
			mutant:     `To("billing@example.test", "")`,
			wantFailed: "receipt-mail-behavior",
		},
		alternates: []evaluationFileMutation{
			{path: "internal/invoices/receipt_mailer.go", old: "func (mailer *ReceiptMailer) Send", replacement: "func (sender *ReceiptMailer) Send"},
			{path: "internal/invoices/receipt_mailer.go", old: "mailer.invoices", replacement: "sender.invoices"},
			{path: "internal/invoices/receipt_mailer.go", old: "mailer.mail", replacement: "sender.mail"},
		},
	})
}

// TestProtectRouteWithAuthVerifierCalibration proves invoices cannot be detached from the generated auth route group.
func TestProtectRouteWithAuthVerifierCalibration(t *testing.T) {
	testMajorSurfaceVerifierCalibration(t, majorSurfaceVerifierCase{
		evaluation: "protect-route-with-auth",
		scenario:   "protect-invoice-route",
		path:       "app/routes.go",
		old:        "\tgroups = append(groups, web.NewRouteGroup(\"/api/v1\", protectedRoutes, authService.RequireAuth))\n",
		mutant: "\tgroups = append(groups, web.NewRouteGroup(\"/api/v1\", protectedRoutes))\n" +
			"\tgroups = append(groups, web.NewRouteGroup(\"/hello\", helloController.Routes(), authService.RequireAuth))\n",
		wantFailed:  "generated-auth-composition",
		wantCompile: true,
	})
}

// testMajorSurfaceVerifierCalibration exercises one contract independently so the integration runner can shard the complete portfolio.
func testMajorSurfaceVerifierCalibration(t *testing.T, test majorSurfaceVerifierCase) {
	t.Helper()
	t.Parallel()
	forjExecutable := testkit.EnsureIntegrationForjBinary(t)
	environment := testkit.IntegrationGoProcessEnv(t, nil)
	preparer := NewPreparer(filepath.Join(t.TempDir(), "bases"), environment, logger.NewAppLogger(), nil)
	t.Cleanup(func() { _ = preparer.Close(context.Background()) })
	runner := eval.VerifierCommands{WorkRoot: t.TempDir(), ForjExecutable: forjExecutable, Environment: environment}
	registry, err := eval.NewRegistry(eval.PromotedWorkflows(), eval.PromotedVerifiers(runner))
	if err != nil {
		t.Fatalf("NewRegistry(): %v", err)
	}
	request := eval.PreparationRequest{
		ScenarioID:      test.scenario,
		DestinationRoot: filepath.Join(t.TempDir(), "project"),
		ForjExecutable:  forjExecutable,
		OrchestrationID: "calibration-" + test.evaluation,
		Environment:     environment,
	}
	plan, err := preparer.Resolve(t.Context(), request)
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}
	prepared, err := preparer.Prepare(t.Context(), request, plan)
	if err != nil {
		t.Fatalf("Prepare(): %v", err)
	}
	t.Cleanup(func() { _ = prepared.Close(context.Background()) })
	workRoot := t.TempDir()
	if err := scenarios.Validate(scenarios.ValidateOptions{Logger: logger.NewAppLogger(), WorkDir: workRoot, Keep: true, IDs: []string{test.scenario}, ForjExec: forjExecutable, Environment: environment}); err != nil {
		t.Fatalf("build %s scenario: %v", test.scenario, err)
	}
	projectRoot := findEvaluationScenarioRoot(t, workRoot, test.scenario)
	definition, err := eval.LoadPromotedDefinition(test.evaluation)
	if err != nil {
		t.Fatalf("LoadPromotedDefinition(): %v", err)
	}
	resolved, err := registry.Resolve(definition)
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}
	changes := evaluationProjectChanges(t, prepared.Result().ProjectRoot, projectRoot)
	result, err := resolved.Verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot, Changes: changes})
	if err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if result.FrameworkOutcome.Status != eval.EndpointPassed {
		t.Fatalf("golden outcome = %#v; checks = %#v", result.FrameworkOutcome, result.Checks)
	}
	if len(test.alternates) > 0 {
		originals := applyEvaluationAlternates(t, projectRoot, test.alternates)
		alternateChanges := evaluationProjectChanges(t, prepared.Result().ProjectRoot, projectRoot)
		alternateResult, verifyErr := resolved.Verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot, Changes: alternateChanges})
		if verifyErr != nil {
			t.Fatalf("Verify(alternate): %v", verifyErr)
		}
		if alternateResult.FrameworkOutcome.Status != eval.EndpointPassed {
			t.Fatalf("alternate outcome = %#v; checks = %#v", alternateResult.FrameworkOutcome, alternateResult.Checks)
		}
		restoreEvaluationAlternates(t, originals)
	}
	if len(test.alternateRenames) > 0 {
		func() {
			restore := applyEvaluationRenames(t, projectRoot, test.alternateRenames)
			defer restore()
			alternateChanges := evaluationProjectChanges(t, prepared.Result().ProjectRoot, projectRoot)
			alternateResult, verifyErr := resolved.Verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot, Changes: alternateChanges})
			if verifyErr != nil {
				t.Fatalf("Verify(relocated alternate): %v", verifyErr)
			}
			if alternateResult.FrameworkOutcome.Status != eval.EndpointPassed {
				t.Fatalf("relocated alternate outcome = %#v; checks = %#v", alternateResult.FrameworkOutcome, alternateResult.Checks)
			}
			if test.behavior != nil && !evaluationCheckHasStatus(alternateResult.Checks, test.behavior.wantFailed, eval.EndpointPassed) {
				t.Fatalf("relocated alternate behavior = %#v", alternateResult.Checks)
			}
		}()
	}
	if test.disconnectedProvider != "" {
		verifyDisconnectedNamedResourceProvider(t, resolved.Verifier, prepared.Result().ProjectRoot, projectRoot, test.disconnectedProvider, test.wantFailed, environment)
	}
	path := filepath.Join(projectRoot, filepath.FromSlash(test.path))
	if strings.ContainsAny(test.path, "*?[") {
		matches, globErr := filepath.Glob(path)
		if globErr != nil || len(matches) != 1 {
			t.Fatalf("mutant target %q matches = %v, error = %v", test.path, matches, globErr)
		}
		path = matches[0]
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mutant target: %v", err)
	}
	if strings.Count(string(body), test.old) != 1 {
		t.Fatalf("mutant target %q count = %d", test.old, strings.Count(string(body), test.old))
	}
	mutant := strings.Replace(string(body), test.old, test.mutant, 1)
	for _, mutation := range test.additional {
		if strings.Count(mutant, mutation.old) != 1 {
			t.Fatalf("additional mutant target %q count = %d", mutation.old, strings.Count(mutant, mutation.old))
		}
		mutant = strings.Replace(mutant, mutation.old, mutation.mutant, 1)
	}
	if err := os.WriteFile(path, []byte(mutant), 0o644); err != nil {
		t.Fatalf("write mutant: %v", err)
	}
	if test.wantCompile {
		command := exec.CommandContext(t.Context(), "go", "test", "-run", "^$", "./...")
		command.Dir = projectRoot
		command.Env = append([]string(nil), environment...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("semantic mutant must compile: %v\n%s", err, output)
		}
	}
	mutantChanges := evaluationProjectChanges(t, prepared.Result().ProjectRoot, projectRoot)
	mutantResult, err := resolved.Verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot, Changes: mutantChanges})
	if err != nil {
		t.Fatalf("Verify(mutant): %v", err)
	}
	if mutantResult.FrameworkOutcome.Status != eval.EndpointFailed {
		t.Fatalf("mutant outcome = %#v; checks = %#v", mutantResult.FrameworkOutcome, mutantResult.Checks)
	}
	if !evaluationCheckFailed(mutantResult.Checks, test.wantFailed) {
		t.Fatalf("mutant checks = %#v, want %q to fail", mutantResult.Checks, test.wantFailed)
	}
	if test.behavior != nil {
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatalf("restore structural mutant target: %v", err)
		}
		verifyEvaluationBehaviorMutant(t, resolved.Verifier, prepared.Result().ProjectRoot, projectRoot, *test.behavior, environment)
	}
}

// verifyDisconnectedNamedResourceProvider proves an otherwise compiling Wire edit cannot detach the provider that selects a named resource.
func verifyDisconnectedNamedResourceProvider(t *testing.T, verifier eval.Verifier, baselineRoot, projectRoot, replacement, wantFailed string, environment []string) {
	t.Helper()
	wirePath := filepath.Join(projectRoot, "app", "wire", "inject_services_app.go")
	body, err := os.ReadFile(wirePath)
	if err != nil {
		t.Fatalf("read App services Wire: %v", err)
	}
	provider := ""
	for _, candidate := range []string{"NewReportDispatcher", "NewProfileCache", "NewAvatarStorage"} {
		if strings.Count(string(body), candidate) == 1 {
			provider = candidate
			break
		}
	}
	if provider == "" {
		t.Fatalf("named resource provider is absent from %s", wirePath)
	}
	mutant := strings.Replace(string(body), provider, strings.TrimPrefix(replacement, "invoices."), 1)
	if err := os.WriteFile(wirePath, []byte(mutant), 0o644); err != nil {
		t.Fatalf("write disconnected App services Wire: %v", err)
	}
	t.Cleanup(func() { _ = os.WriteFile(wirePath, body, 0o644) })
	command := exec.CommandContext(t.Context(), "go", "test", "-run", "^$", "./...")
	command.Dir = projectRoot
	command.Env = append([]string(nil), environment...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("disconnected provider mutant must compile: %v\n%s", err, output)
	}
	changes := evaluationProjectChanges(t, baselineRoot, projectRoot)
	result, err := verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot, Changes: changes})
	if err != nil {
		t.Fatalf("Verify(disconnected provider): %v", err)
	}
	if result.FrameworkOutcome.Status != eval.EndpointFailed || !evaluationCheckFailed(result.Checks, wantFailed) {
		t.Fatalf("disconnected provider outcome = %#v; checks = %#v", result.FrameworkOutcome, result.Checks)
	}
	if err := os.WriteFile(wirePath, body, 0o644); err != nil {
		t.Fatalf("restore App services Wire: %v", err)
	}
}

// verifyEvaluationBehaviorMutant proves the executable oracle catches a compiling implementation with valid structural evidence.
func verifyEvaluationBehaviorMutant(t *testing.T, verifier eval.Verifier, baselineRoot, projectRoot string, mutation evaluationBehaviorMutation, environment []string) {
	t.Helper()
	path := filepath.Join(projectRoot, filepath.FromSlash(mutation.path))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read behavior mutant target: %v", err)
	}
	if strings.Count(string(body), mutation.old) != 1 {
		t.Fatalf("behavior mutant target %q count = %d", mutation.old, strings.Count(string(body), mutation.old))
	}
	mutant := strings.Replace(string(body), mutation.old, mutation.mutant, 1)
	if err := os.WriteFile(path, []byte(mutant), 0o644); err != nil {
		t.Fatalf("write behavior mutant: %v", err)
	}
	if mutation.wantCompile {
		command := exec.CommandContext(t.Context(), "go", "test", "-run", "^$", "./...")
		command.Dir = projectRoot
		command.Env = append([]string(nil), environment...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("behavior mutant must compile: %v\n%s", err, output)
		}
	}
	result, err := verifier.Verify(t.Context(), eval.VerificationInput{
		ProjectRoot: projectRoot,
		Changes:     evaluationProjectChanges(t, baselineRoot, projectRoot),
	})
	if err != nil {
		t.Fatalf("Verify(behavior mutant): %v", err)
	}
	if result.FrameworkOutcome.Status != eval.EndpointFailed || !evaluationCheckFailed(result.Checks, mutation.wantFailed) {
		t.Fatalf("behavior mutant outcome = %#v; checks = %#v; want %q to fail", result.FrameworkOutcome, result.Checks, mutation.wantFailed)
	}
}

// applyEvaluationAlternates mutates only the reviewed files and retains exact originals for the subsequent negative calibration.
func applyEvaluationAlternates(t *testing.T, projectRoot string, mutations []evaluationFileMutation) map[string][]byte {
	t.Helper()
	originals := make(map[string][]byte, len(mutations))
	for _, mutation := range mutations {
		candidatePath := filepath.Join(projectRoot, filepath.FromSlash(mutation.path))
		if strings.ContainsAny(mutation.path, "*?[") {
			matches, err := filepath.Glob(candidatePath)
			if err != nil || len(matches) != 1 {
				t.Fatalf("alternate target %q matches = %v, error = %v", mutation.path, matches, err)
			}
			candidatePath = matches[0]
		}
		body, exists := originals[candidatePath]
		if !exists {
			var err error
			body, err = os.ReadFile(candidatePath)
			if err != nil {
				t.Fatalf("read alternate target %q: %v", mutation.path, err)
			}
			originals[candidatePath] = append([]byte(nil), body...)
		}
		current, err := os.ReadFile(candidatePath)
		if err != nil {
			t.Fatalf("read current alternate target %q: %v", mutation.path, err)
		}
		if !strings.Contains(string(current), mutation.old) {
			t.Fatalf("alternate target %q does not contain %q", mutation.path, mutation.old)
		}
		updated := strings.ReplaceAll(string(current), mutation.old, mutation.replacement)
		if err := os.WriteFile(candidatePath, []byte(updated), 0o644); err != nil {
			t.Fatalf("write alternate target %q: %v", mutation.path, err)
		}
	}
	return originals
}

// restoreEvaluationAlternates returns the golden Project exactly so the targeted mutant remains independent of positive-family calibration.
func restoreEvaluationAlternates(t *testing.T, originals map[string][]byte) {
	t.Helper()
	for path, body := range originals {
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatalf("restore alternate target %q: %v", path, err)
		}
	}
}

// applyEvaluationRenames relocates source files to prove package-wide contracts do not depend on golden filenames.
func applyEvaluationRenames(t *testing.T, projectRoot string, renames []evaluationPathRename) func() {
	t.Helper()
	for _, rename := range renames {
		from := filepath.Join(projectRoot, filepath.FromSlash(rename.from))
		to := filepath.Join(projectRoot, filepath.FromSlash(rename.to))
		if err := os.Rename(from, to); err != nil {
			t.Fatalf("rename alternate %q to %q: %v", rename.from, rename.to, err)
		}
	}
	return func() {
		for index := len(renames) - 1; index >= 0; index-- {
			rename := renames[index]
			if err := os.Rename(filepath.Join(projectRoot, filepath.FromSlash(rename.to)), filepath.Join(projectRoot, filepath.FromSlash(rename.from))); err != nil {
				t.Fatalf("restore renamed alternate %q: %v", rename.from, err)
			}
		}
	}
}

// evaluationCheckFailed reports whether calibration failed for its intended semantic invariant rather than incidental compilation.
func evaluationCheckFailed(checks []eval.EndpointResult, id string) bool {
	for _, check := range checks {
		if check.ID == id && check.Status == eval.EndpointFailed {
			return true
		}
	}
	return false
}

// evaluationProjectChanges projects real prepared-to-golden file changes into the Atlas ownership contract.
func evaluationProjectChanges(t *testing.T, baselineRoot string, finalRoot string) []eval.ProjectChange {
	t.Helper()
	baseline := evaluationFileStates(t, baselineRoot)
	final := evaluationFileStates(t, finalRoot)
	paths := make([]string, 0, len(baseline)+len(final))
	seen := map[string]bool{}
	for path := range baseline {
		seen[path] = true
		paths = append(paths, path)
	}
	for path := range final {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	changes := make([]eval.ProjectChange, 0, len(paths))
	for _, path := range paths {
		before, hadBefore := baseline[path]
		after, hasAfter := final[path]
		if hadBefore && hasAfter && before == after {
			continue
		}
		changes = append(changes, eval.ProjectChange{Path: path, Before: before, After: after})
	}
	return changes
}

// evaluationFileStates mirrors Atlas tree ownership across directories, files, and symlink targets.
func evaluationFileStates(t *testing.T, root string) map[string]eval.ProjectPathState {
	t.Helper()
	states := map[string]eval.ProjectPathState{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == "_data" && entry.IsDir() {
			return filepath.SkipDir
		}
		if strings.HasPrefix(relative, "_data/") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		kind := "file"
		var body []byte
		if entry.IsDir() {
			kind = "directory"
		} else if entry.Type()&os.ModeSymlink != 0 {
			kind = "symlink"
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			body = []byte(target)
		} else {
			body, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		body = normalizeEvaluationFixtureFile(filepath.ToSlash(relative), body)
		digest := sha256.Sum256(body)
		states[relative] = eval.ProjectPathState{Kind: kind, Digest: fmt.Sprintf("sha256:%x", digest), Mode: uint32(info.Mode())}
		return nil
	}); err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return states
}

// normalizeEvaluationFixtureFile removes independently rendered secret entropy without hiding candidate changes in live verification.
func normalizeEvaluationFixtureFile(path string, body []byte) []byte {
	if path != ".env" {
		return body
	}
	lines := strings.Split(string(body), "\n")
	for index, line := range lines {
		key, _, ok := strings.Cut(line, "=")
		if ok && (key == "APP_KEY" || key == "APP_DIAG_TOKEN" || strings.HasSuffix(key, "_SECRET")) {
			lines[index] = key + "=<fixture-secret>"
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// TestUnknownFrameworkShapeCalibratesSafeAbstention proves the ambiguous fixture accepts a precise question and rejects speculative edits.
func TestUnknownFrameworkShapeCalibratesSafeAbstention(t *testing.T) {
	workRoot := t.TempDir()
	forjExecutable := testkit.EnsureIntegrationForjBinary(t)
	environment := testkit.IntegrationGoProcessEnv(t, nil)
	if err := scenarios.Validate(scenarios.ValidateOptions{Logger: logger.NewAppLogger(), WorkDir: workRoot, Keep: true, IDs: []string{"unknown-invoice-reconciliation"}, ForjExec: forjExecutable, Environment: environment}); err != nil {
		t.Fatalf("build unknown framework scenario: %v", err)
	}
	projectRoot := findEvaluationScenarioRoot(t, workRoot, "unknown-invoice-reconciliation")
	verifier := eval.NewSafeAbstentionVerifier()
	response := `ATLAS_CLARIFICATION {"decision":"execution_mode","question":"Should reconciliation run as a command, job, or schedule?","options":["command","job","schedule"]}`
	result, err := verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot, FinalResponse: response})
	if err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if result.FrameworkOutcome.Status != eval.EndpointPassed || result.Abstention == nil || result.Abstention.Status != eval.EndpointPassed {
		t.Fatalf("golden outcome = %#v", result)
	}
	mutant, err := verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot, FinalResponse: response, Changes: []eval.ProjectChange{{Path: "internal/invoices/reconcile.go"}}})
	if err != nil {
		t.Fatalf("Verify(mutant): %v", err)
	}
	if mutant.FrameworkOutcome.Status != eval.EndpointFailed || mutant.Abstention == nil || mutant.Abstention.Status != eval.EndpointFailed {
		t.Fatalf("mutant outcome = %#v", mutant)
	}
}

// findEvaluationScenarioRoot locates the retained workspace for one scenario without depending on its random suffix.
func findEvaluationScenarioRoot(t *testing.T, root string, scenario string) string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read scenario root: %v", err)
	}
	prefix := scenario + "-"
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			return filepath.Join(root, entry.Name())
		}
	}
	t.Fatalf("scenario workspace %q is absent", scenario)
	return ""
}

// materializeTransportInvoiceController converts the golden family into the independently observed transport-package family.
func materializeTransportInvoiceController(t *testing.T, root string, forjExecutable string, environment []string) {
	t.Helper()
	replaceEvaluationFixtureText(t, filepath.Join(root, "app", "routes.go"), `"example.com/invoiceeval/internal/invoices"`, `httptransport "example.com/invoiceeval/internal/http"`)
	replaceEvaluationFixtureText(t, filepath.Join(root, "app", "routes.go"), "invoicesController *invoices.Controller", "invoiceController *httptransport.InvoiceController")
	replaceEvaluationFixtureText(t, filepath.Join(root, "app", "routes.go"), "invoicesController.Routes()", "invoiceController.Routes()")
	replaceEvaluationFixtureText(t, filepath.Join(root, "app", "wire", "inject_http_controllers_app.go"), `"example.com/invoiceeval/internal/invoices"`, `"example.com/invoiceeval/internal/http"`)
	replaceEvaluationFixtureText(t, filepath.Join(root, "app", "wire", "inject_http_controllers_app.go"), "invoices.NewController", "http.NewInvoiceController")
	transportPath := filepath.Join(root, "internal", "http", "invoice_controller.go")
	if err := os.MkdirAll(filepath.Dir(transportPath), 0o755); err != nil {
		t.Fatalf("create transport package: %v", err)
	}
	if err := os.WriteFile(transportPath, []byte(transportInvoiceControllerFixture), 0o644); err != nil {
		t.Fatalf("write transport controller: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "internal", "invoices", "controller.go")); err != nil {
		t.Fatalf("remove golden controller: %v", err)
	}
	command := exec.CommandContext(t.Context(), forjExecutable, "build")
	command.Dir = root
	command.Env = append([]string(nil), environment...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("rebuild transport-package family: %v\n%s", err, output)
	}
}

// replaceEvaluationFixtureText applies one exact family transition so generator-shape drift fails loudly.
func replaceEvaluationFixtureText(t *testing.T, path string, old string, replacement string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if strings.Count(string(body), old) != 1 {
		t.Fatalf("%s does not contain exactly one %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(body), old, replacement, 1)), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const transportInvoiceControllerFixture = `package http

import (
	"errors"
	"net/http"

	"example.com/invoiceeval/internal/invoices"
	"github.com/goforj/web"
)

// InvoiceController translates invoice application behavior into HTTP responses.
type InvoiceController struct {
	service *invoices.Service
}

// NewInvoiceController exposes the existing service to the transport package.
func NewInvoiceController(service *invoices.Service) *InvoiceController {
	return &InvoiceController{service: service}
}

// Routes declares the invoice transport surface.
func (controller *InvoiceController) Routes() []web.Route {
	return []web.Route{
		web.NewRoute(http.MethodGet, "/invoices/:id", controller.Get),
	}
}

// Get translates invoice lookup results into the public HTTP contract.
func (controller *InvoiceController) Get(request web.Context) error {
	invoice, err := controller.service.Find(request.Request().Context(), request.Param("id"))
	if errors.Is(err, invoices.ErrInvoiceNotFound) {
		return request.JSON(http.StatusNotFound, map[string]string{"error": "invoice not found"})
	}
	if err != nil {
		return err
	}
	return request.JSON(http.StatusOK, invoice)
}
`

// evaluationCheckHasStatus locates one promoted verifier result without coupling integration coverage to check ordering.
func evaluationCheckHasStatus(checks []eval.EndpointResult, id string, status eval.EndpointStatus) bool {
	for _, check := range checks {
		if check.ID == id && check.Status == status {
			return true
		}
	}
	return false
}

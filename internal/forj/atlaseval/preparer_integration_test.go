//go:build integration

package atlaseval

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
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

// TestPreparerMaterializesInvoiceStartingState verifies the real CLI reaches a healthy target-free Project.
func TestPreparerMaterializesInvoiceStartingState(t *testing.T) {
	request := eval.PreparationRequest{
		ScenarioID:      "invoice-http-route",
		DestinationRoot: t.TempDir(),
		ForjExecutable:  testkit.EnsureIntegrationForjBinary(t),
		OrchestrationID: "integration-01",
		Environment:     testkit.ProcessGoEnv("", nil),
	}
	preparer := Preparer{}
	plan, err := preparer.Resolve(context.Background(), request)
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}
	project, err := preparer.Prepare(context.Background(), request, plan)
	if err != nil {
		t.Fatalf("Prepare(): %v", err)
	}
	root := project.Result().ProjectRoot
	t.Cleanup(func() {
		if err := project.Close(context.Background()); err != nil {
			t.Fatalf("Close(): %v", err)
		}
	})
	if project.Result().BaselineTree == "" || project.Result().ForjDigest == "" {
		t.Fatalf("preparation result = %#v", project.Result())
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "invoices", "service.go")); err != nil {
		t.Fatalf("invoice service missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "invoices", "controller.go")); !os.IsNotExist(err) {
		t.Fatalf("target controller leaked into preparation: %v", err)
	}
}

// TestPreparerClonesOneIdenticalBaseForPairedTreatments protects the comparison's single-variable invariant.
func TestPreparerClonesOneIdenticalBaseForPairedTreatments(t *testing.T) {
	workRoot := t.TempDir()
	baseRoot := filepath.Join(workRoot, "bases")
	baseBuildCache := filepath.Join(workRoot, "base-gocache")
	trialBuildCache := filepath.Join(workRoot, "trial-gocache")
	baseEnvironment := testkit.ProcessGoEnv("", map[string]string{"GOCACHE": baseBuildCache})
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
	if none.Result().BaselineTree != agents.Result().BaselineTree {
		t.Fatalf("paired baseline trees differ: %s != %s", none.Result().BaselineTree, agents.Result().BaselineTree)
	}
	if entries, err := os.ReadDir(baseBuildCache); err != nil || len(entries) == 0 {
		t.Fatalf("base preparation did not use its private build cache: entries=%v error=%v", entries, err)
	}
	if entries, err := os.ReadDir(trialBuildCache); err == nil && len(entries) > 0 {
		t.Fatalf("preparation warmed the candidate build cache: %v", entries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read candidate build cache: %v", err)
	}
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

// TestInvoiceHTTPVerifierCalibratesBehaviorAndImplementationFamilies proves diagnostic success, mutation rejection, and layout neutrality.
func TestInvoiceHTTPVerifierCalibratesBehaviorAndImplementationFamilies(t *testing.T) {
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
	result, err := verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot})
	if err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if result.FrameworkOutcome.Status != eval.EndpointIneligible {
		t.Fatalf("framework outcome = %#v; checks = %#v", result.FrameworkOutcome, result.Checks)
	}
	if !evaluationCheckHasStatus(result.Checks, "invoice-behavior", eval.EndpointIneligible) {
		t.Fatalf("shared-process behavior check did not retain its evidence limitation: %#v", result.Checks)
	}
	controllerPath := filepath.Join(projectRoot, "internal", "invoices", "controller.go")
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
	mutantResult, err := verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot})
	if err != nil {
		t.Fatalf("Verify(mutant): %v", err)
	}
	if !evaluationCheckHasStatus(mutantResult.Checks, "invoice-behavior", eval.EndpointFailed) {
		t.Fatalf("independent behavior oracle accepted wrong response: %#v", mutantResult.Checks)
	}
	if err := os.WriteFile(controllerPath, controller, 0o644); err != nil {
		t.Fatalf("restore controller: %v", err)
	}
	materializeTransportInvoiceController(t, projectRoot, forjExecutable, environment)
	transportResult, err := verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot})
	if err != nil {
		t.Fatalf("Verify(transport family): %v", err)
	}
	if transportResult.FrameworkOutcome.Status != eval.EndpointIneligible || !evaluationCheckHasStatus(transportResult.Checks, "invoice-behavior", eval.EndpointIneligible) {
		t.Fatalf("transport-package implementation family failed: outcome=%#v checks=%#v", transportResult.FrameworkOutcome, transportResult.Checks)
	}
}

// TestMajorSurfaceVerifiersAcceptGoldenProjectsAndRejectTargetedMutants calibrates every promoted framework contract against executable GoForj scenarios.
func TestMajorSurfaceVerifiersAcceptGoldenProjectsAndRejectTargetedMutants(t *testing.T) {
	tests := []struct {
		evaluation string
		scenario   string
		path       string
		old        string
		mutant     string
	}{
		{evaluation: "add-app-command", scenario: "invoice-app-command", path: "internal/invoices/show_cmd.go", old: "command.service.Find(ctx, command.ID)", mutant: "command.service.Find(context.Background(), command.ID)"},
		{evaluation: "add-job", scenario: "invoice-receipt-job", path: "internal/invoices/receipt_job.go", old: "InvoiceID string", mutant: "Reference string"},
		{evaluation: "add-schedule", scenario: "invoice-reconcile-schedule", path: "internal/invoices/reconcile_schedule.go", old: "schedule.service.Find(ctx, \"inv-42\")", mutant: "schedule.service.Find(context.Background(), \"inv-42\")"},
		{evaluation: "add-event-subscriber", scenario: "invoice-paid-subscriber", path: "internal/invoices/paid_subscriber.go", old: "subscriber.service.Find(ctx, event.InvoiceID)", mutant: "subscriber.service.Find(context.Background(), event.InvoiceID)"},
		{evaluation: "create-model", scenario: "create-user-model", path: "internal/models/user.go", old: "Email", mutant: "EmailAddress"},
		{evaluation: "add-named-app-route", scenario: "admin-audit-route", path: "internal/audits/controller.go", old: "/api/v1/audits", mutant: "/api/v1/wrong"},
		{evaluation: "add-named-resource", scenario: "named-reports-queue", path: "internal/invoices/report_dispatcher.go", old: "manager.Reports()", mutant: "manager.Default()"},
		{evaluation: "add-named-cache", scenario: "named-profiles-cache", path: "internal/invoices/profile_cache.go", old: "manager.Profiles()", mutant: "manager.Default()"},
		{evaluation: "add-named-storage", scenario: "named-avatar-storage", path: "internal/invoices/avatar_storage.go", old: "manager.Avatars()", mutant: "manager.Default()"},
		{evaluation: "repair-wire-provider", scenario: "repair-report-wire-provider", path: "app/wire/inject_services_app.go", old: "reports.NewService", mutant: "app.NewLifecycleRegistry"},
	}
	forjExecutable := testkit.EnsureIntegrationForjBinary(t)
	environment := testkit.IntegrationGoProcessEnv(t, nil)
	preparer := NewPreparer(filepath.Join(t.TempDir(), "bases"), environment, logger.NewAppLogger(), nil)
	t.Cleanup(func() { _ = preparer.Close(context.Background()) })
	runner := eval.VerifierCommands{WorkRoot: t.TempDir(), ForjExecutable: forjExecutable, Environment: environment}
	registry, err := eval.NewRegistry(eval.PromotedWorkflows(), eval.PromotedVerifiers(runner))
	if err != nil {
		t.Fatalf("NewRegistry(): %v", err)
	}
	for _, test := range tests {
		t.Run(test.evaluation, func(t *testing.T) {
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
			path := filepath.Join(projectRoot, filepath.FromSlash(test.path))
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read mutant target: %v", err)
			}
			if strings.Count(string(body), test.old) != 1 {
				t.Fatalf("mutant target %q count = %d", test.old, strings.Count(string(body), test.old))
			}
			mutant := strings.Replace(string(body), test.old, test.mutant, 1)
			if err := os.WriteFile(path, []byte(mutant), 0o644); err != nil {
				t.Fatalf("write mutant: %v", err)
			}
			mutantResult, err := resolved.Verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot})
			if err != nil {
				t.Fatalf("Verify(mutant): %v", err)
			}
			if mutantResult.FrameworkOutcome.Status != eval.EndpointFailed {
				t.Fatalf("mutant outcome = %#v; checks = %#v", mutantResult.FrameworkOutcome, mutantResult.Checks)
			}
		})
	}
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

// evaluationFileStates hashes files and symlink targets while ignoring directory metadata that carries no ownership signal.
func evaluationFileStates(t *testing.T, root string) map[string]eval.ProjectPathState {
	t.Helper()
	states := map[string]eval.ProjectPathState{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(filepath.ToSlash(relative), "_data/") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		kind := "file"
		var body []byte
		if entry.Type()&os.ModeSymlink != 0 {
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
		states[filepath.ToSlash(relative)] = eval.ProjectPathState{Kind: kind, Digest: fmt.Sprintf("sha256:%x", digest), Mode: uint32(info.Mode())}
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

//go:build integration

package atlaseval

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/atlas/eval"
	"github.com/goforj/atlas/eval/isolate"
	"github.com/goforj/atlas/eval/verify"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/scenarios"
	"github.com/goforj/goforj/internal/testkit"
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
	preparer := NewPreparer(baseRoot, baseEnvironment, nil)
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

// TestInvoiceHTTPVerifierRunsIndependentBehaviorOracle proves the promoted verifier accepts the executable golden family without trusting its tests.
func TestInvoiceHTTPVerifierRunsIndependentBehaviorOracle(t *testing.T) {
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
	verifier := verify.NewAddHTTPControllerVerifier(isolate.VerifierCommands{
		WorkRoot:       t.TempDir(),
		ForjExecutable: forjExecutable,
		Environment:    environment,
	})
	result, err := verifier.Verify(t.Context(), eval.VerificationInput{ProjectRoot: projectRoot})
	if err != nil {
		t.Fatalf("Verify(): %v", err)
	}
	if result.FrameworkOutcome.Status != eval.EndpointPassed {
		t.Fatalf("framework outcome = %#v; checks = %#v", result.FrameworkOutcome, result.Checks)
	}
	if !evaluationCheckHasStatus(result.Checks, "invoice-behavior", eval.EndpointPassed) {
		t.Fatalf("independent invoice behavior check did not pass: %#v", result.Checks)
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
	if transportResult.FrameworkOutcome.Status != eval.EndpointPassed || !evaluationCheckHasStatus(transportResult.Checks, "invoice-behavior", eval.EndpointPassed) {
		t.Fatalf("transport-package implementation family failed: outcome=%#v checks=%#v", transportResult.FrameworkOutcome, transportResult.Checks)
	}
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

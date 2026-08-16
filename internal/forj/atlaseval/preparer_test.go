package atlaseval

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/goforj/atlas/eval"
)

// TestPreparerResolveReturnsStableAtlasContract verifies resolution is attributable and mutation-free.
func TestPreparerResolveReturnsStableAtlasContract(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "projects")
	request := eval.PreparationRequest{
		ScenarioID:      "invoice-http-route",
		DestinationRoot: destination,
		ForjExecutable:  os.Args[0],
		OrchestrationID: "trial-01:none",
		Environment:     os.Environ(),
	}
	plan, err := (Preparer{}).Resolve(context.Background(), request)
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}
	if plan.ResolutionID != request.OrchestrationID || plan.ScenarioID != request.ScenarioID || plan.ScenarioSchema != 2 {
		t.Fatalf("resolved identity = %#v", plan)
	}
	if plan.PlanDigest == "" || plan.ScenarioPlanDigest == "" || plan.CatalogDigest == "" || plan.ForjDigest == "" || plan.EnvironmentDigest == "" || !plan.TargetOmitted {
		t.Fatalf("resolved provenance = %#v", plan)
	}
	if plan.DependencyDigests["invoice-domain"] == "" || plan.DependencyDigests["invoice-http-route"] == "" {
		t.Fatalf("dependency digests = %#v", plan.DependencyDigests)
	}
	if !strings.Contains(string(plan.ProjectConfiguration), "project_name: Invoice HTTP Route") {
		t.Fatalf("Project configuration = %q", plan.ProjectConfiguration)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("Resolve() mutated destination: %v", err)
	}
}

// TestPreparerRejectsExecutableOrEnvironmentDrift binds preparation to every material resolution input.
func TestPreparerRejectsExecutableOrEnvironmentDrift(t *testing.T) {
	forjExecutable := filepath.Join(t.TempDir(), "forj")
	if err := os.WriteFile(forjExecutable, []byte("first"), 0o700); err != nil {
		t.Fatal(err)
	}
	baseRequest := eval.PreparationRequest{
		ScenarioID:      "invoice-http-route",
		DestinationRoot: filepath.Join(t.TempDir(), "projects"),
		ForjExecutable:  forjExecutable,
		OrchestrationID: "trial-01:none",
		Environment:     testPreparationEnvironment(t, "first", "off"),
	}
	goExecutable := filepath.Join(strings.TrimPrefix(baseRequest.Environment[0], "PATH="), testToolFileName("go"))
	for _, test := range []struct {
		name   string
		mutate func(*eval.PreparationRequest) error
	}{
		{
			name: "executable",
			mutate: func(_ *eval.PreparationRequest) error {
				return os.WriteFile(forjExecutable, []byte("second"), 0o700)
			},
		},
		{
			name: "environment",
			mutate: func(request *eval.PreparationRequest) error {
				request.Environment = testPreparationEnvironment(t, "first", "off")
				request.Environment = append(request.Environment, "APP_ENV=production")
				return nil
			},
		},
		{
			name: "go executable",
			mutate: func(_ *eval.PreparationRequest) error {
				return os.WriteFile(goExecutable, []byte("second"), 0o700)
			},
		},
		{
			name: "GOWORK",
			mutate: func(request *eval.PreparationRequest) error {
				request.Environment = testPreparationEnvironment(t, "first", "workspace.go.work")
				return nil
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(forjExecutable, []byte("first"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(goExecutable, []byte("first"), 0o700); err != nil {
				t.Fatal(err)
			}
			request := baseRequest
			request.Environment = append([]string(nil), baseRequest.Environment...)
			plan, err := (Preparer{}).Resolve(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.mutate(&request); err != nil {
				t.Fatal(err)
			}
			current, err := (Preparer{}).Resolve(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if current.PlanDigest == plan.PlanDigest {
				t.Fatal("material preparation drift retained the same cache identity")
			}
			_, err = (Preparer{}).Prepare(context.Background(), request, plan)
			if err == nil || !strings.Contains(err.Error(), "inputs changed") {
				t.Fatalf("Prepare() error = %v, want input drift rejection", err)
			}
			if _, statErr := os.Stat(request.DestinationRoot); !os.IsNotExist(statErr) {
				t.Fatalf("input drift mutated destination: %v", statErr)
			}
		})
	}
}

// TestPreparerRejectsUncorrelatedPlan verifies execution cannot substitute another attempt's resolved plan.
func TestPreparerRejectsUncorrelatedPlan(t *testing.T) {
	request := eval.PreparationRequest{
		ScenarioID:      "invoice-http-route",
		DestinationRoot: t.TempDir(),
		ForjExecutable:  os.Args[0],
		OrchestrationID: "trial-01:none",
		Environment:     os.Environ(),
	}
	plan, err := (Preparer{}).Resolve(context.Background(), request)
	if err != nil {
		t.Fatalf("Resolve(): %v", err)
	}
	plan.ResolutionID = "trial-02:agents"
	_, err = (Preparer{}).Prepare(context.Background(), request, plan)
	if err == nil || !strings.Contains(err.Error(), "identity does not match") {
		t.Fatalf("Prepare() error = %v, want identity mismatch", err)
	}
}

// TestPreparerRejectsMaterialBaseEnvironmentDrift prevents cache isolation from changing the Project preparation contract.
func TestPreparerRejectsMaterialBaseEnvironmentDrift(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "projects")
	request := eval.PreparationRequest{
		ScenarioID:      "invoice-http-route",
		DestinationRoot: destination,
		ForjExecutable:  os.Args[0],
		OrchestrationID: "trial-01:none",
		Environment:     testPreparationEnvironment(t, "first", "off"),
	}
	baseEnvironment := testPreparationEnvironment(t, "first", "off")
	baseEnvironment = append(baseEnvironment, "APP_ENV=production")
	preparer := NewPreparer(filepath.Join(t.TempDir(), "bases"), baseEnvironment, nil, nil)
	plan, err := preparer.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = preparer.Prepare(context.Background(), request, plan)
	if err == nil || !strings.Contains(err.Error(), "base environment does not match") {
		t.Fatalf("Prepare() error = %v, want material base environment rejection", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("base environment drift mutated destination: %v", statErr)
	}
}

// TestPreparerCapabilitiesAdvertiseOnlyImplementedSchema keeps negotiation narrower than future scenario formats.
func TestPreparerCapabilitiesAdvertiseOnlyImplementedSchema(t *testing.T) {
	capabilities, err := (Preparer{}).Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities(): %v", err)
	}
	if len(capabilities.ScenarioSchemaVersions) != 1 || capabilities.ScenarioSchemaVersions[0] != 2 {
		t.Fatalf("scenario schemas = %v", capabilities.ScenarioSchemaVersions)
	}
}

// TestPromotedEvaluationsResolveLiveScenarioPrefixes prevents Atlas promotion from outrunning GoForj's executable fixture catalog.
func TestPromotedEvaluationsResolveLiveScenarioPrefixes(t *testing.T) {
	ids, err := eval.PromotedEvaluationIDs("")
	if err != nil {
		t.Fatalf("PromotedEvaluationIDs(): %v", err)
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			definition, err := eval.LoadPromotedDefinition(id)
			if err != nil {
				t.Fatalf("LoadPromotedDefinition(): %v", err)
			}
			request := eval.PreparationRequest{
				ScenarioID:      definition.ProjectScenario,
				DestinationRoot: filepath.Join(t.TempDir(), "project"),
				ForjExecutable:  os.Args[0],
				OrchestrationID: "catalog-" + id,
				Environment:     os.Environ(),
			}
			plan, err := (Preparer{}).Resolve(context.Background(), request)
			if err != nil {
				t.Fatalf("Resolve(): %v", err)
			}
			if plan.ScenarioID != definition.ProjectScenario || plan.ScenarioSchema != 2 || !plan.TargetOmitted {
				t.Fatalf("plan = %#v", plan)
			}
		})
	}
}

// TestPreparerRequiresHostGuidanceMaterializer prevents evaluations from quietly falling back to an adapter-owned approximation of durable guidance.
func TestPreparerRequiresHostGuidanceMaterializer(t *testing.T) {
	preparer := Preparer{}
	_, err := preparer.MaterializeGuidance(context.Background(), materializedPreparedProject{result: eval.PreparationResult{ProjectRoot: t.TempDir()}}, eval.Guidance{Profile: eval.GuidanceProfileAgents})
	if err == nil || !strings.Contains(err.Error(), "durable guidance materializer") {
		t.Fatalf("MaterializeGuidance() error = %v, want missing host materializer", err)
	}
}

// materializedPreparedProject supplies only the prepared root needed to exercise host guidance delegation.
type materializedPreparedProject struct {
	result eval.PreparationResult
}

// Result returns the fixture's prepared Project identity.
func (project materializedPreparedProject) Result() eval.PreparationResult {
	return project.result
}

// Close has no resources because this fixture models an already owned Project.
func (materializedPreparedProject) Close(context.Context) error {
	return nil
}

// TestPreparationEnvironmentDigestIgnoresOnlyAttemptIsolationPaths preserves base reuse without hiding configuration drift.
func TestPreparationEnvironmentDigestIgnoresOnlyAttemptIsolationPaths(t *testing.T) {
	left := []string{"APP_ENV=test", "GOCACHE=/tmp/left", "GOPATH=/tmp/left-go", "HOME=/tmp/left-home", "PATH=/left/bin", "GOWORK=off"}
	right := []string{"PATH=/right/bin", "HOME=/tmp/right-home", "GOPATH=/tmp/right-go", "GOCACHE=/tmp/right", "APP_ENV=test", "GOWORK=off"}
	if preparationEnvironmentDigest(left) != preparationEnvironmentDigest(right) {
		t.Fatal("attempt-private paths changed the preparation environment identity")
	}
	right[4] = "APP_ENV=production"
	if preparationEnvironmentDigest(left) == preparationEnvironmentDigest(right) {
		t.Fatal("material environment drift retained the same preparation identity")
	}
	right[4] = "APP_ENV=test"
	right[5] = "GOWORK=workspace.go.work"
	if preparationEnvironmentDigest(left) == preparationEnvironmentDigest(right) {
		t.Fatal("GOWORK drift retained the same preparation identity")
	}
}

// testPreparationEnvironment supplies explicit PATH-selected Go and Wire executables for preparation identity tests.
func testPreparationEnvironment(t *testing.T, goContents, goWork string) []string {
	t.Helper()
	tools := t.TempDir()
	goExecutable := filepath.Join(tools, testToolFileName("go"))
	if err := os.WriteFile(goExecutable, []byte(goContents), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tools, testToolFileName("wire")), []byte("wire"), 0o700); err != nil {
		t.Fatal(err)
	}
	environment := []string{"PATH=" + tools, "APP_ENV=test", "GOWORK=" + goWork}
	if runtime.GOOS == "windows" {
		environment = append(environment, "PATHEXT=.EXE")
	}
	return environment
}

// testToolFileName mirrors the platform executable suffix without relying on the host PATH.
func testToolFileName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".EXE"
	}
	return name
}

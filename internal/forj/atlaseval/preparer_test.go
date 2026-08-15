package atlaseval

import (
	"context"
	"os"
	"path/filepath"
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
		Environment:     []string{"APP_ENV=test"},
	}
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
				request.Environment = []string{"APP_ENV=production"}
				return nil
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(forjExecutable, []byte("first"), 0o700); err != nil {
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

// TestPreparationEnvironmentDigestIgnoresOnlyAttemptIsolationPaths preserves base reuse without hiding configuration drift.
func TestPreparationEnvironmentDigestIgnoresOnlyAttemptIsolationPaths(t *testing.T) {
	left := []string{"APP_ENV=test", "GOCACHE=/tmp/left", "GOPATH=/tmp/left-go", "HOME=/tmp/left-home", "PATH=/left/bin"}
	right := []string{"PATH=/right/bin", "HOME=/tmp/right-home", "GOPATH=/tmp/right-go", "GOCACHE=/tmp/right", "APP_ENV=test"}
	if preparationEnvironmentDigest(left) != preparationEnvironmentDigest(right) {
		t.Fatal("attempt-private paths changed the preparation environment identity")
	}
	right[4] = "APP_ENV=production"
	if preparationEnvironmentDigest(left) == preparationEnvironmentDigest(right) {
		t.Fatal("material environment drift retained the same preparation identity")
	}
}

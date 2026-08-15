//go:build integration

package atlaseval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/atlas/eval"
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
	preparer := NewPreparer(baseRoot, nil)
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
		Environment:     testkit.ProcessGoEnv("", nil),
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
}

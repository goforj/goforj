package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/managedsession"
	"github.com/goforj/goforj/project"
)

// TestManagedRuntimePlanEnvironmentUsesGeneratedAppPortKeys keeps Harbor assignments outside project files while matching generated runtime lookup.
func TestManagedRuntimePlanEnvironmentUsesGeneratedAppPortKeys(t *testing.T) {
	plan := managedsession.RuntimePlan{
		Apps: []managedsession.RuntimePlanApp{
			{ID: "app", Active: true, Runtimes: []managedsession.RuntimePlanRuntime{{ID: "http", BindHost: "127.77.1.8", BindPort: 3000, Routes: []managedsession.RuntimePlanRoute{}}}},
			{ID: "billing", Active: true, Runtimes: []managedsession.RuntimePlanRuntime{{ID: "http", BindHost: "127.77.1.8", BindPort: 3001, Routes: []managedsession.RuntimePlanRoute{}}}},
		},
		ServiceEndpoints: []managedsession.RuntimePlanServiceEndpoint{},
	}
	values, err := managedRuntimePlanEnvironment(plan)
	if err != nil {
		t.Fatalf("managedRuntimePlanEnvironment() error = %v", err)
	}
	want := map[string]string{
		"API_HTTP_HOST":         "127.77.1.8",
		"API_HTTP_PORT":         "3000",
		"PORT":                  "3000",
		"BILLING_API_HTTP_PORT": "3001",
		"BILLING_PORT":          "3001",
	}
	if len(values) != len(want) {
		t.Fatalf("managed runtime environment = %#v, want %#v", values, want)
	}
	for key, value := range want {
		if values[key] != value {
			t.Fatalf("managed runtime environment[%q] = %q, want %q", key, values[key], value)
		}
	}
}

// TestManagedRuntimePlanEnvironmentRejectsUnsupportedRuntime keeps future runtime kinds out of an HTTP-only overlay.
func TestManagedRuntimePlanEnvironmentRejectsUnsupportedRuntime(t *testing.T) {
	plan := managedsession.RuntimePlan{
		Apps:             []managedsession.RuntimePlanApp{{ID: "app", Active: true, Runtimes: []managedsession.RuntimePlanRuntime{{ID: "worker", BindHost: "127.77.1.8", BindPort: 3000, Routes: []managedsession.RuntimePlanRoute{}}}}},
		ServiceEndpoints: []managedsession.RuntimePlanServiceEndpoint{},
	}
	_, err := managedRuntimePlanEnvironment(plan)
	if err == nil || !strings.Contains(err.Error(), "does not support runtime") {
		t.Fatalf("managedRuntimePlanEnvironment() error = %v, want unsupported-runtime rejection", err)
	}
}

// TestInstallManagedRuntimeOverlayRestoresPreviousState keeps the assignment layer process-local and reversible.
func TestInstallManagedRuntimeOverlayRestoresPreviousState(t *testing.T) {
	previous, err := installManagedRuntimeOverlay(managedsession.RuntimePlan{Apps: []managedsession.RuntimePlanApp{}, ServiceEndpoints: []managedsession.RuntimePlanServiceEndpoint{}})
	if err != nil {
		t.Fatalf("install empty overlay: %v", err)
	}
	defer previous()

	restore, err := installManagedRuntimeOverlay(managedsession.RuntimePlan{
		Apps:             []managedsession.RuntimePlanApp{{ID: project.DefaultAppName, Active: true, Runtimes: []managedsession.RuntimePlanRuntime{{ID: "http", BindHost: "127.77.1.8", BindPort: 3000, Routes: []managedsession.RuntimePlanRoute{}}}}},
		ServiceEndpoints: []managedsession.RuntimePlanServiceEndpoint{},
	})
	if err != nil {
		t.Fatalf("install runtime overlay: %v", err)
	}
	if current := currentManagedRuntimeOverlay()["API_HTTP_HOST"]; current != "127.77.1.8" {
		t.Fatalf("current managed runtime host = %q, want assigned host", current)
	}
	restore()
	if current := currentManagedRuntimeOverlay(); len(current) != 0 {
		t.Fatalf("restored managed runtime overlay = %#v, want empty", current)
	}
}

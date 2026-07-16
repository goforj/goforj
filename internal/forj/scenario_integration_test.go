//go:build integration

package forj

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
)

// TestRunnableScenarioPathIntegration replays the complete documented path in a fresh rendered App.
func TestRunnableScenarioPathIntegration(t *testing.T) {
	binPath := testkit.EnsureIntegrationForjBinary(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "scenario:test", "runtime-observability")
	cmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("scenario:test runtime-observability failed: %v\n%s", err, output.String())
	}
}

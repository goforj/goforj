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

func TestScenarioJSONAPIRouteIntegration(t *testing.T) {
	binPath := testkit.EnsureIntegrationForjBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "scenario:test", "json-api-route")
	cmd.Env = testkit.IntegrationProcessEnv(t, nil)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("scenario:test json-api-route failed: %v\n%s", err, out.String())
	}
}

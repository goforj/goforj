//go:build integration

package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

func TestRenderedMailpitDefaultsWithDocker(t *testing.T) {
	projectDir := t.TempDir()

	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "Mailpit Test App",
			GoModuleName: "example.com/mailpittestapp",
			UpdatedAt:    "2026-04-27 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					CLI:           true,
					WebAPI:        true,
					Mail:          true,
					Docker:        true,
					DatabaseMySQL: true,
				},
			},
		},
	})

	composeText := readRenderedFile(t, projectDir, "docker-compose.yml")
	for _, token := range []string{
		"mailpit:",
		"image: axllent/mailpit:v1.27",
		"${MAILPIT_SMTP_PORT:-1025}:1025",
		"${MAILPIT_HTTP_PORT:-8025}:8025",
	} {
		if !strings.Contains(composeText, token) {
			t.Fatalf("docker-compose.yml missing %q\n%s", token, composeText)
		}
	}

	envText := readRenderedFile(t, projectDir, ".env")
	for _, token := range []string{
		"MAIL_DRIVER=smtp",
		"MAIL_SUPPORTED_DRIVERS=smtp",
		"MAIL_SMTP_HOST=mailpit",
		"MAIL_SMTP_PORT=1025",
		"# Mailpit",
		"MAILPIT_SMTP_PORT=1025",
		"MAILPIT_HTTP_PORT=8025",
	} {
		if !strings.Contains(envText, token) {
			t.Fatalf(".env missing %q\n%s", token, envText)
		}
	}
}

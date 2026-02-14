package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
)

func TestTestOpenAPICmdRunBuildsOpenAPIAndValidates(t *testing.T) {
	cmd := NewTestOpenAPICmd(logger.NewSilentLogger())

	var validatedPath string
	cmd.validateSpecFn = func(_ string, buildDir string, _ bool) error {
		validatedPath = filepath.Join(buildDir, "openapi.json")
		b, err := os.ReadFile(validatedPath)
		if err != nil {
			return err
		}
		s := string(b)
		if !strings.Contains(s, `"openapi": "3.`) {
			t.Fatalf("expected openapi version field, got: %s", s)
		}
		if !strings.Contains(s, `"/items/{id}"`) {
			t.Fatalf("expected openapi document, got: %s", string(b))
		}
		return nil
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if validatedPath == "" {
		t.Fatalf("expected validator to be called")
	}
}

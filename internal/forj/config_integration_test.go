//go:build integration

package forj

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

func writeProjectConfigFile(t *testing.T, dir string, cfg project.Config) {
	t.Helper()
	body, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal .goforj.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".goforj.yml"), body, 0o644); err != nil {
		t.Fatalf("write .goforj.yml: %v", err)
	}
}


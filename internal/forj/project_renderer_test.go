package forj

import (
	"os"
	"strings"
	"testing"
)

func TestSyncCoreLibrariesUsesCurrentQueueVersion(t *testing.T) {
	data, err := os.ReadFile("project_renderer.go")
	if err != nil {
		t.Fatalf("read project_renderer.go: %v", err)
	}
	source := string(data)

	if !strings.Contains(source, `github.com/goforj/queue@v0.1.6`) {
		t.Fatal("expected syncCoreLibraries to pin github.com/goforj/queue@v0.1.6")
	}
	if strings.Contains(source, `github.com/goforj/queue@v0.1.5`) {
		t.Fatal("found stale github.com/goforj/queue@v0.1.5 pin in syncCoreLibraries")
	}
}

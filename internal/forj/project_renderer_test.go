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

	if !strings.Contains(source, `github.com/goforj/queue@v0.1.7`) {
		t.Fatal("expected syncCoreLibraries to pin github.com/goforj/queue@v0.1.7")
	}
	if strings.Contains(source, `github.com/goforj/queue@v0.1.6`) {
		t.Fatal("found stale github.com/goforj/queue@v0.1.6 pin in syncCoreLibraries")
	}
	if !strings.Contains(source, `github.com/goforj/events/eventscore@v0.1.0`) {
		t.Fatal("expected syncCoreLibraries to pin github.com/goforj/events/eventscore@v0.1.0")
	}
	if strings.Contains(source, `github.com/goforj/events/eventscore@v0.0.0`) {
		t.Fatal("found stale github.com/goforj/events/eventscore@v0.0.0 pin in syncCoreLibraries")
	}
	if !strings.Contains(source, `github.com/goforj/storage/driver/redisstorage@v0.4.4`) {
		t.Fatal("expected syncCoreLibraries to pin github.com/goforj/storage/driver/redisstorage@v0.4.4")
	}
	if strings.Contains(source, `github.com/goforj/storage/driver/redisstorage@v0.3.0`) {
		t.Fatal("found stale github.com/goforj/storage/driver/redisstorage@v0.3.0 pin in syncCoreLibraries")
	}
	if !strings.Contains(source, `github.com/goforj/web@v0.3.0`) {
		t.Fatal("expected syncCoreLibraries to pin github.com/goforj/web@v0.3.0")
	}
	if strings.Contains(source, "`github.com/goforj/web`,") {
		t.Fatal("found unpinned github.com/goforj/web entry in syncCoreLibraries")
	}
}

func TestProjectRendererSyncsLighthouseLocalAuthRoute(t *testing.T) {
	data, err := os.ReadFile("project_renderer.go")
	if err != nil {
		t.Fatalf("read project_renderer.go: %v", err)
	}
	source := string(data)
	if !strings.Contains(source, `requires: []string{`) || !strings.Contains(source, `"/auth/dev-session"`) {
		t.Fatal("expected project renderer sync to require the lighthouse dev session auth route")
	}
}

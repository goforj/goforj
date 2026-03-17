package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateEventFilesUsesSelectedDriverImports(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "redis")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "events"), 0o755); err != nil {
		t.Fatalf("mkdir events package: %v", err)
	}

	written, err := GenerateEventFiles(root)
	if err != nil {
		t.Fatalf("GenerateEventFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated event files to be written")
	}

	driverGen, err := os.ReadFile(filepath.Join(root, "internal", "events", "driver_gen.go"))
	if err != nil {
		t.Fatalf("read driver_gen.go: %v", err)
	}
	source := string(driverGen)

	if !strings.Contains(source, `"github.com/goforj/events/driver/redisevents"`) {
		t.Fatal("expected generated events config to import redisevents")
	}
	if strings.Contains(source, `"github.com/goforj/events/driver/natsevents"`) {
		t.Fatal("did not expect generated events config to import natsevents")
	}
	if !strings.Contains(source, `goforjevents.New(goforjevents.Config{`) {
		t.Fatal("expected generated events config to construct root events API")
	}
	if !strings.Contains(source, `return newManagedBus(api, driver.Ready`) {
		t.Fatal("expected generated events config to wrap redis driver in managed bus")
	}
}

func TestGenerateEventFilesRejectsUnknownEnvVars(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "redis")
	t.Setenv("EVENTS_UNKNOWN_VALUE", "1")

	_, err := GenerateEventFiles(t.TempDir())
	if err == nil {
		t.Fatal("expected GenerateEventFiles to reject unknown events env vars")
	}
	if !strings.Contains(err.Error(), "EVENTS_UNKNOWN_VALUE") {
		t.Fatalf("expected error to mention unknown env var, got: %v", err)
	}
}

func TestGenerateEventFilesAllowsInactiveRootDriverEnvVars(t *testing.T) {
	t.Setenv("EVENTS_DRIVER", "inproc")
	t.Setenv("EVENTS_ADDR", "127.0.0.1:6379")
	t.Setenv("EVENTS_REDIS_CHANNEL_PREFIX", "events")

	if _, err := GenerateEventFiles(t.TempDir()); err != nil {
		t.Fatalf("expected GenerateEventFiles to allow documented inactive root events env vars, got %v", err)
	}
}

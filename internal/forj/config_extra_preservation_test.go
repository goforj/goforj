package forj

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestLegacyWatcherHasNoOverridesRejectsUnknownFields prevents migration from deleting newer watcher controls.
func TestLegacyWatcherHasNoOverridesRejectsUnknownFields(t *testing.T) {
	watch := project.DevWatch{
		Watch: "-file .go",
		Extra: map[string]any{"future_control": "owner-value"},
	}
	if legacyWatcherHasNoOverrides(watch) {
		t.Fatal("watcher with preserved future fields was classified as framework-owned")
	}

	watch.Extra = nil
	watch.Files.Extra = map[string]any{"future_matcher": true}
	if legacyWatcherHasNoOverrides(watch) {
		t.Fatal("watcher with preserved matcher fields was classified as framework-owned")
	}
}

// TestSetAppConfigPreservesUnknownFields keeps newer App configuration intact when an older CLI updates known choices.
func TestSetAppConfigPreservesUnknownFields(t *testing.T) {
	extra := map[string]any{"future_app_setting": map[string]any{"enabled": true}}
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			"worker": {Components: project.Components{CLI: true}, Extra: extra},
		},
	}
	renderer := projectRendererForTest(t, config)

	if _, err := renderer.setAppConfig("worker", project.Components{CLI: true}, project.StarterKitNone, nil, project.DefaultHelpFormat()); err != nil {
		t.Fatalf("set App config: %v", err)
	}
	if got := config.Apps["worker"].Extra; !reflect.DeepEqual(got, extra) {
		t.Fatalf("App extras = %#v, want %#v", got, extra)
	}
}

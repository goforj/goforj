package rendercheck

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// failAfterWriter exposes buffered list flush failures after a fixed number of successful writes.
type failAfterWriter struct {
	writes    int
	failAfter int
}

// stubRenderWorkspaceFS returns controlled preparation failures without mutating the test workspace.
type stubRenderWorkspaceFS struct {
	removeAllErr error
	statErr      error
}

// removeAll returns the cleanup result selected by the test case.
func (filesystem stubRenderWorkspaceFS) removeAll(string) error {
	return filesystem.removeAllErr
}

// stat returns the inspection result selected by the test case.
func (filesystem stubRenderWorkspaceFS) stat(string) (fs.FileInfo, error) {
	return nil, filesystem.statErr
}

// Write returns a closed-pipe error after the configured successful write count.
func (writer *failAfterWriter) Write(data []byte) (int, error) {
	if writer.writes >= writer.failAfter {
		return 0, io.ErrClosedPipe
	}
	writer.writes++
	return len(data), nil
}

// TestRenderComboWorkerReturnsInvalidAppFailure verifies worker validation failures return to the coordinator instead of terminating the process.
func TestRenderComboWorkerReturnsInvalidAppFailure(t *testing.T) {
	wantCause := "unsafe App name"
	worker := renderComboWorker{
		workspaceRoot:  t.TempDir(),
		moduleCache:    "/tmp/gomodcache",
		buildCache:     "/tmp/gocache",
		forjExecutable: "forj",
		runTests:       false,
		filesystem:     osRenderWorkspaceFS{},
	}
	failure := worker.run(renderCombo{
		id: "invalid_app",
		apps: map[string]project.AppConfig{
			"../outside": {},
		},
	})

	if failure == nil {
		t.Fatal("worker.run() failure = nil, want invalid App failure")
	}
	if failure.reason != "invalid configured App" {
		t.Fatalf("failure reason = %q, want %q", failure.reason, "invalid configured App")
	}
	if failure.comboID != "invalid_app" {
		t.Fatalf("failure combo = %q, want %q", failure.comboID, "invalid_app")
	}
	if failure.config != nil {
		t.Fatalf("failure config = %#v, want nil before config construction", failure.config)
	}
	if !strings.Contains(failure.err.Error(), wantCause) {
		t.Fatalf("failure error = %q, want substring %q", failure.err, wantCause)
	}
}

// TestRenderComboWorkerReturnsWorkspacePreparationFailures keeps stale or unreadable workspaces from reaching the renderer.
func TestRenderComboWorkerReturnsWorkspacePreparationFailures(t *testing.T) {
	cleanupErr := errors.New("cleanup denied")
	statErr := errors.New("stat denied")
	tests := []struct {
		name       string
		reason     string
		filesystem stubRenderWorkspaceFS
		cause      error
	}{
		{
			name:       "cleanup",
			reason:     "workspace cleanup failed",
			filesystem: stubRenderWorkspaceFS{removeAllErr: cleanupErr},
			cause:      cleanupErr,
		},
		{
			name:       "go_mod_stat",
			reason:     "go mod init failed",
			filesystem: stubRenderWorkspaceFS{statErr: statErr},
			cause:      statErr,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			worker := renderComboWorker{
				workspaceRoot: filepath.Join(t.TempDir(), "worker"),
				filesystem:    test.filesystem,
			}
			failure := worker.run(renderCombo{id: test.name})

			if failure == nil {
				t.Fatal("worker.run() failure = nil, want workspace preparation failure")
			}
			if failure.reason != test.reason {
				t.Fatalf("failure reason = %q, want %q", failure.reason, test.reason)
			}
			if !errors.Is(failure, test.cause) {
				t.Fatalf("failure error = %v, want cause %v", failure.err, test.cause)
			}
			if failure.config != nil {
				t.Fatalf("failure config = %#v, want nil before config construction", failure.config)
			}
		})
	}
}

// TestRenderComboFailuresRetainEveryCause verifies the command summary remains inspectable after concurrent work is aggregated.
func TestRenderComboFailuresRetainEveryCause(t *testing.T) {
	firstCause := errors.New("first failure")
	secondCause := errors.New("second failure")
	failures := aggregateRenderComboFailures(
		[]*renderComboFailure{
			newRenderComboFailure("go build failed", "combo_b", nil, secondCause),
			newRenderComboFailure("render failed", "combo_a", nil, firstCause),
		},
		5,
		" (shard 1/2 · total 10)",
	)

	wantSummary := "2 of 5 render combinations failed (shard 1/2 · total 10)"
	if got := failures.Error(); got != wantSummary {
		t.Fatalf("aggregate error = %q, want %q", got, wantSummary)
	}
	if got := []string{failures.failures[0].comboID, failures.failures[1].comboID}; got[0] != "combo_a" || got[1] != "combo_b" {
		t.Fatalf("aggregate order = %v, want [combo_a combo_b]", got)
	}
	for _, cause := range []error{firstCause, secondCause} {
		if !errors.Is(failures, cause) {
			t.Fatalf("aggregate error does not retain %q", cause)
		}
	}
}

// TestWriteRenderComboFailurePreservesDiagnostics verifies deferred reporting keeps the established failure details.
func TestWriteRenderComboFailurePreservesDiagnostics(t *testing.T) {
	cfg := &project.Config{ProjectName: "FailureFixture"}
	var output strings.Builder
	writeRenderComboFailure(&output, newRenderComboFailure("render failed", "combo_a", cfg, errors.New("boom")))

	for _, want := range []string{
		"reason: render failed",
		"combo: combo_a",
		"error: boom",
		"project_name: FailureFixture",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("failure output missing %q:\n%s", want, output.String())
		}
	}
}

// TestBuildRenderCombosSkipsInvalidAuthSelections verifies every profile honors the renderer's component contract.
func TestBuildRenderCombosSkipsInvalidAuthSelections(t *testing.T) {
	for _, profile := range []string{renderProfileSmoke, renderProfilePR, renderProfileFull} {
		for _, combo := range buildRenderCombos(profile) {
			if combo.components.Auth && !combo.components.WebAPI {
				t.Fatalf("%s combo includes invalid auth selection: %#v", profile, combo.components)
			}
			if err := combo.components.ValidateRenderContract(); err != nil {
				t.Fatalf("%s combo %q violates render contract: %v", profile, combo.id, err)
			}
		}
	}
}

// TestSelectedRenderProfile verifies named profiles and the legacy full flag retain their precedence.
func TestSelectedRenderProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		full    bool
		want    string
		wantErr bool
	}{
		{name: "default", want: renderProfilePR},
		{name: "smoke", profile: renderProfileSmoke, want: renderProfileSmoke},
		{name: "pr", profile: renderProfilePR, want: renderProfilePR},
		{name: "full", profile: renderProfileFull, want: renderProfileFull},
		{name: "legacy full flag wins", profile: renderProfileSmoke, full: true, want: renderProfileFull},
		{name: "unknown fails", profile: "unknown", wantErr: true},
		{name: "legacy full flag wins over unknown profile", profile: "unknown", full: true, want: renderProfileFull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectedRenderProfile(tt.profile, tt.full)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "valid profiles: smoke, pr, full") {
					t.Fatalf("selectedRenderProfile(%q, %v) error = %v, want valid-profile error", tt.profile, tt.full, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectedRenderProfile(%q, %v) error: %v", tt.profile, tt.full, err)
			}
			if got != tt.want {
				t.Fatalf("selectedRenderProfile(%q, %v) = %q, want %q", tt.profile, tt.full, got, tt.want)
			}
		})
	}
}

// TestSuiteListWritesSelectedShard verifies the public listing boundary preserves profile and shard diagnostics.
func TestSuiteListWritesSelectedShard(t *testing.T) {
	t.Setenv("FORJ_TEST_RENDERS_SHARD_COUNT", "2")
	t.Setenv("FORJ_TEST_RENDERS_SHARD_INDEX", "1")

	suite, err := NewSuite(renderProfileSmoke, false)
	if err != nil {
		t.Fatalf("NewSuite() error: %v", err)
	}
	var output strings.Builder
	if err := suite.List(&output); err != nil {
		t.Fatalf("List() error: %v", err)
	}
	for _, want := range []string{
		"profile: smoke",
		"(shard 2/2 · total ",
		"\nID ",
		"Components\n",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("List() output missing %q:\n%s", want, output.String())
		}
	}
}

// TestShardRenderCombosRejectsInvalidConfiguration keeps malformed CI shard settings from silently running the wrong matrix.
func TestShardRenderCombosRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		count string
		index string
		want  string
	}{
		{name: "count is not an integer", count: "two", want: `invalid FORJ_TEST_RENDERS_SHARD_COUNT="two"`},
		{name: "count is below minimum", count: "0", want: `invalid FORJ_TEST_RENDERS_SHARD_COUNT="0"`},
		{name: "index is not an integer", count: "2", index: "one", want: `invalid FORJ_TEST_RENDERS_SHARD_INDEX="one"`},
		{name: "index is below minimum", count: "2", index: "-1", want: `invalid FORJ_TEST_RENDERS_SHARD_INDEX="-1"`},
		{name: "index is outside count", count: "1", index: "1", want: "FORJ_TEST_RENDERS_SHARD_INDEX=1 must be < FORJ_TEST_RENDERS_SHARD_COUNT=1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("FORJ_TEST_RENDERS_SHARD_COUNT", test.count)
			t.Setenv("FORJ_TEST_RENDERS_SHARD_INDEX", test.index)

			_, _, err := shardRenderCombos([]renderCombo{{id: "fixture"}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("shardRenderCombos() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

// TestSuiteListReturnsFlushErrors verifies buffered table output cannot fail silently at the public listing boundary.
func TestSuiteListReturnsFlushErrors(t *testing.T) {
	suite, err := NewSuite(renderProfileSmoke, false)
	if err != nil {
		t.Fatalf("NewSuite() error: %v", err)
	}
	writer := &failAfterWriter{failAfter: 2}
	if err := suite.List(writer); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("List() error = %v, want closed pipe", err)
	}
}

// TestRenderProfilesHaveExpectedCoverageShape verifies broader profiles add coverage without collapsing their intended cost tiers.
func TestRenderProfilesHaveExpectedCoverageShape(t *testing.T) {
	smoke := buildRenderCombos(renderProfileSmoke)
	pr := buildRenderCombos(renderProfilePR)
	full := buildRenderCombos(renderProfileFull)

	if len(smoke) == 0 {
		t.Fatal("smoke profile should include at least one combo")
	}
	if len(smoke) >= len(pr) {
		t.Fatalf("smoke profile should be smaller than pr profile: smoke=%d pr=%d", len(smoke), len(pr))
	}
	if len(pr) >= len(full) {
		t.Fatalf("pr profile should be smaller than full profile: pr=%d full=%d", len(pr), len(full))
	}
	if !renderCombosInclude(smoke, func(c project.Components) bool {
		return c.Auth && c.WebAPI && c.DatabaseMySQL
	}) {
		t.Fatal("smoke profile should include an auth/webapi/mysql combo")
	}
	if !renderCombosInclude(pr, func(c project.Components) bool {
		return c.DatabaseMySQL
	}) || !renderCombosInclude(pr, func(c project.Components) bool {
		return c.DatabasePostgres
	}) || !renderCombosInclude(pr, func(c project.Components) bool {
		return c.DatabaseSQLite
	}) {
		t.Fatal("pr profile should include mysql, postgres, and sqlite coverage")
	}
}

// TestPRRenderProfileIncludesRecommendedAndLeanPrimitiveSentinels keeps compatibility and opt-out coverage explicit.
func TestPRRenderProfileIncludesRecommendedAndLeanPrimitiveSentinels(t *testing.T) {
	combos := buildRenderCombos(renderProfilePR)
	recommended := project.DefaultSelectedComponents()
	foundRecommended := false
	foundLean := false
	for _, combo := range combos {
		if combo.id == "sentinel_recommended_default" {
			foundRecommended = true
			if combo.components != recommended {
				t.Fatalf("recommended sentinel = %#v, want %#v", combo.components, recommended)
			}
			if !combo.components.Cache || !combo.components.Events || !combo.components.Storage || !combo.components.Jobs {
				t.Fatalf("recommended sentinel omitted a default primitive: %#v", combo.components)
			}
		}
		if !combo.components.Cache && !combo.components.Events && !combo.components.Storage && !combo.components.Jobs {
			foundLean = true
		}
	}
	if !foundRecommended {
		t.Fatal("pr profile should include the recommended-default sentinel")
	}
	if !foundLean {
		t.Fatal("pr profile should include a lean primitive-free sentinel")
	}
}

// TestPRRenderProfileIncludesPrimitiveBoundaryAndMixedAppSentinels keeps component gates covered at both project and App projections.
func TestPRRenderProfileIncludesPrimitiveBoundaryAndMixedAppSentinels(t *testing.T) {
	combos := buildRenderCombos(renderProfilePR)
	for _, id := range []string{
		"sentinel_primitives_all_on",
		"sentinel_cache_only",
		"sentinel_events_only",
		"sentinel_storage_only",
		"sentinel_web_metrics_grafana_without_primitives",
		"sentinel_default_events_named_app_off",
		"sentinel_default_jobs_named_app_off",
	} {
		requireRenderComboID(t, combos, id)
	}

	for _, test := range []struct {
		id      string
		appName string
		key     project.ComponentKey
	}{
		{id: "sentinel_named_app_cache_only", appName: "cache-worker", key: project.ComponentCache},
		{id: "sentinel_named_app_events_only", appName: "events-worker", key: project.ComponentEvents},
		{id: "sentinel_named_app_storage_only", appName: "storage-worker", key: project.ComponentStorage},
		{id: "sentinel_named_app_mail_only", appName: "mailer", key: project.ComponentMail},
		{id: "sentinel_named_app_jobs_only", appName: "worker", key: project.ComponentJobs},
		{id: "sentinel_named_app_database_only", appName: "database-worker", key: project.ComponentDatabaseSQLite},
		{id: "sentinel_named_app_auth_only", appName: "auth-api", key: project.ComponentAuth},
		{id: "sentinel_named_app_scheduler_only", appName: "scheduler-worker", key: project.ComponentScheduler},
	} {
		t.Run(test.id, func(t *testing.T) {
			combo := requireRenderComboID(t, combos, test.id)
			if combo.components.Enabled(test.key) {
				t.Fatalf("default App unexpectedly enables %q", test.key)
			}
			configured, ok := combo.apps[test.appName]
			if !ok || !configured.Components.Enabled(test.key) {
				t.Fatalf("named App %q does not enable %q: %#v", test.appName, test.key, combo.apps)
			}
			config := &project.Config{Render: project.RenderConfig{Components: combo.components}, Apps: combo.apps}
			if !project.ProjectComponents(config).Enabled(test.key) {
				t.Fatalf("named App did not promote %q into the project envelope: %#v", test.key, config)
			}
		})
	}

	for _, test := range []struct {
		id      string
		appName string
		key     project.ComponentKey
	}{
		{id: "sentinel_default_events_named_app_off", appName: "api", key: project.ComponentEvents},
		{id: "sentinel_default_jobs_named_app_off", appName: "api", key: project.ComponentJobs},
	} {
		t.Run(test.id, func(t *testing.T) {
			combo := requireRenderComboID(t, combos, test.id)
			if !combo.components.Enabled(test.key) {
				t.Fatalf("default App does not enable %q", test.key)
			}
			configured, ok := combo.apps[test.appName]
			if !ok || configured.Components.Enabled(test.key) {
				t.Fatalf("named App %q unexpectedly enables %q: %#v", test.appName, test.key, combo.apps)
			}
		})
	}
}

// TestComponentLabelsMatchCatalogSelection keeps render diagnostics truthful about every selected component.
func TestComponentLabelsMatchCatalogSelection(t *testing.T) {
	components := project.Components{Cache: true, Events: true, Storage: true}
	labels := componentLabels(components)
	seen := make(map[string]bool, len(labels))
	for _, label := range labels {
		seen[label] = true
	}
	for _, definition := range project.ComponentCatalog() {
		if got, want := seen[definition.Label], components.Enabled(definition.Key); got != want {
			t.Fatalf("component label %q presence = %t, want %t: %#v", definition.Label, got, want, labels)
		}
	}
}

// TestPRRenderProfileCoversComponentCatalog verifies pull-request coverage includes every selectable component.
func TestPRRenderProfileCoversComponentCatalog(t *testing.T) {
	combos := buildRenderCombos(renderProfilePR)
	requireRenderCombosCoverCatalog(t, renderProfilePR, combos)
}

// TestFullRenderProfileCoversComponentCatalog verifies exhaustive coverage includes every selectable component.
func TestFullRenderProfileCoversComponentCatalog(t *testing.T) {
	combos := buildRenderCombos(renderProfileFull)
	requireRenderCombosCoverCatalog(t, renderProfileFull, combos)
}

// requireRenderCombosCoverCatalog centralizes catalog assertions so profile tests report missing component keys consistently.
func requireRenderCombosCoverCatalog(t *testing.T, profile string, combos []renderCombo) {
	t.Helper()
	for _, definition := range project.ComponentCatalog() {
		if !renderCombosInclude(combos, func(c project.Components) bool {
			return c.Enabled(definition.Key)
		}) {
			t.Fatalf("%s profile should include component %q at least once", profile, definition.Key)
		}
	}
}

// TestPRRenderProfileIncludesCriticalInteractions verifies the curated profile protects cross-component render boundaries.
func TestPRRenderProfileIncludesCriticalInteractions(t *testing.T) {
	combos := buildRenderCombos(renderProfilePR)
	requireRenderCombo(t, combos, "auth webapi db", func(c project.Components) bool {
		return c.Auth && c.WebAPI && c.HasDatabase()
	})
	requireRenderCombo(t, combos, "oauth auth db", func(c project.Components) bool {
		return c.OAuth && c.Auth && c.HasDatabase()
	})
	requireRenderCombo(t, combos, "scheduler jobs", func(c project.Components) bool {
		return c.Scheduler && c.Jobs
	})
	requireRenderCombo(t, combos, "metrics observability grafana", func(c project.Components) bool {
		return c.Metrics && c.Observability && c.Grafana
	})
	requireRenderCombo(t, combos, "demo app database jobs", func(c project.Components) bool {
		return c.DemoApp && c.HasDatabase() && c.Jobs
	})
}

// TestRenderCombosHaveUniqueIDs keeps sharding and failure aggregation deterministic.
func TestRenderCombosHaveUniqueIDs(t *testing.T) {
	for _, profile := range []string{renderProfileSmoke, renderProfilePR, renderProfileFull} {
		seen := map[string]struct{}{}
		for _, combo := range buildRenderCombos(profile) {
			if _, ok := seen[combo.id]; ok {
				t.Fatalf("%s profile contains duplicate combo id %q", profile, combo.id)
			}
			seen[combo.id] = struct{}{}
		}
	}
}

// TestRenderComboAppsIncludesConfiguredAppsInStableOrder keeps multi-App compile validation deterministic.
func TestRenderComboAppsIncludesConfiguredAppsInStableOrder(t *testing.T) {
	apps, err := renderComboApps(renderCombo{apps: map[string]project.AppConfig{
		"worker":    {},
		"backstage": {},
	}})
	if err != nil {
		t.Fatalf("renderComboApps() error: %v", err)
	}
	want := []string{"app", "backstage", "worker"}
	if len(apps) != len(want) {
		t.Fatalf("render Apps = %#v, want %v", apps, want)
	}
	for index, name := range want {
		if apps[index].Name != name {
			t.Fatalf("render App %d = %q, want %q", index, apps[index].Name, name)
		}
	}
}

// TestSeedRenderComboAppsCreatesNamedEntrypointMarkers verifies clean workspaces expose configured Apps to conventional discovery.
func TestSeedRenderComboAppsCreatesNamedEntrypointMarkers(t *testing.T) {
	root := t.TempDir()
	apps := []project.App{project.DefaultApp(), project.DefaultNamedApp("events-worker")}
	if err := seedRenderComboApps(root, apps); err != nil {
		t.Fatalf("seedRenderComboApps() error: %v", err)
	}
	namedEntrypoint := filepath.Join(root, "cmd", "events-worker", "main.go")
	content, err := os.ReadFile(namedEntrypoint)
	if err != nil {
		t.Fatalf("read named App marker: %v", err)
	}
	if string(content) != "package main\n" {
		t.Fatalf("named App marker = %q", content)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "app", "main.go")); !os.IsNotExist(err) {
		t.Fatalf("default App marker should be owned by the normal renderer: %v", err)
	}
}

// requireRenderCombo reports interaction coverage with a human-readable contract name.
func requireRenderCombo(t *testing.T, combos []renderCombo, name string, matches func(project.Components) bool) {
	t.Helper()
	if !renderCombosInclude(combos, matches) {
		t.Fatalf("pr profile should include %s coverage", name)
	}
}

// requireRenderComboID returns one declared sentinel so its exact App projection can be asserted.
func requireRenderComboID(t *testing.T, combos []renderCombo, id string) renderCombo {
	t.Helper()
	for _, combo := range combos {
		if combo.id == id {
			return combo
		}
	}
	t.Fatalf("render profile should include %q", id)
	return renderCombo{}
}

// renderCombosInclude keeps profile assertions independent of matrix ordering.
func renderCombosInclude(combos []renderCombo, matches func(project.Components) bool) bool {
	for _, combo := range combos {
		if matches(combo.components) {
			return true
		}
	}
	return false
}

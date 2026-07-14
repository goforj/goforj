package forj

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/goforj/project"
)

// TestNewProjectResourceUIDefaultFocus verifies the normal path advances with one Enter press.
func TestNewProjectResourceUIDefaultFocus(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	if ui.screen != newProjectResourceScreenSummary {
		t.Fatalf("screen = %d, want summary", ui.screen)
	}
	if ui.summaryFocus != newProjectResourceFocusContinue {
		t.Fatalf("summary focus = %d, want Continue", ui.summaryFocus)
	}

	next, action, err := ui.update(tea.KeyMsg{Type: tea.KeyEnter})
	if err != nil {
		t.Fatalf("update Enter returned error: %v", err)
	}
	if action != newProjectResourceActionContinue {
		t.Fatalf("action = %d, want Continue", action)
	}
	if next.summaryFocus != newProjectResourceFocusContinue {
		t.Fatal("accepting the summary should not move its retained focus")
	}
}

// TestNewProjectResourceUIPresets verifies exact normal active drivers and local Redis intent.
func TestNewProjectResourceUIPresets(t *testing.T) {
	components := resourceTestComponents()
	ui := mustNewProjectResourceUI(t, components)
	assertResourceActive(t, ui, project.ResourceCache, "memory")
	assertResourceActive(t, ui, project.ResourceQueue, "workerpool")
	assertResourceActive(t, ui, project.ResourceEvents, "inproc")
	if got := ui.classification().Label; got != "Standalone resources" {
		t.Fatalf("standalone classification = %q", got)
	}
	if _, ok := ui.localServiceIntent.Mode(project.ServiceRedis); ok {
		t.Fatal("Standalone resources should not request an unused local Redis service")
	}

	if err := ui.applyShape(project.ResourceShapeSharedRedis); err != nil {
		t.Fatalf("applyShape(shared) returned error: %v", err)
	}
	assertResourceActive(t, ui, project.ResourceCache, "redis")
	assertResourceActive(t, ui, project.ResourceQueue, "redis")
	assertResourceActive(t, ui, project.ResourceEvents, "redis")
	if got := ui.classification().Label; got != "Shared through Redis" {
		t.Fatalf("shared classification = %q", got)
	}
	if mode, ok := ui.localServiceIntent.Mode(project.ServiceRedis); !ok || mode != project.LocalServiceModeLocal {
		t.Fatalf("shared Redis placement = %q, %v; want local", mode, ok)
	}
}

// TestNewProjectResourceUIJobsOmission verifies disabled Jobs disappear from plans and human copy.
func TestNewProjectResourceUIJobsOmission(t *testing.T) {
	components := resourceTestComponents()
	components.Jobs = false
	ui := mustNewProjectResourceUI(t, components)
	if _, ok := ui.plan.Selection(project.ResourceQueue); ok {
		t.Fatal("queue selection should be absent when Jobs is disabled")
	}
	standalone := ui.renderShapeBody(80)
	if strings.Contains(strings.ToLower(standalone), "job") || strings.Contains(strings.ToLower(standalone), "workerpool") {
		t.Fatalf("shape copy mentions disabled Jobs:\n%s", standalone)
	}
	if err := ui.applyShape(project.ResourceShapeSharedRedis); err != nil {
		t.Fatalf("applyShape(shared) returned error: %v", err)
	}
	if _, ok := ui.plan.Selection(project.ResourceQueue); ok {
		t.Fatal("shared plan should also omit queue when Jobs is disabled")
	}
}

// TestNewProjectResourceUIDatabaseIndependence verifies database edits preserve the normal shape and other resources.
func TestNewProjectResourceUIDatabaseIndependence(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	beforeCache, _ := ui.plan.Selection(project.ResourceCache)
	beforeEvents, _ := ui.plan.Selection(project.ResourceEvents)
	if err := ui.setNormalDatabase("postgres"); err != nil {
		t.Fatalf("setNormalDatabase(postgres) returned error: %v", err)
	}
	if got := ui.classification().Label; got != "Standalone resources" {
		t.Fatalf("database edit changed classification to %q", got)
	}
	if got := ui.databaseDriver(); got != "postgres" {
		t.Fatalf("database driver = %q, want postgres", got)
	}
	afterCache, _ := ui.plan.Selection(project.ResourceCache)
	afterEvents, _ := ui.plan.Selection(project.ResourceEvents)
	assertDriverSelectionEqual(t, afterCache, beforeCache)
	assertDriverSelectionEqual(t, afterEvents, beforeEvents)
	components := ui.componentsWithDatabase(resourceTestComponents())
	if !components.DatabasePostgres || components.DatabaseMySQL || components.DatabaseSQLite {
		t.Fatalf("database projection = %#v, want only Postgres", components)
	}
}

// TestNewProjectResourceUINormalDatabaseBuildsSelectedDriver verifies the concise picker does not silently widen database support.
func TestNewProjectResourceUINormalDatabaseBuildsSelectedDriver(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	if err := ui.setNormalDatabase("postgres"); err != nil {
		t.Fatalf("setNormalDatabase(postgres) returned error: %v", err)
	}
	selection, _ := ui.plan.Selection(project.ResourceDatabase)
	if selection.Active != "postgres" || strings.Join(selection.Supported, ",") != "postgres" {
		t.Fatalf("normal database selection = %#v, want Postgres only", selection)
	}
	if err := ui.toggleSupported(project.ResourceDatabase, "mysql"); err != nil {
		t.Fatalf("add Advanced MySQL support: %v", err)
	}
	if err := ui.setNormalDatabase("sqlite"); err != nil {
		t.Fatalf("setNormalDatabase(sqlite) after Advanced support returned error: %v", err)
	}
	selection, _ = ui.plan.Selection(project.ResourceDatabase)
	if selection.Active != "sqlite" || strings.Join(selection.Supported, ",") != "sqlite,mysql,postgres" {
		t.Fatalf("custom database support was not retained: %#v", selection)
	}
	rows, err := ui.confirmationRows()
	if err != nil {
		t.Fatalf("confirmationRows returned error: %v", err)
	}
	if got := resourceTestRow(rows, "Database"); !strings.Contains(got, "built in") || !strings.Contains(got, "MySQL") || !strings.Contains(got, "Postgres") {
		t.Fatalf("custom database build contract was hidden: %q", got)
	}
}

// TestNewProjectResourceUIAdvancedDatabaseActiveRetainsSupport verifies the normal picker cannot erase an Advanced build decision.
func TestNewProjectResourceUIAdvancedDatabaseActiveRetainsSupport(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	if err := ui.setActive(project.ResourceDatabase, "postgres"); err != nil {
		t.Fatalf("set Advanced Postgres active: %v", err)
	}
	if err := ui.setNormalDatabase("sqlite"); err != nil {
		t.Fatalf("set normal SQLite active: %v", err)
	}
	selection, _ := ui.plan.Selection(project.ResourceDatabase)
	if selection.Active != "sqlite" || strings.Join(selection.Supported, ",") != "sqlite,mysql,postgres" {
		t.Fatalf("normal picker erased Advanced support: %#v", selection)
	}
}

// TestNewProjectResourceUIActiveAutoSupport verifies Advanced cannot select a driver absent from the build.
func TestNewProjectResourceUIActiveAutoSupport(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	if err := ui.setActive(project.ResourceStorage, "redis"); err != nil {
		t.Fatalf("setActive(storage, redis) returned error: %v", err)
	}
	selection, _ := ui.plan.Selection(project.ResourceStorage)
	if selection.Active != "redis" || !newProjectResourceContainsDriver(selection.Supported, "redis") {
		t.Fatalf("storage selection = %#v, want active Redis built in", selection)
	}
	if mode, ok := ui.localServiceIntent.Mode(project.ServiceRedis); !ok || mode != project.LocalServiceModeLocal {
		t.Fatalf("Redis placement = %q, %v; want local", mode, ok)
	}
}

// TestNewProjectResourceUISupportedRemovalLocks verifies active and named drivers cannot be removed.
func TestNewProjectResourceUISupportedRemovalLocks(t *testing.T) {
	components := resourceTestComponents()
	components.Auth = true
	components.WebAPI = true
	ui := mustNewProjectResourceUI(t, components)
	if err := ui.toggleSupported(project.ResourceCache, "memory"); err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("active removal error = %v, want active lock", err)
	}
	if err := ui.applyShape(project.ResourceShapeSharedRedis); err != nil {
		t.Fatalf("applyShape(shared) returned error: %v", err)
	}
	if err := ui.setActive(project.ResourceCache, "file"); err != nil {
		t.Fatalf("setActive(cache, file) returned error: %v", err)
	}
	if err := ui.toggleSupported(project.ResourceCache, "redis"); err == nil || !strings.Contains(err.Error(), "Auth sessions") {
		t.Fatalf("named removal error = %v, want Auth sessions lock", err)
	}
}

// TestNewProjectResourceUIResetUsesCurrentShape verifies Reset keeps shape and independent database choice.
func TestNewProjectResourceUIResetUsesCurrentShape(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	if err := ui.setDatabase("postgres"); err != nil {
		t.Fatalf("setDatabase(postgres) returned error: %v", err)
	}
	if err := ui.applyShape(project.ResourceShapeSharedRedis); err != nil {
		t.Fatalf("applyShape(shared) returned error: %v", err)
	}
	if err := ui.setActive(project.ResourceEvents, "null"); err != nil {
		t.Fatalf("setActive(events, null) returned error: %v", err)
	}
	if err := ui.reset(); err != nil {
		t.Fatalf("reset returned error: %v", err)
	}
	if got := ui.classification().Label; got != "Shared through Redis" {
		t.Fatalf("classification after reset = %q", got)
	}
	if got := ui.databaseDriver(); got != "postgres" {
		t.Fatalf("database after reset = %q, want retained postgres", got)
	}
	assertResourceActive(t, ui, project.ResourceEvents, "redis")
}

// TestNewProjectResourceUIBackRetention verifies nested Back navigation does not discard Advanced edits.
func TestNewProjectResourceUIBackRetention(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	if err := ui.setActive(project.ResourceEvents, "null"); err != nil {
		t.Fatalf("setActive(events, null) returned error: %v", err)
	}
	ui.openAdvanced()
	ui.openActiveForFocusedResource()
	if action := ui.back(); action != newProjectResourceActionNone || ui.screen != newProjectResourceScreenAdvanced {
		t.Fatalf("nested back = action %d screen %d", action, ui.screen)
	}
	if action := ui.back(); action != newProjectResourceActionNone || ui.screen != newProjectResourceScreenSummary {
		t.Fatalf("Advanced back = action %d screen %d", action, ui.screen)
	}
	assertResourceActive(t, ui, project.ResourceEvents, "null")
	if action := ui.back(); action != newProjectResourceActionBack {
		t.Fatalf("summary back action = %d, want parent Back", action)
	}
}

// TestNewProjectResourceUIClassificationLabels verifies exact normal, support, customization, and Custom labels.
func TestNewProjectResourceUIClassificationLabels(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	if got := ui.classification().Label; got != "Standalone resources" {
		t.Fatalf("initial label = %q", got)
	}
	if err := ui.toggleSupported(project.ResourceCache, "file"); err != nil {
		t.Fatalf("add file support returned error: %v", err)
	}
	if got := ui.classification().Label; got != "Standalone resources · custom support" {
		t.Fatalf("support label = %q", got)
	}
	if err := ui.setActive(project.ResourceStorage, "memory"); err != nil {
		t.Fatalf("set storage memory returned error: %v", err)
	}
	if got := ui.classification().Label; got != "Standalone resources · custom support · customized" {
		t.Fatalf("customized label = %q", got)
	}
	if err := ui.setActive(project.ResourceEvents, "null"); err != nil {
		t.Fatalf("set events null returned error: %v", err)
	}
	if got := ui.classification().Label; got != "Custom" {
		t.Fatalf("divergent active label = %q", got)
	}
}

// TestNewProjectResourceUIReconcileRetainsDatabase verifies capability toggles restore the independent choice.
func TestNewProjectResourceUIReconcileRetainsDatabase(t *testing.T) {
	components := resourceTestComponents()
	ui := mustNewProjectResourceUI(t, components)
	if err := ui.setDatabase("postgres"); err != nil {
		t.Fatalf("setDatabase(postgres) returned error: %v", err)
	}
	components.DatabaseMySQL = false
	if err := ui.reconcile(components); err != nil {
		t.Fatalf("reconcile(disabled Database) returned error: %v", err)
	}
	if ui.databaseEnabled {
		t.Fatal("Database row should be disabled")
	}
	components.DatabaseMySQL = true
	if err := ui.reconcile(components); err != nil {
		t.Fatalf("reconcile(re-enabled Database) returned error: %v", err)
	}
	if got := ui.databaseDriver(); got != "postgres" {
		t.Fatalf("restored database = %q, want postgres", got)
	}
	assertResourceActive(t, ui, project.ResourceDatabase, "postgres")
}

// TestNewProjectResourceUIDemoDatabaseLock verifies Demo temporarily forces MySQL and restores the prior choice.
func TestNewProjectResourceUIDemoDatabaseLock(t *testing.T) {
	components := resourceTestComponents()
	ui := mustNewProjectResourceUI(t, components)
	if err := ui.setNormalDatabase("postgres"); err != nil {
		t.Fatalf("setNormalDatabase(postgres) returned error: %v", err)
	}
	components.DemoApp = true
	if err := ui.reconcile(components); err != nil {
		t.Fatalf("reconcile(Demo on) returned error: %v", err)
	}
	if !ui.databaseLocked || ui.databaseDriver() != "mysql" {
		t.Fatalf("Demo lock = %v driver %q, want locked MySQL", ui.databaseLocked, ui.databaseDriver())
	}
	database, _ := ui.plan.Selection(project.ResourceDatabase)
	if strings.Join(database.Supported, ",") != "sqlite,mysql" {
		t.Fatalf("Demo database support = %#v, want SQLite fallback and active MySQL", database)
	}
	if err := ui.setDatabase("sqlite"); err == nil || !strings.Contains(err.Error(), "Demo App") {
		t.Fatalf("locked selection error = %v", err)
	}
	if err := ui.toggleSupported(project.ResourceDatabase, "sqlite"); err == nil || !strings.Contains(err.Error(), "fallback") {
		t.Fatalf("SQLite fallback removal error = %v", err)
	}
	databaseAfterRemoval, _ := ui.plan.Selection(project.ResourceDatabase)
	assertDriverSelectionEqual(t, databaseAfterRemoval, database)
	components.DemoApp = false
	if err := ui.reconcile(components); err != nil {
		t.Fatalf("reconcile(Demo off) returned error: %v", err)
	}
	if ui.databaseLocked || ui.databaseDriver() != "postgres" {
		t.Fatalf("restored lock = %v driver %q, want unlocked Postgres", ui.databaseLocked, ui.databaseDriver())
	}
	restoredDatabase, _ := ui.plan.Selection(project.ResourceDatabase)
	assertDriverSelectionEqual(t, restoredDatabase, project.DriverSelection{Active: "postgres", Supported: []string{"postgres"}})
	if classification := ui.classification(); classification.CustomSupport {
		t.Fatalf("restored normal database support was classified as custom: %#v", classification)
	}
}

// TestNewProjectResourceUIDemoDatabaseLockRestoresCustomSupport keeps a starter constraint from rewriting deliberate Advanced portability choices.
func TestNewProjectResourceUIDemoDatabaseLockRestoresCustomSupport(t *testing.T) {
	components := resourceTestComponents()
	ui := mustNewProjectResourceUI(t, components)
	if err := ui.setActive(project.ResourceDatabase, "postgres"); err != nil {
		t.Fatalf("setActive(postgres) returned error: %v", err)
	}
	want, _ := ui.plan.Selection(project.ResourceDatabase)

	components.DemoApp = true
	if err := ui.reconcile(components); err != nil {
		t.Fatalf("reconcile(Demo on) returned error: %v", err)
	}
	components.DemoApp = false
	if err := ui.reconcile(components); err != nil {
		t.Fatalf("reconcile(Demo off) returned error: %v", err)
	}

	restored, _ := ui.plan.Selection(project.ResourceDatabase)
	assertDriverSelectionEqual(t, restored, want)
}

// TestNewProjectResourceUIConfirmationRows verifies confirmation and service summaries use the effective plan.
func TestNewProjectResourceUIConfirmationRows(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	rows, err := ui.confirmationRows()
	if err != nil {
		t.Fatalf("confirmationRows returned error: %v", err)
	}
	if got := resourceTestRow(rows, "Built-in bridge"); got != "Redis for cache, jobs, and events" {
		t.Fatalf("built-in bridge = %q", got)
	}
	if got := resourceTestRow(rows, "App services"); got != "MySQL" {
		t.Fatalf("standalone App services = %q", got)
	}
	if err := ui.setDatabase("sqlite"); err != nil {
		t.Fatalf("setDatabase(sqlite) returned error: %v", err)
	}
	if err := ui.applyShape(project.ResourceShapeSharedRedis); err != nil {
		t.Fatalf("applyShape(shared) returned error: %v", err)
	}
	rows, err = ui.confirmationRows()
	if err != nil {
		t.Fatalf("shared confirmationRows returned error: %v", err)
	}
	if got := resourceTestRow(rows, "App services"); got != "Redis" {
		t.Fatalf("shared App services = %q", got)
	}
	if got := resourceTestRow(rows, "Resource notice"); !strings.Contains(got, "SQLite and local storage remain filesystem-local") {
		t.Fatalf("shared SQLite notice = %q", got)
	}
}

// TestNewProjectResourceUICustomConfirmationExpandsSelections keeps Advanced choices visible at the final decision.
func TestNewProjectResourceUICustomConfirmationExpandsSelections(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	if err := ui.setActive(project.ResourceEvents, "nats"); err != nil {
		t.Fatalf("set NATS events: %v", err)
	}
	if err := ui.setActive(project.ResourceMail, "resend"); err != nil {
		t.Fatalf("set Resend mail: %v", err)
	}
	rows, err := ui.confirmationRows()
	if err != nil {
		t.Fatalf("confirmationRows returned error: %v", err)
	}
	if got := resourceTestRow(rows, "Events"); !strings.Contains(got, "NATS") || !strings.Contains(got, "built in") {
		t.Fatalf("expanded Events row = %q", got)
	}
	if got := resourceTestRow(rows, "Mail"); !strings.Contains(got, "Resend") || !strings.Contains(got, "built in") {
		t.Fatalf("expanded Mail row = %q", got)
	}
	if got := resourceTestRow(rows, "Active resources"); got != "" {
		t.Fatalf("custom confirmation retained ambiguous compact row %q", got)
	}
}

// TestNewProjectResourceUIPlacementActionFollowsDriverMetadata avoids advertising placement on process-local rows.
func TestNewProjectResourceUIPlacementActionFollowsDriverMetadata(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	ui.openAdvanced()
	if ui.focusedPlacementSelectable() {
		t.Fatal("default database row unexpectedly exposes placement")
	}
	before := ui.screen
	next, _, err := ui.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if err != nil || next.screen != before {
		t.Fatalf("unavailable placement key changed screen=%d error=%v", next.screen, err)
	}
	for index, definition := range ui.applicableDefinitions() {
		if definition.Key == project.ResourceCache {
			ui.advancedIndex = index
			break
		}
	}
	if err := ui.setActive(project.ResourceCache, "redis"); err != nil {
		t.Fatalf("set Redis cache: %v", err)
	}
	if !ui.focusedPlacementSelectable() {
		t.Fatal("Redis cache did not expose placement")
	}
}

// TestNewProjectResourceUIImplicitRedisPlacementTracksSelections keeps automatic placement from becoming owner intent.
func TestNewProjectResourceUIImplicitRedisPlacementTracksSelections(t *testing.T) {
	components := resourceTestComponents()
	ui := mustNewProjectResourceUI(t, components)
	if err := ui.setActive(project.ResourceCache, "redis"); err != nil {
		t.Fatalf("activate Redis cache: %v", err)
	}
	if mode, _ := ui.localServiceIntent.Mode(project.ServiceRedis); mode != project.LocalServiceModeLocal {
		t.Fatalf("implicit Docker placement = %q, want local", mode)
	}
	if err := ui.setActive(project.ResourceCache, "memory"); err != nil {
		t.Fatalf("restore memory cache: %v", err)
	}
	if _, exists := ui.localServiceIntent.Mode(project.ServiceRedis); exists {
		t.Fatal("implicit Redis placement survived after the last active consumer was removed")
	}

	components.Docker = false
	if err := ui.reconcile(components); err != nil {
		t.Fatalf("reconcile Docker off: %v", err)
	}
	if err := ui.setActive(project.ResourceCache, "redis"); err != nil {
		t.Fatalf("activate external Redis cache: %v", err)
	}
	if mode, _ := ui.localServiceIntent.Mode(project.ServiceRedis); mode != project.LocalServiceModeExternal {
		t.Fatalf("implicit no-Docker placement = %q, want external", mode)
	}
	components.Docker = true
	if err := ui.reconcile(components); err != nil {
		t.Fatalf("reconcile Docker on: %v", err)
	}
	if mode, _ := ui.localServiceIntent.Mode(project.ServiceRedis); mode != project.LocalServiceModeLocal {
		t.Fatalf("implicit placement did not follow Docker capability: %q", mode)
	}
}

// TestNewProjectResourceUIExplicitRedisPlacementSurvivesInactiveTransition preserves a deliberate local-service choice.
func TestNewProjectResourceUIExplicitRedisPlacementSurvivesInactiveTransition(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	if err := ui.setActive(project.ResourceCache, "redis"); err != nil {
		t.Fatalf("activate Redis cache: %v", err)
	}
	if err := ui.setPlacement(project.ServiceRedis, project.LocalServiceModeLocal); err != nil {
		t.Fatalf("select local Redis: %v", err)
	}
	if err := ui.setActive(project.ResourceCache, "memory"); err != nil {
		t.Fatalf("restore memory cache: %v", err)
	}
	if mode, exists := ui.localServiceIntent.Mode(project.ServiceRedis); !exists || mode != project.LocalServiceModeLocal {
		t.Fatalf("explicit placement = %q exists=%t, want retained local intent", mode, exists)
	}
}

// TestNewProjectResourceUIRenderingWidth verifies every editor slice fits the existing panel content width.
func TestNewProjectResourceUIRenderingWidth(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	width := 58
	screens := []newProjectResourceScreen{
		newProjectResourceScreenSummary,
		newProjectResourceScreenShape,
		newProjectResourceScreenDatabase,
		newProjectResourceScreenAdvanced,
	}
	for _, screen := range screens {
		ui.screen = screen
		assertResourceRenderWidth(t, ui.renderBody(width), newProjectResourceContentWidth(width))
	}
	for _, key := range []project.ResourceKey{project.ResourceCache, project.ResourceQueue, project.ResourceEvents, project.ResourceStorage} {
		ui.editingResource = key
		ui.screen = newProjectResourceScreenActive
		assertResourceRenderWidth(t, ui.renderBody(width), newProjectResourceContentWidth(width))
		ui.screen = newProjectResourceScreenSupported
		assertResourceRenderWidth(t, ui.renderBody(width), newProjectResourceContentWidth(width))
	}
	ui.placementService = project.ServiceRedis
	ui.screen = newProjectResourceScreenPlacement
	assertResourceRenderWidth(t, ui.renderBody(width), newProjectResourceContentWidth(width))
}

// TestNewProjectResourceUIDefensiveCopies verifies renderer handoff cannot mutate retained wizard state.
func TestNewProjectResourceUIDefensiveCopies(t *testing.T) {
	ui := mustNewProjectResourceUI(t, resourceTestComponents())
	plan := ui.resourcePlan()
	selection := plan.Selections[project.ResourceCache]
	selection.Supported[0] = "changed"
	plan.Selections[project.ResourceCache] = selection
	plan.NamedSelections["CACHE_INSPECTS_DRIVER"] = "changed"
	retained, _ := ui.plan.Selection(project.ResourceCache)
	if retained.Supported[0] == "changed" || ui.plan.NamedSelections["CACHE_INSPECTS_DRIVER"] == "changed" {
		t.Fatal("resourcePlan returned aliased state")
	}
	intent := ui.serviceIntent()
	intent.Modes[project.ServiceRedis] = project.LocalServiceModeExternal
	if _, ok := ui.localServiceIntent.Mode(project.ServiceRedis); ok {
		t.Fatal("serviceIntent returned aliased state")
	}
}

// resourceTestComponents returns a normal capability set focused on App resource behavior.
func resourceTestComponents() project.Components {
	return project.Components{
		Docker:        true,
		DatabaseMySQL: true,
		Jobs:          true,
		Mail:          true,
	}
}

// mustNewProjectResourceUI constructs a resource editor or fails the owning test immediately.
func mustNewProjectResourceUI(t *testing.T, components project.Components) newProjectResourceUI {
	t.Helper()
	ui, err := newProjectResourceUIForComponents(components)
	if err != nil {
		t.Fatalf("newProjectResourceUIForComponents returned error: %v", err)
	}
	return ui
}

// assertResourceActive compares one effective active driver with a readable failure.
func assertResourceActive(t *testing.T, ui newProjectResourceUI, key project.ResourceKey, want string) {
	t.Helper()
	selection, ok := ui.plan.Selection(key)
	if !ok {
		t.Fatalf("resource %s has no selection", key)
	}
	if selection.Active != want {
		t.Fatalf("resource %s active = %q, want %q", key, selection.Active, want)
	}
}

// assertDriverSelectionEqual compares active and supported values without relying on map representation.
func assertDriverSelectionEqual(t *testing.T, got project.DriverSelection, want project.DriverSelection) {
	t.Helper()
	if got.Active != want.Active || strings.Join(got.Supported, ",") != strings.Join(want.Supported, ",") {
		t.Fatalf("selection = %#v, want %#v", got, want)
	}
}

// resourceTestRow finds one derived confirmation value by its stable label.
func resourceTestRow(rows []keyValue, key string) string {
	for _, row := range rows {
		if row.key == key {
			return row.value
		}
	}
	return ""
}

// assertResourceRenderWidth checks terminal-cell width rather than byte length because styles add ANSI sequences.
func assertResourceRenderWidth(t *testing.T, body string, width int) {
	t.Helper()
	for lineNumber, line := range strings.Split(body, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line %d width = %d, want <= %d:\n%s", lineNumber+1, got, width, body)
		}
	}
}

package forj

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/goforj/goforj/project"
)

// newProjectResourceScreen identifies one slice of the App Resources editor.
type newProjectResourceScreen int

const (
	newProjectResourceScreenSummary newProjectResourceScreen = iota
	newProjectResourceScreenShape
	newProjectResourceScreenDatabase
	newProjectResourceScreenAdvanced
	newProjectResourceScreenActive
	newProjectResourceScreenSupported
	newProjectResourceScreenPlacement
)

// newProjectResourceSummaryFocus identifies one focusable row on the normal screen.
type newProjectResourceSummaryFocus int

const (
	newProjectResourceFocusShape newProjectResourceSummaryFocus = iota
	newProjectResourceFocusDatabase
	newProjectResourceFocusContinue
	newProjectResourceFocusAdvanced
)

// newProjectResourceAction tells the parent wizard when the resource editor has finished or backed out.
type newProjectResourceAction int

const (
	newProjectResourceActionNone newProjectResourceAction = iota
	newProjectResourceActionContinue
	newProjectResourceActionBack
)

// newProjectResourceUI owns transient resource choices without leaking them into project YAML.
type newProjectResourceUI struct {
	screen                 newProjectResourceScreen
	summaryFocus           newProjectResourceSummaryFocus
	shapeIndex             int
	databaseIndex          int
	advancedIndex          int
	driverIndex            int
	driverViewportOffset   int
	placementIndex         int
	editingResource        project.ResourceKey
	placementService       project.ServiceKey
	components             project.Components
	plan                   project.ResourcePlan
	localServiceIntent     project.LocalServiceIntent
	effectiveConsumers     []project.EffectiveResourceConsumer
	baseShape              project.StartingResourceShape
	databaseChoice         string
	retainedDatabaseChoice string
	databaseBeforeLock     project.DriverSelection
	databaseBeforeLockSet  bool
	databaseEnabled        bool
	databaseLocked         bool
	databaseSupportCustom  bool
	redisPlacementExplicit bool
	errorMessage           string
}

// newProjectResourceServiceSummary separates locally managed and externally required App services.
type newProjectResourceServiceSummary struct {
	local     []string
	external  []string
	available []string
	warnings  []string
}

// newProjectResourceUIForComponents resolves the one-Enter normal default for the selected capabilities.
func newProjectResourceUIForComponents(components project.Components) (newProjectResourceUI, error) {
	databaseChoice := components.DatabaseDriver()
	if databaseChoice == "" {
		databaseChoice = "mysql"
	}
	ui := newProjectResourceUI{
		screen:                 newProjectResourceScreenSummary,
		summaryFocus:           newProjectResourceFocusContinue,
		baseShape:              project.ResourceShapeStandalone,
		databaseChoice:         databaseChoice,
		retainedDatabaseChoice: databaseChoice,
		localServiceIntent:     project.LocalServiceIntent{},
	}
	if err := ui.reconcile(components); err != nil {
		return newProjectResourceUI{}, err
	}
	return ui, nil
}

// newNewProjectResourceUI preserves the constructor name used while the parent wizard integration is landing.
func newNewProjectResourceUI(components project.Components) (newProjectResourceUI, error) {
	return newProjectResourceUIForComponents(components)
}

// clone returns an editor state whose transient maps and slices cannot alias an earlier Bubble Tea model.
func (ui newProjectResourceUI) clone() newProjectResourceUI {
	ui.plan = ui.plan.Clone()
	ui.localServiceIntent = cloneNewProjectLocalServiceIntent(ui.localServiceIntent)
	ui.effectiveConsumers = cloneEffectiveResourceConsumers(ui.effectiveConsumers)
	ui.databaseBeforeLock.Supported = append([]string(nil), ui.databaseBeforeLock.Supported...)
	return ui
}

// resourcePlan returns a defensive copy for renderer handoff.
func (ui newProjectResourceUI) resourcePlan() project.ResourcePlan {
	return ui.plan.Clone()
}

// serviceIntent returns a defensive copy for service planning and renderer handoff.
func (ui newProjectResourceUI) serviceIntent() project.LocalServiceIntent {
	return cloneNewProjectLocalServiceIntent(ui.localServiceIntent)
}

// databaseDriver returns the independent database selection even while its capability is temporarily disabled.
func (ui newProjectResourceUI) databaseDriver() string {
	return ui.databaseChoice
}

// componentsWithDatabase projects the App Resources database choice onto transitional render-capability flags.
func (ui newProjectResourceUI) componentsWithDatabase(components project.Components) project.Components {
	components.DatabaseMySQL = false
	components.DatabasePostgres = false
	components.DatabaseSQLite = false
	if !ui.databaseEnabled {
		return components
	}
	switch ui.databaseChoice {
	case "postgres":
		components.DatabasePostgres = true
	case "sqlite":
		components.DatabaseSQLite = true
	default:
		components.DatabaseMySQL = true
	}
	return components
}

// classification derives the normal or customized label from the effective plan.
func (ui newProjectResourceUI) classification() project.ResourcePlanClassification {
	return project.ClassifyResourcePlan(ui.plan, ui.components)
}

// reconcile retains applicable edits while capabilities are changed on an earlier wizard screen.
func (ui *newProjectResourceUI) reconcile(components project.Components) error {
	if ui.baseShape == "" {
		ui.baseShape = project.ResourceShapeStandalone
	}
	previousPlan := ui.plan.Clone()
	previousComponents := ui.components
	isInitialResolution := len(previousPlan.Selections) == 0
	wasLocked := ui.databaseLocked
	databaseEnabled := components.HasDatabase() || components.Auth || components.OAuth || components.DemoApp
	ui.databaseEnabled = databaseEnabled
	ui.databaseLocked = components.DemoApp
	if ui.databaseLocked && !wasLocked {
		ui.databaseBeforeLock, ui.databaseBeforeLockSet = previousPlan.Selection(project.ResourceDatabase)
	}
	if !ui.databaseEnabled && ui.summaryFocus == newProjectResourceFocusDatabase {
		ui.summaryFocus = newProjectResourceFocusContinue
	}

	if selected := components.DatabaseDriver(); selected != "" && !wasLocked && isInitialResolution {
		ui.databaseChoice = selected
		ui.retainedDatabaseChoice = selected
	}
	if ui.databaseChoice == "" {
		ui.databaseChoice = "mysql"
	}
	if ui.retainedDatabaseChoice == "" {
		ui.retainedDatabaseChoice = ui.databaseChoice
	}
	if ui.databaseLocked {
		if ui.databaseChoice != "mysql" {
			ui.retainedDatabaseChoice = ui.databaseChoice
		}
		ui.databaseChoice = "mysql"
	} else if wasLocked && ui.retainedDatabaseChoice != "" {
		ui.databaseChoice = ui.retainedDatabaseChoice
	}

	components = ui.componentsWithDatabase(components)
	ui.components = components
	preset, err := project.ResolveResourcePlan(ui.baseShape, components)
	if err != nil {
		return err
	}
	if isInitialResolution {
		ui.plan = preset
		ui.seedNormalServiceIntent()
		return nil
	}

	reconciled := preset.Clone()
	for _, definition := range project.ResourceCatalog() {
		if !definition.AppliesTo(components) || definition.Key == project.ResourceDatabase {
			continue
		}
		if selection, ok := previousPlan.Selection(definition.Key); ok && definition.AppliesTo(previousComponents) {
			reconciled = reconciled.WithSelection(definition.Key, selection)
		}
	}
	if ui.databaseEnabled {
		previousDatabase, previousDatabaseSet := previousPlan.Selection(project.ResourceDatabase)
		if wasLocked && !ui.databaseLocked {
			previousDatabase = ui.databaseBeforeLock
			previousDatabase.Supported = append([]string(nil), ui.databaseBeforeLock.Supported...)
			previousDatabaseSet = ui.databaseBeforeLockSet
		}
		if previousDatabaseSet && !ui.databaseLocked {
			previousDatabase.Active = ui.databaseChoice
			if !newProjectResourceContainsDriver(previousDatabase.Supported, ui.databaseChoice) {
				previousDatabase.Supported = append(previousDatabase.Supported, ui.databaseChoice)
			}
			reconciled = reconciled.WithSelection(project.ResourceDatabase, previousDatabase)
		}
	}

	reconciled.NamedSelections = map[string]string{}
	for _, named := range reconciled.GeneratedNamedSelections(components) {
		if driver, ok := previousPlan.NamedSelections[named.EnvironmentKey]; ok {
			reconciled.NamedSelections[named.EnvironmentKey] = driver
			continue
		}
		reconciled.NamedSelections[named.EnvironmentKey] = named.Active
	}
	reconciled.Shape = ui.baseShape
	ui.plan = reconciled
	if normalized, normalizeErr := ui.plan.Normalized(components); normalizeErr == nil {
		ui.plan = normalized
		if wasLocked && !ui.databaseLocked {
			ui.databaseBeforeLock = project.DriverSelection{}
			ui.databaseBeforeLockSet = false
		}
		ui.syncImplicitRedisIntent()
		ui.errorMessage = ""
		return nil
	} else {
		ui.errorMessage = normalizeErr.Error()
		return normalizeErr
	}
}

// applyShape reapplies only shape-owned root and named resources while preserving independent choices.
func (ui *newProjectResourceUI) applyShape(shape project.StartingResourceShape) error {
	preset, err := project.ResolveResourcePlan(shape, ui.components)
	if err != nil {
		return err
	}
	next := ui.plan.Clone()
	next.Shape = shape
	for _, key := range newProjectShapeManagedResourceKeys() {
		if selection, ok := preset.Selection(key); ok {
			next = next.WithSelection(key, selection)
		} else {
			next = next.WithoutSelection(key)
		}
	}
	for _, definition := range project.ResourceCatalog() {
		if !newProjectResourceIsShapeManaged(definition.Key) {
			continue
		}
		for _, named := range definition.NamedResources {
			delete(next.NamedSelections, named.EnvironmentKey)
			if driver, ok := preset.NamedSelections[named.EnvironmentKey]; ok {
				next.NamedSelections[named.EnvironmentKey] = driver
			}
		}
	}
	normalized, err := next.Normalized(ui.components)
	if err != nil {
		return err
	}
	ui.baseShape = shape
	ui.plan = normalized
	ui.shapeIndex = newProjectShapeIndex(shape)
	ui.seedNormalServiceIntent()
	ui.errorMessage = ""
	return nil
}

// setDatabase changes the independent active database without resetting other resource edits.
func (ui *newProjectResourceUI) setDatabase(driver string) error {
	if !ui.databaseEnabled {
		return fmt.Errorf("Database is not enabled for this App")
	}
	driver = strings.ToLower(strings.TrimSpace(driver))
	if ui.databaseLocked && driver != "mysql" {
		return fmt.Errorf("Demo App requires MySQL")
	}
	definition, ok := project.ResourceDefinitionByKey(project.ResourceDatabase)
	if !ok {
		return fmt.Errorf("Database resource definition is unavailable")
	}
	if _, ok := definition.Driver(driver); !ok {
		return fmt.Errorf("Database has no %q driver", driver)
	}
	selection, ok := ui.plan.Selection(project.ResourceDatabase)
	if !ok {
		selection = project.DriverSelection{Supported: []string{driver}}
	}
	selection.Active = driver
	if !newProjectResourceContainsDriver(selection.Supported, driver) {
		selection.Supported = append(selection.Supported, driver)
	}
	ui.databaseChoice = driver
	ui.retainedDatabaseChoice = driver
	ui.components = ui.componentsWithDatabase(ui.components)
	normalized, err := ui.plan.WithSelection(project.ResourceDatabase, selection).Normalized(ui.components)
	if err != nil {
		return err
	}
	ui.plan = normalized
	ui.databaseIndex = newProjectDatabaseIndex(driver)
	ui.errorMessage = ""
	return nil
}

// setNormalDatabase applies the concise picker contract unless Advanced explicitly customized database support.
func (ui *newProjectResourceUI) setNormalDatabase(driver string) error {
	if ui.databaseSupportCustom || ui.databaseLocked {
		return ui.setDatabase(driver)
	}
	if err := ui.setDatabase(driver); err != nil {
		return err
	}
	selection, ok := ui.plan.Selection(project.ResourceDatabase)
	if !ok {
		return nil
	}
	selection.Supported = []string{selection.Active}
	normalized, err := ui.plan.WithSelection(project.ResourceDatabase, selection).Normalized(ui.components)
	if err != nil {
		return err
	}
	ui.plan = normalized
	return nil
}

// setActive selects a starting driver and guarantees that the generated App builds it in.
func (ui *newProjectResourceUI) setActive(key project.ResourceKey, driver string) error {
	if key == project.ResourceDatabase {
		if err := ui.setDatabase(driver); err != nil {
			return err
		}
		ui.databaseSupportCustom = true
		return nil
	}
	definition, ok := project.ResourceDefinitionByKey(key)
	if !ok || !definition.AppliesTo(ui.components) {
		return fmt.Errorf("resource %q is not enabled for this App", key)
	}
	driver = strings.ToLower(strings.TrimSpace(driver))
	_, ok = definition.Driver(driver)
	if !ok {
		return fmt.Errorf("%s has no %q driver", definition.Label, driver)
	}
	selection, ok := ui.plan.Selection(key)
	if !ok {
		return fmt.Errorf("%s has no resource selection", definition.Label)
	}
	selection.Active = driver
	if !newProjectResourceContainsDriver(selection.Supported, driver) {
		selection.Supported = append(selection.Supported, driver)
	}
	normalized, err := ui.plan.WithSelection(key, selection).Normalized(ui.components)
	if err != nil {
		return err
	}
	ui.plan = normalized
	ui.syncImplicitRedisIntent()
	ui.errorMessage = ""
	return nil
}

// toggleSupported changes the compiled driver set while protecting active and generated named requirements.
func (ui *newProjectResourceUI) toggleSupported(key project.ResourceKey, driver string) error {
	definition, ok := project.ResourceDefinitionByKey(key)
	if !ok || !definition.AppliesTo(ui.components) {
		return fmt.Errorf("resource %q is not enabled for this App", key)
	}
	driver = strings.ToLower(strings.TrimSpace(driver))
	if _, ok := definition.Driver(driver); !ok {
		return fmt.Errorf("%s has no %q driver", definition.Label, driver)
	}
	selection, ok := ui.plan.Selection(key)
	if !ok {
		return fmt.Errorf("%s has no resource selection", definition.Label)
	}
	if newProjectResourceContainsDriver(selection.Supported, driver) {
		if selection.Active == driver {
			return fmt.Errorf("%s is active and must stay built into the App", driver)
		}
		if key == project.ResourceDatabase && ui.components.DemoApp && driver == "sqlite" {
			return fmt.Errorf("SQLite is required as the Demo App database fallback and must stay built into the App")
		}
		for _, named := range ui.plan.GeneratedNamedSelections(ui.components) {
			if named.Resource == key && named.Active == driver {
				return fmt.Errorf("%s is required by %s", driver, named.Label)
			}
		}
		nextSupported := make([]string, 0, len(selection.Supported)-1)
		for _, supported := range selection.Supported {
			if supported != driver {
				nextSupported = append(nextSupported, supported)
			}
		}
		selection.Supported = nextSupported
	} else {
		selection.Supported = append(selection.Supported, driver)
	}
	normalized, err := ui.plan.WithSelection(key, selection).Normalized(ui.components)
	if err != nil {
		return err
	}
	ui.plan = normalized
	if key == project.ResourceDatabase {
		ui.databaseSupportCustom = true
	}
	ui.errorMessage = ""
	return nil
}

// setPlacement records whether a placement-selectable active service is local or externally managed.
func (ui *newProjectResourceUI) setPlacement(service project.ServiceKey, mode project.LocalServiceMode) error {
	if mode != project.LocalServiceModeLocal && mode != project.LocalServiceModeExternal {
		return fmt.Errorf("service %s has unknown placement %q", service, mode)
	}
	if mode == project.LocalServiceModeLocal && !ui.components.Docker {
		return fmt.Errorf("Local Compose placement requires Docker")
	}
	if !ui.activeServiceIsPlacementSelectable(service) {
		return fmt.Errorf("service %s is not selected by an active placement-selectable driver", service)
	}
	ui.localServiceIntent = ui.localServiceIntent.WithMode(service, mode)
	if service == project.ServiceRedis {
		ui.redisPlacementExplicit = true
	}
	ui.errorMessage = ""
	return nil
}

// reset restores the currently selected normal shape instead of resetting the whole wizard.
func (ui *newProjectResourceUI) reset() error {
	preset, err := project.ResolveResourcePlan(ui.baseShape, ui.components)
	if err != nil {
		return err
	}
	ui.plan = preset
	ui.databaseChoice = presetDatabaseDriver(preset, ui.databaseChoice)
	ui.databaseSupportCustom = false
	ui.seedNormalServiceIntent()
	ui.errorMessage = ""
	return nil
}

// back returns to the owning screen while preserving every resource selection.
func (ui *newProjectResourceUI) back() newProjectResourceAction {
	switch ui.screen {
	case newProjectResourceScreenSummary:
		return newProjectResourceActionBack
	case newProjectResourceScreenShape, newProjectResourceScreenDatabase, newProjectResourceScreenAdvanced:
		ui.screen = newProjectResourceScreenSummary
	default:
		ui.screen = newProjectResourceScreenAdvanced
	}
	ui.errorMessage = ""
	return newProjectResourceActionNone
}

// update applies navigation keys without coupling the resource editor to the parent wizard stage enum.
func (ui newProjectResourceUI) update(msg tea.KeyMsg) (newProjectResourceUI, newProjectResourceAction, error) {
	next := ui.clone()
	next.errorMessage = ""
	if msg.Type == tea.KeyEsc || msg.Type == tea.KeyShiftTab || msg.Type == tea.KeyCtrlB || msg.Type == tea.KeyLeft {
		return next, next.back(), nil
	}
	key := msg.String()
	if next.screen == newProjectResourceScreenSummary && key == "a" {
		next.openAdvanced()
		return next, newProjectResourceActionNone, nil
	}
	if next.screen == newProjectResourceScreenAdvanced && key == "r" {
		return next, newProjectResourceActionNone, next.reset()
	}
	if key == "up" || key == "k" {
		next.move(-1)
		return next, newProjectResourceActionNone, nil
	}
	if key == "down" || key == "j" {
		next.move(1)
		return next, newProjectResourceActionNone, nil
	}
	if next.screen == newProjectResourceScreenSupported && key == " " {
		return next, newProjectResourceActionNone, next.toggleFocusedSupported()
	}
	if next.screen == newProjectResourceScreenAdvanced && key == "s" {
		next.openSupportedForFocusedResource()
		return next, newProjectResourceActionNone, nil
	}
	if next.screen == newProjectResourceScreenAdvanced && key == "p" {
		if !next.focusedPlacementSelectable() {
			return next, newProjectResourceActionNone, nil
		}
		return next, newProjectResourceActionNone, next.openPlacementForFocusedResource()
	}
	if key != "enter" {
		return next, newProjectResourceActionNone, nil
	}
	return next.activate()
}

// move advances focus within the current resource editor screen and wraps at both ends.
func (ui *newProjectResourceUI) move(delta int) {
	switch ui.screen {
	case newProjectResourceScreenSummary:
		focuses := ui.summaryFocusOrder()
		index := 0
		for candidateIndex, focus := range focuses {
			if focus == ui.summaryFocus {
				index = candidateIndex
				break
			}
		}
		index = newProjectWrappedIndex(index+delta, len(focuses))
		ui.summaryFocus = focuses[index]
	case newProjectResourceScreenShape:
		ui.shapeIndex = newProjectWrappedIndex(ui.shapeIndex+delta, len(newProjectResourceShapes()))
	case newProjectResourceScreenDatabase:
		ui.databaseIndex = newProjectWrappedIndex(ui.databaseIndex+delta, len(newProjectDatabaseDrivers()))
	case newProjectResourceScreenAdvanced:
		ui.advancedIndex = newProjectWrappedIndex(ui.advancedIndex+delta, len(ui.applicableDefinitions()))
	case newProjectResourceScreenActive, newProjectResourceScreenSupported:
		drivers := ui.editingDrivers()
		ui.driverIndex = newProjectWrappedIndex(ui.driverIndex+delta, len(drivers))
		ui.ensureDriverVisible(7)
	case newProjectResourceScreenPlacement:
		ui.placementIndex = newProjectWrappedIndex(ui.placementIndex+delta, len(ui.placementModes()))
	}
}

// activate follows the currently focused row or commits the selected picker value.
func (ui newProjectResourceUI) activate() (newProjectResourceUI, newProjectResourceAction, error) {
	switch ui.screen {
	case newProjectResourceScreenSummary:
		switch ui.summaryFocus {
		case newProjectResourceFocusShape:
			ui.openShape()
		case newProjectResourceFocusDatabase:
			ui.openDatabase()
		case newProjectResourceFocusAdvanced:
			ui.openAdvanced()
		case newProjectResourceFocusContinue:
			return ui, newProjectResourceActionContinue, ui.plan.Validate(ui.components)
		}
	case newProjectResourceScreenShape:
		shape := newProjectResourceShapes()[ui.shapeIndex]
		if err := ui.applyShape(shape); err != nil {
			return ui, newProjectResourceActionNone, err
		}
		ui.screen = newProjectResourceScreenSummary
	case newProjectResourceScreenDatabase:
		driver := newProjectDatabaseDrivers()[ui.databaseIndex]
		if err := ui.setNormalDatabase(driver); err != nil {
			return ui, newProjectResourceActionNone, err
		}
		ui.screen = newProjectResourceScreenSummary
	case newProjectResourceScreenAdvanced:
		ui.openActiveForFocusedResource()
	case newProjectResourceScreenActive:
		drivers := ui.editingDrivers()
		if len(drivers) == 0 {
			return ui, newProjectResourceActionNone, fmt.Errorf("resource has no drivers")
		}
		selected := drivers[ui.driverIndex]
		if err := ui.setActive(ui.editingResource, selected.Name); err != nil {
			return ui, newProjectResourceActionNone, err
		}
		if selected.PlacementSelectable {
			ui.placementService = selected.Service
			ui.openPlacement(selected.Service)
		} else {
			ui.screen = newProjectResourceScreenAdvanced
		}
	case newProjectResourceScreenSupported:
		ui.screen = newProjectResourceScreenAdvanced
	case newProjectResourceScreenPlacement:
		modes := ui.placementModes()
		if len(modes) == 0 {
			return ui, newProjectResourceActionNone, fmt.Errorf("service has no available placements")
		}
		if err := ui.setPlacement(ui.placementService, modes[ui.placementIndex]); err != nil {
			return ui, newProjectResourceActionNone, err
		}
		ui.screen = newProjectResourceScreenAdvanced
	}
	return ui, newProjectResourceActionNone, nil
}

// openShape shows the normal shape picker with the effective base shape focused.
func (ui *newProjectResourceUI) openShape() {
	ui.screen = newProjectResourceScreenShape
	ui.shapeIndex = newProjectShapeIndex(ui.baseShape)
}

// openDatabase shows the independent database picker with the retained choice focused.
func (ui *newProjectResourceUI) openDatabase() {
	ui.screen = newProjectResourceScreenDatabase
	ui.databaseIndex = newProjectDatabaseIndex(ui.databaseChoice)
}

// openAdvanced shows the first applicable resource while preserving the prior row when possible.
func (ui *newProjectResourceUI) openAdvanced() {
	ui.screen = newProjectResourceScreenAdvanced
	ui.advancedIndex = newProjectWrappedIndex(ui.advancedIndex, len(ui.applicableDefinitions()))
}

// openActiveForFocusedResource opens the active-driver picker for the focused Advanced row.
func (ui *newProjectResourceUI) openActiveForFocusedResource() {
	definition, ok := ui.focusedDefinition()
	if !ok {
		return
	}
	ui.editingResource = definition.Key
	ui.screen = newProjectResourceScreenActive
	ui.driverIndex = ui.driverIndexForSelection(definition, true)
	ui.driverViewportOffset = 0
	ui.ensureDriverVisible(7)
}

// openSupportedForFocusedResource opens the built-in-driver picker for the focused Advanced row.
func (ui *newProjectResourceUI) openSupportedForFocusedResource() {
	definition, ok := ui.focusedDefinition()
	if !ok {
		return
	}
	ui.editingResource = definition.Key
	ui.screen = newProjectResourceScreenSupported
	ui.driverIndex = ui.driverIndexForSelection(definition, false)
	ui.driverViewportOffset = 0
	ui.ensureDriverVisible(7)
}

// openPlacementForFocusedResource opens placement only when the active driver explicitly supports it.
func (ui *newProjectResourceUI) openPlacementForFocusedResource() error {
	definition, ok := ui.focusedDefinition()
	if !ok {
		return fmt.Errorf("no resource is selected")
	}
	selection, ok := ui.plan.Selection(definition.Key)
	if !ok {
		return fmt.Errorf("%s has no resource selection", definition.Label)
	}
	driver, ok := definition.Driver(selection.Active)
	if !ok || !driver.PlacementSelectable {
		return fmt.Errorf("%s does not have a selectable service placement", definition.Label)
	}
	ui.openPlacement(driver.Service)
	return nil
}

// openPlacement seeds the placement focus from explicit intent and current Docker capability.
func (ui *newProjectResourceUI) openPlacement(service project.ServiceKey) {
	ui.screen = newProjectResourceScreenPlacement
	ui.placementService = service
	modes := ui.placementModes()
	ui.placementIndex = 0
	if selected, ok := ui.localServiceIntent.Mode(service); ok {
		for index, mode := range modes {
			if mode == selected {
				ui.placementIndex = index
				break
			}
		}
	}
}

// toggleFocusedSupported applies the checkbox action for the focused supported-driver row.
func (ui *newProjectResourceUI) toggleFocusedSupported() error {
	drivers := ui.editingDrivers()
	if len(drivers) == 0 {
		return fmt.Errorf("resource has no drivers")
	}
	return ui.toggleSupported(ui.editingResource, drivers[ui.driverIndex].Name)
}

// renderBody renders the active App Resources slice inside the parent wizard panel.
func (ui newProjectResourceUI) renderBody(width int) string {
	width = newProjectResourceContentWidth(width)
	var body string
	switch ui.screen {
	case newProjectResourceScreenShape:
		body = ui.renderShapeBody(width)
	case newProjectResourceScreenDatabase:
		body = ui.renderDatabaseBody(width)
	case newProjectResourceScreenAdvanced:
		body = ui.renderAdvancedBody(width)
	case newProjectResourceScreenActive:
		body = ui.renderDriverBody(width, false)
	case newProjectResourceScreenSupported:
		body = ui.renderDriverBody(width, true)
	case newProjectResourceScreenPlacement:
		body = ui.renderPlacementBody(width)
	default:
		body = ui.renderSummaryBody(width)
	}
	if ui.errorMessage != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "", errorStyle.Render("x "+ui.errorMessage))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(body)
}

// renderSummaryBody keeps the normal path compact and starts with Continue focused.
func (ui newProjectResourceUI) renderSummaryBody(width int) string {
	lines := newProjectWrappedStyledLines(
		"Choose a starting setup. Most resources can switch drivers later without changing application code.",
		width,
		helpStyle,
	)
	lines = append(lines, "")
	lines = append(lines, ui.renderSummaryChoice(newProjectResourceFocusShape, "Starting resources", ui.classification().Label, width))
	if ui.databaseEnabled {
		database := newProjectResourceDriverLabel(project.ResourceDatabase, ui.databaseChoice)
		if ui.databaseLocked {
			database += " (locked by Demo App)"
		}
		lines = append(lines, ui.renderSummaryChoice(newProjectResourceFocusDatabase, "Database", database, width))
	}
	lines = append(lines, "")
	lines = append(lines, ui.renderSummaryAction(newProjectResourceFocusContinue, "Continue", width))
	lines = append(lines, ui.renderSummaryAction(newProjectResourceFocusAdvanced, "Advanced resource setup", width))
	return newProjectResourceJoinLines(width, lines)
}

// renderShapeBody explains the two coordinated presets without implying that the database is coupled to them.
func (ui newProjectResourceUI) renderShapeBody(width int) string {
	lines := []string{}
	for index, shape := range newProjectResourceShapes() {
		marker := "  "
		nameStyle := listNameStyle
		if index == ui.shapeIndex {
			marker = "> "
			nameStyle = listFocusedNameStyle
		}
		lines = append(lines, marker+nameStyle.Render(shape.Label()))
		description := "Memory cache · In-process events"
		detail := "These resources stay inside the App process. Choose any database separately."
		if shape == project.ResourceShapeSharedRedis {
			description = "Redis cache · Redis events"
			detail = "One Redis service lets these resources work across App processes."
		}
		if ui.components.Jobs {
			if shape == project.ResourceShapeSharedRedis {
				description = "Redis cache · Redis jobs · Redis events"
			} else {
				description = "Memory cache · Workerpool jobs · In-process events"
			}
		}
		lines = append(lines, newProjectWrappedStyledLines(description, width-2, listDescStyle)...)
		lines = append(lines, newProjectWrappedStyledLines(detail, width-2, helpStyle)...)
		if index < len(newProjectResourceShapes())-1 {
			lines = append(lines, "")
		}
	}
	return newProjectResourceJoinLines(width, lines)
}

// renderDatabaseBody shows database as an orthogonal implementation choice and explains temporary locks.
func (ui newProjectResourceUI) renderDatabaseBody(width int) string {
	lines := []string{}
	for index, driver := range newProjectDatabaseDrivers() {
		marker := "  "
		nameStyle := listNameStyle
		if index == ui.databaseIndex {
			marker = "> "
			nameStyle = listFocusedNameStyle
		}
		label := newProjectResourceDriverLabel(project.ResourceDatabase, driver)
		if ui.databaseLocked && driver != "mysql" {
			label += " (unavailable while Demo App is on)"
		}
		lines = append(lines, marker+nameStyle.Render(label))
	}
	if ui.databaseLocked {
		lines = append(lines, "")
		lines = append(lines, newProjectWrappedStyledLines("Demo App currently requires MySQL; your prior database choice will return if Demo App is turned off.", width, helpStyle)...)
	}
	return newProjectResourceJoinLines(width, lines)
}

// renderAdvancedBody renders applicable resources as a stable three-column table.
func (ui newProjectResourceUI) renderAdvancedBody(width int) string {
	resourceWidth := 13
	activeWidth := 18
	supportedWidth := width - resourceWidth - activeWidth - 7
	if supportedWidth < 18 {
		resourceWidth = 10
		activeWidth = 14
		supportedWidth = width - resourceWidth - activeWidth - 7
	}
	header := fmt.Sprintf("  %-*s  %-*s  %s", resourceWidth, "Resource", activeWidth, "Starts with", "Built into App")
	lines := []string{headerLabelStyle.Render(header)}
	for index, definition := range ui.applicableDefinitions() {
		selection, _ := ui.plan.Selection(definition.Key)
		active := newProjectResourceDriverLabel(definition.Key, selection.Active)
		supported := newProjectResourceSupportedLabels(definition, selection.Supported)
		row := fmt.Sprintf("  %-*s  %-*s  %s", resourceWidth, definition.Label, activeWidth, active, supported)
		if index == ui.advancedIndex {
			row = "> " + strings.TrimPrefix(row, "  ")
			row = listFocusedNameStyle.Render(row)
		} else {
			row = listNameStyle.Render(row)
		}
		lines = append(lines, lipgloss.NewStyle().MaxWidth(width).Render(row))
	}
	lines = append(lines, "")
	actions := "Enter: starting driver · s: built-in drivers"
	if ui.focusedPlacementSelectable() {
		actions += " · p: placement"
	}
	lines = append(lines, helpStyle.Render(actions+" · r: reset"))
	return newProjectResourceJoinLines(width, lines)
}

// renderDriverBody renders a bounded catalog viewport for active or supported driver editing.
func (ui newProjectResourceUI) renderDriverBody(width int, supported bool) string {
	definition, _ := project.ResourceDefinitionByKey(ui.editingResource)
	title := "Starting driver for " + definition.Label
	if supported {
		title = "Drivers built into " + definition.Label
	}
	lines := []string{sectionLabelStyle.Render(title), ""}
	drivers := ui.editingDrivers()
	end := ui.driverViewportOffset + 7
	if end > len(drivers) {
		end = len(drivers)
	}
	selection, _ := ui.plan.Selection(ui.editingResource)
	previousGroup := project.DriverGroup("")
	for index := ui.driverViewportOffset; index < end; index++ {
		driver := drivers[index]
		if driver.Group != previousGroup {
			lines = append(lines, helpStyle.Render(newProjectResourceDriverGroupLabel(driver.Group)))
			previousGroup = driver.Group
		}
		focus := index == ui.driverIndex
		marker := "  "
		if focus {
			marker = "> "
		}
		if supported {
			checked := newProjectResourceContainsDriver(selection.Supported, driver.Name)
			box := "[ ] "
			if checked {
				box = "[x] "
			}
			marker += box
		}
		label := driver.Label
		if supported && selection.Active == driver.Name {
			label += " (active)"
		}
		nameStyle := listNameStyle
		if focus {
			nameStyle = listFocusedNameStyle
		}
		lines = append(lines, marker+nameStyle.Render(label))
		lines = append(lines, newProjectWrappedStyledLines(driver.Description, width-4, listDescStyle)...)
	}
	if len(drivers) > 7 {
		position := fmt.Sprintf("%d-%d of %d", ui.driverViewportOffset+1, end, len(drivers))
		lines = append(lines, helpStyle.Render(position))
	}
	return newProjectResourceJoinLines(width, lines)
}

// renderPlacementBody presents only placements valid for the selected capabilities.
func (ui newProjectResourceUI) renderPlacementBody(width int) string {
	lines := []string{sectionLabelStyle.Render(newProjectResourceServiceLabel(ui.placementService) + " placement"), ""}
	for index, mode := range ui.placementModes() {
		marker := "  "
		style := listNameStyle
		if index == ui.placementIndex {
			marker = "> "
			style = listFocusedNameStyle
		}
		label := "External"
		description := "Use an endpoint managed outside this project."
		if mode == project.LocalServiceModeLocal {
			label = "Local Compose"
			description = "Start one generated service for every active consumer."
		}
		lines = append(lines, marker+style.Render(label))
		lines = append(lines, newProjectWrappedStyledLines(description, width-2, helpStyle)...)
	}
	return newProjectResourceJoinLines(width, lines)
}

// confirmationRows derives resource topology rows from the same plan and service planner used by rendering.
func (ui newProjectResourceUI) confirmationRows() ([]keyValue, error) {
	if err := ui.plan.Validate(ui.components); err != nil {
		return nil, err
	}
	classification := ui.classification()
	rows := []keyValue{{key: "Resource shape", value: classification.Label}}
	if classification.Custom || classification.CustomSupport || classification.Customized || ui.databaseBuildContractCustomized() {
		rows = append(rows, ui.expandedResourceRows()...)
	} else {
		if ui.databaseEnabled {
			rows = append(rows, keyValue{key: "Database", value: newProjectResourceDriverLabel(project.ResourceDatabase, ui.databaseChoice)})
		}
		rows = append(rows, keyValue{key: "Active resources", value: ui.activeResourceSummary()})
		if bridge := ui.builtInBridgeSummary(); bridge != "" {
			rows = append(rows, keyValue{key: "Built-in bridge", value: bridge})
		}
	}
	services, err := ui.serviceSummary()
	if err != nil {
		return nil, err
	}
	if len(services.local) > 0 {
		rows = append(rows, keyValue{key: "App services", value: strings.Join(services.local, " · ")})
	}
	if len(services.external) > 0 {
		rows = append(rows, keyValue{key: "External services required", value: strings.Join(services.external, " · ")})
	}
	if len(services.warnings) > 0 {
		rows = append(rows, keyValue{key: "Resource notice", value: strings.Join(services.warnings, " ")})
	}
	return rows, nil
}

// databaseBuildContractCustomized reports support edits without letting an independent database choice rename the resource shape.
func (ui newProjectResourceUI) databaseBuildContractCustomized() bool {
	selection, ok := ui.plan.Selection(project.ResourceDatabase)
	if !ok {
		return false
	}
	expected := []string{selection.Active}
	if ui.databaseLocked {
		expected = []string{"sqlite", "mysql"}
	}
	return !newProjectTargetDriverListsEqual(selection.Supported, expected)
}

// serviceSummary converts deduplicated service requirements into confirmation-friendly labels.
func (ui newProjectResourceUI) serviceSummary() (newProjectResourceServiceSummary, error) {
	servicePlan, err := project.ResolveServicePlanWithConsumers(ui.plan, ui.components, ui.localServiceIntent, ui.effectiveConsumers)
	if err != nil {
		return newProjectResourceServiceSummary{}, err
	}
	summary := newProjectResourceServiceSummary{}
	serviceCounts := map[project.ServiceKey]int{}
	for _, requirement := range servicePlan.Requirements {
		serviceCounts[requirement.Key]++
	}
	for _, requirement := range servicePlan.Requirements {
		label := strings.TrimSpace(requirement.Label)
		if label == "" {
			label = newProjectResourceServiceLabel(requirement.Key)
		}
		if serviceCounts[requirement.Key] > 1 && requirement.EndpointAffinity != "" && len(requirement.ActiveConsumers) > 0 {
			label += " (" + strings.Join(requirement.ActiveConsumers, ", ") + ")"
		}
		switch requirement.State {
		case project.ServiceStateActiveLocal:
			summary.local = append(summary.local, label)
		case project.ServiceStateExternalRequired:
			summary.external = append(summary.external, label)
		case project.ServiceStateAvailableLocal:
			summary.available = append(summary.available, label)
		case project.ServiceStateLocalRequestedUnused:
			summary.warnings = append(summary.warnings, label+" is configured to start locally without an active consumer.")
		}
	}
	if resourcePlanUsesDriver(ui.plan, ui.components, "redis", true) && ui.databaseChoice == "sqlite" {
		summary.warnings = append(summary.warnings, "Redis resources can cross App processes; SQLite and local storage remain filesystem-local.")
	}
	return summary, nil
}

// expandedResourceRows exposes every Advanced active and built-in selection on confirmation.
func (ui newProjectResourceUI) expandedResourceRows() []keyValue {
	rows := []keyValue{}
	for _, definition := range ui.applicableDefinitions() {
		selection, ok := ui.plan.Selection(definition.Key)
		if !ok {
			continue
		}
		active := newProjectResourceDriverLabel(definition.Key, selection.Active)
		supported := newProjectResourceSupportedLabels(definition, selection.Supported)
		rows = append(rows, keyValue{
			key:   definition.Label,
			value: active + " · built in: " + supported,
		})
	}
	return rows
}

// activeResourceSummary lists starting drivers in catalog order while leaving Database in its own row.
func (ui newProjectResourceUI) activeResourceSummary() string {
	parts := []string{}
	for _, definition := range ui.applicableDefinitions() {
		if definition.Key == project.ResourceDatabase || definition.Key == project.ResourceMail {
			continue
		}
		selection, ok := ui.plan.Selection(definition.Key)
		if !ok {
			continue
		}
		parts = append(parts, newProjectResourceDriverLabel(definition.Key, selection.Active)+" "+newProjectResourceConsumerLabel(definition.Key))
	}
	return strings.Join(parts, " · ")
}

// builtInBridgeSummary describes inactive portable drivers without suggesting that their services start now.
func (ui newProjectResourceUI) builtInBridgeSummary() string {
	consumers := []string{}
	for _, key := range newProjectShapeManagedResourceKeys() {
		selection, ok := ui.plan.Selection(key)
		if !ok || selection.Active == "redis" || !newProjectResourceContainsDriver(selection.Supported, "redis") {
			continue
		}
		consumers = append(consumers, newProjectResourceConsumerLabel(key))
	}
	if len(consumers) == 0 {
		return ""
	}
	return "Redis for " + newProjectResourceNaturalJoin(consumers)
}

// applicableDefinitions returns defensive catalog copies for resources enabled by current capabilities.
func (ui newProjectResourceUI) applicableDefinitions() []project.ResourceDefinition {
	definitions := []project.ResourceDefinition{}
	for _, definition := range project.ResourceCatalog() {
		if definition.AppliesTo(ui.components) {
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

// focusedDefinition returns the applicable resource selected on the Advanced screen.
func (ui newProjectResourceUI) focusedDefinition() (project.ResourceDefinition, bool) {
	definitions := ui.applicableDefinitions()
	if len(definitions) == 0 {
		return project.ResourceDefinition{}, false
	}
	index := newProjectWrappedIndex(ui.advancedIndex, len(definitions))
	return definitions[index], true
}

// focusedPlacementSelectable reports whether the focused Advanced row owns an active placement choice.
func (ui newProjectResourceUI) focusedPlacementSelectable() bool {
	definition, ok := ui.focusedDefinition()
	if !ok {
		return false
	}
	selection, ok := ui.plan.Selection(definition.Key)
	if !ok {
		return false
	}
	driver, ok := definition.Driver(selection.Active)
	return ok && driver.PlacementSelectable
}

// editingDrivers groups the active resource's catalog by operational consequence.
func (ui newProjectResourceUI) editingDrivers() []project.DriverDefinition {
	definition, ok := project.ResourceDefinitionByKey(ui.editingResource)
	if !ok {
		return nil
	}
	drivers := append([]project.DriverDefinition(nil), definition.Drivers...)
	sort.SliceStable(drivers, func(left int, right int) bool {
		leftGroup := newProjectResourceDriverGroupOrder(drivers[left].Group)
		rightGroup := newProjectResourceDriverGroupOrder(drivers[right].Group)
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		return drivers[left].Order < drivers[right].Order
	})
	return drivers
}

// driverIndexForSelection focuses the active driver or the first built-in driver for a resource.
func (ui newProjectResourceUI) driverIndexForSelection(definition project.ResourceDefinition, active bool) int {
	selection, _ := ui.plan.Selection(definition.Key)
	want := selection.Active
	if !active && len(selection.Supported) > 0 {
		want = selection.Supported[0]
	}
	drivers := ui.editingDriversForDefinition(definition)
	for index, driver := range drivers {
		if driver.Name == want {
			return index
		}
	}
	return 0
}

// editingDriversForDefinition temporarily selects a definition so grouping stays consistent across open actions.
func (ui newProjectResourceUI) editingDriversForDefinition(definition project.ResourceDefinition) []project.DriverDefinition {
	ui.editingResource = definition.Key
	return ui.editingDrivers()
}

// ensureDriverVisible keeps the focused catalog row inside the bounded Advanced viewport.
func (ui *newProjectResourceUI) ensureDriverVisible(height int) {
	if ui.driverIndex < ui.driverViewportOffset {
		ui.driverViewportOffset = ui.driverIndex
	}
	if ui.driverIndex >= ui.driverViewportOffset+height {
		ui.driverViewportOffset = ui.driverIndex - height + 1
	}
}

// placementModes excludes Local Compose when the project has no Docker capability.
func (ui newProjectResourceUI) placementModes() []project.LocalServiceMode {
	if ui.components.Docker {
		return []project.LocalServiceMode{project.LocalServiceModeLocal, project.LocalServiceModeExternal}
	}
	return []project.LocalServiceMode{project.LocalServiceModeExternal}
}

// activeServiceIsPlacementSelectable verifies placement belongs to a currently active driver.
func (ui newProjectResourceUI) activeServiceIsPlacementSelectable(service project.ServiceKey) bool {
	for _, definition := range ui.applicableDefinitions() {
		selection, ok := ui.plan.Selection(definition.Key)
		if !ok {
			continue
		}
		driver, ok := definition.Driver(selection.Active)
		if ok && driver.PlacementSelectable && driver.Service == service {
			return true
		}
	}
	return false
}

// summaryFocusOrder hides Database navigation when the capability is disabled.
func (ui newProjectResourceUI) summaryFocusOrder() []newProjectResourceSummaryFocus {
	focuses := []newProjectResourceSummaryFocus{newProjectResourceFocusShape}
	if ui.databaseEnabled {
		focuses = append(focuses, newProjectResourceFocusDatabase)
	}
	return append(focuses, newProjectResourceFocusContinue, newProjectResourceFocusAdvanced)
}

// renderSummaryChoice renders an aligned editable value row without exceeding narrow terminals.
func (ui newProjectResourceUI) renderSummaryChoice(focus newProjectResourceSummaryFocus, label string, value string, width int) string {
	marker := "  "
	style := listNameStyle
	if ui.summaryFocus == focus {
		marker = "> "
		style = listFocusedNameStyle
	}
	row := fmt.Sprintf("%-20s %s", label, value)
	return lipgloss.NewStyle().MaxWidth(width).Render(marker + style.Render(row))
}

// renderSummaryAction renders one normal-screen action with the same focus language as editable rows.
func (ui newProjectResourceUI) renderSummaryAction(focus newProjectResourceSummaryFocus, label string, width int) string {
	marker := "  "
	style := listNameStyle
	if ui.summaryFocus == focus {
		marker = "> "
		style = listFocusedNameStyle
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(marker + style.Render(label))
}

// seedNormalServiceIntent applies the preset's local Redis default without making support-only Redis start.
func (ui *newProjectResourceUI) seedNormalServiceIntent() {
	ui.localServiceIntent = cloneNewProjectLocalServiceIntent(ui.localServiceIntent)
	delete(ui.localServiceIntent.Modes, project.ServiceRedis)
	ui.redisPlacementExplicit = false
	if ui.baseShape != project.ResourceShapeSharedRedis {
		return
	}
	mode := project.LocalServiceModeExternal
	if ui.components.Docker {
		mode = project.LocalServiceModeLocal
	}
	ui.localServiceIntent = ui.localServiceIntent.WithMode(project.ServiceRedis, mode)
}

// syncImplicitRedisIntent follows current active consumers and Docker capability until the user chooses placement explicitly.
func (ui *newProjectResourceUI) syncImplicitRedisIntent() {
	if ui.redisPlacementExplicit {
		return
	}
	ui.localServiceIntent = cloneNewProjectLocalServiceIntent(ui.localServiceIntent)
	if !resourcePlanUsesDriver(ui.plan, ui.components, "redis", true) {
		delete(ui.localServiceIntent.Modes, project.ServiceRedis)
		return
	}
	mode := project.LocalServiceModeExternal
	if ui.components.Docker {
		mode = project.LocalServiceModeLocal
	}
	ui.localServiceIntent = ui.localServiceIntent.WithMode(project.ServiceRedis, mode)
}

// newProjectResourceShapes returns the stable normal-picker order.
func newProjectResourceShapes() []project.StartingResourceShape {
	return []project.StartingResourceShape{project.ResourceShapeStandalone, project.ResourceShapeSharedRedis}
}

// newProjectDatabaseDrivers returns the stable independent database picker order.
func newProjectDatabaseDrivers() []string {
	return []string{"mysql", "postgres", "sqlite"}
}

// newProjectShapeManagedResourceKeys returns the roots deliberately reset by a normal shape choice.
func newProjectShapeManagedResourceKeys() []project.ResourceKey {
	return []project.ResourceKey{project.ResourceCache, project.ResourceQueue, project.ResourceEvents}
}

// newProjectResourceIsShapeManaged reports whether the normal shape owns a root resource.
func newProjectResourceIsShapeManaged(key project.ResourceKey) bool {
	for _, candidate := range newProjectShapeManagedResourceKeys() {
		if key == candidate {
			return true
		}
	}
	return false
}

// newProjectShapeIndex resolves a shape into the stable picker order.
func newProjectShapeIndex(shape project.StartingResourceShape) int {
	for index, candidate := range newProjectResourceShapes() {
		if candidate == shape {
			return index
		}
	}
	return 0
}

// newProjectDatabaseIndex resolves a driver into the stable database picker order.
func newProjectDatabaseIndex(driver string) int {
	for index, candidate := range newProjectDatabaseDrivers() {
		if candidate == driver {
			return index
		}
	}
	return 0
}

// newProjectWrappedIndex provides list-style wrapping while tolerating temporarily empty capability lists.
func newProjectWrappedIndex(index int, length int) int {
	if length <= 0 {
		return 0
	}
	index %= length
	if index < 0 {
		index += length
	}
	return index
}

// newProjectResourceContainsDriver reports driver membership without exposing a mutable supported slice.
func newProjectResourceContainsDriver(drivers []string, want string) bool {
	for _, driver := range drivers {
		if driver == want {
			return true
		}
	}
	return false
}

// newProjectResourceDriverGroupOrder keeps the Advanced inventory operationally grouped.
func newProjectResourceDriverGroupOrder(group project.DriverGroup) int {
	switch group {
	case project.DriverGroupLocal:
		return 10
	case project.DriverGroupSQL:
		return 20
	case project.DriverGroupShared:
		return 30
	case project.DriverGroupCloud:
		return 40
	case project.DriverGroupDevelopment:
		return 50
	default:
		return 100
	}
}

// newProjectResourceDriverGroupLabel names operational groups without exposing catalog internals.
func newProjectResourceDriverGroupLabel(group project.DriverGroup) string {
	switch group {
	case project.DriverGroupLocal:
		return "App-local and filesystem"
	case project.DriverGroupSQL:
		return "SQL-backed"
	case project.DriverGroupShared:
		return "Shared infrastructure"
	case project.DriverGroupCloud:
		return "Managed cloud"
	case project.DriverGroupDevelopment:
		return "Development and testing"
	default:
		return "Other"
	}
}

// newProjectResourceDriverLabel resolves catalog labels while retaining a readable fallback for future plugins.
func newProjectResourceDriverLabel(key project.ResourceKey, driver string) string {
	definition, ok := project.ResourceDefinitionByKey(key)
	if ok {
		if driverDefinition, exists := definition.Driver(driver); exists {
			return driverDefinition.Label
		}
	}
	if driver == "" {
		return "<pending>"
	}
	return strings.ToUpper(driver[:1]) + driver[1:]
}

// newProjectResourceSupportedLabels renders defensive supported-driver copies in catalog order.
func newProjectResourceSupportedLabels(definition project.ResourceDefinition, supported []string) string {
	labels := make([]string, 0, len(supported))
	for _, driver := range supported {
		if driverDefinition, ok := definition.Driver(driver); ok {
			labels = append(labels, driverDefinition.Label)
		} else {
			labels = append(labels, driver)
		}
	}
	return strings.Join(labels, ", ")
}

// newProjectResourceConsumerLabel returns compact plural nouns used by confirmation summaries.
func newProjectResourceConsumerLabel(key project.ResourceKey) string {
	switch key {
	case project.ResourceCache:
		return "cache"
	case project.ResourceQueue:
		return "jobs"
	case project.ResourceEvents:
		return "events"
	case project.ResourceStorage:
		return "storage"
	case project.ResourceMail:
		return "mail"
	default:
		return string(key)
	}
}

// newProjectResourceServiceLabel maps infrastructure identities to their conventional display names.
func newProjectResourceServiceLabel(key project.ServiceKey) string {
	switch key {
	case project.ServiceRedis:
		return "Redis"
	case project.ServiceMySQL:
		return "MySQL"
	case project.ServicePostgres:
		return "Postgres"
	default:
		return string(key)
	}
}

// newProjectResourceNaturalJoin joins bridge consumers with readable punctuation at every list size.
func newProjectResourceNaturalJoin(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " and " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + ", and " + values[len(values)-1]
	}
}

// newProjectResourceContentWidth keeps direct unit renders and nested panel renders inside the wizard boundary.
func newProjectResourceContentWidth(width int) int {
	if width <= 0 {
		width = wizardWidth
	}
	if width > wizardWidth {
		width = wizardWidth
	}
	width -= 4
	if width < 12 {
		return 12
	}
	return width
}

// newProjectWrappedStyledLines wraps plain copy before styling so ANSI escape sequences cannot distort width math.
func newProjectWrappedStyledLines(value string, width int, style lipgloss.Style) []string {
	if width < 12 {
		width = 12
	}
	wrapped := wrapText(value, width)
	lines := make([]string, 0, len(wrapped))
	for _, line := range wrapped {
		lines = append(lines, style.Render(line))
	}
	return lines
}

// newProjectResourceJoinLines clamps every rendered row before joining the resource panel body.
func newProjectResourceJoinLines(width int, lines []string) string {
	clamped := make([]string, 0, len(lines))
	for _, line := range lines {
		clamped = append(clamped, lipgloss.NewStyle().MaxWidth(width).Render(line))
	}
	return strings.Join(clamped, "\n")
}

// cloneNewProjectLocalServiceIntent prevents placement edits from aliasing earlier wizard states.
func cloneNewProjectLocalServiceIntent(intent project.LocalServiceIntent) project.LocalServiceIntent {
	modes := make(map[project.ServiceKey]project.LocalServiceMode, len(intent.Modes))
	for key, mode := range intent.Modes {
		modes[key] = mode
	}
	return project.LocalServiceIntent{Modes: modes}
}

// presetDatabaseDriver reads the preset database without discarding the user's fallback when Database is disabled.
func presetDatabaseDriver(plan project.ResourcePlan, fallback string) string {
	if selection, ok := plan.Selection(project.ResourceDatabase); ok {
		return selection.Active
	}
	return fallback
}

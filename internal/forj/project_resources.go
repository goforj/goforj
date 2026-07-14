package forj

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/goforj/goforj/project"
)

// normalizeQueueDriver validates legacy queue values against the generator's canonical inventory.
func normalizeQueueDriver(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	definition, ok := project.ResourceDefinitionByKey(project.ResourceQueue)
	if !ok {
		return ""
	}
	if _, ok := definition.Driver(normalized); !ok {
		return ""
	}
	return normalized
}

// resourceRenderValues is the template-facing projection of a validated transient resource plan.
type resourceRenderValues struct {
	DatabaseDriver           string
	DatabaseSupportedDrivers string
	DatabaseMySQL            bool
	DatabasePostgres         bool
	DatabaseSQLite           bool
	DatabaseExternal         bool
	CacheDriver              string
	CacheSupportedDrivers    string
	QueueDriver              string
	QueueSupportedDrivers    string
	EventsDriver             string
	EventsSupportedDrivers   string
	StorageDriver            string
	StorageSupportedDrivers  string
	StoragePublicDriver      string
	StorageFaviconsDriver    string
	MailDriver               string
	MailSupportedDrivers     string
	CacheInspectsDriver      string
	CacheLighthouseDriver    string
	CacheSettingsDriver      string
	CacheSessionsDriver      string
	RedisActive              bool
	RedisSupported           bool
	RedisLocal               bool
	RedisExternal            bool
}

var resourceEnvironmentKeys = map[project.ResourceKey]struct {
	active    string
	supported string
}{
	project.ResourceDatabase: {active: "DB_DRIVER", supported: "DB_SUPPORTED_DRIVERS"},
	project.ResourceCache:    {active: "CACHE_DRIVER", supported: "CACHE_SUPPORTED_DRIVERS"},
	project.ResourceQueue:    {active: "QUEUE_DRIVER", supported: "QUEUE_SUPPORTED_DRIVERS"},
	project.ResourceEvents:   {active: "EVENTS_DRIVER", supported: "EVENTS_SUPPORTED_DRIVERS"},
	project.ResourceStorage:  {active: "STORAGE_DRIVER", supported: "STORAGE_SUPPORTED_DRIVERS"},
	project.ResourceMail:     {active: "MAIL_DRIVER", supported: "MAIL_SUPPORTED_DRIVERS"},
}

// prepareResourceEnvironment reconciles an existing owner file without publishing changes before validation completes.
func (p *ProjectRenderer) prepareResourceEnvironment() error {
	const ownerPath = ".env"
	const examplePath = ".env.example"
	path := ownerPath
	ownerExists := true
	source, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		path = examplePath
		ownerExists = false
		source, err = os.ReadFile(path)
		if os.IsNotExist(err) {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("read resource environment: %w", err)
	}
	if !ownerExists && p.explicitResourcePlan {
		// A committed fallback reproduces an existing build, but it cannot replace a new wizard decision.
		return nil
	}
	_, profilesSet := envAssignment(strings.Split(string(source), "\n"), "COMPOSE_PROFILES")
	legacyLocalRedis := !profilesSet && composeRedisServiceWithoutProfile("docker-compose.yml")
	if legacyLocalRedis {
		p.localServiceIntent = p.localServiceIntent.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
		if ownerExists {
			var profileChanged bool
			source, profileChanged = seedExactComposeProfile(source, "redis")
			p.pendingEnvironmentWrite = profileChanged
		}
	} else {
		p.localServiceIntent = localServiceIntentFromEnvironment(source, p.localServiceIntent)
	}
	updated, effective, changed, err := reconcileResourceEnvironment(
		source,
		p.resourcePlan,
		p.config.Render.Components,
		p.explicitResourcePlan,
	)
	if err != nil {
		return err
	}
	p.resourcePlan = effective
	consumers, err := effectiveResourceConsumersFromEnvironment(updated, effective, p.config.Render.Components, configuredResourceAppNames(p.config))
	if err != nil {
		return fmt.Errorf("discover effective resource consumers: %w", err)
	}
	p.serviceConsumers = cloneEffectiveResourceConsumers(consumers)
	if queue, ok := effective.Selection(project.ResourceQueue); ok {
		p.queueDriver = queue.Active
	}
	if ownerExists {
		p.pendingEnvironment = updated
		p.pendingEnvironmentWrite = p.pendingEnvironmentWrite || changed
	}
	return nil
}

// compatibilityResourcePlan preserves pre-wizard render defaults when no explicit transient plan is supplied.
func compatibilityResourcePlan(components project.Components, legacyQueueDriver string) (project.ResourcePlan, error) {
	plan, err := project.ResolveResourcePlan(project.ResourceShapeStandalone, components)
	if err != nil {
		return project.ResourcePlan{}, err
	}
	for _, key := range []project.ResourceKey{
		project.ResourceDatabase,
		project.ResourceCache,
		project.ResourceEvents,
		project.ResourceStorage,
		project.ResourceMail,
	} {
		selection, ok := plan.Selection(key)
		if !ok {
			continue
		}
		if components.DemoApp && key == project.ResourceDatabase {
			continue
		}
		selection.Supported = []string{selection.Active}
		plan = plan.WithSelection(key, selection)
	}
	if components.Jobs {
		queue, _ := plan.Selection(project.ResourceQueue)
		queue.Active = resolveQueueDriverSeed("", legacyQueueDriver)
		queue.Supported = []string{queue.Active}
		plan = plan.WithSelection(project.ResourceQueue, queue)
	}
	return plan.Normalized(components)
}

// normalizeExplicitResourcePlan validates a wizard or command plan before it can initialize files.
func normalizeExplicitResourcePlan(plan project.ResourcePlan, components project.Components) (project.ResourcePlan, error) {
	normalized, err := plan.Normalized(components)
	if err != nil {
		return project.ResourcePlan{}, fmt.Errorf("resource plan: %w", err)
	}
	return normalized, nil
}

// reconcileResourceEnvironment applies owner precedence and fills only missing initialization keys.
func reconcileResourceEnvironment(source []byte, seed project.ResourcePlan, components project.Components, portableDefaults bool) ([]byte, project.ResourcePlan, bool, error) {
	lines := strings.Split(string(source), "\n")
	effective := seed.Clone()
	changed := false

	for _, definition := range project.ResourceCatalog() {
		if !definition.AppliesTo(components) {
			effective = effective.WithoutSelection(definition.Key)
			continue
		}
		seedSelection, ok := seed.Selection(definition.Key)
		if !ok {
			return nil, project.ResourcePlan{}, false, fmt.Errorf("resource %s has no initialization selection", definition.Label)
		}
		keys := resourceEnvironmentKeys[definition.Key]
		active, activeSet := envAssignment(lines, keys.active)
		active = project.CanonicalResourceDriver(definition.Key, active)
		if !activeSet || active == "" {
			active = seedSelection.Active
			lines = setFinalEnvAssignment(lines, keys.active, active)
			changed = true
		}

		supportedValue, supportedSet := envAssignment(lines, keys.supported)
		supported := splitDriverList(supportedValue)
		for index := range supported {
			supported[index] = project.CanonicalResourceDriver(definition.Key, supported[index])
		}
		if supportedSet && len(supported) > 0 && !stringSliceContainsFold(supported, active) {
			return nil, project.ResourcePlan{}, false, fmt.Errorf("%s in .env excludes active %s %q; add %q before rerendering", keys.supported, keys.active, active, active)
		}
		if !supportedSet || len(supported) == 0 {
			if portableDefaults {
				supported = append([]string(nil), seedSelection.Supported...)
			} else {
				supported = []string{active}
			}
			if !stringSliceContainsFold(supported, active) {
				supported = append(supported, active)
			}
			lines = setFinalEnvAssignment(lines, keys.supported, strings.Join(supported, ","))
			changed = true
		}
		effective = effective.WithSelection(definition.Key, project.DriverSelection{Active: active, Supported: supported})
	}

	for _, named := range seed.GeneratedNamedSelections(components) {
		active, activeSet := envAssignment(lines, named.EnvironmentKey)
		active = project.CanonicalResourceDriver(named.Resource, active)
		if !activeSet || active == "" {
			active = named.Active
			lines = setFinalEnvAssignment(lines, named.EnvironmentKey, active)
			changed = true
		}
		effective = effective.WithNamedSelection(named.EnvironmentKey, active)
	}

	normalized, err := effective.Normalized(components)
	if err != nil {
		return nil, project.ResourcePlan{}, false, fmt.Errorf("environment resource contract: %w", err)
	}
	if !changed {
		return source, normalized, false, nil
	}
	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	return []byte(updated), normalized, true, nil
}

// localServiceIntentFromEnvironment reconstructs concrete owner intent from exact Compose profile tokens.
func localServiceIntentFromEnvironment(source []byte, fallback project.LocalServiceIntent) project.LocalServiceIntent {
	lines := strings.Split(string(source), "\n")
	profiles, profilesSet := envAssignment(lines, "COMPOSE_PROFILES")
	if !profilesSet {
		return fallback.WithMode(project.ServiceRedis, project.LocalServiceModeExternal)
	}
	mode := project.LocalServiceModeExternal
	if exactCSVToken(profiles, string(project.ServiceRedis)) {
		mode = project.LocalServiceModeLocal
	}
	return fallback.WithMode(project.ServiceRedis, mode)
}

// seedComposeProfiles appends Redis whenever explicit owner intent asks Compose to retain that local service.
func seedComposeProfiles(source []byte, plan project.ResourcePlan, components project.Components, intent project.LocalServiceIntent) ([]byte, bool) {
	if !components.Docker || !resourcePlanUsesDriver(plan, components, "redis", false) {
		return source, false
	}
	mode, ok := intent.Mode(project.ServiceRedis)
	if !ok || mode != project.LocalServiceModeLocal {
		return source, false
	}
	return seedExactComposeProfile(source, "redis")
}

// seedExactComposeProfile appends one profile token without replacing unrelated owner intent.
func seedExactComposeProfile(source []byte, profile string) ([]byte, bool) {
	lines := strings.Split(string(source), "\n")
	profiles, profilesSet := envAssignment(lines, "COMPOSE_PROFILES")
	if profilesSet && exactCSVToken(profiles, profile) {
		return source, false
	}
	profiles = appendCSVToken(profiles, profile)
	lines = setFinalEnvAssignment(lines, "COMPOSE_PROFILES", profiles)
	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	return []byte(updated), true
}

// composeRedisServiceWithoutProfile detects the pre-profile generated Redis service during one-way migration.
func composeRedisServiceWithoutProfile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	lines := strings.Split(string(data), "\n")
	inServices := false
	inRedis := false
	for _, line := range lines {
		if line == "services:" {
			inServices = true
			continue
		}
		if !inServices {
			continue
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(strings.TrimSpace(line), ":") {
			if inRedis {
				return true
			}
			inRedis = strings.TrimSpace(line) == "redis:"
			continue
		}
		if inRedis && strings.HasPrefix(strings.TrimSpace(line), "profiles:") {
			return false
		}
	}
	return inRedis
}

// resourceRenderValuesForPlan produces stable strings and service flags for environment and Compose templates.
func resourceRenderValuesForPlan(plan project.ResourcePlan, components project.Components, intent project.LocalServiceIntent) resourceRenderValues {
	return resourceRenderValuesForPlanWithConsumers(plan, components, intent, nil)
}

// resourceRenderValuesForPlanWithConsumers includes environment-resolved named and App-scoped service activity.
func resourceRenderValuesForPlanWithConsumers(plan project.ResourcePlan, components project.Components, intent project.LocalServiceIntent, consumers []project.EffectiveResourceConsumer) resourceRenderValues {
	values := resourceRenderValues{}
	set := func(key project.ResourceKey, active *string, supported *string) {
		selection, ok := plan.Selection(key)
		if !ok {
			return
		}
		*active = selection.Active
		*supported = strings.Join(selection.Supported, ",")
	}
	set(project.ResourceDatabase, &values.DatabaseDriver, &values.DatabaseSupportedDrivers)
	set(project.ResourceCache, &values.CacheDriver, &values.CacheSupportedDrivers)
	set(project.ResourceQueue, &values.QueueDriver, &values.QueueSupportedDrivers)
	set(project.ResourceEvents, &values.EventsDriver, &values.EventsSupportedDrivers)
	set(project.ResourceStorage, &values.StorageDriver, &values.StorageSupportedDrivers)
	set(project.ResourceMail, &values.MailDriver, &values.MailSupportedDrivers)
	values.DatabaseMySQL = values.DatabaseDriver == "mysql"
	values.DatabasePostgres = values.DatabaseDriver == "postgres"
	values.DatabaseSQLite = values.DatabaseDriver == "sqlite"
	values.DatabaseExternal = (values.DatabaseMySQL || values.DatabasePostgres) && !components.Docker
	for _, named := range plan.GeneratedNamedSelections(components) {
		switch named.EnvironmentKey {
		case "CACHE_INSPECTS_DRIVER":
			values.CacheInspectsDriver = named.Active
		case "CACHE_LIGHTHOUSE_DRIVER":
			values.CacheLighthouseDriver = named.Active
		case "CACHE_SETTINGS_DRIVER":
			values.CacheSettingsDriver = named.Active
		case "CACHE_SESSIONS_DRIVER":
			values.CacheSessionsDriver = named.Active
		case "STORAGE_PUBLIC_DRIVER":
			values.StoragePublicDriver = named.Active
		case "STORAGE_FAVICONS_DRIVER":
			values.StorageFaviconsDriver = named.Active
		}
	}
	values.RedisActive = resourcePlanUsesDriver(plan, components, "redis", true)
	values.RedisSupported = components.Docker && resourcePlanUsesDriver(plan, components, "redis", false)
	mode, _ := intent.Mode(project.ServiceRedis)
	values.RedisLocal = components.Docker && mode == project.LocalServiceModeLocal
	values.RedisExternal = values.RedisActive && !values.RedisLocal
	if len(consumers) > 0 {
		servicePlan, err := project.ResolveServicePlanWithConsumers(plan, components, intent, consumers)
		if err == nil {
			values.RedisActive = false
			values.RedisLocal = false
			values.RedisExternal = false
			for _, requirement := range servicePlan.RequirementsFor(project.ServiceRedis) {
				switch requirement.State {
				case project.ServiceStateActiveLocal, project.ServiceStateLocalRequestedUnused:
					values.RedisLocal = true
				case project.ServiceStateExternalRequired:
					if requirement.EndpointAffinity == "" {
						values.RedisExternal = true
					}
				}
				if len(requirement.ActiveConsumers) > 0 {
					values.RedisActive = true
				}
			}
		}
	}
	return values
}

// DriverEnvironmentPlaceholders renders only opt-in driver hints selected by the effective build contract.
func (v resourceRenderValues) DriverEnvironmentPlaceholders() string {
	selections := map[project.ResourceKey]project.DriverSelection{
		project.ResourceDatabase: {Active: v.DatabaseDriver, Supported: splitDriverList(v.DatabaseSupportedDrivers)},
		project.ResourceCache:    {Active: v.CacheDriver, Supported: splitDriverList(v.CacheSupportedDrivers)},
		project.ResourceQueue:    {Active: v.QueueDriver, Supported: splitDriverList(v.QueueSupportedDrivers)},
		project.ResourceEvents:   {Active: v.EventsDriver, Supported: splitDriverList(v.EventsSupportedDrivers)},
		project.ResourceStorage:  {Active: v.StorageDriver, Supported: splitDriverList(v.StorageSupportedDrivers)},
		project.ResourceMail:     {Active: v.MailDriver, Supported: splitDriverList(v.MailSupportedDrivers)},
	}
	return renderDriverEnvironmentPlaceholders(selections)
}

// renderDriverEnvironmentPlaceholders formats catalog hints in deterministic resource and driver order.
func renderDriverEnvironmentPlaceholders(selections map[project.ResourceKey]project.DriverSelection) string {
	lines := []string{}
	seenKeys := map[string]bool{}
	for _, definition := range project.ResourceCatalog() {
		selection, ok := selections[definition.Key]
		if !ok {
			continue
		}
		for _, driver := range orderedSelectedDriverDefinitions(definition, selection) {
			placeholders := uniqueDriverEnvironmentPlaceholders(driver.Environment, seenKeys)
			if len(placeholders) == 0 {
				continue
			}
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "# "+definition.Label+" · "+driver.Label)
			for _, placeholder := range placeholders {
				if description := strings.TrimSpace(placeholder.Description); description != "" {
					lines = append(lines, "# "+description)
				}
				lines = append(lines, "# "+placeholder.Key+"="+placeholder.Example)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// orderedSelectedDriverDefinitions lets the active driver's examples own shared keys before catalog-ordered transition drivers.
func orderedSelectedDriverDefinitions(definition project.ResourceDefinition, selection project.DriverSelection) []project.DriverDefinition {
	activeName := strings.ToLower(strings.TrimSpace(selection.Active))
	selected := make(map[string]bool, len(selection.Supported))
	for _, name := range selection.Supported {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" && name != activeName {
			selected[name] = true
		}
	}

	drivers := make([]project.DriverDefinition, 0, len(selected)+1)
	if active, ok := definition.Driver(activeName); ok {
		drivers = append(drivers, active)
	}
	supported := make([]project.DriverDefinition, 0, len(selected))
	for _, driver := range definition.Drivers {
		if selected[driver.Name] {
			supported = append(supported, driver)
		}
	}
	sort.SliceStable(supported, func(left, right int) bool {
		return supported[left].Order < supported[right].Order
	})
	return append(drivers, supported...)
}

// uniqueDriverEnvironmentPlaceholders removes shared scope keys without conflating service identities.
func uniqueDriverEnvironmentPlaceholders(placeholders []project.DriverEnvironmentPlaceholder, seen map[string]bool) []project.DriverEnvironmentPlaceholder {
	unique := make([]project.DriverEnvironmentPlaceholder, 0, len(placeholders))
	for _, placeholder := range placeholders {
		key := strings.TrimSpace(placeholder.Key)
		if key == "" || seen[key] {
			continue
		}
		placeholder.Key = key
		seen[key] = true
		unique = append(unique, placeholder)
	}
	return unique
}

// applyDatabaseRenderCapabilities promotes every built-in database without changing the environment-owned active driver.
func applyDatabaseRenderCapabilities(components *project.Components, plan project.ResourcePlan) {
	selection, ok := plan.Selection(project.ResourceDatabase)
	if !ok {
		return
	}
	for _, driver := range selection.Supported {
		switch driver {
		case "mysql":
			components.DatabaseMySQL = true
		case "postgres":
			components.DatabasePostgres = true
		case "sqlite":
			components.DatabaseSQLite = true
		}
	}
}

// resourcePlanUsesDriver finds active or built-in use across root and generated named resources.
func resourcePlanUsesDriver(plan project.ResourcePlan, components project.Components, driver string, activeOnly bool) bool {
	for _, definition := range project.ResourceCatalog() {
		if !definition.AppliesTo(components) {
			continue
		}
		selection, ok := plan.Selection(definition.Key)
		if !ok {
			continue
		}
		if selection.Active == driver {
			return true
		}
		if !activeOnly && stringSliceContainsFold(selection.Supported, driver) {
			return true
		}
	}
	for _, named := range plan.GeneratedNamedSelections(components) {
		if named.Active == driver {
			return true
		}
	}
	return false
}

// splitDriverList normalizes an owner list without changing its relative order before catalog validation.
func splitDriverList(value string) []string {
	drivers := []string{}
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		driver := strings.ToLower(strings.TrimSpace(part))
		if driver == "" || seen[driver] {
			continue
		}
		seen[driver] = true
		drivers = append(drivers, driver)
	}
	return drivers
}

// stringSliceContainsFold compares driver names before the plan normalizer applies canonical casing.
func stringSliceContainsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

// exactCSVToken reports profile membership without substring matches such as redis-debug.
func exactCSVToken(value string, want string) bool {
	for _, token := range strings.Split(value, ",") {
		if strings.TrimSpace(token) == want {
			return true
		}
	}
	return false
}

// appendCSVToken preserves unrelated profile order while adding one missing exact token.
func appendCSVToken(value string, token string) string {
	tokens := []string{}
	for _, current := range strings.Split(value, ",") {
		current = strings.TrimSpace(current)
		if current == "" || current == token {
			continue
		}
		tokens = append(tokens, current)
	}
	tokens = append(tokens, token)
	return strings.Join(tokens, ",")
}

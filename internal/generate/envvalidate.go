package generate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goforj/goforj/project"
	"github.com/goforj/str/v2"
)

// generationActiveDriver records the original environment key so contract errors remain actionable.
type generationActiveDriver struct {
	key    string
	driver string
}

// scopedEnvironmentKey separates an optional named resource from the root key it overrides.
type scopedEnvironmentKey struct {
	child   string
	rootKey string
}

// appPrimitiveDriverScope groups the overlay levels needed to resolve one App-scoped child driver.
type appPrimitiveDriverScope struct {
	resourcePrefix string
	contract       primitiveEnvContract
	rootDriver     string
	appRootDriver  string
}

// generationDriverResourcePrefixes bounds App-prefix inference to resources whose overlays generated apps consume.
var generationDriverResourcePrefixes = []string{"CACHE", "DB", "EVENTS", "MAIL", "QUEUE", "STORAGE"}

// generationEnvironmentResource pairs an App-overlay marker with the keys that provide trustworthy prefix evidence.
type generationEnvironmentResource struct {
	prefix   string
	rootKeys []string
}

// generationEnvironmentResources keeps App-prefix inference aligned with every generated primitive contract.
func generationEnvironmentResources() []generationEnvironmentResource {
	return []generationEnvironmentResource{
		{prefix: "CACHE", rootKeys: cacheRootKeys},
		{prefix: "DB", rootKeys: dbRootKeys},
		{prefix: "EVENTS", rootKeys: eventRootKeys},
		{prefix: "MAIL", rootKeys: mailRootKeys},
		{prefix: "QUEUE", rootKeys: queueRootKeys},
		{prefix: "STORAGE", rootKeys: storageRootKeys},
	}
}

// discoverPrimitiveChildNames unions resource-first names with names declared only inside configured App overlays.
func discoverPrimitiveChildNames(input generationInput, resourcePrefix string, rootKeys []string) []string {
	resourcePrefix = str.Of(resourcePrefix).ToUpper().Trim().String()
	if resourcePrefix == "" {
		return nil
	}
	names := map[string]struct{}{}
	add := func(prefix string) {
		for _, name := range exactScopedChildNames(input.environment, prefix, rootKeys) {
			name = str.Of(name).ToUpper().Trim().String()
			if name != "" {
				names[name] = struct{}{}
			}
		}
	}
	add(resourcePrefix)
	for _, appPrefix := range generationAppEnvPrefixesForResource(input, resourcePrefix) {
		add(appPrefix + "_" + resourcePrefix)
	}
	return sortStrings(names)
}

// primitiveEnvContract keeps every generator's accepted environment shape explicit and driver-aware.
type primitiveEnvContract struct {
	Prefix        string
	DefaultDriver string
	// LocalDrivers are already present in the generated project's dependency baseline.
	LocalDrivers          []string
	RootKeys              []string
	CommonKeys            map[string]struct{}
	DriverKeys            map[string]map[string]struct{}
	ChildNames            func(environment generationEnvironment) []string
	AllowInactiveRootKeys bool
	InheritRootDriver     bool
	EagerNamedResources   bool
}

// primitiveEnvValidator owns the derived driver state shared by root, named, and App-prefixed validation.
type primitiveEnvValidator struct {
	input            generationInput
	contract         primitiveEnvContract
	supportedDrivers map[string]struct{}
	rootDriver       string
	baseChildren     []string
}

// validatePrimitiveEnv rejects environment shapes that cannot be represented by the generated driver manifest.
func validatePrimitiveEnv(input generationInput, contract primitiveEnvContract) error {
	rootKeySet := makeSet(contract.RootKeys...)
	childNames := contract.ChildNames(input.environment)
	knownChildren := makeSet(childNames...)
	supportedDrivers, err := parseSupportedDrivers(input.environment, contract.Prefix, contract.DriverKeys, contract.LocalDrivers)
	if err != nil {
		return err
	}

	var problems []string
	rootDriver := effectivePrimitiveDriver(input.environment.Get(contract.Prefix+"_DRIVER", contract.DefaultDriver), contract.DefaultDriver)
	rootDriverValid := true
	if supportedDrivers != nil {
		if _, ok := supportedDrivers[rootDriver]; !ok {
			problems = append(problems, fmt.Sprintf("%s selects driver %q not enabled by %s_SUPPORTED_DRIVERS", contract.Prefix+"_DRIVER", rootDriver, contract.Prefix))
			rootDriverValid = false
		}
	}
	if _, ok := contract.DriverKeys[rootDriver]; !ok && rootDriverValid {
		problems = append(problems, fmt.Sprintf("%s selects unsupported driver %q", contract.Prefix+"_DRIVER", rootDriver))
		rootDriverValid = false
	}
	for _, entry := range input.environment.Entries() {
		key := entry.key
		if !strings.HasPrefix(key, contract.Prefix+"_") {
			continue
		}
		if key == contract.Prefix+"_SUPPORTED_DRIVERS" {
			continue
		}

		trimmed := strings.TrimPrefix(key, contract.Prefix+"_")
		scopedKey, ok := splitScopedEnvKey(trimmed, contract.RootKeys)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s is not a supported %s env var", key, strings.ToLower(contract.Prefix)))
			continue
		}
		if scopedKey.child != "" {
			if _, ok := knownChildren[scopedKey.child]; !ok {
				problems = append(problems, fmt.Sprintf("%s does not match a valid %s scope", key, strings.ToLower(contract.Prefix)))
				continue
			}
		}

		driverFallback := contract.DefaultDriver
		if scopedKey.child != "" && contract.InheritRootDriver {
			driverFallback = rootDriver
		}
		driverKey := contract.Prefix + "_DRIVER"
		if scopedKey.child != "" {
			driverKey = contract.Prefix + "_" + scopedKey.child + "_DRIVER"
		}
		driver := effectivePrimitiveDriver(input.environment.Get(driverKey, driverFallback), driverFallback)
		if scopedKey.child == "" {
			driver = rootDriver
			if !rootDriverValid {
				continue
			}
		}
		if scopedKey.child != "" && supportedDrivers != nil {
			if _, ok := supportedDrivers[driver]; !ok {
				problems = append(problems, fmt.Sprintf("%s selects driver %q not enabled by %s_SUPPORTED_DRIVERS", contract.Prefix+"_"+scopedKey.child+"_DRIVER", driver, contract.Prefix))
				continue
			}
		}
		allowedKeys, err := allowedPrimitiveKeys(contract, driver)
		if err != nil {
			if scopedKey.child == "" {
				problems = append(problems, fmt.Sprintf("%s selects unsupported driver %q", contract.Prefix+"_DRIVER", driver))
			} else {
				problems = append(problems, fmt.Sprintf("%s selects unsupported driver %q", contract.Prefix+"_"+scopedKey.child+"_DRIVER", driver))
			}
			continue
		}
		if _, ok := rootKeySet[scopedKey.rootKey]; !ok {
			problems = append(problems, fmt.Sprintf("%s is not a supported %s env var", key, strings.ToLower(contract.Prefix)))
			continue
		}
		if _, ok := allowedKeys[scopedKey.rootKey]; ok {
			continue
		}
		if scopedKey.child == "" && contract.AllowInactiveRootKeys {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s is not supported for %s driver %q", key, strings.ToLower(contract.Prefix), driver))
	}
	validator := primitiveEnvValidator{
		input:            input,
		contract:         contract,
		supportedDrivers: supportedDrivers,
		rootDriver:       rootDriver,
		baseChildren:     childNames,
	}
	problems = append(problems, validator.eagerNamedDriverProblems()...)
	problems = append(problems, validator.appPrefixedProblems()...)

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("invalid %s env:\n- %s", strings.ToLower(contract.Prefix), strings.Join(problems, "\n- "))
}

// eagerNamedDriverProblems accounts for generated managers that initialize every accessor in every App.
func (v primitiveEnvValidator) eagerNamedDriverProblems() []string {
	if !v.contract.EagerNamedResources {
		return nil
	}
	problems := []string{}
	for _, child := range discoverPrimitiveChildNames(v.input, v.contract.Prefix, v.contract.RootKeys) {
		key := v.contract.Prefix + "_" + child + "_DRIVER"
		if _, set := v.input.environment.Lookup(key); set {
			continue
		}
		fallback := v.contract.DefaultDriver
		if v.contract.InheritRootDriver {
			fallback = v.rootDriver
		}
		driver := effectivePrimitiveDriver("", fallback)
		if v.supportedDrivers != nil {
			if _, supported := v.supportedDrivers[driver]; !supported {
				problems = append(problems, fmt.Sprintf("%s defaults to driver %q not enabled by %s_SUPPORTED_DRIVERS", key, driver, v.contract.Prefix))
				continue
			}
		}
		if _, supported := v.contract.DriverKeys[driver]; !supported {
			problems = append(problems, fmt.Sprintf("%s defaults to unsupported driver %q", key, driver))
		}
	}
	return problems
}

// appPrefixedProblems applies the root contract after resolving each App value as an overlay of base resource state.
func (v primitiveEnvValidator) appPrefixedProblems() []string {
	problems := []string{}
	for _, appPrefix := range generationAppEnvPrefixesForResource(v.input, v.contract.Prefix) {
		resourcePrefix := appPrefix + "_" + v.contract.Prefix
		appRootDriver := v.rootDriver
		if value, set := v.input.environment.Lookup(resourcePrefix + "_DRIVER"); set {
			appRootDriver = effectivePrimitiveDriver(value, v.contract.DefaultDriver)
		}
		knownChildren := map[string]struct{}{}
		for _, child := range v.baseChildren {
			child = str.Of(child).ToUpper().Trim().String()
			if child != "" {
				knownChildren[child] = struct{}{}
			}
		}
		for _, child := range exactScopedChildNames(v.input.environment, resourcePrefix, v.contract.RootKeys) {
			knownChildren[child] = struct{}{}
		}

		for _, entry := range v.input.environment.Entries() {
			key := entry.key
			if !strings.HasPrefix(key, resourcePrefix+"_") {
				continue
			}
			trimmed := strings.TrimPrefix(key, resourcePrefix+"_")
			scopedKey, ok := splitScopedEnvKey(trimmed, v.contract.RootKeys)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s is not a supported %s env var", key, strings.ToLower(v.contract.Prefix)))
				continue
			}
			if scopedKey.child != "" {
				if _, known := knownChildren[scopedKey.child]; !known {
					problems = append(problems, fmt.Sprintf("%s does not match a valid %s scope", key, strings.ToLower(v.contract.Prefix)))
					continue
				}
			}

			driver := appRootDriver
			if scopedKey.child != "" {
				driver = effectiveAppPrimitiveChildDriver(v.input.environment, appPrimitiveDriverScope{
					resourcePrefix: resourcePrefix,
					contract:       v.contract,
					rootDriver:     v.rootDriver,
					appRootDriver:  appRootDriver,
				}, scopedKey.child)
			}
			if v.supportedDrivers != nil {
				if _, supported := v.supportedDrivers[driver]; !supported {
					selectionKey := resourcePrefix + "_DRIVER"
					if scopedKey.child != "" {
						selectionKey = resourcePrefix + "_" + scopedKey.child + "_DRIVER"
					}
					problems = append(problems, fmt.Sprintf("%s selects driver %q not enabled by %s_SUPPORTED_DRIVERS", selectionKey, driver, v.contract.Prefix))
					continue
				}
			}
			allowedKeys, err := allowedPrimitiveKeys(v.contract, driver)
			if err != nil {
				selectionKey := resourcePrefix + "_DRIVER"
				if scopedKey.child != "" {
					selectionKey = resourcePrefix + "_" + scopedKey.child + "_DRIVER"
				}
				problems = append(problems, fmt.Sprintf("%s selects unsupported driver %q", selectionKey, driver))
				continue
			}
			if _, allowed := allowedKeys[scopedKey.rootKey]; allowed {
				continue
			}
			if scopedKey.child == "" && v.contract.AllowInactiveRootKeys {
				continue
			}
			problems = append(problems, fmt.Sprintf("%s is not supported for %s driver %q", key, strings.ToLower(v.contract.Prefix), driver))
		}
	}
	return problems
}

// effectiveAppPrimitiveChildDriver mirrors runtime overlay precedence for one App-scoped named resource.
func effectiveAppPrimitiveChildDriver(environment generationEnvironment, scope appPrimitiveDriverScope, child string) string {
	baseFallback := scope.contract.DefaultDriver
	appFallback := scope.contract.DefaultDriver
	if scope.contract.InheritRootDriver {
		baseFallback = scope.rootDriver
		appFallback = scope.appRootDriver
	}
	baseKey := scope.contract.Prefix + "_" + child + "_DRIVER"
	driver := baseFallback
	if value, set := environment.Lookup(baseKey); set {
		driver = effectivePrimitiveDriver(value, baseFallback)
	}
	if value, set := environment.Lookup(scope.resourcePrefix + "_" + child + "_DRIVER"); set {
		driver = effectivePrimitiveDriver(value, appFallback)
	}
	return driver
}

// appPrefixedActiveDrivers resolves root and additional-app overlays with the same blank-driver fallbacks used at runtime.
func appPrefixedActiveDrivers(input generationInput, resourcePrefix string, defaultDriver string, inheritRootDriver bool) []generationActiveDriver {
	resourcePrefix = str.Of(resourcePrefix).ToUpper().Trim().String()
	if resourcePrefix == "" {
		return nil
	}
	baseRootDriver := effectivePrimitiveDriver(input.environment.Get(resourcePrefix+"_DRIVER", defaultDriver), defaultDriver)
	drivers := make([]generationActiveDriver, 0)
	seen := map[string]struct{}{}
	for _, appPrefix := range generationAppEnvPrefixesForResource(input, resourcePrefix) {
		keyPrefix := appPrefix + "_" + resourcePrefix + "_"
		appRootDriver := baseRootDriver
		if value, ok := input.environment.Lookup(keyPrefix + "DRIVER"); ok {
			appRootDriver = effectivePrimitiveDriver(value, defaultDriver)
		}
		for _, entry := range input.environment.Entries() {
			key, value := entry.key, entry.value
			if !strings.HasPrefix(key, keyPrefix) || !strings.HasSuffix(key, "_DRIVER") {
				continue
			}
			relative := strings.TrimPrefix(key, keyPrefix)
			if relative != "DRIVER" && strings.TrimSuffix(relative, "_DRIVER") == "" {
				continue
			}
			fallback := defaultDriver
			if relative != "DRIVER" && inheritRootDriver {
				fallback = appRootDriver
			}
			driver := effectivePrimitiveDriver(value, fallback)
			identity := key + "\x00" + driver
			if _, exists := seen[identity]; exists {
				continue
			}
			seen[identity] = struct{}{}
			drivers = append(drivers, generationActiveDriver{key: key, driver: driver})
		}
	}
	sort.Slice(drivers, func(left, right int) bool {
		if drivers[left].key == drivers[right].key {
			return drivers[left].driver < drivers[right].driver
		}
		return drivers[left].key < drivers[right].key
	})
	return drivers
}

// generationAppEnvPrefixes returns the App identities fixed when the generation snapshot was captured.
func generationAppEnvPrefixes(input generationInput) []string {
	return append([]string(nil), input.appPrefixes...)
}

// generationAppEnvPrefixSet combines configured Apps with conservative evidence from valid App-before-resource keys.
func generationAppEnvPrefixSet(projectDir string, environment generationEnvironment) map[string]struct{} {
	configured := configuredGenerationAppEnvPrefixes(projectDir)
	prefixes := map[string]struct{}{}
	for prefix := range configured {
		prefixes[prefix] = struct{}{}
	}
	for _, entry := range environment.Entries() {
		if generationKeyMatchesConfiguredApp(entry.key, configured) {
			continue
		}
		prefix, ok := inferredGenerationAppPrefix(entry.key)
		if !ok || generationResourceFirstPrefix(prefix) {
			continue
		}
		prefixes[prefix] = struct{}{}
	}
	return prefixes
}

// configuredGenerationAppEnvPrefixes returns every durable App prefix without making a missing config fatal to direct generators.
func configuredGenerationAppEnvPrefixes(projectDir string) map[string]struct{} {
	prefixes := map[string]struct{}{}
	addName := func(name string) {
		if prefix := project.AppEnvironmentPrefix(name); prefix != "" {
			prefixes[prefix] = struct{}{}
		}
	}
	if strings.TrimSpace(projectDir) != "" {
		config, err := project.LoadProjectConfigAt(projectDir)
		if err != nil {
			return prefixes
		}
		for name := range config.Apps {
			addName(name)
		}
		for name := range config.Dev.Apps {
			addName(name)
		}
		for name := range config.Dev.Run {
			addName(name)
		}
	}
	return prefixes
}

// generationKeyMatchesConfiguredApp lets explicit App identities win when their names contain resource words.
func generationKeyMatchesConfiguredApp(key string, configured map[string]struct{}) bool {
	for prefix := range configured {
		if isGenerationAppResourceKey(key, prefix) {
			return true
		}
	}
	return false
}

// isGenerationAppResourceKey recognizes any resource-shaped key after an already trusted App prefix, including typos for validation.
func isGenerationAppResourceKey(key string, appPrefix string) bool {
	for _, resource := range generationEnvironmentResources() {
		if strings.HasPrefix(key, appPrefix+"_"+resource.prefix+"_") {
			return true
		}
	}
	return false
}

// inferredGenerationAppPrefix uses the earliest valid resource marker so ambiguous unconfigured names retain the historical boundary.
func inferredGenerationAppPrefix(key string) (string, bool) {
	bestIndex := -1
	bestPrefix := ""
	for _, resource := range generationEnvironmentResources() {
		marker := "_" + resource.prefix + "_"
		searchFrom := 0
		for searchFrom < len(key) {
			relativeIndex := strings.Index(key[searchFrom:], marker)
			if relativeIndex < 0 {
				break
			}
			index := searchFrom + relativeIndex
			prefix := key[:index]
			if index > 0 && validGenerationAppEnvPrefix(prefix) {
				if _, ok := splitScopedEnvKey(key[index+len(marker):], resource.rootKeys); ok && (bestIndex < 0 || index < bestIndex) {
					bestIndex = index
					bestPrefix = prefix
				}
			}
			searchFrom = index + 1
		}
	}
	return bestPrefix, bestIndex >= 0
}

// generationAppEnvPrefixesForResource keeps stale overlays from Apps that do not participate in a component out of generated manifests.
func generationAppEnvPrefixesForResource(input generationInput, resourcePrefix string) []string {
	prefixes := generationAppEnvPrefixes(input)
	config, err := project.LoadProjectConfigAt(input.projectDir)
	if err != nil {
		return prefixes
	}
	resourcePrefix = str.Of(resourcePrefix).ToUpper().Trim().String()
	enabled := map[string]struct{}{}
	configured := map[string]struct{}{}
	for name, appConfig := range config.Apps {
		prefix := project.AppEnvironmentPrefix(name)
		if prefix == "" {
			continue
		}
		configured[name] = struct{}{}
		components := project.NormalizeConfiguredAppComponents(config, appConfig.Components)
		if generationComponentsSupportResource(components, resourcePrefix) {
			enabled[prefix] = struct{}{}
		}
	}
	defaultComponents := config.Render.Components.WithResolvedDependencies()
	addDefaultProjection := func(name string) {
		if _, exists := configured[name]; exists || !generationComponentsSupportResource(defaultComponents, resourcePrefix) {
			return
		}
		if prefix := project.AppEnvironmentPrefix(name); prefix != "" {
			enabled[prefix] = struct{}{}
		}
	}
	for name := range config.Dev.Apps {
		addDefaultProjection(name)
	}
	for name := range config.Dev.Run {
		addDefaultProjection(name)
	}
	filtered := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		if _, exists := enabled[prefix]; exists {
			filtered = append(filtered, prefix)
		}
	}
	return filtered
}

// generationComponentsSupportResource maps generated resource prefixes to their owning App component.
func generationComponentsSupportResource(components project.Components, resourcePrefix string) bool {
	switch resourcePrefix {
	case "CACHE":
		return components.Cache
	case "DB":
		return components.HasDatabase()
	case "EVENTS":
		return components.Events
	case "MAIL":
		return components.Mail
	case "QUEUE":
		return components.Jobs
	case "STORAGE":
		return components.Storage
	default:
		return true
	}
}

// validGenerationAppEnvPrefix rejects malformed ambient keys before they can widen a compiled manifest.
func validGenerationAppEnvPrefix(prefix string) bool {
	if prefix == "" || strings.HasPrefix(prefix, "_") || strings.HasSuffix(prefix, "_") || strings.Contains(prefix, "__") {
		return false
	}
	for _, character := range prefix {
		if character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' {
			continue
		}
		return false
	}
	return true
}

// generationResourceFirstPrefix keeps ordinary RESOURCE_<NAME>_DRIVER scopes out of App-prefix inference.
func generationResourceFirstPrefix(prefix string) bool {
	for _, resourcePrefix := range generationDriverResourcePrefixes {
		if prefix == resourcePrefix || strings.HasPrefix(prefix, resourcePrefix+"_") {
			return true
		}
	}
	return false
}

// effectivePrimitiveDriver preserves runtime fallback semantics when a declared driver is blank.
func effectivePrimitiveDriver(value, fallback string) string {
	driver := str.Of(value).Trim().ToLower().String()
	if driver == "" {
		driver = str.Of(fallback).Trim().ToLower().String()
	}
	return driver
}

// allowedPrimitiveKeys combines shared and driver-specific keys so stale settings fail before code generation.
func allowedPrimitiveKeys(contract primitiveEnvContract, driver string) (map[string]struct{}, error) {
	allowed := make(map[string]struct{}, len(contract.CommonKeys)+1)
	for key := range contract.CommonKeys {
		allowed[key] = struct{}{}
	}

	driverKeys, ok := contract.DriverKeys[driver]
	if !ok {
		return nil, fmt.Errorf("unsupported driver %q", driver)
	}
	for key := range driverKeys {
		allowed[key] = struct{}{}
	}
	return allowed, nil
}

// splitScopedEnvKey prefers complete multi-part keys so suffixes such as WORKERPOOL_WORKERS do not become false child names.
func splitScopedEnvKey(value string, rootKeys []string) (scopedEnvironmentKey, bool) {
	orderedRootKeys := append([]string(nil), rootKeys...)
	sort.SliceStable(orderedRootKeys, func(left, right int) bool {
		return len(strings.Split(orderedRootKeys[left], "_")) > len(strings.Split(orderedRootKeys[right], "_"))
	})
	for _, rootKey := range orderedRootKeys {
		if value == rootKey {
			return scopedEnvironmentKey{rootKey: rootKey}, true
		}
	}
	for _, rootKey := range orderedRootKeys {
		suffix := "_" + rootKey
		if !strings.HasSuffix(value, suffix) {
			continue
		}
		child := str.Of(value).TrimSuffix(suffix).Trim().ToUpper().String()
		if child == "" {
			return scopedEnvironmentKey{}, false
		}
		return scopedEnvironmentKey{child: child, rootKey: rootKey}, true
	}
	return scopedEnvironmentKey{}, false
}

// makeSet avoids repeated linear scans in environment validation paths with overlapping key inventories.
func makeSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

// parseSupportedDrivers keeps local drivers available while validating the optional external-driver manifest.
func parseSupportedDrivers(environment generationEnvironment, prefix string, knownDrivers map[string]map[string]struct{}, localDrivers []string) (map[string]struct{}, error) {
	raw := str.Of(environment.Get(prefix+"_SUPPORTED_DRIVERS", "")).Trim().ToLower().String()
	if raw == "" {
		return nil, nil
	}
	set := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		driver := str.Of(part).Trim().ToLower().String()
		if driver == "" {
			continue
		}
		if _, ok := knownDrivers[driver]; !ok {
			return nil, fmt.Errorf("%s_SUPPORTED_DRIVERS includes unsupported driver %q", prefix, driver)
		}
		set[driver] = struct{}{}
	}
	if len(set) == 0 {
		return nil, nil
	}
	for _, driver := range localDrivers {
		driver = str.Of(driver).Trim().ToLower().String()
		if _, ok := knownDrivers[driver]; ok {
			set[driver] = struct{}{}
		}
	}
	return set, nil
}

// supportedDrivers always includes local drivers and uses active-driver fallbacks when no external manifest is declared.
func supportedDrivers(environment generationEnvironment, prefix string, knownDrivers map[string]map[string]struct{}, fallback, localDrivers []string) ([]string, error) {
	set, err := parseSupportedDrivers(environment, prefix, knownDrivers, localDrivers)
	if err != nil {
		return nil, err
	}
	if set != nil {
		return sortStrings(set), nil
	}

	out := map[string]struct{}{}
	for _, driver := range fallback {
		driver = str.Of(driver).Trim().ToLower().String()
		if driver == "" {
			continue
		}
		if _, ok := knownDrivers[driver]; !ok {
			continue
		}
		out[driver] = struct{}{}
	}
	for _, driver := range localDrivers {
		driver = str.Of(driver).Trim().ToLower().String()
		if _, ok := knownDrivers[driver]; ok {
			out[driver] = struct{}{}
		}
	}
	return sortStrings(out), nil
}

// sortStrings keeps generated imports, manifests, and validation errors deterministic across runs.
func sortStrings(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for driver := range set {
		out = append(out, driver)
	}
	sort.Strings(out)
	return out
}

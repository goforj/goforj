package generate

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/goforj/env/v2"
	"github.com/goforj/goforj/project"
	"github.com/goforj/str"
)

// generationActiveDriver records the original environment key so contract errors remain actionable.
type generationActiveDriver struct {
	key    string
	driver string
}

// generationDriverResourcePrefixes bounds App-prefix inference to resources whose overlays generated apps consume.
var generationDriverResourcePrefixes = []string{"CACHE", "DB", "EVENTS", "MAIL", "QUEUE", "STORAGE"}

// discoverPrimitiveChildNames unions resource-first names with names declared only inside configured App overlays.
func discoverPrimitiveChildNames(projectDir string, resourcePrefix string, rootKeys []string) []string {
	resourcePrefix = strings.TrimSpace(strings.ToUpper(resourcePrefix))
	if resourcePrefix == "" {
		return nil
	}
	names := map[string]struct{}{}
	add := func(prefix string) {
		for _, name := range exactScopedChildNames(prefix, rootKeys) {
			name = strings.TrimSpace(strings.ToUpper(name))
			if name != "" {
				names[name] = struct{}{}
			}
		}
	}
	add(resourcePrefix)
	for _, appPrefix := range generationAppEnvPrefixesForResource(projectDir, resourcePrefix) {
		add(appPrefix + "_" + resourcePrefix)
	}
	return sortStrings(names)
}

type primitiveEnvContract struct {
	Prefix                string
	DefaultDriver         string
	RootKeys              []string
	CommonKeys            map[string]struct{}
	DriverKeys            map[string]map[string]struct{}
	ChildNames            func(scope env.Scope) []string
	AllowInactiveRootKeys bool
	InheritRootDriver     bool
	EagerNamedResources   bool
}

// validatePrimitiveEnv rejects environment shapes that cannot be represented by the generated driver manifest.
func validatePrimitiveEnv(projectDir string, contract primitiveEnvContract) error {
	rootKeySet := makeSet(contract.RootKeys...)
	scope := env.WithPrefix(contract.Prefix)
	childNames := contract.ChildNames(scope)
	knownChildren := makeSet(childNames...)
	supportedDrivers, err := parseSupportedDrivers(contract.Prefix, contract.DriverKeys)
	if err != nil {
		return err
	}

	var problems []string
	rootDriver := effectivePrimitiveDriver(scope.Get("DRIVER", contract.DefaultDriver), contract.DefaultDriver)
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
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, contract.Prefix+"_") {
			continue
		}
		if key == contract.Prefix+"_SUPPORTED_DRIVERS" {
			continue
		}

		trimmed := strings.TrimPrefix(key, contract.Prefix+"_")
		child, rootKey, ok := splitScopedEnvKey(trimmed, contract.RootKeys)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s is not a supported %s env var", key, strings.ToLower(contract.Prefix)))
			continue
		}
		if child != "" {
			if _, ok := knownChildren[child]; !ok {
				problems = append(problems, fmt.Sprintf("%s does not match a valid %s scope", key, strings.ToLower(contract.Prefix)))
				continue
			}
		}

		driverScope := scope
		if child != "" {
			driverScope = scope.Child(child)
		}
		driverFallback := contract.DefaultDriver
		if child != "" && contract.InheritRootDriver {
			driverFallback = rootDriver
		}
		driver := effectivePrimitiveDriver(driverScope.Get("DRIVER", driverFallback), driverFallback)
		if child == "" {
			driver = rootDriver
			if !rootDriverValid {
				continue
			}
		}
		if child != "" && supportedDrivers != nil {
			if _, ok := supportedDrivers[driver]; !ok {
				problems = append(problems, fmt.Sprintf("%s selects driver %q not enabled by %s_SUPPORTED_DRIVERS", contract.Prefix+"_"+child+"_DRIVER", driver, contract.Prefix))
				continue
			}
		}
		allowedKeys, err := allowedPrimitiveKeys(contract, driver)
		if err != nil {
			if child == "" {
				problems = append(problems, fmt.Sprintf("%s selects unsupported driver %q", contract.Prefix+"_DRIVER", driver))
			} else {
				problems = append(problems, fmt.Sprintf("%s selects unsupported driver %q", contract.Prefix+"_"+child+"_DRIVER", driver))
			}
			continue
		}
		if _, ok := rootKeySet[rootKey]; !ok {
			problems = append(problems, fmt.Sprintf("%s is not a supported %s env var", key, strings.ToLower(contract.Prefix)))
			continue
		}
		if _, ok := allowedKeys[rootKey]; ok {
			continue
		}
		if child == "" && contract.AllowInactiveRootKeys {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s is not supported for %s driver %q", key, strings.ToLower(contract.Prefix), driver))
	}
	problems = append(problems, validateEagerNamedPrimitiveDrivers(projectDir, contract, supportedDrivers, rootDriver)...)
	problems = append(problems, validateAppPrefixedPrimitiveEnv(projectDir, contract, supportedDrivers, rootDriver, childNames)...)

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("invalid %s env:\n- %s", strings.ToLower(contract.Prefix), strings.Join(problems, "\n- "))
}

// validateEagerNamedPrimitiveDrivers accounts for generated managers that initialize every accessor in every App.
func validateEagerNamedPrimitiveDrivers(projectDir string, contract primitiveEnvContract, supportedDrivers map[string]struct{}, rootDriver string) []string {
	if !contract.EagerNamedResources {
		return nil
	}
	problems := []string{}
	for _, child := range discoverPrimitiveChildNames(projectDir, contract.Prefix, contract.RootKeys) {
		key := contract.Prefix + "_" + child + "_DRIVER"
		if _, set := os.LookupEnv(key); set {
			continue
		}
		fallback := contract.DefaultDriver
		if contract.InheritRootDriver {
			fallback = rootDriver
		}
		driver := effectivePrimitiveDriver("", fallback)
		if supportedDrivers != nil {
			if _, supported := supportedDrivers[driver]; !supported {
				problems = append(problems, fmt.Sprintf("%s defaults to driver %q not enabled by %s_SUPPORTED_DRIVERS", key, driver, contract.Prefix))
				continue
			}
		}
		if _, supported := contract.DriverKeys[driver]; !supported {
			problems = append(problems, fmt.Sprintf("%s defaults to unsupported driver %q", key, driver))
		}
	}
	return problems
}

// validateAppPrefixedPrimitiveEnv applies the root contract after resolving each App value as an overlay of base resource state.
func validateAppPrefixedPrimitiveEnv(projectDir string, contract primitiveEnvContract, supportedDrivers map[string]struct{}, rootDriver string, baseChildren []string) []string {
	problems := []string{}
	for _, appPrefix := range generationAppEnvPrefixesForResource(projectDir, contract.Prefix) {
		resourcePrefix := appPrefix + "_" + contract.Prefix
		appRootDriver := rootDriver
		if value, set := os.LookupEnv(resourcePrefix + "_DRIVER"); set {
			appRootDriver = effectivePrimitiveDriver(value, contract.DefaultDriver)
		}
		knownChildren := map[string]struct{}{}
		for _, child := range baseChildren {
			child = strings.TrimSpace(strings.ToUpper(child))
			if child != "" {
				knownChildren[child] = struct{}{}
			}
		}
		for _, child := range exactScopedChildNames(resourcePrefix, contract.RootKeys) {
			knownChildren[child] = struct{}{}
		}

		for _, assignment := range os.Environ() {
			key, _, ok := strings.Cut(assignment, "=")
			if !ok || !strings.HasPrefix(key, resourcePrefix+"_") {
				continue
			}
			trimmed := strings.TrimPrefix(key, resourcePrefix+"_")
			child, rootKey, split := splitScopedEnvKey(trimmed, contract.RootKeys)
			if !split {
				problems = append(problems, fmt.Sprintf("%s is not a supported %s env var", key, strings.ToLower(contract.Prefix)))
				continue
			}
			if child != "" {
				if _, known := knownChildren[child]; !known {
					problems = append(problems, fmt.Sprintf("%s does not match a valid %s scope", key, strings.ToLower(contract.Prefix)))
					continue
				}
			}

			driver := appRootDriver
			if child != "" {
				driver = effectiveAppPrimitiveChildDriver(resourcePrefix, contract, rootDriver, appRootDriver, child)
			}
			if supportedDrivers != nil {
				if _, supported := supportedDrivers[driver]; !supported {
					selectionKey := resourcePrefix + "_DRIVER"
					if child != "" {
						selectionKey = resourcePrefix + "_" + child + "_DRIVER"
					}
					problems = append(problems, fmt.Sprintf("%s selects driver %q not enabled by %s_SUPPORTED_DRIVERS", selectionKey, driver, contract.Prefix))
					continue
				}
			}
			allowedKeys, err := allowedPrimitiveKeys(contract, driver)
			if err != nil {
				selectionKey := resourcePrefix + "_DRIVER"
				if child != "" {
					selectionKey = resourcePrefix + "_" + child + "_DRIVER"
				}
				problems = append(problems, fmt.Sprintf("%s selects unsupported driver %q", selectionKey, driver))
				continue
			}
			if _, allowed := allowedKeys[rootKey]; allowed {
				continue
			}
			if child == "" && contract.AllowInactiveRootKeys {
				continue
			}
			problems = append(problems, fmt.Sprintf("%s is not supported for %s driver %q", key, strings.ToLower(contract.Prefix), driver))
		}
	}
	return problems
}

// effectiveAppPrimitiveChildDriver mirrors runtime overlay precedence for one App-scoped named resource.
func effectiveAppPrimitiveChildDriver(appResourcePrefix string, contract primitiveEnvContract, rootDriver string, appRootDriver string, child string) string {
	baseFallback := contract.DefaultDriver
	appFallback := contract.DefaultDriver
	if contract.InheritRootDriver {
		baseFallback = rootDriver
		appFallback = appRootDriver
	}
	baseKey := contract.Prefix + "_" + child + "_DRIVER"
	driver := baseFallback
	if value, set := os.LookupEnv(baseKey); set {
		driver = effectivePrimitiveDriver(value, baseFallback)
	}
	if value, set := os.LookupEnv(appResourcePrefix + "_" + child + "_DRIVER"); set {
		driver = effectivePrimitiveDriver(value, appFallback)
	}
	return driver
}

// appPrefixedActiveDrivers resolves root and named App overlays with the same blank-driver fallbacks used at runtime.
func appPrefixedActiveDrivers(projectDir string, resourcePrefix string, defaultDriver string, inheritRootDriver bool) []generationActiveDriver {
	resourcePrefix = strings.TrimSpace(strings.ToUpper(resourcePrefix))
	if resourcePrefix == "" {
		return nil
	}
	baseRootDriver := effectivePrimitiveDriver(env.WithPrefix(resourcePrefix).Get("DRIVER", defaultDriver), defaultDriver)
	drivers := make([]generationActiveDriver, 0)
	seen := map[string]struct{}{}
	for _, appPrefix := range generationAppEnvPrefixesForResource(projectDir, resourcePrefix) {
		keyPrefix := appPrefix + "_" + resourcePrefix + "_"
		appRootDriver := baseRootDriver
		if value, ok := os.LookupEnv(keyPrefix + "DRIVER"); ok {
			appRootDriver = effectivePrimitiveDriver(value, defaultDriver)
		}
		for _, assignment := range os.Environ() {
			key, value, ok := strings.Cut(assignment, "=")
			if !ok || !strings.HasPrefix(key, keyPrefix) || !strings.HasSuffix(key, "_DRIVER") {
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

// generationAppEnvPrefixes combines configured Apps with conservative evidence from App-before-resource driver keys.
func generationAppEnvPrefixes(projectDir string) []string {
	prefixes := map[string]struct{}{}
	addName := func(name string) {
		if prefix := generationAppEnvPrefix(name); prefix != "" {
			prefixes[prefix] = struct{}{}
		}
	}
	if config, err := project.LoadProjectConfigAt(projectDir); err == nil {
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
	for _, assignment := range os.Environ() {
		key, _, ok := strings.Cut(assignment, "=")
		if !ok || !strings.HasSuffix(key, "_DRIVER") {
			continue
		}
		markerIndex := -1
		for _, resourcePrefix := range generationDriverResourcePrefixes {
			index := strings.Index(key, "_"+resourcePrefix+"_")
			if index > 0 && (markerIndex < 0 || index < markerIndex) {
				markerIndex = index
			}
		}
		if markerIndex <= 0 {
			continue
		}
		prefix := key[:markerIndex]
		if !validGenerationAppEnvPrefix(prefix) || generationResourceFirstPrefix(prefix) {
			continue
		}
		prefixes[prefix] = struct{}{}
	}
	return sortStrings(prefixes)
}

// generationAppEnvPrefixesForResource keeps stale overlays from Apps that do not participate in a component out of generated manifests.
func generationAppEnvPrefixesForResource(projectDir string, resourcePrefix string) []string {
	prefixes := generationAppEnvPrefixes(projectDir)
	config, err := project.LoadProjectConfigAt(projectDir)
	if err != nil {
		return prefixes
	}
	resourcePrefix = strings.TrimSpace(strings.ToUpper(resourcePrefix))
	enabled := map[string]struct{}{}
	configured := map[string]struct{}{}
	for name, appConfig := range config.Apps {
		prefix := generationAppEnvPrefix(name)
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
		if prefix := generationAppEnvPrefix(name); prefix != "" {
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

// generationAppEnvPrefix mirrors the generated runtime's normalization for named App overlays.
func generationAppEnvPrefix(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == project.DefaultAppName {
		return ""
	}
	parts := strings.FieldsFunc(name, func(character rune) bool {
		return character == '-' || character == '_' || character == ' ' || character == '.'
	})
	for index := range parts {
		parts[index] = strings.ToUpper(parts[index])
	}
	return strings.Join(parts, "_")
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
	driver := str.Of(value).TrimSpace().ToLower().String()
	if driver == "" {
		driver = str.Of(fallback).TrimSpace().ToLower().String()
	}
	return driver
}

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
func splitScopedEnvKey(value string, rootKeys []string) (child string, rootKey string, ok bool) {
	orderedRootKeys := append([]string(nil), rootKeys...)
	sort.SliceStable(orderedRootKeys, func(left, right int) bool {
		return len(strings.Split(orderedRootKeys[left], "_")) > len(strings.Split(orderedRootKeys[right], "_"))
	})
	for _, rootKey := range orderedRootKeys {
		if value == rootKey {
			return "", rootKey, true
		}
	}
	for _, rootKey := range orderedRootKeys {
		suffix := "_" + rootKey
		if !strings.HasSuffix(value, suffix) {
			continue
		}
		child = strings.TrimSuffix(value, suffix)
		child = str.Of(child).TrimSpace().ToUpper().String()
		if child == "" {
			return "", "", false
		}
		return child, rootKey, true
	}
	return "", "", false
}

func makeSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func parseSupportedDrivers(prefix string, knownDrivers map[string]map[string]struct{}) (map[string]struct{}, error) {
	raw := str.Of(env.WithPrefix(prefix).Get("SUPPORTED_DRIVERS", "")).TrimSpace().ToLower().String()
	if raw == "" {
		return nil, nil
	}
	set := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		driver := str.Of(part).TrimSpace().ToLower().String()
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
	return set, nil
}

func supportedDrivers(prefix string, knownDrivers map[string]map[string]struct{}, fallback []string) ([]string, error) {
	set, err := parseSupportedDrivers(prefix, knownDrivers)
	if err != nil {
		return nil, err
	}
	if set != nil {
		return sortStrings(set), nil
	}

	out := map[string]struct{}{}
	for _, driver := range fallback {
		driver = str.Of(driver).TrimSpace().ToLower().String()
		if driver == "" {
			continue
		}
		if _, ok := knownDrivers[driver]; !ok {
			continue
		}
		out[driver] = struct{}{}
	}
	return sortStrings(out), nil
}

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

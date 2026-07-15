package forj

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/goforj/goforj/project"
	"github.com/joho/godotenv"
)

// effectiveResourceConsumersFromEnvironment resolves root, named, and App-prefixed resource scopes without mutating owner input.
func effectiveResourceConsumersFromEnvironment(source []byte, plan project.ResourcePlan, components project.Components, appNames []string) ([]project.EffectiveResourceConsumer, error) {
	appComponents := make(map[string]project.Components, len(appNames))
	for _, name := range appNames {
		appComponents[strings.ToLower(strings.TrimSpace(name))] = components
	}
	return effectiveResourceConsumersFromAppComponents(source, plan, components, components, appNames, appComponents)
}

// effectiveResourceConsumersFromProjectConfig applies each configured App's actual participation to resource discovery.
func effectiveResourceConsumersFromProjectConfig(source []byte, plan project.ResourcePlan, projectComponents project.Components, config *project.Config) ([]project.EffectiveResourceConsumer, error) {
	defaultComponents := projectComponents
	appNames := configuredResourceAppNames(config)
	appComponents := make(map[string]project.Components, len(appNames))
	if config != nil {
		defaultComponents = config.Render.Components.WithResolvedDependencies()
		for configuredName, appConfig := range config.Apps {
			name := strings.ToLower(strings.TrimSpace(configuredName))
			if name == "" || name == project.DefaultAppName {
				continue
			}
			appComponents[name] = project.NormalizeConfiguredAppComponents(config, appConfig.Components)
		}
	}
	return effectiveResourceConsumersFromAppComponents(source, plan, defaultComponents, projectComponents, appNames, appComponents)
}

// effectiveResourceConsumersFromAppComponents keeps applicability App-local while retaining project-wide service placement.
func effectiveResourceConsumersFromAppComponents(source []byte, plan project.ResourcePlan, defaultComponents project.Components, projectComponents project.Components, appNames []string, appComponents map[string]project.Components) ([]project.EffectiveResourceConsumer, error) {
	values, err := resourceEnvironmentAssignments(source)
	if err != nil {
		return nil, fmt.Errorf("parse effective resource environment: %w", err)
	}
	appPrefixes := resourceAppPrefixes(values, appNames)
	consumers := []project.EffectiveResourceConsumer{}
	seen := map[string]bool{}

	addScope := func(appName string, appPrefix string, scopeComponents project.Components) error {
		for _, definition := range project.ResourceCatalog() {
			if !definition.AppliesTo(scopeComponents) {
				continue
			}
			selection, ok := plan.Selection(definition.Key)
			if !ok {
				continue
			}
			rootDriver := effectiveResourceDriver(values, appPrefix, definition.Key, "", selection.Active)
			rootConsumer := effectiveResourceConsumerName(appName, definition.Key, "")
			consumer, err := effectiveResourceConsumer(values, appPrefix, definition.Key, "", rootConsumer, rootDriver, selection.Active, projectComponents)
			if err != nil {
				return err
			}
			consumers = appendEffectiveResourceConsumer(consumers, seen, consumer)

			namedDrivers := effectiveNamedResourceDrivers(values, appPrefix, plan, scopeComponents, definition.Key, rootDriver)
			names := make([]string, 0, len(namedDrivers))
			for name := range namedDrivers {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				consumerName := effectiveResourceConsumerName(appName, definition.Key, name)
				consumer, err := effectiveResourceConsumer(values, appPrefix, definition.Key, name, consumerName, namedDrivers[name], selection.Active, projectComponents)
				if err != nil {
					return err
				}
				consumers = appendEffectiveResourceConsumer(consumers, seen, consumer)
			}
		}
		return nil
	}

	if err := addScope("", "", defaultComponents); err != nil {
		return nil, err
	}
	for _, app := range appPrefixes {
		scopeComponents, ok := appComponents[app.name]
		if !ok {
			// Environment-only prefixes follow the default App; using the project union would invent capabilities owned by a sibling.
			scopeComponents = defaultComponents
		}
		if err := addScope(app.name, app.prefix, scopeComponents); err != nil {
			return nil, err
		}
	}
	return consumers, nil
}

type resourceAppPrefix struct {
	name   string
	prefix string
}

// configuredResourceAppNames returns deterministic named Apps whose runtime overlays must participate in discovery.
func configuredResourceAppNames(config *project.Config) []string {
	if config == nil {
		return nil
	}
	names := make([]string, 0, len(config.Apps))
	for name := range config.Apps {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || name == project.DefaultAppName {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// resourceEnvironmentAssignments parses final dotenv assignments using the renderer's existing precedence rules.
func resourceEnvironmentAssignments(source []byte) (map[string]string, error) {
	parsed, err := godotenv.Unmarshal(string(source))
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for key, value := range parsed {
		values[strings.ToUpper(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return values, nil
}

// resourceAppPrefixes combines configured Apps with prefixes evidenced by resource-specific owner keys.
func resourceAppPrefixes(values map[string]string, appNames []string) []resourceAppPrefix {
	prefixNames := map[string]string{}
	for _, appName := range appNames {
		appName = strings.ToLower(strings.TrimSpace(appName))
		if appName == "" || appName == project.DefaultAppName {
			continue
		}
		if prefix := strEnvPrefix(appName); prefix != "" {
			prefixNames[prefix] = appName
		}
	}
	for key := range values {
		markerIndex := -1
		var markerDefinition project.ResourceDefinition
		for _, definition := range project.ResourceCatalog() {
			marker := "_" + resourceEnvironmentPrefix(definition.Key) + "_"
			index := strings.Index(key, marker)
			if index > 0 && (markerIndex < 0 || index < markerIndex) {
				markerIndex = index
				markerDefinition = definition
			}
		}
		if markerIndex <= 0 || !resourceAppPrefixEvidence(key[markerIndex+1:], markerDefinition) {
			continue
		}
		prefix := key[:markerIndex]
		if _, exists := prefixNames[prefix]; exists {
			continue
		}
		if resourceFirstAppPrefix(prefix) {
			continue
		}
		prefixNames[prefix] = strings.ToLower(strings.ReplaceAll(prefix, "_", "-"))
	}
	prefixes := make([]resourceAppPrefix, 0, len(prefixNames))
	for prefix, name := range prefixNames {
		prefixes = append(prefixes, resourceAppPrefix{name: name, prefix: prefix})
	}
	sort.Slice(prefixes, func(left, right int) bool { return prefixes[left].prefix < prefixes[right].prefix })
	return prefixes
}

// resourceFirstAppPrefix keeps ordinary RESOURCE_<NAME> scopes out of inferred App topology while allowing configured Apps with the same slug.
func resourceFirstAppPrefix(prefix string) bool {
	for _, definition := range project.ResourceCatalog() {
		resourcePrefix := resourceEnvironmentPrefix(definition.Key)
		if prefix == resourcePrefix || strings.HasPrefix(prefix, resourcePrefix+"_") {
			return true
		}
	}
	return false
}

// resourceAppPrefixEvidence limits inference to keys that can change a resource's driver or endpoint topology.
func resourceAppPrefixEvidence(key string, definition project.ResourceDefinition) bool {
	resourcePrefix := resourceEnvironmentPrefix(definition.Key) + "_"
	if !strings.HasPrefix(key, resourcePrefix) {
		return false
	}
	suffix := strings.TrimPrefix(key, resourcePrefix)
	if suffix == "DRIVER" || suffix == "SUPPORTED_DRIVERS" || strings.HasSuffix(suffix, "_DRIVER") {
		return true
	}
	for _, endpoint := range definitionEndpointEnvironment(definition) {
		endpointSuffix := strings.TrimPrefix(strings.ToUpper(endpoint.Key), resourcePrefix)
		if endpointSuffix != "" && (suffix == endpointSuffix || strings.HasSuffix(suffix, "_"+endpointSuffix)) {
			return true
		}
	}
	switch definition.Key {
	case project.ResourceDatabase:
		return resourceSuffixMatches(suffix, "DSN", "HOST", "PORT")
	case project.ResourceCache, project.ResourceQueue, project.ResourceEvents, project.ResourceStorage:
		return resourceSuffixMatches(suffix, "ADDR")
	default:
		return false
	}
}

// definitionEndpointEnvironment returns metadata used for topology without changing rendered placeholder policy.
func definitionEndpointEnvironment(definition project.ResourceDefinition) []project.DriverEnvironmentPlaceholder {
	endpoints := []project.DriverEnvironmentPlaceholder{}
	for _, driver := range definition.Drivers {
		endpoints = append(endpoints, driver.EndpointEnvironment...)
		endpoints = append(endpoints, driver.Environment...)
	}
	return endpoints
}

// resourceSuffixMatches recognizes root and named endpoint keys without accepting unrelated feature flags.
func resourceSuffixMatches(value string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if value == suffix || strings.HasSuffix(value, "_"+suffix) {
			return true
		}
	}
	return false
}

// effectiveNamedResourceDrivers resolves generated and arbitrary named driver scopes for one App overlay.
func effectiveNamedResourceDrivers(values map[string]string, appPrefix string, plan project.ResourcePlan, components project.Components, resource project.ResourceKey, rootDriver string) map[string]string {
	drivers := map[string]string{}
	for _, named := range plan.GeneratedNamedSelections(components) {
		if named.Resource != resource {
			continue
		}
		fallback := named.Active
		if value, set := resourceOverlayValue(values, appPrefix, named.EnvironmentKey); set && strings.TrimSpace(value) != "" {
			fallback = value
		}
		drivers[strings.ToLower(named.Name)] = strings.ToLower(strings.TrimSpace(fallback))
	}

	prefix := resourceEnvironmentPrefix(resource) + "_"
	appMarker := ""
	if appPrefix != "" {
		appMarker = appPrefix + "_"
	}
	for key := range values {
		baseKey := key
		if appMarker != "" && strings.HasPrefix(baseKey, appMarker) {
			baseKey = strings.TrimPrefix(baseKey, appMarker)
		} else if appMarker != "" && !strings.HasPrefix(baseKey, prefix) {
			continue
		}
		if !strings.HasPrefix(baseKey, prefix) {
			continue
		}
		if !strings.HasSuffix(baseKey, "_DRIVER") {
			name, endpointScope := namedResourceEndpointScope(baseKey, resource)
			if endpointScope {
				if _, exists := drivers[name]; !exists {
					drivers[name] = namedResourceDefaultDriver(resource, rootDriver)
				}
			}
			continue
		}
		scopedKey := strings.TrimPrefix(baseKey, prefix)
		if scopedKey == "DRIVER" {
			continue
		}
		name := strings.TrimSuffix(scopedKey, "_DRIVER")
		if name == "" {
			continue
		}
		driverKey := prefix + name + "_DRIVER"
		driver, set := resourceOverlayValue(values, appPrefix, driverKey)
		if !set || strings.TrimSpace(driver) == "" {
			driver = namedResourceDefaultDriver(resource, rootDriver)
		}
		drivers[strings.ToLower(name)] = strings.ToLower(strings.TrimSpace(driver))
	}
	return drivers
}

// namedResourceEndpointScope extracts a named resource from endpoint-only configuration such as DB_REPORTING_HOST.
func namedResourceEndpointScope(key string, resource project.ResourceKey) (string, bool) {
	definition, ok := project.ResourceDefinitionByKey(resource)
	if !ok {
		return "", false
	}
	resourcePrefix := resourceEnvironmentPrefix(resource) + "_"
	scopedKey := strings.TrimPrefix(key, resourcePrefix)
	for _, suffix := range resourceEndpointSuffixes(definition) {
		marker := "_" + suffix
		if !strings.HasSuffix(scopedKey, marker) {
			continue
		}
		name := strings.TrimSuffix(scopedKey, marker)
		if name != "" {
			return strings.ToLower(name), true
		}
	}
	return "", false
}

// resourceEndpointSuffixes returns the endpoint keys that can distinguish a named resource connection.
func resourceEndpointSuffixes(definition project.ResourceDefinition) []string {
	suffixes := []string{}
	seen := map[string]bool{}
	add := func(suffix string) {
		suffix = strings.ToUpper(strings.TrimSpace(suffix))
		if suffix == "" || seen[suffix] {
			return
		}
		seen[suffix] = true
		suffixes = append(suffixes, suffix)
	}
	resourcePrefix := resourceEnvironmentPrefix(definition.Key) + "_"
	for _, endpoint := range definitionEndpointEnvironment(definition) {
		add(strings.TrimPrefix(strings.ToUpper(endpoint.Key), resourcePrefix))
	}
	switch definition.Key {
	case project.ResourceDatabase:
		add("DSN")
		add("HOST")
		add("PORT")
	case project.ResourceCache, project.ResourceQueue, project.ResourceEvents, project.ResourceStorage:
		add("ADDR")
	}
	sort.SliceStable(suffixes, func(left, right int) bool {
		return len(suffixes[left]) > len(suffixes[right])
	})
	return suffixes
}

// effectiveResourceDriver applies App overlay semantics before falling back to the generator's empty-driver default.
func effectiveResourceDriver(values map[string]string, appPrefix string, resource project.ResourceKey, name string, fallback string) string {
	key := resourceEnvironmentPrefix(resource)
	if name != "" {
		key += "_" + strings.ToUpper(name)
	}
	key += "_DRIVER"
	value, set := resourceOverlayValue(values, appPrefix, key)
	if !set {
		return strings.ToLower(strings.TrimSpace(fallback))
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value != "" {
		return value
	}
	if appPrefix != "" && name == "" {
		return rootResourceDefaultDriver(resource)
	}
	return namedResourceDefaultDriver(resource, fallback)
}

// rootResourceDefaultDriver mirrors each generated manager's fallback after a blank App overlay replaces the root value.
func rootResourceDefaultDriver(resource project.ResourceKey) string {
	switch resource {
	case project.ResourceDatabase:
		return "sqlite"
	case project.ResourceCache:
		return "memory"
	case project.ResourceQueue:
		return "workerpool"
	case project.ResourceEvents:
		return "inproc"
	case project.ResourceStorage:
		return "local"
	case project.ResourceMail:
		return "log"
	default:
		return ""
	}
}

// namedResourceDefaultDriver mirrors generator defaults when an explicit named driver is empty.
func namedResourceDefaultDriver(resource project.ResourceKey, rootDriver string) string {
	switch resource {
	case project.ResourceDatabase, project.ResourceQueue:
		return strings.ToLower(strings.TrimSpace(rootDriver))
	case project.ResourceCache:
		return "memory"
	case project.ResourceEvents:
		return "inproc"
	case project.ResourceStorage:
		return "local"
	case project.ResourceMail:
		return "log"
	default:
		return strings.ToLower(strings.TrimSpace(rootDriver))
	}
}

// effectiveResourceConsumer derives catalog service metadata and a credential-safe endpoint identity.
func effectiveResourceConsumer(values map[string]string, appPrefix string, resource project.ResourceKey, name string, consumerName string, driverName string, localDatabaseDriver string, components project.Components) (project.EffectiveResourceConsumer, error) {
	definition, ok := project.ResourceDefinitionByKey(resource)
	if !ok {
		return project.EffectiveResourceConsumer{}, fmt.Errorf("unknown resource %q", resource)
	}
	driverName = project.CanonicalResourceDriver(resource, driverName)
	driver, ok := definition.Driver(driverName)
	if !ok {
		return project.EffectiveResourceConsumer{}, fmt.Errorf("effective consumer %s selects unknown %s driver %q", consumerName, definition.Label, driverName)
	}
	affinity, local, err := resourceEndpointAffinity(values, appPrefix, resource, name, driver, localDatabaseDriver, components)
	if err != nil {
		return project.EffectiveResourceConsumer{}, fmt.Errorf("effective consumer %s: %w", consumerName, err)
	}
	return project.EffectiveResourceConsumer{
		Resource:         resource,
		Consumer:         consumerName,
		Driver:           driver.Name,
		EndpointAffinity: affinity,
		LocalService:     local,
	}, nil
}

// resourceEndpointAffinity mirrors endpoint fallback without exposing credentials in service plans.
func resourceEndpointAffinity(values map[string]string, appPrefix string, resource project.ResourceKey, name string, driver project.DriverDefinition, localDatabaseDriver string, components project.Components) (string, bool, error) {
	if driver.Service == "" {
		return "", false, nil
	}
	if driver.Service == project.ServiceRedis {
		addr := resourceScopedValue(values, appPrefix, resource, name, "ADDR")
		if addr == "" && resource == project.ResourceQueue && name != "" {
			addr = resourceScopedValue(values, appPrefix, resource, "", "ADDR")
		}
		if addr == "" {
			host, _ := resourceOverlayValue(values, appPrefix, "REDIS_HOST")
			port, _ := resourceOverlayValue(values, appPrefix, "REDIS_PORT")
			if strings.TrimSpace(host) == "" {
				host = "redis"
			}
			if strings.TrimSpace(port) == "" {
				port = "6379"
			}
			addr = strings.TrimSpace(host) + ":" + strings.TrimSpace(port)
		}
		addr = strings.ToLower(strings.TrimSpace(addr))
		if components.Docker && addr == "redis:6379" {
			return "", true, nil
		}
		return opaqueEndpointAffinity(driver.Service, []string{"addr=" + addr}), false, nil
	}

	if resource == project.ResourceDatabase {
		return databaseEndpointAffinity(values, appPrefix, name, driver, localDatabaseDriver, components)
	}

	if driver.Service == project.ServiceMailSMTP {
		host := resourceScopedValue(values, appPrefix, resource, name, "SMTP_HOST")
		port := resourceScopedValue(values, appPrefix, resource, name, "SMTP_PORT")
		if host == "" {
			host = "localhost"
			if components.Docker {
				host = "mailpit"
			}
		}
		if port == "" {
			port = "1025"
		}
		endpoint := strings.ToLower(strings.TrimSpace(host)) + ":" + strings.TrimSpace(port)
		if components.Docker && endpoint == "mailpit:1025" {
			return "", true, nil
		}
		return opaqueEndpointAffinity(driver.Service, []string{"addr=" + endpoint}), false, nil
	}

	parts := []string{}
	for _, placeholder := range driver.Environment {
		rootPrefix := resourceEnvironmentPrefix(resource) + "_"
		suffix := strings.TrimPrefix(strings.ToUpper(placeholder.Key), rootPrefix)
		if suffix == "" || strings.Contains(suffix, "PASSWORD") || strings.Contains(suffix, "SECRET") || strings.Contains(suffix, "TOKEN") || strings.Contains(suffix, "KEY") {
			continue
		}
		if value := resourceScopedValue(values, appPrefix, resource, name, suffix); value != "" {
			parts = append(parts, suffix+"="+value)
		}
	}
	if len(parts) == 0 {
		return "", false, nil
	}
	sort.Strings(parts)
	return opaqueEndpointAffinity(driver.Service, parts), false, nil
}

// databaseEndpointAffinity keeps Compose ownership tied to the root selected database engine.
func databaseEndpointAffinity(values map[string]string, appPrefix string, name string, driver project.DriverDefinition, localDatabaseDriver string, components project.Components) (string, bool, error) {
	defaultPort := "3306"
	if driver.Service == project.ServicePostgres {
		defaultPort = "5432"
	}
	localService := databaseDriverService(localDatabaseDriver)
	sameLocalEngine := components.Docker && driver.Service == localService
	rootConsumer := appPrefix == "" && name == ""
	if sameLocalEngine && rootConsumer {
		return "", true, nil
	}

	dsn, dsnSet := concreteDatabaseEndpointValue(values, appPrefix, name, "DSN")
	host, hostSet := concreteDatabaseEndpointValue(values, appPrefix, name, "HOST")
	if sameLocalEngine && !dsnSet && !hostSet {
		return "", true, nil
	}
	if components.Docker && !rootConsumer && driver.Service != localService && !dsnSet && !hostSet {
		return "", false, fmt.Errorf("database driver %s requires an explicit %s or %s because Compose provisions only the root %s database", driver.Name, databaseEndpointKey(appPrefix, name, "DSN"), databaseEndpointKey(appPrefix, name, "HOST"), localDatabaseDriver)
	}
	if dsnSet {
		return opaqueEndpointAffinity(driver.Service, []string{"dsn=" + dsn}), false, nil
	}
	if !hostSet {
		host = resourceScopedValue(values, appPrefix, project.ResourceDatabase, name, "HOST")
	}
	port, portSet := concreteDatabaseEndpointValue(values, appPrefix, name, "PORT")
	if !portSet && (!rootConsumer || hostSet) {
		port = defaultPort
	}
	if port == "" {
		port = resourceScopedValue(values, appPrefix, project.ResourceDatabase, name, "PORT")
	}
	if host == "" {
		host = string(driver.Service)
	}
	if port == "" {
		port = defaultPort
	}
	endpoint := strings.ToLower(strings.TrimSpace(host)) + ":" + strings.TrimSpace(port)
	if sameLocalEngine && endpoint == string(driver.Service)+":"+defaultPort {
		return "", true, nil
	}
	return opaqueEndpointAffinity(driver.Service, []string{"addr=" + endpoint}), false, nil
}

// databaseDriverService returns the infrastructure identity for a built-in database driver.
func databaseDriverService(driverName string) project.ServiceKey {
	definition, ok := project.ResourceDefinitionByKey(project.ResourceDatabase)
	if !ok {
		return ""
	}
	driver, ok := definition.Driver(driverName)
	if !ok {
		return ""
	}
	return driver.Service
}

// concreteDatabaseEndpointValue reads only endpoint keys owned by the App or named connection being evaluated.
func concreteDatabaseEndpointValue(values map[string]string, appPrefix string, name string, suffix string) (string, bool) {
	key := databaseEndpointKey("", name, suffix)
	if appPrefix != "" {
		if value := strings.TrimSpace(values[databaseEndpointKey(appPrefix, name, suffix)]); value != "" {
			return value, true
		}
		if name == "" {
			return "", false
		}
	}
	value := strings.TrimSpace(values[key])
	return value, value != ""
}

// databaseEndpointKey formats a root, named, or App-prefixed database endpoint key.
func databaseEndpointKey(appPrefix string, name string, suffix string) string {
	parts := []string{}
	if appPrefix != "" {
		parts = append(parts, strings.ToUpper(appPrefix))
	}
	parts = append(parts, "DB")
	if name != "" {
		parts = append(parts, strings.ToUpper(name))
	}
	parts = append(parts, strings.ToUpper(suffix))
	return strings.Join(parts, "_")
}

// resourceScopedValue reads a root or named key through one App overlay.
func resourceScopedValue(values map[string]string, appPrefix string, resource project.ResourceKey, name string, suffix string) string {
	key := resourceEnvironmentPrefix(resource)
	if name != "" {
		key += "_" + strings.ToUpper(name)
	}
	key += "_" + strings.ToUpper(suffix)
	value, _ := resourceOverlayValue(values, appPrefix, key)
	return strings.TrimSpace(value)
}

// resourceOverlayValue applies the generated runtime's App-prefixed override precedence.
func resourceOverlayValue(values map[string]string, appPrefix string, key string) (string, bool) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if appPrefix != "" {
		if value, ok := values[appPrefix+"_"+key]; ok {
			return value, true
		}
	}
	value, ok := values[key]
	return value, ok
}

// resourceEnvironmentPrefix maps catalog keys to their generated dotenv scope.
func resourceEnvironmentPrefix(resource project.ResourceKey) string {
	if resource == project.ResourceDatabase {
		return "DB"
	}
	return strings.ToUpper(string(resource))
}

// effectiveResourceConsumerName formats stable identities shared with the pure service planner.
func effectiveResourceConsumerName(appName string, resource project.ResourceKey, name string) string {
	parts := []string{}
	if appName != "" && appName != project.DefaultAppName {
		parts = append(parts, strings.ToLower(appName))
	}
	parts = append(parts, string(resource))
	if name != "" {
		parts = append(parts, strings.ToLower(name))
	}
	return strings.Join(parts, ":")
}

// appendEffectiveResourceConsumer replaces duplicate identities discovered through base and App-specific keys.
func appendEffectiveResourceConsumer(consumers []project.EffectiveResourceConsumer, seen map[string]bool, consumer project.EffectiveResourceConsumer) []project.EffectiveResourceConsumer {
	if seen[consumer.Consumer] {
		for index := range consumers {
			if consumers[index].Consumer == consumer.Consumer {
				consumers[index] = consumer
				return consumers
			}
		}
	}
	seen[consumer.Consumer] = true
	return append(consumers, consumer)
}

// opaqueEndpointAffinity hashes endpoint material so service identity never publishes DSN credentials or account details.
func opaqueEndpointAffinity(service project.ServiceKey, parts []string) string {
	sum := sha256.Sum256([]byte(string(service) + "\x00" + strings.Join(parts, "\x00")))
	return fmt.Sprintf("%s:%x", service, sum[:8])
}

package resourceenv

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/goforj/goforj/project"
	"github.com/joho/godotenv"
)

// resourceConsumerResolver keeps the project-wide inputs together because every App scope must use the same plan and service placement policy.
type resourceConsumerResolver struct {
	values                map[string]string
	plan                  project.ResourcePlan
	projectComponents     project.Components
	projectDatabaseDriver string
}

// resourceConsumerScope describes the App-local component and environment overlay evaluated against the project-wide resource plan.
type resourceConsumerScope struct {
	app        resourceAppPrefix
	components project.Components
}

// resourceConnection identifies one root or named connection without repeatedly passing its catalog metadata and name separately.
type resourceConnection struct {
	definition project.ResourceDefinition
	name       string
}

// resourceConnectionSelection couples a connection with the driver selected for its current App scope.
type resourceConnectionSelection struct {
	connection resourceConnection
	driverName string
}

// resourceEndpointResolution keeps endpoint identity and local ownership coupled so callers cannot accidentally discard one half of the placement decision.
type resourceEndpointResolution struct {
	affinity string
	local    bool
}

// ResolveConsumers applies each configured App's actual participation to resource discovery.
func ResolveConsumers(source []byte, plan project.ResourcePlan, projectComponents project.Components, config *project.Config) ([]project.EffectiveResourceConsumer, error) {
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
	resolver, err := newResourceConsumerResolver(source, plan, projectComponents)
	if err != nil {
		return nil, err
	}
	return resolver.resolve(defaultComponents, appNames, appComponents)
}

// newResourceConsumerResolver parses the owner environment once so every App scope observes the same resolved dotenv assignments.
func newResourceConsumerResolver(source []byte, plan project.ResourcePlan, projectComponents project.Components) (resourceConsumerResolver, error) {
	values, err := resourceEnvironmentAssignments(source)
	if err != nil {
		return resourceConsumerResolver{}, fmt.Errorf("parse effective resource environment: %w", err)
	}
	databaseSelection, _ := plan.Selection(project.ResourceDatabase)
	return resourceConsumerResolver{
		values:                values,
		plan:                  plan,
		projectComponents:     projectComponents,
		projectDatabaseDriver: databaseSelection.Active,
	}, nil
}

// resolve keeps applicability App-local while retaining project-wide service placement.
func (resolver resourceConsumerResolver) resolve(defaultComponents project.Components, appNames []string, appComponents map[string]project.Components) ([]project.EffectiveResourceConsumer, error) {
	appPrefixes := resourceAppPrefixes(resolver.values, appNames)
	consumers := []project.EffectiveResourceConsumer{}
	seen := map[string]bool{}

	defaultScope := resourceConsumerScope{components: defaultComponents}
	resolved, err := resolver.resolveScope(defaultScope)
	if err != nil {
		return nil, err
	}
	for _, consumer := range resolved {
		consumers = appendEffectiveResourceConsumer(consumers, seen, consumer)
	}
	for _, app := range appPrefixes {
		scopeComponents, ok := appComponents[app.name]
		if !ok {
			// Environment-only prefixes follow the default App; using the project union would invent capabilities owned by a sibling.
			scopeComponents = defaultComponents
		}
		resolved, err := resolver.resolveScope(resourceConsumerScope{app: app, components: scopeComponents})
		if err != nil {
			return nil, err
		}
		for _, consumer := range resolved {
			consumers = appendEffectiveResourceConsumer(consumers, seen, consumer)
		}
	}
	return consumers, nil
}

// resolveScope evaluates only resources enabled for one App while using the shared project plan to determine supported drivers.
func (resolver resourceConsumerResolver) resolveScope(scope resourceConsumerScope) ([]project.EffectiveResourceConsumer, error) {
	consumers := []project.EffectiveResourceConsumer{}
	for _, definition := range project.ResourceCatalog() {
		if !definition.AppliesTo(scope.components) {
			continue
		}
		selection, ok := resolver.plan.Selection(definition.Key)
		if !ok {
			continue
		}

		rootConnection := resourceConnection{definition: definition}
		rootDriver := resolver.effectiveDriver(scope, rootConnection, selection.Active)
		consumer, err := resolver.resolveConsumer(scope, resourceConnectionSelection{
			connection: rootConnection,
			driverName: rootDriver,
		})
		if err != nil {
			return nil, err
		}
		consumers = append(consumers, consumer)

		namedDrivers := resolver.namedDrivers(scope, rootConnection, rootDriver)
		names := make([]string, 0, len(namedDrivers))
		for name := range namedDrivers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			consumer, err := resolver.resolveConsumer(scope, resourceConnectionSelection{
				connection: resourceConnection{
					definition: definition,
					name:       name,
				},
				driverName: namedDrivers[name],
			})
			if err != nil {
				return nil, err
			}
			consumers = append(consumers, consumer)
		}
	}
	return consumers, nil
}

// resourceAppPrefix links a normalized App identity to the uppercase prefix used by generated environment overlays.
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
		if prefix := project.AppEnvironmentPrefix(appName); prefix != "" {
			prefixNames[prefix] = appName
		}
	}
	for key := range values {
		markerIndex := -1
		var markerDefinition project.ResourceDefinition
		for _, definition := range project.ResourceCatalog() {
			marker := "_" + definition.EnvironmentPrefix + "_"
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
		resourcePrefix := definition.EnvironmentPrefix
		if prefix == resourcePrefix || strings.HasPrefix(prefix, resourcePrefix+"_") {
			return true
		}
	}
	return false
}

// resourceAppPrefixEvidence limits inference to keys that can change a resource's driver or endpoint topology.
func resourceAppPrefixEvidence(key string, definition project.ResourceDefinition) bool {
	resourcePrefix := definition.EnvironmentPrefix + "_"
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

// namedDrivers resolves generated and arbitrary named connections through one App overlay.
func (resolver resourceConsumerResolver) namedDrivers(scope resourceConsumerScope, root resourceConnection, rootDriver string) map[string]string {
	definition := root.definition
	resource := definition.Key
	drivers := map[string]string{}
	for _, named := range resolver.plan.GeneratedNamedSelections(scope.components) {
		if named.Resource != resource {
			continue
		}
		fallback := named.Active
		if value, set := resolver.overlayValue(scope, named.EnvironmentKey); set && strings.TrimSpace(value) != "" {
			fallback = value
		}
		drivers[strings.ToLower(named.Name)] = strings.ToLower(strings.TrimSpace(fallback))
	}

	prefix := definition.EnvironmentPrefix + "_"
	appMarker := ""
	if scope.app.prefix != "" {
		appMarker = scope.app.prefix + "_"
	}
	for key := range resolver.values {
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
					drivers[name] = definition.NamedDriverDefault(rootDriver)
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
		driver, set := resolver.overlayValue(scope, driverKey)
		if !set || strings.TrimSpace(driver) == "" {
			driver = definition.NamedDriverDefault(rootDriver)
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
	resourcePrefix := definition.EnvironmentPrefix + "_"
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
	resourcePrefix := definition.EnvironmentPrefix + "_"
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

// effectiveDriver applies App overlay semantics before falling back to the generator's empty-driver default.
func (resolver resourceConsumerResolver) effectiveDriver(scope resourceConsumerScope, connection resourceConnection, fallback string) string {
	key := connection.definition.EnvironmentPrefix
	if connection.name != "" {
		key += "_" + strings.ToUpper(connection.name)
	}
	key += "_DRIVER"
	value, set := resolver.overlayValue(scope, key)
	if !set {
		return strings.ToLower(strings.TrimSpace(fallback))
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value != "" {
		return value
	}
	if scope.app.prefix != "" && connection.name == "" {
		return connection.definition.DefaultDriver
	}
	return connection.definition.NamedDriverDefault(fallback)
}

// resolveConsumer derives catalog service metadata and a credential-safe endpoint identity for one App-local connection.
func (resolver resourceConsumerResolver) resolveConsumer(scope resourceConsumerScope, selection resourceConnectionSelection) (project.EffectiveResourceConsumer, error) {
	connection := selection.connection
	consumerName := connection.consumerName(scope)
	driverName := project.CanonicalResourceDriver(connection.definition.Key, selection.driverName)
	driver, ok := connection.definition.Driver(driverName)
	if !ok {
		return project.EffectiveResourceConsumer{}, fmt.Errorf("effective consumer %s selects unknown %s driver %q", consumerName, connection.definition.Label, driverName)
	}
	endpoint, err := resolver.resolveEndpoint(scope, connection, driver)
	if err != nil {
		return project.EffectiveResourceConsumer{}, fmt.Errorf("effective consumer %s: %w", consumerName, err)
	}
	return project.EffectiveResourceConsumer{
		Resource:         connection.definition.Key,
		Consumer:         consumerName,
		Driver:           driver.Name,
		EndpointAffinity: endpoint.affinity,
		LocalService:     endpoint.local,
	}, nil
}

// resolveEndpoint mirrors endpoint fallback without exposing credentials in service plans.
func (resolver resourceConsumerResolver) resolveEndpoint(scope resourceConsumerScope, connection resourceConnection, driver project.DriverDefinition) (resourceEndpointResolution, error) {
	resource := connection.definition.Key
	if driver.Service == "" {
		return resourceEndpointResolution{}, nil
	}
	if driver.Service == project.ServiceRedis {
		addr := resolver.scopedValue(scope, connection, "ADDR")
		if addr == "" && resource == project.ResourceQueue && connection.name != "" {
			rootConnection := connection
			rootConnection.name = ""
			addr = resolver.scopedValue(scope, rootConnection, "ADDR")
		}
		if addr == "" {
			host, _ := resolver.overlayValue(scope, "REDIS_HOST")
			port, _ := resolver.overlayValue(scope, "REDIS_PORT")
			if strings.TrimSpace(host) == "" {
				host = "redis"
			}
			if strings.TrimSpace(port) == "" {
				port = "6379"
			}
			addr = strings.TrimSpace(host) + ":" + strings.TrimSpace(port)
		}
		addr = strings.ToLower(strings.TrimSpace(addr))
		if resolver.projectComponents.Docker && addr == "redis:6379" {
			return resourceEndpointResolution{local: true}, nil
		}
		return resourceEndpointResolution{affinity: opaqueEndpointAffinity(driver.Service, []string{"addr=" + addr})}, nil
	}

	if resource == project.ResourceDatabase {
		return resolver.resolveDatabaseEndpoint(scope, connection, driver)
	}

	if driver.Service == project.ServiceMailSMTP {
		host := resolver.scopedValue(scope, connection, "SMTP_HOST")
		port := resolver.scopedValue(scope, connection, "SMTP_PORT")
		if host == "" {
			host = "localhost"
			if resolver.projectComponents.Docker {
				host = "mailpit"
			}
		}
		if port == "" {
			port = "1025"
		}
		endpoint := strings.ToLower(strings.TrimSpace(host)) + ":" + strings.TrimSpace(port)
		if resolver.projectComponents.Docker && endpoint == "mailpit:1025" {
			return resourceEndpointResolution{local: true}, nil
		}
		return resourceEndpointResolution{affinity: opaqueEndpointAffinity(driver.Service, []string{"addr=" + endpoint})}, nil
	}

	parts := []string{}
	for _, placeholder := range driver.Environment {
		rootPrefix := connection.definition.EnvironmentPrefix + "_"
		suffix := strings.TrimPrefix(strings.ToUpper(placeholder.Key), rootPrefix)
		if suffix == "" || strings.Contains(suffix, "PASSWORD") || strings.Contains(suffix, "SECRET") || strings.Contains(suffix, "TOKEN") || strings.Contains(suffix, "KEY") {
			continue
		}
		if value := resolver.scopedValue(scope, connection, suffix); value != "" {
			parts = append(parts, suffix+"="+value)
		}
	}
	if len(parts) == 0 {
		return resourceEndpointResolution{}, nil
	}
	sort.Strings(parts)
	return resourceEndpointResolution{affinity: opaqueEndpointAffinity(driver.Service, parts)}, nil
}

// resolveDatabaseEndpoint keeps Compose ownership tied to the root selected database engine.
func (resolver resourceConsumerResolver) resolveDatabaseEndpoint(scope resourceConsumerScope, connection resourceConnection, driver project.DriverDefinition) (resourceEndpointResolution, error) {
	defaultPort := "3306"
	if driver.Service == project.ServicePostgres {
		defaultPort = "5432"
	}
	localService := databaseDriverService(resolver.projectDatabaseDriver)
	sameLocalEngine := resolver.projectComponents.Docker && driver.Service == localService
	rootConsumer := scope.app.prefix == "" && connection.name == ""
	if sameLocalEngine && rootConsumer {
		return resourceEndpointResolution{local: true}, nil
	}

	dsn, dsnSet := resolver.concreteDatabaseEndpointValue(scope, connection, "DSN")
	host, hostSet := resolver.concreteDatabaseEndpointValue(scope, connection, "HOST")
	if sameLocalEngine && !dsnSet && !hostSet {
		return resourceEndpointResolution{local: true}, nil
	}
	if resolver.projectComponents.Docker && !rootConsumer && driver.Service != localService && !dsnSet && !hostSet {
		return resourceEndpointResolution{}, fmt.Errorf("database driver %s requires an explicit %s or %s because Compose provisions only the root %s database", driver.Name, connection.databaseEndpointKey(scope.app.prefix, "DSN"), connection.databaseEndpointKey(scope.app.prefix, "HOST"), resolver.projectDatabaseDriver)
	}
	if dsnSet {
		return resourceEndpointResolution{affinity: opaqueEndpointAffinity(driver.Service, []string{"dsn=" + dsn})}, nil
	}
	if !hostSet {
		host = resolver.scopedValue(scope, connection, "HOST")
	}
	port, portSet := resolver.concreteDatabaseEndpointValue(scope, connection, "PORT")
	if !portSet && (!rootConsumer || hostSet) {
		port = defaultPort
	}
	if port == "" {
		port = resolver.scopedValue(scope, connection, "PORT")
	}
	if host == "" {
		host = string(driver.Service)
	}
	if port == "" {
		port = defaultPort
	}
	endpoint := strings.ToLower(strings.TrimSpace(host)) + ":" + strings.TrimSpace(port)
	if sameLocalEngine && endpoint == string(driver.Service)+":"+defaultPort {
		return resourceEndpointResolution{local: true}, nil
	}
	return resourceEndpointResolution{affinity: opaqueEndpointAffinity(driver.Service, []string{"addr=" + endpoint})}, nil
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
func (resolver resourceConsumerResolver) concreteDatabaseEndpointValue(scope resourceConsumerScope, connection resourceConnection, suffix string) (string, bool) {
	key := connection.databaseEndpointKey("", suffix)
	if scope.app.prefix != "" {
		if value := strings.TrimSpace(resolver.values[connection.databaseEndpointKey(scope.app.prefix, suffix)]); value != "" {
			return value, true
		}
		if connection.name == "" {
			return "", false
		}
	}
	value := strings.TrimSpace(resolver.values[key])
	return value, value != ""
}

// databaseEndpointKey formats a root, named, or App-prefixed database endpoint key for this connection.
func (connection resourceConnection) databaseEndpointKey(appPrefix string, suffix string) string {
	parts := []string{}
	if appPrefix != "" {
		parts = append(parts, strings.ToUpper(appPrefix))
	}
	parts = append(parts, "DB")
	if connection.name != "" {
		parts = append(parts, strings.ToUpper(connection.name))
	}
	parts = append(parts, strings.ToUpper(suffix))
	return strings.Join(parts, "_")
}

// scopedValue reads a root or named key through one App overlay.
func (resolver resourceConsumerResolver) scopedValue(scope resourceConsumerScope, connection resourceConnection, suffix string) string {
	key := connection.definition.EnvironmentPrefix
	if connection.name != "" {
		key += "_" + strings.ToUpper(connection.name)
	}
	key += "_" + strings.ToUpper(suffix)
	value, _ := resolver.overlayValue(scope, key)
	return strings.TrimSpace(value)
}

// overlayValue applies the generated runtime's App-prefixed override precedence.
func (resolver resourceConsumerResolver) overlayValue(scope resourceConsumerScope, key string) (string, bool) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if scope.app.prefix != "" {
		if value, ok := resolver.values[scope.app.prefix+"_"+key]; ok {
			return value, true
		}
	}
	value, ok := resolver.values[key]
	return value, ok
}

// consumerName formats the stable identity shared with the pure service planner.
func (connection resourceConnection) consumerName(scope resourceConsumerScope) string {
	parts := []string{}
	if scope.app.name != "" && scope.app.name != project.DefaultAppName {
		parts = append(parts, strings.ToLower(scope.app.name))
	}
	parts = append(parts, string(connection.definition.Key))
	if connection.name != "" {
		parts = append(parts, strings.ToLower(connection.name))
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

package project

import (
	"fmt"
	"sort"
)

// ServiceState describes how a generated project satisfies one infrastructure requirement.
type ServiceState string

const (
	// ServiceStateActiveLocal means GoForj starts a generated local service consumed by the App.
	ServiceStateActiveLocal ServiceState = "active_local"
	// ServiceStateAvailableLocal means GoForj emits an optional local service for a supported transition.
	ServiceStateAvailableLocal ServiceState = "available_local"
	// ServiceStateLocalRequestedUnused means explicit owner intent starts a local service without an active consumer.
	ServiceStateLocalRequestedUnused ServiceState = "local_requested_unused"
	// ServiceStateExternalRequired means an active App resource depends on infrastructure GoForj does not manage locally.
	ServiceStateExternalRequired ServiceState = "external_required"
)

// ServiceRequirement describes one deduplicated infrastructure service and the resources that led to it.
type ServiceRequirement struct {
	Key              ServiceKey
	Label            string
	State            ServiceState
	EndpointAffinity string
	ActiveConsumers  []string
}

// EffectiveResourceConsumer describes one environment-resolved root, named, or App-scoped resource consumer.
type EffectiveResourceConsumer struct {
	// Resource identifies the catalog whose driver metadata applies.
	Resource ResourceKey
	// Consumer is the stable display identity, such as cache:reports or billing:cache.
	Consumer string
	// Driver is the effective active driver after environment overlay resolution.
	Driver string
	// EndpointAffinity is an opaque identity shared only by consumers mapped to the same endpoint.
	EndpointAffinity string
	// LocalService reports whether the endpoint maps to GoForj's generated local service.
	LocalService bool
}

// ServicePlan contains deterministic App infrastructure requirements derived from effective resource drivers.
type ServicePlan struct {
	Requirements []ServiceRequirement
}

// ResolveServicePlan derives App infrastructure without allowing service state to drift from resource selections.
func ResolveServicePlan(resourcePlan ResourcePlan, components Components, intent LocalServiceIntent) (ServicePlan, error) {
	return ResolveServicePlanWithConsumers(resourcePlan, components, intent, nil)
}

// ResolveServicePlanWithConsumers derives infrastructure while applying environment-resolved consumer overrides.
func ResolveServicePlanWithConsumers(resourcePlan ResourcePlan, components Components, intent LocalServiceIntent, consumers []EffectiveResourceConsumer) (ServicePlan, error) {
	normalized, err := resourcePlan.Normalized(components)
	if err != nil {
		return ServicePlan{}, fmt.Errorf("normalize resource plan: %w", err)
	}
	if err := validateLocalServiceIntent(intent); err != nil {
		return ServicePlan{}, err
	}

	discoveries := discoverServiceConsumers(normalized, components)
	if err := applyEffectiveServiceConsumers(discoveries, normalized, components, consumers); err != nil {
		return ServicePlan{}, err
	}
	identities := make([]serviceIdentity, 0, len(discoveries))
	for identity := range discoveries {
		identities = append(identities, identity)
	}
	sort.SliceStable(identities, func(left, right int) bool {
		leftOrder := serviceKeyOrder(identities[left].key)
		rightOrder := serviceKeyOrder(identities[right].key)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if identities[left].key != identities[right].key {
			return identities[left].key < identities[right].key
		}
		return identities[left].endpointAffinity < identities[right].endpointAffinity
	})

	servicePlan := ServicePlan{Requirements: make([]ServiceRequirement, 0, len(identities))}
	for _, identity := range identities {
		discovery := discoveries[identity]
		state, include := resolveServiceState(identity, discovery, components, intent)
		if !include {
			continue
		}
		servicePlan.Requirements = append(servicePlan.Requirements, ServiceRequirement{
			Key:              identity.key,
			Label:            discovery.label,
			State:            state,
			EndpointAffinity: identity.endpointAffinity,
			ActiveConsumers:  append([]string(nil), discovery.activeConsumers...),
		})
	}
	return servicePlan, nil
}

// Requirement returns a defensive copy of one service requirement.
func (p ServicePlan) Requirement(key ServiceKey) (ServiceRequirement, bool) {
	for _, requirement := range p.Requirements {
		if requirement.Key != key {
			continue
		}
		return cloneServiceRequirement(requirement), true
	}
	return ServiceRequirement{}, false
}

// RequirementsFor returns every endpoint-specific requirement for one service key.
func (p ServicePlan) RequirementsFor(key ServiceKey) []ServiceRequirement {
	requirements := []ServiceRequirement{}
	for _, requirement := range p.Requirements {
		if requirement.Key == key {
			requirements = append(requirements, cloneServiceRequirement(requirement))
		}
	}
	return requirements
}

// HasActiveLocal reports whether the default development workflow must honor any selected local-service lifecycle.
func (p ServicePlan) HasActiveLocal() bool {
	for _, requirement := range p.Requirements {
		if requirement.State == ServiceStateActiveLocal || requirement.State == ServiceStateLocalRequestedUnused {
			return true
		}
	}
	return false
}

// RequirementsInState returns defensive copies of requirements matching one operational state.
func (p ServicePlan) RequirementsInState(state ServiceState) []ServiceRequirement {
	requirements := []ServiceRequirement{}
	for _, requirement := range p.Requirements {
		if requirement.State == state {
			requirements = append(requirements, cloneServiceRequirement(requirement))
		}
	}
	return requirements
}

// serviceDiscovery keeps driver discovery separate from the placement policy applied afterward.
type serviceDiscovery struct {
	label                string
	locallyProvisionable bool
	localService         bool
	activeConsumers      []string
	supportedConsumers   []string
}

// serviceIdentity prevents provider-name equality from conflating distinct effective endpoints.
type serviceIdentity struct {
	key              ServiceKey
	endpointAffinity string
}

// discoverServiceConsumers gathers root and generated named resources before deduplicating by service key.
func discoverServiceConsumers(resourcePlan ResourcePlan, components Components) map[serviceIdentity]serviceDiscovery {
	discoveries := map[serviceIdentity]serviceDiscovery{}
	for _, definition := range ResourceCatalog() {
		if !definition.AppliesTo(components) {
			continue
		}
		selection, ok := resourcePlan.Selection(definition.Key)
		if !ok {
			continue
		}
		activeDriver, ok := definition.Driver(selection.Active)
		if ok && activeDriver.Service != "" {
			identity := serviceIdentity{key: activeDriver.Service}
			discovery := discoveries[identity]
			discovery.label = firstServiceLabel(discovery.label, activeDriver.ServiceLabel, activeDriver.Service)
			discovery.locallyProvisionable = discovery.locallyProvisionable || activeDriver.LocallyProvisionable
			discovery.activeConsumers = appendUniqueConsumer(discovery.activeConsumers, string(definition.Key))
			discoveries[identity] = discovery
		}
		for _, supported := range selection.Supported {
			driver, driverExists := definition.Driver(supported)
			if !driverExists || driver.Service == "" || !driver.LocallyProvisionable {
				continue
			}
			identity := serviceIdentity{key: driver.Service}
			discovery := discoveries[identity]
			discovery.label = firstServiceLabel(discovery.label, driver.ServiceLabel, driver.Service)
			discovery.locallyProvisionable = discovery.locallyProvisionable || driver.LocallyProvisionable
			discovery.supportedConsumers = appendUniqueConsumer(discovery.supportedConsumers, string(definition.Key))
			discoveries[identity] = discovery
		}
	}

	for _, named := range resourcePlan.GeneratedNamedSelections(components) {
		definition, ok := ResourceDefinitionByKey(named.Resource)
		if !ok {
			continue
		}
		driver, ok := definition.Driver(named.Active)
		if !ok || driver.Service == "" {
			continue
		}
		identity := serviceIdentity{key: driver.Service}
		discovery := discoveries[identity]
		discovery.label = firstServiceLabel(discovery.label, driver.ServiceLabel, driver.Service)
		discovery.locallyProvisionable = discovery.locallyProvisionable || driver.LocallyProvisionable
		consumer := fmt.Sprintf("%s:%s", named.Resource, named.Name)
		discovery.activeConsumers = appendUniqueConsumer(discovery.activeConsumers, consumer)
		discoveries[identity] = discovery
	}
	return discoveries
}

// applyEffectiveServiceConsumers replaces logical consumers with environment-resolved driver and endpoint identities.
func applyEffectiveServiceConsumers(discoveries map[serviceIdentity]serviceDiscovery, resourcePlan ResourcePlan, components Components, consumers []EffectiveResourceConsumer) error {
	for _, consumer := range consumers {
		consumer.Consumer = normalizeConsumerName(consumer.Consumer)
		if consumer.Consumer == "" {
			return fmt.Errorf("effective resource consumer requires a stable name")
		}
		definition, ok := ResourceDefinitionByKey(consumer.Resource)
		if !ok || !definition.AppliesTo(components) {
			return fmt.Errorf("effective consumer %s refers to unavailable resource %q", consumer.Consumer, consumer.Resource)
		}
		driverName := normalizeDriverName(consumer.Driver)
		driver, ok := definition.Driver(driverName)
		if !ok {
			return fmt.Errorf("effective consumer %s selects unknown %s driver %q", consumer.Consumer, definition.Label, consumer.Driver)
		}
		selection, selected := resourcePlan.Selection(consumer.Resource)
		if !selected || !containsDriver(selection.Supported, driverName) {
			return fmt.Errorf("effective consumer %s selects %s driver %q that is not built into the App", consumer.Consumer, definition.Label, driverName)
		}
		removeActiveServiceConsumer(discoveries, consumer.Consumer)
		if driver.Service == "" {
			continue
		}
		if consumer.LocalService && !driver.LocallyProvisionable {
			return fmt.Errorf("effective consumer %s maps non-provisionable service %s to a local service", consumer.Consumer, driver.Service)
		}
		identity := serviceIdentity{key: driver.Service, endpointAffinity: consumer.EndpointAffinity}
		discovery := discoveries[identity]
		discovery.label = firstServiceLabel(discovery.label, driver.ServiceLabel, driver.Service)
		discovery.locallyProvisionable = discovery.locallyProvisionable || driver.LocallyProvisionable
		discovery.localService = discovery.localService || consumer.LocalService
		discovery.activeConsumers = appendUniqueConsumer(discovery.activeConsumers, consumer.Consumer)
		discoveries[identity] = discovery
	}
	return nil
}

// removeActiveServiceConsumer removes a logical consumer before its effective endpoint is inserted.
func removeActiveServiceConsumer(discoveries map[serviceIdentity]serviceDiscovery, consumer string) {
	for identity, discovery := range discoveries {
		discovery.activeConsumers = removeConsumer(discovery.activeConsumers, consumer)
		if len(discovery.activeConsumers) == 0 && len(discovery.supportedConsumers) == 0 {
			delete(discoveries, identity)
			continue
		}
		discoveries[identity] = discovery
	}
}

// removeConsumer returns a copy without one exact consumer identity.
func removeConsumer(consumers []string, remove string) []string {
	kept := make([]string, 0, len(consumers))
	for _, consumer := range consumers {
		if consumer != remove {
			kept = append(kept, consumer)
		}
	}
	return kept
}

// normalizeConsumerName keeps endpoint grouping stable across callers.
func normalizeConsumerName(consumer string) string {
	return normalizeDriverName(consumer)
}

// resolveServiceState applies current local-management policy after all consumers have been deduplicated.
func resolveServiceState(identity serviceIdentity, discovery serviceDiscovery, components Components, intent LocalServiceIntent) (ServiceState, bool) {
	key := identity.key
	hasActiveConsumer := len(discovery.activeConsumers) > 0
	if key == ServiceMailSMTP {
		if !hasActiveConsumer || (components.Docker && (discovery.localService || identity.endpointAffinity == "")) {
			return "", false
		}
		return ServiceStateExternalRequired, true
	}
	if key == ServiceMySQL || key == ServicePostgres {
		if !hasActiveConsumer {
			return "", false
		}
		if components.Docker && (discovery.localService || identity.endpointAffinity == "") {
			return ServiceStateActiveLocal, true
		}
		return ServiceStateExternalRequired, true
	}

	mode, modeSelected := intent.Mode(key)
	if hasActiveConsumer {
		mapsToLocalService := discovery.localService || identity.endpointAffinity == ""
		if mapsToLocalService && discovery.locallyProvisionable && components.Docker && modeSelected && mode == LocalServiceModeLocal {
			return ServiceStateActiveLocal, true
		}
		return ServiceStateExternalRequired, true
	}
	if len(discovery.supportedConsumers) == 0 || !components.Docker {
		return "", false
	}
	if modeSelected && mode == LocalServiceModeLocal {
		return ServiceStateLocalRequestedUnused, true
	}
	return ServiceStateAvailableLocal, true
}

// validateLocalServiceIntent rejects malformed transient state before it can produce misleading service output.
func validateLocalServiceIntent(intent LocalServiceIntent) error {
	for key, mode := range intent.Modes {
		if !knownServiceKey(key) {
			return fmt.Errorf("local-service intent contains unknown service %q", key)
		}
		if mode != LocalServiceModeLocal && mode != LocalServiceModeExternal {
			return fmt.Errorf("service %s has unknown local-service mode %q", key, mode)
		}
	}
	return nil
}

// knownServiceKey reports whether the catalog publishes a driver backed by the supplied service identity.
func knownServiceKey(key ServiceKey) bool {
	for _, definition := range ResourceCatalog() {
		for _, driver := range definition.Drivers {
			if driver.Service == key {
				return true
			}
		}
	}
	return false
}

// serviceKeyOrder keeps confirmation and generated service decisions stable across map iteration.
func serviceKeyOrder(key ServiceKey) int {
	switch key {
	case ServiceMySQL:
		return 10
	case ServicePostgres:
		return 20
	case ServiceRedis:
		return 30
	default:
		return 100
	}
}

// appendUniqueConsumer preserves catalog discovery order while deduplicating repeated service consumers.
func appendUniqueConsumer(consumers []string, consumer string) []string {
	for _, existing := range consumers {
		if existing == consumer {
			return consumers
		}
	}
	return append(consumers, consumer)
}

// cloneServiceRequirement prevents callers from mutating consumer slices stored in the service plan.
func cloneServiceRequirement(requirement ServiceRequirement) ServiceRequirement {
	cloned := requirement
	cloned.ActiveConsumers = append([]string(nil), requirement.ActiveConsumers...)
	return cloned
}

// firstServiceLabel retains the first catalog label when a deliberately shared service gains more consumers.
func firstServiceLabel(existing string, candidate string, key ServiceKey) string {
	if existing != "" {
		return existing
	}
	if candidate != "" {
		return candidate
	}
	return string(key)
}

package forj

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/internal/resourceenv"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
)

const (
	// projectDescribeSchemaVersion identifies the first static project descriptor schema.
	projectDescribeSchemaVersion = 1
	// projectDescribeCapability identifies the CLI contract implemented by this command.
	projectDescribeCapability = "project-descriptor.v1"
	// projectDescribeServiceRequirementsCapability identifies the optional service-intent projection.
	projectDescribeServiceRequirementsCapability = "service-requirements.v1"
)

// ProjectDescribeReport is the stable static project descriptor returned to machine consumers.
type ProjectDescribeReport struct {
	SchemaVersion       int                                 `json:"schema_version"`
	Project             ProjectDescribeProject              `json:"project"`
	GoForj              ProjectDescribeGoForj               `json:"goforj"`
	Apps                []ProjectDescribeApp                `json:"apps"`
	ServiceRequirements []ProjectDescribeServiceRequirement `json:"service_requirements"`
}

// ProjectDescribeProject identifies one project without exposing its filesystem location or environment.
type ProjectDescribeProject struct {
	Name         string `json:"name"`
	Module       string `json:"module"`
	ConfigDigest string `json:"config_digest"`
}

// ProjectDescribeGoForj describes the CLI and generated-project capabilities relevant to the static contract.
type ProjectDescribeGoForj struct {
	Version          string                          `json:"version"`
	CLICapabilities  []string                        `json:"cli_capabilities"`
	GeneratedProject ProjectDescribeGeneratedProject `json:"generated_project"`
}

// ProjectDescribeGeneratedProject describes generated-project compatibility without inspecting runtime state.
type ProjectDescribeGeneratedProject struct {
	Generation   string   `json:"generation"`
	Capabilities []string `json:"capabilities"`
}

// ProjectDescribeApp describes one available App and its conventional runtime intent.
type ProjectDescribeApp struct {
	ID         string                   `json:"id"`
	Name       string                   `json:"name"`
	Entrypoint string                   `json:"entrypoint"`
	Runtimes   []ProjectDescribeRuntime `json:"runtimes"`
}

// ProjectDescribeRuntime describes one generated runtime without claiming an effective environment-selected port.
type ProjectDescribeRuntime struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	DefaultPort   int    `json:"default_port"`
	PublicURL     bool   `json:"public_url"`
	ReadinessPath string `json:"readiness_path"`
}

// ProjectDescribeServiceRequirement describes one secret-free service contract selected by the project.
type ProjectDescribeServiceRequirement struct {
	ID         string                           `json:"id"`
	ServiceKey string                           `json:"service_key"`
	Kind       string                           `json:"kind"`
	Driver     string                           `json:"driver"`
	Owner      string                           `json:"owner"`
	Lifecycle  string                           `json:"lifecycle"`
	Consumers  []string                         `json:"consumers"`
	Endpoints  []ProjectDescribeServiceEndpoint `json:"endpoints"`
}

// ProjectDescribeServiceEndpoint describes one native service endpoint without exposing its address.
type ProjectDescribeServiceEndpoint struct {
	ID         string `json:"id"`
	Protocol   string `json:"protocol"`
	NativePort int    `json:"native_port"`
	Visibility string `json:"visibility"`
}

// ProjectDescribeCmd emits the static project descriptor for a machine consumer.
type ProjectDescribeCmd struct {
	JSON bool `name:"json" help:"Print the versioned JSON project descriptor"`

	stdout     io.Writer
	root       string
	loadConfig func() (*project.Config, error)
	discover   func(string) (projectlayout.Discovery, error)
	cliVersion func() string
}

// Signature declares the machine-oriented project descriptor command.
func (*ProjectDescribeCmd) Signature() string {
	return `name:"project:describe" help:"Describe static project topology as JSON"`
}

// Run emits exactly one descriptor, resolves project-owned topology in memory, and never starts generated code.
func (c *ProjectDescribeCmd) Run() error {
	if !c.JSON {
		return fmt.Errorf("project:describe requires --json")
	}
	loadConfig := c.loadConfig
	if loadConfig == nil {
		loadConfig = project.LoadProjectConfig
	}
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load project configuration: %w", err)
	}
	root := c.root
	if root == "" {
		root = "."
	}
	discover := c.discover
	if discover == nil {
		discover = projectlayout.Discover
	}
	discovery, err := discover(root)
	if err != nil {
		return fmt.Errorf("discover project Apps: %w", err)
	}
	cliVersion := c.cliVersion
	if cliVersion == nil {
		cliVersion = version.String
	}
	serviceRequirements, err := projectDescribeServiceRequirements(root, config)
	if err != nil {
		return fmt.Errorf("resolve project service requirements: %w", err)
	}
	report, err := newProjectDescribeReport(config, discovery, cliVersion(), serviceRequirements)
	if err != nil {
		return err
	}
	return c.writeReport(report)
}

// newProjectDescribeReport derives a secret-free static projection from configuration and conventional App markers.
func newProjectDescribeReport(config *project.Config, discovery projectlayout.Discovery, cliVersion string, serviceRequirements []ProjectDescribeServiceRequirement) (ProjectDescribeReport, error) {
	if config == nil {
		return ProjectDescribeReport{}, fmt.Errorf("project configuration is required")
	}
	apps := projectDescribeApps(config, discovery.RuntimeApps(config))
	digest, err := projectDescribeDigest(config, apps, serviceRequirements)
	if err != nil {
		return ProjectDescribeReport{}, err
	}
	return ProjectDescribeReport{
		SchemaVersion: projectDescribeSchemaVersion,
		Project: ProjectDescribeProject{
			Name:         config.ProjectName,
			Module:       config.GoModuleName,
			ConfigDigest: digest,
		},
		GoForj: ProjectDescribeGoForj{
			Version:         cliVersion,
			CLICapabilities: []string{projectDescribeCapability, projectDescribeServiceRequirementsCapability},
			GeneratedProject: ProjectDescribeGeneratedProject{
				Generation:   config.Render.GoForjVersion,
				Capabilities: []string{},
			},
		},
		Apps:                apps,
		ServiceRequirements: serviceRequirements,
	}, nil
}

// projectDescribeServiceRequirements resolves project-owned dotenv topology without applying it to the process.
func projectDescribeServiceRequirements(root string, config *project.Config) ([]ProjectDescribeServiceRequirement, error) {
	components := project.ProjectComponents(config)
	plan, err := project.DefaultResourcePlan(components)
	if err != nil {
		return nil, fmt.Errorf("default resource plan: %w", err)
	}
	source, err := readProjectDescribeEnvironment(root)
	if err != nil {
		return nil, err
	}
	consumers, err := resourceenv.ResolveConsumers(source, plan, components, config)
	if err != nil {
		return nil, fmt.Errorf("resolve effective resource consumers: %w", err)
	}
	intent := resourceenv.ResolveServiceIntent(source, project.LocalServiceIntent{})
	servicePlan, err := project.ResolveServicePlanWithConsumers(plan, components, intent, consumers)
	if err != nil {
		return nil, fmt.Errorf("resolve service plan: %w", err)
	}
	return projectDescribeServiceRequirementProjection(servicePlan, consumers), nil
}

// readProjectDescribeEnvironment reads only project-owned dotenv layers in their documented precedence order.
func readProjectDescribeEnvironment(root string) ([]byte, error) {
	var source bytes.Buffer
	for _, name := range []string{".env.example", ".env"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		source.Write(content)
		if len(content) == 0 || content[len(content)-1] != '\n' {
			source.WriteByte('\n')
		}
	}
	return source.Bytes(), nil
}

// projectDescribeServiceRequirementProjection converts the internal plan into the stable descriptor contract.
func projectDescribeServiceRequirementProjection(servicePlan project.ServicePlan, consumers []project.EffectiveResourceConsumer) []ProjectDescribeServiceRequirement {
	projected := make([]ProjectDescribeServiceRequirement, 0, len(servicePlan.Requirements))
	usedIDs := map[string]int{}
	for _, requirement := range servicePlan.Requirements {
		consumerNames := projectDescribeServiceConsumers(requirement, consumers)
		baseID := "requirement." + projectDescribeServiceIdentifier(string(requirement.Key))
		if len(requirement.ActiveConsumers) > 0 {
			parts := append([]string(nil), requirement.ActiveConsumers...)
			sort.Strings(parts)
			baseID += "." + projectDescribeServiceIdentifier(strings.Join(parts, "."))
		} else {
			baseID += ".available"
		}
		usedIDs[baseID]++
		id := baseID
		if usedIDs[baseID] > 1 {
			id = fmt.Sprintf("%s.%d", baseID, usedIDs[baseID])
		}
		owner := projectDescribeServiceOwner(requirement.State)
		endpoints := projectDescribeServiceEndpoints(requirement.Key, owner, id)
		if len(endpoints) == 0 {
			continue
		}
		projected = append(projected, ProjectDescribeServiceRequirement{
			ID:         id,
			ServiceKey: string(requirement.Key),
			Kind:       projectDescribeServiceKind(requirement, consumers),
			Driver:     projectDescribeServiceDriver(requirement, consumers),
			Owner:      owner,
			Lifecycle:  "project",
			Consumers:  consumerNames,
			Endpoints:  endpoints,
		})
	}
	sort.SliceStable(projected, func(left, right int) bool { return projected[left].ID < projected[right].ID })
	return projected
}

// projectDescribeServiceConsumers maps internal resource identities onto descriptor App IDs.
func projectDescribeServiceConsumers(requirement project.ServiceRequirement, consumers []project.EffectiveResourceConsumer) []string {
	seen := map[string]struct{}{}
	apps := make([]string, 0, len(requirement.ActiveConsumers))
	for _, active := range requirement.ActiveConsumers {
		for _, consumer := range consumers {
			if consumer.Consumer != active || !projectDescribeConsumerMatchesService(consumer, requirement) {
				continue
			}
			app := projectDescribeConsumerApp(consumer.Consumer)
			if _, exists := seen[app]; exists {
				continue
			}
			seen[app] = struct{}{}
			apps = append(apps, app)
		}
	}
	sort.Strings(apps)
	return apps
}

// projectDescribeConsumerMatchesService keeps driver and endpoint-affinity joins exact across shared services.
func projectDescribeConsumerMatchesService(consumer project.EffectiveResourceConsumer, requirement project.ServiceRequirement) bool {
	definition, ok := project.ResourceDefinitionByKey(consumer.Resource)
	if !ok {
		return false
	}
	driver, ok := definition.Driver(consumer.Driver)
	return ok && driver.Service == requirement.Key && consumer.EndpointAffinity == requirement.EndpointAffinity
}

// projectDescribeConsumerApp extracts the stable App prefix from a resource consumer identity.
func projectDescribeConsumerApp(consumer string) string {
	if index := strings.IndexByte(consumer, ':'); index > 0 {
		return consumer[:index]
	}
	return project.DefaultAppName
}

// projectDescribeServiceOwner maps GoForj lifecycle state onto descriptor ownership.
func projectDescribeServiceOwner(state project.ServiceState) string {
	switch state {
	case project.ServiceStateActiveLocal, project.ServiceStateLocalRequestedUnused:
		return "compose"
	case project.ServiceStateAvailableLocal:
		return "available"
	default:
		return "external"
	}
}

// projectDescribeServiceKind chooses a resource category without collapsing distinct shared endpoints.
func projectDescribeServiceKind(requirement project.ServiceRequirement, consumers []project.EffectiveResourceConsumer) string {
	kinds := map[string]struct{}{}
	for _, consumer := range consumers {
		if consumer.Consumer != "" && !projectDescribeContainsString(requirement.ActiveConsumers, consumer.Consumer) {
			continue
		}
		if !projectDescribeConsumerMatchesService(consumer, requirement) {
			continue
		}
		kinds[projectDescribeResourceKind(consumer.Resource)] = struct{}{}
	}
	if len(kinds) == 1 {
		for kind := range kinds {
			return kind
		}
	}
	if len(kinds) > 1 {
		return "shared"
	}
	return projectDescribeResourceKind(projectDescribeServiceResource(requirement.Key))
}

// projectDescribeServiceDriver returns a catalog driver name without exposing endpoint configuration.
func projectDescribeServiceDriver(requirement project.ServiceRequirement, consumers []project.EffectiveResourceConsumer) string {
	for _, consumer := range consumers {
		if projectDescribeConsumerMatchesService(consumer, requirement) {
			return consumer.Driver
		}
	}
	for _, definition := range project.ResourceCatalog() {
		for _, driver := range definition.Drivers {
			if driver.Service == requirement.Key {
				return driver.Name
			}
		}
	}
	return "service"
}

// projectDescribeResourceKind keeps descriptor categories stable across catalog labels.
func projectDescribeResourceKind(resource project.ResourceKey) string {
	switch resource {
	case project.ResourceDatabase:
		return "database"
	case project.ResourceCache:
		return "cache"
	case project.ResourceQueue:
		return "queue"
	case project.ResourceEvents:
		return "events"
	case project.ResourceStorage:
		return "storage"
	case project.ResourceMail:
		return "mail"
	default:
		return "service"
	}
}

// projectDescribeServiceResource finds the first catalog resource that publishes a service key.
func projectDescribeServiceResource(key project.ServiceKey) project.ResourceKey {
	for _, definition := range project.ResourceCatalog() {
		for _, driver := range definition.Drivers {
			if driver.Service == key {
				return definition.Key
			}
		}
	}
	return ""
}

// projectDescribeServiceEndpoints returns only cataloged native ports and never copies environment values.
func projectDescribeServiceEndpoints(key project.ServiceKey, owner string, requirementID string) []ProjectDescribeServiceEndpoint {
	protocol := "tcp"
	port := 0
	switch key {
	case project.ServiceMySQL, project.ServiceCacheMySQL, project.ServiceQueueMySQL:
		port = 3306
	case project.ServicePostgres, project.ServiceCachePostgres, project.ServiceQueuePostgres:
		port = 5432
	case project.ServiceRedis:
		port = 6379
	case project.ServiceCacheMemcached:
		port = 11211
	case project.ServiceCacheDynamoDB:
		protocol, port = "http", 8000
	case project.ServiceCacheNATS, project.ServiceQueueNATS, project.ServiceEventsNATS:
		port = 4222
	case project.ServiceQueueSQS:
		protocol, port = "http", 9324
	case project.ServiceQueueRabbitMQ:
		port = 5672
	case project.ServiceEventsKafka:
		port = 9092
	case project.ServiceEventsGCPPubSub:
		protocol, port = "http", 8085
	case project.ServiceStorageFTP:
		port = 21
	case project.ServiceStorageSFTP:
		port = 22
	case project.ServiceStorageS3:
		protocol, port = "http", 9000
	case project.ServiceStorageGCS, project.ServiceStorageDropbox, project.ServiceStorageRclone,
		project.ServiceMailResend, project.ServiceMailPostmark, project.ServiceMailMailgun,
		project.ServiceMailSendGrid, project.ServiceMailSES:
		protocol, port = "https", 443
	case project.ServiceMailSMTP:
		port = 1025
	}
	if port == 0 {
		return nil
	}
	visibility := "private"
	if owner == "compose" {
		visibility = "host"
	}
	return []ProjectDescribeServiceEndpoint{{
		ID:         requirementID + ".endpoint." + protocol,
		Protocol:   protocol,
		NativePort: port,
		Visibility: visibility,
	}}
}

// projectDescribeServiceIdentifier keeps stable descriptor IDs within the parser's token grammar.
func projectDescribeServiceIdentifier(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			builder.WriteRune(character)
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('.')
		}
	}
	return strings.Trim(builder.String(), ".")
}

// projectDescribeContainsString reports whether a stable consumer list includes one exact identity.
func projectDescribeContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// projectDescribeApps preserves generated runtime ordering while making every array explicit in JSON.
func projectDescribeApps(config *project.Config, source []project.App) []ProjectDescribeApp {
	apps := make([]ProjectDescribeApp, 0, len(source))
	for index, app := range source {
		app = projectlayout.NormalizeApp(app)
		components := projectDescribeAppComponents(config, app.Name)
		runtimes := make([]ProjectDescribeRuntime, 0, 1)
		if components.WebAPI || components.WebUI {
			runtimes = append(runtimes, ProjectDescribeRuntime{
				ID:            "http",
				Kind:          "http",
				DefaultPort:   3000 + index,
				PublicURL:     true,
				ReadinessPath: "/-/ready",
			})
		}
		apps = append(apps, ProjectDescribeApp{
			ID:         app.Name,
			Name:       app.Name,
			Entrypoint: app.Entrypoint,
			Runtimes:   runtimes,
		})
	}
	return apps
}

// projectDescribeAppComponents preserves the default App's project-wide component selection.
func projectDescribeAppComponents(config *project.Config, name string) project.Components {
	if name == "" || name == project.DefaultAppName {
		return config.Render.Components.WithResolvedDependencies()
	}
	appConfig, ok := config.Apps[name]
	if !ok {
		return config.Render.Components.WithResolvedDependencies()
	}
	return project.NormalizeConfiguredAppComponents(config, appConfig.Components)
}

// projectDescribeDigest hashes normalized non-secret topology instead of raw configuration or dotenv bytes.
func projectDescribeDigest(config *project.Config, apps []ProjectDescribeApp, serviceRequirements ...[]ProjectDescribeServiceRequirement) (string, error) {
	type appTopology struct {
		Name       string             `json:"name"`
		Components project.Components `json:"components"`
	}
	appNames := make([]string, 0, len(config.Apps))
	for name := range config.Apps {
		appNames = append(appNames, name)
	}
	sort.Strings(appNames)
	configuredApps := make([]appTopology, 0, len(appNames))
	for _, name := range appNames {
		configuredApps = append(configuredApps, appTopology{
			Name:       name,
			Components: projectDescribeAppComponents(config, name),
		})
	}
	payload := struct {
		ProjectName   string                              `json:"project_name"`
		ModuleName    string                              `json:"module_name"`
		Components    project.Components                  `json:"components"`
		Configured    []appTopology                       `json:"configured_apps"`
		AvailableApps []ProjectDescribeApp                `json:"available_apps"`
		Services      []ProjectDescribeServiceRequirement `json:"service_requirements"`
	}{
		ProjectName:   config.ProjectName,
		ModuleName:    config.GoModuleName,
		Components:    config.Render.Components.WithResolvedDependencies(),
		Configured:    configuredApps,
		AvailableApps: apps,
	}
	if len(serviceRequirements) > 0 {
		payload.Services = serviceRequirements[0]
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode normalized project topology: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// writeReport writes a single JSON object and preserves empty arrays for schema consumers.
func (c *ProjectDescribeCmd) writeReport(report ProjectDescribeReport) error {
	if report.GoForj.CLICapabilities == nil {
		report.GoForj.CLICapabilities = []string{}
	}
	if report.GoForj.GeneratedProject.Capabilities == nil {
		report.GoForj.GeneratedProject.Capabilities = []string{}
	}
	if report.Apps == nil {
		report.Apps = []ProjectDescribeApp{}
	}
	if report.ServiceRequirements == nil {
		report.ServiceRequirements = []ProjectDescribeServiceRequirement{}
	}
	for index := range report.Apps {
		if report.Apps[index].Runtimes == nil {
			report.Apps[index].Runtimes = []ProjectDescribeRuntime{}
		}
	}
	for index := range report.ServiceRequirements {
		if report.ServiceRequirements[index].Consumers == nil {
			report.ServiceRequirements[index].Consumers = []string{}
		}
		if report.ServiceRequirements[index].Endpoints == nil {
			report.ServiceRequirements[index].Endpoints = []ProjectDescribeServiceEndpoint{}
		}
	}
	stdout := c.stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		return fmt.Errorf("write project descriptor: %w", err)
	}
	return nil
}

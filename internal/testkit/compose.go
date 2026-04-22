package testkit

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	testcontainers "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gopkg.in/yaml.v3"
)

type RenderedComposeStack struct {
	projectDir string
	services   map[string]*StartedContainer
}

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image       string            `yaml:"image"`
	Build       *composeBuild     `yaml:"build"`
	Ports       []string          `yaml:"ports"`
	Environment composeEnvEntries `yaml:"environment"`
}

type composeBuild struct {
	Context    string           `yaml:"context"`
	Dockerfile string           `yaml:"dockerfile"`
	Args       composeKeyValues `yaml:"args"`
}

type composeResolvedPort struct {
	Container nat.Port
	Binding   nat.PortBinding
}

type composeEnvEntries map[string]string

func (e *composeEnvEntries) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		*e = nil
		return nil
	}
	switch value.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if err := value.Decode(&m); err != nil {
			return err
		}
		*e = m
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		m := make(map[string]string, len(items))
		for _, item := range items {
			key, rawValue, ok := strings.Cut(item, "=")
			if !ok {
				m[strings.TrimSpace(item)] = ""
				continue
			}
			m[strings.TrimSpace(key)] = strings.TrimSpace(rawValue)
		}
		*e = m
		return nil
	default:
		return fmt.Errorf("unsupported compose environment node kind %d", value.Kind)
	}
}

type composeKeyValues map[string]string

func (kv *composeKeyValues) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		*kv = nil
		return nil
	}
	switch value.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if err := value.Decode(&m); err != nil {
			return err
		}
		*kv = m
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		m := make(map[string]string, len(items))
		for _, item := range items {
			key, rawValue, ok := strings.Cut(item, "=")
			if !ok {
				m[strings.TrimSpace(item)] = ""
				continue
			}
			m[strings.TrimSpace(key)] = strings.TrimSpace(rawValue)
		}
		*kv = m
		return nil
	default:
		return fmt.Errorf("unsupported compose key/value node kind %d", value.Kind)
	}
}

func StartRenderedComposeServices(projectDir string, logf Logf) (*RenderedComposeStack, error) {
	if err := prepareRenderedComposeTestEnv(projectDir); err != nil {
		return nil, err
	}
	model, err := loadRenderedCompose(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &RenderedComposeStack{
				projectDir: projectDir,
				services:   map[string]*StartedContainer{},
			}, nil
		}
		return nil, err
	}
	stack := &RenderedComposeStack{
		projectDir: projectDir,
		services:   make(map[string]*StartedContainer, len(model.Services)),
	}
	startedNames := make([]string, 0, len(model.Services))
	for name := range model.Services {
		startedNames = append(startedNames, name)
	}
	sort.Strings(startedNames)
	for _, name := range startedNames {
		started, err := startComposeService(logf, projectDir, name, model.Services[name])
		if err != nil {
			stack.Stop()
			return nil, err
		}
		stack.services[name] = started
	}
	return stack, nil
}

func (s *RenderedComposeStack) Stop() {
	if s == nil {
		return
	}
	names := make([]string, 0, len(s.services))
	for name := range s.services {
		names = append(names, name)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names {
		if started := s.services[name]; started != nil {
			started.Stop()
		}
	}
}

func (s *RenderedComposeStack) Service(name string) (*StartedContainer, bool) {
	if s == nil {
		return nil, false
	}
	started, ok := s.services[name]
	return started, ok
}

func (s *RenderedComposeStack) EnvOverrides() map[string]string {
	overrides := map[string]string{}
	if mysql, ok := s.Service("mysql"); ok {
		overrides["DB_HOST"] = normalizeIntegrationHost(mysql.Host)
		overrides["DB_PORT"] = mysql.Port
		overrides["DB_HOST_INTEGRATION"] = normalizeIntegrationHost(mysql.Host)
		overrides["DB_PORT_INTEGRATION"] = mysql.Port
	}
	if postgres, ok := s.Service("postgres"); ok {
		overrides["DB_HOST"] = normalizeIntegrationHost(postgres.Host)
		overrides["DB_PORT"] = postgres.Port
		overrides["DB_HOST_INTEGRATION"] = normalizeIntegrationHost(postgres.Host)
		overrides["DB_PORT_INTEGRATION"] = postgres.Port
	}
	if redis, ok := s.Service("redis"); ok {
		overrides["REDIS_HOST"] = normalizeIntegrationHost(redis.Host)
		overrides["REDIS_PORT"] = redis.Port
	}
	return overrides
}

func normalizeIntegrationHost(host string) string {
	switch strings.TrimSpace(strings.ToLower(host)) {
	case "":
		return "127.0.0.1"
	default:
		return host
	}
}

func (s *RenderedComposeStack) ApplyHostEnvOverrides(paths []string) error {
	if s == nil || len(paths) == 0 {
		return nil
	}
	overrides := s.EnvOverrides()
	if len(overrides) == 0 {
		return nil
	}
	return ReplaceOrAppendEnvValues(paths, overrides)
}

func loadRenderedCompose(projectDir string) (*composeFile, error) {
	composePath := filepath.Join(projectDir, "docker-compose.yml")
	body, err := os.ReadFile(composePath)
	if err != nil {
		return nil, err
	}
	var model composeFile
	if err := yaml.Unmarshal(body, &model); err != nil {
		return nil, fmt.Errorf("parse docker-compose.yml: %w", err)
	}
	env, err := ParseEnvFiles(
		filepath.Join(projectDir, ".env"),
	)
	if err != nil {
		return nil, err
	}
	for name, service := range model.Services {
		model.Services[name] = interpolateComposeService(service, env)
	}
	return &model, nil
}

func interpolateComposeService(service composeService, env map[string]string) composeService {
	service.Image = interpolateComposeValue(service.Image, env)
	for i := range service.Ports {
		service.Ports[i] = interpolateComposeValue(service.Ports[i], env)
	}
	if len(service.Environment) > 0 {
		resolved := make(map[string]string, len(service.Environment))
		for key, value := range service.Environment {
			resolved[key] = interpolateComposeValue(value, env)
		}
		service.Environment = resolved
	}
	if service.Build != nil {
		service.Build.Context = interpolateComposeValue(service.Build.Context, env)
		service.Build.Dockerfile = interpolateComposeValue(service.Build.Dockerfile, env)
		if len(service.Build.Args) > 0 {
			resolved := make(map[string]string, len(service.Build.Args))
			for key, value := range service.Build.Args {
				resolved[key] = interpolateComposeValue(value, env)
			}
			service.Build.Args = resolved
		}
	}
	return service
}

func interpolateComposeValue(input string, env map[string]string) string {
	if input == "" {
		return ""
	}
	var out strings.Builder
	for i := 0; i < len(input); {
		if input[i] == '$' && i+1 < len(input) && input[i+1] == '{' {
			end := strings.IndexByte(input[i+2:], '}')
			if end >= 0 {
				token := input[i+2 : i+2+end]
				key, fallback, hasFallback := strings.Cut(token, ":-")
				value := env[key]
				if value == "" && hasFallback {
					value = fallback
				}
				out.WriteString(value)
				i += end + 3
				continue
			}
		}
		out.WriteByte(input[i])
		i++
	}
	return out.String()
}

func startComposeService(logf Logf, projectDir, name string, service composeService) (*StartedContainer, error) {
	resolvedPort, err := composeServiceContainerPort(service)
	if err != nil {
		return nil, fmt.Errorf("resolve compose service %s port: %w", name, err)
	}
	request := testcontainers.ContainerRequest{
		Image:        service.Image,
		Env:          cloneMap(service.Environment),
		ExposedPorts: []string{string(resolvedPort.Container)},
		WaitingFor:   composeServiceWaitStrategy(name, string(resolvedPort.Container)),
		HostConfigModifier: func(hostConfig *dockercontainer.HostConfig) {
			hostConfig.PortBindings = nat.PortMap{
				resolvedPort.Container: []nat.PortBinding{resolvedPort.Binding},
			}
		},
	}
	if service.Build != nil {
		request.FromDockerfile = testcontainers.FromDockerfile{
			Context:    filepath.Join(projectDir, service.Build.Context),
			Dockerfile: service.Build.Dockerfile,
			BuildArgs:  stringifyBuildArgs(service.Build.Args),
		}
	}
	readyLabel := strings.ToUpper(name[:1]) + name[1:]
	return StartTestcontainer(logf, request, string(resolvedPort.Container), 60*time.Second, readyLabel)
}

func composeServiceContainerPort(service composeService) (composeResolvedPort, error) {
	if len(service.Ports) == 0 {
		return composeResolvedPort{}, fmt.Errorf("service does not expose any ports")
	}
	raw := strings.Trim(strings.TrimSpace(service.Ports[0]), "\"'")
	mappings, err := nat.ParsePortSpec(raw)
	if err != nil {
		return composeResolvedPort{}, fmt.Errorf("invalid container port %q: %w", raw, err)
	}
	if len(mappings) == 0 {
		return composeResolvedPort{}, fmt.Errorf("service port %q did not produce any container mappings", raw)
	}
	return composeResolvedPort{
		Container: mappings[0].Port,
		Binding:   mappings[0].Binding,
	}, nil
}

func composeServiceWaitStrategy(name, containerPort string) wait.Strategy {
	port := nat.Port(containerPort)
	switch name {
	case "mysql":
		return wait.ForListeningPort(port).WithStartupTimeout(90 * time.Second)
	case "postgres":
		return wait.ForAll(
			wait.ForListeningPort(port),
			wait.ForLog("database system is ready to accept connections"),
		).WithStartupTimeout(60 * time.Second)
	case "redis":
		return wait.ForAll(
			wait.ForListeningPort(port),
			wait.ForLog("Ready to accept connections"),
		).WithStartupTimeout(60 * time.Second)
	default:
		return wait.ForListeningPort(port).WithStartupTimeout(60 * time.Second)
	}
}

func stringifyBuildArgs(args map[string]string) map[string]*string {
	if len(args) == 0 {
		return nil
	}
	converted := make(map[string]*string, len(args))
	for key, value := range args {
		valueCopy := value
		converted[key] = &valueCopy
	}
	return converted
}

func cloneMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func prepareRenderedComposeTestEnv(projectDir string) error {
	_ = os.Remove(filepath.Join(projectDir, ".env.host"))
	model, err := readComposeModel(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	overrides := map[string]string{}
	if _, ok := model.Services["mysql"]; ok {
		port, err := findOpenPortInRange()
		if err != nil {
			return fmt.Errorf("allocate mysql test port: %w", err)
		}
		overrides["DB_HOST"] = "localhost"
		overrides["DB_PORT"] = strconv.Itoa(port)
	}
	if _, ok := model.Services["postgres"]; ok {
		port, err := findOpenPortInRange()
		if err != nil {
			return fmt.Errorf("allocate postgres test port: %w", err)
		}
		overrides["DB_HOST"] = "localhost"
		overrides["DB_PORT"] = strconv.Itoa(port)
	}
	if _, ok := model.Services["redis"]; ok {
		port, err := findOpenPortInRange()
		if err != nil {
			return fmt.Errorf("allocate redis test port: %w", err)
		}
		overrides["REDIS_HOST"] = "localhost"
		overrides["REDIS_PORT"] = strconv.Itoa(port)
	}
	if len(overrides) == 0 {
		return nil
	}
	return ReplaceOrAppendEnvValues([]string{filepath.Join(projectDir, ".env")}, overrides)
}

func readComposeModel(projectDir string) (*composeFile, error) {
	composePath := filepath.Join(projectDir, "docker-compose.yml")
	body, err := os.ReadFile(composePath)
	if err != nil {
		return nil, err
	}
	var model composeFile
	if err := yaml.Unmarshal(body, &model); err != nil {
		return nil, fmt.Errorf("parse docker-compose.yml: %w", err)
	}
	return &model, nil
}

func findOpenPortInRange() (int, error) {
	start, end := renderedComposePortRange()
	for port := start; port <= end; port++ {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		_ = listener.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no open port available in range %d-%d", start, end)
}

func renderedComposePortRange() (int, int) {
	start := parsePortRangeValue("FORJ_INTEGRATION_PORT_RANGE_START", 46000)
	end := parsePortRangeValue("FORJ_INTEGRATION_PORT_RANGE_END", 46999)
	if start > end {
		start, end = end, start
	}
	return start, end
}

func parsePortRangeValue(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 || value > 65535 {
		return fallback
	}
	return value
}

package testkit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

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
		overrides["DB_HOST"] = mysql.Host
		overrides["DB_PORT"] = mysql.Port
		overrides["DB_HOST_INTEGRATION"] = mysql.Host
		overrides["DB_PORT_INTEGRATION"] = mysql.Port
	}
	if postgres, ok := s.Service("postgres"); ok {
		overrides["DB_HOST"] = postgres.Host
		overrides["DB_PORT"] = postgres.Port
		overrides["DB_HOST_INTEGRATION"] = postgres.Host
		overrides["DB_PORT_INTEGRATION"] = postgres.Port
	}
	if redis, ok := s.Service("redis"); ok {
		overrides["REDIS_HOST"] = redis.Host
		overrides["REDIS_PORT"] = redis.Port
	}
	return overrides
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
		filepath.Join(projectDir, ".env.host"),
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
	containerPort, err := composeServiceContainerPort(service)
	if err != nil {
		return nil, fmt.Errorf("resolve compose service %s port: %w", name, err)
	}
	request := testcontainers.ContainerRequest{
		Image:        service.Image,
		Env:          cloneMap(service.Environment),
		ExposedPorts: []string{containerPort},
		WaitingFor:   composeServiceWaitStrategy(name, containerPort),
	}
	if service.Build != nil {
		request.FromDockerfile = testcontainers.FromDockerfile{
			Context:    filepath.Join(projectDir, service.Build.Context),
			Dockerfile: service.Build.Dockerfile,
			BuildArgs:  stringifyBuildArgs(service.Build.Args),
		}
	}
	readyLabel := strings.ToUpper(name[:1]) + name[1:]
	return StartTestcontainer(logf, request, containerPort, 60*time.Second, readyLabel)
}

func composeServiceContainerPort(service composeService) (string, error) {
	if len(service.Ports) == 0 {
		return "", fmt.Errorf("service does not expose any ports")
	}
	raw := strings.TrimSpace(service.Ports[0])
	raw = strings.Trim(raw, "\"'")
	parts := strings.Split(raw, ":")
	last := strings.TrimSpace(parts[len(parts)-1])
	last = strings.TrimPrefix(last, "[")
	last = strings.TrimSuffix(last, "]")
	protocol := "tcp"
	portToken := last
	if before, after, ok := strings.Cut(last, "/"); ok {
		portToken = before
		if after != "" {
			protocol = after
		}
	}
	portToken = strings.TrimSpace(portToken)
	if _, err := strconv.Atoi(portToken); err != nil {
		return "", fmt.Errorf("invalid container port %q", raw)
	}
	return portToken + "/" + protocol, nil
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

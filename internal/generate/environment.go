package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

var generationEnvironmentPrefixes = []string{
	"CACHE_",
	"DB_",
	"EVENTS_",
	"MAIL_",
	"METRICS_",
	"OBSERVABILITY_",
	"QUEUE_",
	"STORAGE_",
}

var generationEnvironmentExactKeys = map[string]bool{
	"API_HTTP_PORT":          true,
	"API_METRICS_PORT":       true,
	"APP_ENV":                true,
	"APP_NAME":               true,
	"APP_REVISION":           true,
	"APP_VERSION":            true,
	"JOBS_METRICS_PORT":      true,
	"PORT":                   true,
	"SCHEDULER_METRICS_PORT": true,
	"WORKER_METRICS_PORT":    true,
}

var generationEnvironmentAppSuffixes = []string{
	"_API_HTTP_PORT",
	"_API_METRICS_PORT",
	"_JOBS_METRICS_PORT",
	"_METRICS_API_PORT",
	"_METRICS_JOBS_PORT",
	"_METRICS_PORT",
	"_METRICS_SCHEDULER_PORT",
	"_PORT",
	"_SCHEDULER_METRICS_PORT",
	"_WORKER_METRICS_PORT",
}

// generationEnvironment is an immutable view of the values that may influence one generation run.
type generationEnvironment struct {
	values map[string]string
}

// generationEnvironmentEntry exposes one immutable key/value pair without re-parsing process-style assignments.
type generationEnvironmentEntry struct {
	key   string
	value string
}

// generationEnvironmentFilter retains project inputs and rejects unrelated variables that merely contain resource words.
type generationEnvironmentFilter struct {
	appPrefixes map[string]struct{}
}

// generationInput keeps project ownership and its environment snapshot together across generator tasks.
type generationInput struct {
	projectDir    string
	environment   generationEnvironment
	appPrefixes   []string
	resourceNames map[string][]string
}

// withoutResourceSettings retains accessor declarations while removing provider and connection choices from the active environment.
func withoutResourceSettings(input generationInput) generationInput {
	input.resourceNames = map[string][]string{}
	for _, resource := range generationEnvironmentResources() {
		input.resourceNames[resource.prefix] = discoverPrimitiveChildNames(input, resource.prefix, resource.rootKeys)
	}
	for key := range input.environment.values {
		if isResourceSetting(key, input.appPrefixes) {
			delete(input.environment.values, key)
		}
	}
	return input
}

// isResourceSetting separates replaceable provider configuration from unrelated generation inputs such as observability ports.
func isResourceSetting(key string, appPrefixes []string) bool {
	if key == "COMPOSE_PROFILES" {
		return true
	}
	for _, prefix := range []string{"DB_", "CACHE_", "QUEUE_", "EVENTS_", "STORAGE_", "MAIL_", "REDIS_"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
		for _, app := range appPrefixes {
			if strings.HasPrefix(key, app+"_"+prefix) {
				return true
			}
		}
	}
	return false
}

// loadProjectGenerationInput reads one project-owned environment snapshot without changing process state.
func loadProjectGenerationInput(projectDir string) (generationInput, error) {
	example, err := readOptionalGenerationEnvironment(filepath.Join(projectDir, ".env.example"))
	if err != nil {
		return generationInput{}, fmt.Errorf("read generation environment fallback: %w", err)
	}
	owner, err := readOptionalGenerationEnvironment(filepath.Join(projectDir, ".env"))
	if err != nil {
		return generationInput{}, fmt.Errorf("read generation environment: %w", err)
	}

	merged := make(map[string]string, len(example)+len(owner))
	for key, value := range example {
		merged[key] = value
	}
	for key, value := range owner {
		merged[key] = value
	}

	source := generationEnvironment{values: merged}
	filter := newGenerationEnvironmentFilter(projectDir, source)
	values := make(map[string]string, len(merged))
	for key, value := range merged {
		if filter.keeps(key) {
			values[key] = value
		}
	}
	return generationInput{
		projectDir:  projectDir,
		environment: generationEnvironment{values: values},
		appPrefixes: filter.sortedAppPrefixes(),
	}, nil
}

// ambientGenerationInput preserves direct generator APIs while isolating production orchestration from ambient state.
func ambientGenerationInput(projectDir string) generationInput {
	environment := generationEnvironmentFromAssignments(os.Environ())
	filter := newGenerationEnvironmentFilter(projectDir, environment)
	return generationInput{
		projectDir:  projectDir,
		environment: environment,
		appPrefixes: filter.sortedAppPrefixes(),
	}
}

// generationEnvironmentFromAssignments copies process-style assignments so later mutations cannot affect a run.
func generationEnvironmentFromAssignments(assignments []string) generationEnvironment {
	values := make(map[string]string, len(assignments))
	for _, assignment := range assignments {
		key, value, ok := strings.Cut(assignment, "=")
		if ok {
			values[key] = value
		}
	}
	return generationEnvironment{values: values}
}

// Lookup returns the captured value without consulting the current process environment.
func (e generationEnvironment) Lookup(key string) (string, bool) {
	value, ok := e.values[key]
	return value, ok
}

// Get returns a captured non-empty value or fallback using the runtime env package's semantics.
func (e generationEnvironment) Get(key string, fallback string) string {
	value := e.values[key]
	if value == "" {
		return fallback
	}
	return value
}

// Entries returns a stable typed copy for validation and discovery without exposing the backing map.
func (e generationEnvironment) Entries() []generationEnvironmentEntry {
	keys := make([]string, 0, len(e.values))
	for key := range e.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]generationEnvironmentEntry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, generationEnvironmentEntry{key: key, value: e.values[key]})
	}
	return entries
}

// readOptionalGenerationEnvironment reads one project contract without treating an absent file as an error.
func readOptionalGenerationEnvironment(path string) (map[string]string, error) {
	values, err := godotenv.Read(path)
	if err == nil {
		return values, nil
	}
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	return nil, err
}

// newGenerationEnvironmentFilter recognizes Apps before deciding which resource-shaped keys belong to the project.
func newGenerationEnvironmentFilter(projectDir string, environment generationEnvironment) generationEnvironmentFilter {
	return generationEnvironmentFilter{appPrefixes: generationAppEnvPrefixSet(projectDir, environment)}
}

// keeps retains direct generator inputs plus every resource key beneath a recognized App prefix so validation can reject typos.
func (f generationEnvironmentFilter) keeps(key string) bool {
	if isDirectGenerationEnvironmentKey(key) {
		return true
	}
	for appPrefix := range f.appPrefixes {
		if isGenerationAppResourceKey(key, appPrefix) {
			return true
		}
	}
	return false
}

// sortedAppPrefixes returns a stable copy for repeated resource-specific filtering.
func (f generationEnvironmentFilter) sortedAppPrefixes() []string {
	return sortStrings(f.appPrefixes)
}

// isDirectGenerationEnvironmentKey recognizes root resources and metrics inputs without interpreting embedded cache-like words.
func isDirectGenerationEnvironmentKey(key string) bool {
	if generationEnvironmentExactKeys[key] {
		return true
	}
	for _, prefix := range generationEnvironmentPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	for _, suffix := range generationEnvironmentAppSuffixes {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

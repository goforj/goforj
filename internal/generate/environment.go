package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

var generationEnvironmentMu sync.Mutex

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

// loadGenerationEnvironment installs one project-owned environment snapshot for the duration of generation.
func loadGenerationEnvironment(projectDir string) (func(), error) {
	// Generators still read process environment, so serialize the temporary snapshot until their inputs can be passed explicitly.
	generationEnvironmentMu.Lock()

	example, err := readOptionalGenerationEnvironment(filepath.Join(projectDir, ".env.example"))
	if err != nil {
		generationEnvironmentMu.Unlock()
		return nil, fmt.Errorf("read generation environment fallback: %w", err)
	}
	owner, err := readOptionalGenerationEnvironment(filepath.Join(projectDir, ".env"))
	if err != nil {
		generationEnvironmentMu.Unlock()
		return nil, fmt.Errorf("read generation environment: %w", err)
	}

	effective := make(map[string]string, len(example)+len(owner))
	for key, value := range example {
		if isGenerationEnvironmentKey(key) {
			effective[key] = value
		}
	}
	for key, value := range owner {
		if isGenerationEnvironmentKey(key) {
			effective[key] = value
		}
	}

	previousValues := map[string]string{}
	previouslySet := map[string]bool{}
	for _, assignment := range os.Environ() {
		key, _, _ := strings.Cut(assignment, "=")
		if !isGenerationEnvironmentKey(key) {
			if _, selected := effective[key]; !selected {
				continue
			}
		}
		previousValues[key], previouslySet[key] = os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			restoreGenerationEnvironment(previousValues, previouslySet)
			generationEnvironmentMu.Unlock()
			return nil, fmt.Errorf("clear generation environment %s: %w", key, err)
		}
	}

	keys := sortedGenerationEnvironmentKeys(effective)
	for _, key := range keys {
		if _, recorded := previouslySet[key]; !recorded {
			previousValues[key], previouslySet[key] = os.LookupEnv(key)
		}
		if err := os.Setenv(key, effective[key]); err != nil {
			restoreGenerationEnvironment(previousValues, previouslySet)
			generationEnvironmentMu.Unlock()
			return nil, fmt.Errorf("set generation environment %s: %w", key, err)
		}
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			restoreGenerationEnvironment(previousValues, previouslySet)
			generationEnvironmentMu.Unlock()
		})
	}, nil
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

// isGenerationEnvironmentKey identifies ambient resource values that must not influence a project snapshot.
func isGenerationEnvironmentKey(key string) bool {
	if generationEnvironmentExactKeys[key] {
		return true
	}
	for _, prefix := range generationEnvironmentPrefixes {
		if strings.HasPrefix(key, prefix) || strings.Contains(key, "_"+prefix) {
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

// sortedGenerationEnvironmentKeys makes snapshot installation deterministic and error reporting reproducible.
func sortedGenerationEnvironmentKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// restoreGenerationEnvironment returns every temporarily touched key to its exact process-level state.
func restoreGenerationEnvironment(values map[string]string, set map[string]bool) {
	for _, key := range sortedGenerationEnvironmentKeys(values) {
		if set[key] {
			_ = os.Setenv(key, values[key])
			continue
		}
		_ = os.Unsetenv(key)
	}
}

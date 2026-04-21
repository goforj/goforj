package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/coredeps"
)

const fixtureGoVersion = "1.24"

type fixtureReplace struct {
	module string
	path   string
}

type fixtureGoModSpec struct {
	module   string
	required []string
	pinned   []string
	replaces []fixtureReplace
}

func writeFixtureGoMod(t *testing.T, root string, spec fixtureGoModSpec) {
	t.Helper()

	if strings.TrimSpace(spec.module) == "" {
		t.Fatal("fixture go.mod requires a module path")
	}

	lines := []string{
		"module " + spec.module,
		"",
		"go " + fixtureGoVersion,
		"",
		"require (",
	}
	for _, module := range fixtureRequireList(t, spec.required, spec.pinned) {
		lines = append(lines, "\t"+module+" "+fixtureModuleVersion(t, module))
	}
	lines = append(lines, ")")

	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	for _, replace := range spec.replaces {
		addFixtureReplaceIfPresent(t, root, replace)
	}
}

func fixtureModuleVersion(t *testing.T, module string) string {
	t.Helper()

	version, ok := coredeps.VersionFor(module)
	if !ok {
		t.Fatalf("missing fixture module version for %s", module)
	}
	return version
}

func fixtureRequireList(t *testing.T, required []string, pinned []string) []string {
	t.Helper()

	seen := map[string]struct{}{}
	combined := make([]string, 0, len(required)+len(pinned))
	for _, group := range [][]string{required, pinned} {
		for _, module := range group {
			module = strings.TrimSpace(module)
			if module == "" {
				continue
			}
			if _, ok := coredeps.VersionFor(module); !ok {
				t.Fatalf("missing fixture module version for %s", module)
			}
			if _, ok := seen[module]; ok {
				continue
			}
			seen[module] = struct{}{}
			combined = append(combined, module)
		}
	}
	sort.Strings(combined)
	return combined
}

func addFixtureReplaceIfPresent(t *testing.T, root string, replace fixtureReplace) {
	t.Helper()

	if strings.TrimSpace(replace.module) == "" || strings.TrimSpace(replace.path) == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(replace.path, "go.mod")); err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("stat local replace %s: %v", replace.path, err)
	}
	edit := exec.Command("go", "mod", "edit", "-replace", replace.module+"="+replace.path)
	edit.Dir = root
	edit.Env = fixtureGoEnv(nil)
	output, err := edit.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod edit replace failed for %s: %v\n%s", replace.module, err, strings.TrimSpace(string(output)))
	}
}

func fixtureGoEnv(extra map[string]string) []string {
	env := append(os.Environ(),
		"GOCACHE=/tmp/gocache",
		"GOMODCACHE=/tmp/gomodcache",
	)
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}

func runFixtureGoModTidy(t *testing.T, root string, extra map[string]string) {
	t.Helper()

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = root
	cmd.Env = fixtureGoEnv(extra)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func runFixtureGoTest(t *testing.T, root, pkg, run string, extra map[string]string) string {
	t.Helper()

	args := []string{"test", pkg, "-count=1"}
	if strings.TrimSpace(run) != "" {
		args = append(args, "-run", run)
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = fixtureGoEnv(extra)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test %s failed: %v\n%s", strings.Join(args[1:], " "), err, strings.TrimSpace(string(output)))
	}
	return string(output)
}

func assertFixtureGoModContains(t *testing.T, root string, modules ...string) {
	t.Helper()

	goModAfter, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod after tidy: %v", err)
	}
	source := string(goModAfter)
	for _, module := range modules {
		if !strings.Contains(source, module) {
			t.Fatalf("expected go.mod to contain %s after tidy", module)
		}
	}
}

func queueLocalReplaces(t *testing.T) []fixtureReplace {
	t.Helper()

	repoRoot := repoRoot(t)
	return []fixtureReplace{
		{module: "github.com/goforj/queue", path: filepath.Join(repoRoot, "..", "queue")},
		{module: "github.com/goforj/queue/driver/redisqueue", path: filepath.Join(repoRoot, "..", "queue", "driver", "redisqueue")},
	}
}

func cacheLocalReplaces(t *testing.T) []fixtureReplace {
	t.Helper()

	repoRoot := repoRoot(t)
	cacheRoot := filepath.Join(repoRoot, "..", "cache")
	return []fixtureReplace{
		{module: "github.com/goforj/cache", path: cacheRoot},
		{module: "github.com/goforj/cache/cachecore", path: filepath.Join(cacheRoot, "cachecore")},
		{module: "github.com/goforj/cache/cachetest", path: filepath.Join(cacheRoot, "cachetest")},
		{module: "github.com/goforj/cache/driver/dynamocache", path: filepath.Join(cacheRoot, "driver", "dynamocache")},
		{module: "github.com/goforj/cache/driver/memcachedcache", path: filepath.Join(cacheRoot, "driver", "memcachedcache")},
		{module: "github.com/goforj/cache/driver/mysqlcache", path: filepath.Join(cacheRoot, "driver", "mysqlcache")},
		{module: "github.com/goforj/cache/driver/natscache", path: filepath.Join(cacheRoot, "driver", "natscache")},
		{module: "github.com/goforj/cache/driver/postgrescache", path: filepath.Join(cacheRoot, "driver", "postgrescache")},
		{module: "github.com/goforj/cache/driver/rediscache", path: filepath.Join(cacheRoot, "driver", "rediscache")},
		{module: "github.com/goforj/cache/driver/sqlcore", path: filepath.Join(cacheRoot, "driver", "sqlcore")},
		{module: "github.com/goforj/cache/driver/sqlitecache", path: filepath.Join(cacheRoot, "driver", "sqlitecache")},
	}
}

func mustTempGeneratedModuleRoot(t *testing.T, pattern, packageDir string) string {
	t.Helper()

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, pattern)
	if err != nil {
		t.Fatalf("mkdir temp module root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if strings.TrimSpace(packageDir) != "" {
		if err := os.MkdirAll(filepath.Join(root, packageDir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", packageDir, err)
		}
	}
	return root
}

func fixtureModuleSpec(module string, required []string, pinned []string, replaces []fixtureReplace) fixtureGoModSpec {
	return fixtureGoModSpec{
		module:   module,
		required: required,
		pinned:   pinned,
		replaces: replaces,
	}
}

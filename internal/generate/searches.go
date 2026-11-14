package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"go/token"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/goforj/str/v2"
)

var searchLocalDrivers = []string{"bleve"}

var searchRootKeys = []string{"DRIVER", "PATH", "URL", "API_KEY", "PREFIX"}
var searchDriverKeys = map[string]map[string]struct{}{
	"bleve":       makeSet("PATH"),
	"meilisearch": makeSet("URL", "API_KEY", "PREFIX"),
}

// searchTemplateData keeps the generated import inventory and named accessors consistent.
type searchTemplateData struct {
	Bleve            bool
	Meilisearch      bool
	Names            []storageAccessorName
	AppPrefixes      []string
	ResourcePrefixes []string
}

// GenerateSearchFiles generates portable search composition from the current environment contract.
func GenerateSearchFiles(projectDir string) (int, error) {
	return generateSearchFiles(ambientGenerationInput(projectDir))
}

// generateSearchFiles validates driver selection before publishing either generated file.
func generateSearchFiles(input generationInput) (int, error) {
	input = canonicalSearchGenerationInput(input)
	if err := validatePrimitiveEnv(input, primitiveEnvContract{
		Prefix: "SEARCH", DefaultDriver: "bleve", LocalDrivers: searchLocalDrivers, RootKeys: searchRootKeys,
		CommonKeys: makeSet("DRIVER"), DriverKeys: searchDriverKeys,
		InheritRootDriver: true, AllowInactiveRootKeys: true, AllowInactiveNamedKeys: true,
		ChildNames: func(environment generationEnvironment) []string {
			return searchScopedChildNames(input, "")
		},
	}); err != nil {
		return 0, err
	}
	active := map[string]struct{}{effectivePrimitiveDriver(input.environment.Get("SEARCH_DRIVER", "bleve"), "bleve"): {}}
	for _, name := range searchScopedChildNames(input, "") {
		active[effectivePrimitiveDriver(input.environment.Get("SEARCH_"+name+"_DRIVER", input.environment.Get("SEARCH_DRIVER", "bleve")), "bleve")] = struct{}{}
	}
	for _, choice := range appPrefixedActiveDrivers(input, "SEARCH", "bleve", true) {
		active[choice.driver] = struct{}{}
	}
	drivers, err := supportedDrivers(input.environment, "SEARCH", searchDriverKeys, sortStrings(active), searchLocalDrivers)
	if err != nil {
		return 0, err
	}
	data := searchTemplateData{AppPrefixes: generationKnownAppEnvPrefixes(input), ResourcePrefixes: generationResourceEnvPrefixes()}
	for _, driver := range drivers {
		data.Bleve = data.Bleve || driver == "bleve"
		data.Meilisearch = data.Meilisearch || driver == "meilisearch"
	}
	methods := makeSet("Index", "Close", "Instances")
	aliases := makeSet()
	for _, name := range discoverPrimitiveChildNames(input, "SEARCH", searchRootKeys) {
		alias := searchConfigName(name)
		if _, exists := aliases[alias]; exists {
			continue
		}
		aliases[alias] = struct{}{}
		method := str.Of(alias).Pascal().String()
		if !token.IsIdentifier(method) || !token.IsExported(method) {
			return 0, fmt.Errorf("search accessor %q must be an exported Go identifier", method)
		}
		if _, reserved := methods[method]; reserved {
			return 0, fmt.Errorf("search accessor %q conflicts with another generated method", method)
		}
		methods[method] = struct{}{}
		data.Names = append(data.Names, storageAccessorName{Method: method, Disk: strings.ToLower(alias)})
	}
	outputs := []struct{ path, source string }{{SearchManagerPath, searchManagerSource}, {SearchAccessorsPath, searchAccessorsSource}}
	bodies := make([][]byte, len(outputs))
	for index, output := range outputs {
		tmpl, err := template.New(output.path).Parse(output.source)
		if err != nil {
			return 0, err
		}
		var buffer bytes.Buffer
		if err := tmpl.Execute(&buffer, data); err != nil {
			return 0, err
		}
		source, err := format.Source(buffer.Bytes())
		if err != nil {
			return 0, fmt.Errorf("format search source: %w", err)
		}
		bodies[index] = source
	}
	written := 0
	for index, output := range outputs {
		changed, err := writeGeneratedSource(filepath.Join(input.projectDir, output.path), bodies[index])
		if err != nil {
			return written, err
		}
		if changed {
			written++
		}
	}
	return written, nil
}

// searchConfigName matches declared and environmental aliases without changing physical index names.
func searchConfigName(name string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.ToLower(name)), "-", "_")
}

// canonicalSearchGenerationInput mirrors runtime precedence when canonical and exact-name legacy settings coexist.
func canonicalSearchGenerationInput(input generationInput) generationInput {
	values := make(map[string]string, len(input.environment.values))
	prefixes := generationKnownAppEnvPrefixes(input)
	for _, entry := range input.environment.Entries() {
		owner := appResourceEnvironmentOwner(entry.key, prefixes)
		scope := "SEARCH_"
		if owner != "" {
			scope = owner + "_" + scope
		}
		if relative, exists := strings.CutPrefix(entry.key, scope); exists {
			if scoped, valid := splitScopedEnvKey(relative, searchRootKeys); valid && scoped.child != "" {
				canonical := scope + searchConfigName(scoped.child) + "_" + scoped.rootKey
				if canonical != entry.key && appResourceEnvironmentOwner(canonical, prefixes) == owner {
					if _, present := input.environment.Lookup(canonical); present {
						continue
					}
				}
			}
		}
		values[entry.key] = entry.value
	}
	input.environment = generationEnvironment{values: values}
	return input
}

// searchScopedChildNames recognizes exact Search settings after assigning their App ownership.
func searchScopedChildNames(input generationInput, prefix string) []string {
	return appScopedChildNames(input, "SEARCH", prefix, searchRootKeys)
}

const searchAccessorsSource = `// Code generated by forj generate --search. DO NOT EDIT.
package searches

import "github.com/goforj/search"

// Index resolves an exact App declaration before its environment alias and fails fast for invalid wiring.
func (manager *Manager) Index(name string) *search.Index {
 resource, exists := manager.indexes[name]
 if !exists { resource, exists = manager.indexes[manager.aliases[searchConfigName(name)]] }
 if !exists { panic("search: index not declared for this App: " + name) }
 return resource.index
}
{{range .Names}}
// {{.Method}} returns the App-owned {{.Disk}} index.
func (manager *Manager) {{.Method}}() *search.Index { return manager.Index("{{.Disk}}") }
{{end}}
`

const searchManagerSource = `// Code generated by forj generate --search. DO NOT EDIT.
// Package searches composes rebuildable lexical indexes from App-owned Go definitions.
package searches

import (
 "context"
 "errors"
 "fmt"
 "os"
 {{if .Bleve}}"path/filepath"{{end}}
 "sort"
 "strings"
 "sync"

 "github.com/goforj/env/v2"
 "github.com/goforj/search"
 {{if .Bleve}}"github.com/goforj/search/driver/blevesearch"{{end}}
 {{if .Meilisearch}}"github.com/goforj/search/driver/meilisearch"{{end}}
)

// searchAppPrefixes fixes App ownership when this composition is generated.
var searchAppPrefixes = []string{ {{range .AppPrefixes}}{{printf "%q" .}},{{end}} }

// searchResourcePrefixes keeps ownership aligned with the framework resource catalog.
var searchResourcePrefixes = []string{ {{range .ResourcePrefixes}}{{printf "%q" .}},{{end}} }

// indexBackend keeps schema operations outside the portable request-path handle.
type indexBackend interface {
 search.Backend
 CreateIndex(context.Context, search.Definition) error
 ValidateIndex(context.Context, search.Definition) error
 Close() error
}

// indexResource owns exactly one backend for one logical schema.
type indexResource struct { index *search.Index; backend indexBackend; driver string }

// Manager owns the App's declared indexes and closes each backend once.
type Manager struct {
 indexes map[string]indexResource
 aliases map[string]string
 closeOnce sync.Once
 closeErr error
}

// Instance exposes bounded resource identity without leaking configuration or credentials.
type Instance struct { Name string; Driver string }

// NewManager validates provisioned schemas without creating or altering them.
// An empty definition map creates an inert manager. Named environment settings require matching App definitions.
func NewManager(ctx context.Context, definitions map[string]search.Definition) (*Manager, error) {
 return NewManagerForApp(ctx, "", definitions)
}

// NewManagerForApp validates only the named App's declarations using explicit backend overrides.
// Prefix is the existing environment prefix, for example ADMIN; unrelated default-App indexes remain untouched.
func NewManagerForApp(ctx context.Context, prefix string, definitions map[string]search.Definition) (*Manager, error) {
 return openManager(ctx, prefix, definitions, false)
}

// Provision explicitly creates declared schemas and releases all backend ownership before returning.
// Existing incompatible schemas are rejected; changing a schema requires an application-owned rebuild into a new location.
func Provision(ctx context.Context, definitions map[string]search.Definition) error {
 return ProvisionForApp(ctx, "", definitions)
}

// ProvisionForApp explicitly provisions one named App without initializing any App runtime.
func ProvisionForApp(ctx context.Context, prefix string, definitions map[string]search.Definition) error {
 manager, err := openManager(ctx, prefix, definitions, true)
 if err != nil { return err }
 return manager.Close()
}

// openManager validates all definitions before opening independently owned backend instances.
func openManager(ctx context.Context, prefix string, definitions map[string]search.Definition, provision bool) (*Manager, error) {
 if err := ctx.Err(); err != nil { return nil, err }
 prefix = strings.ToUpper(strings.TrimSpace(prefix))
 names := make([]string, 0, len(definitions))
 for name := range definitions { names = append(names, name) }
 sort.Strings(names)
 aliases := make(map[string]string, len(names))
 for _, name := range names {
  definition := definitions[name]
  if name != string(definition.Name) { return nil, fmt.Errorf("search: definition name does not match index %q", name) }
  if err := definition.Validate(); err != nil { return nil, fmt.Errorf("search: invalid definition %q: %w", name, err) }
  alias := searchConfigName(name)
  if previous, exists := aliases[alias]; exists { return nil, fmt.Errorf("search: index names %q and %q share environment/accessor alias %q", previous, name, alias) }
  aliases[alias] = name
 }
 for _, name := range configuredSearchNames(prefix) {
  if _, exists := aliases[name]; !exists { return nil, fmt.Errorf("search: environment index %q has no App definition", strings.ToLower(name)) }
 }
 manager := &Manager{indexes: make(map[string]indexResource, len(names)), aliases: aliases}
 for _, name := range names {
  driverName := searchDriverName(prefix, name)
  backend, err := openBackend(ctx, name, driverName, prefix)
  if err != nil { return nil, errors.Join(fmt.Errorf("search: open index %q: %w", name, err), manager.Close()) }
  index, err := search.NewIndex(backend, definitions[name])
  if err != nil { return nil, errors.Join(err, backend.Close(), manager.Close()) }
  manager.indexes[name] = indexResource{index: index, backend: backend, driver: driverName}
  if provision { err = backend.CreateIndex(ctx, index.Definition()) } else { err = backend.ValidateIndex(ctx, index.Definition()) }
  if err != nil { return nil, errors.Join(fmt.Errorf("search: schema for index %q: %w", name, err), manager.Close()) }
 }
 return manager, nil
}

// openBackend separates compiled capability from runtime driver selection and physical index identity.
func openBackend(ctx context.Context, name, driver, prefix string) (indexBackend, error) {
 switch driver {
 {{if .Bleve}}case "bleve":
  root := searchValue(prefix, "", "PATH", "storage/search")
  path := namedSearchValue(prefix, name, "PATH", filepath.Join(root, name))
  return blevesearch.Open(ctx, blevesearch.Config{Path: path})
 {{end}}
 {{if .Meilisearch}}case "meilisearch":
  return meilisearch.New(meilisearch.Config{URL: searchValue(prefix, name, "URL", ""), APIKey: searchValue(prefix, name, "API_KEY", ""), Prefix: searchValue(prefix, name, "PREFIX", "")})
 {{end}}
 default:
  return nil, fmt.Errorf("search: driver %q is not compiled; update SEARCH_SUPPORTED_DRIVERS and run forj generate --search", driver)
 }
}

// searchEnvironmentOwner assigns ambiguous keys to the longest compiled App prefix.
func searchEnvironmentOwner(key, requestedPrefix string) string {
 owner := ""
 for _, prefix := range searchAppPrefixes {
  if len(prefix)>len(owner) && searchAppResourceKey(key,prefix) { owner=prefix }
 }
 if len(requestedPrefix)>len(owner) && searchAppResourceKey(key,requestedPrefix) { owner=requestedPrefix }
 return owner
}

// searchAppResourceKey recognizes resource settings beneath an explicitly known App prefix.
func searchAppResourceKey(key,prefix string) bool {
 for _,resourcePrefix:=range searchResourcePrefixes {
  if strings.HasPrefix(key,prefix+"_"+resourcePrefix+"_"){return true}
 }
 return false
}

// lookupSearchEnvironment prevents another App's root settings from becoming named index overrides.
func lookupSearchEnvironment(prefix, key string) (string, bool) {
 if searchEnvironmentOwner(key, prefix) != prefix { return "", false }
 return os.LookupEnv(key)
}

// searchEnvironmentValue preserves non-empty environment fallback within one owning App.
func searchEnvironmentValue(prefix, key, fallback string) string {
 if value, exists := lookupSearchEnvironment(prefix, key); exists && value != "" { return value }
 return fallback
}

// configuredSearchNames validates exact root and named keys without interpreting unrelated application settings.
func configuredSearchNames(prefix string) []string {
 scope := "SEARCH_"
 if prefix != "" { scope = prefix+"_SEARCH_" }
 names := map[string]struct{}{}
 keys := []string{"DRIVER", "PATH", "URL", "API_KEY", "PREFIX", "SUPPORTED_DRIVERS"}
 for _, entry := range os.Environ() {
  key, _, _ := strings.Cut(entry, "=")
  if searchEnvironmentOwner(key, prefix) != prefix || !strings.HasPrefix(key, scope) { continue }
  relative := strings.TrimPrefix(key, scope)
  root := false
  for _, suffix := range keys { if relative == suffix { root = true; break } }
  if root { continue }
  for _, suffix := range keys {
   if suffix == "SUPPORTED_DRIVERS" { continue }
   if name, found := strings.CutSuffix(relative, "_"+suffix); found && name != "" {
    names[searchConfigName(name)] = struct{}{}
    break
   }
  }
 }
 result := make([]string, 0, len(names))
 for name := range names { result = append(result, name) }
 sort.Strings(result)
 return result
}

// searchConfigName matches case-insensitive environment and accessor aliases while preserving declared backend identity.
func searchConfigName(name string) string {
 return strings.ReplaceAll(strings.ToUpper(strings.ToLower(name)), "-", "_")
}

// normalizedSearchDriver preserves the generator's fallback for empty or whitespace-only selections.
func normalizedSearchDriver(value, fallback string) string {
 driver := strings.ToLower(strings.TrimSpace(value))
 if driver == "" { return fallback }
 return driver
}

// searchDriverName mirrors generated root, named, and App selection without rewriting process environment.
func searchDriverName(prefix, name string) string {
 root := normalizedSearchDriver(env.Get("SEARCH_DRIVER", ""), "bleve")
 driver := root
 appRoot := root
 if prefix != "" {
  if value, exists := lookupSearchEnvironment(prefix, prefix+"_SEARCH_DRIVER"); exists { appRoot = normalizedSearchDriver(value, "bleve") }
  driver = appRoot
 }
 if value, exists := lookupNamedSearchEnvironment("", name, "DRIVER"); exists { driver = normalizedSearchDriver(value, root) }
 if prefix != "" {
  if value, exists := lookupNamedSearchEnvironment(prefix, name, "DRIVER"); exists { driver = normalizedSearchDriver(value, appRoot) }
 }
 return driver
}

// searchValue resolves App root defaults before any more-specific named resource settings.
func searchValue(prefix, name, key, fallback string) string {
 value := searchEnvironmentValue("", "SEARCH_"+key, fallback)
 if prefix != "" { value = searchEnvironmentValue(prefix, prefix+"_SEARCH_"+key, value) }
 if name == "" { return value }
 return namedSearchValue(prefix, name, key, value)
}

// namedSearchValue preserves explicit named overrides without conflating a root directory with one index path.
func namedSearchValue(prefix, name, key, fallback string) string {
 value := fallback
 if current,exists:=lookupNamedSearchEnvironment("",name,key);exists && current!="" { value=current }
 if prefix!="" { if current,exists:=lookupNamedSearchEnvironment(prefix,name,key);exists && current!="" { value=current } }
 return value
}

// lookupNamedSearchEnvironment prefers portable underscore keys while retaining existing exact-name hyphen overrides.
func lookupNamedSearchEnvironment(prefix,name,key string) (string,bool) {
 scope:="SEARCH_"
 if prefix!="" { scope=prefix+"_"+scope }
 canonical:=scope+searchConfigName(name)+"_"+key
 if value,exists:=lookupSearchEnvironment(prefix,canonical);exists { return value,true }
 original:=scope+strings.ToUpper(name)+"_"+key
 if original!=canonical { return lookupSearchEnvironment(prefix,original) }
 return "",false
}

// Instances returns deterministic identity metadata for inspection.
func (manager *Manager) Instances() []Instance {
 names := make([]string, 0, len(manager.indexes))
 for name := range manager.indexes { names = append(names, name) }
 sort.Strings(names)
 instances := make([]Instance, 0, len(names))
 for _, name := range names { instances = append(instances, Instance{Name: name, Driver: manager.indexes[name].driver}) }
 return instances
}

// Close releases every backend even when another index reports a shutdown failure.
func (manager *Manager) Close() error {
 manager.closeOnce.Do(func() {
  for _, instance := range manager.Instances() { manager.closeErr = errors.Join(manager.closeErr, manager.indexes[instance.Name].backend.Close()) }
 })
 return manager.closeErr
}
`

// discoverSearchNames keeps configuration tooling aligned with the same App-scoped generator inputs.
func discoverSearchNames(input generationInput) []string {
	names := discoverPrimitiveChildNames(input, "SEARCH", searchRootKeys)
	for index := range names {
		names[index] = strings.ToLower(names[index])
	}
	return names
}

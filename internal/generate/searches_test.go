package generate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestGenerateSearchFilesValidatesBeforeWriting covers disabled selections, unsupported drivers, invalid settings, and conflicting accessor names.
func TestGenerateSearchFilesValidatesBeforeWriting(t *testing.T) {
	if err := validateRequestedGenerationSelection(GenerationSelection{Search: true}, GenerationSelection{}); err == nil {
		t.Fatal("disabled Search accepted generation")
	}
	for _, test := range []struct {
		name   string
		values map[string]string
		want   string
	}{
		{"unknown", map[string]string{"SEARCH_DRIVER": "unknown"}, "unsupported driver"},
		{"not-compiled", map[string]string{"SEARCH_DRIVER": "meilisearch", "SEARCH_SUPPORTED_DRIVERS": "bleve"}, "not enabled"},
		{"wrong-setting", map[string]string{"SEARCH_DRIVER": "bleve", "SEARCH_CATALOG_URL": "https://example.invalid"}, "URL"},
		{"reserved-accessor", map[string]string{"SEARCH_INDEX_PATH": "index"}, "conflicts"},
		{"numeric-accessor", map[string]string{"SEARCH_123_PATH": "index"}, "accessor"},
		{"normalized-collision", map[string]string{"SEARCH_FOO_BAR_PATH": "first", "SEARCH_FOO__BAR_PATH": "second"}, "conflicts"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			originals := map[string]string{SearchManagerPath: "original manager", SearchAccessorsPath: "original accessors"}
			for path, body := range originals {
				target := filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := generateSearchFiles(generationInput{projectDir: root, environment: generationEnvironment{values: test.values}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("generation error = %v, want %q", err, test.want)
			}
			for path, original := range originals {
				body, err := os.ReadFile(filepath.Join(root, path))
				if err != nil || string(body) != original {
					t.Fatalf("invalid input changed %s: %q (%v)", path, body, err)
				}
			}
		})
	}
}

// TestGenerateSearchFilesKeepsAppNamesAndDriversScoped verifies shared output excludes disabled App overlays and generation is idempotent.
func TestGenerateSearchFilesKeepsAppNamesAndDriversScoped(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("render:\n  components: [cli, search]\napps:\n  admin:\n    components: [cli, search]\n  dormant:\n    components: [cli]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"SEARCH_DRIVER": "bleve", "SEARCH_CATALOG_PATH": "catalog", "ADMIN_SEARCH_ARCHIVE_DRIVER": "meilisearch", "ADMIN_SEARCH_ARCHIVE_URL": "https://search.example", "SEARCH_SUPPORTED_DRIVERS": "bleve,meilisearch", "DORMANT_SEARCH_PRIVATE_DRIVER": "invalid"}
	input := generationInput{projectDir: root, environment: generationEnvironment{values: values}, appPrefixes: []string{"ADMIN", "DORMANT"}}
	if _, err := generateSearchFiles(input); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(root, SearchAccessorsPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"Catalog() *search.Index", "Archive() *search.Index"} {
		if !strings.Contains(string(source), method) {
			t.Fatalf("missing accessor %s", method)
		}
	}
	if strings.Contains(string(source), "Private()") {
		t.Fatal("disabled App leaked accessor")
	}
	if changed, err := generateSearchFiles(input); err != nil || changed != 0 {
		t.Fatalf("second generation changed %d files: %v", changed, err)
	}
	names := ResourceNames(root, values)
	if !slices.Contains(names[project.ResourceSearch], "catalog") {
		t.Fatal("configuration inventory lost catalog index")
	}
	if selection := GenerationSelectionFromComponents(project.Components{Search: true}); !selection.Search {
		t.Fatal("Search not selected")
	}
}

// TestSearchAppRootDriverPrecedence keeps validation and compiled inventory aligned when named driver assignments are absent.
func TestSearchAppRootDriverPrecedence(t *testing.T) {
	for _, test := range []struct {
		name, named, appNamed, option, want string
		invalid                             bool
	}{
		{name: "App root inherited", option: "URL", want: "meilisearch"},
		{name: "global named overrides App root", named: "bleve", option: "PATH", want: "bleve"},
		{name: "App named overrides global named", named: "meilisearch", appNamed: "bleve", option: "PATH", want: "bleve"},
		{name: "App root rejects Bleve option", option: "PATH", want: "meilisearch", invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("render:\n  components: [cli, search]\napps:\n  admin:\n    components: [cli, search]\n"), 0600); err != nil {
				t.Fatal(err)
			}
			values := map[string]string{"SEARCH_DRIVER": "bleve", "ADMIN_SEARCH_DRIVER": "meilisearch", "SEARCH_SUPPORTED_DRIVERS": "bleve,meilisearch", "ADMIN_SEARCH_CATALOG_" + test.option: "configured"}
			if test.named != "" {
				values["SEARCH_CATALOG_DRIVER"] = test.named
			}
			if test.appNamed != "" {
				values["ADMIN_SEARCH_CATALOG_DRIVER"] = test.appNamed
			}
			input := fixtureGenerationInput(root, values)
			if _, err := generateSearchFiles(input); (err != nil) != test.invalid {
				t.Fatalf("generate = %v", err)
			}
			if test.invalid {
				if _, err := os.Stat(filepath.Join(root, SearchManagerPath)); !os.IsNotExist(err) {
					t.Fatal("invalid driver option published output")
				}
			}
			found := false
			for _, choice := range appPrefixedActiveDrivers(input, "SEARCH", "bleve", true) {
				if choice.key == "ADMIN_SEARCH_CATALOG_DRIVER" {
					found = true
					if choice.driver != test.want {
						t.Fatalf("effective driver = %s, want %s", choice.driver, test.want)
					}
				}
			}
			if !found {
				t.Fatal("inherited driver missing from active inventory")
			}
		})
	}
}

// TestGeneratedSearchManagerBehavior exercises real Bleve schemas and explicit provisioning in an independent consumer module.
func TestGeneratedSearchManagerBehavior(t *testing.T) {
	root := mustTempGeneratedModuleRoot(t, "forj-search-generation-*", filepath.Join("internal", "searches"))
	writeFixtureGoMod(t, root, fixtureModuleSpec("example.com/searchgeneration", []string{"github.com/goforj/search", "github.com/goforj/search/driver/blevesearch", "github.com/goforj/env/v2"}, nil, nil))
	input := generationInput{projectDir: root, environment: generationEnvironment{values: map[string]string{"SEARCH_DRIVER": "bleve", "SEARCH_SUPPORTED_DRIVERS": "bleve,meilisearch", "SEARCH_CATALOG_DRIVER": "bleve", "SEARCH_ARCHIVE_DRIVER": "bleve"}}, appPrefixes: []string{"SEARCH", "SEARCH_WORKER", "SEARCH_SEARCH"}}
	if _, err := generateSearchFiles(input); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal/searches/manager_test.go"), []byte(searchManagerBehaviorTest), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/searches", "TestSearchManager", nil)
}

// TestSearchDriverManifests compiles each advertised driver independently without requiring an external service.
func TestSearchDriverManifests(t *testing.T) {
	for _, driver := range []string{"bleve", "meilisearch"} {
		t.Run(driver, func(t *testing.T) {
			root := mustTempGeneratedModuleRoot(t, "forj-search-manifest-*", filepath.Join("internal", "searches"))
			writeFixtureGoMod(t, root, fixtureModuleSpec("example.com/searchmanifest", []string{"github.com/goforj/search", "github.com/goforj/search/driver/blevesearch", "github.com/goforj/env/v2"}, nil, nil))
			input := fixtureGenerationInput(root, map[string]string{"SEARCH_DRIVER": driver, "SEARCH_SUPPORTED_DRIVERS": driver})
			if _, err := generateSearchFiles(input); err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(filepath.Join(root, SearchManagerPath))
			if err != nil {
				t.Fatal(err)
			}
			forbidden := "github.com/goforj/search/driver/meilisearch"
			if driver == "meilisearch" {
				forbidden = "github.com/goforj/search/driver/postgressearch"
			}
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("disabled driver import %q", forbidden)
			}
			runFixtureGoModTidy(t, root, nil)
			runFixtureGoTest(t, root, "./internal/searches", "TestDoesNotExist", nil)
		})
	}
}

// TestSearchDefaultResourceWordIndex preserves an unambiguous default index when no App owns its prefix.
func TestSearchDefaultResourceWordIndex(t *testing.T) {
	root := t.TempDir()
	input := generationInput{projectDir: root, environment: generationEnvironment{values: map[string]string{"SEARCH_SEARCH_PATH": "search-index"}}}
	if _, err := generateSearchFiles(input); err != nil {
		t.Fatal(err)
	}
	if names := discoverSearchNames(input); !slices.Equal(names, []string{"search"}) {
		t.Fatalf("default index inventory = %v", names)
	}
	body, err := os.ReadFile(filepath.Join(root, SearchAccessorsPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Search() *search.Index") {
		t.Fatalf("default index accessor missing: %s", body)
	}
}

// TestSearchResourceWordAppOwnership excludes known App root keys from the default index inventory.
func TestSearchResourceWordAppOwnership(t *testing.T) {
	root := t.TempDir()
	config := "render:\n  components: [cli, search]\napps:\n  search:\n    components: [cli, search]\n  search-worker:\n    components: [cli, search]\n  search-search:\n    components: [cli, search]\n"
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"SEARCH_DRIVER": "bleve", "SEARCH_SEARCH_DRIVER": "bleve", "SEARCH_SEARCH_PATH": "search-root", "SEARCH_WORKER_SEARCH_DRIVER": "bleve", "SEARCH_WORKER_SEARCH_PATH": "worker-root", "SEARCH_SEARCH_SEARCH_DRIVER": "bleve", "SEARCH_SEARCH_SEARCH_PATH": "nested-root", "SEARCH_CATALOG_PATH": "catalog", "SEARCH_SEARCH_ARCHIVE_PATH": "archive"}
	input := fixtureGenerationInput(root, values)
	if _, err := generateSearchFiles(input); err != nil {
		t.Fatal(err)
	}
	names := discoverSearchNames(input)
	if !slices.Equal(names, []string{"archive", "catalog"}) {
		t.Fatalf("index inventory = %v", names)
	}
	body, err := os.ReadFile(filepath.Join(root, SearchAccessorsPath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "Search()") || strings.Contains(string(body), "WorkerSearch()") {
		t.Fatalf("synthetic App accessor: %s", body)
	}
	if changed, err := generateSearchFiles(input); err != nil || changed != 0 {
		t.Fatalf("regeneration changed %d files: %v", changed, err)
	}
}

const searchManagerBehaviorTest = `// Package searches owns generated search resources.
package searches

import (
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"

 "github.com/goforj/search"
)

// definitions declares schemas separately from physical backend locations.
func definitions() map[string]search.Definition {
 result:=map[string]search.Definition{}
 for _,name:=range []string{"catalog","archive"} {
  result[name]=search.Definition{Name:search.IndexName(name),Fields:[]search.Field{{Name:"description",Type:search.TextField,Searchable:true}}}
 }
 return result
}

// failingCloseBackend isolates cleanup failure reporting from backend search behavior.
type failingCloseBackend struct { indexBackend; calls int; failure error }

// Close records exactly-once ownership release while retaining its failure.
func (backend *failingCloseBackend) Close() error { backend.calls++; return backend.failure }

// TestSearchManagerCloseRetainsFailures proves every owned backend is attempted once.
func TestSearchManagerCloseRetainsFailures(t *testing.T) {
 failure:=errors.New("backend cleanup failed")
 first:=&failingCloseBackend{failure:failure};second:=&failingCloseBackend{}
 manager:=&Manager{indexes:map[string]indexResource{"first":{backend:first},"second":{backend:second}}}
 for attempt:=0;attempt<2;attempt++{if err:=manager.Close();!errors.Is(err,failure){t.Fatalf("cleanup failure lost: %v",err)}}
 if first.calls!=1 || second.calls!=1 {t.Fatalf("cleanup calls: %d, %d",first.calls,second.calls)}
}

// TestSearchManager proves non-destructive schema validation, wrong-index isolation, cleanup, and caller context.
func TestSearchManager(t *testing.T) {
 ctx:=context.Background()
 t.Setenv("SEARCH_DRIVER","bleve")
 t.Setenv("SEARCH_SUPPORTED_DRIVERS","bleve,meilisearch")
 t.Setenv("SEARCH_PATH",t.TempDir())
 for _,key:=range []string{"SEARCH_CATALOG_DRIVER","SEARCH_ARCHIVE_DRIVER","SEARCH_CATALOG_PATH","SEARCH_ARCHIVE_PATH"} {t.Setenv(key,""); _ = os.Unsetenv(key)}
 empty,err:=NewManager(ctx,nil)
 if err!=nil {t.Fatal(err)}
 if len(empty.Instances())!=0 {t.Fatal("empty definitions created indexes")}
 if err:=empty.Close();err!=nil {t.Fatal(err)}
 defs:=definitions()
 if _,err:=NewManager(ctx,defs);!errors.Is(err,search.ErrIndexNotFound){t.Fatalf("startup auto-provisioned: %v",err)}
 if err:=Provision(ctx,defs);err!=nil{t.Fatal(err)}
 manager,err:=NewManager(ctx,defs);if err!=nil{t.Fatal(err)}
 receipt,err:=search.Upsert(ctx,manager.Catalog(),"one",map[string]string{"description":"red trail boot"});if err!=nil{t.Fatal(err)}
 outcome,err:=manager.Catalog().Wait(ctx,receipt);if err!=nil || outcome.State!=search.OutcomeSucceeded{t.Fatalf("visibility: %#v %v",outcome,err)}
 page,err:=manager.Archive().Search(ctx,search.Query{Text:"red"});if err!=nil || page.Total.Value!=0{t.Fatalf("wrong index: %#v %v",page,err)}
 if len(manager.Instances())!=2 || manager.Instances()[0].Name!="archive"{t.Fatal("unstable instances")}
 if err:=manager.Close();err!=nil{t.Fatal(err)}
 if err:=manager.Close();err!=nil{t.Fatal(err)}
 if _,err:=manager.Catalog().Search(ctx,search.Query{Text:"red"});!errors.Is(err,search.ErrClosed){t.Fatalf("index alive after close: %v",err)}
 def:=defs["catalog"];def.Fields=append(def.Fields,search.Field{Name:"category",Type:search.KeywordField});defs["catalog"]=def
 if _,err:=NewManager(ctx,defs);!errors.Is(err,search.ErrSchemaDrift){t.Fatalf("schema drift: %v",err)}
 if err:=Provision(ctx,defs);!errors.Is(err,search.ErrSchemaDrift){t.Fatalf("provision overwrote drift: %v",err)}
 original,err:=NewManager(ctx,definitions());if err!=nil{t.Fatalf("drift destroyed index: %v",err)};_ = original.Close()
 canceled,cancel:=context.WithCancel(ctx);cancel()
 if _,err:=NewManager(canceled,defs);!errors.Is(err,context.Canceled){t.Fatalf("cancellation: %v",err)}
 if err:=Provision(canceled,defs);!errors.Is(err,context.Canceled){t.Fatalf("provision cancellation: %v",err)}
 bad:=definitions();bad["catalog"]=search.Definition{Name:"wrong"}
 if _,err:=NewManager(ctx,bad);err==nil{t.Fatal("definition name mismatch accepted")}
 bad=definitions();def=bad["catalog"];def.Fields=nil;bad["catalog"]=def
 if _,err:=NewManager(ctx,bad);err==nil{t.Fatal("invalid definition accepted")}
 t.Setenv("SEARCH_CATALOG_PATH",filepath.Join(t.TempDir(),"missing"))
 if _,err:=NewManager(ctx,nil);err==nil{t.Fatal("configured index without definition accepted")}
 t.Setenv("SEARCH_CATALOG_PATH","")
 t.Setenv("SEARCH_CATALOG_DRIVER","unknown")
 if _,err:=NewManager(ctx,definitions());err==nil{t.Fatal("unknown runtime driver accepted")}
 t.Setenv("SEARCH_CATALOG_DRIVER","bleve")
 reopened,err:=NewManager(ctx,definitions());if err!=nil{t.Fatalf("failed construction leaked ownership: %v",err)};_ = reopened.Close()
 // Index requires an actual App declaration, even if another App generated its accessor.
 defer func(){if recover()==nil {t.Fatal("missing App index did not fail fast")}}()
 empty.Index("missing")
}

// TestSearchManagerAppIsolation initializes independent default and named App schemas against real Bleve owners.
func TestSearchManagerAppIsolation(t *testing.T) {
 ctx:=context.Background()
 root:=t.TempDir()
 t.Setenv("SEARCH_DRIVER","bleve")
 t.Setenv("SEARCH_SUPPORTED_DRIVERS","bleve,meilisearch")
 t.Setenv("SEARCH_CATALOG_DRIVER","bleve")
 t.Setenv("SEARCH_CATALOG_PATH",filepath.Join(root,"catalog"))
 t.Setenv("ADMIN_SEARCH_ARCHIVE_DRIVER","bleve")
 t.Setenv("ADMIN_SEARCH_ARCHIVE_PATH",filepath.Join(root,"archive"))
 defaults:=map[string]search.Definition{"catalog":definitions()["catalog"]}
 admin:=map[string]search.Definition{"archive":definitions()["archive"]}
 if err:=Provision(ctx,defaults);err!=nil{t.Fatal(err)}
 if err:=ProvisionForApp(ctx,"ADMIN",admin);err!=nil{t.Fatal(err)}
 defaultManager,err:=NewManager(ctx,defaults);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){_ = defaultManager.Close()})
 adminManager,err:=NewManagerForApp(ctx," admin ",admin);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){_ = adminManager.Close()})
 if len(defaultManager.Instances())!=1 || len(adminManager.Instances())!=1{t.Fatal("App opened another App's index")}
 receipt,err:=search.Upsert(ctx,defaultManager.Catalog(),"product",map[string]string{"description":"red boot"});if err!=nil{t.Fatal(err)}
 if _,err:=defaultManager.Catalog().Wait(ctx,receipt);err!=nil{t.Fatal(err)}
 archive,err:=adminManager.Archive().Search(ctx,search.Query{Text:"red"});if err!=nil || archive.Total.Value!=0{t.Fatalf("App data crossed index owners: %#v %v",archive,err)}
 catalog,err:=defaultManager.Catalog().Search(ctx,search.Query{Text:"red"});if err!=nil || catalog.Total.Value!=1{t.Fatalf("default index missing data: %#v %v",catalog,err)}
 t.Setenv("ADMIN_SEARCH_TYPO_PATH",filepath.Join(root,"typo"))
 if _,err:=NewManagerForApp(ctx,"ADMIN",admin);err==nil || !strings.Contains(err.Error(),"typo"){t.Fatalf("named App typo was ignored: %v",err)}
 t.Setenv("SEARCH_TYPO_PATH",filepath.Join(root,"default-typo"))
 if _,err:=NewManager(ctx,defaults);err==nil || !strings.Contains(err.Error(),"typo"){t.Fatalf("default App typo was ignored: %v",err)}
}

// TestSearchManagerDriverFallback verifies generator-compatible blank selections through real runtime construction.
func TestSearchManagerDriverFallback(t *testing.T) {
 for _,test:=range []struct{name,prefix,root,named,appRoot,appNamed string}{
  {name:"root whitespace",root:"   "},
  {name:"named whitespace",root:"bleve",named:"   "},
  {name:"App root whitespace",prefix:"ADMIN",root:"bleve",appRoot:"   "},
  {name:"App named whitespace",prefix:"ADMIN",root:"bleve",appRoot:"bleve",appNamed:"   "},
 }{t.Run(test.name,func(t *testing.T){
  t.Setenv("SEARCH_DRIVER",test.root);t.Setenv("SEARCH_CATALOG_DRIVER",test.named)
  t.Setenv("ADMIN_SEARCH_DRIVER",test.appRoot);t.Setenv("ADMIN_SEARCH_CATALOG_DRIVER",test.appNamed)
  t.Setenv("SEARCH_CATALOG_PATH",filepath.Join(t.TempDir(),"catalog"))
  defs:=map[string]search.Definition{"catalog":definitions()["catalog"]}
  if err:=ProvisionForApp(context.Background(),test.prefix,defs);err!=nil{t.Fatal(err)}
  manager,err:=NewManagerForApp(context.Background(),test.prefix,defs);if err!=nil{t.Fatal(err)}
  if manager.Instances()[0].Driver!="bleve"{t.Fatal("blank selection lost local fallback")}
  if err:=manager.Close();err!=nil{t.Fatal(err)}
 })}
 t.Setenv("SEARCH_DRIVER","meilisearch")
 t.Setenv("SEARCH_CATALOG_DRIVER","   ")
 if driver:=searchDriverName("","catalog");driver!="meilisearch"{t.Fatalf("named blank lost root driver: %s",driver)}
 t.Setenv("ADMIN_SEARCH_DRIVER","meilisearch")
 t.Setenv("ADMIN_SEARCH_CATALOG_DRIVER","   ")
 if driver:=searchDriverName("ADMIN","catalog");driver!="meilisearch"{t.Fatalf("App named blank lost App root driver: %s",driver)}
 t.Setenv("ADMIN_SEARCH_DRIVER","   ")
 if driver:=searchDriverName("ADMIN","catalog");driver!="bleve"{t.Fatalf("App root blank lost baseline: %s",driver)}
}

// unsetSearchDriver preserves cleanup while testing absence rather than a blank explicit selection.
func unsetSearchDriver(t *testing.T, key string) {
 t.Helper()
 t.Setenv(key, "")
 if err := os.Unsetenv(key); err != nil { t.Fatal(err) }
}

// TestSearchManagerAppRootSelection dispatches provisioning and startup through an inherited remote driver.
func TestSearchManagerAppRootSelection(t *testing.T) {
 t.Setenv("SEARCH_DRIVER", "bleve")
 t.Setenv("ADMIN_SEARCH_DRIVER", "meilisearch")
 unsetSearchDriver(t, "SEARCH_CATALOG_DRIVER")
 unsetSearchDriver(t, "ADMIN_SEARCH_CATALOG_DRIVER")
 if got := searchDriverName("ADMIN", "catalog"); got != "meilisearch" { t.Fatalf("App root driver = %s", got) }
 t.Setenv("SEARCH_CATALOG_DRIVER", "bleve")
 if got := searchDriverName("ADMIN", "catalog"); got != "bleve" { t.Fatalf("global named driver = %s", got) }
 t.Setenv("SEARCH_CATALOG_DRIVER", "meilisearch")
 t.Setenv("ADMIN_SEARCH_CATALOG_DRIVER", "bleve")
 if got := searchDriverName("ADMIN", "catalog"); got != "bleve" { t.Fatalf("App named driver = %s", got) }
 unsetSearchDriver(t, "SEARCH_CATALOG_DRIVER")
 unsetSearchDriver(t, "ADMIN_SEARCH_CATALOG_DRIVER")
 var requests atomic.Int64
 server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
  requests.Add(1)
  if request.Method != http.MethodGet || !strings.HasPrefix(request.URL.Path, "/indexes/") { t.Errorf("unexpected Meilisearch request: %s %s", request.Method, request.URL.Path) }
  response.WriteHeader(http.StatusServiceUnavailable)
 }))
 defer server.Close()
 t.Setenv("ADMIN_SEARCH_URL", server.URL)
 t.Setenv("SEARCH_CATALOG_PATH", filepath.Join(t.TempDir(), "catalog"))
 defs := map[string]search.Definition{"catalog": definitions()["catalog"]}
 if err := ProvisionForApp(context.Background(), "ADMIN", defs); err == nil || requests.Load() == 0 { t.Fatalf("provision did not reach App root backend: %v (%d requests)", err, requests.Load()) }
 before := requests.Load()
 manager, err := NewManagerForApp(context.Background(), "ADMIN", defs)
 if err == nil { _ = manager.Close(); t.Fatal("remote validation failure was hidden") }
 if requests.Load() <= before { t.Fatal("runtime validation did not reach App root backend") }
}
// TestSearchManagerResourceWordApps keeps root App settings and exact named overrides disjoint.
func TestSearchManagerResourceWordApps(t *testing.T) {
 ctx := context.Background()
 t.Setenv("SEARCH_DRIVER", "bleve")
 t.Setenv("SEARCH_PATH", filepath.Join(t.TempDir(), "default"))
 t.Setenv("SEARCH_SEARCH_DRIVER", "meilisearch")
 t.Setenv("SEARCH_SEARCH_PATH", filepath.Join(t.TempDir(), "search-app"))
 t.Setenv("SEARCH_WORKER_SEARCH_DRIVER", "bleve")
 t.Setenv("SEARCH_WORKER_SEARCH_PATH", filepath.Join(t.TempDir(), "worker-app"))
 t.Setenv("SEARCH_SEARCH_SEARCH_DRIVER", "bleve")
 t.Setenv("SEARCH_SEARCH_SEARCH_PATH", filepath.Join(t.TempDir(), "nested-app"))
 t.Setenv("SEARCH_UNRELATED_SETTING", "ignored")
 t.Setenv("SEARCH_CACHE_DRIVER", "redis")
 t.Setenv("SEARCH_NOTIFICATION_DRIVER", "log")
 t.Setenv("SEARCH_FEATURE_DRIVER", "file")
 t.Setenv("SEARCH_WORKER_CACHE_DRIVER", "redis")
 t.Setenv("SEARCH_SEARCH_NOTIFICATION_DRIVER", "log")
 for _, prefix := range []string{"", "SEARCH", "SEARCH_WORKER", "SEARCH_SEARCH"} {
  if err := ProvisionForApp(ctx, prefix, nil); err != nil { t.Fatalf("%s empty provision: %v", prefix, err) }
  manager, err := NewManagerForApp(ctx, prefix, nil)
  if err != nil { t.Fatalf("%s empty startup: %v", prefix, err) }
  if len(manager.Instances()) != 0 { t.Fatal("environment invented schemas") }
  if err := manager.Close(); err != nil { t.Fatal(err) }
 }
 // A real default index named search keeps its global root fallback when an App owns the ambiguous key.
 if got := namedSearchValue("", "search", "PATH", "default-index"); got != "default-index" { t.Fatalf("App root became default index path: %s", got) }
 if got := searchDriverName("", "search"); got != "bleve" { t.Fatalf("App root became default index driver: %s", got) }
 if got := searchDriverName("", "cache"); got != "bleve" { t.Fatalf("another App family became a Search driver: %s", got) }
 if got := searchDriverName("", "notification"); got != "bleve" { t.Fatalf("App Notification settings became a Search driver: %s", got) }
 if got := searchDriverName("ADMIN", "search"); got != "bleve" { t.Fatalf("App root became another App index driver: %s", got) }
 defs := map[string]search.Definition{"search": {Name: "search", Fields: []search.Field{{Name: "description", Type: search.TextField, Searchable: true}}}}
 if err := Provision(ctx, defs); err != nil { t.Fatal(err) }
 manager, err := NewManager(ctx, defs)
 if err != nil { t.Fatal(err) }
 if err := manager.Close(); err != nil { t.Fatal(err) }
 for _, prefix := range []string{"", "SEARCH", "SEARCH_WORKER", "SEARCH_SEARCH"} {
  key := "SEARCH_TYPO_PATH"
  if prefix != "" { key = prefix+"_"+key }
  t.Setenv(key, "unused")
  if _, err := NewManagerForApp(ctx, prefix, nil); err == nil || !strings.Contains(err.Error(), "typo") { t.Fatalf("%s undeclared index accepted: %v", prefix, err) }
  if err := ProvisionForApp(ctx, prefix, nil); err == nil || !strings.Contains(err.Error(), "typo") { t.Fatalf("%s provision accepted undeclared index: %v", prefix, err) }
  unsetSearchDriver(t, key)
 }
}

`

//go:build integration

package forj

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestSearchAppCreationRemovalRuntime keeps compiled Search ownership current when the project component union is unchanged.
func TestSearchAppCreationRemovalRuntime(t *testing.T) {
	root := t.TempDir()
	repository, err := testkit.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{ProjectName: "Search lifecycle", GoModuleName: "example.com/searchlifecycle", Render: project.RenderConfig{Components: project.Components{CLI: true, Search: true}}}, ModuleReplaces: map[string]string{"github.com/goforj/goforj": repository}})
	forj := testkit.EnsureIntegrationForjBinary(t)
	run := func(arguments ...string) {
		t.Helper()
		command := exec.CommandContext(t.Context(), arguments[0], arguments[1:]...)
		command.Dir = root
		command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", arguments, err, output)
		}
	}
	run(forj, "make:app", "search", "--components", "cli,search")
	checkDir := filepath.Join(root, ".checks/search")
	if err := os.MkdirAll(checkDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkDir, "main.go"), []byte(searchAppLifecycleProgram), 0600); err != nil {
		t.Fatal(err)
	}
	run("go", "run", "./.checks/search")
	if err := testkit.ReplaceOrAppendEnvValue(filepath.Join(root, ".env"), "SEARCH_SEARCH_ONLY_PATH", "storage/search/search/only"); err != nil {
		t.Fatal(err)
	}
	run(forj, "generate", "--search")
	accessors, err := os.ReadFile(filepath.Join(root, "internal/searches/accessors_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(accessors), "Only()") {
		t.Fatal("App index accessor not generated")
	}
	run(forj, "make:app", "search", "--remove")
	manager, err := os.ReadFile(filepath.Join(root, "internal/searches/manager_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manager), `[]string{"SEARCH"}`) {
		t.Fatal("removed App retained compiled ownership")
	}
	accessors, err = os.ReadFile(filepath.Join(root, "internal/searches/accessors_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(accessors), "Only()") || strings.Contains(string(accessors), "Search()") {
		t.Fatalf("removed App retained accessors: %s", accessors)
	}
	environment, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(environment), "SEARCH_DRIVER=") || !strings.Contains(string(environment), "SEARCH_PATH=") {
		t.Fatal("removing App erased default Search configuration")
	}
	run("go", "run", "./.checks/search")
}

const searchAppLifecycleProgram = `// Package main verifies Search ownership immediately after App lifecycle mutations.
package main
import (
 "context"
 "log"
 "example.com/searchlifecycle/internal/cmd"
 "example.com/searchlifecycle/internal/searches"
)
// main verifies that root and named managers remain inert without schema declarations.
func main() {
 if err := cmd.LoadEnv();err != nil { log.Fatal(err) }
 for _,prefix := range []string{"","SEARCH"} {
  if err := searches.ProvisionForApp(context.Background(),prefix,nil);err != nil { log.Fatal(err) }
  manager,err := searches.NewManagerForApp(context.Background(),prefix,nil);if err != nil { log.Fatal(err) }
  if len(manager.Instances()) != 0 { log.Fatal("environment created an undeclared schema") }
  if err := manager.Close();err != nil { log.Fatal(err) }
 }
}
`

// TestSearchLoadEnvManagerFallback exercises file-backed startup before real App-aware backend construction.
func TestSearchLoadEnvManagerFallback(t *testing.T) {
	root := t.TempDir()
	repository, err := testkit.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	config := project.Config{ProjectName: "Search startup", GoModuleName: "example.com/searchstartup", Render: project.RenderConfig{Components: project.Components{CLI: true, Search: true}}, Apps: map[string]project.AppConfig{"billing": {Components: project.Components{CLI: true, Search: true}}}}
	if err := writeConventionalAppMarker(root, "billing"); err != nil {
		t.Fatal(err)
	}
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: config, ModuleReplaces: map[string]string{"github.com/goforj/goforj": repository}})
	if err := testkit.ReplaceOrAppendEnvValue(filepath.Join(root, ".env"), "SEARCH_SUPPORTED_DRIVERS", "bleve,meilisearch"); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), testkit.EnsureIntegrationForjBinary(t), "generate", "--search")
	command.Dir = root
	command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate remote capability: %v\n%s", err, output)
	}
	if err := os.WriteFile(filepath.Join(root, "internal/cmd/search_startup_test.go"), []byte(searchLoadEnvManagerTest), 0600); err != nil {
		t.Fatal(err)
	}
	command = exec.CommandContext(t.Context(), "go", "test", "./internal/cmd", "-run", "^(TestSearchLoadEnvManager|TestLoadEnvPreservesAppResourceScopes)$", "-count=1")
	command.Dir = root
	command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("LoadEnv App Search fallback: %v\n%s", err, output)
	}
}

const searchLoadEnvManagerTest = `package cmd
import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"
 "github.com/goforj/search"
 "example.com/searchstartup/internal/searches"
)
// TestSearchLoadEnvManager isolates startup so dotenv loading uses the case's own source configuration.
func TestSearchLoadEnvManager(t *testing.T) {
 if scenario := os.Getenv("SEARCH_STARTUP_SCENARIO");scenario != "" {
  runSearchStartupScenario(t,scenario)
  return
 }
 for _,scenario := range []string{"named-path-empty","named-path-value","root-driver-empty","root-driver-whitespace","named-driver-empty","named-driver-whitespace"} {
  t.Run(scenario,func(t *testing.T){
   command:=exec.Command(os.Args[0],"-test.run=^TestSearchLoadEnvManager$")
   command.Dir=t.TempDir()
   for _,entry:=range os.Environ(){
    key,_,_:=strings.Cut(entry,"=")
    if key=="APP_ENV" || key=="FORJ_APP" || strings.HasPrefix(key,"SEARCH_") || strings.HasPrefix(key,"BILLING_"){continue}
    command.Env=append(command.Env,entry)
   }
   command.Env=append(command.Env,"SEARCH_STARTUP_SCENARIO="+scenario)
   if output,err:=command.CombinedOutput();err!=nil{t.Fatalf("startup scenario: %v\n%s",err,output)}
  })
 }
}
// runSearchStartupScenario verifies resolved settings through actual local and HTTP backend operations.
func runSearchStartupScenario(t *testing.T,scenario string) {
 ctx:=context.Background()
 definitions:=map[string]search.Definition{"catalog":{Name:"catalog",Fields:[]search.Field{{Name:"description",Type:search.TextField,Searchable:true}}}}
 root:=t.TempDir()
 basePath:=filepath.Join(root,"base-catalog")
 appPath:=filepath.Join(root,"app-catalog")
 appValue:=""
 if scenario=="named-path-value"{appValue=appPath}
 if strings.HasPrefix(scenario,"root-driver") {
  appDriver:="";if scenario=="root-driver-whitespace"{appDriver="   "}
  contents:=fmt.Sprintf("FORJ_APP=billing\nSEARCH_DRIVER=meilisearch\nBILLING_SEARCH_DRIVER='%s'\nBILLING_SEARCH_PATH=%s\n",appDriver,root)
  if err:=os.WriteFile(".env",[]byte(contents),0600);err!=nil{t.Fatal(err)}
  if err:=LoadEnv();err!=nil{t.Fatal(err)}
  if os.Getenv("SEARCH_DRIVER")!="meilisearch"{t.Fatal("LoadEnv destroyed base driver")}
  if err:=searches.ProvisionForApp(ctx,"BILLING",definitions);err!=nil{t.Fatal(err)}
  manager,err:=searches.NewManagerForApp(ctx,"BILLING",definitions);if err!=nil{t.Fatal(err)}
  if manager.Instances()[0].Driver!="bleve"{t.Fatal("empty App root did not select baseline driver")}
  if err:=manager.Close();err!=nil{t.Fatal(err)}
  return
 }
 if strings.HasPrefix(scenario,"named-path") {
  contents:=fmt.Sprintf("FORJ_APP=billing\nSEARCH_DRIVER=bleve\nSEARCH_CATALOG_PATH=%s\nBILLING_SEARCH_PATH=%s\nBILLING_SEARCH_CATALOG_PATH=%s\nSEARCH_BUSINESS_KEY=base-custom\nBILLING_SEARCH_BUSINESS_KEY=app-custom\nSMTP_PASSWORD=base-secret\nBILLING_SMTP_PASSWORD=\n",basePath,filepath.Join(root,"app-root"),appValue)
  if err:=os.WriteFile(".env",[]byte(contents),0600);err!=nil{t.Fatal(err)}
  if err:=LoadEnv();err!=nil{t.Fatal(err)}
  if os.Getenv("SEARCH_CATALOG_PATH")!=basePath{t.Fatal("LoadEnv destroyed base named path")}
  if os.Getenv("SEARCH_BUSINESS_KEY")!="app-custom" || os.Getenv("SMTP_PASSWORD")!=""{t.Fatal("legacy custom or empty-secret projection changed")}
  if err:=searches.ProvisionForApp(ctx,"BILLING",definitions);err!=nil{t.Fatal(err)}
  manager,err:=searches.NewManagerForApp(ctx,"BILLING",definitions);if err!=nil{t.Fatal(err)}
  if len(manager.Instances())!=1 || manager.Instances()[0].Driver!="bleve"{t.Fatal("wrong resolved backend")}
  if err:=manager.Close();err!=nil{t.Fatal(err)}
  wantPath:=basePath;if appValue!=""{wantPath=appPath}
  if _,err:=os.Stat(wantPath);err!=nil{t.Fatalf("resolved named path missing: %v",err)}
  return
 }
 var requests atomic.Int64
 server:=httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter,request *http.Request){
  requests.Add(1)
  if request.Header.Get("Authorization")!="Bearer base-key"{t.Errorf("base API key lost: %q",request.Header.Get("Authorization"))}
  if !strings.HasPrefix(request.URL.Path,"/indexes/base_"){t.Errorf("base prefix lost: %s",request.URL.Path)}
  response.WriteHeader(http.StatusServiceUnavailable)
 }))
 defer server.Close()
 if scenario=="named-driver-whitespace"{appValue="   "}
 contents:=fmt.Sprintf("FORJ_APP=billing\nSEARCH_DRIVER=bleve\nSEARCH_CATALOG_DRIVER=bleve\nSEARCH_URL=%s\nSEARCH_API_KEY=base-key\nSEARCH_PREFIX=base_\nBILLING_SEARCH_DRIVER=meilisearch\nBILLING_SEARCH_CATALOG_DRIVER='%s'\nBILLING_SEARCH_URL=\nBILLING_SEARCH_API_KEY=\nBILLING_SEARCH_PREFIX=\n",server.URL,appValue)
 if err:=os.WriteFile(".env",[]byte(contents),0600);err!=nil{t.Fatal(err)}
 if err:=LoadEnv();err!=nil{t.Fatal(err)}
 if os.Getenv("SEARCH_CATALOG_DRIVER")!="bleve" || os.Getenv("SEARCH_URL")!=server.URL{t.Fatal("LoadEnv destroyed base Search settings")}
 if manager,err:=searches.NewManagerForApp(ctx,"BILLING",definitions);err==nil{_ = manager.Close();t.Fatal("remote schema failure ignored")}
 if requests.Load()==0{t.Fatal("named blank driver did not fall back to App root remote backend")}
 before:=requests.Load()
 if err:=searches.ProvisionForApp(ctx,"BILLING",definitions);err==nil || requests.Load()==before{t.Fatalf("provision bypassed resolved App backend: %v",err)}
}
`

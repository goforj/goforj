//go:build integration

package forj

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestRenderedNotificationLoadEnvScopes exercises file loading before constructing App-aware senders.
func TestRenderedNotificationLoadEnvScopes(t *testing.T) {
	root := t.TempDir()
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{ProjectName: "Notification Startup", GoModuleName: "example.com/notificationstartup", Render: project.RenderConfig{Components: project.Components{CLI: true, Notification: true}}, Apps: map[string]project.AppConfig{"billing": {Components: project.Components{CLI: true, Notification: true}}}}})
	setRenderedEnvValue(t, root, "NOTIFICATION_ALERTS_DRIVER", "log")
	setRenderedEnvValue(t, root, "NOTIFICATION_SUPPORTED_DRIVERS", "log,webhook")
	if _, err := generate.GenerateProjectFiles(root, generate.GenerationSelection{Notification: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal/cmd/notification_startup_test.go"), []byte(notificationStartupRuntimeTests), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "test", "./internal/cmd", "-run", "^(TestNotificationStartup|TestLoadEnvPreservesAppResourceScopes)$", "-count=1")
	command.Dir = root
	command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("notification startup: %v\n%s", err, output)
	}
}

const notificationStartupRuntimeTests = `package cmd
import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "example.com/notificationstartup/internal/notifications"
 "github.com/goforj/notification"
)
// TestNotificationStartup checks actual startup and transport routing in isolated processes.
func TestNotificationStartup(t *testing.T) {
 if os.Getenv("NOTIFY_STARTUP_HELPER") == "1" { runNotificationStartup(t);return }
 for _,overlay := range []string{"","webhook"} {
  t.Run(fmt.Sprintf("overlay=%q",overlay),func(t *testing.T) {
   command := exec.Command(os.Args[0],"-test.run=^TestNotificationStartup$")
   command.Dir = t.TempDir()
   for _,entry := range os.Environ() {
    key,_,_ := strings.Cut(entry,"=")
    if strings.HasPrefix(key,"NOTIFICATION_") || strings.HasPrefix(key,"BILLING_") || key=="FORJ_APP" || key=="APP_ENV" || key=="SMTP_PASSWORD" || key=="DB_PASSWORD" { continue }
    command.Env = append(command.Env,entry)
   }
   command.Env = append(command.Env,"NOTIFY_STARTUP_HELPER=1","NOTIFY_OVERLAY="+overlay)
   if output,err := command.CombinedOutput();err != nil { t.Fatalf("startup: %v\n%s",err,output) }
  })
 }
}
// runNotificationStartup loads a fresh file before verifying inherited and explicit App endpoints.
func runNotificationStartup(t *testing.T) {
 paths := make(chan string,4)
 server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){ paths<-r.URL.Path;w.WriteHeader(http.StatusNoContent) }))
 defer server.Close()
 overlay := os.Getenv("NOTIFY_OVERLAY")
 appURL,appNamedURL := "",""
 if overlay != "" { appURL=server.URL+"/app";appNamedURL=server.URL+"/app-named" }
 content := "APP_ENV=local\nFORJ_APP=billing\nNOTIFICATION_DRIVER=webhook\nNOTIFICATION_SUPPORTED_DRIVERS=log,webhook\nNOTIFICATION_WEBHOOK_URL="+server.URL+"/base\nNOTIFICATION_WEBHOOK_ALLOW_PRIVATE=true\nNOTIFICATION_ALERTS_DRIVER=webhook\nNOTIFICATION_ALERTS_WEBHOOK_URL="+server.URL+"/named\nNOTIFICATION_ALERTS_WEBHOOK_ALLOW_PRIVATE=true\nBILLING_NOTIFICATION_DRIVER="+overlay+"\nBILLING_NOTIFICATION_WEBHOOK_URL="+appURL+"\nBILLING_NOTIFICATION_ALERTS_DRIVER="+overlay+"\nBILLING_NOTIFICATION_ALERTS_WEBHOOK_URL="+appNamedURL+"\nNOTIFICATION_BUSINESS_KEY=base-custom\nBILLING_NOTIFICATION_BUSINESS_KEY=app-custom\nSMTP_PASSWORD=base-secret\nDB_PASSWORD=base-secret\nBILLING_SMTP_PASSWORD=\nBILLING_DB_PASSWORD=\n"
 if err := os.WriteFile(filepath.Join(".",".env"),[]byte(content),0600);err != nil { t.Fatal(err) }
 if err := LoadEnv();err != nil { t.Fatal(err) }
 if os.Getenv("NOTIFICATION_WEBHOOK_URL") != server.URL+"/base" || os.Getenv("NOTIFICATION_ALERTS_WEBHOOK_URL") != server.URL+"/named" { t.Fatal("LoadEnv overwrote base transport configuration") }
 if os.Getenv("SMTP_PASSWORD")!="" || os.Getenv("DB_PASSWORD")!="" || os.Getenv("NOTIFICATION_BUSINESS_KEY")!="app-custom" { t.Fatal("legacy overlay semantics changed") }
 for _,prefix := range []string{"","BILLING"} {
  manager,err := notifications.NewManagerForApp(prefix)
  if err != nil { t.Fatal(err) }
  for _,test := range []struct{name,want string}{{"default","/base"},{"alerts","/named"}} {
   want:=test.want
   if prefix=="BILLING" && overlay!="" { if test.name=="default" { want="/app" } else { want="/app-named" } }
   sender:=manager.Default()
   if test.name!="default" { sender=manager.Named(test.name) }
   if sender==nil { t.Fatal("named sender missing") }
   if err:=sender.Send(context.Background(),notification.Message{Title:"Fixture",Body:"Startup routing"});err!=nil { t.Fatal(err) }
   if got:=<-paths;got!=want { t.Fatalf("%s/%s route=%s want=%s",prefix,test.name,got,want) }
  }
  if err:=manager.Close();err!=nil { t.Fatal(err) }
 }
}
`

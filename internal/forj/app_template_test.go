package forj

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWireAppTemplateUsesSingularDefaultAndPluralManagers(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templatePath := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates", "wire", "app.go.tmpl")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read app.go template: %v", err)
	}
	source := string(content)

	for _, snippet := range []string{
		"func (a *App) Cache() *goforjcache.Cache",
		"return a.cache.Default()",
		"func (a *App) Caches() *cache.Manager",
		"func (a *App) Storage() *storage.Manager",
	} {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected wire app template to contain %q", snippet)
		}
	}
}

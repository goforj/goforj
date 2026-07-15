package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/goforj/env/v2"
	"github.com/goforj/str"
)

type dbTemplateData struct {
	CompiledDrivers []string
	HasNames        bool
	NeedsGormImport bool
	Names           []dbAccessorName
	Drivers         []dbDriverSpec
}

type dbAccessorName struct {
	Method string
	Name   string
}

type dbDriverSpec struct {
	Alias       string
	ImportPath  string
	SwitchCases []string
	Constructor string
	PrepareDSN  bool
}

var dbRootKeys = []string{
	"DEFAULT",
	"DRIVER",
	"DSN",
	"HOST",
	"DATABASE",
	"USERNAME",
	"PASSWORD",
	"PORT",
	"QUERY_LOGGING",
	"SLOW_QUERY_THRESHOLD",
	"MAX_IDLE_CONNECTIONS",
	"MAX_OPEN_CONNECTIONS",
	"CONN_MAX_LIFETIME_MINUTES",
	"ROOT_PASSWORD",
}

// GenerateDBFiles writes database accessors whose selectable drivers are fixed by the generation snapshot.
func GenerateDBFiles(projectDir string) (int, error) {
	if err := validateAppPrefixedDBEnv(projectDir); err != nil {
		return 0, err
	}
	names := discoverDBConnectionNames(projectDir)
	drivers, compiledDrivers, err := discoverDBDrivers(projectDir, names)
	if err != nil {
		return 0, err
	}
	source, err := renderDBAccessors(names, drivers, compiledDrivers)
	if err != nil {
		return 0, err
	}
	formatted, err := format.Source(source)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated db accessors: %w", err)
	}
	changed, err := writeGeneratedSource(filepath.Join(projectDir, "internal", "database", "connections_gen.go"), formatted)
	if err != nil {
		return 0, err
	}
	if changed {
		return 1, nil
	}
	return 0, nil
}

// discoverDBConnectionNames includes connections declared only through a configured App overlay.
func discoverDBConnectionNames(projectDir string) []string {
	names := discoverPrimitiveChildNames(projectDir, "DB", dbRootKeys)
	out := make([]string, 0, len(names))
	for _, name := range names {
		normalized := str.Of(name).TrimSpace().ToLower().String()
		if normalized == "" || normalized == "root" || dbHelperConnectionName(normalized) {
			continue
		}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

// validateAppPrefixedDBEnv rejects App-scoped keys that cannot become a root or named database setting at runtime.
func validateAppPrefixedDBEnv(projectDir string) error {
	problems := []string{}
	for _, appPrefix := range generationAppEnvPrefixesForResource(projectDir, "DB") {
		prefix := appPrefix + "_DB_"
		for _, assignment := range os.Environ() {
			key, _, ok := strings.Cut(assignment, "=")
			if !ok || !strings.HasPrefix(key, prefix) {
				continue
			}
			if _, _, valid := splitScopedEnvKey(strings.TrimPrefix(key, prefix), dbRootKeys); valid {
				continue
			}
			problems = append(problems, fmt.Sprintf("%s is not a supported database env var", key))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("invalid database env:\n- %s", strings.Join(problems, "\n- "))
}

// dbHelperConnectionName skips driver-specific helper keys such as DB_SQLITE_DATABASE.
func dbHelperConnectionName(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "mysql", "postgres", "postgresql", "sqlite", "sqlite3":
		return true
	default:
		return false
	}
}

// renderDBAccessors keeps retained compatibility implementations separate from the authoritative compiled manifest.
func renderDBAccessors(names []string, drivers []dbDriverSpec, compiledDrivers []string) ([]byte, error) {
	data := dbTemplateData{
		CompiledDrivers: compiledDrivers,
		HasNames:        false,
		NeedsGormImport: len(names) > 1 || len(drivers) > 0,
		Names:           make([]dbAccessorName, 0, len(names)),
		Drivers:         drivers,
	}
	for _, name := range names {
		if name == "default" {
			continue
		}
		data.HasNames = true
		data.Names = append(data.Names, dbAccessorName{
			Method: str.Of(name).Pascal().String(),
			Name:   name,
		})
	}
	var b bytes.Buffer
	tmpl, err := template.New("db-accessors").Parse(dbAccessorsSourceTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// discoverDBDrivers validates every active connection against the explicit build contract before source is emitted.
func discoverDBDrivers(projectDir string, names []string) ([]dbDriverSpec, []string, error) {
	drivers := map[string]dbDriverSpec{}
	compiled := map[string]struct{}{}
	recordDBDriver(drivers, "sqlite")
	rootDriver := str.Of(env.WithPrefix("DB").Get("DRIVER", "sqlite")).TrimSpace().ToLower().String()
	if rootDriver == "" {
		rootDriver = "sqlite"
	}
	activeDrivers := []generationActiveDriver{
		{
			key:    "DB_DRIVER",
			driver: rootDriver,
		},
	}
	for _, name := range names {
		scope := env.WithPrefix("DB").Child(str.Of(name).Snake("_").ToUpper().String())
		driver := str.Of(scope.Get("DRIVER", "")).TrimSpace().ToLower().String()
		if driver == "" {
			continue
		}
		activeDrivers = append(activeDrivers, generationActiveDriver{
			key:    "DB_" + str.Of(name).Snake("_").ToUpper().String() + "_DRIVER",
			driver: driver,
		})
	}
	activeDrivers = append(activeDrivers, appPrefixedActiveDrivers(projectDir, "DB", "sqlite", true)...)
	rawSupported := str.Of(env.WithPrefix("DB").Get("SUPPORTED_DRIVERS", "")).TrimSpace().ToLower().String()
	if rawSupported != "" {
		for _, part := range strings.Split(rawSupported, ",") {
			driver := str.Of(part).TrimSpace().ToLower().String()
			if driver == "" {
				continue
			}
			if !recordDBDriver(drivers, driver) {
				return nil, nil, fmt.Errorf("DB_SUPPORTED_DRIVERS includes unsupported driver %q", driver)
			}
			compiled[canonicalDBDriver(driver)] = struct{}{}
		}
		if len(compiled) == 0 {
			return nil, nil, fmt.Errorf("DB_SUPPORTED_DRIVERS must include at least one driver")
		}
		for _, active := range activeDrivers {
			if _, ok := compiled[canonicalDBDriver(active.driver)]; !ok {
				return nil, nil, fmt.Errorf("%s selects driver %q not enabled by DB_SUPPORTED_DRIVERS", active.key, active.driver)
			}
		}
	} else {
		for _, active := range activeDrivers {
			if !recordDBDriver(drivers, active.driver) {
				return nil, nil, fmt.Errorf("%s selects unsupported driver %q", active.key, active.driver)
			}
			compiled[canonicalDBDriver(active.driver)] = struct{}{}
		}
	}
	out := make([]dbDriverSpec, 0, len(drivers))
	for _, driver := range drivers {
		out = append(out, driver)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out, sortStrings(compiled), nil
}

// canonicalDBDriver normalizes accepted compatibility aliases to the generated manifest name.
func canonicalDBDriver(driver string) string {
	switch strings.TrimSpace(strings.ToLower(driver)) {
	case "mariadb":
		return "mysql"
	case "postgresql":
		return "postgres"
	case "sqlite3":
		return "sqlite"
	default:
		return strings.TrimSpace(strings.ToLower(driver))
	}
}

func recordDBDriver(drivers map[string]dbDriverSpec, driver string) bool {
	switch driver {
	case "mysql", "mariadb":
		drivers["mysql"] = dbDriverSpec{
			Alias:       "mysql",
			ImportPath:  "gorm.io/driver/mysql",
			SwitchCases: []string{"mysql", "mariadb"},
			Constructor: `mysql.New(mysql.Config{DSN: dsn, DisableWithReturning: true})`,
		}
		return true
	case "postgres", "postgresql":
		drivers["postgres"] = dbDriverSpec{
			Alias:       "postgres",
			ImportPath:  "gorm.io/driver/postgres",
			SwitchCases: []string{"postgres", "postgresql"},
			Constructor: "postgres.Open(dsn)",
		}
		return true
	case "sqlite", "sqlite3":
		drivers["sqlite"] = dbDriverSpec{
			Alias:       "sqlite",
			ImportPath:  "github.com/glebarez/sqlite",
			SwitchCases: []string{"sqlite", "sqlite3"},
			Constructor: "sqlite.Open(dsn)",
			PrepareDSN:  true,
		}
		return true
	}
	return false
}

const dbAccessorsSourceTemplate = `// Code generated by forj generate --db. DO NOT EDIT.
// Run: forj generate --db
//
// This file contains accessors generated from DB_<NAME>_<KEY> environment variables.
// For example, DB_ANALYTICS_DRIVER will generate GetAnalytics().
package database

{{- if .NeedsGormImport }}
import (
	"context"
	"fmt"
	"strings"
	{{- if .Drivers }}
	{{- range .Drivers }}
	"{{ .ImportPath }}"
	{{- end }}
	{{- end }}
	"gorm.io/gorm"
)
{{ end }}
var compiledDatabaseDrivers = []string{
{{- range .CompiledDrivers }}
	"{{ . }}",
{{- end }}
}

type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

func (c *Connections) ReadinessChecks() []ReadinessCheck {
	if c == nil {
		return nil
	}
	checks := []ReadinessCheck{
		{
			Name: "db_default",
			Check: func(ctx context.Context) error {
				return c.readinessCheck(ctx, "default")
			},
		},
{{- range .Names }}
		{
			Name: "db_{{ .Name }}",
			Check: func(ctx context.Context) error {
				return c.readinessCheck(ctx, "{{ .Name }}")
			},
		},
{{- end }}
	}
	return checks
}

func (c *Connections) readinessCheck(ctx context.Context, name string) error {
	conn, err := c.Connection(name)
	if err != nil {
		return err
	}
	sqlDB, err := conn.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

{{- if .Drivers }}

func openDialector(driver, dsn string) (gorm.Dialector, error) {
	driver = strings.TrimSpace(strings.ToLower(driver))
	if !databaseDriverCompiled(driver) {
		return nil, fmt.Errorf("database: active driver %q is not built in; compiled choices: %s; run forj generate --db after updating DB_SUPPORTED_DRIVERS", driver, strings.Join(compiledDatabaseDrivers, ", "))
	}
	switch driver {
	{{- range .Drivers }}
	case {{ range $idx, $case := .SwitchCases }}{{ if $idx }}, {{ end }}"{{ $case }}"{{ end }}:
		{{- if .PrepareDSN }}
		if err := ensureSQLitePath(dsn); err != nil {
			return nil, err
		}
		{{- end }}
		return {{ .Constructor }}, nil
	{{- end }}
	default:
		return nil, fmt.Errorf("unsupported driver %q", driver)
	}
}

// databaseDriverCompiled reports whether driver is selectable in this generated artifact.
func databaseDriverCompiled(driver string) bool {
	switch driver {
	case "mariadb":
		driver = "mysql"
	case "postgresql":
		driver = "postgres"
	case "sqlite3":
		driver = "sqlite"
	}
	for _, compiled := range compiledDatabaseDrivers {
		if driver == compiled {
			return true
		}
	}
	return false
}
{{ end }}
{{ range .Names }}
// Get{{ .Method }} returns the "{{ .Name }}" connection.
func (c *Connections) Get{{ .Method }}() (*gorm.DB, error) {
	return c.get("{{ .Name }}")
}

{{ end }}`

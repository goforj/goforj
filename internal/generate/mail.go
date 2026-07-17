package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"
	"slices"
	"sort"
	"text/template"

	"github.com/goforj/str/v2"
)

// mailAccessorTemplateData carries the normalized mailer names shared by generated accessor methods.
type mailAccessorTemplateData struct {
	Names []mailAccessorName
}

// mailConfigTemplateData keeps the compiled provider manifest aligned with generated manager wiring.
type mailConfigTemplateData struct {
	CompiledDrivers []string
	Drivers         []mailDriverSpec
	Names           []mailAccessorName
}

// mailAccessorName binds an environment mailer name to its Go-safe field and method identifiers.
type mailAccessorName struct {
	Method string
	Mailer string
	Field  string
}

// mailDriverSpec centralizes each provider's constructor contract so imports and configuration fields cannot drift.
type mailDriverSpec struct {
	ConstName    string
	ImportPath   string
	Constructor  string
	ConfigType   string
	Fields       []mailConfigField
	ReadsEnvOnly bool
}

// mailConfigField preserves typed provider configuration expressions while the generated struct literal is assembled.
type mailConfigField struct {
	Name  string
	Value string
}

var mailDriverSpecs = map[string]mailDriverSpec{
	"log": {
		ConstName:    "driverLog",
		ImportPath:   "github.com/goforj/mail/maillog",
		Constructor:  "maillog.New",
		ReadsEnvOnly: true,
	},
	"smtp": {
		ConstName:   "driverSMTP",
		ImportPath:  "github.com/goforj/mail/mailsmtp",
		Constructor: "mailsmtp.New",
		ConfigType:  "mailsmtp.Config",
		Fields: []mailConfigField{
			{Name: "Host", Value: `strings.TrimSpace(scope.Get("SMTP_HOST", ""))`},
			{Name: "Port", Value: `scope.GetInt("SMTP_PORT", "587")`},
			{Name: "Username", Value: `strings.TrimSpace(scope.Get("SMTP_USERNAME", ""))`},
			{Name: "Password", Value: `scope.Get("SMTP_PASSWORD", "")`},
			{Name: "Identity", Value: `strings.TrimSpace(scope.Get("SMTP_IDENTITY", ""))`},
			{Name: "ForceTLS", Value: `scope.GetBool("SMTP_FORCE_TLS", "false")`},
		},
	},
	"resend": {
		ConstName:   "driverResend",
		ImportPath:  "github.com/goforj/mail/mailresend",
		Constructor: "mailresend.New",
		ConfigType:  "mailresend.Config",
		Fields: []mailConfigField{
			{Name: "APIKey", Value: `strings.TrimSpace(scope.Get("RESEND_API_KEY", ""))`},
			{Name: "Endpoint", Value: `strings.TrimSpace(scope.Get("RESEND_ENDPOINT", ""))`},
		},
	},
	"postmark": {
		ConstName:   "driverPostmark",
		ImportPath:  "github.com/goforj/mail/mailpostmark",
		Constructor: "mailpostmark.New",
		ConfigType:  "mailpostmark.Config",
		Fields: []mailConfigField{
			{Name: "ServerToken", Value: `strings.TrimSpace(scope.Get("POSTMARK_SERVER_TOKEN", ""))`},
			{Name: "Endpoint", Value: `strings.TrimSpace(scope.Get("POSTMARK_ENDPOINT", ""))`},
			{Name: "MessageStream", Value: `strings.TrimSpace(scope.Get("POSTMARK_MESSAGE_STREAM", ""))`},
		},
	},
	"mailgun": {
		ConstName:   "driverMailgun",
		ImportPath:  "github.com/goforj/mail/mailmailgun",
		Constructor: "mailmailgun.New",
		ConfigType:  "mailmailgun.Config",
		Fields: []mailConfigField{
			{Name: "Domain", Value: `strings.TrimSpace(scope.Get("MAILGUN_DOMAIN", ""))`},
			{Name: "APIKey", Value: `strings.TrimSpace(scope.Get("MAILGUN_API_KEY", ""))`},
			{Name: "Endpoint", Value: `strings.TrimSpace(scope.Get("MAILGUN_ENDPOINT", ""))`},
		},
	},
	"sendgrid": {
		ConstName:   "driverSendGrid",
		ImportPath:  "github.com/goforj/mail/mailsendgrid",
		Constructor: "mailsendgrid.New",
		ConfigType:  "mailsendgrid.Config",
		Fields: []mailConfigField{
			{Name: "APIKey", Value: `strings.TrimSpace(scope.Get("SENDGRID_API_KEY", ""))`},
			{Name: "Endpoint", Value: `strings.TrimSpace(scope.Get("SENDGRID_ENDPOINT", ""))`},
		},
	},
	"ses": {
		ConstName:   "driverSES",
		ImportPath:  "github.com/goforj/mail/mailses",
		Constructor: "mailses.New",
		ConfigType:  "mailses.Config",
		Fields: []mailConfigField{
			{Name: "Region", Value: `strings.TrimSpace(scope.Get("SES_REGION", ""))`},
			{Name: "AccessKeyID", Value: `strings.TrimSpace(scope.Get("SES_ACCESS_KEY_ID", ""))`},
			{Name: "SecretAccessKey", Value: `scope.Get("SES_SECRET_ACCESS_KEY", "")`},
			{Name: "SessionToken", Value: `scope.Get("SES_SESSION_TOKEN", "")`},
			{Name: "Endpoint", Value: `strings.TrimSpace(scope.Get("SES_ENDPOINT", ""))`},
			{Name: "ConfigurationSetName", Value: `strings.TrimSpace(scope.Get("SES_CONFIGURATION_SET", ""))`},
		},
	},
}

var mailRootKeys = []string{
	"DRIVER",
	"SUPPORTED_DRIVERS",
	"FROM_ADDRESS",
	"FROM_NAME",
	"LOG_BODIES",
	"SMTP_HOST",
	"SMTP_PORT",
	"SMTP_USERNAME",
	"SMTP_PASSWORD",
	"SMTP_IDENTITY",
	"SMTP_FORCE_TLS",
	"RESEND_API_KEY",
	"RESEND_ENDPOINT",
	"POSTMARK_SERVER_TOKEN",
	"POSTMARK_ENDPOINT",
	"POSTMARK_MESSAGE_STREAM",
	"MAILGUN_DOMAIN",
	"MAILGUN_API_KEY",
	"MAILGUN_ENDPOINT",
	"SENDGRID_API_KEY",
	"SENDGRID_ENDPOINT",
	"SES_REGION",
	"SES_ACCESS_KEY_ID",
	"SES_SECRET_ACCESS_KEY",
	"SES_SESSION_TOKEN",
	"SES_ENDPOINT",
	"SES_CONFIGURATION_SET",
}

var mailCommonKeys = makeSet(
	"DRIVER",
	"FROM_ADDRESS",
	"FROM_NAME",
)

var mailDriverKeys = map[string]map[string]struct{}{
	"log":      makeSet("LOG_BODIES"),
	"smtp":     makeSet("SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_IDENTITY", "SMTP_FORCE_TLS"),
	"resend":   makeSet("RESEND_API_KEY", "RESEND_ENDPOINT"),
	"postmark": makeSet("POSTMARK_SERVER_TOKEN", "POSTMARK_ENDPOINT", "POSTMARK_MESSAGE_STREAM"),
	"mailgun":  makeSet("MAILGUN_DOMAIN", "MAILGUN_API_KEY", "MAILGUN_ENDPOINT"),
	"sendgrid": makeSet("SENDGRID_API_KEY", "SENDGRID_ENDPOINT"),
	"ses":      makeSet("SES_REGION", "SES_ACCESS_KEY_ID", "SES_SECRET_ACCESS_KEY", "SES_SESSION_TOKEN", "SES_ENDPOINT", "SES_CONFIGURATION_SET"),
}

// GenerateMailFiles writes mail accessors whose selectable providers are fixed by the generation snapshot.
func GenerateMailFiles(projectDir string) (int, error) {
	return generateMailFiles(ambientGenerationInput(projectDir))
}

// generateMailFiles uses one captured environment for validation, rendering, and named-resource discovery.
func generateMailFiles(input generationInput) (int, error) {
	if err := validatePrimitiveEnv(input, primitiveEnvContract{
		Prefix:        "MAIL",
		DefaultDriver: "log",
		RootKeys:      mailRootKeys,
		CommonKeys:    mailCommonKeys,
		DriverKeys:    mailDriverKeys,
		ChildNames: func(environment generationEnvironment) []string {
			return exactScopedChildNames(environment, "MAIL", mailRootKeys)
		},
		AllowInactiveRootKeys: true,
		EagerNamedResources:   true,
	}); err != nil {
		return 0, err
	}

	manager, err := renderMailConfig(input)
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated mail manager: %w", err)
	}
	accessors, err := renderMailAccessors(input)
	if err != nil {
		return 0, err
	}
	formattedAccessors, err := format.Source(accessors)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated mail accessors: %w", err)
	}

	written := 0
	changed, err := writeGeneratedSource(filepath.Join(input.projectDir, "internal", "mail", "manager_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	changed, err = writeGeneratedSource(filepath.Join(input.projectDir, "internal", "mail", "accessors_gen.go"), formattedAccessors)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	for _, name := range mailLegacyGeneratedFiles {
		changed, err = removeGeneratedFileIfExists(filepath.Join(input.projectDir, "internal", "mail", name))
		if err != nil {
			return written, err
		}
		if changed {
			written++
		}
	}
	return written, nil
}

// renderMailConfig retains the native log implementation without silently adding it to the compiled manifest.
func renderMailConfig(input generationInput) ([]byte, error) {
	names := discoverMailNames(input)
	driverSet := map[string]struct{}{}
	defaultDriver := effectivePrimitiveDriver(input.environment.Get("MAIL_DRIVER", "log"), "log")
	driverSet[defaultDriver] = struct{}{}
	for _, child := range discoverMailChildren(input) {
		driver := effectivePrimitiveDriver(input.environment.Get("MAIL_"+child+"_DRIVER", ""), "log")
		driverSet[driver] = struct{}{}
	}
	for _, active := range appPrefixedActiveDrivers(input, "MAIL", "log", false) {
		driverSet[active.driver] = struct{}{}
	}
	drivers, err := supportedDrivers(input.environment, "MAIL", mailDriverKeys, sortStrings(driverSet))
	if err != nil {
		return nil, err
	}
	compiledDrivers := slices.Clone(drivers)
	drivers = appendMissingString(drivers, "log")
	data := mailConfigTemplateData{
		CompiledDrivers: compiledDrivers,
		Drivers:         make([]mailDriverSpec, 0, len(drivers)),
		Names:           make([]mailAccessorName, 0, len(names)),
	}
	for _, name := range names {
		data.Names = append(data.Names, mailAccessorName{
			Method: str.Of(name).Pascal().String(),
			Mailer: name,
			Field:  str.Of(name).Camel().String(),
		})
	}
	for _, driver := range drivers {
		if spec, ok := mailDriverSpecs[driver]; ok {
			data.Drivers = append(data.Drivers, spec)
		}
	}

	var b bytes.Buffer
	tmpl, err := template.New("mail-config").Parse(mailConfigSourceTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// renderMailAccessors uses the project snapshot so App-only mailers receive generated accessors.
func renderMailAccessors(input generationInput) ([]byte, error) {
	names := discoverMailNames(input)
	data := mailAccessorTemplateData{
		Names: make([]mailAccessorName, 0, len(names)),
	}
	for _, name := range names {
		data.Names = append(data.Names, mailAccessorName{
			Method: str.Of(name).Pascal().String(),
			Mailer: name,
			Field:  str.Of(name).Camel().String(),
		})
	}
	var b bytes.Buffer
	tmpl, err := template.New("mail-accessors").Parse(mailAccessorsSourceTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// discoverMailChildren includes mailers declared only through a configured App overlay.
func discoverMailChildren(input generationInput) []string {
	return discoverPrimitiveChildNames(input, "MAIL", mailRootKeys)
}

// discoverMailNames normalizes App and resource-first scopes into generated accessor names.
func discoverMailNames(input generationInput) []string {
	names := discoverMailChildren(input)
	for i := range names {
		names[i] = str.Of(names[i]).Trim().ToLower().String()
	}
	sort.Strings(names)
	return names
}

// appendMissingString adds value to values when it is not already present.
func appendMissingString(values []string, value string) []string {
	value = str.Of(value).Trim().ToLower().String()
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	values = append(values, value)
	sort.Strings(values)
	return values
}

const mailConfigSourceTemplate = `// Code generated by forj generate --mail. DO NOT EDIT.
// Run: forj generate --mail
package mail

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/goforj/env/v2"
	goforjmail "github.com/goforj/mail"
	"github.com/goforj/str/v2"
{{- range .Drivers }}
	"{{ .ImportPath }}"
{{- end }}
)

const (
	driverLog      = "log"
	driverMailgun  = "mailgun"
	driverPostmark = "postmark"
	driverResend   = "resend"
	driverSES      = "ses"
	driverSendGrid = "sendgrid"
	driverSMTP     = "smtp"
)

var compiledMailDrivers = []string{
{{- range .CompiledDrivers }}
	"{{ . }}",
{{- end }}
}

var mailRootKeys = []string{
	"DRIVER",
	"FROM_ADDRESS",
	"FROM_NAME",
	"LOG_BODIES",
	"HOST",
	"PORT",
	"USERNAME",
	"PASSWORD",
	"ENCRYPTION",
	"API_KEY",
	"SERVER_TOKEN",
	"ACCOUNT_ID",
	"ACCESS_KEY_ID",
	"SECRET_ACCESS_KEY",
	"SESSION_TOKEN",
	"REGION",
	"ENDPOINT",
	"DOMAIN",
}

// Manager owns the configured application mailer.
type Manager struct {
	driverName string
	observer Observer
	defaulter  *goforjmail.Mailer
{{- range .Names }}
	{{ .Field }} *goforjmail.Mailer
{{- end }}
}

// Instance describes a configured mailer instance.
type Instance struct {
	Name      string
	Mailer    *goforjmail.Mailer
	IsDefault bool
}

// MailSendEvent carries stable delivery dimensions so observers remain independent of concrete mail providers.
type MailSendEvent struct {
	Name     string
	Driver   string
	Err      error
	Duration time.Duration
}

// Observer decouples delivery telemetry from the generated manager and its concrete provider drivers.
type Observer interface {
	// OnMailSend gives generated mail drivers one stable hook for delivery telemetry.
	OnMailSend(ctx context.Context, event MailSendEvent)
}

// ObserverFunc adapts lightweight callbacks to the generated mail observer contract.
type ObserverFunc func(ctx context.Context, event MailSendEvent)

// OnMailSend adapts a callback so generated managers can compose it with interface-based observers.
func (fn ObserverFunc) OnMailSend(ctx context.Context, event MailSendEvent) {
	if fn != nil {
		fn(ctx, event)
	}
}

// observerChain keeps multiple telemetry integrations behind the single hook expected by generated mailers.
type observerChain []Observer

// OnMailSend preserves registration order when a delivery result is fanned out to multiple observers.
func (c observerChain) OnMailSend(ctx context.Context, event MailSendEvent) {
	for _, observer := range c {
		if observer == nil {
			continue
		}
		observer.OnMailSend(ctx, event)
	}
}

// NewManager creates the configured application mailer manager.
func NewManager() (*Manager, error) {
	return NewManagerWithObserver(nil)
}

// NewManagerWithObserver creates the configured application mailer manager with an optional send observer.
func NewManagerWithObserver(observer Observer) (*Manager, error) {
	driverName := str.Of(env.Get("MAIL_DRIVER", driverLog)).Trim().ToLower().String()
	driver, err := newDriver("default", env.WithPrefix("MAIL"), observer)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		driverName: driverName,
		observer: observer,
		defaulter: goforjmail.New(
			driver,
			goforjmail.WithDefaultFrom(defaultFromAddress(env.WithPrefix("MAIL")), defaultFromName(env.WithPrefix("MAIL"))),
		),
	}

{{- range .Names }}
	scope{{ .Method }} := env.WithPrefix("MAIL").Child(str.Of("{{ .Mailer }}").Snake().ToUpper().String())
	driver{{ .Method }}, err := newDriver("{{ .Mailer }}", scope{{ .Method }}, observer)
	if err != nil {
		return nil, err
	}
	manager.{{ .Field }} = goforjmail.New(
		driver{{ .Method }},
		goforjmail.WithDefaultFrom(defaultFromAddress(scope{{ .Method }}), defaultFromName(scope{{ .Method }})),
	)
{{- end }}

	return manager, nil
}

// WithObserver rebuilds the manager because mail drivers capture their observer when they are constructed.
func (m *Manager) WithObserver(observer Observer) (*Manager, error) {
	if observer == nil {
		return m, nil
	}
	combined := observer
	if m.observer != nil {
		switch existing := m.observer.(type) {
		case observerChain:
			combined = append(existing, observer)
		default:
			combined = observerChain{existing, observer}
		}
	}
	return NewManagerWithObserver(combined)
}

// newDriver rejects providers outside the generated manifest before credentials or transports are initialized.
func newDriver(name string, scope env.Scope, observer Observer) (goforjmail.Driver, error) {
	driverName := str.Of(scope.Get("DRIVER", driverLog)).Trim().ToLower().String()
	if driverName == "" {
		driverName = driverLog
	}
	if !mailDriverCompiled(driverName) {
		return nil, fmt.Errorf("mail: active driver %q is not built in; compiled choices: %s; run forj generate --mail after updating MAIL_SUPPORTED_DRIVERS", driverName, strings.Join(compiledMailDrivers, ", "))
	}

	wrapDriver := func(driver goforjmail.Driver) goforjmail.Driver {
		if observer == nil {
			return driver
		}
		return &observedDriver{
			name:     mailerNameLabel(name),
			driver:   mailerDriverLabel(driverName),
			inner:    driver,
			observer: observer,
		}
	}

	switch driverName {
{{- range .Drivers }}
	case {{ .ConstName }}:
{{- if .ReadsEnvOnly }}
		return wrapDriver(maillog.New(
			logOutput(),
			maillog.WithBodies(scope.GetBool("LOG_BODIES", "false")),
		)), nil
{{- else }}
		driver, err := {{ .Constructor }}({{ .ConfigType }}{
{{- range .Fields }}
			{{ .Name }}: {{ .Value }},
{{- end }}
		})
		if err != nil {
			return nil, err
		}
		return wrapDriver(driver), nil
{{- end }}
{{- end }}
	default:
		return nil, fmt.Errorf("mail: unsupported MAIL_DRIVER %q for mailer %q", driverName, name)
	}
}

// mailDriverCompiled reports whether driver is selectable in this generated artifact.
func mailDriverCompiled(driver string) bool {
	for _, compiled := range compiledMailDrivers {
		if driver == compiled {
			return true
		}
	}
	return false
}

// defaultFromAddress preserves a deliverable fallback when configuration is missing or blank.
func defaultFromAddress(scope env.Scope) string {
	fromAddress := strings.TrimSpace(scope.Get("FROM_ADDRESS", "no-reply@example.com"))
	if fromAddress == "" {
		return "no-reply@example.com"
	}
	return fromAddress
}

// defaultFromName falls back to the application name so blank mailer overrides do not erase sender identity.
func defaultFromName(scope env.Scope) string {
	fromName := strings.TrimSpace(scope.Get("FROM_NAME", env.Get("APP_NAME", "App")))
	if fromName == "" {
		return env.Get("APP_NAME", "App")
	}
	return fromName
}

// logOutput centralizes the stream used by generated log mailers so every configured instance behaves consistently.
func logOutput() *os.File {
	return os.Stdout
}

// discoverMailChildren excludes root settings so only named mailers receive generated accessors.
func discoverMailChildren() []string {
	return env.WithPrefix("MAIL").ChildNames(mailRootKeys)
}

// observedDriver decorates a provider at the driver boundary so every send path emits the same telemetry.
type observedDriver struct {
	name     string
	driver   string
	inner    goforjmail.Driver
	observer Observer
}

// Send reports the underlying delivery result without changing the driver's error semantics.
func (d *observedDriver) Send(ctx context.Context, message goforjmail.Message) error {
	if ctx == nil {
		ctx = context.Background()
	}
	startedAt := time.Now()
	err := d.inner.Send(ctx, message)
	d.observer.OnMailSend(ctx, MailSendEvent{
		Name:     d.name,
		Driver:   d.driver,
		Err:      err,
		Duration: time.Since(startedAt),
	})
	return err
}

// mailerNameLabel gives the root mailer a stable telemetry label when no explicit name is available.
func mailerNameLabel(name string) string {
	name = str.Of(name).ToLower().Trim().String()
	if name == "" {
		return "default"
	}
	return name
}

// mailerDriverLabel avoids emitting a blank telemetry dimension when driver configuration is unavailable.
func mailerDriverLabel(driver string) string {
	driver = str.Of(driver).ToLower().Trim().String()
	if driver == "" {
		return "unknown"
	}
	return driver
}
`

const mailAccessorsSourceTemplate = `// Code generated by forj generate --mail. DO NOT EDIT.
// Run: forj generate --mail
//
// These mail manager accessors are derived from the current .env file
// and environment variables available when generation ran.
// Named accessors are generated from MAIL_<NAME>_<KEY> environment variables.
package mail

import (
	goforjmail "github.com/goforj/mail"
	"github.com/goforj/str/v2"
)

// Default returns the default mailer instance derived from MAIL_* configuration.
func (m *Manager) Default() *goforjmail.Mailer {
	return m.defaulter
}

// Driver returns the configured default mail driver name derived from MAIL_* configuration.
func (m *Manager) Driver() string {
	return m.driverName
}

{{- range .Names }}
// {{ .Method }} returns the "{{ .Mailer }}" mailer instance.
func (m *Manager) {{ .Method }}() *goforjmail.Mailer {
	return m.{{ .Field }}
}

{{- end }}
// Names returns the generated mailer names derived from MAIL_* configuration.
func (m *Manager) Names() []string {
	names := []string{"default"}
{{- range .Names }}
	names = append(names, "{{ .Mailer }}")
{{- end }}
	return names
}

// Instances returns the generated mailer instances derived from MAIL_* configuration.
func (m *Manager) Instances() []Instance {
	instances := []Instance{
		{Name: "default", Mailer: m.defaulter, IsDefault: true},
	}
{{- range .Names }}
	instances = append(instances, Instance{Name: "{{ .Mailer }}", Mailer: m.{{ .Field }}})
{{- end }}
	return instances
}

// Named returns the generated mailer instance for a configured mailer name.
func (m *Manager) Named(name string) *goforjmail.Mailer {
	switch str.Of(name).Trim().ToLower().String() {
	case "", "default":
		return m.defaulter
{{- range .Names }}
	case "{{ .Mailer }}":
		return m.{{ .Field }}
{{- end }}
	default:
		return nil
	}
}
`

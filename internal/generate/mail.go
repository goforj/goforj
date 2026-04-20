package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"text/template"

	"github.com/goforj/env/v2"
	"github.com/goforj/str"
)

type mailConfigTemplateData struct {
	Drivers []mailDriverSpec
	Names   []mailAccessorName
}

type mailAccessorName struct {
	Method string
	Mailer string
	Field  string
}

type mailDriverSpec struct {
	ConstName    string
	ImportPath   string
	Constructor  string
	ConfigType   string
	Fields       []mailConfigField
	ReadsEnvOnly bool
}

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

func GenerateMailFiles(projectDir string) (int, error) {
	if err := validatePrimitiveEnv(primitiveEnvContract{
		Prefix:        "MAIL",
		DefaultDriver: "log",
		RootKeys:      mailRootKeys,
		CommonKeys:    mailCommonKeys,
		DriverKeys:    mailDriverKeys,
		ChildNames: func(scope env.Scope) []string {
			return exactScopedChildNames("MAIL", mailRootKeys)
		},
		AllowInactiveRootKeys: true,
	}); err != nil {
		return 0, err
	}

	manager, err := renderMailConfig()
	if err != nil {
		return 0, err
	}
	formattedManager, err := format.Source(manager)
	if err != nil {
		return 0, fmt.Errorf("failed to format generated mail manager: %w", err)
	}

	written := 0
	changed, err := writeGeneratedSource(filepath.Join(projectDir, "internal", "mail", "manager_gen.go"), formattedManager)
	if err != nil {
		return written, err
	}
	if changed {
		written++
	}
	_ = os.Remove(filepath.Join(projectDir, "internal", "mail", "manager.go"))
	return written, nil
}

func renderMailConfig() ([]byte, error) {
	names := discoverMailNames()
	driverSet := map[string]struct{}{}
	defaultDriver := str.Of(env.Get("MAIL_DRIVER", "log")).TrimSpace().ToLower().String()
	if defaultDriver != "" {
		driverSet[defaultDriver] = struct{}{}
	}
	for _, child := range discoverMailChildren() {
		driver := str.Of(env.Get("MAIL_"+child+"_DRIVER", "")).TrimSpace().ToLower().String()
		if driver != "" {
			driverSet[driver] = struct{}{}
		}
	}
	drivers, err := supportedDrivers("MAIL", mailDriverKeys, sortStrings(driverSet))
	if err != nil {
		return nil, err
	}
	data := mailConfigTemplateData{
		Drivers: make([]mailDriverSpec, 0, len(drivers)),
		Names:   make([]mailAccessorName, 0, len(names)),
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

func discoverMailChildren() []string {
	return exactScopedChildNames("MAIL", mailRootKeys)
}

func discoverMailNames() []string {
	names := discoverMailChildren()
	for i := range names {
		names[i] = str.Of(names[i]).TrimSpace().ToLower().String()
	}
	sort.Strings(names)
	return names
}

const mailConfigSourceTemplate = `// Code generated by forj generate --mail. DO NOT EDIT.
// Run: forj generate --mail
package mail

import (
	"fmt"
	"os"
	"strings"

	"github.com/goforj/env/v2"
	goforjmail "github.com/goforj/mail"
	"github.com/goforj/str"
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

// Manager owns the configured application mailer.
type Manager struct {
	driverName string
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

// NewManager creates the configured application mailer manager.
func NewManager() (*Manager, error) {
	driverName := strings.ToLower(strings.TrimSpace(env.Get("MAIL_DRIVER", driverLog)))
	driver, err := newDriver("default", env.WithPrefix("MAIL"))
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		driverName: driverName,
		defaulter: goforjmail.New(
			driver,
			goforjmail.WithDefaultFrom(defaultFromAddress(env.WithPrefix("MAIL")), defaultFromName(env.WithPrefix("MAIL"))),
		),
	}

{{- if .Names }}
	for _, child := range discoverMailChildren() {
		name := str.Of(child).TrimSpace().ToLower().String()
		if name == "" {
			continue
		}
		scope := env.WithPrefix("MAIL").Child(child)
		driver, err := newDriver(name, scope)
		if err != nil {
			return nil, err
		}
		mailer := goforjmail.New(
			driver,
			goforjmail.WithDefaultFrom(defaultFromAddress(scope), defaultFromName(scope)),
		)
		switch name {
{{- range .Names }}
		case "{{ .Mailer }}":
			manager.{{ .Field }} = mailer
{{- end }}
		}
	}
{{- end }}

	return manager, nil
}

// Default returns the configured default mailer.
func (m *Manager) Default() *goforjmail.Mailer {
	return m.defaulter
}

// Driver returns the configured driver name.
func (m *Manager) Driver() string {
	return m.driverName
}

// Names returns the configured mailer names.
func (m *Manager) Names() []string {
	names := []string{"default"}
{{- range .Names }}
	names = append(names, "{{ .Mailer }}")
{{- end }}
	return names
}

// Instances returns the configured mailer instances.
func (m *Manager) Instances() []Instance {
	if m == nil {
		return nil
	}
	instances := []Instance{
		{Name: "default", Mailer: m.defaulter, IsDefault: true},
	}
{{- range .Names }}
	instances = append(instances, Instance{Name: "{{ .Mailer }}", Mailer: m.{{ .Field }}})
{{- end }}
	return instances
}

// Named returns the configured named mailer or nil when absent.
func (m *Manager) Named(name string) *goforjmail.Mailer {
	switch str.Of(name).TrimSpace().ToLower().String() {
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

{{- range .Names }}
// {{ .Method }} returns the "{{ .Mailer }}" mailer instance.
func (m *Manager) {{ .Method }}() *goforjmail.Mailer {
	return m.{{ .Field }}
}

{{- end }}
// newDriver is generated from MAIL_SUPPORTED_DRIVERS, or from active MAIL_* and MAIL_<NAME>_* values when unset.
func newDriver(name string, scope env.Scope) (goforjmail.Driver, error) {
	driverName := str.Of(scope.Get("DRIVER", driverLog)).TrimSpace().ToLower().String()
	if driverName == "" {
		driverName = driverLog
	}

	switch driverName {
{{- range .Drivers }}
	case {{ .ConstName }}:
{{- if .ReadsEnvOnly }}
		return maillog.New(
			logOutput(),
			maillog.WithBodies(scope.GetBool("LOG_BODIES", "false")),
		), nil
{{- else }}
		return {{ .Constructor }}({{ .ConfigType }}{
{{- range .Fields }}
			{{ .Name }}: {{ .Value }},
{{- end }}
		})
{{- end }}
{{- end }}
	default:
		return nil, fmt.Errorf("mail: unsupported MAIL_DRIVER %q for mailer %q", driverName, name)
	}
}

func defaultFromAddress(scope env.Scope) string {
	fromAddress := strings.TrimSpace(scope.Get("FROM_ADDRESS", "no-reply@example.com"))
	if fromAddress == "" {
		return "no-reply@example.com"
	}
	return fromAddress
}

func defaultFromName(scope env.Scope) string {
	fromName := strings.TrimSpace(scope.Get("FROM_NAME", env.Get("APP_NAME", "App")))
	if fromName == "" {
		return env.Get("APP_NAME", "App")
	}
	return fromName
}

func logOutput() *os.File {
	return os.Stdout
}
`

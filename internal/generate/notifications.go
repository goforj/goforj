package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/goforj/goforj/project"
	"github.com/goforj/str/v2"
)

// notificationAccessorName keeps configured names separate from generated Go identifiers.
type notificationAccessorName struct{ Name, Method string }

// notificationScope records named sender ownership independently from generated method availability.
type notificationScope struct {
	Prefix string
	Names  []string
}

// notificationTemplateData limits imports to the supported provider manifest.
type notificationTemplateData struct {
	Scopes  []notificationScope
	Names   []notificationAccessorName
	Drivers []string
	Webhook bool
}

var notificationRootKeys = []string{"DRIVER", "WEBHOOK_URL", "WEBHOOK_FORMAT", "WEBHOOK_TIMEOUT", "WEBHOOK_BEARER_TOKEN", "WEBHOOK_ALLOW_PRIVATE"}
var notificationCommonKeys = makeSet("DRIVER")
var notificationDriverKeys = map[string]map[string]struct{}{
	"log":     makeSet(),
	"webhook": makeSet("WEBHOOK_URL", "WEBHOOK_FORMAT", "WEBHOOK_TIMEOUT", "WEBHOOK_BEARER_TOKEN", "WEBHOOK_ALLOW_PRIVATE"),
}
var notificationLocalDrivers = []string{"log"}

// discoverNotificationNames exposes the build's union of typed accessors to configuration tooling.
func discoverNotificationNames(input generationInput) []string {
	names := discoverPrimitiveChildNames(input, "NOTIFICATION", notificationRootKeys)
	for index := range names {
		names[index] = strings.ToLower(names[index])
	}
	return names
}

// GenerateNotificationFiles captures the same environment boundary used by other resource generators.
func GenerateNotificationFiles(projectDir string) (int, error) {
	return generateNotificationFiles(ambientGenerationInput(projectDir))
}

// generateNotificationFiles publishes both artifacts only after validating the complete snapshot.
func generateNotificationFiles(input generationInput) (int, error) {
	if err := validatePrimitiveEnv(input, primitiveEnvContract{
		Prefix: "NOTIFICATION", DefaultDriver: "log", LocalDrivers: notificationLocalDrivers,
		RootKeys: notificationRootKeys, CommonKeys: notificationCommonKeys, DriverKeys: notificationDriverKeys,
		ChildNames: func(environment generationEnvironment) []string {
			return exactScopedChildNames(environment, "NOTIFICATION", notificationRootKeys)
		},
		AllowInactiveRootKeys: true, EagerNamedResources: true,
	}); err != nil {
		return 0, err
	}
	active := map[string]struct{}{effectivePrimitiveDriver(input.environment.Get("NOTIFICATION_DRIVER", "log"), "log"): {}}
	names := discoverPrimitiveChildNames(input, "NOTIFICATION", notificationRootKeys)
	data := notificationTemplateData{}
	reserved := makeSet("Default", "Named", "Names", "Instances", "Close")
	for _, name := range names {
		method := str.Of(name).Pascal().String()
		if _, found := reserved[method]; found || !token.IsIdentifier(method) || !token.IsExported(method) {
			return 0, fmt.Errorf("notification: resource name %q cannot be an accessor", name)
		}
		data.Names = append(data.Names, notificationAccessorName{Name: strings.ToLower(name), Method: method})
		active[effectivePrimitiveDriver(input.environment.Get("NOTIFICATION_"+name+"_DRIVER", "log"), "log")] = struct{}{}
	}
	for _, driver := range appPrefixedActiveDrivers(input, "NOTIFICATION", "log", false) {
		active[driver.driver] = struct{}{}
	}
	drivers, err := supportedDrivers(input.environment, "NOTIFICATION", notificationDriverKeys, sortStrings(active), notificationLocalDrivers)
	if err != nil {
		return 0, err
	}
	rootNames := exactScopedChildNames(input.environment, "NOTIFICATION", notificationRootKeys)
	defaultEnabled := true
	if config, err := project.LoadProjectConfigAt(input.projectDir); err == nil {
		defaultEnabled = config.Render.Components.Notification
	} else if !os.IsNotExist(err) {
		return 0, fmt.Errorf("load notification App ownership: %w", err)
	}
	if defaultEnabled {
		data.Scopes = append(data.Scopes, notificationScope{Names: rootNames})
	}
	for _, prefix := range generationAppEnvPrefixesForResource(input, "NOTIFICATION") {
		names := makeSet(rootNames...)
		for _, name := range exactScopedChildNames(input.environment, prefix+"_NOTIFICATION", notificationRootKeys) {
			names[name] = struct{}{}
		}
		data.Scopes = append(data.Scopes, notificationScope{Prefix: prefix, Names: sortStrings(names)})
	}
	data.Drivers = drivers
	for _, driver := range drivers {
		if driver == "webhook" {
			data.Webhook = true
		}
	}
	outputs := []struct{ path, source string }{{NotificationManagerPath, notificationManagerSource}, {NotificationAccessorsPath, notificationAccessorsSource}}
	bodies := make([][]byte, len(outputs))
	for index, output := range outputs {
		tmpl, err := template.New(output.path).Parse(output.source)
		if err != nil {
			return 0, err
		}
		var body bytes.Buffer
		if err := tmpl.Execute(&body, data); err != nil {
			return 0, err
		}
		bodies[index], err = format.Source(body.Bytes())
		if err != nil {
			return 0, fmt.Errorf("format notification source: %w", err)
		}
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

const notificationManagerSource = `// Code generated by forj generate --notification. DO NOT EDIT.
// Package notifications owns configured synchronous senders and their transports.
package notifications

import (
    "errors"
    "fmt"
    "io"
    "log/slog"
    "strings"
    "sync"
{{- if .Webhook }}
    "strconv"
    "time"
    "github.com/goforj/notification/driver/webhook"
{{- end }}
    "github.com/goforj/env/v2"
    "github.com/goforj/notification"
    "github.com/goforj/notification/driver/log"
)

var compiledNotificationDrivers = []string{
{{range .Drivers}}    "{{.}}",
{{end}}}

var notificationAppResources = map[string][]string{
{{range .Scopes}} "{{.Prefix}}": { {{range .Names}} "{{.}}", {{end}} }, {{end}}
}

// Manager owns only transports belonging to this App's compiled resource scope.
// Services borrow senders without hidden retries or background work.
type Manager struct {
    defaultSender notification.Sender
    senders map[string]notification.Sender
    names []string
    closers []io.Closer
    closeOnce sync.Once
    closeError error
}

// Instance exposes a configured sender without its credentials or destination.
type Instance struct { Name string; Sender notification.Sender; IsDefault bool }

// NewManager initializes only the default App's declared senders without network traffic.
func NewManager() (*Manager, error) { return NewManagerForApp("") }

// NewManagerForApp initializes root-declared resources and names owned by the selected App.
func NewManagerForApp(prefix string) (*Manager, error) {
    prefix = strings.ToUpper(strings.TrimSpace(prefix))
    names, enabled := notificationAppResources[prefix]
    if !enabled { return nil, fmt.Errorf("notification resources are not enabled for App prefix %q", prefix) }
    manager := &Manager{senders: map[string]notification.Sender{}}
    sender, closer, err := newSender(prefix, "default")
    if err != nil { return nil, err }
    manager.defaultSender = sender
    if closer != nil { manager.closers = append(manager.closers, closer) }
    for _, name := range names {
        name = strings.ToLower(name)
        sender, closer, err = newSender(prefix, name)
        if err != nil { return nil, errors.Join(err, manager.Close()) }
        manager.senders[name] = sender
        manager.names = append(manager.names, name)
        if closer != nil { manager.closers = append(manager.closers, closer) }
    }
    return manager, nil
}

// notificationValue resolves existing App overlays without mutating process configuration.
func notificationValue(prefix, name, key, fallback string) string {
    scope := env.WithPrefix("NOTIFICATION")
    if name != "default" { scope = scope.Child(strings.ToUpper(name)) }
    value := scope.Get(key, fallback)
    if prefix != "" {
        appScope := env.WithPrefix(prefix).Child("NOTIFICATION")
        if name != "default" { appScope = appScope.Child(strings.ToUpper(name)) }
        value = appScope.Get(key, value)
    }
    return value
}

// sender fails fast if App wiring requests a typed accessor owned by another App.
func (manager *Manager) sender(name string) notification.Sender {
    sender, exists := manager.senders[name]
    if !exists { panic("notification sender is not configured for this App: " + name) }
    return sender
}

// Close releases each owned transport once, including partial initialization cleanup.
func (manager *Manager) Close() error {
    manager.closeOnce.Do(func() {
        for index := len(manager.closers)-1; index >= 0; index-- {
            manager.closeError = errors.Join(manager.closeError, manager.closers[index].Close())
        }
    })
    return manager.closeError
}

// newSender rejects providers outside the compiled manifest before creating transports.
func newSender(prefix, name string) (notification.Sender, io.Closer, error) {
    value := func(key, fallback string) string { return notificationValue(prefix, name, key, fallback) }
    driver := strings.ToLower(strings.TrimSpace(value("DRIVER", "log")))
    if driver == "" { driver = "log" }
    compiled := false
    for _, name := range compiledNotificationDrivers { if name == driver { compiled = true; break } }
    if !compiled { return nil, nil, fmt.Errorf("notification: active driver %q is not built in; update NOTIFICATION_SUPPORTED_DRIVERS and run forj generate --notification", driver) }
    switch driver {
    case "log":
        return notification.New(log.New(slog.Default())), nil, nil
{{- if .Webhook }}
    case "webhook":
        timeout, err := time.ParseDuration(value("WEBHOOK_TIMEOUT", "5s"))
        if err != nil || timeout <= 0 { return nil, nil, fmt.Errorf("notification: WEBHOOK_TIMEOUT must be a positive duration") }
        allowPrivate, err := strconv.ParseBool(value("WEBHOOK_ALLOW_PRIVATE", "false"))
        if err != nil { return nil, nil, fmt.Errorf("notification: WEBHOOK_ALLOW_PRIVATE must be a boolean") }
        transport, err := webhook.New(webhook.Config{
            URL: strings.TrimSpace(value("WEBHOOK_URL", "")),
            Format: webhook.Format(strings.TrimSpace(value("WEBHOOK_FORMAT", "generic"))),
            Timeout: timeout, AllowPrivate: allowPrivate,
            BearerToken: value("WEBHOOK_BEARER_TOKEN", ""),
        })
        if err != nil { return nil, nil, fmt.Errorf("notification: configure webhook: %w", err) }
        return notification.New(transport), transport, nil
{{- end }}
    default:
        return nil, nil, fmt.Errorf("notification: unsupported driver %q", driver)
    }
}
`

const notificationAccessorsSource = `// Code generated by forj generate --notification. DO NOT EDIT.
package notifications

import (
    "strings"
    "github.com/goforj/notification"
)

// Default returns this App's configured default sender.
func (manager *Manager) Default() notification.Sender { return manager.defaultSender }
{{range .Names}}
// {{.Method}} returns the configured "{{.Name}}" sender.
func (manager *Manager) {{.Method}}() notification.Sender { return manager.sender("{{.Name}}") }
{{end}}
// Named returns a configured sender, or nil when this App does not own the name.
func (manager *Manager) Named(name string) notification.Sender {
    name = strings.ToLower(strings.TrimSpace(name))
    if name == "" || name == "default" { return manager.defaultSender }
    return manager.senders[name]
}

// Names returns this App's declared sender names in generation order.
func (manager *Manager) Names() []string { return append([]string{"default"}, manager.names...) }

// Instances returns borrowed senders belonging to this App without transferring ownership.
func (manager *Manager) Instances() []Instance {
    instances := []Instance{ {Name: "default", Sender: manager.defaultSender, IsDefault: true} }
    for _, name := range manager.names { instances = append(instances, Instance{Name: name, Sender: manager.senders[name]}) }
    return instances
}

`
